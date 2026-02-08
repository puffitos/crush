// Package wakatime provides WakaTime integration for tracking AI coding activity.
package wakatime

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/version"
)

const (
	// DefaultCategory is the default WakaTime activity category.
	DefaultCategory = "ai coding"

	// heartbeatThreshold is the minimum time between heartbeats for the same file.
	heartbeatThreshold = 2 * time.Minute
)

// Config holds WakaTime configuration.
type Config struct {
	Enabled  bool
	APIKey   string
	Category string
	CLIPath  string
}

// Service manages WakaTime heartbeat tracking.
type Service struct {
	cfg      Config
	cliPath  string
	category string

	mu             sync.RWMutex
	lastHeartbeats map[string]time.Time
}

// New creates a new WakaTime service. Returns (nil, nil) if disabled or CLI not found,
// which allows callers to safely skip initialization without error handling.
func New(cfg Config) (*Service, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	cliPath := cfg.CLIPath
	if cliPath == "" {
		var err error
		cliPath, err = findCLI()
		if err != nil {
			slog.Warn("WakaTime CLI not found, integration disabled", "error", err)
			return nil, nil
		}
	}

	category := cfg.Category
	if category == "" {
		category = DefaultCategory
	}

	slog.Info("WakaTime integration enabled", "cli", cliPath, "category", category)

	return &Service{
		cfg:            cfg,
		cliPath:        cliPath,
		category:       category,
		lastHeartbeats: make(map[string]time.Time),
	}, nil
}

// Heartbeat represents a file activity event.
type Heartbeat struct {
	FilePath string
	IsWrite  bool
	Project  string
}

// SendHeartbeat sends a heartbeat to WakaTime if appropriate.
func (s *Service) SendHeartbeat(ctx context.Context, h Heartbeat) {
	if s == nil {
		return
	}

	if !s.shouldSend(h.FilePath, h.IsWrite) {
		return
	}

	s.recordHeartbeat(h.FilePath)

	// Run in background to avoid blocking.
	go s.send(h)
}

// shouldSend determines if a heartbeat should be sent based on throttling rules.
func (s *Service) shouldSend(filePath string, isWrite bool) bool {
	// Always send on write events.
	if isWrite {
		return true
	}

	s.mu.RLock()
	lastSent, exists := s.lastHeartbeats[filePath]
	s.mu.RUnlock()

	if !exists {
		return true
	}

	return time.Since(lastSent) >= heartbeatThreshold
}

// recordHeartbeat records when a heartbeat was last sent for a file.
func (s *Service) recordHeartbeat(filePath string) {
	s.mu.Lock()
	s.lastHeartbeats[filePath] = time.Now()
	s.mu.Unlock()
}

// send executes wakatime-cli to send a heartbeat.
func (s *Service) send(h Heartbeat) {
	args := []string{
		"--entity", h.FilePath,
		"--category", s.category,
		"--plugin", "crush/" + version.Version + " crush-wakatime/1.0.0",
	}

	if h.IsWrite {
		args = append(args, "--write")
	}

	if h.Project != "" {
		args = append(args, "--project", h.Project)
	}

	if s.cfg.APIKey != "" {
		args = append(args, "--key", s.cfg.APIKey)
	}

	// Use a short timeout context for the CLI call.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, s.cliPath, args...)
	if err := cmd.Run(); err != nil {
		slog.Debug("WakaTime heartbeat failed", "error", err, "file", h.FilePath)
	}
}

// findCLI locates the wakatime-cli binary.
func findCLI() (string, error) {
	// Check ~/.wakatime/ directory first.
	home, err := os.UserHomeDir()
	if err == nil {
		wakatimeDir := filepath.Join(home, ".wakatime")
		entries, err := os.ReadDir(wakatimeDir)
		if err == nil {
			for _, entry := range entries {
				name := entry.Name()
				if strings.HasPrefix(name, "wakatime-cli") && !entry.IsDir() {
					path := filepath.Join(wakatimeDir, name)
					if isExecutable(path) {
						return path, nil
					}
				}
			}
		}
	}

	// Fall back to PATH.
	path, err := exec.LookPath("wakatime-cli")
	if err == nil {
		return path, nil
	}

	// Also check for "wakatime" in PATH.
	path, err = exec.LookPath("wakatime")
	if err == nil {
		return path, nil
	}

	return "", err
}

// isExecutable checks if a file is executable.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode()&0o111 != 0
}
