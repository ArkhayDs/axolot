// Package runner executes a campaign: one model at a time, one item at a time, since a single
// machine has one GPU and parallel runs would falsify the timings.
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/arkhayds/axolot/cases"
	"github.com/arkhayds/axolot/internal/provider"
	"github.com/arkhayds/axolot/internal/rag"
	"github.com/arkhayds/axolot/internal/scoring"
	"github.com/arkhayds/axolot/internal/store"
)

const (
	itemTimeout = 5 * time.Minute
	maxTokens   = 1024
)

// Event types pushed to the UI.
const (
	EventItemStart = "item_start"
	EventDelta     = "delta"
	EventItemEnd   = "item_end"
	EventFinished  = "campaign_end"
)

// Target is a model on a connection.
type Target struct {
	ProviderID     int64
	ProviderName   string
	ConnectionType string
	Model          string
}

// Plan lists the models to run on one case.
type Plan struct {
	Case    cases.Case
	Targets []Target
}

// Event reports campaign progress, models named by their label as the stored results read back.
type Event struct {
	Type       string     `json:"type"`
	Campaign   string     `json:"campaign"`
	Case       string     `json:"case,omitempty"`
	Item       string     `json:"item,omitempty"`
	Provider   string     `json:"provider,omitempty"`
	Connection string     `json:"connection,omitempty"`
	Model      string     `json:"model,omitempty"`
	Text       string     `json:"text,omitempty"`
	Run        *store.Run `json:"run,omitempty"`
	Status     string     `json:"status,omitempty"`
}

// Results persists campaign outcomes.
type Results interface {
	CreateRun(ctx context.Context, run store.Run) error
	FinishCampaign(ctx context.Context, id, status string) error
}

// Deps is what a campaign needs from the outside.
type Deps struct {
	Results  Results
	Provider func(ctx context.Context, id int64) (provider.Provider, error)
	// Search returns the excerpts of a RAG question.
	Search func(ctx context.Context, question string) ([]rag.Hit, error)
	Emit   func(Event)
}

// CountItems returns how many runs a plan will produce.
func CountItems(plans []Plan) int {
	total := 0
	for _, plan := range plans {
		total += len(plan.Case.Items) * len(plan.Targets)
	}
	return total
}

// Run executes the plans and closes the campaign. A failing item is recorded and the run continues,
// because one model running out of memory is itself a result to report.
func Run(ctx context.Context, campaignID string, plans []Plan, deps Deps) error {
	retrieved := map[string][]rag.Hit{}
	for _, target := range orderedTargets(plans) {
		for _, plan := range plans {
			if !hasTarget(plan, target) {
				continue
			}
			for _, item := range plan.Case.Items {
				if ctx.Err() != nil {
					return finish(ctx, campaignID, store.StatusCancelled, deps)
				}
				if err := runItem(ctx, campaignID, plan.Case, item, target, retrieved, deps); err != nil {
					if ctx.Err() != nil {
						return finish(ctx, campaignID, store.StatusCancelled, deps)
					}
					_ = finish(ctx, campaignID, store.StatusInterrupted, deps)
					return err
				}
			}
		}
	}
	return finish(ctx, campaignID, store.StatusDone, deps)
}

// finish closes the campaign even when the run context was cancelled.
func finish(ctx context.Context, campaignID, status string, deps Deps) error {
	err := deps.Results.FinishCampaign(context.WithoutCancel(ctx), campaignID, status)
	deps.Emit(Event{Type: EventFinished, Campaign: campaignID, Status: status})
	return err
}

// orderedTargets returns each distinct model once, sorted, so a model stays loaded across its cases.
func orderedTargets(plans []Plan) []Target {
	seen := map[Target]bool{}
	var targets []Target
	for _, plan := range plans {
		for _, target := range plan.Targets {
			if !seen[target] {
				seen[target] = true
				targets = append(targets, target)
			}
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].ProviderID != targets[j].ProviderID {
			return targets[i].ProviderID < targets[j].ProviderID
		}
		return targets[i].Model < targets[j].Model
	})
	return targets
}

// hasTarget reports whether a plan includes the model.
func hasTarget(plan Plan, target Target) bool {
	for _, t := range plan.Targets {
		if t == target {
			return true
		}
	}
	return false
}

