package minutes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/adler32"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const fileHeaderSize = 512

// UploadFile uploads a local media file to Feishu Minutes.
func (c *Client) UploadFile(ctx context.Context, options UploadOptions) (*UploadResult, error) {
	upload, err := newUploadSource(options)
	if err != nil {
		return nil, err
	}
	if closer, ok := upload.reader.(io.Closer); ok {
		defer closer.Close()
	}

	if err := seekStart(upload.reader); err != nil {
		return nil, err
	}
	fileHeader, err := readFileHeader(upload.reader)
	if err != nil {
		return nil, err
	}
	if err := seekStart(upload.reader); err != nil {
		return nil, err
	}

	fileID := strings.TrimSpace(options.FileID)
	if fileID == "" {
		fileID = newFileID()
	}
	fileInfo := fmt.Sprintf("%s_%d", fileID, upload.size)

	uploadToken, err := c.getUploadToken(ctx, fileInfo, defaultString(options.Language, defaultLanguage))
	if err != nil {
		return nil, err
	}

	prepare, err := c.prepareUpload(ctx, prepareUploadRequest{
		Name:        upload.name,
		FileSize:    upload.size,
		FileHeader:  fileHeader,
		DriveUpload: true,
		UploadToken: uploadToken,
		Language:    defaultString(options.Language, defaultLanguage),
	})
	if err != nil {
		return nil, err
	}
	if err := prepare.validate(); err != nil {
		return nil, err
	}

	blocks, err := computeBlocks(upload.reader, upload.size, prepare.BlockSize)
	if err != nil {
		return nil, err
	}
	if len(blocks) != prepare.NumBlocks {
		return nil, fmt.Errorf("prepare response num_blocks=%d does not match computed blocks=%d", prepare.NumBlocks, len(blocks))
	}

	neededBlocks, err := c.getNeededUploadBlocks(ctx, prepare.UploadID, blocks, defaultString(options.Language, defaultLanguage))
	if err != nil {
		return nil, err
	}

	if err := uploadNeededBlocks(ctx, c, upload.reader, prepare.UploadID, prepare.BlockSize, neededBlocks); err != nil {
		return nil, err
	}

	if err := c.finishSpaceUpload(ctx, finishSpaceUploadRequest{
		UploadID:           prepare.UploadID,
		NumBlocks:          prepare.NumBlocks,
		VHID:               prepare.VHID,
		RiskDetectionExtra: riskDetectionExtra(),
		Language:           defaultString(options.Language, defaultLanguage),
	}); err != nil {
		return nil, err
	}

	autoTranscribe := true
	if options.AutoTranscribe != nil {
		autoTranscribe = *options.AutoTranscribe
	}
	if err := c.finishMinutesUpload(ctx, finishMinutesUploadRequest{
		AutoTranscribe: autoTranscribe,
		Language:       defaultString(options.TranscribeLanguage, "mixed"),
		NumBlocks:      prepare.NumBlocks,
		ObjectToken:    prepare.ObjectToken,
		UploadID:       prepare.UploadID,
		UploadToken:    uploadToken,
		VHID:           prepare.VHID,
	}); err != nil {
		return nil, err
	}

	return &UploadResult{
		ObjectToken: prepare.ObjectToken,
		UploadID:    prepare.UploadID,
		VHID:        prepare.VHID,
		UploadToken: uploadToken,
		NumBlocks:   prepare.NumBlocks,
	}, nil
}

func (c *Client) getUploadToken(ctx context.Context, fileInfo, language string) (string, error) {
	query := url.Values{}
	query.Add("file_info[]", fileInfo)
	query.Set("without_quota", "true")
	query.Set("language", language)

	req, err := c.newAPIRequest(ctx, http.MethodGet, "/minutes/api/quota", query, nil)
	if err != nil {
		return "", err
	}

	var data quotaResponse
	if err := c.doJSON(req, &data); err != nil {
		return "", err
	}

	token := data.UploadToken[fileInfo]
	if token == "" {
		return "", fmt.Errorf("quota response missing upload_token for %s", fileInfo)
	}

	return token, nil
}

