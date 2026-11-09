package analytics

import "testing"

func ms(v float64) *float64 { return &v }

func stat(model, category string, score, total float64) ModelStats {
	return ModelStats{Provider: "p", Model: model, Category: category, Runs: 4, AvgScore: ms(score), P50TotalMs: ms(total)}
}

func TestVerdictsPickQualitySpeedAndCompromise(t *testing.T) {
	verdicts := Verdicts([]ModelStats{
		stat("big", "", 95, 9000),
		stat("mid", "", 88, 3000),
		stat("tiny", "", 60, 500),
	})
	if len(verdicts) != 1 {
		t.Fatalf("got %d verdicts", len(verdicts))
	}
	v := verdicts[0]
	if v.Quality.Model != "big" || v.Speed.Model != "tiny" || v.Compromise.Model != "mid" {
		t.Fatalf("quality=%s speed=%s compromise=%s", v.Quality.Model, v.Speed.Model, v.Compromise.Model)
	}
}

func TestVerdictsSeparateCategories(t *testing.T) {
	verdicts := Verdicts([]ModelStats{stat("a", "code", 90, 100), stat("b", "rag", 70, 100)})
	if len(verdicts) != 2 || verdicts[0].Quality.Model != "a" || verdicts[1].Quality.Model != "b" {
		t.Fatalf("verdicts: %+v", verdicts)
	}
}

func TestVerdictsExcludeFullyFailedModels(t *testing.T) {
	failed := ModelStats{Provider: "p", Model: "oom", Runs: 4, Errors: 4, P50TotalMs: ms(1)}
	verdicts := Verdicts([]ModelStats{failed, stat("ok", "", 50, 900)})
	if verdicts[0].Speed.Model != "ok" {
		t.Fatalf("failed model won speed: %+v", verdicts[0].Speed)
	}
}

func TestVerdictsQualityTieGoesToFaster(t *testing.T) {
	verdicts := Verdicts([]ModelStats{stat("slow", "", 80, 900), stat("fast", "", 80, 100)})
	if verdicts[0].Quality.Model != "fast" {
		t.Fatalf("quality=%s", verdicts[0].Quality.Model)
	}
}

func TestVerdictsHandleEmptyAndMissingTimings(t *testing.T) {
	if got := Verdicts(nil); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
	noTiming := ModelStats{Provider: "p", Model: "x", Runs: 1, AvgScore: ms(50)}
	v := Verdicts([]ModelStats{noTiming})[0]
	if v.Quality.Model != "x" || v.Speed != nil {
		t.Fatalf("verdict: %+v", v)
	}
}
