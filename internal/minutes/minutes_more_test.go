package minutes

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListMinutesPageReturnsRequestCreationError(t *testing.T) {
	client := newTestClient(t, "https://example.test", "https://space.example.test")
	client.baseURL = "http://[::1"

	_, err := client.ListMinutesPage(context.Background(), ListOptions{})
	if err == nil {
		t.Fatal("ListMinutesPage() error = nil, want request creation error")
	}
	if !strings.Contains(err.Error(), "missing ']'") {
		t.Fatalf("ListMinutesPage() error = %q, want URL parse error", err.Error())
	}
}

func TestListMinutesReturnsPaginationNotAdvancedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("timestamp"); got != "100" {
			t.Fatalf("timestamp = %q, want 100", got)
		}
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{"has_more":true,"list":[{"object_token":"token-1","share_time":100}]}}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	_, err := client.ListMinutes(context.Background(), ListOptions{Timestamp: 100})
	if err == nil {
		t.Fatal("ListMinutes() error = nil, want pagination error")
	}
	if !strings.Contains(err.Error(), "did not advance") {
		t.Fatalf("ListMinutes() error = %q, want did not advance", err.Error())
	}
}

func TestExportSubtitleValidationAndRequestErrors(t *testing.T) {
	client := newTestClient(t, "https://example.test", "https://space.example.test")

	if _, err := client.ExportSubtitle(context.Background(), "", SubtitleOptions{}); err == nil {
		t.Fatal("ExportSubtitle(empty token) error = nil, want error")
	}

	client.baseURL = "http://[::1"
	_, err := client.ExportSubtitle(context.Background(), "token-1", SubtitleOptions{})
	if err == nil {
		t.Fatal("ExportSubtitle() error = nil, want request creation error")
	}
	if !strings.Contains(err.Error(), "missing ']'") {
		t.Fatalf("ExportSubtitle() error = %q, want URL parse error", err.Error())
	}
}

func TestGetStatusValidationAndRequestErrors(t *testing.T) {
	client := newTestClient(t, "https://example.test", "https://space.example.test")

	if _, err := client.GetStatus(context.Background(), ""); err == nil {
		t.Fatal("GetStatus(empty token) error = nil, want error")
	}

	client.baseURL = "http://[::1"
	_, err := client.GetStatus(context.Background(), "token-1")
	if err == nil {
		t.Fatal("GetStatus() error = nil, want request creation error")
	}
	if !strings.Contains(err.Error(), "missing ']'") {
		t.Fatalf("GetStatus() error = %q, want URL parse error", err.Error())
	}
}

func TestGetStatusReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":10,"msg":"status denied"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	_, err := client.GetStatus(context.Background(), "token-1")
	if err == nil {
		t.Fatal("GetStatus() error = nil, want API error")
	}
	if !strings.Contains(err.Error(), "status denied") {
		t.Fatalf("GetStatus() error = %q, want status denied", err.Error())
	}
}

func TestGetDownloadURLReturnsStatusError(t *testing.T) {
	client := newTestClient(t, "https://example.test", "https://space.example.test")

	_, err := client.GetDownloadURL(context.Background(), "")
	if err == nil {
		t.Fatal("GetDownloadURL() error = nil, want status error")
	}
	if err.Error() != "object token is required" {
		t.Fatalf("GetDownloadURL() error = %q, want object token error", err.Error())
	}
}

func TestDownloadFileValidationAndRequestErrors(t *testing.T) {
	client := newTestClient(t, "https://example.test", "https://space.example.test")

	if err := client.DownloadFile(context.Background(), "token-1", nil); err == nil {
		t.Fatal("DownloadFile(nil writer) error = nil, want error")
	}
	if err := client.DownloadFile(context.Background(), "", new(bytes.Buffer)); err == nil {
		t.Fatal("DownloadFile(empty token) error = nil, want status error")
	}

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{"video_info":{"video_download_url":"http://[::1"}}}`)
	}))
	t.Cleanup(apiServer.Close)

	client = newTestClient(t, apiServer.URL, apiServer.URL)
	err := client.DownloadFile(context.Background(), "token-1", new(bytes.Buffer))
	if err == nil {
		t.Fatal("DownloadFile() error = nil, want request creation error")
	}
	if !strings.Contains(err.Error(), "missing ']'") {
		t.Fatalf("DownloadFile() error = %q, want URL parse error", err.Error())
	}
}

func TestListQueryCustomOptions(t *testing.T) {
	asc := true
	noteInfo := false
	query := listQuery(ListOptions{
		Size:      7,
		SpaceName: 3,
		Rank:      2,
		Asc:       &asc,
		NoteInfo:  &noteInfo,
		OwnerType: 4,
		Language:  "en_us",
	}, 123)

	want := map[string]string{
		"size":       "7",
		"space_name": "3",
		"rank":       "2",
		"asc":        "true",
		"note_info":  "false",
		"owner_type": "4",
		"language":   "en_us",
		"timestamp":  "123",
	}
	for key, value := range want {
		if got := query.Get(key); got != value {
			t.Fatalf("query %s = %q, want %q", key, got, value)
		}
	}
}

func TestSubtitleFormatDefaultsToSRT(t *testing.T) {
	for _, format := range []string{"", "srt", "vtt"} {
		t.Run(format, func(t *testing.T) {
			if got := subtitleFormat(format); got != 3 {
				t.Fatalf("subtitleFormat(%q) = %d, want 3", format, got)
			}
		})
	}
}
