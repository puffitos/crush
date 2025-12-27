// Package mcp provides functionality for managing Model Context Protocol (MCP)
// clients within the Crush application.
package mcp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/home"
	"github.com/charmbracelet/crush/internal/oauth"
	mcpoauth "github.com/charmbracelet/crush/internal/oauth/mcp"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func parseLevel(level mcp.LoggingLevel) slog.Level {
	switch level {
	case "info":
		return slog.LevelInfo
	case "notice":
		return slog.LevelInfo
	case "warning":
		return slog.LevelWarn
	default:
		return slog.LevelDebug
	}
}

// ClientSession wraps an mcp.ClientSession with a context cancel function so
// that the context created during session establishment is properly cleaned up
// on close.
type ClientSession struct {
	*mcp.ClientSession
	cancel context.CancelFunc
}

// Close cancels the session context and then closes the underlying session.
func (s *ClientSession) Close() error {
	s.cancel()
	return s.ClientSession.Close()
}

var (
	sessions       = csync.NewMap[string, *ClientSession]()
	states         = csync.NewMap[string, ClientInfo]()
	broker         = pubsub.NewBroker[Event]()
	tokenProviders = csync.NewMap[string, *OAuthTokenProvider]()
	tokenStore     *TokenStore
	initOnce       sync.Once
	initDone       = make(chan struct{})
)

// State represents the current state of an MCP client
type State int

const (
	StateDisabled State = iota
	StateStarting
	StateConnected
	StateError
)

