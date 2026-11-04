package rag

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/arkhayds/axolot/internal/provider"
)

func TestSplitCoversTextAndOverlaps(t *testing.T) {
	text := strings.Repeat("abcdefghij", 30)
	chunks, err := Split("f.md", text, 100, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 3 {
		t.Fatalf("got %d chunks", len(chunks))
	}
	if chunks[1].Start != 80 {
		t.Fatalf("overlap not applied, start=%d", chunks[1].Start)
	}
	last := chunks[len(chunks)-1]
	if last.Start+len(last.Content) != len(text) {
		t.Fatalf("tail lost: start=%d len=%d", last.Start, len(last.Content))
	}
}

func TestSplitPrefersParagraphBoundary(t *testing.T) {
	text := strings.Repeat("a", 80) + "\n\n" + strings.Repeat("b", 80)
	chunks, err := Split("f.md", text, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if chunks[0].Content != strings.Repeat("a", 80) {
		t.Fatalf("first chunk: %q", chunks[0].Content)
	}
}

func TestSplitCountsRunesNotBytes(t *testing.T) {
	chunks, err := Split("f.md", strings.Repeat("é", 150), 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || !strings.HasPrefix(chunks[1].Content, "é") {
		t.Fatalf("chunks: %+v", chunks)
	}
}

func TestSplitAlwaysTerminates(t *testing.T) {
	chunks, err := Split("f.md", strings.Repeat("a b. ", 200), 10, 9)
	if err != nil || len(chunks) == 0 {
		t.Fatalf("chunks=%d err=%v", len(chunks), err)
	}
}

func TestSplitRejectsInvalidParameters(t *testing.T) {
	for _, tt := range [][2]int{{0, 0}, {10, 10}, {10, -1}} {
		if _, err := Split("f.md", "x", tt[0], tt[1]); err == nil {
			t.Errorf("size=%d overlap=%d accepted", tt[0], tt[1])
		}
	}
}

// fakeEmbedder returns one vector per input, or fails.
type fakeEmbedder struct {
	provider.Provider
	err error
}

func (f fakeEmbedder) Embed(_ context.Context, _ string, inputs []string) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	vectors := make([][]float32, len(inputs))
	for i := range inputs {
		vectors[i] = []float32{0.1}
	}
	return vectors, nil
}

// fakeStore records what would be persisted.
type fakeStore struct {
	replaced int
}

func (s *fakeStore) ReplaceChunks(_ context.Context, _, _ string, chunks []Chunk, _ [][]float32) error {
	s.replaced = len(chunks)
	return nil
}

func (s *fakeStore) SearchChunks(context.Context, string, string, []float32, int) ([]Hit, error) {
	return nil, nil
}

func corpusFS() fstest.MapFS {
	return fstest.MapFS{
		"a.md":      {Data: []byte("Texte a.")},
		"b.txt":     {Data: []byte("Texte b.")},
		"image.png": {Data: []byte{0xff}},
	}
}

func TestIngestIndexesTextFilesOnly(t *testing.T) {
	store := &fakeStore{}
	total, err := Ingest(context.Background(), store, fakeEmbedder{}, "m", "demo", corpusFS(), 100, 0, func(int, int) {})
	if err != nil || total != 2 || store.replaced != 2 {
		t.Fatalf("total=%d replaced=%d err=%v", total, store.replaced, err)
	}
}

func TestIngestKeepsIndexWhenEmbeddingFails(t *testing.T) {
	store := &fakeStore{replaced: -1}
	_, err := Ingest(context.Background(), store, fakeEmbedder{err: errors.New("down")}, "m", "demo", corpusFS(), 100, 0, func(int, int) {})
	if err == nil || store.replaced != -1 {
		t.Fatalf("replaced=%d err=%v", store.replaced, err)
	}
}

func TestIngestRejectsNonUTF8(t *testing.T) {
	corpus := fstest.MapFS{"latin1.txt": {Data: []byte{0xe9, 0xe8}}}
	if _, err := Ingest(context.Background(), &fakeStore{}, fakeEmbedder{}, "m", "demo", corpus, 100, 0, func(int, int) {}); err == nil {
		t.Fatal("expected encoding error")
	}
}

func TestBuildMessagesNamesSources(t *testing.T) {
	messages := BuildMessages("Consigne du cas.", "Combien ?", []Hit{{Chunk: Chunk{File: "conges.md", Content: "25 jours"}}})
	if len(messages) != 2 || !strings.Contains(messages[1].Content, "[conges.md]") || !strings.Contains(messages[1].Content, "Combien ?") {
		t.Fatalf("messages: %+v", messages)
	}
	if !strings.Contains(messages[0].Content, "Consigne du cas.") {
		t.Fatalf("case system prompt dropped: %q", messages[0].Content)
	}
}
