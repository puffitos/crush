// Package zellij provides Zellij notification integration via the
// zellij-attention plugin (https://github.com/KiryuuLight/zellij-attention).
//
// When enabled, Crush sends a pipe message to the current Zellij pane on
// each agent turn boundary so that zellij-attention can highlight the tab
// (waiting -> Crush is working, completed -> Crush is waiting for user
// input).
package zellij

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

// State is the zellij-attention state to signal.
type State string

const (
	// StateWaiting indicates Crush is busy working.
	StateWaiting State = "waiting"
	// StateCompleted indicates Crush is idle and waiting for user input.
	StateCompleted State = "completed"
)

// Config holds Zellij notification configuration.
type Config struct {
	Enabled bool
}

// Service sends zellij-attention pipe messages.
type Service struct {
	cliPath string
	paneID  string
}

// New creates a new Zellij notifier. Returns (nil, nil) if disabled, if the
// process is not running inside Zellij (ZELLIJ_PANE_ID unset), or if the
// zellij binary is not on PATH — callers can safely ignore the result.
func New(cfg Config) (*Service, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	paneID := os.Getenv("ZELLIJ_PANE_ID")
	if paneID == "" {
		slog.Debug("Zellij notifications enabled but ZELLIJ_PANE_ID not set; disabling")
		return nil, nil
	}

	cliPath, err := exec.LookPath("zellij")
	if err != nil {
		slog.Warn("Zellij notifications enabled but zellij binary not found on PATH; disabling", "error", err)
		return nil, nil
	}

	slog.Info("Zellij notifications enabled", "cli", cliPath, "pane_id", paneID)

	return &Service{
		cliPath: cliPath,
		paneID:  paneID,
	}, nil
}

// Notify sends a zellij-attention pipe message for the given state.
// Safe to call on a nil receiver.
func (s *Service) Notify(state State) {
	if s == nil {
		return
	}

	name := "zellij-attention::" + string(state) + "::" + s.paneID

	// Run in background with a short timeout to avoid blocking the agent.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, s.cliPath, "pipe", "--name", name)
		if err := cmd.Run(); err != nil {
			slog.Debug("Zellij pipe failed", "error", err, "name", name)
		}
	}()
}
