package minutes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"openminutes/internal/media"

	"go.uber.org/zap"
)

var jsonMarshal = json.Marshal

// UploadFile uploads a local media file to Feishu Minutes.
func (c *Client) UploadFile(ctx context.Context, options UploadOptions) (*UploadResult, error) {
	upload, err := c.openUploadSource(options)
	if err != nil {
		return nil, err
	}
	if closer, ok := upload.Reader.(io.Closer); ok {
		defer closer.Close()
	}

	fileHeader, err := c.prepareUploadSource(upload)
	if err != nil {
		return nil, err
	}

	language := defaultString(options.Language, defaultLanguage)
	session, err := c.prepareUploadSession(ctx, upload, fileHeader, options, language)
	if err != nil {
		return nil, err
	}

	blocks, err := c.planUploadBlocks(upload, session)
	if err != nil {
		return nil, err
	}

	if err := c.transferUploadBlocks(ctx, upload, session, blocks, language); err != nil {
		return nil, err
	}

	if err := c.finalizeUpload(ctx, session, options, language); err != nil {
		return nil, err
	}

	result := session.result()
	c.logger.Debug("upload file completed",
		zap.String("name", upload.Name),
		zap.String("object_token", result.ObjectToken),
		zap.String("upload_id", result.UploadID),
		zap.String("vhid", result.VHID),
		zap.Int("num_blocks", result.NumBlocks),
		zap.Bool("upload_token_present", result.UploadToken != ""),
	)
	return result, nil
}

func (c *Client) openUploadSource(options UploadOptions) (*media.Source, error) {
	upload, err := media.OpenSource(media.SourceOptions{
		FilePath: options.FilePath,
		Reader:   options.Reader,
		Size:     options.Size,
		Name:     options.Name,
	})
	if err != nil {
		c.logger.Debug("upload file source failed", zap.Error(err))
		return nil, err
	}

	c.logger.Debug("upload file started",
		zap.String("name", upload.Name),
		zap.Int64("size", upload.Size),
	)
	return upload, nil
}

func (c *Client) prepareUploadSource(upload *media.Source) (string, error) {
	if err := media.SeekStart(upload.Reader); err != nil {
		c.logger.Debug("upload file seek failed",
			zap.String("name", upload.Name),
			zap.Error(err),
		)
		return "", err
	}

	fileHeader, err := media.ReadHeader(upload.Reader)
	if err != nil {
		c.logger.Debug("upload file header read failed",
			zap.String("name", upload.Name),
			zap.Error(err),
		)
		return "", err
	}

	if err := media.SeekStart(upload.Reader); err != nil {
		c.logger.Debug("upload file seek failed",
			zap.String("name", upload.Name),
			zap.Error(err),
		)
		return "", err
	}

	return fileHeader, nil
}

