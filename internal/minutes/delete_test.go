package minutes

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDeleteMinuteSoftDeleteFlow(t *testing.T) {
	var calls []string
	expectedReferer := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertCommonHeaders(t, r, testCookie, "csrf-token", expectedReferer, "openminutes-test")
		calls = append(calls, r.Method+" "+r.URL.Path+" "+readFormBody(t, r))
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{}}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)
	expectedReferer = client.referer

	if err := client.DeleteMinute(context.Background(), "token-1", DeleteOptions{}); err != nil {
		t.Fatalf("DeleteMinute() error = %v, want nil", err)
	}

	want := strings.Join([]string{
		"POST /minutes/api/space/remove language=zh_cn&object_tokens=token-1&space_name=1",
		"POST /minutes/api/space/delete is_destroyed=false&language=zh_cn&object_tokens=token-1",
	}, ",")
	if got := strings.Join(calls, ","); got != want {
		t.Fatalf("calls = %s, want %s", got, want)
	}
}

func TestDeleteMinuteUsesCustomLanguageAndSpaceName(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path+" "+readFormBody(t, r))
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{}}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	if err := client.DeleteMinute(context.Background(), "token-1", DeleteOptions{
		Language:  "en_us",
		SpaceName: 3,
	}); err != nil {
		t.Fatalf("DeleteMinute() error = %v, want nil", err)
	}

	want := strings.Join([]string{
		"POST /minutes/api/space/remove language=en_us&object_tokens=token-1&space_name=3",
		"POST /minutes/api/space/delete is_destroyed=false&language=en_us&object_tokens=token-1",
	}, ",")
	if got := strings.Join(calls, ","); got != want {
		t.Fatalf("calls = %s, want %s", got, want)
	}
}

func TestDeleteMinuteDestroyAddsPermanentDelete(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path+" "+readFormBody(t, r))
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{}}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	if err := client.DeleteMinute(context.Background(), "token-1", DeleteOptions{Destroy: true}); err != nil {
		t.Fatalf("DeleteMinute() error = %v, want nil", err)
	}

	want := strings.Join([]string{
		"POST /minutes/api/space/remove language=zh_cn&object_tokens=token-1&space_name=1",
		"POST /minutes/api/space/delete is_destroyed=false&language=zh_cn&object_tokens=token-1",
		"POST /minutes/api/space/delete is_destroyed=true&language=zh_cn&object_tokens=token-1",
	}, ",")
	if got := strings.Join(calls, ","); got != want {
		t.Fatalf("calls = %s, want %s", got, want)
	}
}

func TestDeleteMinuteRejectsInvalidOptions(t *testing.T) {
	client := newTestClient(t, "https://example.test", "https://space.example.test")

	tests := []struct {
		name        string
		objectToken string
		options     DeleteOptions
		wantErr     string
	}{
		{
			name:        "empty token",
			objectToken: "",
			wantErr:     "object token is required",
		},
		{
			name:        "blank token",
			objectToken: " \t",
			wantErr:     "object token is required",
		},
		{
			name:        "negative space name",
			objectToken: "token-1",
			options:     DeleteOptions{SpaceName: -1},
			wantErr:     "space name must be greater than 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.DeleteMinute(context.Background(), tt.objectToken, tt.options)
			if err == nil {
				t.Fatal("DeleteMinute() error = nil, want error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("DeleteMinute() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestDeleteMinuteReturnsServerCodeErrorAndStops(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"code":9,"msg":"remove denied"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	err := client.DeleteMinute(context.Background(), "token-1", DeleteOptions{})
	if err == nil {
		t.Fatal("DeleteMinute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "server code 9") || !strings.Contains(err.Error(), "remove denied") {
		t.Fatalf("DeleteMinute() error = %q, want server code and message", err.Error())
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestDeleteMinuteReturnsHTTPStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	err := client.DeleteMinute(context.Background(), "token-1", DeleteOptions{})
	if err == nil {
		t.Fatal("DeleteMinute() error = nil, want error")
	}
	for _, want := range []string{"POST", "/minutes/api/space/remove", "500 Internal Server Error"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("DeleteMinute() error = %q, want to contain %q", err.Error(), want)
		}
	}
}

func TestDeleteMinuteStopsWhenTrashDeleteFails(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch len(calls) {
		case 1:
			fmt.Fprint(w, `{"code":0,"msg":"success","data":{}}`)
		case 2:
			fmt.Fprint(w, `{"code":7,"msg":"trash delete denied"}`)
		default:
			t.Fatalf("unexpected request %d %s", len(calls), r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	err := client.DeleteMinute(context.Background(), "token-1", DeleteOptions{Destroy: true})
	if err == nil {
		t.Fatal("DeleteMinute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "trash delete denied") {
		t.Fatalf("DeleteMinute() error = %q, want trash delete denied", err.Error())
	}
	if got := strings.Join(calls, ","); got != "/minutes/api/space/remove,/minutes/api/space/delete" {
		t.Fatalf("calls = %s, want remove then delete only", got)
	}
}

func TestDeleteMinuteReturnsDestroyError(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if len(calls) == 3 {
			fmt.Fprint(w, `{"code":8,"msg":"destroy denied"}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{}}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	err := client.DeleteMinute(context.Background(), "token-1", DeleteOptions{Destroy: true})
	if err == nil {
		t.Fatal("DeleteMinute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "destroy denied") {
		t.Fatalf("DeleteMinute() error = %q, want destroy denied", err.Error())
	}
	if len(calls) != 3 {
		t.Fatalf("requests = %d, want 3", len(calls))
	}
}

func TestDeleteMinuteReturnsRequestCreationError(t *testing.T) {
	client := newTestClient(t, "https://example.test", "https://space.example.test")
	client.baseURL = "http://[::1"

	err := client.DeleteMinute(context.Background(), "token-1", DeleteOptions{})
	if err == nil {
		t.Fatal("DeleteMinute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing ']'") {
		t.Fatalf("DeleteMinute() error = %q, want URL parse error", err.Error())
	}
}

func readFormBody(t *testing.T, r *http.Request) string {
	t.Helper()

	assertMethod(t, r, http.MethodPost)
	if got := r.Header.Get("content-type"); got != "application/x-www-form-urlencoded" {
		t.Fatalf("content-type = %q, want application/x-www-form-urlencoded", got)
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if _, err := url.ParseQuery(string(body)); err != nil {
		t.Fatalf("ParseQuery(%q) error = %v", body, err)
	}

	return string(body)
}
