package media

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/adler32"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	apperrors "openminutes/internal/errors"

	"github.com/tcolgate/mp3"
	"go.uber.org/zap"
)

func TestValidateUploadFile(t *testing.T) {
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
	if err := oversizedFile.Truncate(MaxUploadFileSize + 1); err != nil {
		_ = oversizedFile.Close()
		t.Fatalf("Truncate() error = %v", err)
	}
	if err := oversizedFile.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	tooLongPath := filepath.Join(tempDir, "too-long.wav")
	writeWAVHeader(t, tooLongPath, MaxUploadDuration+time.Second)

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "missing file", path: missingPath, wantErr: "does not exist"},
		{name: "stat error", path: "bad\x00path.mp3", wantErr: "stat file"},
		{name: "directory", path: directoryPath, wantErr: "is a directory"},
		{name: "unsupported extension", path: unsupportedPath, wantErr: `unsupported file extension ".txt"`},
		{name: "oversized file", path: oversizedPath, wantErr: "exceeds maximum"},
		{name: "too long duration", path: tooLongPath, wantErr: "file duration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUploadFile(tt.path, zap.NewNop())
			if err == nil {
				t.Fatal("ValidateUploadFile() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateUploadFile() error = %q, want %q", err.Error(), tt.wantErr)
			}
			if !apperrors.IsKind(err, apperrors.KindValidation) && !apperrors.IsKind(err, apperrors.KindFileSystem) {
				t.Fatalf("ValidateUploadFile() error kind = %q, want validation or file system", apperrors.KindOf(err))
			}
		})
	}

	t.Run("known duration", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "short.wav")
		writeWAVHeader(t, filePath, 3*time.Second)
		if err := ValidateUploadFile(filePath, nil); err != nil {
			t.Fatalf("ValidateUploadFile() error = %v, want nil", err)
		}
	})

	t.Run("unprobed supported extension", func(t *testing.T) {
		filePath := writeUploadFile(t, tempDir, "clip.aac", []byte("audio"))
		if err := ValidateUploadFile(filePath, nil); err != nil {
			t.Fatalf("ValidateUploadFile() error = %v, want nil", err)
		}
	})

	t.Run("probe failure allowed", func(t *testing.T) {
		filePath := writeUploadFile(t, tempDir, "clip.ogg", []byte("not ogg"))
		if err := ValidateUploadFile(filePath, nil); err != nil {
			t.Fatalf("ValidateUploadFile() error = %v, want nil", err)
		}
	})
}

