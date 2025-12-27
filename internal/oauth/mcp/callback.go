package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// callbackResult holds the result of an OAuth callback.
type callbackResult struct {
	Code  string
	State string
	Error string
}

// callbackServer runs a temporary local HTTP server to receive OAuth callbacks.
type callbackServer struct {
	port     int
	path     string
	server   *http.Server
	listener net.Listener
	result   chan callbackResult
	once     sync.Once
}

// newCallbackServer creates a new callback server on the specified port and path.
// If port is 0, a random available port will be used.
// If path is empty, it defaults to "/callback".
func newCallbackServer(ctx context.Context, port int, path string) (*callbackServer, error) {
	if path == "" {
		path = "/callback"
	}

	lc := &net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return nil, fmt.Errorf("failed to start callback server: %w", err)
	}

	cs := &callbackServer{
		port:     listener.Addr().(*net.TCPAddr).Port,
		path:     path,
		listener: listener,
		result:   make(chan callbackResult, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, cs.handleCallback)

	cs.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return cs, nil
}

// RedirectURI returns the redirect URI for OAuth configuration.
func (cs *callbackServer) RedirectURI() string {
	return fmt.Sprintf("http://localhost:%d%s", cs.port, cs.path)
}

// Start starts the callback server in a goroutine.
func (cs *callbackServer) Start() {
	go func() {
		if err := cs.server.Serve(cs.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			cs.sendResult(callbackResult{Error: err.Error()})
		}
	}()
}

// waitForCallback waits for the OAuth callback with a timeout.
func (cs *callbackServer) waitForCallback(ctx context.Context) (callbackResult, error) {
	slog.Debug("Waiting for OAuth callback...")

	select {
	case result := <-cs.result:
		return result, nil
	case <-ctx.Done():
		return callbackResult{}, ctx.Err()
	}
}

// Close shuts down the callback server.
func (cs *callbackServer) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return cs.server.Shutdown(ctx)
}

func (cs *callbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errMsg := r.URL.Query().Get("error")
	errDesc := r.URL.Query().Get("error_description")

	if errMsg != "" {
		// error_description is optional
		if errDesc != "" {
			errMsg = fmt.Sprintf("%s: %s", errMsg, errDesc)
		}
		cs.sendResult(callbackResult{Error: errMsg})
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(errorHTML(errMsg)))
		return
	}

	if code == "" {
		cs.sendResult(callbackResult{Error: "no authorization code received"})
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(errorHTML("No authorization code received")))
		return
	}

	cs.sendResult(callbackResult{Code: code, State: state})
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(successHTML()))
}

func (cs *callbackServer) sendResult(result callbackResult) {
	cs.once.Do(func() {
		cs.result <- result
	})
}

func successHTML() string {
	return `<!DOCTYPE html>
<html>
<head>
    <title>Authorization Successful</title>
    <style>
        body { font-family: system-ui, -apple-system, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #1a1a2e; color: #eee; }
        .container { text-align: center; padding: 2rem; }
        .check { font-size: 4rem; color: #4ade80; }
        h1 { margin: 1rem 0; }
        p { color: #aaa; }
    </style>
</head>
<body>
    <div class="container">
        <div class="check">✓</div>
        <h1>Authorization Successful</h1>
        <p>You can close this window and return to Crush.</p>
    </div>
</body>
</html>`
}

func errorHTML(errMsg string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Authorization Failed</title>
    <style>
        body { font-family: system-ui, -apple-system, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #1a1a2e; color: #eee; }
        .container { text-align: center; padding: 2rem; }
        .error { font-size: 4rem; color: #f87171; }
        h1 { margin: 1rem 0; }
        p { color: #aaa; }
        .msg { color: #f87171; margin-top: 1rem; }
    </style>
</head>
<body>
    <div class="container">
        <div class="error">✗</div>
        <h1>Authorization Failed</h1>
        <p class="msg">%s</p>
        <p>Please close this window and try again.</p>
    </div>
</body>
</html>`, errMsg)
}
