// Command axolot serves the benchmark lab: API, event stream and embedded web UI.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arkhayds/axolot/cases"
	"github.com/arkhayds/axolot/internal/api"
	"github.com/arkhayds/axolot/internal/hardware"
	"github.com/arkhayds/axolot/internal/secret"
	"github.com/arkhayds/axolot/internal/store"
	"github.com/arkhayds/axolot/web"
)

const shutdownTimeout = 10 * time.Second

// main reports startup failures with a non-zero exit.
func main() {
	if err := run(); err != nil {
		slog.Error("axolot stopped", "err", err)
		os.Exit(1)
	}
}

// run wires dependencies and serves until a termination signal.
func run() error {
	env, err := requireEnv("DATABASE_URL", "AXOLOT_PORT", "AXOLOT_SECRET", "AXOLOT_CORPUS_ROOT", "AXOLOT_HARDWARE_FILE", "OLLAMA_URL",
		"AXOLOT_BRIDGE_URL", "AXOLOT_BRIDGE_TOKEN")
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	box, err := secret.NewBox(env["AXOLOT_SECRET"])
	if err != nil {
		return err
	}
	loaded, err := cases.Load()
	if err != nil {
		return fmt.Errorf("load cases: %w", err)
	}
	machine, err := hardware.Read(env["AXOLOT_HARDWARE_FILE"])
	if err != nil {
		return err
	}
	static, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return fmt.Errorf("open web assets: %w", err)
	}
	db, err := store.Open(ctx, env["DATABASE_URL"])
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.MarkInterrupted(ctx); err != nil {
		return err
	}
	if err := seedConnection(ctx, db, env["OLLAMA_URL"]); err != nil {
		return err
	}

	handler := api.New(api.Config{
		Store: db, Box: box, Cases: loaded, Hardware: machine,
		CorpusRoot: env["AXOLOT_CORPUS_ROOT"], Static: static, BaseContext: ctx,
		BridgeURL: env["AXOLOT_BRIDGE_URL"], BridgeToken: env["AXOLOT_BRIDGE_TOKEN"],
	})
	// No write timeout: the event stream and model downloads stay open for minutes.
	server := &http.Server{
		Addr:              net.JoinHostPort("", env["AXOLOT_PORT"]),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errs := make(chan error, 1)
	go func() { errs <- server.ListenAndServe() }()
	slog.Info("axolot listening", "port", env["AXOLOT_PORT"], "machine", machine.Label, "cases", len(loaded))

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

// requireEnv reads mandatory variables, failing at boot with every missing name at once.
func requireEnv(names ...string) (map[string]string, error) {
	values := make(map[string]string, len(names))
	var missing []string
	for _, name := range names {
		value := os.Getenv(name)
		if value == "" {
			missing = append(missing, name)
		}
		values[name] = value
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing environment variables: %v", missing)
	}
	return values, nil
}

// seedConnection adds the local Ollama connection on first start.
func seedConnection(ctx context.Context, db *store.Store, ollamaURL string) error {
	providers, err := db.Providers(ctx)
	if err != nil {
		return err
	}
	if len(providers) > 0 {
		return nil
	}
	_, err = db.CreateProvider(ctx, "Ollama local", "ollama", ollamaURL, nil)
	return err
}