func TestProbeUploadDuration(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("skipped extension", func(t *testing.T) {
		duration, known, err := ProbeUploadDuration(filepath.Join(tempDir, "missing.aac"), ".aac")
		if err != nil {
			t.Fatalf("ProbeUploadDuration() error = %v, want nil", err)
		}
		if known || duration != 0 {
			t.Fatalf("ProbeUploadDuration() = %v, %v, want unknown zero", duration, known)
		}
	})

	t.Run("missing probed file", func(t *testing.T) {
		_, known, err := ProbeUploadDuration(filepath.Join(tempDir, "missing.wav"), ".wav")
		if err == nil {
			t.Fatal("ProbeUploadDuration() error = nil, want error")
		}
		if known {
			t.Fatal("ProbeUploadDuration() known = true, want false")
		}
	})

	t.Run("wav", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "known.wav")
		writeWAVHeader(t, filePath, 3*time.Second)
		duration, known, err := ProbeUploadDuration(filePath, ".wav")
		if err != nil {
			t.Fatalf("ProbeUploadDuration() error = %v, want nil", err)
		}
		if !known || duration != 3*time.Second {
			t.Fatalf("ProbeUploadDuration() = %v, %v, want 3s known", duration, known)
		}
	})

	t.Run("mp4", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "known.mp4")
		writeMP4WithMovieDuration(t, filePath, 1000, 2500)
		duration, known, err := ProbeUploadDuration(filePath, ".mp4")
		if err != nil {
			t.Fatalf("ProbeUploadDuration() error = %v, want nil", err)
		}
		if !known || duration != 2500*time.Millisecond {
			t.Fatalf("ProbeUploadDuration() = %v, %v, want 2.5s known", duration, known)
		}
	})

	t.Run("mp4 decode error", func(t *testing.T) {
		filePath := writeUploadFile(t, tempDir, "broken.mp4", []byte("not mp4"))
		_, known, err := ProbeUploadDuration(filePath, ".mp4")
		if err == nil {
			t.Fatal("ProbeUploadDuration() error = nil, want error")
		}
		if known {
			t.Fatal("ProbeUploadDuration() known = true, want false")
		}
	})

	t.Run("mp4 unknown duration", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "unknown.mp4")
		if err := os.WriteFile(filePath, mp4Box(t, "ftyp", []byte("isom\x00\x00\x00\x00isom")), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		_, known, err := ProbeUploadDuration(filePath, ".mp4")
		if !errors.Is(err, ErrUploadDurationUnknown) {
			t.Fatalf("ProbeUploadDuration() error = %v, want unknown", err)
		}
		if known {
			t.Fatal("ProbeUploadDuration() known = true, want false")
		}
	})

	t.Run("mp3", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "known.mp3")
		writeMP3SilentFrame(t, filePath)
		duration, known, err := ProbeUploadDuration(filePath, ".mp3")
		if err != nil {
			t.Fatalf("ProbeUploadDuration() error = %v, want nil", err)
		}
		if !known || duration <= 0 {
			t.Fatalf("ProbeUploadDuration() = %v, %v, want known positive duration", duration, known)
		}
	})

	t.Run("mp3 decode error", func(t *testing.T) {
		filePath := writeUploadFile(t, tempDir, "broken.mp3", []byte("not mp3"))
		_, known, err := ProbeUploadDuration(filePath, ".mp3")
		if err == nil {
			t.Fatal("ProbeUploadDuration() error = nil, want error")
		}
		if known {
			t.Fatal("ProbeUploadDuration() known = true, want false")
		}
	})

	t.Run("ogg", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "known.ogg")
		writeOggVorbisFile(t, filePath, 44100, 44100)
		duration, known, err := ProbeUploadDuration(filePath, ".ogg")
		if err != nil {
			t.Fatalf("ProbeUploadDuration() error = %v, want nil", err)
		}
		if !known || duration != time.Second {
			t.Fatalf("ProbeUploadDuration() = %v, %v, want 1s known", duration, known)
		}
	})
}

