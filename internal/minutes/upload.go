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

	"go.uber.org/zap"
)

const fileHeaderSize = 512

// UploadFile uploads a local media file to Feishu Minutes.
func (c *Client) UploadFile(ctx context.Context, options UploadOptions) (*UploadResult, error) {
	upload, err := newUploadSource(options)
	if err != nil {
		c.logger.Debug("upload file source failed", zap.Error(err))
		return nil, err
	}
	if closer, ok := upload.reader.(io.Closer); ok {
		defer closer.Close()
	}
	c.logger.Debug("upload file started",
		zap.String("name", upload.name),
		zap.Int64("size", upload.size),
	)

	if err := seekStart(upload.reader); err != nil {
		c.logger.Debug("upload file seek failed",
			zap.String("name", upload.name),
			zap.Error(err),
		)
		return nil, err
	}
	fileHeader, err := readFileHeader(upload.reader)
	if err != nil {
		c.logger.Debug("upload file header read failed",
			zap.String("name", upload.name),
			zap.Error(err),
		)
		return nil, err
	}
	if err := seekStart(upload.reader); err != nil {
		c.logger.Debug("upload file seek failed",
			zap.String("name", upload.name),
			zap.Error(err),
		)
		return nil, err
	}

	fileID := strings.TrimSpace(options.FileID)
	fileIDDefaulted := fileID == ""
	if fileID == "" {
		fileID = newFileID()
	}
	fileInfo := fmt.Sprintf("%s_%d", fileID, upload.size)
	c.logger.Debug("upload file id prepared",
		zap.String("name", upload.name),
		zap.String("file_id", fileID),
		zap.Bool("file_id_defaulted", fileIDDefaulted),
		zap.Int64("size", upload.size),
	)

	uploadToken, err := c.getUploadToken(ctx, fileInfo, defaultString(options.Language, defaultLanguage))
	if err != nil {
		c.logger.Debug("upload token request failed",
			zap.String("file_id", fileID),
			zap.Int64("size", upload.size),
			zap.Error(err),
		)
		return nil, err
	}
	c.logger.Debug("upload token received",
		zap.String("file_id", fileID),
		zap.Bool("upload_token_present", uploadToken != ""),
	)

	prepare, err := c.prepareUpload(ctx, prepareUploadRequest{
		Name:        upload.name,
		FileSize:    upload.size,
		FileHeader:  fileHeader,
		DriveUpload: true,
		UploadToken: uploadToken,
		Language:    defaultString(options.Language, defaultLanguage),
	})
	if err != nil {
		c.logger.Debug("upload prepare failed",
			zap.String("name", upload.name),
			zap.String("file_id", fileID),
			zap.Error(err),
		)
		return nil, err
	}
	if err := prepare.validate(); err != nil {
		c.logger.Debug("upload prepare response invalid",
			zap.String("name", upload.name),
			zap.String("upload_id", prepare.UploadID),
			zap.String("object_token", prepare.ObjectToken),
			zap.String("vhid", prepare.VHID),
			zap.Int64("block_size", prepare.BlockSize),
			zap.Int("num_blocks", prepare.NumBlocks),
			zap.Error(err),
		)
		return nil, err
	}
	c.logger.Debug("upload prepared",
		zap.String("name", upload.name),
		zap.String("upload_id", prepare.UploadID),
		zap.String("object_token", prepare.ObjectToken),
		zap.String("vhid", prepare.VHID),
		zap.Int64("block_size", prepare.BlockSize),
		zap.Int("num_blocks", prepare.NumBlocks),
	)

	blocks, err := computeBlocks(upload.reader, upload.size, prepare.BlockSize)
	if err != nil {
		c.logger.Debug("upload block computation failed",
			zap.String("upload_id", prepare.UploadID),
			zap.Int64("block_size", prepare.BlockSize),
			zap.Error(err),
		)
		return nil, err
	}
	c.logger.Debug("upload blocks computed",
		zap.String("upload_id", prepare.UploadID),
		zap.Int64("block_size", prepare.BlockSize),
		zap.Int("num_blocks", len(blocks)),
	)
	if len(blocks) != prepare.NumBlocks {
		c.logger.Debug("upload block count mismatch",
			zap.String("upload_id", prepare.UploadID),
			zap.Int("prepared_num_blocks", prepare.NumBlocks),
			zap.Int("computed_num_blocks", len(blocks)),
		)
		return nil, fmt.Errorf("prepare response num_blocks=%d does not match computed blocks=%d", prepare.NumBlocks, len(blocks))
	}

	neededBlocks, err := c.getNeededUploadBlocks(ctx, prepare.UploadID, blocks, defaultString(options.Language, defaultLanguage))
	if err != nil {
		c.logger.Debug("upload needed blocks request failed",
			zap.String("upload_id", prepare.UploadID),
			zap.Error(err),
		)
		return nil, err
	}
	c.logger.Debug("upload needed blocks received",
		zap.String("upload_id", prepare.UploadID),
		zap.Int("needed_blocks", len(neededBlocks)),
	)

	if err := uploadNeededBlocks(ctx, c, upload.reader, prepare.UploadID, prepare.BlockSize, neededBlocks); err != nil {
		c.logger.Debug("upload needed blocks failed",
			zap.String("upload_id", prepare.UploadID),
			zap.Error(err),
		)
		return nil, err
	}
	c.logger.Debug("upload needed blocks completed", zap.String("upload_id", prepare.UploadID))

	if err := c.finishSpaceUpload(ctx, finishSpaceUploadRequest{
		UploadID:           prepare.UploadID,
		NumBlocks:          prepare.NumBlocks,
		VHID:               prepare.VHID,
		RiskDetectionExtra: riskDetectionExtra(),
		Language:           defaultString(options.Language, defaultLanguage),
	}); err != nil {
		c.logger.Debug("upload space finish failed",
			zap.String("upload_id", prepare.UploadID),
			zap.String("vhid", prepare.VHID),
			zap.Error(err),
		)
		return nil, err
	}
	c.logger.Debug("upload space finish completed",
		zap.String("upload_id", prepare.UploadID),
		zap.String("vhid", prepare.VHID),
	)

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
		c.logger.Debug("upload minutes finish failed",
			zap.String("upload_id", prepare.UploadID),
			zap.String("object_token", prepare.ObjectToken),
			zap.String("vhid", prepare.VHID),
			zap.Bool("auto_transcribe", autoTranscribe),
			zap.Error(err),
		)
		return nil, err
	}
	c.logger.Debug("upload minutes finish completed",
		zap.String("upload_id", prepare.UploadID),
		zap.String("object_token", prepare.ObjectToken),
		zap.String("vhid", prepare.VHID),
		zap.Bool("auto_transcribe", autoTranscribe),
	)

	result := &UploadResult{
		ObjectToken: prepare.ObjectToken,
		UploadID:    prepare.UploadID,
		VHID:        prepare.VHID,
		UploadToken: uploadToken,
		NumBlocks:   prepare.NumBlocks,
	}
	c.logger.Debug("upload file completed",
		zap.String("name", upload.name),
		zap.String("object_token", result.ObjectToken),
		zap.String("upload_id", result.UploadID),
		zap.String("vhid", result.VHID),
		zap.Int("num_blocks", result.NumBlocks),
		zap.Bool("upload_token_present", result.UploadToken != ""),
	)
	return result, nil
}

