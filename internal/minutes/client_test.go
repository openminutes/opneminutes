package minutes

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testCookie = "session=abc; bv_csrf_token=csrf-token; other=value"

func TestNewClientExtractsCSRFToken(t *testing.T) {
	client, err := NewClient(Config{
		Region:    "feishu",
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
		Region: "feishu",
		Cookie: "session=abc",
	})
	if err == nil {
		t.Fatal("NewClient() error = nil, want error")
	}

	if err.Error() != "cookie does not contain bv_csrf_token" {
		t.Fatalf("NewClient() error = %q, want missing csrf token", err.Error())
	}
}

func TestNewClientRequiresBaseURLForLarkSuite(t *testing.T) {
	_, err := NewClient(Config{
		Region: "larksuite",
		Cookie: testCookie,
	})
	if err == nil {
		t.Fatal("NewClient() error = nil, want error")
	}
	if err.Error() != `base URL is required for region "larksuite"` {
		t.Fatalf("NewClient() error = %q, want larksuite base URL requirement", err.Error())
	}
}

func TestNewClientAcceptsLarkSuiteWithBaseURL(t *testing.T) {
	_, err := NewClient(Config{
		Region:  "larksuite",
		Cookie:  testCookie,
		BaseURL: "https://example.larksuite.test",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
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
}

func newTestClient(t *testing.T, baseURL, spaceBaseURL string) *Client {
	t.Helper()

	client, err := NewClient(Config{
		Region:       "feishu",
		Cookie:       testCookie,
		BaseURL:      baseURL,
		SpaceBaseURL: spaceBaseURL,
		UserAgent:    "openminutes-test",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
	}

	return client
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
