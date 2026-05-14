package minutes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const testCookie = "session=abc; bv_csrf_token=csrf-token; other=value"

func TestNewClientExtractsCSRFToken(t *testing.T) {
	client, err := NewClient(Config{
		Cookie:    "session=abc; bv_csrf_token=csrf-token; other=value",
		BaseURL:   "https://example.test",
		UserAgent: "test-agent",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
	}

	if client.csrfToken != "csrf-token" {
		t.Fatalf("csrf token = %q, want csrf-token", client.csrfToken)
	}
	if client.referer != "https://example.test/minutes/home" {
		t.Fatalf("referer = %q, want https://example.test/minutes/home", client.referer)
	}
}

func TestNewClientRejectsMissingCSRFToken(t *testing.T) {
	_, err := NewClient(Config{
		Cookie: "session=abc",
	})
	if err == nil {
		t.Fatal("NewClient() error = nil, want error")
	}

	if err.Error() != "cookie does not contain bv_csrf_token" {
		t.Fatalf("NewClient() error = %q, want missing csrf token", err.Error())
	}
}

func TestNewClientDefaultsBaseURLs(t *testing.T) {
	client, err := NewClient(Config{Cookie: testCookie})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
	}
	if client.baseURL != defaultBaseURL {
		t.Fatalf("baseURL = %q, want %q", client.baseURL, defaultBaseURL)
	}
	if client.spaceBaseURL != defaultSpaceBaseURL {
		t.Fatalf("spaceBaseURL = %q, want %q", client.spaceBaseURL, defaultSpaceBaseURL)
	}
}

func TestNewClientUsesDefaultHTTPTimeout(t *testing.T) {
	client, err := NewClient(Config{Cookie: testCookie})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
	}
	if client.httpClient == nil {
		t.Fatal("httpClient = nil, want default client")
	}
	if client.httpClient.Timeout != defaultClientTimeout {
		t.Fatalf("httpClient.Timeout = %s, want %s", client.httpClient.Timeout, defaultClientTimeout)
	}
	if defaultClientTimeout <= 0 || defaultClientTimeout > time.Minute {
		t.Fatalf("defaultClientTimeout = %s, want positive bounded timeout", defaultClientTimeout)
	}
}

func TestNewClientPreservesProvidedHTTPClient(t *testing.T) {
	httpClient := &http.Client{Timeout: 7 * time.Second}
	client, err := NewClient(Config{
		Cookie:     testCookie,
		HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
	}
	if client.httpClient != httpClient {
		t.Fatalf("httpClient = %#v, want provided client", client.httpClient)
	}
}

func TestNewClientPreservesCustomBaseURLs(t *testing.T) {
	client, err := NewClient(Config{
		Cookie:       testCookie,
		BaseURL:      "https://example.test/root/",
		SpaceBaseURL: "https://space.example.test/root/",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
	}
	if client.baseURL != "https://example.test/root" {
		t.Fatalf("baseURL = %q, want custom base URL", client.baseURL)
	}
	if client.spaceBaseURL != "https://space.example.test/root" {
		t.Fatalf("spaceBaseURL = %q, want custom space base URL", client.spaceBaseURL)
	}
}

func TestClientAddsCommonHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertCommonHeaders(t, r, testCookie, "csrf-token", "https://example.test/minutes/home", "test-agent")
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{"list":[],"has_more":false}}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)
	client.referer = "https://example.test/minutes/home"
	client.userAgent = "test-agent"

	if _, err := client.ListMinutes(context.Background(), ListOptions{}); err != nil {
		t.Fatalf("ListMinutes() error = %v, want nil", err)
	}
}

func TestClientReturnsHTTPStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	_, err := client.ListMinutes(context.Background(), ListOptions{})
	if err == nil {
		t.Fatal("ListMinutes() error = nil, want error")
	}

	for _, want := range []string{"GET", "/minutes/api/space/list", "418 I'm a teapot"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want to contain %q", err.Error(), want)
		}
	}
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error type = %T, want *HTTPStatusError", err)
	}
	if statusErr.Method != http.MethodGet || !strings.HasPrefix(statusErr.RequestURI, "/minutes/api/space/list") || statusErr.StatusCode != http.StatusTeapot {
		t.Fatalf("HTTPStatusError = %#v, want request and status details", statusErr)
	}
}

func TestClientReturnsServerCodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":123,"msg":"expired"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	_, err := client.ListMinutes(context.Background(), ListOptions{})
	if err == nil {
		t.Fatal("ListMinutes() error = nil, want error")
	}

	for _, want := range []string{"GET", "/minutes/api/space/list", "server code 123", "expired"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want to contain %q", err.Error(), want)
		}
	}
	var serverErr *ServerCodeError
	if !errors.As(err, &serverErr) {
		t.Fatalf("error type = %T, want *ServerCodeError", err)
	}
	if serverErr.Method != http.MethodGet || !strings.HasPrefix(serverErr.RequestURI, "/minutes/api/space/list") || serverErr.Code != 123 || serverErr.Message != "expired" {
		t.Fatalf("ServerCodeError = %#v, want request and server details", serverErr)
	}
}

