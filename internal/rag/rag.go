// Package rag holds the deliberately simple retrieval pipeline: chunk, embed, store, search, prompt.
package rag

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/arkhayds/axolot/internal/provider"
)

// embedBatch bounds one embedding request, so a slow CPU machine still reports progress.
const embedBatch = 16

// breakWindow is the end share of a chunk where a cleaner break point is searched for.
const breakWindow = 0.3

// systemPrompt is the same for every model, so differences come from the model alone.
const systemPrompt = `Tu réponds en français, uniquement à partir des extraits fournis.
Cite le fichier source entre crochets, par exemple [conges-2026.md].
Si plusieurs versions d'un document existent, utilise la plus récente.
Vérifie que la règle concerne bien l'entité demandée.
Si l'information ne figure pas dans les extraits, réponds : "Je ne sais pas, cette information ne figure pas dans les documents."`

// Chunk is a slice of a document, with its offset in runes for display.
type Chunk struct {
	File     string `json:"file"`
	Position int    `json:"position"`
	Start    int    `json:"start_offset"`
	Content  string `json:"content"`
}

// Hit is a chunk returned by a search.
type Hit struct {
	Chunk
	Similarity float64 `json:"similarity"`
}

// Store persists and searches embedded chunks.
type Store interface {
	ReplaceChunks(ctx context.Context, corpus, embedModel string, chunks []Chunk, vectors [][]float32) error
	SearchChunks(ctx context.Context, corpus, embedModel string, vector []float32, limit int) ([]Hit, error)
}

// Split cuts text into overlapping chunks, preferring paragraph then sentence boundaries.
// @param size - max runes per chunk
// @param overlap - runes shared with the next chunk, must stay below size
func Split(file, text string, size, overlap int) ([]Chunk, error) {
	if size <= 0 || overlap < 0 || overlap >= size {
		return nil, fmt.Errorf("invalid chunking: size %d, overlap %d", size, overlap)
	}
	runes := []rune(text)
	var chunks []Chunk
	for start := 0; start < len(runes); {
		end := min(start+size, len(runes))
		if end < len(runes) {
			end = breakPoint(runes, start, end)
		}
		content := strings.TrimSpace(string(runes[start:end]))
		if content != "" {
			chunks = append(chunks, Chunk{File: file, Position: len(chunks), Start: start, Content: content})
		}
		if end == len(runes) {
			break
		}
		// max guarantees progress even when a short boundary leaves less than the overlap.
		start = max(end-overlap, start+1)
	}
	return chunks, nil
}

// breakPoint looks backward from end for a paragraph, then sentence, boundary.
func breakPoint(runes []rune, start, end int) int {
	floor := end - int(float64(end-start)*breakWindow)
	for _, boundary := range []string{"\n\n", ". ", "\n"} {
		sep := []rune(boundary)
		for i := end - len(sep); i >= floor; i-- {
			if string(runes[i:i+len(sep)]) == boundary {
				return i + len(sep)
			}
		}
	}
	return end
}

// Files lists the .md and .txt documents directly under a corpus folder.
func Files(corpus fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(corpus, ".")
	if err != nil {
		return nil, fmt.Errorf("read corpus: %w", err)
	}
	var names []string
	for _, entry := range entries {
		ext := strings.ToLower(path.Ext(entry.Name()))
		if !entry.IsDir() && (ext == ".md" || ext == ".txt") {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// Ingest rebuilds a corpus index for one embedding model.
// The replace happens only after every embedding succeeded, so a failure keeps the previous index.
// @param onProgress - called with done and total chunks
func Ingest(ctx context.Context, store Store, embedder provider.Provider, embedModel, corpusName string, corpus fs.FS, size, overlap int, onProgress func(done, total int)) (int, error) {
	names, err := Files(corpus)
	if err != nil {
		return 0, err
	}
	var chunks []Chunk
	for _, name := range names {
		raw, err := fs.ReadFile(corpus, name)
		if err != nil {
			return 0, fmt.Errorf("read %s: %w", name, err)
		}
		if !utf8.Valid(raw) {
			return 0, fmt.Errorf("%s is not UTF-8", name)
		}
		fileChunks, err := Split(name, string(raw), size, overlap)
		if err != nil {
			return 0, err
		}
		chunks = append(chunks, fileChunks...)
	}
	if len(chunks) == 0 {
		return 0, fmt.Errorf("no text found in corpus %q", corpusName)
	}

	vectors := make([][]float32, 0, len(chunks))
	for start := 0; start < len(chunks); start += embedBatch {
		end := min(start+embedBatch, len(chunks))
		inputs := make([]string, 0, end-start)
		for _, chunk := range chunks[start:end] {
			inputs = append(inputs, chunk.Content)
		}
		batch, err := embedder.Embed(ctx, embedModel, inputs)
		if err != nil {
			return 0, fmt.Errorf("embed chunks: %w", err)
		}
		vectors = append(vectors, batch...)
		onProgress(end, len(chunks))
	}
	if err := store.ReplaceChunks(ctx, corpusName, embedModel, chunks, vectors); err != nil {
		return 0, err
	}
	return len(chunks), nil
}

// Search embeds the question and returns the closest chunks.
func Search(ctx context.Context, store Store, embedder provider.Provider, embedModel, corpusName, question string, limit int) ([]Hit, error) {
	vectors, err := embedder.Embed(ctx, embedModel, []string{question})
	if err != nil {
		return nil, fmt.Errorf("embed question: %w", err)
	}
	return store.SearchChunks(ctx, corpusName, embedModel, vectors[0], limit)
}

// BuildMessages assembles the chat prompt from retrieved excerpts.
// @param caseSystem - extra instructions of the case, appended to the shared prompt
func BuildMessages(caseSystem, question string, hits []Hit) []provider.Message {
	var excerpts strings.Builder
	for _, hit := range hits {
		fmt.Fprintf(&excerpts, "--- [%s]\n%s\n\n", hit.File, hit.Content)
	}
	system := systemPrompt
	if caseSystem != "" {
		system += "\n" + caseSystem
	}
	user := "Extraits :\n\n" + excerpts.String() + "Question : " + question
	return []provider.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
}
