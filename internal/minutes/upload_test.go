package minutes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/adler32"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestUploadFileFullFlow(t *testing.T) {
	content := []byte("abcdefghijkl")
	expectedBlocks := expectedUploadBlocks(t, content, 5)
	var calls []string
	var expectedReferer string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		assertCommonHeaders(t, r, testCookie, "csrf-token", expectedReferer, "openminutes-test")

		switch r.URL.Path {
		case "/minutes/api/quota":
			assertMethod(t, r, http.MethodGet)
			if got := r.URL.Query().Get("file_info[]"); got != "file-id_12" {
				t.Fatalf("file_info[] = %q, want file-id_12", got)
			}
			if got := r.URL.Query().Get("without_quota"); got != "true" {
				t.Fatalf("without_quota = %q, want true", got)
			}
			if got := r.URL.Query().Get("language"); got != "zh_cn" {
				t.Fatalf("language = %q, want zh_cn", got)
			}
			fmt.Fprint(w, `{"code":0,"msg":"success","data":{"upload_token":{"file-id_12":"upload-token"}}}`)
		case "/minutes/api/upload/prepare":
			assertMethod(t, r, http.MethodPost)
			var payload prepareUploadRequest
			decodeJSONBody(t, r, &payload)
			wantHeader := base64.StdEncoding.EncodeToString(content)
			if payload.Name != "clip.mp4" || payload.FileSize != 12 || payload.FileHeader != wantHeader || !payload.DriveUpload || payload.UploadToken != "upload-token" || payload.Language != "zh_cn" {
				t.Fatalf("prepare payload = %#v, want expected upload prepare body", payload)
			}
			fmt.Fprint(w, `{"code":0,"msg":"success","data":{"vhid":"vhid-1","object_token":"object-1","upload_id":"upload-1","block_size":5,"num_blocks":3}}`)
		case "/space/api/box/upload/blocks":
			assertMethod(t, r, http.MethodPost)
			var payload uploadBlocksRequest
			decodeJSONBody(t, r, &payload)
			if payload.UploadID != "upload-1" || payload.Language != "zh_cn" {
				t.Fatalf("upload blocks payload = %#v, want upload id and language", payload)
			}
			if !blocksEqual(payload.Blocks, expectedBlocks) {
				t.Fatalf("blocks = %#v, want %#v", payload.Blocks, expectedBlocks)
			}
			response := uploadBlocksResponse{NeededUploadBlocks: []uploadBlock{expectedBlocks[0], expectedBlocks[2]}}
			writeEnvelope(t, w, response)
		case "/space/api/box/stream/upload/block":
			assertMethod(t, r, http.MethodPost)
			if got := r.Header.Get("content-type"); got != "application/octet-stream" {
				t.Fatalf("content-type = %q, want application/octet-stream", got)
			}
			seq := mustAtoi(t, r.URL.Query().Get("seq"))
			if seq != 0 && seq != 2 {
				t.Fatalf("uploaded seq = %d, want 0 or 2", seq)
			}
			block := expectedBlocks[seq]
			if got := r.URL.Query().Get("upload_id"); got != "upload-1" {
				t.Fatalf("upload_id = %q, want upload-1", got)
			}
			if got := r.URL.Query().Get("size"); got != strconv.FormatInt(block.Size, 10) {
				t.Fatalf("size = %q, want %d", got, block.Size)
			}
			if got := r.URL.Query().Get("checksum"); got != block.Checksum {
				t.Fatalf("checksum = %q, want %q", got, block.Checksum)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if !bytes.Equal(body, blockBytes(content, 5, seq)) {
				t.Fatalf("block body = %q, want %q", body, blockBytes(content, 5, seq))
			}
			fmt.Fprint(w, `{"code":0,"message":"Success","data":{}}`)
		case "/space/api/box/upload/finish/":
			assertMethod(t, r, http.MethodPost)
			var payload finishSpaceUploadRequest
			decodeJSONBody(t, r, &payload)
			if payload.UploadID != "upload-1" || payload.NumBlocks != 3 || payload.VHID != "vhid-1" || payload.Language != "zh_cn" {
				t.Fatalf("space finish payload = %#v, want upload finish identifiers", payload)
			}
			if payload.RiskDetectionExtra != `{"file_operate_usage":3,"locale":"zh_cn"}` {
				t.Fatalf("risk_detection_extra = %q, want HAR value", payload.RiskDetectionExtra)
			}
			fmt.Fprint(w, `{"code":0,"message":"Success","data":{}}`)
		case "/minutes/api/upload/finish":
			assertMethod(t, r, http.MethodPost)
			var payload finishMinutesUploadRequest
			decodeJSONBody(t, r, &payload)
			if !payload.AutoTranscribe || payload.Language != "mixed" || payload.NumBlocks != 3 || payload.ObjectToken != "object-1" || payload.UploadID != "upload-1" || payload.UploadToken != "upload-token" || payload.VHID != "vhid-1" {
				t.Fatalf("minutes finish payload = %#v, want expected final body", payload)
			}
			fmt.Fprint(w, `{"code":0,"msg":"success","data":"[]"}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)
	client.referer = server.URL + "/minutes/home"
	expectedReferer = client.referer

	result, err := client.UploadFile(context.Background(), UploadOptions{
		Reader: bytes.NewReader(content),
		Size:   int64(len(content)),
		Name:   "clip.mp4",
		FileID: "file-id",
	})
	if err != nil {
		t.Fatalf("UploadFile() error = %v, want nil", err)
	}

	if *result != (UploadResult{ObjectToken: "object-1", UploadID: "upload-1", VHID: "vhid-1", UploadToken: "upload-token", NumBlocks: 3}) {
		t.Fatalf("result = %#v, want upload identifiers", result)
	}

	wantCalls := strings.Join([]string{
		"GET /minutes/api/quota",
		"POST /minutes/api/upload/prepare",
		"POST /space/api/box/upload/blocks",
		"POST /space/api/box/stream/upload/block",
		"POST /space/api/box/stream/upload/block",
		"POST /space/api/box/upload/finish/",
		"POST /minutes/api/upload/finish",
	}, ",")
	if got := strings.Join(calls, ","); got != wantCalls {
		t.Fatalf("calls = %s, want %s", got, wantCalls)
	}
}

func TestUploadFileMissingUploadToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{"upload_token":{}}}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	_, err := client.UploadFile(context.Background(), UploadOptions{
		Reader: bytes.NewReader([]byte("abc")),
		Size:   3,
		Name:   "clip.mp4",
		FileID: "file-id",
	})
	if err == nil {
		t.Fatal("UploadFile() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing upload_token") {
		t.Fatalf("error = %q, want missing upload_token", err.Error())
	}
}

func TestUploadFileMissingPrepareField(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			fmt.Fprint(w, `{"code":0,"msg":"success","data":{"upload_token":{"file-id_3":"upload-token"}}}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{"vhid":"vhid-1","object_token":"object-1","block_size":5,"num_blocks":1}}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	_, err := client.UploadFile(context.Background(), UploadOptions{
		Reader: bytes.NewReader([]byte("abc")),
		Size:   3,
		Name:   "clip.mp4",
		FileID: "file-id",
	})
	if err == nil {
		t.Fatal("UploadFile() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "upload_id") {
		t.Fatalf("error = %q, want upload_id", err.Error())
	}
}

func TestUploadFileMissingNeededBlocks(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch requestCount {
		case 1:
			fmt.Fprint(w, `{"code":0,"msg":"success","data":{"upload_token":{"file-id_3":"upload-token"}}}`)
		case 2:
			fmt.Fprint(w, `{"code":0,"msg":"success","data":{"vhid":"vhid-1","object_token":"object-1","upload_id":"upload-1","block_size":5,"num_blocks":1}}`)
		case 3:
			fmt.Fprint(w, `{"code":0,"msg":"success","data":{}}`)
		default:
			t.Fatalf("unexpected request %d %s", requestCount, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, server.URL)

	_, err := client.UploadFile(context.Background(), UploadOptions{
		Reader: bytes.NewReader([]byte("abc")),
		Size:   3,
		Name:   "clip.mp4",
		FileID: "file-id",
	})
	if err == nil {
		t.Fatal("UploadFile() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "needed_upload_blocks") {
		t.Fatalf("error = %q, want needed_upload_blocks", err.Error())
	}
}

func expectedUploadBlocks(t *testing.T, content []byte, blockSize int) []uploadBlock {
	t.Helper()

	var blocks []uploadBlock
	for seq, offset := 0, 0; offset < len(content); seq, offset = seq+1, offset+blockSize {
		end := offset + blockSize
		if end > len(content) {
			end = len(content)
		}
		data := content[offset:end]
		hash := sha256.Sum256(data)
		blocks = append(blocks, uploadBlock{
			Hash:     base64.StdEncoding.EncodeToString(hash[:]),
			Seq:      seq,
			Size:     int64(len(data)),
			Checksum: strconv.FormatUint(uint64(adler32.Checksum(data)), 10),
		})
	}

	return blocks
}

func blockBytes(content []byte, blockSize int, seq int) []byte {
	offset := seq * blockSize
	end := offset + blockSize
	if end > len(content) {
		end = len(content)
	}

	return content[offset:end]
}

func blocksEqual(a, b []uploadBlock) bool {
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

func decodeJSONBody(t *testing.T, r *http.Request, result any) {
	t.Helper()

	if got := r.Header.Get("content-type"); got != "application/json" {
		t.Fatalf("content-type = %q, want application/json", got)
	}
	if err := json.NewDecoder(r.Body).Decode(result); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
}

func writeEnvelope(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()

	payload, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	fmt.Fprintf(w, `{"code":0,"msg":"success","data":%s}`, payload)
}

func assertMethod(t *testing.T, r *http.Request, method string) {
	t.Helper()

	if r.Method != method {
		t.Fatalf("method = %s, want %s", r.Method, method)
	}
}

func mustAtoi(t *testing.T, value string) int {
	t.Helper()

	number, err := strconv.Atoi(value)
	if err != nil {
		t.Fatalf("Atoi(%q) error = %v", value, err)
	}

	return number
}