func (c *Client) prepareUpload(ctx context.Context, payload prepareUploadRequest) (*prepareUploadResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := c.newAPIRequest(ctx, http.MethodPost, "/minutes/api/upload/prepare", nil, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")

	var data prepareUploadResponse
	if err := c.doJSON(req, &data); err != nil {
		return nil, err
	}

	return &data, nil
}

func (c *Client) getNeededUploadBlocks(ctx context.Context, uploadID string, blocks []uploadBlock, language string) ([]uploadBlock, error) {
	payload := uploadBlocksRequest{
		UploadID: uploadID,
		Blocks:   blocks,
		Language: language,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := c.newSpaceRequest(ctx, http.MethodPost, "/space/api/box/upload/blocks", nil, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")

	var data uploadBlocksResponse
	if err := c.doJSON(req, &data); err != nil {
		return nil, err
	}
	if data.NeededUploadBlocks == nil {
		return nil, errors.New("upload blocks response missing needed_upload_blocks")
	}

	return data.NeededUploadBlocks, nil
}

func (c *Client) uploadBlock(ctx context.Context, uploadID string, block uploadBlock, data []byte) error {
	query := url.Values{}
	query.Set("upload_id", uploadID)
	query.Set("seq", strconv.Itoa(block.Seq))
	query.Set("size", strconv.FormatInt(block.Size, 10))
	query.Set("checksum", block.Checksum)

	req, err := c.newSpaceRequest(ctx, http.MethodPost, "/space/api/box/stream/upload/block", query, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/octet-stream")

	return c.doJSON(req, nil)
}

func (c *Client) finishSpaceUpload(ctx context.Context, payload finishSpaceUploadRequest) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := c.newSpaceRequest(ctx, http.MethodPost, "/space/api/box/upload/finish/", nil, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")

	return c.doJSON(req, nil)
}

func (c *Client) finishMinutesUpload(ctx context.Context, payload finishMinutesUploadRequest) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := c.newAPIRequest(ctx, http.MethodPost, "/minutes/api/upload/finish", nil, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")

	return c.doJSON(req, nil)
}

type uploadSource struct {
	reader io.ReadSeeker
	size   int64
	name   string
}

func newUploadSource(options UploadOptions) (*uploadSource, error) {
	name := strings.TrimSpace(options.Name)
	size := options.Size
	reader := options.Reader

	if reader == nil {
		if options.FilePath == "" {
			return nil, errors.New("upload file path or reader is required")
		}

		file, err := os.Open(options.FilePath)
		if err != nil {
			return nil, err
		}

		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, err
		}

		reader = file
		size = info.Size()
		if name == "" {
			name = filepath.Base(options.FilePath)
		}
	}

	if reader == nil {
		return nil, errors.New("upload reader is required")
	}
	if size < 0 {
		return nil, errors.New("upload size cannot be negative")
	}
	if size == 0 {
		current, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, errors.New("upload size is required when reader cannot report size")
		}
		end, err := reader.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, errors.New("upload size is required when reader cannot report size")
		}
		if _, err := reader.Seek(current, io.SeekStart); err != nil {
			return nil, err
		}
		size = end
	}
	if name == "" {
		name = "upload"
	}

	return &uploadSource{
		reader: reader,
		size:   size,
		name:   name,
	}, nil
}

func readFileHeader(reader io.Reader) (string, error) {
	header := make([]byte, fileHeaderSize)
	n, err := io.ReadFull(reader, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(header[:n]), nil
}

func computeBlocks(reader io.ReadSeeker, size int64, blockSize int64) ([]uploadBlock, error) {
	if blockSize <= 0 {
		return nil, errors.New("prepare response missing block_size")
	}
	if err := seekStart(reader); err != nil {
		return nil, err
	}

	var blocks []uploadBlock
	buffer := make([]byte, blockSize)
	for seq := 0; ; seq++ {
		n, err := io.ReadFull(reader, buffer)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if n == 0 {
			break
		}

		data := buffer[:n]
		hash := sha256.Sum256(data)
		blocks = append(blocks, uploadBlock{
			Hash:     base64.StdEncoding.EncodeToString(hash[:]),
			Seq:      seq,
			Size:     int64(n),
			Checksum: strconv.FormatUint(uint64(adler32.Checksum(data)), 10),
		})

		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			break
		}
	}

	if size == 0 && len(blocks) == 0 {
		return []uploadBlock{}, nil
	}

	return blocks, nil
}

func uploadNeededBlocks(ctx context.Context, c *Client, reader io.ReadSeeker, uploadID string, blockSize int64, neededBlocks []uploadBlock) error {
	for _, block := range neededBlocks {
		data, err := readBlock(reader, block, blockSize)
		if err != nil {
			return err
		}
		if err := c.uploadBlock(ctx, uploadID, block, data); err != nil {
			return err
		}
	}

	return nil
}

func readBlock(reader io.ReadSeeker, block uploadBlock, blockSize int64) ([]byte, error) {
	offset := int64(block.Seq) * blockSize
	if _, err := reader.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}

	data := make([]byte, block.Size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}

	hash := sha256.Sum256(data)
	if got := base64.StdEncoding.EncodeToString(hash[:]); got != block.Hash {
		return nil, fmt.Errorf("block %d hash mismatch", block.Seq)
	}
	if got := strconv.FormatUint(uint64(adler32.Checksum(data)), 10); got != block.Checksum {
		return nil, fmt.Errorf("block %d checksum mismatch", block.Seq)
	}

	return data, nil
}

