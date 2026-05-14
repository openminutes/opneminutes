package minutes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"go.uber.org/zap"
)

// ListMinutes lists all available minutes, following server pagination.
func (c *Client) ListMinutes(ctx context.Context, options ListOptions) ([]Minute, error) {
	var all []Minute
	timestamp := options.Timestamp
	page := 0
	c.logger.Debug("list minutes started",
		zap.Int64("initial_timestamp", timestamp),
		zap.Int("size", options.Size),
		zap.String("language", defaultString(options.Language, defaultLanguage)),
	)

	for {
		query := listQuery(options, timestamp)
		c.logger.Debug("list minutes page requested",
			zap.Int("page", page),
			zap.Int64("timestamp", timestamp),
		)
		req, err := c.newAPIRequest(ctx, http.MethodGet, "/minutes/api/space/list", query, nil)
		if err != nil {
			c.logger.Debug("list minutes request creation failed",
				zap.Int("page", page),
				zap.Int64("timestamp", timestamp),
				zap.Error(err),
			)
			return nil, err
		}

		var data listResponse
		if err := c.doJSON(req, &data); err != nil {
			c.logger.Debug("list minutes page failed",
				zap.Int("page", page),
				zap.Int64("timestamp", timestamp),
				zap.Error(err),
			)
			return nil, err
		}
		if data.List == nil {
			c.logger.Debug("list minutes page invalid",
				zap.Int("page", page),
				zap.Int64("timestamp", timestamp),
				zap.String("reason", "missing_list"),
			)
			return nil, errors.New("minutes list response missing list")
		}

		all = append(all, data.List...)
		nextTimestamp := int64(0)
		if len(data.List) > 0 {
			nextTimestamp = data.List[len(data.List)-1].ShareTime
		}
		c.logger.Debug("list minutes page received",
			zap.Int("page", page),
			zap.Int64("timestamp", timestamp),
			zap.Int("count", len(data.List)),
			zap.Bool("has_more", data.HasMore),
			zap.Int64("next_timestamp", nextTimestamp),
			zap.Int("total", len(all)),
		)
		if !data.HasMore || len(data.List) == 0 {
			c.logger.Debug("list minutes completed", zap.Int("total", len(all)))
			return all, nil
		}

		if nextTimestamp == 0 {
			c.logger.Debug("list minutes pagination failed",
				zap.Int("page", page),
				zap.Int64("timestamp", timestamp),
				zap.String("reason", "missing_next_timestamp"),
			)
			return nil, errors.New("minutes list response missing next page share_time")
		}
		if nextTimestamp == timestamp {
			c.logger.Debug("list minutes pagination failed",
				zap.Int("page", page),
				zap.Int64("timestamp", timestamp),
				zap.Int64("next_timestamp", nextTimestamp),
				zap.String("reason", "pagination_not_advanced"),
			)
			return nil, fmt.Errorf("minutes list pagination did not advance from timestamp %d", timestamp)
		}
		timestamp = nextTimestamp
		page++
	}
}

// ExportSubtitle exports subtitle content for a minute.
func (c *Client) ExportSubtitle(ctx context.Context, objectToken string, options SubtitleOptions) ([]byte, error) {
	if objectToken == "" {
		return nil, errors.New("object token is required")
	}
	c.logger.Debug("export subtitle started",
		zap.String("object_token", objectToken),
		zap.String("format", options.Format),
		zap.Bool("add_speaker", options.AddSpeaker),
		zap.Bool("add_timestamp", options.AddTimestamp),
		zap.String("language", defaultString(options.Language, defaultLanguage)),
	)

	query := url.Values{}
	query.Set("object_token", objectToken)
	query.Set("format", strconv.Itoa(subtitleFormat(options.Format)))
	query.Set("add_speaker", strconv.FormatBool(options.AddSpeaker))
	query.Set("add_timestamp", strconv.FormatBool(options.AddTimestamp))
	query.Set("language", defaultString(options.Language, defaultLanguage))

	req, err := c.newAPIRequest(ctx, http.MethodPost, "/minutes/api/export", query, nil)
	if err != nil {
		return nil, err
	}

	data, err := c.doRaw(req)
	if err != nil {
		c.logger.Debug("export subtitle failed",
			zap.String("object_token", objectToken),
			zap.Error(err),
		)
		return nil, err
	}
	c.logger.Debug("export subtitle completed",
		zap.String("object_token", objectToken),
		zap.Int("bytes", len(data)),
	)
	return data, nil
}

