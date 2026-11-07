// Command axolot-bridge serves the Claude CLI as a local OpenAI-compatible API, so that the subscription,
// which has no API key, can be added to Axolot as a plain connection.
// It runs on the host, where the macOS keychain holds the login. Loopback only: passing other users
// through a subscription is forbidden, so the address is not configurable.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

const shutdownTimeout = 10 * time.Second

// main reports startup failures with a non-zero exit.
func main() {
	if err := run(); err != nil {
		slog.Error("axolot-bridge stopped", "err", err)
		os.Exit(1)
	}
}

// run checks the setup and serves until a termination signal, which also kills any running CLI.
func run() error {
	token, port := os.Getenv("AXOLOT_BRIDGE_TOKEN"), os.Getenv("AXOLOT_BRIDGE_PORT")
	if token == "" || port == "" {
		return errors.New("missing environment variables: AXOLOT_BRIDGE_TOKEN, AXOLOT_BRIDGE_PORT")
	}
	cli, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("find the Claude CLI: %w", err)
	}
	dir, err := os.MkdirTemp("", "axolot-bridge-")
	if err != nil {
		return fmt.Errorf("create working directory: %w", err)
	}
	defer os.RemoveAll(dir)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// No write timeout: answers stream for minutes.
	server := &http.Server{
		Addr:              net.JoinHostPort("127.0.0.1", port),
		Handler:           (&bridge{token: token, cli: cli, dir: dir}).handler(),
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	errs := make(chan error, 1)
	go func() { errs <- server.ListenAndServe() }()
	slog.Info("axolot-bridge listening", "addr", server.Addr, "cli", cli)

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
