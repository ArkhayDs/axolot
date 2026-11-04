package scoring

import (
	"encoding/json"
	"testing"

	"github.com/arkhayds/axolot/cases"
)

func TestGradeChecks(t *testing.T) {
	schema := json.RawMessage(`{"required":["tool","args"]}`)
	tests := []struct {
		name   string
		check  cases.Check
		output string
		want   bool
	}{
		{"json valid", cases.Check{Type: "json_valid"}, `{"a":1}`, true},
		{"json in fence", cases.Check{Type: "json_valid"}, "```json\n{\"a\":1}\n```", true},
		{"json broken", cases.Check{Type: "json_valid"}, `{"a":`, false},
		{"required ok", cases.Check{Type: "json_required"}, `{"tool":"x","args":{}}`, true},
		{"required missing", cases.Check{Type: "json_required"}, `{"tool":"x"}`, false},
		{"required not object", cases.Check{Type: "json_required"}, `[1]`, false},
		{"equals text ignores case", cases.Check{Type: "json_equals", Path: "tool", Value: "Run_SQL"}, `{"tool":" run_sql "}`, true},
		{"equals number", cases.Check{Type: "json_equals", Path: "total", Value: 12.5}, `{"total":12.5}`, true},
		{"equals number differs", cases.Check{Type: "json_equals", Path: "total", Value: 12.5}, `{"total":13}`, false},
		{"equals nested", cases.Check{Type: "json_equals", Path: "a.b", Value: "x"}, `{"a":{"b":"x"}}`, true},
		{"equals missing path", cases.Check{Type: "json_equals", Path: "a.b", Value: "x"}, `{"a":"x"}`, false},
		{"contains all", cases.Check{Type: "contains_all", Values: []string{"LUNDI", "mardi"}}, "lundi et Mardi", true},
		{"contains all partial", cases.Check{Type: "contains_all", Values: []string{"lundi", "mardi"}}, "lundi", false},
		{"contains any", cases.Check{Type: "contains_any", Values: []string{"a1", "b2"}}, "B2", true},
		{"contains any none", cases.Check{Type: "contains_any", Values: []string{"a1"}}, "z", false},
		{"not contains", cases.Check{Type: "not_contains", Values: []string{"TODO"}}, "done", true},
		{"not contains found", cases.Check{Type: "not_contains", Values: []string{"todo"}}, "TODO here", false},
		{"regex", cases.Check{Type: "regex", Pattern: `(?i)^func\s+Add`}, "FUNC Add(", true},
		{"max words", cases.Check{Type: "max_words", Value: 2.0}, "a b", true},
		{"max words over", cases.Check{Type: "max_words", Value: 2.0}, "a b c", false},
		{"unknown type fails", cases.Check{Type: "nope"}, "x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, _, _ := Grade(cases.Item{Schema: schema, Checks: []cases.Check{tt.check}}, tt.output, nil)
			if results[0].Passed != tt.want {
				t.Fatalf("got %+v", results[0])
			}
		})
	}
}

func TestGradeScoresPercentage(t *testing.T) {
	item := cases.Item{Checks: []cases.Check{
		{Type: "contains_all", Values: []string{"a"}},
		{Type: "contains_all", Values: []string{"b"}},
	}}
	if _, got, hit := Grade(item, "a", nil); got != 50 || hit != nil {
		t.Fatalf("score=%v hit=%v", got, hit)
	}
	if _, got, _ := Grade(cases.Item{}, "a", nil); got != 0 {
		t.Fatalf("no check must score 0, got %v", got)
	}
}

func TestGradeRAGAddsRetrievalAndCitation(t *testing.T) {
	item := cases.Item{RAG: &cases.RAGSpec{
		Trap:    cases.TrapNone,
		Sources: []cases.Source{{File: "conges.md", Quote: "q"}},
	}}
	_, got, hit := Grade(item, "Selon [conges.md], 25 jours.", []string{"conges.md"})
	if got != 100 || hit == nil || !*hit {
		t.Fatalf("score=%v hit=%v", got, hit)
	}
	_, got, hit = Grade(item, "25 jours.", []string{"autre.md"})
	if got != 0 || *hit {
		t.Fatalf("score=%v hit=%v", got, *hit)
	}
}

func TestGradeRAGAbsentTrapSkipsRetrieval(t *testing.T) {
	item := cases.Item{
		RAG:    &cases.RAGSpec{Trap: cases.TrapAbsent},
		Checks: []cases.Check{{Type: "regex", Pattern: "(?i)je ne sais pas"}},
	}
	results, got, hit := Grade(item, "Je ne sais pas.", []string{"x.md"})
	if len(results) != 1 || got != 100 || hit != nil {
		t.Fatalf("results=%v score=%v hit=%v", results, got, hit)
	}
}