func (s State) String() string {
	switch s {
	case StateDisabled:
		return "disabled"
	case StateStarting:
		return "starting"
	case StateConnected:
		return "connected"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// EventType represents the type of MCP event
type EventType uint

const (
	EventStateChanged EventType = iota
	EventToolsListChanged
	EventPromptsListChanged
	EventResourcesListChanged
)

// Event represents an event in the MCP system
type Event struct {
	Type   EventType
	Name   string
	State  State
	Error  error
	Counts Counts
}

// Counts number of available tools, prompts, etc.
type Counts struct {
	Tools     int
	Prompts   int
	Resources int
}

// ClientInfo holds information about an MCP client's state
type ClientInfo struct {
	Name        string
	State       State
	Error       error
	Client      *ClientSession
	Counts      Counts
	ConnectedAt time.Time
}

// SubscribeEvents returns a channel for MCP events
func SubscribeEvents(ctx context.Context) <-chan pubsub.Event[Event] {
	return broker.Subscribe(ctx)
}

// GetStates returns the current state of all MCP clients
func GetStates() map[string]ClientInfo {
	return states.Copy()
}

// GetState returns the state of a specific MCP client
func GetState(name string) (ClientInfo, bool) {
	return states.Get(name)
}

// Close closes all MCP clients. This should be called during application shutdown.
func Close(ctx context.Context) error {
	var wg sync.WaitGroup
	for name, session := range sessions.Seq2() {
		wg.Go(func() {
			done := make(chan error, 1)
			go func() {
				done <- session.Close()
			}()
			select {
			case err := <-done:
				if err != nil &&
					!errors.Is(err, io.EOF) &&
					!errors.Is(err, context.Canceled) &&
					err.Error() != "signal: killed" {
					slog.Warn("Failed to shutdown MCP client", "name", name, "error", err)
				}
			case <-ctx.Done():
			}
		})
	}
	wg.Wait()
	broker.Shutdown()
	return nil
}

// Initialize initializes MCP clients based on the provided configuration.
func Initialize(ctx context.Context, permissions permission.Service, cfg *config.Config) {
	slog.Info("Initializing MCP clients")
	// Initialize the token store for OAuth token persistence (uses global data directory)
	tokenStore = NewTokenStore()

	var wg sync.WaitGroup
	// Initialize states for all configured MCPs
	for name, m := range cfg.MCP {
		if m.Disabled {
			updateState(name, StateDisabled, nil, nil, Counts{})
			slog.Debug("Skipping disabled MCP", "name", name)
			continue
		}

		// Set initial starting state
		updateState(name, StateStarting, nil, nil, Counts{})

		wg.Add(1)
		go func(name string, m config.MCPConfig) {
			defer func() {
				wg.Done()
				if r := recover(); r != nil {
					var err error
					switch v := r.(type) {
					case error:
						err = v
					case string:
						err = fmt.Errorf("panic: %s", v)
					default:
						err = fmt.Errorf("panic: %v", v)
					}
					updateState(name, StateError, err, nil, Counts{})
					slog.Error("Panic in MCP client initialization", "error", err, "name", name)
				}
			}()

			// createSession handles its own timeout internally.
			session, err := createSession(ctx, name, m, cfg.Resolver())
			if err != nil {
				return
			}

			tools, err := getTools(ctx, session)
			if err != nil {
				slog.Error("Error listing tools", "error", err)
				updateState(name, StateError, err, nil, Counts{})
				session.Close()
				return
			}

			prompts, err := getPrompts(ctx, session)
			if err != nil {
				slog.Error("Error listing prompts", "error", err)
				updateState(name, StateError, err, nil, Counts{})
				session.Close()
				return
			}

			resources, err := getResources(ctx, session)
			if err != nil {
				slog.Error("Error listing resources", "error", err)
				updateState(name, StateError, err, nil, Counts{})
				session.Close()
				return
			}

			toolCount := updateTools(cfg, name, tools)
			updatePrompts(name, prompts)
			resourceCount := updateResources(name, resources)
			sessions.Set(name, session)

			updateState(name, StateConnected, nil, session, Counts{
				Tools:     toolCount,
				Prompts:   len(prompts),
				Resources: resourceCount,
			})
		}(name, m)
	}
	wg.Wait()
	initOnce.Do(func() { close(initDone) })
}

// WaitForInit blocks until MCP initialization is complete.
// If Initialize was never called, this returns immediately.
func WaitForInit(ctx context.Context) error {
	select {
	case <-initDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func getOrRenewClient(ctx context.Context, cfg *config.Config, name string) (*ClientSession, error) {
	sess, ok := sessions.Get(name)
	if !ok {
		return nil, fmt.Errorf("mcp '%s' not available", name)
	}

	m := cfg.MCP[name]
	state, _ := states.Get(name)

	timeout := mcpTimeout(m)
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := sess.Ping(pingCtx, nil)
	if err == nil {
		return sess, nil
	}
	updateState(name, StateError, maybeTimeoutErr(err, timeout), nil, state.Counts)

	sess, err = createSession(ctx, name, m, cfg.Resolver())
	if err != nil {
		return nil, err
	}

	updateState(name, StateConnected, nil, sess, state.Counts)
	sessions.Set(name, sess)
	return sess, nil
}

// updateState updates the state of an MCP client and publishes an event
func updateState(name string, state State, err error, client *ClientSession, counts Counts) {
	info := ClientInfo{
		Name:   name,
		State:  state,
		Error:  err,
		Client: client,
		Counts: counts,
	}
	switch state {
	case StateConnected:
		info.ConnectedAt = time.Now()
	case StateError:
		sessions.Del(name)
	}
	states.Set(name, info)

	// Publish state change event
	broker.Publish(pubsub.UpdatedEvent, Event{
		Type:   EventStateChanged,
		Name:   name,
		State:  state,
		Error:  err,
		Counts: counts,
	})
}

func createSession(ctx context.Context, name string, m config.MCPConfig, resolver config.VariableResolver) (*ClientSession, error) {
	timeout := mcpTimeout(m)
	mcpCtx, cancel := context.WithCancel(ctx)
	cancelTimer := time.AfterFunc(timeout, cancel)

	transport, err := createTransport(mcpCtx, name, m, resolver, tokenStore)
	if err != nil {
		updateState(name, StateError, err, nil, Counts{})
		slog.Error("Error creating MCP client", "error", err, "name", name)
		cancel()
		cancelTimer.Stop()
		return nil, err
	}

	client := mcp.NewClient(
		&mcp.Implementation{
			Name:    "crush",
			Version: version.Version,
			Title:   "Crush",
		},
		&mcp.ClientOptions{
			ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {
				broker.Publish(pubsub.UpdatedEvent, Event{
					Type: EventToolsListChanged,
					Name: name,
				})
			},
			PromptListChangedHandler: func(context.Context, *mcp.PromptListChangedRequest) {
				broker.Publish(pubsub.UpdatedEvent, Event{
					Type: EventPromptsListChanged,
					Name: name,
				})
			},
			ResourceListChangedHandler: func(context.Context, *mcp.ResourceListChangedRequest) {
				broker.Publish(pubsub.UpdatedEvent, Event{
					Type: EventResourcesListChanged,
					Name: name,
				})
			},
			LoggingMessageHandler: func(ctx context.Context, req *mcp.LoggingMessageRequest) {
				level := parseLevel(req.Params.Level)
				slog.Log(ctx, level, "MCP log", "name", name, "logger", req.Params.Logger, "data", req.Params.Data)
			},
		},
	)

	session, err := client.Connect(mcpCtx, transport, nil)
	if err != nil {
		err = maybeStdioErr(err, transport)
		updateState(name, StateError, maybeTimeoutErr(err, timeout), nil, Counts{})
		slog.Error("MCP client failed to initialize", "error", err, "name", name)
		cancel()
		cancelTimer.Stop()
		return nil, err
	}

	cancelTimer.Stop()
	slog.Debug("MCP client initialized", "name", name)
	return &ClientSession{session, cancel}, nil
}

// maybeStdioErr if a stdio mcp prints an error in non-json format, it'll fail
// to parse, and the cli will then close it, causing the EOF error.
// so, if we got an EOF err, and the transport is STDIO, we try to exec it
// again with a timeout and collect the output so we can add details to the
// error.
// this happens particularly when starting things with npx, e.g. if node can't
// be found or some other error like that.
func maybeStdioErr(err error, transport mcp.Transport) error {
	if !errors.Is(err, io.EOF) {
		return err
	}
	ct, ok := transport.(*mcp.CommandTransport)
	if !ok {
		return err
	}
	if err2 := stdioCheck(ct.Command); err2 != nil {
		err = errors.Join(err, err2)
	}
	return err
}

func maybeTimeoutErr(err error, timeout time.Duration) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("timed out after %s", timeout)
	}
	return err
}

func createTransport(ctx context.Context, name string, m config.MCPConfig, resolver config.VariableResolver, tokenStore *TokenStore) (mcp.Transport, error) {
	switch m.Type {
	case config.MCPStdio:
		command, err := resolver.ResolveValue(m.Command)
		if err != nil {
			return nil, fmt.Errorf("invalid mcp command: %w", err)
		}
		if strings.TrimSpace(command) == "" {
			return nil, fmt.Errorf("mcp stdio config requires a non-empty 'command' field")
		}
		cmd := exec.CommandContext(ctx, home.Long(command), m.Args...)
		cmd.Env = append(os.Environ(), m.ResolvedEnv()...)
		return &mcp.CommandTransport{
			Command: cmd,
		}, nil
	case config.MCPHttp:
		if strings.TrimSpace(m.URL) == "" {
			return nil, fmt.Errorf("mcp http config requires a non-empty 'url' field")
		}
		transport := buildHTTPTransport(ctx, name, m, tokenStore)
		client := &http.Client{Transport: transport}
		return &mcp.StreamableClientTransport{
			Endpoint:   m.URL,
			HTTPClient: client,
		}, nil
	case config.MCPSSE:
		if strings.TrimSpace(m.URL) == "" {
			return nil, fmt.Errorf("mcp sse config requires a non-empty 'url' field")
		}
		transport := buildHTTPTransport(ctx, name, m, tokenStore)
		client := &http.Client{Transport: transport}
		return &mcp.SSEClientTransport{
			Endpoint:   m.URL,
			HTTPClient: client,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported mcp type: %s", m.Type)
	}
}

// buildHTTPTransport creates an http.RoundTripper with appropriate middleware.
// It stacks OAuth (if configured or discovered) on top of static headers.
func buildHTTPTransport(ctx context.Context, name string, m config.MCPConfig, tokenStore *TokenStore) http.RoundTripper {
	transport := http.DefaultTransport

	// Add static headers layer
	if len(m.Headers) > 0 {
		transport = &headerRoundTripper{
			headers: m.ResolvedHeaders(),
			base:    transport,
		}
	}

	// Skip OAuth if explicitly disabled
	if !m.OAuth.IsEnabled() {
		slog.Debug("OAuth disabled for MCP", "name", name)
		return transport
	}

	// Resolve OAuth configuration (explicit or auto-discovered)
	oauthCfg := resolveOAuthConfig(ctx, m)

	// Add OAuth layer if we have configuration
	if oauthCfg != nil && oauthCfg.AuthURL != "" && oauthCfg.TokenURL != "" {
		provider, err := NewOAuthTokenProvider(name, *oauthCfg, tokenStore)
		if err != nil {
			slog.Error("Failed to create OAuth provider", "mcp", name, "error", err)
			return transport // Fall back to non-OAuth transport
		}

		// Set up the auth function immediately so it's available when needed
		mcpName := name // capture for closure
		provider.SetAuthFunc(func(ctx context.Context, cfg mcpoauth.Config) (*oauth.Token, error) {
			slog.Info("Starting OAuth authorization flow", "mcp", mcpName)

			opts := mcpoauth.DefaultAuthFlowOptions()
			opts.OnAuthURL = func(url string) {
				slog.Info("Please authorize in your browser", "mcp", mcpName, "url", url)
			}

			return mcpoauth.StartAuthFlow(ctx, cfg, opts)
		})
		slog.Debug("OAuth auth function configured for MCP", "name", name)

		registerTokenProvider(name, provider)

		transport = NewOAuthRoundTripper(provider, transport)
	}

	return transport
}

// resolveOAuthConfig returns the OAuth configuration for an MCP server.
// It first checks for explicit configuration, then attempts auto-discovery.
// Returns nil if no OAuth configuration is available.
func resolveOAuthConfig(ctx context.Context, m config.MCPConfig) *mcpoauth.Config {
	// Check for explicit configuration
	if m.OAuth != nil && m.OAuth.ClientID != "" {
		return &mcpoauth.Config{
			ClientID:     m.OAuth.ClientID,
			ClientSecret: m.OAuth.ClientSecret,
			AuthURL:      m.OAuth.AuthURL,
			TokenURL:     m.OAuth.TokenURL,
			Scopes:       m.OAuth.Scopes,
			RedirectURI:  m.OAuth.RedirectURI,
		}
	}

	// Try auto-discovery
	cfg, err := mcpoauth.DiscoverOAuth(ctx, m.URL)
	if err != nil || cfg == nil {
		return nil
	}

	return cfg
}

type headerRoundTripper struct {
	headers map[string]string
	base    http.RoundTripper
}

func (rt headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range rt.headers {
		req.Header.Set(k, v)
	}
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func mcpTimeout(m config.MCPConfig) time.Duration {
	return time.Duration(cmp.Or(m.Timeout, 15)) * time.Second
}

func stdioCheck(old *exec.Cmd) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	cmd := exec.CommandContext(ctx, old.Path, old.Args...)
	cmd.Env = old.Env
	out, err := cmd.CombinedOutput()
	if err == nil || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil
	}
	return fmt.Errorf("%w: %s", err, string(out))
}

// registerTokenProvider registers a token provider for an MCP server.
func registerTokenProvider(name string, provider *OAuthTokenProvider) {
	tokenProviders.Set(name, provider)
}