func TestNewClientDefaultsToNoopLogger(t *testing.T) {
	client, err := NewClient(Config{
		Cookie:  testCookie,
		BaseURL: "https://example.test",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
	}

	if client.logger == nil {
		t.Fatal("client.logger = nil, want no-op logger")
	}
}

func TestClientDebugLogsHTTPSuccess(t *testing.T) {
	logger, logs := observedLogger()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json; charset=utf-8")
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{"list":[],"has_more":false}}`)
	}))
	t.Cleanup(server.Close)

	client := newObservedTestClient(t, server.URL, server.URL, logger)

	if _, err := client.ListMinutes(context.Background(), ListOptions{}); err != nil {
		t.Fatalf("ListMinutes() error = %v, want nil", err)
	}

	entry := findObservedMessage(t, logs, "http request completed")
	fields := entry.ContextMap()
	if fields["method"] != "GET" {
		t.Fatalf("method = %#v, want GET", fields["method"])
	}
	if fields["url_path"] != "/minutes/api/space/list" {
		t.Fatalf("url_path = %#v, want /minutes/api/space/list", fields["url_path"])
	}
	if fields["status"] != int64(http.StatusOK) {
		t.Fatalf("status = %#v, want 200", fields["status"])
	}
	if fields["content_type"] != "application/json; charset=utf-8" {
		t.Fatalf("content_type = %#v, want application/json", fields["content_type"])
	}
	if _, ok := fields["duration"]; !ok {
		t.Fatalf("duration field missing from %#v", fields)
	}
	if _, ok := fields["response_size"]; !ok {
		t.Fatalf("response_size field missing from %#v", fields)
	}
	assertObservedLogsDoNotContainSecrets(t, logs)
}

func TestClientDebugLogsHTTPStatusError(t *testing.T) {
	logger, logs := observedLogger()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	t.Cleanup(server.Close)

	client := newObservedTestClient(t, server.URL, server.URL, logger)

	_, err := client.ListMinutes(context.Background(), ListOptions{})
	if err == nil {
		t.Fatal("ListMinutes() error = nil, want error")
	}

	entry := findObservedMessage(t, logs, "http status error")
	fields := entry.ContextMap()
	if fields["method"] != "GET" {
		t.Fatalf("method = %#v, want GET", fields["method"])
	}
	if fields["url_path"] != "/minutes/api/space/list" {
		t.Fatalf("url_path = %#v, want /minutes/api/space/list", fields["url_path"])
	}
	if fields["status"] != int64(http.StatusTeapot) {
		t.Fatalf("status = %#v, want 418", fields["status"])
	}
	assertObservedLogsDoNotContainSecrets(t, logs)
}

func TestClientDebugLogsServerCodeError(t *testing.T) {
	logger, logs := observedLogger()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		fmt.Fprint(w, `{"code":123,"msg":"expired"}`)
	}))
	t.Cleanup(server.Close)

	client := newObservedTestClient(t, server.URL, server.URL, logger)

	_, err := client.ListMinutes(context.Background(), ListOptions{})
	if err == nil {
		t.Fatal("ListMinutes() error = nil, want error")
	}

	entry := findObservedMessage(t, logs, "http server code error")
	fields := entry.ContextMap()
	if fields["method"] != "GET" {
		t.Fatalf("method = %#v, want GET", fields["method"])
	}
	if fields["url_path"] != "/minutes/api/space/list" {
		t.Fatalf("url_path = %#v, want /minutes/api/space/list", fields["url_path"])
	}
	if fields["status"] != int64(http.StatusOK) {
		t.Fatalf("status = %#v, want 200", fields["status"])
	}
	if fields["server_code"] != int64(123) {
		t.Fatalf("server_code = %#v, want 123", fields["server_code"])
	}
	if fields["server_message"] != "expired" {
		t.Fatalf("server_message = %#v, want expired", fields["server_message"])
	}
	assertObservedLogsDoNotContainSecrets(t, logs)
}

func TestClientDebugLogsRedactExternalDownloadURLQuery(t *testing.T) {
	logger, logs := observedLogger()
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "video bytes")
	}))
	t.Cleanup(downloadServer.Close)

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"video_info":{"video_download_url":%q}}}`,
			downloadServer.URL+"/video.mp4?auth=secret-token&x=1")
	}))
	t.Cleanup(apiServer.Close)

	client := newObservedTestClient(t, apiServer.URL, apiServer.URL, logger)
	var dst strings.Builder
	if err := client.DownloadFile(context.Background(), "token-1", &dst); err != nil {
		t.Fatalf("DownloadFile() error = %v, want nil", err)
	}

	var downloadEntry zapcore.Entry
	var downloadFields map[string]interface{}
	found := false
	for _, entry := range logs.All() {
		fields := entry.ContextMap()
		if fields["url_path"] == "/video.mp4" {
			downloadEntry = entry.Entry
			downloadFields = fields
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("download HTTP log not found; logs = %#v", logs.All())
	}
	if downloadEntry.Message != "http request completed" {
		t.Fatalf("download log message = %q, want http request completed", downloadEntry.Message)
	}
	if downloadFields["url_query_redacted"] != true {
		t.Fatalf("url_query_redacted = %#v, want true", downloadFields["url_query_redacted"])
	}
	if _, ok := downloadFields["url_query"]; ok {
		t.Fatalf("url_query = %#v, want omitted for external download URL", downloadFields["url_query"])
	}
	for _, entry := range logs.All() {
		if strings.Contains(fmt.Sprint(entry.ContextMap()), "secret-token") {
			t.Fatalf("log fields contain secret download query: %#v", entry.ContextMap())
		}
	}
	assertObservedLogsDoNotContainSecrets(t, logs)
}