func (c *Client) getUploadToken(ctx context.Context, fileInfo, language string) (string, error) {
	c.logger.Debug("upload token request started",
		zap.String("file_info", fileInfo),
		zap.String("language", language),
	)
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
		c.logger.Debug("upload token request failed",
			zap.String("file_info", fileInfo),
			zap.Error(err),
		)
		return "", err
	}

	token := data.UploadToken[fileInfo]
	if token == "" {
		c.logger.Debug("upload token response invalid",
			zap.String("file_info", fileInfo),
			zap.String("reason", "missing_upload_token"),
		)
		return "", fmt.Errorf("quota response missing upload_token for %s", fileInfo)
	}

	c.logger.Debug("upload token request completed",
		zap.String("file_info", fileInfo),
		zap.Bool("upload_token_present", true),
	)
	return token, nil
}

func (c *Client) prepareUpload(ctx context.Context, payload prepareUploadRequest) (*prepareUploadResponse, error) {
	c.logger.Debug("upload prepare request started",
		zap.String("name", payload.Name),
		zap.Int64("file_size", payload.FileSize),
		zap.Bool("drive_upload", payload.DriveUpload),
		zap.Bool("upload_token_present", payload.UploadToken != ""),
	)
	body, err := json.Marshal(payload)
	if err != nil {
		c.logger.Debug("upload prepare payload marshal failed",
			zap.String("name", payload.Name),
			zap.Error(err),
		)
		return nil, err
	}

	req, err := c.newAPIRequest(ctx, http.MethodPost, "/minutes/api/upload/prepare", nil, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")

	var data prepareUploadResponse
	if err := c.doJSON(req, &data); err != nil {
		c.logger.Debug("upload prepare request failed",
			zap.String("name", payload.Name),
			zap.Error(err),
		)
		return nil, err
	}

	c.logger.Debug("upload prepare request completed",
		zap.String("name", payload.Name),
		zap.String("upload_id", data.UploadID),
		zap.String("object_token", data.ObjectToken),
		zap.String("vhid", data.VHID),
		zap.Int64("block_size", data.BlockSize),
		zap.Int("num_blocks", data.NumBlocks),
	)
	return &data, nil
}

func (c *Client) getNeededUploadBlocks(ctx context.Context, uploadID string, blocks []uploadBlock, language string) ([]uploadBlock, error) {
	c.logger.Debug("upload blocks request started",
		zap.String("upload_id", uploadID),
		zap.Int("num_blocks", len(blocks)),
		zap.String("language", language),
	)
	payload := uploadBlocksRequest{
		UploadID: uploadID,
		Blocks:   blocks,
		Language: language,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		c.logger.Debug("upload blocks payload marshal failed",
			zap.String("upload_id", uploadID),
			zap.Error(err),
		)
		return nil, err
	}

	req, err := c.newSpaceRequest(ctx, http.MethodPost, "/space/api/box/upload/blocks", nil, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")

	var data uploadBlocksResponse
	if err := c.doJSON(req, &data); err != nil {
		c.logger.Debug("upload blocks request failed",
			zap.String("upload_id", uploadID),
			zap.Error(err),
		)
		return nil, err
	}
	if data.NeededUploadBlocks == nil {
		c.logger.Debug("upload blocks response invalid",
			zap.String("upload_id", uploadID),
			zap.String("reason", "missing_needed_upload_blocks"),
		)
		return nil, errors.New("upload blocks response missing needed_upload_blocks")
	}

	c.logger.Debug("upload blocks request completed",
		zap.String("upload_id", uploadID),
		zap.Int("needed_blocks", len(data.NeededUploadBlocks)),
	)
	return data.NeededUploadBlocks, nil
}

func (c *Client) uploadBlock(ctx context.Context, uploadID string, block uploadBlock, data []byte) error {
	c.logger.Debug("upload block request started",
		zap.String("upload_id", uploadID),
		zap.Int("seq", block.Seq),
		zap.Int64("size", block.Size),
		zap.String("checksum", block.Checksum),
		zap.Bool("hash_present", block.Hash != ""),
	)
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

	if err := c.doJSON(req, nil); err != nil {
		c.logger.Debug("upload block request failed",
			zap.String("upload_id", uploadID),
			zap.Int("seq", block.Seq),
			zap.Error(err),
		)
		return err
	}
	c.logger.Debug("upload block request completed",
		zap.String("upload_id", uploadID),
		zap.Int("seq", block.Seq),
		zap.Int64("size", block.Size),
	)
	return nil
}

func (c *Client) finishSpaceUpload(ctx context.Context, payload finishSpaceUploadRequest) error {
	c.logger.Debug("upload space finish request started",
		zap.String("upload_id", payload.UploadID),
		zap.Int("num_blocks", payload.NumBlocks),
		zap.String("vhid", payload.VHID),
	)
	body, err := json.Marshal(payload)
	if err != nil {
		c.logger.Debug("upload space finish payload marshal failed",
			zap.String("upload_id", payload.UploadID),
			zap.Error(err),
		)
		return err
	}

	req, err := c.newSpaceRequest(ctx, http.MethodPost, "/space/api/box/upload/finish/", nil, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")

	if err := c.doJSON(req, nil); err != nil {
		c.logger.Debug("upload space finish request failed",
			zap.String("upload_id", payload.UploadID),
			zap.String("vhid", payload.VHID),
			zap.Error(err),
		)
		return err
	}
	c.logger.Debug("upload space finish request completed",
		zap.String("upload_id", payload.UploadID),
		zap.String("vhid", payload.VHID),
	)
	return nil
}

func (c *Client) finishMinutesUpload(ctx context.Context, payload finishMinutesUploadRequest) error {
	c.logger.Debug("upload minutes finish request started",
		zap.String("upload_id", payload.UploadID),
		zap.String("object_token", payload.ObjectToken),
		zap.String("vhid", payload.VHID),
		zap.Int("num_blocks", payload.NumBlocks),
		zap.Bool("upload_token_present", payload.UploadToken != ""),
		zap.Bool("auto_transcribe", payload.AutoTranscribe),
	)
	body, err := json.Marshal(payload)
	if err != nil {
		c.logger.Debug("upload minutes finish payload marshal failed",
			zap.String("upload_id", payload.UploadID),
			zap.String("object_token", payload.ObjectToken),
			zap.Error(err),
		)
		return err
	}

	req, err := c.newAPIRequest(ctx, http.MethodPost, "/minutes/api/upload/finish", nil, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")

	if err := c.doJSON(req, nil); err != nil {
		c.logger.Debug("upload minutes finish request failed",
			zap.String("upload_id", payload.UploadID),
			zap.String("object_token", payload.ObjectToken),
			zap.Error(err),
		)
		return err
	}
	c.logger.Debug("upload minutes finish request completed",
		zap.String("upload_id", payload.UploadID),
		zap.String("object_token", payload.ObjectToken),
		zap.String("vhid", payload.VHID),
	)
	return nil
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
			c.logger.Debug("upload block verification failed",
				zap.String("upload_id", uploadID),
				zap.Int("seq", block.Seq),
				zap.Int64("size", block.Size),
				zap.String("checksum", block.Checksum),
				zap.Bool("hash_present", block.Hash != ""),
				zap.Error(err),
			)
			return err
		}
		c.logger.Debug("upload block verified",
			zap.String("upload_id", uploadID),
			zap.Int("seq", block.Seq),
			zap.Int64("size", block.Size),
			zap.String("checksum", block.Checksum),
			zap.Bool("hash_present", block.Hash != ""),
		)
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
