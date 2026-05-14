package minutes

import (
	"bytes"
	"context"
	"os"
	"testing"
)

func TestIntegrationMinutesClient(t *testing.T) {
	cookie := os.Getenv("OPENMINUTES_TEST_COOKIE")
	if cookie == "" {
		t.Skip("set OPENMINUTES_TEST_COOKIE to run integration test")
	}

	client, err := NewClient(Config{
		Cookie:       cookie,
		BaseURL:      os.Getenv("OPENMINUTES_TEST_BASE_URL"),
		SpaceBaseURL: os.Getenv("OPENMINUTES_TEST_SPACE_BASE_URL"),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	ctx := context.Background()
	minutes, err := client.ListMinutes(ctx, ListOptions{Size: 20})
	if err != nil {
		t.Fatalf("ListMinutes() error = %v", err)
	}

	objectToken := os.Getenv("OPENMINUTES_TEST_OBJECT_TOKEN")
	if objectToken == "" && len(minutes) > 0 {
		objectToken = minutes[0].ObjectToken
	}

	if objectToken != "" {
		if _, err := client.GetStatus(ctx, objectToken); err != nil {
			t.Fatalf("GetStatus() error = %v", err)
		}
		if _, err := client.ExportSubtitle(ctx, objectToken, SubtitleOptions{Format: "srt"}); err != nil {
			t.Fatalf("ExportSubtitle() error = %v", err)
		}
	}

	uploadFile := os.Getenv("OPENMINUTES_TEST_UPLOAD_FILE")
	if uploadFile == "" {
		return
	}

	result, err := client.UploadFile(ctx, UploadOptions{FilePath: uploadFile})
	if err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}
	if result.ObjectToken == "" {
		t.Fatal("UploadFile() object token is empty")
	}
}

func TestIntegrationDownloadFile(t *testing.T) {
	cookie := os.Getenv("OPENMINUTES_TEST_COOKIE")
	objectToken := os.Getenv("OPENMINUTES_TEST_OBJECT_TOKEN")
	if cookie == "" || objectToken == "" {
		t.Skip("set OPENMINUTES_TEST_COOKIE and OPENMINUTES_TEST_OBJECT_TOKEN to run download integration test")
	}

	client, err := NewClient(Config{
		Cookie:       cookie,
		BaseURL:      os.Getenv("OPENMINUTES_TEST_BASE_URL"),
		SpaceBaseURL: os.Getenv("OPENMINUTES_TEST_SPACE_BASE_URL"),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	var dst bytes.Buffer
	if err := client.DownloadFile(context.Background(), objectToken, &dst); err != nil {
		t.Fatalf("DownloadFile() error = %v", err)
	}
	if dst.Len() == 0 {
		t.Fatal("DownloadFile() wrote 0 bytes")
	}
}
