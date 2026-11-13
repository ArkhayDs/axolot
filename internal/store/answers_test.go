package store

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCaseAnswersKeepLatestAndCumulate(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	empty := json.RawMessage(`{}`)
	add := func(name, hash string, score float64, failure string) {
		t.Helper()
		id, err := s.CreateCampaign(ctx, name, empty, empty, 1)
		if err != nil {
			t.Fatal(err)
		}
		run := Run{CampaignID: id, CaseID: "c", ItemID: "q", Category: "t", ProviderName: "p", Model: "m", Output: name, Score: score, Error: failure, Checks: json.RawMessage(`[]`), ItemHash: hash,
			DefaultTemperature: name == "second"}
		if err := s.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	add("old wording", "before", 0, "")
	add("first", "now", 50, "")
	add("lost", "now", 0, "timeout")
	add("second", "now", 100, "")

	got, err := s.CaseAnswers(ctx, "c", []string{"now"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d answers, want 1", len(got))
	}
	if got[0].Output != "second" || got[0].Answers != 3 || scoreOf(got[0].AvgScore) != 75 || !got[0].DefaultTemperature {
		t.Fatalf("got output %q, %d answers, average %v, default temperature %v", got[0].Output, got[0].Answers, scoreOf(got[0].AvgScore), got[0].DefaultTemperature)
	}
}
