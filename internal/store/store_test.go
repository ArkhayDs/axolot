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

func TestMapCumulatesAnswersPerQuestion(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mac, pc := json.RawMessage(`{"label":"mac"}`), json.RawMessage(`{"label":"pc"}`)
	play := func(hardware json.RawMessage, status, item, category string, score float64, totalMs int64) string {
		t.Helper()
		id, err := s.CreateCampaign(ctx, "demo", hardware, json.RawMessage(`{}`), 1)
		if err != nil {
			t.Fatal(err)
		}
		run := Run{CampaignID: id, CaseID: "c", ItemID: item, Category: category, ProviderName: "p", Model: "m", TotalMs: totalMs, Score: score, Checks: json.RawMessage(`[]`), ItemHash: "h-" + item}
		if err := s.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		if err := s.FinishCampaign(ctx, id, status); err != nil {
			t.Fatal(err)
		}
		return id
	}
	play(mac, StatusDone, "i1", "code", 0, 100)
	cancelled := play(mac, StatusCancelled, "i1", "code", 100, 200)
	play(mac, StatusInterrupted, "i1", "code", 100, 300)
	play(mac, StatusDone, "i2", "rag", 100, 400)
	play(pc, StatusDone, "i1", "code", 20, 500)
	play(mac, StatusRunning, "i2", "rag", 0, 600)
	play(mac, StatusDone, "old", "rag", 0, 700)

	hashes := []string{"h-i1", "h-i2"}
	cells, err := s.MapCells(ctx, hashes)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]MapCell{}
	for _, cell := range cells {
		found[cell.Machine+"|"+cell.Category] = cell
	}
	if len(cells) != 8 {
		t.Fatalf("cells=%+v", cells)
	}
	if cell := found["mac|code"]; cell.Runs != 3 || cell.Questions != 1 || *cell.P50TotalMs != 200 {
		t.Fatalf("every answer counts, cancelled ones included: %+v", cell)
	}
	if cell := found["mac|rag"]; cell.Runs != 1 || cell.AvgScore != 100 {
		t.Fatalf("running campaign or stale version counted: %+v", cell)
	}
	// Questions weigh the same however often they were played: (66.67 + 100) / 2, not the 75 of the raw answers.
	if cell := found["mac|"]; cell.Runs != 4 || cell.Questions != 2 || cell.AvgScore < 83.3 || cell.AvgScore > 83.4 {
		t.Fatalf("machine rollup: %+v", cell)
	}
	if cell := found["|"]; cell.Runs != 5 || cell.Questions != 2 || cell.P95TotalMs == nil {
		t.Fatalf("model rollup: %+v", cell)
	}
	estimates, err := s.EstimateRows(ctx, "mac", hashes)
	if err != nil {
		t.Fatal(err)
	}
	means := map[string]float64{}
	for _, row := range estimates {
		means[row.Category+"|"+row.ItemID] = row.AvgMs
	}
	if means["code|i1"] != 200 || means["code|"] != 200 || means["rag|i2"] != 400 || len(estimates) != 5 {
		t.Fatalf("estimates=%+v", estimates)
	}
	answers, err := s.MapAnswers(ctx, hashes)
	if err != nil || len(answers) != 5 {
		t.Fatalf("answers=%+v err=%v", answers, err)
	}

	benchmarks, err := s.MapBenchmarks(ctx)
	if err != nil || len(benchmarks) != 7 {
		t.Fatalf("benchmarks=%+v err=%v", benchmarks, err)
	}
	for _, benchmark := range benchmarks {
		if benchmark.ID != cancelled {
			continue
		}
		if benchmark.Machine != "mac" || benchmark.DoneItems != 1 || benchmark.Models[0] != "m" || benchmark.Categories[0] != "code" {
			t.Fatalf("benchmark=%+v", benchmark)
		}
		var picked []struct {
			Model    string  `json:"model"`
			AvgScore float64 `json:"avg_score"`
		}
		if err := json.Unmarshal(benchmark.Cells, &picked); err != nil || len(picked) != 1 || picked[0].AvgScore != 100 {
			t.Fatalf("cells=%s err=%v", benchmark.Cells, err)
		}
	}
}
