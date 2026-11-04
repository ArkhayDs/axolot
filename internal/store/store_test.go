package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/arkhayds/axolot/internal/rag"
)

// openTestStore runs against a throwaway schema; skipped without TEST_DATABASE_URL.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, base+"?search_path="+schema+",public")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(ctx)
	})
	return s
}

func TestSettingsDefaultThenSaved(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	got, err := s.Settings(ctx)
	if err != nil || got != DefaultSettings() {
		t.Fatalf("got %+v err %v", got, err)
	}
	got.TopK = 9
	if err := s.SaveSettings(ctx, got); err != nil {
		t.Fatal(err)
	}
	if saved, _ := s.Settings(ctx); saved.TopK != 9 {
		t.Fatalf("saved %+v", saved)
	}
}

func TestProviderSelectionCascade(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	id, err := s.CreateProvider(ctx, "local", "ollama", "http://x", []byte("sealed"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCaseModels(ctx, "c", []Selection{{CaseID: "c", ProviderID: id, Model: "m"}}); err != nil {
		t.Fatal(err)
	}
	selections, err := s.Selections(ctx)
	if err != nil || len(selections) != 1 {
		t.Fatalf("selections=%v err=%v", selections, err)
	}
	providers, err := s.Providers(ctx)
	if err != nil || !providers[0].HasAPIKey {
		t.Fatalf("providers=%+v err=%v", providers, err)
	}
	if err := s.DeleteProvider(ctx, id); err != nil {
		t.Fatal(err)
	}
	if selections, _ := s.Selections(ctx); len(selections) != 0 {
		t.Fatalf("selection survived: %v", selections)
	}
}

func TestChunkSearchRanksAndIsolatesModels(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	chunks := []rag.Chunk{{File: "a.md", Content: "a"}, {File: "b.md", Position: 1, Content: "b"}}
	if err := s.ReplaceChunks(ctx, "demo", "m1", chunks, [][]float32{{1, 0}, {0, 1}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceChunks(ctx, "demo", "m2", chunks[:1], [][]float32{{1, 0, 0}}); err != nil {
		t.Fatal(err)
	}
	hits, err := s.SearchChunks(ctx, "demo", "m1", []float32{0, 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].File != "b.md" || hits[0].Similarity < 0.99 {
		t.Fatalf("hits=%+v", hits)
	}
	if err := s.ReplaceChunks(ctx, "demo", "m1", chunks[:1], [][]float32{{1, 0}}); err != nil {
		t.Fatal(err)
	}
	indexes, err := s.CorpusIndexes(ctx)
	if err != nil || len(indexes) != 2 || indexes[0].Chunks != 1 {
		t.Fatalf("indexes=%+v err=%v", indexes, err)
	}
}

func TestCampaignRunsAndStats(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	id, err := s.CreateCampaign(ctx, "demo", json.RawMessage(`{"label":"mac"}`), json.RawMessage(`{}`), 3)
	if err != nil {
		t.Fatal(err)
	}
	ttft, rate, hit := int64(100), 40.0, true
	base := Run{CampaignID: id, CaseID: "c", ProviderName: "p", Model: "m", Checks: json.RawMessage(`[]`), TTFTMs: &ttft, TokPerSec: &rate}
	for i, category := range []string{"rag", "rag", "code"} {
		run := base
		run.ItemID, run.Category, run.TotalMs, run.Score = fmt.Sprintf("i%d", i), category, int64(100*(i+1)), 50
		if category == "rag" {
			run.RetrievalHit = &hit
			run.Retrieved = json.RawMessage(`[{"file":"a.md"}]`)
		}
		if i == 2 {
			run.Error, run.Score = "out of memory", 0
		}
		if err := s.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
	}

	campaign, err := s.CampaignByID(ctx, id)
	if err != nil || campaign.DoneItems != 3 || campaign.Status != StatusRunning {
		t.Fatalf("campaign=%+v err=%v", campaign, err)
	}
	stats, err := s.Stats(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 3 || stats[0].Category != "" || stats[0].Runs != 3 || stats[0].Errors != 1 {
		t.Fatalf("stats=%+v", stats)
	}
	if *stats[0].P50TotalMs != 200 || stats[0].RetrievalHits != 2 || stats[0].RetrievalTotal != 2 {
		t.Fatalf("overall=%+v", stats[0])
	}
	runs, err := s.Runs(ctx, id)
	if err != nil || len(runs) != 3 {
		t.Fatalf("runs=%d err=%v", len(runs), err)
	}

	if err := s.MarkInterrupted(ctx); err != nil {
		t.Fatal(err)
	}
	if campaign, _ = s.CampaignByID(ctx, id); campaign.Status != StatusInterrupted || campaign.FinishedAt == nil {
		t.Fatalf("campaign=%+v", campaign)
	}
	if _, err := s.CampaignByID(ctx, "00000000-0000-0000-0000-000000000000"); err != ErrNotFound {
		t.Fatalf("err=%v", err)
	}
}
