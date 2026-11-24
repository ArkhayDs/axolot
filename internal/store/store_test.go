package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/arkhayds/axolot/internal/rag"
)

// scoreOf reads a nullable average, -1 standing for none.
func scoreOf(score *float64) float64 {
	if score == nil {
		return -1
	}
	return *score
}

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

func TestCreateProviderRefusesTakenName(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, err := s.CreateProvider(ctx, "local", "ollama", "http://x", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProvider(ctx, "local", "openai", "http://y", nil); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("duplicate name: err=%v", err)
	}
}

func TestMcpServerLifecycle(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	id, err := s.CreateMcpServer(ctx, "sentinelle", "https://x/mcp", []byte("sealed"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMcpServerEnabled(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	servers, err := s.McpServers(ctx)
	if err != nil || len(servers) != 1 || servers[0].Enabled || len(servers[0].TokenEnc) == 0 {
		t.Fatalf("servers=%+v err=%v", servers, err)
	}
	if m, err := s.McpServer(ctx, id); err != nil || m.Name != "sentinelle" || m.Enabled {
		t.Fatalf("server=%+v err=%v", m, err)
	}
	if err := s.DeleteMcpServer(ctx, id); err != nil {
		t.Fatal(err)
	}
	if servers, _ := s.McpServers(ctx); len(servers) != 0 {
		t.Fatalf("server survived: %v", servers)
	}
	if _, err := s.McpServer(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("load deleted server: err=%v", err)
	}
	if err := s.SetMcpServerEnabled(ctx, id, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("toggle deleted server: err=%v", err)
	}
	if err := s.DeleteMcpServer(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete deleted server: err=%v", err)
	}
}

func TestSetMcpServerOAuthReplacesSavedCredentials(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	id, err := s.CreateMcpServer(ctx, "sentinelle", "https://x/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, sealed := range []string{"first", "rotated"} {
		if err := s.SetMcpServerOAuth(ctx, id, []byte(sealed)); err != nil {
			t.Fatal(err)
		}
	}
	if m, err := s.McpServer(ctx, id); err != nil || string(m.OAuthEnc) != "rotated" {
		t.Fatalf("server=%+v err=%v", m, err)
	}
	if err := s.SetMcpServerOAuth(ctx, id+1, []byte("x")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown server: err=%v", err)
	}
}

func TestCreateMcpServerRefusesTakenName(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, err := s.CreateMcpServer(ctx, "sentinelle", "https://x/mcp", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateMcpServer(ctx, "sentinelle", "https://y/mcp", nil); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("duplicate name: err=%v", err)
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
	ttft, rate, hit, startup, exit := int64(100), 40.0, true, int64(30), int64(20)
	base := Run{CampaignID: id, CaseID: "c", ProviderName: "p", ConnectionType: ConnectionLocal, Model: "m", Checks: json.RawMessage(`[]`),
		TTFTMs: &ttft, TokPerSec: &rate, CLIStartupMs: &startup, CLIExitMs: &exit}
	for i, category := range []string{"rag", "rag", "code"} {
		run := base
		run.ItemID, run.Category, run.TotalMs, run.Score = fmt.Sprintf("i%d", i), category, int64(100*(i+1)), 50
		if category == "rag" {
			run.RetrievalHit = &hit
			run.Retrieved = json.RawMessage(`[{"file":"a.md"}]`)
		}
		if i == 1 {
			run.CLIStartupMs, run.DefaultTemperature = nil, true
		}
		if i == 2 {
			run.Error, run.Score, run.TotalMs, run.ConnectionType, run.DefaultTemperature = "out of memory", 0, 1, "", true
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
	if *stats[0].P50TotalMs != 150 || stats[0].RetrievalHits != 2 || stats[0].RetrievalTotal != 2 {
		t.Fatalf("overall=%+v", stats[0])
	}
	if *stats[0].P50NetTTFTMs != 70 || *stats[0].P50NetTotalMs != 50 || stats[1].P50NetTotalMs != nil {
		t.Fatalf("net medians take the CLI out of fully measured answers only, a theme without measure has none: overall=%+v code=%+v", stats[0], stats[1])
	}
	if scoreOf(stats[0].AvgScore) != 50 || stats[1].Category != "code" || stats[1].AvgScore != nil {
		t.Fatalf("an unmeasured answer must stay out of the average, a theme without measure has none: overall=%+v code=%+v", stats[0], stats[1])
	}
	if !stats[0].DefaultTemperature || stats[1].DefaultTemperature || !stats[2].DefaultTemperature {
		t.Fatalf("one measured answer at the default temperature marks its group: overall=%v code=%v rag=%v",
			stats[0].DefaultTemperature, stats[1].DefaultTemperature, stats[2].DefaultTemperature)
	}
	runs, err := s.Runs(ctx, id)
	if err != nil || len(runs) != 3 {
		t.Fatalf("runs=%d err=%v", len(runs), err)
	}
	if *runs[0].CLIStartupMs != 30 || *runs[0].CLIExitMs != 20 {
		t.Fatalf("cli startup=%v exit=%v", runs[0].CLIStartupMs, runs[0].CLIExitMs)
	}
	if runs[0].DefaultTemperature || !runs[1].DefaultTemperature {
		t.Fatalf("default temperature %v, %v", runs[0].DefaultTemperature, runs[1].DefaultTemperature)
	}
	if runs[0].ConnectionType != ConnectionLocal || runs[2].ConnectionType != "" {
		t.Fatalf("an untyped answer reads back empty: connection types %q, %q", runs[0].ConnectionType, runs[2].ConnectionType)
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
	play := func(hardware json.RawMessage, status, item, category string, score float64, totalMs int64, failure string) string {
		t.Helper()
		id, err := s.CreateCampaign(ctx, "demo", hardware, json.RawMessage(`{}`), 1)
		if err != nil {
			t.Fatal(err)
		}
		run := Run{CampaignID: id, CaseID: "c", ItemID: item, Category: category, ProviderName: "p", Model: "m", TotalMs: totalMs, Score: score, Error: failure, Checks: json.RawMessage(`[]`), ItemHash: "h-" + item}
		if err := s.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		if err := s.FinishCampaign(ctx, id, status); err != nil {
			t.Fatal(err)
		}
		return id
	}
	play(mac, StatusDone, "i1", "code", 0, 100, "")
	cancelled := play(mac, StatusCancelled, "i1", "code", 100, 200, "")
	play(mac, StatusInterrupted, "i1", "code", 100, 300, "")
	play(mac, StatusDone, "i2", "rag", 100, 400, "")
	unmeasured := play(mac, StatusDone, "i2", "rag", 0, 1, "quota exceeded")
	play(pc, StatusDone, "i1", "code", 20, 500, "")
	play(pc, StatusDone, "i2", "rag", 0, 500, "quota exceeded")
	play(mac, StatusRunning, "i2", "rag", 0, 600, "")
	play(mac, StatusDone, "old", "rag", 0, 700, "")

	hashes := []string{"h-i1", "h-i2"}
	cells, err := s.MapCells(ctx, hashes, "")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]MapCell{}
	for _, cell := range cells {
		found[cell.Machine+"|"+cell.Category] = cell
	}
	if len(cells) != 9 {
		t.Fatalf("cells=%+v", cells)
	}
	if cell := found["mac|code"]; cell.Runs != 3 || cell.Questions != 1 || *cell.P50TotalMs != 200 || cell.P50NetTotalMs != nil {
		t.Fatalf("every answer counts, cancelled ones included: %+v", cell)
	}
	if cell := found["mac|rag"]; cell.Runs != 2 || cell.Errors != 1 || scoreOf(cell.AvgScore) != 100 || *cell.P50TotalMs != 400 {
		t.Fatalf("running campaign, stale version or unmeasured answer counted in the score or times: %+v", cell)
	}
	// Questions weigh the same however often they were played: (66.67 + 100) / 2, not the 75 of the raw answers.
	if cell := found["mac|"]; cell.Runs != 5 || cell.Questions != 2 || scoreOf(cell.AvgScore) < 83.3 || scoreOf(cell.AvgScore) > 83.4 {
		t.Fatalf("machine rollup: %+v", cell)
	}
	if cell := found["pc|rag"]; cell.Runs != 1 || cell.AvgScore != nil || cell.P50TotalMs != nil {
		t.Fatalf("a theme without measure has no score nor time: %+v", cell)
	}
	if cell := found["|"]; cell.Runs != 7 || cell.Questions != 2 || cell.P95TotalMs == nil {
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
	items, err := s.MapItems(ctx, hashes, "")
	if err != nil {
		t.Fatal(err)
	}
	perItem := map[string]MapItem{}
	for _, item := range items {
		perItem[item.Machine+"|"+item.CaseID+"|"+item.ItemID] = item
	}
	if len(items) != 6 {
		t.Fatalf("items=%+v", items)
	}
	if item := perItem["mac|c|i1"]; item.Runs != 3 || scoreOf(item.AvgScore) < 66.6 || scoreOf(item.AvgScore) > 66.7 || *item.P50TotalMs != 200 || item.Errors != 0 {
		t.Fatalf("question on one machine: %+v", item)
	}
	if item := perItem["|c|i1"]; item.Runs != 4 || scoreOf(item.AvgScore) < 43.3 || scoreOf(item.AvgScore) > 43.4 {
		t.Fatalf("question over machines, machines weighing the same: %+v", item)
	}
	if item := perItem["mac|c|i2"]; item.Runs != 2 || item.Errors != 1 || scoreOf(item.AvgScore) != 100 || *item.P50TotalMs != 400 {
		t.Fatalf("running campaign, stale version or unmeasured answer counted in the score or time: %+v", item)
	}
	if item := perItem["pc|c|i2"]; item.Runs != 1 || item.AvgScore != nil {
		t.Fatalf("a question without measure has no score: %+v", item)
	}
	if item := perItem["|c|i2"]; scoreOf(item.AvgScore) != 100 {
		t.Fatalf("running campaign, stale version or unmeasured answer counted in the score: %+v", item)
	}
	answers, err := s.MapAnswers(ctx, hashes, "")
	if err != nil || len(answers) != 7 {
		t.Fatalf("answers=%+v err=%v", answers, err)
	}

	benchmarks, err := s.MapBenchmarks(ctx, "")
	if err != nil || len(benchmarks) != 9 {
		t.Fatalf("benchmarks=%+v err=%v", benchmarks, err)
	}
	type benchmarkCell struct {
		Measured int      `json:"measured"`
		AvgScore *float64 `json:"avg_score"`
	}
	cellsOf := func(benchmark MapBenchmark) []benchmarkCell {
		t.Helper()
		var picked []benchmarkCell
		if err := json.Unmarshal(benchmark.Cells, &picked); err != nil || len(picked) != 1 {
			t.Fatalf("cells=%s err=%v", benchmark.Cells, err)
		}
		return picked
	}
	for _, benchmark := range benchmarks {
		switch benchmark.ID {
		case cancelled:
			if benchmark.Machine != "mac" || benchmark.DoneItems != 1 || benchmark.Models[0] != "m" || benchmark.Categories[0] != "code" {
				t.Fatalf("benchmark=%+v", benchmark)
			}
			if cell := cellsOf(benchmark)[0]; cell.Measured != 1 || scoreOf(cell.AvgScore) != 100 {
				t.Fatalf("cell=%+v", cell)
			}
		case unmeasured:
			if cell := cellsOf(benchmark)[0]; cell.Measured != 0 || cell.AvgScore != nil {
				t.Fatalf("an unmeasured answer must weigh nothing in the benchmark score: %+v", cell)
			}
		}
	}
}

func TestModelLabel(t *testing.T) {
	for connection, want := range map[string]string{ConnectionLocal: "m", "": "m", ConnectionAPI: "m (API · OpenAI)", ConnectionSubscription: "m (abo)"} {
		if got := ModelLabel("m", connection, "OpenAI"); got != want {
			t.Fatalf("%q: got %q, want %q", connection, got, want)
		}
	}
}

// Scores of the OpenAI label: i1 played twice at 100, i2 once at 0, so 50 when questions weigh the same within the label.
func TestModelLabelSplitsConnections(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	id, err := s.CreateCampaign(ctx, "demo", json.RawMessage(`{"label":"mac"}`), json.RawMessage(`{}`), 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []Run{
		{ProviderName: "OpenAI", ConnectionType: ConnectionAPI, ItemID: "i1", Score: 100},
		{ProviderName: "OpenAI", ConnectionType: ConnectionAPI, ItemID: "i1", Score: 100},
		{ProviderName: "OpenAI", ConnectionType: ConnectionAPI, ItemID: "i2", Score: 0},
		{ProviderName: "OpenRouter", ConnectionType: ConnectionAPI, ItemID: "i1", Score: 0},
		{ProviderName: "Claude (abonnement)", ConnectionType: ConnectionSubscription, ItemID: "i1", Score: 0},
	} {
		run.CampaignID, run.CaseID, run.Category, run.Model = id, "c", "code", "claude"
		run.Checks, run.ItemHash = json.RawMessage(`[]`), "h-"+run.ItemID
		if err := s.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.FinishCampaign(ctx, id, StatusDone); err != nil {
		t.Fatal(err)
	}
	apiLabel, otherAPILabel, subscriptionLabel := "claude (API · OpenAI)", "claude (API · OpenRouter)", "claude (abo)"
	hashes := []string{"h-i1", "h-i2"}

	cells, err := s.MapCells(ctx, hashes, "")
	if err != nil {
		t.Fatal(err)
	}
	rollups := map[string]float64{}
	for _, cell := range cells {
		if cell.Machine == "" && cell.Category == "" {
			rollups[cell.Model] = scoreOf(cell.AvgScore)
		}
	}
	if len(rollups) != 3 || rollups[apiLabel] != 50 || rollups[otherAPILabel] != 0 || rollups[subscriptionLabel] != 0 {
		t.Fatalf("one row per label, questions weighing the same within it: %v", rollups)
	}
	items, err := s.MapItems(ctx, hashes, "")
	if err != nil || len(items) != 8 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	answers, err := s.MapAnswers(ctx, hashes, ConnectionSubscription)
	if err != nil || len(answers) != 1 || answers[0].Label != subscriptionLabel {
		t.Fatalf("answers=%+v err=%v", answers, err)
	}

	stats, err := s.Stats(ctx, id)
	if err != nil || len(stats) != 6 || stats[0].Model != subscriptionLabel || stats[0].ConnectionType != ConnectionSubscription || stats[2].Model != apiLabel || stats[4].Model != otherAPILabel {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	runs, err := s.Runs(ctx, id)
	if err != nil || runs[1].Model != "claude" || runs[1].Label != apiLabel {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	for _, run := range runs {
		if run.Label != ModelLabel(run.Model, run.ConnectionType, run.ProviderName) {
			t.Fatalf("stored label drifted from ModelLabel: %+v", run)
		}
	}
	caseAnswers, err := s.CaseAnswers(ctx, "c", hashes)
	if err != nil || len(caseAnswers) != 4 {
		t.Fatalf("caseAnswers=%+v err=%v", caseAnswers, err)
	}
	for _, answer := range caseAnswers {
		if answer.Label == apiLabel && answer.ItemID == "i1" && answer.Answers != 2 {
			t.Fatalf("another connection's answer counted with the OpenAI ones: %+v", answer)
		}
	}
	benchmarks, err := s.MapBenchmarks(ctx, "")
	if err != nil || len(benchmarks) != 1 || len(benchmarks[0].Models) != 3 || !strings.Contains(string(benchmarks[0].Cells), subscriptionLabel) {
		t.Fatalf("benchmarks=%+v err=%v", benchmarks, err)
	}
	estimates, err := s.EstimateRows(ctx, "mac", hashes)
	overall := map[string]bool{}
	for _, row := range estimates {
		if row.Category == "" {
			overall[row.Model] = true
		}
	}
	if err != nil || len(overall) != 3 || !overall[apiLabel] || !overall[otherAPILabel] || !overall[subscriptionLabel] {
		t.Fatalf("estimates=%+v err=%v", estimates, err)
	}
	played, err := s.PlayedItems(ctx, id)
	if err != nil || len(played) != 5 || !slices.Contains(played, PlayedItem{Model: subscriptionLabel, CaseID: "c", ItemID: "i1"}) {
		t.Fatalf("played=%+v err=%v", played, err)
	}
}

func TestMapKeepsOneConnectionType(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	id, err := s.CreateCampaign(ctx, "demo", json.RawMessage(`{"label":"mac"}`), json.RawMessage(`{}`), 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []Run{
		{Model: "qwen", ConnectionType: ConnectionLocal},
		{Model: "claude", ConnectionType: ConnectionSubscription},
		{Model: "claude"},
	} {
		run.CampaignID, run.CaseID, run.ItemID, run.Category, run.ProviderName, run.Score = id, "c", "i", "code", "p", 100
		run.Checks, run.ItemHash = json.RawMessage(`[]`), "h"
		if err := s.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.FinishCampaign(ctx, id, StatusDone); err != nil {
		t.Fatal(err)
	}

	cells, err := s.MapCells(ctx, []string{"h"}, ConnectionLocal)
	if err != nil {
		t.Fatal(err)
	}
	for _, cell := range cells {
		if cell.Model != "qwen" {
			t.Fatalf("an answer of another type or of an unknown one counted as local: %+v", cell)
		}
	}
	items, err := s.MapItems(ctx, []string{"h"}, ConnectionSubscription)
	if err != nil || len(items) != 2 || items[0].Runs != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	answers, err := s.MapAnswers(ctx, []string{"h"}, ConnectionLocal)
	if err != nil || len(answers) != 1 || answers[0].Label != "qwen" {
		t.Fatalf("answers=%+v err=%v", answers, err)
	}

	subscriptionOnly, err := s.CreateCampaign(ctx, "abo", json.RawMessage(`{"label":"mac"}`), json.RawMessage(`{}`), 1)
	if err != nil {
		t.Fatal(err)
	}
	run := Run{CampaignID: subscriptionOnly, CaseID: "c", ItemID: "i", Category: "code", Model: "claude", ConnectionType: ConnectionSubscription,
		ProviderName: "p", Checks: json.RawMessage(`[]`), ItemHash: "h"}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	benchmarks, err := s.MapBenchmarks(ctx, ConnectionLocal)
	if err != nil || len(benchmarks) != 1 || benchmarks[0].ID != id || benchmarks[0].DoneItems != 1 ||
		!slices.Equal(benchmarks[0].Models, []string{"qwen"}) || strings.Contains(string(benchmarks[0].Cells), "claude") {
		t.Fatalf("a benchmark keeps the chosen type's answers only, one without any is left out: %+v err=%v", benchmarks, err)
	}
	if all, err := s.MapBenchmarks(ctx, ""); err != nil || len(all) != 2 || all[1].DoneItems != 3 {
		t.Fatalf("all=%+v err=%v", all, err)
	}
}
