package cmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	apperrors "openminutes/internal/errors"
	"openminutes/internal/minutes"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type uploadMinutesClientFunc func(context.Context, minutes.UploadOptions) (*minutes.UploadResult, error)

func (f uploadMinutesClientFunc) UploadFile(ctx context.Context, options minutes.UploadOptions) (*minutes.UploadResult, error) {
	return f(ctx, options)
}

func withUploadMinutesClient(t *testing.T, factory func(minutes.Config) (uploadMinutesClient, error)) {
	t.Helper()

	oldFactory := newUploadMinutesClient
	newUploadMinutesClient = factory
	t.Cleanup(func() {
		newUploadMinutesClient = oldFactory
	})
}

func executeUploadCommand(t *testing.T, config Config, args ...string) (string, error) {
	t.Helper()

	cmd := newUploadCommand()
	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetArgs(args)
	cmd.SetContext(contextWithConfig(context.Background(), config))

	err := cmd.Execute()
	return stdout.String(), err
}

func TestUploadCommandReadsConfigAndCallsUploadAPI(t *testing.T) {
	wantConfig := minutes.Config{
		BaseURL:      "https://meetings.example.test",
		SpaceBaseURL: "https://space.example.test",
		Cookie:       "session=abc",
	}
	filePath := writeUploadFile(t, t.TempDir(), "clip.aac", []byte("audio"))
	ctxKey := struct{}{}
	ctx := context.WithValue(context.Background(), ctxKey, "marker")
	logger := zap.NewNop()
	var gotConfig minutes.Config
	var gotOptions minutes.UploadOptions
	var gotMarker any
	calls := 0

	withUploadMinutesClient(t, func(config minutes.Config) (uploadMinutesClient, error) {
		gotConfig = config
		return uploadMinutesClientFunc(func(ctx context.Context, options minutes.UploadOptions) (*minutes.UploadResult, error) {
			calls++
			gotOptions = options
			gotMarker = ctx.Value(ctxKey)
			return &minutes.UploadResult{ObjectToken: "object-1"}, nil
		}), nil
	})

	cmd := newUploadCommand()
	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetArgs([]string{filePath})
	cmd.SetContext(contextWithLogger(contextWithConfig(ctx, Config{
		BaseURL:      wantConfig.BaseURL,
		SpaceBaseURL: wantConfig.SpaceBaseURL,
		Cookie:       wantConfig.Cookie,
	}), logger))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if gotConfig.Logger != logger {
		t.Fatalf("client logger = %#v, want context logger", gotConfig.Logger)
	}
	gotConfig.Logger = nil
	if gotConfig != wantConfig {
		t.Fatalf("client config = %#v, want %#v", gotConfig, wantConfig)
	}
	if calls != 1 {
		t.Fatalf("UploadFile() calls = %d, want 1", calls)
	}
	if gotMarker != "marker" {
		t.Fatalf("UploadFile() context marker = %#v, want marker", gotMarker)
	}
	if !reflect.DeepEqual(gotOptions, minutes.UploadOptions{FilePath: filePath}) {
		t.Fatalf("UploadFile() options = %#v, want file path", gotOptions)
	}
	wantStdout := "Uploaded object-1 https://meetings.example.test/minutes/object-1\n"
	if stdout.String() != wantStdout {
		t.Fatalf("stdout = %q, want %q", stdout.String(), wantStdout)
	}
}

func TestUploadCommandAcceptsRelativeAndAbsolutePaths(t *testing.T) {
	tempDir := t.TempDir()
	absolutePath := writeUploadFile(t, tempDir, "absolute.aac", []byte("audio"))

	t.Run("absolute", func(t *testing.T) {
		var gotPath string
		withUploadMinutesClient(t, func(minutes.Config) (uploadMinutesClient, error) {
			return uploadMinutesClientFunc(func(ctx context.Context, options minutes.UploadOptions) (*minutes.UploadResult, error) {
				gotPath = options.FilePath
				return &minutes.UploadResult{ObjectToken: "object-1"}, nil
			}), nil
		})

		if _, err := executeUploadCommand(t, testCommandConfig(), absolutePath); err != nil {
			t.Fatalf("Execute() error = %v, want nil", err)
		}
		if gotPath != absolutePath {
			t.Fatalf("UploadFile() path = %q, want %q", gotPath, absolutePath)
		}
	})

	t.Run("relative", func(t *testing.T) {
		t.Chdir(tempDir)
		relativePath := "relative.aac"
		writeUploadFile(t, tempDir, relativePath, []byte("audio"))
		var gotPath string
		withUploadMinutesClient(t, func(minutes.Config) (uploadMinutesClient, error) {
			return uploadMinutesClientFunc(func(ctx context.Context, options minutes.UploadOptions) (*minutes.UploadResult, error) {
				gotPath = options.FilePath
				return &minutes.UploadResult{ObjectToken: "object-1"}, nil
			}), nil
		})

		if _, err := executeUploadCommand(t, testCommandConfig(), relativePath); err != nil {
			t.Fatalf("Execute() error = %v, want nil", err)
		}
		if gotPath != relativePath {
			t.Fatalf("UploadFile() path = %q, want %q", gotPath, relativePath)
		}
	})
}

