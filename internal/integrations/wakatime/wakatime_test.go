package wakatime

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNew_DisabledReturnsNil(t *testing.T) {
	t.Parallel()

	svc, err := New(Config{Enabled: false})
	require.NoError(t, err)
	require.Nil(t, svc)
}

func TestService_SendHeartbeat_NilSafe(t *testing.T) {
	t.Parallel()

	var svc *Service
	// Should not panic.
	svc.SendHeartbeat(context.Background(), Heartbeat{
		FilePath: "/test/file.go",
		IsWrite:  false,
	})
}

func TestService_ShouldSend_AlwaysOnWrite(t *testing.T) {
	t.Parallel()

	svc := &Service{
		lastHeartbeats: make(map[string]time.Time),
	}

	// Record a recent heartbeat.
	svc.lastHeartbeats["/test/file.go"] = time.Now()

	// Write events should always be sent.
	require.True(t, svc.shouldSend("/test/file.go", true))
}

func TestService_ShouldSend_ThrottlesReads(t *testing.T) {
	t.Parallel()

	svc := &Service{
		lastHeartbeats: make(map[string]time.Time),
	}

	// First read should send.
	require.True(t, svc.shouldSend("/test/file.go", false))

	// Record heartbeat.
	svc.lastHeartbeats["/test/file.go"] = time.Now()

	// Immediate second read should be throttled.
	require.False(t, svc.shouldSend("/test/file.go", false))

	// Different file should send.
	require.True(t, svc.shouldSend("/test/other.go", false))
}

func TestHook_WrapTools_NilSafe(t *testing.T) {
	t.Parallel()

	var hook *Hook
	result := hook.WrapTools(nil)
	require.Nil(t, result)
}

func TestExtractFilePath_FilePath(t *testing.T) {
	t.Parallel()

	params := `{"file_path": "/test/file.go", "content": "test"}`
	path := extractFilePath(params, "/working")
	require.Equal(t, "/test/file.go", path)
}

func TestExtractFilePath_RelativePath(t *testing.T) {
	t.Parallel()

	params := `{"file_path": "src/main.go"}`
	path := extractFilePath(params, "/working")
	require.Equal(t, "/working/src/main.go", path)
}

func TestExtractFilePath_PathParam(t *testing.T) {
	t.Parallel()

	params := `{"pattern": "*.go", "path": "/src"}`
	path := extractFilePath(params, "/working")
	require.Equal(t, "/src", path)
}

func TestDetectProject_ReturnsBasename(t *testing.T) {
	t.Parallel()

	// Without project markers, returns parent directory name.
	project := detectProject("/some/random/path/file.go")
	require.Equal(t, "path", project)
}
