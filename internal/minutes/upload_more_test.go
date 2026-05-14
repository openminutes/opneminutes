package minutes

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/adler32"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestUploadFileSourceAndReaderFailures(t *testing.T) {
	client := newTestClient(t, "https://example.test", "https://space.example.test")

	t.Run("missing source", func(t *testing.T) {
		_, err := client.UploadFile(context.Background(), UploadOptions{})
		if err == nil {
			t.Fatal("UploadFile() error = nil, want source error")
		}
		if !strings.Contains(err.Error(), "path or reader") {
			t.Fatalf("UploadFile() error = %q, want source error", err.Error())
		}
	})

	t.Run("initial seek failure", func(t *testing.T) {
		wantErr := errors.New("initial seek failed")
		reader := &controlledReadSeeker{reader: bytes.NewReader([]byte("abc")), failSeekCall: 1, seekErr: wantErr}

		_, err := client.UploadFile(context.Background(), UploadOptions{Reader: reader, Size: 3, Name: "clip.mp4"})
		if !errors.Is(err, wantErr) {
			t.Fatalf("UploadFile() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("header read failure", func(t *testing.T) {
		wantErr := errors.New("header read failed")
		reader := &controlledReadSeeker{reader: bytes.NewReader([]byte("abc")), readErr: wantErr}

		_, err := client.UploadFile(context.Background(), UploadOptions{Reader: reader, Size: 3, Name: "clip.mp4"})
		if !errors.Is(err, wantErr) {
			t.Fatalf("UploadFile() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("second seek failure", func(t *testing.T) {
		wantErr := errors.New("second seek failed")
		reader := &controlledReadSeeker{reader: bytes.NewReader([]byte("abc")), failSeekCall: 2, seekErr: wantErr}

		_, err := client.UploadFile(context.Background(), UploadOptions{Reader: reader, Size: 3, Name: "clip.mp4"})
		if !errors.Is(err, wantErr) {
			t.Fatalf("UploadFile() error = %v, want %v", err, wantErr)
		}
	})
}

func TestUploadFileDefaultFileIDCloserAndAutoTranscribeFalse(t *testing.T) {
	content := []byte("abc")
	reader := &closableReadSeeker{Reader: bytes.NewReader(content)}
	autoTranscribe := false
	var sawMinutesFinish bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/minutes/api/quota":
			fileInfo := r.URL.Query().Get("file_info[]")
			if !strings.HasSuffix(fileInfo, "_3") || strings.HasPrefix(fileInfo, "_") {
				t.Fatalf("file_info[] = %q, want generated file id and size", fileInfo)
			}
			writeEnvelope(t, w, quotaResponse{UploadToken: map[string]string{fileInfo: "upload-token"}})
		case "/minutes/api/upload/prepare":
			var payload prepareUploadRequest
			decodeJSONBody(t, r, &payload)
			if payload.Language != "en_us" {
				t.Fatalf("prepare language = %q, want en_us", payload.Language)
			}
			writeEnvelope(t, w, prepareUploadResponse{
				VHID:        "vhid-1",
				ObjectToken: "object-1",
				UploadID:    "upload-1",
				BlockSize:   5,
				NumBlocks:   1,
			})
		case "/space/api/box/upload/blocks":
			writeEnvelope(t, w, uploadBlocksResponse{NeededUploadBlocks: []uploadBlock{}})
		case "/space/api/box/upload/finish/":
			writeEnvelope(t, w, map[string]string{})
		case "/minutes/api/upload/finish":
			sawMinutesFinish = true
			var payload finishMinutesUploadRequest
			decodeJSONBody(t, r, &payload)
			if payload.AutoTranscribe {
				t.Fatal("AutoTranscribe = true, want false")
			}
			if payload.Language != "ja_jp" {
				t.Fatalf("transcribe language = %q, want ja_jp", payload.Language)
			}
			writeEnvelope(t, w, map[string]string{})
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	result, err := client.UploadFile(context.Background(), UploadOptions{
		Reader:             reader,
		Name:               "clip.mp4",
		Language:           "en_us",
		TranscribeLanguage: "ja_jp",
		AutoTranscribe:     &autoTranscribe,
	})
	if err != nil {
		t.Fatalf("UploadFile() error = %v, want nil", err)
	}
	if result.ObjectToken != "object-1" || result.NumBlocks != 1 {
		t.Fatalf("UploadFile() result = %#v, want upload identifiers", result)
	}
	if !reader.closed {
		t.Fatal("reader was not closed")
	}
	if !sawMinutesFinish {
		t.Fatal("minutes finish request was not sent")
	}
}

func TestUploadFileFailureStages(t *testing.T) {
	content := []byte("abc")
	goodBlock := expectedUploadBlocks(t, content, 5)[0]

	tests := []struct {
		name      string
		reader    io.ReadSeeker
		handler   func(t *testing.T, w http.ResponseWriter, r *http.Request, requestNumber int)
		wantError string
	}{
		{
			name: "upload token API error",
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request, requestNumber int) {
				fmt.Fprint(w, `{"code":9,"msg":"quota denied"}`)
			},
			wantError: "quota denied",
		},
		{
			name: "prepare API error",
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request, requestNumber int) {
				switch requestNumber {
				case 1:
					writeQuotaForRequest(t, w, r)
				case 2:
					fmt.Fprint(w, `{"code":9,"msg":"prepare denied"}`)
				default:
					t.Fatalf("unexpected request %d", requestNumber)
				}
			},
			wantError: "prepare denied",
		},
		{
			name: "compute blocks seek error",
			reader: &controlledReadSeeker{
				reader:       bytes.NewReader(content),
				failSeekCall: 3,
				seekErr:      errors.New("compute seek failed"),
			},
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request, requestNumber int) {
				switch requestNumber {
				case 1:
					writeQuotaForRequest(t, w, r)
				case 2:
					writePrepare(t, w, 5, 1)
				default:
					t.Fatalf("unexpected request %d", requestNumber)
				}
			},
			wantError: "compute seek failed",
		},
		{
			name: "block count mismatch",
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request, requestNumber int) {
				switch requestNumber {
				case 1:
					writeQuotaForRequest(t, w, r)
				case 2:
					writePrepare(t, w, 5, 2)
				default:
					t.Fatalf("unexpected request %d", requestNumber)
				}
			},
			wantError: "does not match computed blocks",
		},
		{
			name: "needed blocks API error",
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request, requestNumber int) {
				switch requestNumber {
				case 1:
					writeQuotaForRequest(t, w, r)
				case 2:
					writePrepare(t, w, 5, 1)
				case 3:
					fmt.Fprint(w, `{"code":9,"msg":"blocks denied"}`)
				default:
					t.Fatalf("unexpected request %d", requestNumber)
				}
			},
			wantError: "blocks denied",
		},
		{
			name: "block verification error",
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request, requestNumber int) {
				switch requestNumber {
				case 1:
					writeQuotaForRequest(t, w, r)
				case 2:
					writePrepare(t, w, 5, 1)
				case 3:
					badBlock := goodBlock
					badBlock.Hash = "bad"
					writeEnvelope(t, w, uploadBlocksResponse{NeededUploadBlocks: []uploadBlock{badBlock}})
				default:
					t.Fatalf("unexpected request %d", requestNumber)
				}
			},
			wantError: "hash mismatch",
		},
		{
			name: "upload block error",
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request, requestNumber int) {
				switch requestNumber {
				case 1:
					writeQuotaForRequest(t, w, r)
				case 2:
					writePrepare(t, w, 5, 1)
				case 3:
					writeEnvelope(t, w, uploadBlocksResponse{NeededUploadBlocks: []uploadBlock{goodBlock}})
				case 4:
					fmt.Fprint(w, `{"code":9,"msg":"block denied"}`)
				default:
					t.Fatalf("unexpected request %d", requestNumber)
				}
			},
			wantError: "block denied",
		},
		{
			name: "space finish error",
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request, requestNumber int) {
				switch requestNumber {
				case 1:
					writeQuotaForRequest(t, w, r)
				case 2:
					writePrepare(t, w, 5, 1)
				case 3:
					writeEnvelope(t, w, uploadBlocksResponse{NeededUploadBlocks: []uploadBlock{}})
				case 4:
					fmt.Fprint(w, `{"code":9,"msg":"space finish denied"}`)
				default:
					t.Fatalf("unexpected request %d", requestNumber)
				}
			},
			wantError: "space finish denied",
		},
		{
			name: "minutes finish error",
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request, requestNumber int) {
				switch requestNumber {
				case 1:
					writeQuotaForRequest(t, w, r)
				case 2:
					writePrepare(t, w, 5, 1)
				case 3:
					writeEnvelope(t, w, uploadBlocksResponse{NeededUploadBlocks: []uploadBlock{}})
				case 4:
					writeEnvelope(t, w, map[string]string{})
				case 5:
					fmt.Fprint(w, `{"code":9,"msg":"minutes finish denied"}`)
				default:
					t.Fatalf("unexpected request %d", requestNumber)
				}
			},
			wantError: "minutes finish denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				tt.handler(t, w, r, requests)
			}))
			t.Cleanup(server.Close)

			reader := tt.reader
			if reader == nil {
				reader = bytes.NewReader(content)
			}
			client := newTestClient(t, server.URL, server.URL)
			_, err := client.UploadFile(context.Background(), UploadOptions{
				Reader: reader,
				Size:   int64(len(content)),
				Name:   "clip.mp4",
				FileID: "file-id",
			})
			if err == nil {
				t.Fatal("UploadFile() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("UploadFile() error = %q, want %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestUploadRequestCreationAndMarshalErrors(t *testing.T) {
	client := newTestClient(t, "https://example.test", "https://space.example.test")
	ctx := context.Background()

	t.Run("get upload token request creation", func(t *testing.T) {
		client := newTestClient(t, "https://example.test", "https://space.example.test")
		client.baseURL = "http://[::1"
		if _, err := client.getUploadToken(ctx, "file_1", "zh_cn"); err == nil {
			t.Fatal("getUploadToken() error = nil, want request creation error")
		}
	})

	marshalErr := errors.New("marshal failed")
	marshalTests := []struct {
		name string
		call func() error
	}{
		{
			name: "prepare",
			call: func() error {
				_, err := client.prepareUpload(ctx, prepareUploadRequest{Name: "clip.mp4"})
				return err
			},
		},
		{
			name: "needed blocks",
			call: func() error {
				_, err := client.getNeededUploadBlocks(ctx, "upload-1", nil, "zh_cn")
				return err
			},
		},
		{
			name: "space finish",
			call: func() error {
				return client.finishSpaceUpload(ctx, finishSpaceUploadRequest{UploadID: "upload-1"})
			},
		},
		{
			name: "minutes finish",
			call: func() error {
				return client.finishMinutesUpload(ctx, finishMinutesUploadRequest{UploadID: "upload-1"})
			},
		},
	}
	for _, tt := range marshalTests {
		t.Run(tt.name+" marshal", func(t *testing.T) {
			withJSONMarshal(t, func(any) ([]byte, error) {
				return nil, marshalErr
			})
			if err := tt.call(); !errors.Is(err, marshalErr) {
				t.Fatalf("%s error = %v, want %v", tt.name, err, marshalErr)
			}
		})
	}

	requestTests := []struct {
		name  string
		setup func(*Client)
		call  func(*Client) error
	}{
		{
			name:  "prepare request creation",
			setup: func(c *Client) { c.baseURL = "http://[::1" },
			call: func(c *Client) error {
				_, err := c.prepareUpload(ctx, prepareUploadRequest{Name: "clip.mp4"})
				return err
			},
		},
		{
			name:  "needed blocks request creation",
			setup: func(c *Client) { c.spaceBaseURL = "http://[::1" },
			call: func(c *Client) error {
				_, err := c.getNeededUploadBlocks(ctx, "upload-1", nil, "zh_cn")
				return err
			},
		},
		{
			name:  "upload block request creation",
			setup: func(c *Client) { c.spaceBaseURL = "http://[::1" },
			call: func(c *Client) error {
				return c.uploadBlock(ctx, "upload-1", uploadBlock{Seq: 0, Size: 3, Checksum: "1"}, []byte("abc"))
			},
		},
		{
			name:  "space finish request creation",
			setup: func(c *Client) { c.spaceBaseURL = "http://[::1" },
			call: func(c *Client) error {
				return c.finishSpaceUpload(ctx, finishSpaceUploadRequest{UploadID: "upload-1"})
			},
		},
		{
			name:  "minutes finish request creation",
			setup: func(c *Client) { c.baseURL = "http://[::1" },
			call: func(c *Client) error {
				return c.finishMinutesUpload(ctx, finishMinutesUploadRequest{UploadID: "upload-1"})
			},
		},
	}
	for _, tt := range requestTests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, "https://example.test", "https://space.example.test")
			tt.setup(client)
			err := tt.call(client)
			if err == nil {
				t.Fatalf("%s error = nil, want request creation error", tt.name)
			}
			if !strings.Contains(err.Error(), "missing ']'") {
				t.Fatalf("%s error = %q, want URL parse error", tt.name, err.Error())
			}
		})
	}
}

