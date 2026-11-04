// Package scoring grades outputs with deterministic checks only, so the tool needs no AI of its own.
package scoring

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/arkhayds/axolot/cases"
)

// fencePattern matches a Markdown code fence wrapping a whole JSON answer.
var fencePattern = regexp.MustCompile("(?s)^```[a-zA-Z]*\\s*(.*?)\\s*```$")

// Result is the outcome of one check.
type Result struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

// Grade runs every check of item against output.
// @param retrieved - corpus files returned by the search, used for RAG items only
// @returns results, score in 0-100, and whether the search found the expected files (nil outside RAG)
func Grade(item cases.Item, output string, retrieved []string) ([]Result, float64, *bool) {
	results := make([]Result, 0, len(item.Checks)+2)
	for _, check := range item.Checks {
		results = append(results, runCheck(check, item.Schema, output))
	}
	var hit *bool
	if item.RAG != nil && item.RAG.Trap != cases.TrapAbsent {
		found := containsAllSources(item.RAG.Sources, retrieved)
		hit = &found
		results = append(results,
			Result{Name: "retrieval", Passed: found, Detail: "expected passage retrieved"},
			citationResult(item.RAG.Sources, output),
		)
	}
	return results, score(results), hit
}

// score is the percentage of passed checks; no check means nothing was proven, so zero.
func score(results []Result) float64 {
	if len(results) == 0 {
		return 0
	}
	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
	}
	return 100 * float64(passed) / float64(len(results))
}

// runCheck dispatches one check; an unknown type fails rather than silently inflating the score.
func runCheck(check cases.Check, schema json.RawMessage, output string) Result {
	result := Result{Name: check.Type}
	lower := strings.ToLower(output)
	switch check.Type {
	case "json_valid":
		_, err := parseJSON(output)
		result.Passed = err == nil
	case "json_required":
		result.Passed, result.Detail = hasRequired(schema, output)
	case "json_equals":
		result.Name = "json_equals:" + check.Path
		result.Passed, result.Detail = jsonEquals(output, check.Path, check.Value)
	case "contains_all":
		missing := filter(check.Values, func(v string) bool { return !strings.Contains(lower, strings.ToLower(v)) })
		result.Passed = len(missing) == 0
		result.Detail = detail("missing", missing)
	case "contains_any":
		found := filter(check.Values, func(v string) bool { return strings.Contains(lower, strings.ToLower(v)) })
		result.Passed = len(found) > 0
		if !result.Passed {
			result.Detail = detail("none of", check.Values)
		}
	case "not_contains":
		found := filter(check.Values, func(v string) bool { return strings.Contains(lower, strings.ToLower(v)) })
		result.Passed = len(found) == 0
		result.Detail = detail("found", found)
	case "regex":
		re, err := regexp.Compile(check.Pattern)
		result.Passed = err == nil && re.MatchString(output)
	case "max_words":
		limit, ok := check.Value.(float64)
		words := len(strings.Fields(output))
		result.Passed = ok && float64(words) <= limit
		result.Detail = fmt.Sprintf("%d words, limit %v", words, check.Value)
	default:
		result.Detail = "unknown check type"
	}
	return result
}

// parseJSON decodes output, tolerating a Markdown fence around it.
func parseJSON(output string) (any, error) {
	text := strings.TrimSpace(output)
	if match := fencePattern.FindStringSubmatch(text); match != nil {
		text = match[1]
	}
	var value any
	err := json.Unmarshal([]byte(text), &value)
	return value, err
}

// hasRequired checks that every field listed by the schema is present in the output.
func hasRequired(schema json.RawMessage, output string) (bool, string) {
	var spec struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &spec); err != nil {
		return false, "invalid schema"
	}
	value, err := parseJSON(output)
	if err != nil {
		return false, "invalid json"
	}
	object, ok := value.(map[string]any)
	if !ok {
		return false, "not an object"
	}
	missing := filter(spec.Required, func(field string) bool {
		_, present := object[field]
		return !present
	})
	return len(missing) == 0, detail("missing", missing)
}

// jsonEquals compares the value at a dotted path; strings ignore case and surrounding spaces.
func jsonEquals(output, path string, want any) (bool, string) {
	value, err := parseJSON(output)
	if err != nil {
		return false, "invalid json"
	}
	for _, key := range strings.Split(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return false, "path not found"
		}
		if value, ok = object[key]; !ok {
			return false, "path not found"
		}
	}
	if sameValue(value, want) {
		return true, ""
	}
	return false, fmt.Sprintf("got %v, want %v", value, want)
}

// sameValue compares decoded JSON values.
func sameValue(got, want any) bool {
	gotText, gotIsText := got.(string)
	wantText, wantIsText := want.(string)
	if gotIsText && wantIsText {
		return strings.EqualFold(strings.TrimSpace(gotText), strings.TrimSpace(wantText))
	}
	return got == want
}

// containsAllSources reports whether each expected file was retrieved.
func containsAllSources(sources []cases.Source, retrieved []string) bool {
	seen := make(map[string]bool, len(retrieved))
	for _, file := range retrieved {
		seen[file] = true
	}
	for _, source := range sources {
		if !seen[source.File] {
			return false
		}
	}
	return true
}

// citationResult checks that the answer names at least one expected file.
func citationResult(sources []cases.Source, output string) Result {
	for _, source := range sources {
		if strings.Contains(output, source.File) {
			return Result{Name: "citation", Passed: true}
		}
	}
	return Result{Name: "citation", Detail: "expected source not cited"}
}

// filter keeps the values matching keep.
func filter(values []string, keep func(string) bool) []string {
	var kept []string
	for _, v := range values {
		if keep(v) {
			kept = append(kept, v)
		}
	}
	return kept
}

// detail formats a label and values, empty when there is nothing to report.
func detail(label string, values []string) string {
	if len(values) == 0 {
		return ""
	}
	return label + ": " + strings.Join(values, ", ")
}
