package logic

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"openminutes/internal/config"
	apperrors "openminutes/internal/errors"
	"openminutes/internal/media"
	"openminutes/internal/minutes"

	"go.uber.org/zap"
)

type listClientStub struct {
	listPage func(context.Context, minutes.ListOptions) (*minutes.ListMinutesPageResult, error)
	listAll  func(context.Context, minutes.ListOptions) ([]minutes.Minute, error)
}

func (s listClientStub) ListMinutesPage(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
	if s.listPage == nil {
		return nil, errors.New("ListMinutesPage() should not be called")
	}

	return s.listPage(ctx, options)
}

func (s listClientStub) ListMinutes(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
	if s.listAll == nil {
		return nil, errors.New("ListMinutes() should not be called")
	}

	return s.listAll(ctx, options)
}

func TestListMinutesBranches(t *testing.T) {
	options := minutes.ListOptions{Size: 50, Timestamp: 100}

	t.Run("single page", func(t *testing.T) {
		want := &minutes.ListMinutesPageResult{
			Items:         []minutes.Minute{{ObjectToken: "token-1"}},
			HasMore:       true,
			NextTimestamp: 200,
		}
		client := listClientStub{
			listPage: func(ctx context.Context, got minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
				if !reflect.DeepEqual(got, options) {
					t.Fatalf("options = %#v, want %#v", got, options)
				}
				return want, nil
			},
		}

		got, err := ListMinutes(context.Background(), client, options, false)
		if err != nil {
			t.Fatalf("ListMinutes() error = %v, want nil", err)
		}
		if got != want {
			t.Fatalf("ListMinutes() = %#v, want same page result", got)
		}
	})

	t.Run("all pages", func(t *testing.T) {
		items := []minutes.Minute{{ObjectToken: "token-1"}, {ObjectToken: "token-2"}}
		client := listClientStub{
			listAll: func(ctx context.Context, got minutes.ListOptions) ([]minutes.Minute, error) {
				if !reflect.DeepEqual(got, options) {
					t.Fatalf("options = %#v, want %#v", got, options)
				}
				return items, nil
			},
		}

		got, err := ListMinutes(context.Background(), client, options, true)
		if err != nil {
			t.Fatalf("ListMinutes() error = %v, want nil", err)
		}
		want := &minutes.ListMinutesPageResult{Items: items}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ListMinutes() = %#v, want %#v", got, want)
		}
	})

	t.Run("all error", func(t *testing.T) {
		wantErr := errors.New("list failed")
		client := listClientStub{
			listAll: func(context.Context, minutes.ListOptions) ([]minutes.Minute, error) {
				return nil, wantErr
			},
		}

		if _, err := ListMinutes(context.Background(), client, options, true); !errors.Is(err, wantErr) {
			t.Fatalf("ListMinutes() error = %v, want %v", err, wantErr)
		}
	})
}

type getClientFunc func(context.Context, string, minutes.SubtitleOptions) ([]byte, error)

func (f getClientFunc) ExportSubtitle(ctx context.Context, token string, options minutes.SubtitleOptions) ([]byte, error) {
	return f(ctx, token, options)
}

func TestExportSubtitle(t *testing.T) {
	options := minutes.SubtitleOptions{Format: "srt", AddSpeaker: true}
	want := []byte("subtitle")
	ctxKey := struct{}{}
	ctx := context.WithValue(context.Background(), ctxKey, "marker")

	client := getClientFunc(func(gotCtx context.Context, token string, gotOptions minutes.SubtitleOptions) ([]byte, error) {
		if gotCtx.Value(ctxKey) != "marker" {
			t.Fatalf("context marker = %#v, want marker", gotCtx.Value(ctxKey))
		}
		if token != "token-1" {
			t.Fatalf("token = %q, want token-1", token)
		}
		if !reflect.DeepEqual(gotOptions, options) {
			t.Fatalf("options = %#v, want %#v", gotOptions, options)
		}
		return want, nil
	})

	got, err := ExportSubtitle(ctx, client, "token-1", options)
	if err != nil {
		t.Fatalf("ExportSubtitle() error = %v, want nil", err)
	}
	if string(got) != string(want) {
		t.Fatalf("ExportSubtitle() = %q, want %q", got, want)
	}
}

type deleteClientFunc func(context.Context, string, minutes.DeleteOptions) error

func (f deleteClientFunc) DeleteMinute(ctx context.Context, token string, options minutes.DeleteOptions) error {
	return f(ctx, token, options)
}

func TestDeleteMinutesDeletesInOrderAndStopsOnFirstError(t *testing.T) {
	wantErr := errors.New("delete failed")
	var tokens []string
	options := minutes.DeleteOptions{Destroy: true}

	client := deleteClientFunc(func(ctx context.Context, token string, gotOptions minutes.DeleteOptions) error {
		tokens = append(tokens, token)
		if !reflect.DeepEqual(gotOptions, options) {
			t.Fatalf("options = %#v, want %#v", gotOptions, options)
		}
		if token == "token-2" {
			return wantErr
		}
		return nil
	})

	err := DeleteMinutes(context.Background(), client, []string{" token-1 ", "token-2", "token-3"}, options)
	if !errors.Is(err, wantErr) {
		t.Fatalf("DeleteMinutes() error = %v, want %v", err, wantErr)
	}
	if got := strings.Join(tokens, ","); got != "token-1,token-2" {
		t.Fatalf("tokens = %s, want token-1,token-2", got)
	}
}

type uploadClientFunc func(context.Context, minutes.UploadOptions) (*minutes.UploadResult, error)

func (f uploadClientFunc) UploadFile(ctx context.Context, options minutes.UploadOptions) (*minutes.UploadResult, error) {
	return f(ctx, options)
}

