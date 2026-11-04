package cases

import "testing"

func TestItemHash(t *testing.T) {
	base := Item{ID: "a", Prompt: "question", Checks: []Check{{Type: "regex", Pattern: "x"}}}
	first, err := itemHash("system", base)
	if err != nil {
		t.Fatal(err)
	}
	same, err := itemHash("system", base)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != same {
		t.Fatalf("hash not stable: %q, %q", first, same)
	}

	edited := base
	edited.Prompt = "autre question"
	regraded := base
	regraded.Checks = []Check{{Type: "regex", Pattern: "y"}}
	for name, got := range map[string]func() (string, error){
		"prompt": func() (string, error) { return itemHash("system", edited) },
		"checks": func() (string, error) { return itemHash("system", regraded) },
		"system": func() (string, error) { return itemHash("autre", base) },
	} {
		hash, err := got()
		if err != nil {
			t.Fatal(err)
		}
		if hash == first {
			t.Errorf("changing %s kept the hash", name)
		}
	}
}

func TestLoadSetsItemHash(t *testing.T) {
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range loaded {
		for _, item := range c.Items {
			if item.Hash == "" {
				t.Fatalf("%s/%s has no hash", c.ID, item.ID)
			}
		}
	}
}