func TestUploadCommandReturnsMissingConfigError(t *testing.T) {
	filePath := writeUploadFile(t, t.TempDir(), "clip.aac", []byte("audio"))

	cmd := newUploadCommand()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{filePath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if err.Error() != "config is required" {
		t.Fatalf("Execute() error = %q, want config is required", err.Error())
	}
	if !apperrors.IsKind(err, apperrors.KindConfig) {
		t.Fatalf("Execute() error kind = %q, want config", apperrors.KindOf(err))
	}
}

func TestUploadCommandReturnsClientError(t *testing.T) {
	wantErr := errors.New("client failed")
	filePath := writeUploadFile(t, t.TempDir(), "clip.aac", []byte("audio"))
	withUploadMinutesClient(t, func(minutes.Config) (uploadMinutesClient, error) {
		return nil, wantErr
	})

	_, err := executeUploadCommand(t, testCommandConfig(), filePath)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestUploadCommandReturnsUploadError(t *testing.T) {
	wantErr := errors.New("upload failed")
	filePath := writeUploadFile(t, t.TempDir(), "clip.aac", []byte("audio"))
	withUploadMinutesClient(t, func(minutes.Config) (uploadMinutesClient, error) {
		return uploadMinutesClientFunc(func(ctx context.Context, options minutes.UploadOptions) (*minutes.UploadResult, error) {
			return nil, wantErr
		}), nil
	})

	_, err := executeUploadCommand(t, testCommandConfig(), filePath)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestUploadCommandRejectsInvalidArgsBeforeClientCreation(t *testing.T) {
	filePath := writeUploadFile(t, t.TempDir(), "clip.aac", []byte("audio"))

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing path", args: []string{}, wantErr: "file path is required"},
		{name: "extra args", args: []string{filePath, filePath}, wantErr: "upload accepts exactly one file path"},
		{name: "blank path", args: []string{" "}, wantErr: "file path is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientCreated := false
			withUploadMinutesClient(t, func(minutes.Config) (uploadMinutesClient, error) {
				clientCreated = true
				return nil, errors.New("client should not be created")
			})

			_, err := executeUploadCommand(t, testCommandConfig(), tt.args...)
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("Execute() error = %q, want %q", err.Error(), tt.wantErr)
			}
			if clientCreated {
				t.Fatal("client created for invalid args")
			}
		})
	}
}

func TestUploadCommandRejectsInvalidFilesBeforeUploadAPI(t *testing.T) {
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
	if err := oversizedFile.Truncate(6*1024*1024*1024 + 1); err != nil {
		_ = oversizedFile.Close()
		t.Fatalf("Truncate() error = %v", err)
	}
	if err := oversizedFile.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	tooLongPath := filepath.Join(tempDir, "too-long.wav")
	writeWAVHeader(t, tooLongPath, 6*time.Hour+time.Second)

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "missing file", path: missingPath, wantErr: "does not exist"},
		{name: "directory", path: directoryPath, wantErr: "is a directory"},
		{name: "unsupported extension", path: unsupportedPath, wantErr: `unsupported file extension ".txt"`},
		{name: "oversized file", path: oversizedPath, wantErr: "exceeds maximum"},
		{name: "too long known duration", path: tooLongPath, wantErr: "file duration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uploadCalled := false
			withUploadMinutesClient(t, func(minutes.Config) (uploadMinutesClient, error) {
				return uploadMinutesClientFunc(func(context.Context, minutes.UploadOptions) (*minutes.UploadResult, error) {
					uploadCalled = true
					return nil, errors.New("upload should not be called")
				}), nil
			})

			_, err := executeUploadCommand(t, testCommandConfig(), tt.path)
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Execute() error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
			if uploadCalled {
				t.Fatal("upload API called for invalid file")
			}
		})
	}
}