func TestUploadFileValidatesAndUploads(t *testing.T) {
	filePath := writeUploadFile(t, t.TempDir(), "clip.aac", []byte("audio"))
	ctxKey := struct{}{}
	ctx := context.WithValue(context.Background(), ctxKey, "marker")
	var gotOptions minutes.UploadOptions
	var gotMarker any

	client := uploadClientFunc(func(ctx context.Context, options minutes.UploadOptions) (*minutes.UploadResult, error) {
		gotMarker = ctx.Value(ctxKey)
		gotOptions = options
		return &minutes.UploadResult{ObjectToken: "object-1"}, nil
	})

	result, err := UploadFile(ctx, client, filePath, nil)
	if err != nil {
		t.Fatalf("UploadFile() error = %v, want nil", err)
	}
	if result.ObjectToken != "object-1" {
		t.Fatalf("ObjectToken = %q, want object-1", result.ObjectToken)
	}
	if gotMarker != "marker" {
		t.Fatalf("context marker = %#v, want marker", gotMarker)
	}
	if !reflect.DeepEqual(gotOptions, minutes.UploadOptions{FilePath: filePath}) {
		t.Fatalf("options = %#v, want file path", gotOptions)
	}
}

func TestUploadFileReturnsUploadErrorAndRejectsNilResult(t *testing.T) {
	filePath := writeUploadFile(t, t.TempDir(), "clip.aac", []byte("audio"))

	t.Run("upload error", func(t *testing.T) {
		wantErr := errors.New("upload failed")
		client := uploadClientFunc(func(context.Context, minutes.UploadOptions) (*minutes.UploadResult, error) {
			return nil, wantErr
		})

		if _, err := UploadFile(context.Background(), client, filePath, nil); !errors.Is(err, wantErr) {
			t.Fatalf("UploadFile() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("nil result", func(t *testing.T) {
		client := uploadClientFunc(func(context.Context, minutes.UploadOptions) (*minutes.UploadResult, error) {
			return nil, nil
		})

		_, err := UploadFile(context.Background(), client, filePath, nil)
		if err == nil {
			t.Fatal("UploadFile() error = nil, want error")
		}
		if err.Error() != "upload result is empty" {
			t.Fatalf("UploadFile() error = %q, want upload result is empty", err.Error())
		}
		if !apperrors.IsKind(err, apperrors.KindRemote) {
			t.Fatalf("UploadFile() error kind = %q, want remote", apperrors.KindOf(err))
		}
	})
}

func TestUploadFileRejectsInvalidFilesBeforeClientCall(t *testing.T) {
	tempDir := t.TempDir()
	missingPath := filepath.Join(tempDir, "missing.mp3")
	directoryPath := filepath.Join(tempDir, "directory")
	if err := os.Mkdir(directoryPath, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	unsupportedPath := writeUploadFile(t, tempDir, "clip.txt", []byte("text"))
	oversizedPath := filepath.Join(tempDir, "large.mp4")
	oversizedFile, err := os.Create(oversizedPath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := oversizedFile.Truncate(media.MaxUploadFileSize + 1); err != nil {
		_ = oversizedFile.Close()
		t.Fatalf("Truncate() error = %v", err)
	}
	if err := oversizedFile.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "missing file", path: missingPath, wantErr: "does not exist"},
		{name: "directory", path: directoryPath, wantErr: "is a directory"},
		{name: "unsupported extension", path: unsupportedPath, wantErr: `unsupported file extension ".txt"`},
		{name: "oversized file", path: oversizedPath, wantErr: "exceeds maximum"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientCalled := false
			client := uploadClientFunc(func(context.Context, minutes.UploadOptions) (*minutes.UploadResult, error) {
				clientCalled = true
				return nil, errors.New("client should not be called")
			})

			_, err := UploadFile(context.Background(), client, tt.path, zap.NewNop())
			if err == nil {
				t.Fatal("UploadFile() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("UploadFile() error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
			if clientCalled {
				t.Fatal("client called for invalid file")
			}
		})
	}
}

func TestValidateUploadFileReturnsStatError(t *testing.T) {
	err := ValidateUploadFile("bad\x00path.mp3", zap.NewNop())
	if err == nil {
		t.Fatal("ValidateUploadFile() error = nil, want stat error")
	}
	if !strings.Contains(err.Error(), "stat file") {
		t.Fatalf("ValidateUploadFile() error = %q, want stat file", err.Error())
	}
}

func TestMinuteURL(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		objectToken string
		want        string
	}{
		{
			name:        "custom trailing slash",
			baseURL:     "https://meetings.example.test/",
			objectToken: "object-1",
			want:        "https://meetings.example.test/minutes/object-1",
		},
		{
			name:        "default base url",
			baseURL:     "",
			objectToken: "object-1",
			want:        "https://meetings.feishu.cn/minutes/object-1",
		},
		{
			name:        "invalid base url falls back to default",
			baseURL:     "://bad",
			objectToken: "object-1",
			want:        "https://meetings.feishu.cn/minutes/object-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MinuteURL(tt.baseURL, tt.objectToken); got != tt.want {
				t.Fatalf("MinuteURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMinuteTopic(t *testing.T) {
	tests := []struct {
		name  string
		topic string
		want  string
	}{
		{name: "title", topic: " Meeting ", want: "Meeting"},
		{name: "empty", topic: " ", want: "(untitled)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MinuteTopic(tt.topic); got != tt.want {
				t.Fatalf("MinuteTopic() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMinuteURLUsesConfigDefault(t *testing.T) {
	if got := MinuteURL("", "object-1"); got != config.DefaultBaseURL+"/minutes/object-1" {
		t.Fatalf("MinuteURL() = %q, want config default", got)
	}
}

func writeUploadFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()

	filePath := filepath.Join(dir, name)
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return filePath
}