func TestNewUploadSource(t *testing.T) {
	t.Run("file path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "clip.mp4")
		if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		source, err := newUploadSource(UploadOptions{FilePath: path})
		if err != nil {
			t.Fatalf("newUploadSource() error = %v, want nil", err)
		}
		if closer, ok := source.reader.(io.Closer); ok {
			t.Cleanup(func() { _ = closer.Close() })
		}
		if source.name != "clip.mp4" || source.size != 3 {
			t.Fatalf("source = %#v, want file name and size", source)
		}
	})

	t.Run("file path open failure", func(t *testing.T) {
		_, err := newUploadSource(UploadOptions{FilePath: filepath.Join(t.TempDir(), "missing.mp4")})
		if err == nil {
			t.Fatal("newUploadSource() error = nil, want open error")
		}
	})

	t.Run("negative size", func(t *testing.T) {
		_, err := newUploadSource(UploadOptions{Reader: bytes.NewReader(nil), Size: -1})
		if err == nil || err.Error() != "upload size cannot be negative" {
			t.Fatalf("newUploadSource() error = %v, want negative size", err)
		}
	})

	t.Run("infers size and default name", func(t *testing.T) {
		source, err := newUploadSource(UploadOptions{Reader: bytes.NewReader([]byte("abc"))})
		if err != nil {
			t.Fatalf("newUploadSource() error = %v, want nil", err)
		}
		if source.size != 3 || source.name != "upload" {
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
			_, err := newUploadSource(UploadOptions{Reader: reader})
			if err == nil {
				t.Fatal("newUploadSource() error = nil, want seek error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("newUploadSource() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestUploadHelpers(t *testing.T) {
	t.Run("read short header", func(t *testing.T) {
		got, err := readFileHeader(strings.NewReader("abc"))
		if err != nil {
			t.Fatalf("readFileHeader() error = %v, want nil", err)
		}
		if got != base64.StdEncoding.EncodeToString([]byte("abc")) {
			t.Fatalf("readFileHeader() = %q, want base64 header", got)
		}
	})

	t.Run("read header error", func(t *testing.T) {
		wantErr := errors.New("read failed")
		if _, err := readFileHeader(errorReader{err: wantErr}); !errors.Is(err, wantErr) {
			t.Fatalf("readFileHeader() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("compute invalid block size", func(t *testing.T) {
		if _, err := computeBlocks(bytes.NewReader([]byte("abc")), 3, 0); err == nil {
			t.Fatal("computeBlocks() error = nil, want block size error")
		}
	})

	t.Run("compute seek error", func(t *testing.T) {
		wantErr := errors.New("seek failed")
		reader := &controlledReadSeeker{reader: bytes.NewReader([]byte("abc")), failSeekCall: 1, seekErr: wantErr}
		if _, err := computeBlocks(reader, 3, 5); !errors.Is(err, wantErr) {
			t.Fatalf("computeBlocks() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("compute read error", func(t *testing.T) {
		wantErr := errors.New("read failed")
		reader := &controlledReadSeeker{reader: bytes.NewReader([]byte("abc")), readErr: wantErr}
		if _, err := computeBlocks(reader, 3, 5); !errors.Is(err, wantErr) {
			t.Fatalf("computeBlocks() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("compute zero size", func(t *testing.T) {
		blocks, err := computeBlocks(bytes.NewReader(nil), 0, 5)
		if err != nil {
			t.Fatalf("computeBlocks() error = %v, want nil", err)
		}
		if blocks == nil || len(blocks) != 0 {
			t.Fatalf("blocks = %#v, want empty slice", blocks)
		}
	})
}

func TestReadBlockErrors(t *testing.T) {
	content := []byte("abc")
	block := expectedUploadBlocks(t, content, 5)[0]

	t.Run("seek error", func(t *testing.T) {
		wantErr := errors.New("seek failed")
		reader := &controlledReadSeeker{reader: bytes.NewReader(content), failSeekCall: 1, seekErr: wantErr}
		if _, err := readBlock(reader, block, 5); !errors.Is(err, wantErr) {
			t.Fatalf("readBlock() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("read error", func(t *testing.T) {
		wantErr := errors.New("read failed")
		reader := &controlledReadSeeker{reader: bytes.NewReader(content), readErr: wantErr}
		if _, err := readBlock(reader, block, 5); !errors.Is(err, wantErr) {
			t.Fatalf("readBlock() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("hash mismatch", func(t *testing.T) {
		badBlock := block
		badBlock.Hash = "bad"
		_, err := readBlock(bytes.NewReader(content), badBlock, 5)
		if err == nil || !strings.Contains(err.Error(), "hash mismatch") {
			t.Fatalf("readBlock() error = %v, want hash mismatch", err)
		}
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		badBlock := block
		badBlock.Checksum = "bad"
		_, err := readBlock(bytes.NewReader(content), badBlock, 5)
		if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
			t.Fatalf("readBlock() error = %v, want checksum mismatch", err)
		}
	})
}

func TestPrepareUploadResponseValidate(t *testing.T) {
	valid := prepareUploadResponse{
		VHID:        "vhid-1",
		ObjectToken: "object-1",
		UploadID:    "upload-1",
		BlockSize:   5,
		NumBlocks:   1,
	}

	tests := []struct {
		name     string
		response prepareUploadResponse
		want     string
	}{
		{name: "valid", response: valid},
		{name: "missing vhid", response: prepareUploadResponse{ObjectToken: "object-1", UploadID: "upload-1", BlockSize: 5}, want: "vhid"},
		{name: "missing object token", response: prepareUploadResponse{VHID: "vhid-1", UploadID: "upload-1", BlockSize: 5}, want: "object_token"},
		{name: "missing upload id", response: prepareUploadResponse{VHID: "vhid-1", ObjectToken: "object-1", BlockSize: 5}, want: "upload_id"},
		{name: "missing block size", response: prepareUploadResponse{VHID: "vhid-1", ObjectToken: "object-1", UploadID: "upload-1"}, want: "block_size"},
		{name: "negative num blocks", response: prepareUploadResponse{VHID: "vhid-1", ObjectToken: "object-1", UploadID: "upload-1", BlockSize: 5, NumBlocks: -1}, want: "num_blocks"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.response.validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func writeQuotaForRequest(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	fileInfo := r.URL.Query().Get("file_info[]")
	if fileInfo == "" {
		t.Fatal("file_info[] query is empty")
	}
	writeEnvelope(t, w, quotaResponse{UploadToken: map[string]string{fileInfo: "upload-token"}})
}

func writePrepare(t *testing.T, w http.ResponseWriter, blockSize int64, numBlocks int) {
	t.Helper()

	writeEnvelope(t, w, prepareUploadResponse{
		VHID:        "vhid-1",
		ObjectToken: "object-1",
		UploadID:    "upload-1",
		BlockSize:   blockSize,
		NumBlocks:   numBlocks,
	})
}

func withJSONMarshal(t *testing.T, fn func(any) ([]byte, error)) {
	t.Helper()

	old := jsonMarshal
	jsonMarshal = fn
	t.Cleanup(func() {
		jsonMarshal = old
	})
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

type closableReadSeeker struct {
	*bytes.Reader
	closed bool
}

func (r *closableReadSeeker) Close() error {
	r.closed = true
	return nil
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestBlockChecksumHelperSanity(t *testing.T) {
	data := []byte("abc")
	block := expectedUploadBlocks(t, data, 5)[0]
	if block.Checksum != strconv.FormatUint(uint64(adler32.Checksum(data)), 10) {
		t.Fatalf("checksum = %q, want adler32", block.Checksum)
	}
}