func TestProbeUploadDurationFileHandlesProbeResults(t *testing.T) {
	filePath := writeUploadFile(t, t.TempDir(), "clip.aac", []byte("audio"))
	wantErr := errors.New("probe failed")

	t.Run("probe error", func(t *testing.T) {
		_, known, err := ProbeUploadDurationFile(filePath, func(*os.File) (time.Duration, error) {
			return 0, wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("ProbeUploadDurationFile() error = %v, want %v", err, wantErr)
		}
		if known {
			t.Fatal("ProbeUploadDurationFile() known = true, want false")
		}
	})

	t.Run("zero duration", func(t *testing.T) {
		_, known, err := ProbeUploadDurationFile(filePath, func(*os.File) (time.Duration, error) {
			return 0, nil
		})
		if !errors.Is(err, ErrUploadDurationUnknown) {
			t.Fatalf("ProbeUploadDurationFile() error = %v, want unknown", err)
		}
		if known {
			t.Fatal("ProbeUploadDurationFile() known = true, want false")
		}
	})
}

func TestDurationProbeFailures(t *testing.T) {
	t.Run("seek errors", func(t *testing.T) {
		tests := []struct {
			name  string
			probe func(*os.File) (time.Duration, error)
		}{
			{name: "mp4", probe: ProbeMP4Duration},
			{name: "wav", probe: ProbeWAVDuration},
			{name: "mp3", probe: ProbeMP3Duration},
			{name: "ogg", probe: ProbeOggVorbisDuration},
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
	})

	t.Run("wav invalid header", func(t *testing.T) {
		filePath := writeUploadFile(t, t.TempDir(), "broken.wav", []byte("not wav"))
		file, err := os.Open(filePath)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer file.Close()

		if _, err := ProbeWAVDuration(file); err == nil {
			t.Fatal("ProbeWAVDuration() error = nil, want error")
		}
	})

	t.Run("wav invalid metadata", func(t *testing.T) {
		filePath := filepath.Join(t.TempDir(), "invalid.wav")
		writeCustomWAVHeader(t, filePath, 1, 16, 0)
		file, err := os.Open(filePath)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer file.Close()

		if _, err := ProbeWAVDuration(file); !errors.Is(err, ErrUploadDurationUnknown) {
			t.Fatalf("ProbeWAVDuration() error = %v, want unknown", err)
		}
	})

	t.Run("ogg invalid metadata", func(t *testing.T) {
		filePath := filepath.Join(t.TempDir(), "invalid.ogg")
		writeOggVorbisFile(t, filePath, 0, 44100)
		file, err := os.Open(filePath)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer file.Close()

		if _, err := ProbeOggVorbisDuration(file); !errors.Is(err, ErrUploadDurationUnknown) {
			t.Fatalf("ProbeOggVorbisDuration() error = %v, want unknown", err)
		}
	})
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
			if got := DurationFromTimeUnits(tt.units, tt.unitsPerSecond); got != tt.want {
				t.Fatalf("DurationFromTimeUnits() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOpenSource(t *testing.T) {
	t.Run("file path", func(t *testing.T) {
		path := writeUploadFile(t, t.TempDir(), "clip.mp4", []byte("abc"))
		source, err := OpenSource(SourceOptions{FilePath: path})
		if err != nil {
			t.Fatalf("OpenSource() error = %v, want nil", err)
		}
		if closer, ok := source.Reader.(io.Closer); ok {
			t.Cleanup(func() { _ = closer.Close() })
		}
		if source.Name != "clip.mp4" || source.Size != 3 {
			t.Fatalf("source = %#v, want file name and size", source)
		}
	})

	t.Run("file path open failure", func(t *testing.T) {
		_, err := OpenSource(SourceOptions{FilePath: filepath.Join(t.TempDir(), "missing.mp4")})
		if err == nil {
			t.Fatal("OpenSource() error = nil, want open error")
		}
	})

	t.Run("missing source", func(t *testing.T) {
		_, err := OpenSource(SourceOptions{})
		if err == nil || !strings.Contains(err.Error(), "path or reader") {
			t.Fatalf("OpenSource() error = %v, want source error", err)
		}
	})

	t.Run("negative size", func(t *testing.T) {
		_, err := OpenSource(SourceOptions{Reader: bytes.NewReader(nil), Size: -1})
		if err == nil || err.Error() != "upload size cannot be negative" {
			t.Fatalf("OpenSource() error = %v, want negative size", err)
		}
	})

	t.Run("provided size and trimmed name", func(t *testing.T) {
		source, err := OpenSource(SourceOptions{
			Reader: bytes.NewReader([]byte("abc")),
			Size:   10,
			Name:   " clip.mp4 ",
		})
		if err != nil {
			t.Fatalf("OpenSource() error = %v, want nil", err)
		}
		if source.Size != 10 || source.Name != "clip.mp4" {
			t.Fatalf("source = %#v, want provided size and trimmed name", source)
		}
	})

	t.Run("infers size and default name", func(t *testing.T) {
		source, err := OpenSource(SourceOptions{Reader: bytes.NewReader([]byte("abc"))})
		if err != nil {
			t.Fatalf("OpenSource() error = %v, want nil", err)
		}
		if source.Size != 3 || source.Name != "upload" {
			t.Fatalf("source = %#v, want inferred size and default name", source)
		}
	})

	for _, tt := range []struct {
		name         string
		failSeekCall int
		want         string
	}{
		{name: "current seek failure", failSeekCall: 1, want: "upload size is required"},
		{name: "end seek failure", failSeekCall: 2, want: "upload size is required"},
		{name: "restore seek failure", failSeekCall: 3, want: "seek failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reader := &controlledReadSeeker{
				reader:       bytes.NewReader([]byte("abc")),
				failSeekCall: tt.failSeekCall,
				seekErr:      errors.New("seek failed"),
			}
			_, err := OpenSource(SourceOptions{Reader: reader})
			if err == nil {
				t.Fatal("OpenSource() error = nil, want seek error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("OpenSource() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestReadHeader(t *testing.T) {
	t.Run("short header", func(t *testing.T) {
		got, err := ReadHeader(strings.NewReader("abc"))
		if err != nil {
			t.Fatalf("ReadHeader() error = %v, want nil", err)
		}
		if got != base64.StdEncoding.EncodeToString([]byte("abc")) {
			t.Fatalf("ReadHeader() = %q, want base64 header", got)
		}
	})

	t.Run("max header", func(t *testing.T) {
		content := bytes.Repeat([]byte("a"), FileHeaderSize+10)
		got, err := ReadHeader(bytes.NewReader(content))
		if err != nil {
			t.Fatalf("ReadHeader() error = %v, want nil", err)
		}
		want := base64.StdEncoding.EncodeToString(content[:FileHeaderSize])
		if got != want {
			t.Fatalf("ReadHeader() length = %d, want encoded first %d bytes", len(got), FileHeaderSize)
		}
	})

	t.Run("read error", func(t *testing.T) {
		wantErr := errors.New("read failed")
		if _, err := ReadHeader(errorReader{err: wantErr}); !errors.Is(err, wantErr) {
			t.Fatalf("ReadHeader() error = %v, want %v", err, wantErr)
		}
	})
}

func TestComputeBlocks(t *testing.T) {
	content := []byte("abcdefghijkl")
	wantBlocks := expectedBlocks(content, 5)
	blocks, err := ComputeBlocks(bytes.NewReader(content), int64(len(content)), 5)
	if err != nil {
		t.Fatalf("ComputeBlocks() error = %v, want nil", err)
	}
	if !blocksEqual(blocks, wantBlocks) {
		t.Fatalf("blocks = %#v, want %#v", blocks, wantBlocks)
	}

	t.Run("exact block size", func(t *testing.T) {
		blocks, err := ComputeBlocks(bytes.NewReader([]byte("abcde")), 5, 5)
		if err != nil {
			t.Fatalf("ComputeBlocks() error = %v, want nil", err)
		}
		if !blocksEqual(blocks, expectedBlocks([]byte("abcde"), 5)) {
			t.Fatalf("blocks = %#v, want single full block", blocks)
		}
	})

	t.Run("invalid block size", func(t *testing.T) {
		if _, err := ComputeBlocks(bytes.NewReader([]byte("abc")), 3, 0); err == nil {
			t.Fatal("ComputeBlocks() error = nil, want block size error")
		}
	})

	t.Run("seek error", func(t *testing.T) {
		wantErr := errors.New("seek failed")
		reader := &controlledReadSeeker{reader: bytes.NewReader([]byte("abc")), failSeekCall: 1, seekErr: wantErr}
		if _, err := ComputeBlocks(reader, 3, 5); !errors.Is(err, wantErr) {
			t.Fatalf("ComputeBlocks() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("read error", func(t *testing.T) {
		wantErr := errors.New("read failed")
		reader := &controlledReadSeeker{reader: bytes.NewReader([]byte("abc")), readErr: wantErr}
		if _, err := ComputeBlocks(reader, 3, 5); !errors.Is(err, wantErr) {
			t.Fatalf("ComputeBlocks() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("zero size", func(t *testing.T) {
		blocks, err := ComputeBlocks(bytes.NewReader(nil), 0, 5)
		if err != nil {
			t.Fatalf("ComputeBlocks() error = %v, want nil", err)
		}
		if blocks == nil || len(blocks) != 0 {
			t.Fatalf("blocks = %#v, want empty slice", blocks)
		}
	})
}

func TestReadBlock(t *testing.T) {
	content := []byte("abc")
	block := expectedBlocks(content, 5)[0]

	t.Run("success", func(t *testing.T) {
		got, err := ReadBlock(bytes.NewReader(content), block, 5)
		if err != nil {
			t.Fatalf("ReadBlock() error = %v, want nil", err)
		}
		if string(got) != string(content) {
			t.Fatalf("ReadBlock() = %q, want %q", got, content)
		}
	})

	t.Run("seek error", func(t *testing.T) {
		wantErr := errors.New("seek failed")
		reader := &controlledReadSeeker{reader: bytes.NewReader(content), failSeekCall: 1, seekErr: wantErr}
		if _, err := ReadBlock(reader, block, 5); !errors.Is(err, wantErr) {
			t.Fatalf("ReadBlock() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("read error", func(t *testing.T) {
		wantErr := errors.New("read failed")
		reader := &controlledReadSeeker{reader: bytes.NewReader(content), readErr: wantErr}
		if _, err := ReadBlock(reader, block, 5); !errors.Is(err, wantErr) {
			t.Fatalf("ReadBlock() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("hash mismatch", func(t *testing.T) {
		badBlock := block
		badBlock.Hash = "bad"
		_, err := ReadBlock(bytes.NewReader(content), badBlock, 5)
		if err == nil || !strings.Contains(err.Error(), "hash mismatch") {
			t.Fatalf("ReadBlock() error = %v, want hash mismatch", err)
		}
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		badBlock := block
		badBlock.Checksum = "bad"
		_, err := ReadBlock(bytes.NewReader(content), badBlock, 5)
		if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
			t.Fatalf("ReadBlock() error = %v, want checksum mismatch", err)
		}
	})
}

func TestBlockChecksumHelperSanity(t *testing.T) {
	data := []byte("abc")
	block := NewBlock(0, data)
	if block.Hash != Hash(data) {
		t.Fatalf("hash = %q, want helper hash", block.Hash)
	}
	if block.Checksum != Checksum(data) {
		t.Fatalf("checksum = %q, want helper checksum", block.Checksum)
	}
	if block.Checksum != strconv.FormatUint(uint64(adler32.Checksum(data)), 10) {
		t.Fatalf("checksum = %q, want adler32", block.Checksum)
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
	writeBigEndian(t, mvhdPayload, uint32(0))
	writeBigEndian(t, mvhdPayload, uint32(0))
	writeBigEndian(t, mvhdPayload, uint32(0))
	writeBigEndian(t, mvhdPayload, timescale)
	writeBigEndian(t, mvhdPayload, duration)
	writeBigEndian(t, mvhdPayload, uint32(0x00010000))
	writeBigEndian(t, mvhdPayload, uint16(0x0100))
	writeBigEndian(t, mvhdPayload, uint16(0))
	writeBigEndian(t, mvhdPayload, uint32(0))
	writeBigEndian(t, mvhdPayload, uint32(0))
	for _, value := range []uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000} {
		writeBigEndian(t, mvhdPayload, value)
	}
	for i := 0; i < 6; i++ {
		writeBigEndian(t, mvhdPayload, uint32(0))
	}
	writeBigEndian(t, mvhdPayload, uint32(1))

	content := append(mp4Box(t, "ftyp", []byte("isom\x00\x00\x00\x00isom")), mp4Box(t, "moov", mp4Box(t, "mvhd", mvhdPayload.Bytes()))...)
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func mp4Box(t *testing.T, boxType string, payload []byte) []byte {
	t.Helper()

	buffer := new(bytes.Buffer)
	writeBigEndian(t, buffer, uint32(8+len(payload)))
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

func writeOggVorbisFile(t *testing.T, filePath string, sampleRate uint32, samples int64) {
	t.Helper()

	const streamSerial = uint32(1)
	content := append(
		oggPage(t, 2, 0, streamSerial, 0, [][]byte{vorbisIdentificationPacket(t, sampleRate)}),
		oggPage(t, 4, samples, streamSerial, 1, [][]byte{{}})...,
	)
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func vorbisIdentificationPacket(t *testing.T, sampleRate uint32) []byte {
	t.Helper()

	buffer := new(bytes.Buffer)
	buffer.WriteByte(1)
	buffer.WriteString("vorbis")
	writeLittleEndian(t, buffer, uint32(0))
	buffer.WriteByte(1)
	writeLittleEndian(t, buffer, sampleRate)
	writeLittleEndian(t, buffer, uint32(0))
	writeLittleEndian(t, buffer, uint32(0))
	writeLittleEndian(t, buffer, uint32(0))
	buffer.WriteByte(4 | (5 << 4))
	buffer.WriteByte(1)
	return buffer.Bytes()
}

func oggPage(t *testing.T, headerType byte, granule int64, serial uint32, seq uint32, packets [][]byte) []byte {
	t.Helper()

	var body bytes.Buffer
	var segments []byte
	for _, packet := range packets {
		if len(packet) > 255 {
			t.Fatalf("test ogg packet is too large: %d", len(packet))
		}
		segments = append(segments, byte(len(packet)))
		body.Write(packet)
	}

	page := new(bytes.Buffer)
	page.WriteString("OggS")
	page.WriteByte(0)
	page.WriteByte(headerType)
	writeLittleEndian(t, page, granule)
	writeLittleEndian(t, page, serial)
	writeLittleEndian(t, page, seq)
	writeLittleEndian(t, page, uint32(0))
	page.WriteByte(byte(len(segments)))
	page.Write(segments)
	page.Write(body.Bytes())

	data := page.Bytes()
	binary.LittleEndian.PutUint32(data[22:26], oggChecksum(data))
	return data
}

func oggChecksum(data []byte) uint32 {
	var table [256]uint32
	for i := range table {
		r := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if r&0x80000000 != 0 {
				r = (r << 1) ^ 0x04c11db7
			} else {
				r <<= 1
			}
		}
		table[i] = r
	}

	var crc uint32
	for _, b := range data {
		crc = (crc << 8) ^ table[byte(crc>>24)^b]
	}
	return crc
}

func expectedBlocks(content []byte, blockSize int) []Block {
	var blocks []Block
	for seq, offset := 0, 0; offset < len(content); seq, offset = seq+1, offset+blockSize {
		end := offset + blockSize
		if end > len(content) {
			end = len(content)
		}
		blocks = append(blocks, NewBlock(seq, content[offset:end]))
	}

	return blocks
}

func blocksEqual(a, b []Block) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func writeBigEndian(t *testing.T, buffer *bytes.Buffer, value any) {
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

type controlledReadSeeker struct {
	reader       *bytes.Reader
	failSeekCall int
	seekCalls    int
	seekErr      error
	readErr      error
}

func (r *controlledReadSeeker) Read(p []byte) (int, error) {
	if r.readErr != nil {
		return 0, r.readErr
	}

	return r.reader.Read(p)
}

func (r *controlledReadSeeker) Seek(offset int64, whence int) (int64, error) {
	r.seekCalls++
	if r.failSeekCall == r.seekCalls {
		return 0, r.seekErr
	}

	return r.reader.Seek(offset, whence)
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}
