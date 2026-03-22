package handler

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanPathSegments(t *testing.T) {
	assert.Nil(t, cleanPathSegments("/"))
	assert.Nil(t, cleanPathSegments(""))
	assert.Equal(t, []string{"hooks", "webhook-123"}, cleanPathSegments("/hooks/webhook-123"))
	assert.Equal(t, []string{"api", "webhooks", "id", "messages"}, cleanPathSegments("api/webhooks/id/messages"))
}

func TestCapabilityURL(t *testing.T) {
	assert.Equal(t, "/hooks/id", capabilityURL("", "/hooks/id"))
	assert.Equal(t, "https://hooks.example.com/hooks/id", capabilityURL("https://hooks.example.com", "/hooks/id"))
}

func TestRequestBaseURLUsesConfiguredPublicBaseURL(t *testing.T) {
	h := NewHandler(nil, WithPublicBaseURL("https://hooks.example.com"))
	req := httptest.NewRequest(http.MethodGet, "http://localhost/webhooks/id", nil)

	assert.Equal(t, "https://hooks.example.com", h.requestBaseURL(req))
}

func TestRequestBaseURLAllowsOnlySafeLocalFallbacks(t *testing.T) {
	h := NewHandler(nil)

	localReq := httptest.NewRequest(http.MethodGet, "http://localhost:8080/webhooks/id", nil)
	localReq.Host = "localhost:8080"
	localReq.RemoteAddr = "127.0.0.1:1234"
	assert.Equal(t, "http://localhost:8080", h.requestBaseURL(localReq))

	secureLocalReq := httptest.NewRequest(http.MethodGet, "https://[::1]:8080/webhooks/id", nil)
	secureLocalReq.Host = "[::1]:8080"
	secureLocalReq.RemoteAddr = "[::1]:4567"
	secureLocalReq.TLS = &tls.ConnectionState{}
	assert.Equal(t, "https://[::1]:8080", h.requestBaseURL(secureLocalReq))

	externalHostReq := httptest.NewRequest(http.MethodGet, "http://localhost/webhooks/id", nil)
	externalHostReq.Host = "evil.example"
	externalHostReq.RemoteAddr = "127.0.0.1:1234"
	assert.Empty(t, h.requestBaseURL(externalHostReq))

	externalRemoteReq := httptest.NewRequest(http.MethodGet, "http://localhost/webhooks/id", nil)
	externalRemoteReq.Host = "localhost:8080"
	externalRemoteReq.RemoteAddr = "198.51.100.10:1234"
	assert.Empty(t, h.requestBaseURL(externalRemoteReq))
}

func TestWriteJSON(t *testing.T) {
	h := NewHandler(nil)
	w := httptest.NewRecorder()

	h.writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})

	assert.Equal(t, http.StatusCreated, w.Result().StatusCode)
	assert.Equal(t, "application/json", w.Result().Header.Get("Content-Type"))

	var payload map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, map[string]string{"status": "ok"}, payload)
}

func TestTooManyRequestsHandlerReturnsJSONForAPIAndHooks(t *testing.T) {
	h := NewHandler(nil)

	for _, path := range []string{"/api/webhooks", "/hooks/webhook-123"} {
		req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
		w := httptest.NewRecorder()

		h.tooManyRequestsHandler(w, req)

		assert.Equal(t, http.StatusTooManyRequests, w.Result().StatusCode)
		assert.Equal(t, "application/json", w.Result().Header.Get("Content-Type"))
		assert.JSONEq(t, `{"message":"Too many requests from this IP. Please retry later."}`, w.Body.String())
	}
}

func TestTooManyRequestsHandlerReturnsPlainTextForUI(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	w := httptest.NewRecorder()

	h.tooManyRequestsHandler(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Result().StatusCode)
	assert.Contains(t, w.Body.String(), "Too many requests from this IP. Please retry later.")
	assert.Contains(t, w.Result().Header.Get("Content-Type"), "text/plain")
}

func TestAllowRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	req.RemoteAddr = "198.51.100.10:1234"

	noLimiterHandler := NewHandler(nil)
	noLimiterHandler.limiter = nil
	assert.True(t, noLimiterHandler.allowRequest(httptest.NewRecorder(), req))

	limitedHandler := NewHandler(nil, WithRateLimit(1, time.Hour))
	first := httptest.NewRecorder()
	second := httptest.NewRecorder()

	assert.True(t, limitedHandler.allowRequest(first, req))
	assert.False(t, limitedHandler.allowRequest(second, req))
	assert.Equal(t, http.StatusTooManyRequests, second.Result().StatusCode)
}

func TestClientIP(t *testing.T) {
	headerHandler := NewHandler(nil, WithClientIPHeader("Fly-Client-IP"))
	headerReq := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	headerReq.RemoteAddr = "127.0.0.1:1234"
	headerReq.Header.Set("Fly-Client-IP", "203.0.113.10")
	assert.Equal(t, "203.0.113.10", headerHandler.clientIP(headerReq))

	invalidHeaderReq := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	invalidHeaderReq.RemoteAddr = "198.51.100.10:4321"
	invalidHeaderReq.Header.Set("Fly-Client-IP", "not-an-ip")
	assert.Equal(t, "198.51.100.10", headerHandler.clientIP(invalidHeaderReq))

	unknownReq := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	unknownReq.RemoteAddr = ""
	assert.Equal(t, "unknown", headerHandler.clientIP(unknownReq))
}

func TestRemoteRequestIP(t *testing.T) {
	hostPortReq := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	hostPortReq.RemoteAddr = "198.51.100.10:1234"
	parsedIP, value := remoteRequestIP(hostPortReq)
	require.NotNil(t, parsedIP)
	assert.Equal(t, "198.51.100.10", parsedIP.String())
	assert.Equal(t, "198.51.100.10", value)

	hostReq := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	hostReq.RemoteAddr = "client.internal"
	parsedIP, value = remoteRequestIP(hostReq)
	assert.Nil(t, parsedIP)
	assert.Equal(t, "client.internal", value)

	emptyReq := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	emptyReq.RemoteAddr = ""
	parsedIP, value = remoteRequestIP(emptyReq)
	assert.Nil(t, parsedIP)
	assert.Empty(t, value)
}

func TestSafeLocalRequestBaseURL(t *testing.T) {
	localReq := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	localReq.Host = "localhost:8080"
	localReq.RemoteAddr = "127.0.0.1:1234"
	assert.True(t, safeLocalRequestBaseURL(localReq))

	loopbackReq := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	loopbackReq.Host = "[::1]:8080"
	loopbackReq.RemoteAddr = "[::1]:1234"
	assert.True(t, safeLocalRequestBaseURL(loopbackReq))

	externalHostReq := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	externalHostReq.Host = "evil.example"
	externalHostReq.RemoteAddr = "127.0.0.1:1234"
	assert.False(t, safeLocalRequestBaseURL(externalHostReq))

	externalRemoteReq := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	externalRemoteReq.Host = "localhost:8080"
	externalRemoteReq.RemoteAddr = "198.51.100.10:1234"
	assert.False(t, safeLocalRequestBaseURL(externalRemoteReq))
}

func TestHostWithoutPort(t *testing.T) {
	assert.Equal(t, "localhost", hostWithoutPort("localhost:8080"))
	assert.Equal(t, "::1", hostWithoutPort("[::1]:8080"))
	assert.Equal(t, "evil.example", hostWithoutPort("evil.example"))
	assert.Equal(t, "", hostWithoutPort(""))
}