func TestUploadCommandAllowsUnknownDuration(t *testing.T) {
	filePath := writeUploadFile(t, t.TempDir(), "clip.ogg", []byte("not ogg"))
	withUploadMinutesClient(t, func(minutes.Config) (uploadMinutesClient, error) {
		return uploadMinutesClientFunc(func(ctx context.Context, options minutes.UploadOptions) (*minutes.UploadResult, error) {
			return &minutes.UploadResult{ObjectToken: "object-1"}, nil
		}), nil
	})

	stdout, err := executeUploadCommand(t, testCommandConfig(), filePath)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	wantStdout := "Uploaded object-1 https://meetings.feishu.cn/minutes/object-1\n"
	if stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", stdout, wantStdout)
	}
}

func TestUploadCommandReturnsEmptyResultError(t *testing.T) {
	filePath := writeUploadFile(t, t.TempDir(), "clip.aac", []byte("audio"))
	withUploadMinutesClient(t, func(minutes.Config) (uploadMinutesClient, error) {
		return uploadMinutesClientFunc(func(ctx context.Context, options minutes.UploadOptions) (*minutes.UploadResult, error) {
			return nil, nil
		}), nil
	})

	_, err := executeUploadCommand(t, testCommandConfig(), filePath)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if err.Error() != "upload result is empty" {
		t.Fatalf("Execute() error = %q, want upload result is empty", err.Error())
	}
}

func TestUploadCommandReturnsStdoutWriteError(t *testing.T) {
	wantErr := errors.New("write failed")
	filePath := writeUploadFile(t, t.TempDir(), "clip.aac", []byte("audio"))
	withUploadMinutesClient(t, func(minutes.Config) (uploadMinutesClient, error) {
		return uploadMinutesClientFunc(func(ctx context.Context, options minutes.UploadOptions) (*minutes.UploadResult, error) {
			return &minutes.UploadResult{ObjectToken: "object-1"}, nil
		}), nil
	})

	cmd := newUploadCommand()
	cmd.SetOut(&failingWriter{failAt: 1, err: wantErr})
	cmd.SetArgs([]string{filePath})
	cmd.SetContext(contextWithConfig(context.Background(), testCommandConfig()))

	if err := cmd.Execute(); !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestRunUploadCommandRejectsUnvalidatedArgs(t *testing.T) {
	cmd := &cobra.Command{Use: "upload"}
	cmd.SetContext(contextWithConfig(context.Background(), testCommandConfig()))

	err := runUploadCommand(cmd, nil)
	if err == nil {
		t.Fatal("runUploadCommand() error = nil, want error")
	}
	if err.Error() != "file path is required" {
		t.Fatalf("runUploadCommand() error = %q, want file path is required", err.Error())
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

func writeWAVHeader(t *testing.T, filePath string, duration time.Duration) {
	t.Helper()

	writeCustomWAVHeader(t, filePath, 1, 16, duration)
}

func writeCustomWAVHeader(t *testing.T, filePath string, numChannels uint16, bitsPerSample uint16, duration time.Duration) {
	t.Helper()

	const (
		sampleRate  = uint32(8000)
		audioFormat = uint16(1)
	)
	blockAlign := numChannels * bitsPerSample / 8
	avgBytesPerSecond := sampleRate * uint32(blockAlign)
	var dataSize uint32
	if avgBytesPerSecond > 0 {
		dataSize = uint32((duration * time.Duration(avgBytesPerSecond)) / time.Second)
	}
	riffSize := uint32(4 + 8 + 16 + 8 + dataSize)

	buffer := new(bytes.Buffer)
	buffer.WriteString("RIFF")
	writeLittleEndian(t, buffer, riffSize)
	buffer.WriteString("WAVE")
	buffer.WriteString("fmt ")
	writeLittleEndian(t, buffer, uint32(16))
	writeLittleEndian(t, buffer, audioFormat)
	writeLittleEndian(t, buffer, numChannels)
	writeLittleEndian(t, buffer, sampleRate)
	writeLittleEndian(t, buffer, avgBytesPerSecond)
	writeLittleEndian(t, buffer, blockAlign)
	writeLittleEndian(t, buffer, bitsPerSample)
	buffer.WriteString("data")
	writeLittleEndian(t, buffer, dataSize)

	if err := os.WriteFile(filePath, buffer.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func writeLittleEndian(t *testing.T, buffer *bytes.Buffer, value any) {
	t.Helper()

	if err := binary.Write(buffer, binary.LittleEndian, value); err != nil {
		t.Fatalf("binary.Write() error = %v", err)
	}
}
