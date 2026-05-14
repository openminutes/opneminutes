package cmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"openminutes/internal/minutes"

	"github.com/spf13/cobra"
	"github.com/tcolgate/mp3"
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

func TestUploadCommandRejectsInvalidFilesBeforeClientCreation(t *testing.T) {
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
	if err := oversizedFile.Truncate(maxUploadFileSize + 1); err != nil {
		_ = oversizedFile.Close()
		t.Fatalf("Truncate() error = %v", err)
	}
	if err := oversizedFile.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	tooLongPath := filepath.Join(tempDir, "too-long.wav")
	writeWAVHeader(t, tooLongPath, maxUploadDuration+time.Second)

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
			clientCreated := false
			withUploadMinutesClient(t, func(minutes.Config) (uploadMinutesClient, error) {
				clientCreated = true
				return nil, errors.New("client should not be created")
			})

			_, err := executeUploadCommand(t, testCommandConfig(), tt.path)
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Execute() error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
			if clientCreated {
				t.Fatal("client created for invalid file")
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

func TestValidateUploadFileAcceptsKnownDuration(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "short.wav")
	writeWAVHeader(t, filePath, 3*time.Second)

	if err := validateUploadFile(filePath, nil); err != nil {
		t.Fatalf("validateUploadFile() error = %v, want nil", err)
	}
}

func TestValidateUploadFileReturnsStatError(t *testing.T) {
	err := validateUploadFile("bad\x00path.mp3", zap.NewNop())
	if err == nil {
		t.Fatal("validateUploadFile() error = nil, want stat error")
	}
	if !strings.Contains(err.Error(), "stat file") {
		t.Fatalf("validateUploadFile() error = %q, want stat file", err.Error())
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

func TestProbeUploadDuration(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("skipped extension", func(t *testing.T) {
		duration, known, err := probeUploadDuration(filepath.Join(tempDir, "missing.aac"), ".aac")
		if err != nil {
			t.Fatalf("probeUploadDuration() error = %v, want nil", err)
		}
		if known || duration != 0 {
			t.Fatalf("probeUploadDuration() = %v, %v, want unknown zero", duration, known)
		}
	})

	t.Run("missing probed file", func(t *testing.T) {
		_, known, err := probeUploadDuration(filepath.Join(tempDir, "missing.wav"), ".wav")
		if err == nil {
			t.Fatal("probeUploadDuration() error = nil, want error")
		}
		if known {
			t.Fatal("probeUploadDuration() known = true, want false")
		}
	})

	t.Run("wav", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "known.wav")
		writeWAVHeader(t, filePath, 3*time.Second)
		duration, known, err := probeUploadDuration(filePath, ".wav")
		if err != nil {
			t.Fatalf("probeUploadDuration() error = %v, want nil", err)
		}
		if !known || duration != 3*time.Second {
			t.Fatalf("probeUploadDuration() = %v, %v, want 3s known", duration, known)
		}
	})

	t.Run("mp4", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "known.mp4")
		writeMP4WithMovieDuration(t, filePath, 1000, 2500)
		duration, known, err := probeUploadDuration(filePath, ".mp4")
		if err != nil {
			t.Fatalf("probeUploadDuration() error = %v, want nil", err)
		}
		if !known || duration != 2500*time.Millisecond {
			t.Fatalf("probeUploadDuration() = %v, %v, want 2.5s known", duration, known)
		}
	})

	t.Run("mp4 decode error", func(t *testing.T) {
		filePath := writeUploadFile(t, tempDir, "broken.mp4", []byte("not mp4"))
		_, known, err := probeUploadDuration(filePath, ".mp4")
		if err == nil {
			t.Fatal("probeUploadDuration() error = nil, want error")
		}
		if known {
			t.Fatal("probeUploadDuration() known = true, want false")
		}
	})

	t.Run("mp4 unknown duration", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "unknown.mp4")
		if err := os.WriteFile(filePath, mp4Box(t, "ftyp", []byte("isom\x00\x00\x00\x00isom")), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		_, known, err := probeUploadDuration(filePath, ".mp4")
		if !errors.Is(err, errUploadDurationUnknown) {
			t.Fatalf("probeUploadDuration() error = %v, want unknown", err)
		}
		if known {
			t.Fatal("probeUploadDuration() known = true, want false")
		}
	})

	t.Run("mp3", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "known.mp3")
		writeMP3SilentFrame(t, filePath)
		duration, known, err := probeUploadDuration(filePath, ".mp3")
		if err != nil {
			t.Fatalf("probeUploadDuration() error = %v, want nil", err)
		}
		if !known || duration <= 0 {
			t.Fatalf("probeUploadDuration() = %v, %v, want known positive duration", duration, known)
		}
	})

	t.Run("mp3 decode error", func(t *testing.T) {
		filePath := writeUploadFile(t, tempDir, "broken.mp3", []byte("not mp3"))
		_, known, err := probeUploadDuration(filePath, ".mp3")
		if err == nil {
			t.Fatal("probeUploadDuration() error = nil, want error")
		}
		if known {
			t.Fatal("probeUploadDuration() known = true, want false")
		}
	})
}

