// Package cases loads the benchmark use cases embedded in the binary.
package cases

import (
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
)

//go:embed *.json
var files embed.FS

// Case kinds.
const (
	KindText     = "text"
	KindDecision = "decision"
	KindRAG      = "rag"
)

// RAG trap types.
const (
	TrapNone        = "none"
	TrapAbsent      = "absent"
	TrapOutdated    = "outdated"
	TrapOtherEntity = "other_entity"
)

// Case is a family of prompts scored the same way.
type Case struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Category string `json:"category"`
	Language string `json:"language"`
	System   string `json:"system,omitempty"`
	Items    []Item `json:"items"`
}

// Item is one prompt with its expected result.
type Item struct {
	ID     string          `json:"id"`
	Prompt string          `json:"prompt"`
	Schema json.RawMessage `json:"schema,omitempty"`
	Checks []Check         `json:"checks"`
	RAG    *RAGSpec        `json:"rag,omitempty"`
}

// Check is one deterministic assertion on an output.
type Check struct {
	Type    string   `json:"type"`
	Path    string   `json:"path,omitempty"`
	Values  []string `json:"values,omitempty"`
	Value   any      `json:"value,omitempty"`
	Pattern string   `json:"pattern,omitempty"`
}

// RAGSpec is the ground truth of a RAG question: where the answer lives, and which trap it sets.
type RAGSpec struct {
	Trap    string   `json:"trap"`
	Sources []Source `json:"sources"`
}

// Source locates an expected passage in the corpus.
type Source struct {
	File  string `json:"file"`
	Quote string `json:"quote"`
}

// Load returns all embedded cases sorted by category then id.
// Invalid definitions are startup errors: scoring on a broken case would corrupt the results.
func Load() ([]Case, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("list cases: %w", err)
	}
	loaded := make([]Case, 0, len(entries))
	for _, entry := range entries {
		raw, err := files.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		var c Case
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, fmt.Errorf("parse %s: %w", entry.Name(), err)
		}
		if err := c.validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		loaded = append(loaded, c)
	}
	sort.Slice(loaded, func(i, j int) bool {
		if loaded[i].Category != loaded[j].Category {
			return loaded[i].Category < loaded[j].Category
		}
		return loaded[i].ID < loaded[j].ID
	})
	return loaded, nil
}

// validate rejects a case that could not be run or scored.
func (c Case) validate() error {
	if c.ID == "" || len(c.Items) == 0 {
		return fmt.Errorf("case needs an id and items")
	}
	if c.Kind != KindText && c.Kind != KindDecision && c.Kind != KindRAG {
		return fmt.Errorf("unknown kind %q", c.Kind)
	}
	for _, item := range c.Items {
		if item.ID == "" || item.Prompt == "" {
			return fmt.Errorf("item needs an id and a prompt")
		}
		if c.Kind == KindRAG && item.RAG == nil {
			return fmt.Errorf("rag item %s has no rag spec", item.ID)
		}
		for _, check := range item.Checks {
			if check.Type != "regex" {
				continue
			}
			if _, err := regexp.Compile(check.Pattern); err != nil {
				return fmt.Errorf("item %s: %w", item.ID, err)
			}
		}
	}
	return nil
}
