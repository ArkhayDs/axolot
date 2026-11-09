package runner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/arkhayds/axolot/cases"
	"github.com/arkhayds/axolot/internal/provider"
	"github.com/arkhayds/axolot/internal/rag"
	"github.com/arkhayds/axolot/internal/store"
)

type fakeResults struct {
	runs   []store.Run
	status string
}

func (f *fakeResults) CreateRun(_ context.Context, run store.Run) error {
	f.runs = append(f.runs, run)
	return nil
}

func (f *fakeResults) FinishCampaign(_ context.Context, _, status string) error {
	f.status = status
	return nil
}

// fakeModel answers per model name, with an optional hook on each call.
type fakeModel struct {
	provider.Provider
	failModel string
	slowModel string
	onChat    func(provider.ChatRequest)
}

func (f fakeModel) Chat(_ context.Context, req provider.ChatRequest, onDelta func(string)) (provider.ChatResult, error) {
	if f.onChat != nil {
		f.onChat(req)
	}
	if req.Model == f.failModel {
		return provider.ChatResult{TotalMs: 1}, errors.New("out of memory")
	}
	if req.Model == f.slowModel {
		return provider.ChatResult{TotalMs: 1}, context.DeadlineExceeded
	}
	onDelta("bonjour")
	return provider.ChatResult{Output: "bonjour", TotalMs: 5}, nil
}

func textPlan(id string, models ...string) Plan {
	targets := make([]Target, 0, len(models))
	for _, m := range models {
		targets = append(targets, Target{ProviderID: 1, ProviderName: "p", Model: m})
	}
	return Plan{
		Case: cases.Case{ID: id, Kind: cases.KindText, Category: "resume", Items: []cases.Item{
			{ID: id + "-1", Prompt: "q", Checks: []cases.Check{{Type: "contains_all", Values: []string{"bonjour"}}}},
		}},
		Targets: targets,
	}
}

func newDeps(results *fakeResults, model fakeModel, events *[]Event) Deps {
	return Deps{
		Results:  results,
		Provider: func(context.Context, int64) (provider.Provider, error) { return model, nil },
		Search: func(context.Context, string) ([]rag.Hit, error) {
			return []rag.Hit{{Chunk: rag.Chunk{File: "a.md", Content: "extrait"}}}, nil
		},
		Emit: func(e Event) {
			if events != nil {
				*events = append(*events, e)
			}
		},
	}
}

func TestRunGradesAndStoresEveryPair(t *testing.T) {
	results := &fakeResults{}
	var events []Event
	plans := []Plan{textPlan("a", "m1", "m2"), textPlan("b", "m2")}

	if err := Run(context.Background(), "c1", plans, newDeps(results, fakeModel{}, &events)); err != nil {
		t.Fatal(err)
	}
	if len(results.runs) != 3 || results.status != store.StatusDone || CountItems(plans) != 3 {
		t.Fatalf("runs=%d status=%s count=%d", len(results.runs), results.status, CountItems(plans))
	}
	if results.runs[0].Score != 100 {
		t.Fatalf("score=%v", results.runs[0].Score)
	}
	if last := events[len(events)-1]; last.Type != EventFinished || last.Status != store.StatusDone {
		t.Fatalf("last event: %+v", last)
	}
}

func TestRunGroupsCasesByModel(t *testing.T) {
	results := &fakeResults{}
	plans := []Plan{textPlan("a", "m2", "m1"), textPlan("b", "m1", "m2")}
	if err := Run(context.Background(), "c1", plans, newDeps(results, fakeModel{}, nil)); err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, r := range results.runs {
		order = append(order, r.Model)
	}
	if got := strings.Join(order, ","); got != "m1,m1,m2,m2" {
		t.Fatalf("order=%s", got)
	}
}

func TestRunRecordsModelFailureAndContinues(t *testing.T) {
	results := &fakeResults{}
	plans := []Plan{textPlan("a", "bad", "good")}
	if err := Run(context.Background(), "c1", plans, newDeps(results, fakeModel{failModel: "bad"}, nil)); err != nil {
		t.Fatal(err)
	}
	if len(results.runs) != 2 || results.status != store.StatusDone {
		t.Fatalf("runs=%d status=%s", len(results.runs), results.status)
	}
	if results.runs[0].Error == "" || results.runs[0].Score != 0 {
		t.Fatalf("failed run: %+v", results.runs[0])
	}
}

func TestRunGradesTimeoutAsAFailedAnswer(t *testing.T) {
	results := &fakeResults{}
	if err := Run(context.Background(), "c1", []Plan{textPlan("a", "slow")}, newDeps(results, fakeModel{slowModel: "slow"}, nil)); err != nil {
		t.Fatal(err)
	}
	run := results.runs[0]
	if run.Error != "" || run.Score != 0 || run.TotalMs != itemTimeout.Milliseconds() || !strings.Contains(string(run.Checks), `"name":"timeout","passed":false`) {
		t.Fatalf("timed out run: %+v, checks %s", run, run.Checks)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	results := &fakeResults{}
	ctx, cancel := context.WithCancel(context.Background())
	model := fakeModel{onChat: func(provider.ChatRequest) { cancel() }}
	if err := Run(ctx, "c1", []Plan{textPlan("a", "m1", "m2")}, newDeps(results, model, nil)); err != nil {
		t.Fatal(err)
	}
	if results.status != store.StatusCancelled || len(results.runs) != 0 {
		t.Fatalf("status=%s runs=%d", results.status, len(results.runs))
	}
}

func TestRunRAGReusesSameExcerptsForEveryModel(t *testing.T) {
	results := &fakeResults{}
	searches := 0
	var prompts []string
	deps := newDeps(results, fakeModel{onChat: func(req provider.ChatRequest) {
		prompts = append(prompts, req.Messages[1].Content)
	}}, nil)
	deps.Search = func(context.Context, string) ([]rag.Hit, error) {
		searches++
		return []rag.Hit{{Chunk: rag.Chunk{File: "a.md", Content: "extrait"}}}, nil
	}
	plan := textPlan("r", "m1", "m2")
	plan.Case.Kind = cases.KindRAG
	plan.Case.Items[0].RAG = &cases.RAGSpec{Trap: cases.TrapNone, Sources: []cases.Source{{File: "a.md"}}}

	if err := Run(context.Background(), "c1", []Plan{plan}, deps); err != nil {
		t.Fatal(err)
	}
	if searches != 1 || len(prompts) != 2 || prompts[0] != prompts[1] {
		t.Fatalf("searches=%d prompts=%v", searches, prompts)
	}
	if hit := results.runs[0].RetrievalHit; hit == nil || !*hit {
		t.Fatalf("retrieval_hit=%v", hit)
	}
}