func TestClientDebugLogsRedactExternalDownloadURLQueryOnStatusError(t *testing.T) {
	logger, logs := observedLogger()
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied", http.StatusForbidden)
	}))
	t.Cleanup(downloadServer.Close)

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"video_info":{"video_download_url":%q}}}`,
			downloadServer.URL+"/video.mp4?auth=secret-token&x=1")
	}))
	t.Cleanup(apiServer.Close)

	client := newObservedTestClient(t, apiServer.URL, apiServer.URL, logger)
	var dst strings.Builder
	if err := client.DownloadFile(context.Background(), "token-1", &dst); err == nil {
		t.Fatal("DownloadFile() error = nil, want error")
	}

	entry := findObservedMessage(t, logs, "http status error")
	fields := entry.ContextMap()
	if fields["url_path"] != "/video.mp4" {
		t.Fatalf("url_path = %#v, want /video.mp4", fields["url_path"])
	}
	if fields["url_query_redacted"] != true {
		t.Fatalf("url_query_redacted = %#v, want true", fields["url_query_redacted"])
	}
	if strings.Contains(fmt.Sprint(fields), "secret-token") {
		t.Fatalf("log fields contain secret download query: %#v", fields)
	}
	assertObservedLogsDoNotContainSecrets(t, logs)
}

func newTestClient(t *testing.T, baseURL, spaceBaseURL string) *Client {
	t.Helper()

	return newObservedTestClient(t, baseURL, spaceBaseURL, nil)
}

func newObservedTestClient(t *testing.T, baseURL, spaceBaseURL string, logger *zap.Logger) *Client {
	t.Helper()

	client, err := NewClient(Config{
		Cookie:       testCookie,
		BaseURL:      baseURL,
		SpaceBaseURL: spaceBaseURL,
		UserAgent:    "openminutes-test",
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
	}

	return client
}

func observedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	return zap.New(core), logs
}

func findObservedMessage(t *testing.T, logs *observer.ObservedLogs, message string) observer.LoggedEntry {
	t.Helper()

	for _, entry := range logs.All() {
		if entry.Message == message {
			return entry
		}
	}

	t.Fatalf("log message %q not found; logs = %#v", message, logs.All())
	return observer.LoggedEntry{}
}

func assertObservedLogsDoNotContainSecrets(t *testing.T, logs *observer.ObservedLogs) {
	t.Helper()

	for _, entry := range logs.All() {
		text := fmt.Sprint(entry.ContextMap())
		for _, secret := range []string{testCookie, "session=abc", "csrf-token"} {
			if strings.Contains(text, secret) {
				t.Fatalf("log fields contain secret %q: %#v", secret, entry.ContextMap())
			}
		}
	}
}

func assertCommonHeaders(t *testing.T, r *http.Request, cookie, csrfToken, referer, userAgent string) {
	t.Helper()

	if got := r.Header.Get("cookie"); got != cookie {
		t.Fatalf("cookie header = %q, want %q", got, cookie)
	}
	if got := r.Header.Get("bv-csrf-token"); got != csrfToken {
		t.Fatalf("bv-csrf-token header = %q, want %q", got, csrfToken)
	}
	if got := r.Header.Get("referer"); got != referer {
		t.Fatalf("referer header = %q, want %q", got, referer)
	}
	if got := r.Header.Get("user-agent"); got != userAgent {
		t.Fatalf("user-agent header = %q, want %q", got, userAgent)
	}
}
