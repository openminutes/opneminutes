package minutes

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestNewClientValidationAndDefaults(t *testing.T) {
	t.Run("default URLs", func(t *testing.T) {
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
		if client.userAgent != defaultUserAgent {
			t.Fatalf("userAgent = %q, want default user agent", client.userAgent)
		}
	})

	t.Run("missing cookie", func(t *testing.T) {
		_, err := NewClient(Config{Cookie: " \t"})
		if err == nil {
			t.Fatal("NewClient() error = nil, want cookie error")
		}
		if err.Error() != "cookie is required" {
			t.Fatalf("NewClient() error = %q, want cookie required", err.Error())
		}
	})

	t.Run("trims custom values", func(t *testing.T) {
		client, err := NewClient(Config{
			Cookie:       " " + testCookie + " ",
			BaseURL:      " https://example.test/ ",
			SpaceBaseURL: " https://space.example.test/ ",
			UserAgent:    " custom-agent ",
		})
		if err != nil {
			t.Fatalf("NewClient() error = %v, want nil", err)
		}
		if client.baseURL != "https://example.test" {
			t.Fatalf("baseURL = %q, want trimmed custom base", client.baseURL)
		}
		if client.spaceBaseURL != "https://space.example.test" {
			t.Fatalf("spaceBaseURL = %q, want trimmed custom space base", client.spaceBaseURL)
		}
		if client.userAgent != "custom-agent" {
			t.Fatalf("userAgent = %q, want custom-agent", client.userAgent)
		}
	})

	t.Run("invalid base url", func(t *testing.T) {
		_, err := NewClient(Config{
			Cookie:  testCookie,
			BaseURL: "meetings.example.test",
		})
		if err == nil {
			t.Fatal("NewClient() error = nil, want invalid base URL error")
		}
		want := `invalid base_url "meetings.example.test": must be an absolute http or https URL with a host`
		if err.Error() != want {
			t.Fatalf("NewClient() error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("invalid base url query", func(t *testing.T) {
		_, err := NewClient(Config{
			Cookie:  testCookie,
			BaseURL: "https://example.test?token=secret",
		})
		if err == nil {
			t.Fatal("NewClient() error = nil, want invalid base URL error")
		}
		want := `invalid base_url "https://example.test?token=secret": must be an absolute http or https URL with a host`
		if err.Error() != want {
			t.Fatalf("NewClient() error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("invalid space base url", func(t *testing.T) {
		_, err := NewClient(Config{
			Cookie:       testCookie,
			SpaceBaseURL: "ftp://space.example.test",
		})
		if err == nil {
			t.Fatal("NewClient() error = nil, want invalid space base URL error")
		}
		want := `invalid space_base_url "ftp://space.example.test": must be an absolute http or https URL with a host`
		if err.Error() != want {
			t.Fatalf("NewClient() error = %q, want %q", err.Error(), want)
		}
	})
}

func TestCSRFTokenFromCookieSkipsMalformedSegments(t *testing.T) {
	got, err := csrfTokenFromCookie("malformed; session=abc; bv_csrf_token=csrf-token")
	if err != nil {
		t.Fatalf("csrfTokenFromCookie() error = %v, want nil", err)
	}
	if got != "csrf-token" {
		t.Fatalf("csrfTokenFromCookie() = %q, want csrf-token", got)
	}
}

func TestDoJSONEdgeCases(t *testing.T) {
	t.Run("transport error", func(t *testing.T) {
		wantErr := errors.New("transport failed")
		client := newClientWithTransport(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, wantErr
		}))
		req := newTestRequest(t, http.MethodGet, "https://example.test/api?secret=value")

		if err := client.doJSON(req, nil); !errors.Is(err, wantErr) {
			t.Fatalf("doJSON() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("empty body without result", func(t *testing.T) {
		client := newClientWithResponse(t, http.StatusOK, "", nil)
		req := newTestRequest(t, http.MethodPost, "https://example.test/api")

		if err := client.doJSON(req, nil); err != nil {
			t.Fatalf("doJSON() error = %v, want nil", err)
		}
	})

	t.Run("decode error", func(t *testing.T) {
		client := newClientWithResponse(t, http.StatusOK, "{", nil)
		req := newTestRequest(t, http.MethodGet, "https://example.test/api")
		var result struct{}

		if err := client.doJSON(req, &result); err == nil {
			t.Fatal("doJSON() error = nil, want decode error")
		}
	})

	t.Run("missing data", func(t *testing.T) {
		client := newClientWithResponse(t, http.StatusOK, `{"code":0}`, nil)
		req := newTestRequest(t, http.MethodGet, "https://example.test/api")
		var result struct{}

		err := client.doJSON(req, &result)
		if err == nil || !strings.Contains(err.Error(), "response missing data") {
			t.Fatalf("doJSON() error = %v, want missing data", err)
		}
	})

	t.Run("null data", func(t *testing.T) {
		client := newClientWithResponse(t, http.StatusOK, `{"code":0,"data":null}`, nil)
		req := newTestRequest(t, http.MethodGet, "https://example.test/api")
		var result struct{}

		err := client.doJSON(req, &result)
		if err == nil || !strings.Contains(err.Error(), "response missing data") {
			t.Fatalf("doJSON() error = %v, want missing data", err)
		}
	})

	t.Run("data unmarshal error", func(t *testing.T) {
		client := newClientWithResponse(t, http.StatusOK, `{"code":0,"data":"bad"}`, nil)
		req := newTestRequest(t, http.MethodGet, "https://example.test/api")
		var result struct {
			Value int `json:"value"`
		}

		if err := client.doJSON(req, &result); err == nil {
			t.Fatal("doJSON() error = nil, want unmarshal error")
		}
	})

	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "message field", body: `{"code":7,"message":"from message"}`, want: "from message"},
		{name: "fallback message", body: `{"code":7}`, want: "request failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newClientWithResponse(t, http.StatusOK, tt.body, nil)
			req := newTestRequest(t, http.MethodGet, "https://example.test/api")

			err := client.doJSON(req, nil)
			if err == nil {
				t.Fatal("doJSON() error = nil, want server code error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("doJSON() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestDoRawEdgeCases(t *testing.T) {
	t.Run("transport error", func(t *testing.T) {
		wantErr := errors.New("transport failed")
		client := newClientWithTransport(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, wantErr
		}))
		req := newTestRequest(t, http.MethodGet, "https://example.test/raw")

		if _, err := client.doRaw(req); !errors.Is(err, wantErr) {
			t.Fatalf("doRaw() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("status error", func(t *testing.T) {
		client := newClientWithResponse(t, http.StatusBadGateway, "bad gateway", nil)
		req := newTestRequest(t, http.MethodGet, "https://example.test/raw")

		if _, err := client.doRaw(req); err == nil || !strings.Contains(err.Error(), "502 Bad Gateway") {
			t.Fatalf("doRaw() error = %v, want status error", err)
		}
	})

	t.Run("read error", func(t *testing.T) {
		wantErr := errors.New("read failed")
		client := newClientWithTransport(t, responseTransport(http.StatusOK, "text/plain", errReadCloser{err: wantErr}))
		req := newTestRequest(t, http.MethodGet, "https://example.test/raw")

		if _, err := client.doRaw(req); !errors.Is(err, wantErr) {
			t.Fatalf("doRaw() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("json content type with invalid JSON returns bytes", func(t *testing.T) {
		headers := http.Header{"Content-Type": []string{"application/json"}}
		client := newClientWithResponse(t, http.StatusOK, "not-json", headers)
		req := newTestRequest(t, http.MethodGet, "https://example.test/raw")

		got, err := client.doRaw(req)
		if err != nil {
			t.Fatalf("doRaw() error = %v, want nil", err)
		}
		if string(got) != "not-json" {
			t.Fatalf("doRaw() = %q, want original bytes", got)
		}
	})

	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "message field", body: `{"code":8,"message":"raw denied"}`, want: "raw denied"},
		{name: "fallback message", body: `{"code":8}`, want: "request failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}
			client := newClientWithResponse(t, http.StatusOK, tt.body, headers)
			req := newTestRequest(t, http.MethodGet, "https://example.test/raw")

			_, err := client.doRaw(req)
			if err == nil {
				t.Fatal("doRaw() error = nil, want server code error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("doRaw() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestDoStreamEdgeCases(t *testing.T) {
	t.Run("transport error", func(t *testing.T) {
		wantErr := errors.New("transport failed")
		client := newClientWithTransport(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, wantErr
		}))
		req := newTestRequest(t, http.MethodGet, "https://example.test/video")

		if err := client.doStream(req, io.Discard); !errors.Is(err, wantErr) {
			t.Fatalf("doStream() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("read error", func(t *testing.T) {
		wantErr := errors.New("read failed")
		client := newClientWithTransport(t, responseTransport(http.StatusOK, "video/mp4", errReadCloser{err: wantErr}))
		req := newTestRequest(t, http.MethodGet, "https://example.test/video")

		if err := client.doStream(req, io.Discard); !errors.Is(err, wantErr) {
			t.Fatalf("doStream() error = %v, want %v", err, wantErr)
		}
	})
}

func TestErrorLogFieldAndRedactionHelpers(t *testing.T) {
	client := newTestClient(t, "https://example.test", "https://space.example.test")
	req := newTestRequest(t, http.MethodGet, "https://external.example.test/path?secret=value")

	if field := client.errorLogField(req, nil); field.String != "" {
		t.Fatalf("errorLogField(nil) = %#v, want empty string field", field)
	}
	if got := redactURLQueryText("no query here"); got != "no query here" {
		t.Fatalf("redactURLQueryText() = %q, want unchanged", got)
	}
	if got := redactURLQueryText("GET /path?token=secret next"); got != "GET /path?<redacted> next" {
		t.Fatalf("redactURLQueryText() = %q, want redacted query", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newClientWithTransport(t *testing.T, transport http.RoundTripper) *Client {
	t.Helper()

	client := newTestClient(t, "https://example.test", "https://space.example.test")
	client.httpClient = &http.Client{Transport: transport}
	return client
}

func newClientWithResponse(t *testing.T, status int, body string, headers http.Header) *Client {
	t.Helper()

	contentType := ""
	if headers != nil {
		contentType = headers.Get("content-type")
	}
	return newClientWithTransport(t, responseTransport(status, contentType, io.NopCloser(strings.NewReader(body))))
}

func responseTransport(status int, contentType string, body io.ReadCloser) roundTripFunc {
	return func(req *http.Request) (*http.Response, error) {
		header := http.Header{}
		if contentType != "" {
			header.Set("content-type", contentType)
		}

		return &http.Response{
			StatusCode: status,
			Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
			Header:     header,
			Body:       body,
			Request:    req,
		}, nil
	}
}

func newTestRequest(t *testing.T, method, rawURL string) *http.Request {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	return &http.Request{
		Method: method,
		URL:    parsed,
		Header: http.Header{},
	}
}

type errReadCloser struct {
	err error
}

func (r errReadCloser) Read([]byte) (int, error) {
	return 0, r.err
}

func (r errReadCloser) Close() error {
	return nil
}