func TestProbeUploadDurationFileHandlesProbeResults(t *testing.T) {
	filePath := writeUploadFile(t, t.TempDir(), "clip.aac", []byte("audio"))
	wantErr := errors.New("probe failed")

	t.Run("probe error", func(t *testing.T) {
		_, known, err := probeUploadDurationFile(filePath, func(*os.File) (time.Duration, error) {
			return 0, wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("probeUploadDurationFile() error = %v, want %v", err, wantErr)
		}
		if known {
			t.Fatal("probeUploadDurationFile() known = true, want false")
		}
	})

	t.Run("zero duration", func(t *testing.T) {
		_, known, err := probeUploadDurationFile(filePath, func(*os.File) (time.Duration, error) {
			return 0, nil
		})
		if !errors.Is(err, errUploadDurationUnknown) {
			t.Fatalf("probeUploadDurationFile() error = %v, want unknown", err)
		}
		if known {
			t.Fatal("probeUploadDurationFile() known = true, want false")
		}
	})
}

func TestDurationProbeSeekErrors(t *testing.T) {
	tests := []struct {
		name  string
		probe func(*os.File) (time.Duration, error)
	}{
		{name: "mp4", probe: probeMP4Duration},
		{name: "wav", probe: probeWAVDuration},
		{name: "mp3", probe: probeMP3Duration},
		{name: "ogg", probe: probeOggVorbisDuration},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := writeUploadFile(t, t.TempDir(), "closed.bin", []byte("data"))
			file, err := os.Open(filePath)
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if err := file.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			if _, err := tt.probe(file); err == nil {
				t.Fatal("probe() error = nil, want closed file error")
			}
		})
	}
}

func TestWAVDurationProbeRejectsInvalidHeader(t *testing.T) {
	filePath := writeUploadFile(t, t.TempDir(), "broken.wav", []byte("not wav"))
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()

	if _, err := probeWAVDuration(file); err == nil {
		t.Fatal("probeWAVDuration() error = nil, want error")
	}
}

func TestWAVDurationProbeReturnsUnknownForInvalidMetadata(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "invalid.wav")
	writeCustomWAVHeader(t, filePath, 1, 16, 0)
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()

	if _, err := probeWAVDuration(file); !errors.Is(err, errUploadDurationUnknown) {
		t.Fatalf("probeWAVDuration() error = %v, want unknown", err)
	}
}

func TestDurationFromTimeUnits(t *testing.T) {
	maxWholeSeconds := uint64(maxTimeDuration / time.Second)
	tests := []struct {
		name           string
		units          uint64
		unitsPerSecond uint64
		want           time.Duration
	}{
		{name: "zero units", units: 0, unitsPerSecond: 1000, want: 0},
		{name: "zero scale", units: 1000, unitsPerSecond: 0, want: 0},
		{name: "fractional", units: 1500, unitsPerSecond: 1000, want: 1500 * time.Millisecond},
		{name: "clamped", units: uint64(maxTimeDuration/time.Second) + 1, unitsPerSecond: 1, want: maxTimeDuration},
		{name: "clamped nanoseconds", units: maxWholeSeconds*10 + 9, unitsPerSecond: 10, want: maxTimeDuration},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := durationFromTimeUnits(tt.units, tt.unitsPerSecond); got != tt.want {
				t.Fatalf("durationFromTimeUnits() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUploadMinutesURL(t *testing.T) {
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
			if got := uploadMinutesURL(tt.baseURL, tt.objectToken); got != tt.want {
				t.Fatalf("uploadMinutesURL() = %q, want %q", got, tt.want)
			}
		})
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

func writeMP4WithMovieDuration(t *testing.T, filePath string, timescale uint32, duration uint32) {
	t.Helper()

	mvhdPayload := new(bytes.Buffer)
	writeBinary(t, mvhdPayload, uint32(0))
	writeBinary(t, mvhdPayload, uint32(0))
	writeBinary(t, mvhdPayload, uint32(0))
	writeBinary(t, mvhdPayload, timescale)
	writeBinary(t, mvhdPayload, duration)
	writeBinary(t, mvhdPayload, uint32(0x00010000))
	writeBinary(t, mvhdPayload, uint16(0x0100))
	writeBinary(t, mvhdPayload, uint16(0))
	writeBinary(t, mvhdPayload, uint32(0))
	writeBinary(t, mvhdPayload, uint32(0))
	for _, value := range []uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000} {
		writeBinary(t, mvhdPayload, value)
	}
	for i := 0; i < 6; i++ {
		writeBinary(t, mvhdPayload, uint32(0))
	}
	writeBinary(t, mvhdPayload, uint32(1))

	content := append(mp4Box(t, "ftyp", []byte("isom\x00\x00\x00\x00isom")), mp4Box(t, "moov", mp4Box(t, "mvhd", mvhdPayload.Bytes()))...)
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func mp4Box(t *testing.T, boxType string, payload []byte) []byte {
	t.Helper()

	buffer := new(bytes.Buffer)
	writeBinary(t, buffer, uint32(8+len(payload)))
	buffer.WriteString(boxType)
	buffer.Write(payload)
	return buffer.Bytes()
}

func writeMP3SilentFrame(t *testing.T, filePath string) {
	t.Helper()

	data, err := io.ReadAll(mp3.SilentFrame.Reader())
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func writeBinary(t *testing.T, buffer *bytes.Buffer, value any) {
	t.Helper()

	if err := binary.Write(buffer, binary.BigEndian, value); err != nil {
		t.Fatalf("binary.Write() error = %v", err)
	}
}

func writeLittleEndian(t *testing.T, buffer *bytes.Buffer, value any) {
	t.Helper()

	if err := binary.Write(buffer, binary.LittleEndian, value); err != nil {
		t.Fatalf("binary.Write() error = %v", err)
	}
}