// GetStatus returns a minute status.
func (c *Client) GetStatus(ctx context.Context, objectToken string) (*MinuteStatus, error) {
	if objectToken == "" {
		return nil, errors.New("object token is required")
	}
	c.logger.Debug("get minute status started", zap.String("object_token", objectToken))

	query := url.Values{}
	query.Set("object_token", objectToken)
	query.Set("language", defaultLanguage)
	query.Set("_t", strconv.FormatInt(nowMillis(), 10))

	req, err := c.newAPIRequest(ctx, http.MethodGet, "/minutes/api/status", query, nil)
	if err != nil {
		return nil, err
	}

	var data statusResponse
	if err := c.doJSON(req, &data); err != nil {
		c.logger.Debug("get minute status failed",
			zap.String("object_token", objectToken),
			zap.Error(err),
		)
		return nil, err
	}

	c.logger.Debug("get minute status completed",
		zap.String("object_token", objectToken),
		zap.Int("object_status", data.ObjectStatus),
		zap.Bool("download_url_present", data.VideoInfo.VideoDownloadURL != ""),
	)
	return &data.MinuteStatus, nil
}

// GetDownloadURL returns a minute video download URL.
func (c *Client) GetDownloadURL(ctx context.Context, objectToken string) (string, error) {
	c.logger.Debug("get download url started", zap.String("object_token", objectToken))
	status, err := c.GetStatus(ctx, objectToken)
	if err != nil {
		c.logger.Debug("get download url failed",
			zap.String("object_token", objectToken),
			zap.Error(err),
		)
		return "", err
	}

	if status.VideoInfo.VideoDownloadURL == "" {
		c.logger.Debug("get download url failed",
			zap.String("object_token", objectToken),
			zap.String("reason", "missing_download_url"),
		)
		return "", errors.New("status response missing video_info.video_download_url")
	}

	c.logger.Debug("get download url completed",
		zap.String("object_token", objectToken),
		zap.Bool("download_url_present", true),
	)
	return status.VideoInfo.VideoDownloadURL, nil
}

// DownloadFile streams a minute video into dst.
func (c *Client) DownloadFile(ctx context.Context, objectToken string, dst io.Writer) error {
	if dst == nil {
		return errors.New("destination writer is required")
	}
	c.logger.Debug("download file started", zap.String("object_token", objectToken))

	downloadURL, err := c.GetDownloadURL(ctx, objectToken)
	if err != nil {
		c.logger.Debug("download file failed",
			zap.String("object_token", objectToken),
			zap.Error(err),
		)
		return err
	}
	c.logger.Debug("download file url resolved",
		zap.String("object_token", objectToken),
		zap.Bool("download_url_present", downloadURL != ""),
	)

	req, err := c.newRequest(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		c.logger.Debug("download file request creation failed",
			zap.String("object_token", objectToken),
			zap.Error(err),
		)
		return err
	}

	c.logger.Debug("download file stream started", zap.String("object_token", objectToken))
	if err := c.doStream(req, dst); err != nil {
		c.logger.Debug("download file stream failed",
			zap.String("object_token", objectToken),
			zap.Error(err),
		)
		return err
	}
	c.logger.Debug("download file completed", zap.String("object_token", objectToken))
	return nil
}

type listResponse struct {
	Timestamp string   `json:"timestamp"`
	Size      int      `json:"size"`
	HasMore   bool     `json:"has_more"`
	List      []Minute `json:"list"`
}

type statusResponse struct {
	MinuteStatus
}

func listQuery(options ListOptions, timestamp int64) url.Values {
	query := url.Values{}
	size := options.Size
	if size == 0 {
		size = 20
	}
	spaceName := options.SpaceName
	if spaceName == 0 {
		spaceName = 1
	}
	rank := options.Rank
	if rank == 0 {
		rank = 1
	}
	ownerType := options.OwnerType
	if ownerType == 0 {
		ownerType = 1
	}
	asc := false
	if options.Asc != nil {
		asc = *options.Asc
	}
	noteInfo := true
	if options.NoteInfo != nil {
		noteInfo = *options.NoteInfo
	}

	query.Set("size", strconv.Itoa(size))
	query.Set("space_name", strconv.Itoa(spaceName))
	query.Set("rank", strconv.Itoa(rank))
	query.Set("asc", strconv.FormatBool(asc))
	query.Set("note_info", strconv.FormatBool(noteInfo))
	query.Set("owner_type", strconv.Itoa(ownerType))
	query.Set("language", defaultString(options.Language, defaultLanguage))
	if timestamp > 0 {
		query.Set("timestamp", strconv.FormatInt(timestamp, 10))
	}

	return query
}

func subtitleFormat(format string) int {
	switch format {
	case "txt":
		return 2
	case "", "srt":
		return 3
	default:
		return 3
	}
}
