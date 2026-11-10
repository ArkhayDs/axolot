package cases

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/arkhayds/axolot/internal/rag"
)

func TestLoadValidatesEmbeddedCases(t *testing.T) {
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range loaded {
		if seen[c.ID] {
			t.Errorf("duplicate case id %s", c.ID)
		}
		seen[c.ID] = true
	}
	for _, want := range []string{"extraction-facture", "rag-helianthe", "routage-sentinelle"} {
		if !seen[want] {
			t.Errorf("missing case %s", want)
		}
	}
}

// Cloud APIs reject a strict schema whose objects leave a property optional.
func TestSchemasRequireEveryProperty(t *testing.T) {
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range loaded {
		for _, item := range c.Items {
			if len(item.Schema) == 0 {
				continue
			}
			var schema map[string]any
			if err := json.Unmarshal(item.Schema, &schema); err != nil {
				t.Fatalf("%s: %v", item.ID, err)
			}
			if missing := optionalProperty(schema); missing != "" {
				t.Errorf("%s: property %s not required", item.ID, missing)
			}
		}
	}
}

// optionalProperty returns the first property of the schema tree missing from its object's required list.
// @param node - schema node
// @returns string - property name, "" when all are required
func optionalProperty(node map[string]any) string {
	properties, _ := node["properties"].(map[string]any)
	required, _ := node["required"].([]any)
	for name, property := range properties {
		if !slices.Contains(required, any(name)) {
			return name
		}
		if child, ok := property.(map[string]any); ok {
			if missing := optionalProperty(child); missing != "" {
				return missing
			}
		}
	}
	if items, ok := node["items"].(map[string]any); ok {
		return optionalProperty(items)
	}
	return ""
}

// The presenter opens these passages by hand during the demo, so each quote must exist verbatim
// in the extracted text: that is what the models read and where the front-end highlights.
func TestRAGQuotesExistInCorpus(t *testing.T) {
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range loaded {
		for _, item := range c.Items {
			if item.RAG == nil {
				continue
			}
			if item.RAG.Trap == TrapAbsent && len(item.RAG.Sources) > 0 {
				t.Errorf("%s: absent trap must have no source", item.ID)
			}
			for _, source := range item.RAG.Sources {
				raw, err := os.ReadFile(filepath.Join("..", "corpus", "demo", source.File))
				if err != nil {
					t.Errorf("%s: %v", item.ID, err)
					continue
				}
				text, err := rag.Extract(source.File, raw)
				if err != nil {
					t.Errorf("%s: %v", item.ID, err)
					continue
				}
				if !strings.Contains(text, source.Quote) {
					t.Errorf("%s: quote not found in %s: %q", item.ID, source.File, source.Quote)
				}
			}
		}
	}
}

// One unreadable document fails the whole indexing, so each one must extract, cited or not.
func TestCorpusDocumentsExtract(t *testing.T) {
	demo := os.DirFS(filepath.Join("..", "corpus", "demo"))
	names, err := rag.Files(demo)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("empty corpus")
	}
	for _, name := range names {
		raw, err := fs.ReadFile(demo, name)
		if err != nil {
			t.Fatal(err)
		}
		text, err := rag.Extract(name, raw)
		if err != nil {
			t.Error(err)
			continue
		}
		if !strings.Contains(text, "Document fictif") {
			t.Errorf("%s: fiction notice lost", name)
		}
	}
}
