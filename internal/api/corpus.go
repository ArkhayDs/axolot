package api

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/arkhayds/axolot/internal/provider"
	"github.com/arkhayds/axolot/internal/rag"
	"github.com/arkhayds/axolot/internal/store"
)

const maxUpload = 20 << 20

var (
	corpusName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)
	fileName   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*\.(md|txt)$`)
)

// corpusView is a folder under the mounted root.
type corpusView struct {
	Name  string   `json:"name"`
	Files []string `json:"files"`
}

// openCorpus opens a folder; os.Root rejects any path escaping the mounted root.
func (s *Server) openCorpus(name string) (*os.Root, error) {
	root, err := os.OpenRoot(s.cfg.CorpusRoot)
	if err != nil {
		return nil, fmt.Errorf("open corpus root: %w", err)
	}
	defer root.Close()
	return root.OpenRoot(name)
}

// listCorpus returns folders, their documents, and the indexes.
func (s *Server) listCorpus(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.cfg.CorpusRoot)
	if err != nil {
		internalError(w, r, err)
		return
	}
	folders := make([]corpusView, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !corpusName.MatchString(entry.Name()) {
			continue
		}
		folder, err := s.openCorpus(entry.Name())
		if err != nil {
			internalError(w, r, err)
			return
		}
		files, err := rag.Files(folder.FS())
		folder.Close()
		if err != nil {
			internalError(w, r, err)
			return
		}
		sort.Strings(files)
		folders = append(folders, corpusView{Name: entry.Name(), Files: files})
	}
	indexes, err := s.cfg.Store.CorpusIndexes(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.mu.Lock()
	ingesting := s.ingest
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"folders": folders, "indexes": indexes, "ingesting": ingesting})
}

// createCorpus adds an empty folder.
func (s *Server) createCorpus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if !corpusName.MatchString(body.Name) {
		writeError(w, http.StatusBadRequest, "Nom de dossier invalide (lettres, chiffres, - et _)")
		return
	}
	root, err := os.OpenRoot(s.cfg.CorpusRoot)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer root.Close()
	if err := root.Mkdir(body.Name, 0o755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			writeError(w, http.StatusConflict, "Ce dossier existe déjà")
			return
		}
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// readCorpusFile returns a document as text, so the presenter can check a passage by hand.
func (s *Server) readCorpusFile(w http.ResponseWriter, r *http.Request) {
	name, file := r.PathValue("name"), r.PathValue("file")
	if !corpusName.MatchString(name) || !fileName.MatchString(file) {
		writeError(w, http.StatusBadRequest, "Chemin invalide")
		return
	}
	folder, err := s.openCorpus(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "Dossier introuvable")
		return
	}
	defer folder.Close()
	raw, err := fs.ReadFile(folder.FS(), file)
	if err != nil {
		writeError(w, http.StatusNotFound, "Fichier introuvable")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(raw)
}

// uploadCorpusFile stores a .md or .txt document, refusing to overwrite an existing one.
func (s *Server) uploadCorpusFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !corpusName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "Dossier invalide")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	source, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Fichier manquant ou trop volumineux (20 Mo max)")
		return
	}
	defer source.Close()
	base := path.Base(strings.ReplaceAll(header.Filename, `\`, "/"))
	if !fileName.MatchString(base) {
		writeError(w, http.StatusBadRequest, "Formats acceptés : .md, .txt (nom sans espace)")
		return
	}
	folder, err := s.openCorpus(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "Dossier introuvable")
		return
	}
	defer folder.Close()
	target, err := folder.OpenFile(base, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			writeError(w, http.StatusConflict, "Ce fichier existe déjà")
			return
		}
		internalError(w, r, err)
		return
	}
	defer target.Close()
	if _, err := io.Copy(target, source); err != nil {
		_ = folder.Remove(base)
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// embedConnection resolves the configured embedding server, defaulting to the first connection.
func (s *Server) embedConnection(r *http.Request, settings store.Settings) (provider.Provider, error) {
	id := settings.EmbedProviderID
	if id == 0 {
		providers, err := s.cfg.Store.Providers(r.Context())
		if err != nil {
			return nil, err
		}
		if len(providers) == 0 {
			return nil, errors.New("no model connection configured")
		}
		id = providers[0].ID
	}
	client, _, err := s.connection(r.Context(), id)
	return client, err
}

// ingestCorpus rebuilds the configured corpus index in the background.
func (s *Server) ingestCorpus(w http.ResponseWriter, r *http.Request) {
	settings, err := s.cfg.Store.Settings(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	embedder, err := s.embedConnection(r, settings)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Aucune connexion de modèles disponible")
		return
	}
	folder, err := s.openCorpus(settings.CorpusDir)
	if err != nil {
		writeError(w, http.StatusNotFound, "Dossier de corpus introuvable")
		return
	}
	s.mu.Lock()
	if s.ingest {
		s.mu.Unlock()
		folder.Close()
		writeError(w, http.StatusConflict, "Indexation déjà en cours")
		return
	}
	s.ingest = true
	s.mu.Unlock()
	go s.runIngest(folder, embedder, settings)
	w.WriteHeader(http.StatusAccepted)
}

// runIngest indexes a folder and publishes progress.
func (s *Server) runIngest(folder *os.Root, embedder provider.Provider, settings store.Settings) {
	defer folder.Close()
	publish := func(done, total int, finished bool, message string) {
		s.hub.publish(map[string]any{"type": "ingest", "done": done, "total": total, "finished": finished, "error": message})
	}
	total, err := rag.Ingest(s.cfg.BaseContext, s.cfg.Store, embedder, settings.EmbedModel, settings.CorpusDir, folder.FS(),
		settings.ChunkSize, settings.ChunkOverlap, func(done, total int) { publish(done, total, false, "") })
	s.mu.Lock()
	s.ingest = false
	s.mu.Unlock()
	message := ""
	if err != nil {
		slog.Warn("corpus ingest failed", "corpus", settings.CorpusDir, "err", err)
		message = err.Error()
	}
	publish(total, total, true, message)
}
