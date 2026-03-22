package receiver

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func TestLoadConfigFromEnvAndSetup(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "webhook-receiver.db")
	t.Setenv("WEBHOOK_RECEIVER_LISTEN_ADDR", " 127.0.0.1:0 ")
	t.Setenv("WEBHOOK_RECEIVER_STORE_PATH", storePath)
	t.Setenv(encryptionKeyEnvName, "  "+testEncryptionKey+"  ")
	t.Setenv(publicBaseURLEnvName, " https://hooks.example.com/base/ ")
	t.Setenv(clientIPHeaderEnvName, " Fly-Client-IP ")

	config := LoadConfigFromEnv()
	assert.Equal(t, "127.0.0.1:0", config.ListenAddr)
	assert.Equal(t, storePath, config.StorePath)
	assert.Equal(t, testEncryptionKey, config.EncryptionKey)
	assert.Equal(t, "https://hooks.example.com/base/", config.PublicBaseURL)
	assert.Equal(t, "Fly-Client-IP", config.ClientIPHeader)

	server := Setup()
	require.NotNil(t, server)
	require.NotNil(t, server.httpServer)
	assert.Equal(t, "127.0.0.1:0", server.httpServer.Addr)
	assert.FileExists(t, storePath)
	require.NoError(t, server.Close())
}

func TestNormalizePublicBaseURL(t *testing.T) {
	normalized, err := normalizePublicBaseURL(" https://hooks.example.com/base/ ")
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.example.com/base", normalized)

	_, err = normalizePublicBaseURL("hooks.example.com/base")
	require.Error(t, err)
	assert.Contains(t, err.Error(), publicBaseURLEnvName)
}

func TestNewServerRejectsInvalidConfig(t *testing.T) {
	tempDir := t.TempDir()

	_, err := NewServer(Config{
		StorePath: filepath.Join(tempDir, "missing-key.db"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), encryptionKeyEnvName)

	_, err = NewServer(Config{
		ListenAddr:    "127.0.0.1:0",
		StorePath:     filepath.Join(tempDir, "invalid-url.db"),
		EncryptionKey: testEncryptionKey,
		PublicBaseURL: "relative-path",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), publicBaseURLEnvName)
}

func TestServerRunShutsDownOnContextCancel(t *testing.T) {
	listenAddr := freeLocalAddress(t)
	server, err := NewServer(Config{
		ListenAddr:    listenAddr,
		StorePath:     filepath.Join(t.TempDir(), "run.db"),
		EncryptionKey: testEncryptionKey,
		PublicBaseURL: "https://hooks.example.com/",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx)
	}()

	var createResponse struct {
		ID        string `json:"id"`
		DetailURL string `json:"detailUrl"`
		HookURL   string `json:"hookUrl"`
	}

	require.Eventually(t, func() bool {
		req, err := http.NewRequest(http.MethodPost, "http://"+listenAddr+"/api/webhooks", strings.NewReader(`{}`))
		if err != nil {
			return false
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return false
		}

		decodeErr := json.NewDecoder(resp.Body).Decode(&createResponse)
		closeErr := resp.Body.Close()
		return decodeErr == nil && closeErr == nil
	}, 5*time.Second, 50*time.Millisecond)

	assert.NotEmpty(t, createResponse.ID)
	assert.Equal(t, "https://hooks.example.com/webhooks/"+createResponse.ID, createResponse.DetailURL)
	assert.Equal(t, "https://hooks.example.com/hooks/"+createResponse.ID, createResponse.HookURL)

	cancel()
	require.NoError(t, <-errCh)
	assert.Nil(t, server.store)
	assert.Nil(t, server.cleanupStop)
	assert.Nil(t, server.cleanupDone)
}

func TestCloseIsIdempotent(t *testing.T) {
	server, err := NewServer(Config{
		ListenAddr:    "127.0.0.1:0",
		StorePath:     filepath.Join(t.TempDir(), "close.db"),
		EncryptionKey: testEncryptionKey,
	})
	require.NoError(t, err)

	require.NoError(t, server.Close())
	require.NoError(t, server.Close())
}

func freeLocalAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	return addr
}