// runItem generates, grades and stores one answer.
// @returns only errors preventing the result from being stored; a model failure becomes a stored run
func runItem(ctx context.Context, campaignID string, c cases.Case, item cases.Item, target Target, retrieved map[string][]rag.Hit, deps Deps) error {
	base := Event{Campaign: campaignID, Case: c.ID, Item: item.ID, Provider: target.ProviderName, Connection: target.ConnectionType, Model: store.ModelLabel(target.Model, target.ConnectionType, target.ProviderName)}
	start := base
	start.Type = EventItemStart
	deps.Emit(start)

	run := store.Run{
		CampaignID: campaignID, CaseID: c.ID, ItemID: item.ID, Category: c.Category,
		ProviderName: target.ProviderName, ConnectionType: target.ConnectionType, Model: target.Model, Label: base.Model, Checks: json.RawMessage(`[]`), ItemHash: item.Hash,
	}
	result, hits, err := generate(ctx, c, item, target, retrieved, base, deps)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	run.Output, run.TotalMs = result.Output, result.TotalMs
	run.TTFTMs, run.LoadMs = result.TTFTMs, result.LoadMs
	run.CLIStartupMs, run.CLIExitMs = result.CLIStartupMs, result.CLIExitMs
	// The Claude CLI behind a subscription has no temperature option.
	run.DefaultTemperature = result.DefaultTemperature || target.ConnectionType == store.ConnectionSubscription
	run.PromptTokens, run.OutputTokens, run.TokPerSec = result.PromptTokens, result.OutputTokens, result.TokPerSec
	if hits != nil {
		raw, marshalErr := json.Marshal(hits)
		if marshalErr != nil {
			return fmt.Errorf("encode hits: %w", marshalErr)
		}
		run.Retrieved = raw
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		timedOut(&run)
	case err != nil:
		run.Error = err.Error()
	default:
		if err := grade(&run, item, hits); err != nil {
			return err
		}
	}
	if err := deps.Results.CreateRun(ctx, run); err != nil {
		return err
	}
	end := base
	end.Type = EventItemEnd
	end.Run = &run
	deps.Emit(end)
	return nil
}

// generate prepares the prompt and calls the model; retrieval is cached so all models read the same excerpts.
func generate(ctx context.Context, c cases.Case, item cases.Item, target Target, retrieved map[string][]rag.Hit, base Event, deps Deps) (provider.ChatResult, []rag.Hit, error) {
	req := provider.ChatRequest{Model: target.Model, Schema: item.Schema, MaxTokens: maxTokens}
	var hits []rag.Hit
	if c.Kind == cases.KindRAG {
		var cached bool
		if hits, cached = retrieved[item.ID]; !cached {
			var err error
			if hits, err = deps.Search(ctx, item.Prompt); err != nil {
				return provider.ChatResult{}, nil, fmt.Errorf("search corpus: %w", err)
			}
			retrieved[item.ID] = hits
		}
		req.Messages = rag.BuildMessages(c.System, item.Prompt, hits)
	} else {
		if c.System != "" {
			req.Messages = append(req.Messages, provider.Message{Role: "system", Content: c.System})
		}
		req.Messages = append(req.Messages, provider.Message{Role: "user", Content: item.Prompt})
	}

	p, err := deps.Provider(ctx, target.ProviderID)
	if err != nil {
		return provider.ChatResult{}, hits, err
	}
	itemCtx, cancel := context.WithTimeout(ctx, itemTimeout)
	defer cancel()
	result, err := p.Chat(itemCtx, req, func(text string) {
		delta := base
		delta.Type = EventDelta
		delta.Text = text
		deps.Emit(delta)
	})
	return result, hits, err
}

// timedOut fails a run the model did not finish in time: a slow model answers wrong, unlike an execution error
// that leaves the answer unmeasured. Its total is the timeout so that medians do not read the cut as a fast answer.
func timedOut(run *store.Run) {
	run.TotalMs = itemTimeout.Milliseconds()
	run.Checks = json.RawMessage(`[{"name":"timeout","passed":false}]`)
}

// grade fills the score fields of a successful run.
func grade(run *store.Run, item cases.Item, hits []rag.Hit) error {
	files := make([]string, 0, len(hits))
	for _, hit := range hits {
		files = append(files, hit.File)
	}
	results, score, hit := scoring.Grade(item, run.Output, files)
	raw, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("encode checks: %w", err)
	}
	run.Checks, run.Score, run.RetrievalHit = raw, score, hit
	return nil
}