func (c *Client) prepareUploadSession(ctx context.Context, upload *media.Source, fileHeader string, options UploadOptions, language string) (*uploadSession, error) {
	fileID := strings.TrimSpace(options.FileID)
	fileIDDefaulted := fileID == ""
	if fileID == "" {
		fileID = newFileID()
	}
	fileInfo := fmt.Sprintf("%s_%d", fileID, upload.Size)
	c.logger.Debug("upload file id prepared",
		zap.String("name", upload.Name),
		zap.String("file_id", fileID),
		zap.Bool("file_id_defaulted", fileIDDefaulted),
		zap.Int64("size", upload.Size),
	)

	uploadToken, err := c.getUploadToken(ctx, fileInfo, language)
	if err != nil {
		c.logger.Debug("upload token request failed",
			zap.String("file_id", fileID),
			zap.Int64("size", upload.Size),
			zap.Error(err),
		)
		return nil, err
	}
	c.logger.Debug("upload token received",
		zap.String("file_id", fileID),
		zap.Bool("upload_token_present", uploadToken != ""),
	)

	prepare, err := c.prepareUpload(ctx, prepareUploadRequest{
		Name:        upload.Name,
		FileSize:    upload.Size,
		FileHeader:  fileHeader,
		DriveUpload: true,
		UploadToken: uploadToken,
		Language:    language,
	})
	if err != nil {
		c.logger.Debug("upload prepare failed",
			zap.String("name", upload.Name),
			zap.String("file_id", fileID),
			zap.Error(err),
		)
		return nil, err
	}
	if err := prepare.validate(); err != nil {
		c.logger.Debug("upload prepare response invalid",
			zap.String("name", upload.Name),
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
		zap.String("name", upload.Name),
		zap.String("upload_id", prepare.UploadID),
		zap.String("object_token", prepare.ObjectToken),
		zap.String("vhid", prepare.VHID),
		zap.Int64("block_size", prepare.BlockSize),
		zap.Int("num_blocks", prepare.NumBlocks),
	)

	return &uploadSession{
		uploadToken: uploadToken,
		prepare:     prepare,
	}, nil
}

func (c *Client) planUploadBlocks(upload *media.Source, session *uploadSession) ([]media.Block, error) {
	blocks, err := media.ComputeBlocks(upload.Reader, upload.Size, session.prepare.BlockSize)
	if err != nil {
		c.logger.Debug("upload block computation failed",
			zap.String("upload_id", session.prepare.UploadID),
			zap.Int64("block_size", session.prepare.BlockSize),
			zap.Error(err),
		)
		return nil, err
	}
	c.logger.Debug("upload blocks computed",
		zap.String("upload_id", session.prepare.UploadID),
		zap.Int64("block_size", session.prepare.BlockSize),
		zap.Int("num_blocks", len(blocks)),
	)
	if len(blocks) != session.prepare.NumBlocks {
		c.logger.Debug("upload block count mismatch",
			zap.String("upload_id", session.prepare.UploadID),
			zap.Int("prepared_num_blocks", session.prepare.NumBlocks),
			zap.Int("computed_num_blocks", len(blocks)),
		)
		return nil, fmt.Errorf("prepare response num_blocks=%d does not match computed blocks=%d", session.prepare.NumBlocks, len(blocks))
	}

	return blocks, nil
}

func (c *Client) transferUploadBlocks(ctx context.Context, upload *media.Source, session *uploadSession, blocks []media.Block, language string) error {
	neededBlocks, err := c.getNeededUploadBlocks(ctx, session.prepare.UploadID, blocks, language)
	if err != nil {
		c.logger.Debug("upload needed blocks request failed",
			zap.String("upload_id", session.prepare.UploadID),
			zap.Error(err),
		)
		return err
	}
	c.logger.Debug("upload needed blocks received",
		zap.String("upload_id", session.prepare.UploadID),
		zap.Int("needed_blocks", len(neededBlocks)),
	)

	if err := uploadNeededBlocks(ctx, c, upload.Reader, session.prepare.UploadID, session.prepare.BlockSize, neededBlocks); err != nil {
		c.logger.Debug("upload needed blocks failed",
			zap.String("upload_id", session.prepare.UploadID),
			zap.Error(err),
		)
		return err
	}
	c.logger.Debug("upload needed blocks completed", zap.String("upload_id", session.prepare.UploadID))
	return nil
}

func (c *Client) finalizeUpload(ctx context.Context, session *uploadSession, options UploadOptions, language string) error {
	if err := c.finishSpaceUpload(ctx, finishSpaceUploadRequest{
		UploadID:           session.prepare.UploadID,
		NumBlocks:          session.prepare.NumBlocks,
		VHID:               session.prepare.VHID,
		RiskDetectionExtra: riskDetectionExtra(),
		Language:           language,
	}); err != nil {
		c.logger.Debug("upload space finish failed",
			zap.String("upload_id", session.prepare.UploadID),
			zap.String("vhid", session.prepare.VHID),
			zap.Error(err),
		)
		return err
	}
	c.logger.Debug("upload space finish completed",
		zap.String("upload_id", session.prepare.UploadID),
		zap.String("vhid", session.prepare.VHID),
	)

	autoTranscribe := true
	if options.AutoTranscribe != nil {
		autoTranscribe = *options.AutoTranscribe
	}
	if err := c.finishMinutesUpload(ctx, finishMinutesUploadRequest{
		AutoTranscribe: autoTranscribe,
		Language:       defaultString(options.TranscribeLanguage, "mixed"),
		NumBlocks:      session.prepare.NumBlocks,
		ObjectToken:    session.prepare.ObjectToken,
		UploadID:       session.prepare.UploadID,
		UploadToken:    session.uploadToken,
		VHID:           session.prepare.VHID,
	}); err != nil {
		c.logger.Debug("upload minutes finish failed",
			zap.String("upload_id", session.prepare.UploadID),
			zap.String("object_token", session.prepare.ObjectToken),
			zap.String("vhid", session.prepare.VHID),
			zap.Bool("auto_transcribe", autoTranscribe),
			zap.Error(err),
		)
		return err
	}
	c.logger.Debug("upload minutes finish completed",
		zap.String("upload_id", session.prepare.UploadID),
		zap.String("object_token", session.prepare.ObjectToken),
		zap.String("vhid", session.prepare.VHID),
		zap.Bool("auto_transcribe", autoTranscribe),
	)

	return nil
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
	body, err := jsonMarshal(payload)
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

func (c *Client) getNeededUploadBlocks(ctx context.Context, uploadID string, blocks []media.Block, language string) ([]media.Block, error) {
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
	body, err := jsonMarshal(payload)
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

func (c *Client) uploadBlock(ctx context.Context, uploadID string, block media.Block, data []byte) error {
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
	body, err := jsonMarshal(payload)
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
	body, err := jsonMarshal(payload)
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

type uploadSession struct {
	uploadToken string
	prepare     *prepareUploadResponse
}

func (s *uploadSession) result() *UploadResult {
	return &UploadResult{
		ObjectToken: s.prepare.ObjectToken,
		UploadID:    s.prepare.UploadID,
		VHID:        s.prepare.VHID,
		UploadToken: s.uploadToken,
		NumBlocks:   s.prepare.NumBlocks,
	}
}

func uploadNeededBlocks(ctx context.Context, c *Client, reader io.ReadSeeker, uploadID string, blockSize int64, neededBlocks []media.Block) error {
	for _, block := range neededBlocks {
		data, err := media.ReadBlock(reader, block, blockSize)
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
	Blocks   []media.Block `json:"blocks"`
	Language string        `json:"language"`
}

type uploadBlocksResponse struct {
	NeededUploadBlocks []media.Block `json:"needed_upload_blocks"`
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
