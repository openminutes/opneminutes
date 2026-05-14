package minutes

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestListMinutesSinglePage(t *testing.T) {
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/minutes/api/space/list" {
			t.Fatalf("path = %q, want /minutes/api/space/list", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{"has_more":false,"list":[{"object_token":"one","topic":"First","share_time":100},{"object_token":"two","topic":"Second","share_time":90}]}}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	minutes, err := client.ListMinutes(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("ListMinutes() error = %v, want nil", err)
	}

	if len(minutes) != 2 {
		t.Fatalf("len(minutes) = %d, want 2", len(minutes))
	}
	if minutes[0].ObjectToken != "one" || minutes[1].ObjectToken != "two" {
		t.Fatalf("minutes = %#v, want server order", minutes)
	}

	wantQuery := map[string]string{
		"size":       "20",
		"space_name": "1",
		"rank":       "1",
		"asc":        "false",
		"note_info":  "true",
		"owner_type": "1",
		"language":   "zh_cn",
	}
	for key, want := range wantQuery {
		if got := gotQuery.Get(key); got != want {
			t.Fatalf("query %s = %q, want %q", key, got, want)
		}
	}
	if got := gotQuery.Get("timestamp"); got != "" {
		t.Fatalf("timestamp = %q, want empty", got)
	}
}

func TestListMinutesMultiplePages(t *testing.T) {
	var timestamps []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timestamps = append(timestamps, r.URL.Query().Get("timestamp"))
		switch r.URL.Query().Get("timestamp") {
		case "":
			fmt.Fprint(w, `{"code":0,"msg":"success","data":{"has_more":true,"list":[{"object_token":"new","share_time":200},{"object_token":"middle","share_time":100}]}}`)
		case "100":
			fmt.Fprint(w, `{"code":0,"msg":"success","data":{"has_more":false,"list":[{"object_token":"old","share_time":50}]}}`)
		default:
			t.Fatalf("unexpected timestamp %q", r.URL.Query().Get("timestamp"))
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	minutes, err := client.ListMinutes(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("ListMinutes() error = %v, want nil", err)
	}

	if got := tokens(minutes); strings.Join(got, ",") != "new,middle,old" {
		t.Fatalf("tokens = %#v, want new,middle,old", got)
	}
	if strings.Join(timestamps, ",") != ",100" {
		t.Fatalf("timestamps = %#v, want empty then 100", timestamps)
	}
}

func TestListMinutesMissingPaginationShareTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{"has_more":true,"list":[{"object_token":"bad"}]}}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	_, err := client.ListMinutes(context.Background(), ListOptions{})
	if err == nil {
		t.Fatal("ListMinutes() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "share_time") {
		t.Fatalf("error = %q, want share_time", err.Error())
	}
}

func TestListMinutesMissingList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{"has_more":false}}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	_, err := client.ListMinutes(context.Background(), ListOptions{})
	if err == nil {
		t.Fatal("ListMinutes() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing list") {
		t.Fatalf("error = %q, want missing list", err.Error())
	}
}

func TestExportSubtitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/minutes/api/export" {
			t.Fatalf("path = %q, want /minutes/api/export", r.URL.Path)
		}
		query := r.URL.Query()
		wantQuery := map[string]string{
			"object_token":  "token-1",
			"format":        "2",
			"add_speaker":   "true",
			"add_timestamp": "true",
			"language":      "zh_cn",
		}
		for key, want := range wantQuery {
			if got := query.Get(key); got != want {
				t.Fatalf("query %s = %q, want %q", key, got, want)
			}
		}
		fmt.Fprint(w, "subtitle text")
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	got, err := client.ExportSubtitle(context.Background(), "token-1", SubtitleOptions{
		Format:       "txt",
		AddSpeaker:   true,
		AddTimestamp: true,
	})
	if err != nil {
		t.Fatalf("ExportSubtitle() error = %v, want nil", err)
	}
	if string(got) != "subtitle text" {
		t.Fatalf("subtitle = %q, want subtitle text", got)
	}
}

func TestExportSubtitleReturnsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json; charset=utf-8")
		fmt.Fprint(w, `{"code":9,"msg":"export denied"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	_, err := client.ExportSubtitle(context.Background(), "token-1", SubtitleOptions{})
	if err == nil {
		t.Fatal("ExportSubtitle() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "export denied") {
		t.Fatalf("error = %q, want export denied", err.Error())
	}
}

func TestGetDownloadURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/minutes/api/status" {
			t.Fatalf("path = %q, want /minutes/api/status", r.URL.Path)
		}
		if got := r.URL.Query().Get("object_token"); got != "token-1" {
			t.Fatalf("object_token = %q, want token-1", got)
		}
		if got := r.URL.Query().Get("language"); got != "zh_cn" {
			t.Fatalf("language = %q, want zh_cn", got)
		}
		if got := r.URL.Query().Get("_t"); got == "" {
			t.Fatal("_t is empty, want timestamp")
		}
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{"object_token":"token-1","video_info":{"video_download_url":"http://download.test/video.mp4"}}}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	got, err := client.GetDownloadURL(context.Background(), "token-1")
	if err != nil {
		t.Fatalf("GetDownloadURL() error = %v, want nil", err)
	}
	if got != "http://download.test/video.mp4" {
		t.Fatalf("download URL = %q, want http://download.test/video.mp4", got)
	}
}

func TestGetDownloadURLMissingField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{"video_info":{}}}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	_, err := client.GetDownloadURL(context.Background(), "token-1")
	if err == nil {
		t.Fatal("GetDownloadURL() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "video_download_url") {
		t.Fatalf("error = %q, want video_download_url", err.Error())
	}
}

func TestDownloadFileStreamsToWriter(t *testing.T) {
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertCommonHeaders(t, r, testCookie, "csrf-token", "http://base.test/minutes/home", "openminutes-test")
		fmt.Fprint(w, "video bytes")
	}))
	t.Cleanup(downloadServer.Close)

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"video_info":{"video_download_url":%q}}}`, downloadServer.URL+"/video.mp4")
	}))
	t.Cleanup(apiServer.Close)

	client := newTestClient(t, apiServer.URL, apiServer.URL)
	client.referer = "http://base.test/minutes/home"

	var dst bytes.Buffer
	if err := client.DownloadFile(context.Background(), "token-1", &dst); err != nil {
		t.Fatalf("DownloadFile() error = %v, want nil", err)
	}

	if dst.String() != "video bytes" {
		t.Fatalf("downloaded bytes = %q, want video bytes", dst.String())
	}
}

func tokens(minutes []Minute) []string {
	result := make([]string, 0, len(minutes))
	for _, minute := range minutes {
		result = append(result, minute.ObjectToken)
	}

	return result
}
