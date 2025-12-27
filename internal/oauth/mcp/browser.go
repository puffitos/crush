package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// openBrowser opens the specified URL in the system's default browser.
func openBrowser(url string) error {
	ctx := context.Background()
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url)
	case "linux":
		cmd = exec.CommandContext(ctx, "xdg-open", url)
	case "windows":
		cmd = exec.CommandContext(ctx, "cmd", "/c", "start", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}
