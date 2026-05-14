package logic

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

	"openminutes/internal/config"
	"openminutes/internal/minutes"

	"github.com/tcolgate/mp3"
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

func TestValidateUploadFileAllowsUnknownDuration(t *testing.T) {
	filePath := writeUploadFile(t, t.TempDir(), "clip.ogg", []byte("not ogg"))

	if err := ValidateUploadFile(filePath, nil); err != nil {
		t.Fatalf("ValidateUploadFile() error = %v, want nil", err)
	}
}

func TestValidateUploadFileAcceptsKnownDuration(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "short.wav")
	writeWAVHeader(t, filePath, 3*time.Second)

	if err := ValidateUploadFile(filePath, nil); err != nil {
		t.Fatalf("ValidateUploadFile() error = %v, want nil", err)
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
