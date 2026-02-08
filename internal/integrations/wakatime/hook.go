// Package wakatime provides WakaTime integration for tracking AI coding activity.
package wakatime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"charm.land/fantasy"
)

// fileTools are tool names that interact with files.
var fileTools = map[string]bool{
	"view":      true,
	"edit":      true,
	"multiedit": true,
	"write":     true,
	"grep":      true,
	"glob":      true,
}

// Hook wraps fantasy tools to send WakaTime heartbeats.
type Hook struct {
	service    *Service
	workingDir string
}

// NewHook creates a new WakaTime hook.
func NewHook(service *Service, workingDir string) *Hook {
	if service == nil {
		return nil
	}
	return &Hook{
		service:    service,
		workingDir: workingDir,
	}
}

// WrapTools wraps the given tools to send WakaTime heartbeats on file operations.
func (h *Hook) WrapTools(tools []fantasy.AgentTool) []fantasy.AgentTool {
	if h == nil {
		return tools
	}

	wrapped := make([]fantasy.AgentTool, len(tools))
	for i, tool := range tools {
		if fileTools[tool.Info().Name] {
			wrapped[i] = &wrappedTool{
				AgentTool:  tool,
				hook:       h,
				workingDir: h.workingDir,
			}
		} else {
			wrapped[i] = tool
		}
	}
	return wrapped
}

// wrappedTool wraps a fantasy.AgentTool to send heartbeats.
type wrappedTool struct {
	fantasy.AgentTool
	hook       *Hook
	workingDir string
}

// Run executes the tool and sends a heartbeat for file operations.
func (w *wrappedTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	result, err := w.AgentTool.Run(ctx, call)

	// Extract file path from params and send heartbeat.
	filePath := extractFilePath(call.Input, w.workingDir)
	if filePath != "" {
		toolName := w.AgentTool.Info().Name
		isWrite := toolName == "edit" || toolName == "multiedit" || toolName == "write"

		w.hook.service.SendHeartbeat(ctx, Heartbeat{
			FilePath: filePath,
			IsWrite:  isWrite,
			Project:  detectProject(filePath),
		})
	}

	return result, err
}

// extractFilePath extracts the file path from tool parameters.
func extractFilePath(params string, workingDir string) string {
	// Parse JSON to extract file path.
	var data map[string]any
	if err := json.Unmarshal([]byte(params), &data); err != nil {
		return ""
	}

	// Try file_path first (view, edit, multiedit, write).
	if path, ok := data["file_path"].(string); ok && path != "" {
		if !filepath.IsAbs(path) && workingDir != "" {
			path = filepath.Join(workingDir, path)
		}
		return path
	}

	// Try path (grep, glob).
	if path, ok := data["path"].(string); ok && path != "" {
		if !filepath.IsAbs(path) && workingDir != "" {
			path = filepath.Join(workingDir, path)
		}
		return path
	}

	return ""
}

// detectProject attempts to detect the project name from a file path.
func detectProject(filePath string) string {
	// Walk up directories looking for common project markers.
	dir := filepath.Dir(filePath)
	markers := []string{".git", "go.mod", "package.json", "Cargo.toml", "pyproject.toml"}

	for {
		parent := filepath.Dir(dir)
		// Stop at filesystem root (Unix: /, Windows: C:\, etc.).
		if parent == dir || dir == "." {
			break
		}
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return filepath.Base(dir)
			}
		}
		dir = parent
	}

	// Fall back to parent directory name.
	return filepath.Base(filepath.Dir(filePath))
}
