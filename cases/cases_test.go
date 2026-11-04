package cases

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// The presenter opens these passages by hand during the demo, so each quote must exist verbatim.
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
				if !strings.Contains(string(raw), source.Quote) {
					t.Errorf("%s: quote not found in %s: %q", item.ID, source.File, source.Quote)
				}
			}
		}
	}
}