func seekStart(reader io.Seeker) error {
	_, err := reader.Seek(0, io.SeekStart)
	return err
}

func riskDetectionExtra() string {
	return `{"file_operate_usage":3,"locale":"zh_cn"}`
}

type quotaResponse struct {
	UploadToken map[string]string `json:"upload_token"`
}

type prepareUploadRequest struct {
	Name        string `json:"name"`
	FileSize    int64  `json:"file_size"`
	FileHeader  string `json:"file_header"`
	DriveUpload bool   `json:"drive_upload"`
	UploadToken string `json:"upload_token"`
	Language    string `json:"language"`
}

type prepareUploadResponse struct {
	VHID        string `json:"vhid"`
	ObjectToken string `json:"object_token"`
	UploadID    string `json:"upload_id"`
	BlockSize   int64  `json:"block_size"`
	NumBlocks   int    `json:"num_blocks"`
}

func (r prepareUploadResponse) validate() error {
	switch {
	case r.VHID == "":
		return errors.New("prepare response missing vhid")
	case r.ObjectToken == "":
		return errors.New("prepare response missing object_token")
	case r.UploadID == "":
		return errors.New("prepare response missing upload_id")
	case r.BlockSize <= 0:
		return errors.New("prepare response missing block_size")
	case r.NumBlocks < 0:
		return errors.New("prepare response invalid num_blocks")
	default:
		return nil
	}
}

type uploadBlocksRequest struct {
	UploadID string        `json:"upload_id"`
	Blocks   []uploadBlock `json:"blocks"`
	Language string        `json:"language"`
}

type uploadBlocksResponse struct {
	NeededUploadBlocks []uploadBlock `json:"needed_upload_blocks"`
}

type uploadBlock struct {
	Hash     string `json:"hash"`
	Seq      int    `json:"seq"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

type finishSpaceUploadRequest struct {
	UploadID           string `json:"upload_id"`
	NumBlocks          int    `json:"num_blocks"`
	VHID               string `json:"vhid"`
	RiskDetectionExtra string `json:"risk_detection_extra"`
	Language           string `json:"language"`
}

type finishMinutesUploadRequest struct {
	AutoTranscribe bool   `json:"auto_transcribe"`
	Language       string `json:"language"`
	NumBlocks      int    `json:"num_blocks"`
	ObjectToken    string `json:"object_token"`
	UploadID       string `json:"upload_id"`
	UploadToken    string `json:"upload_token"`
	VHID           string `json:"vhid"`
}
