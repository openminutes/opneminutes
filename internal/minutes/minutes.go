package minutes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// ListMinutes lists all available minutes, following server pagination.
func (c *Client) ListMinutes(ctx context.Context, options ListOptions) ([]Minute, error) {
	var all []Minute
	timestamp := options.Timestamp

	for {
		query := listQuery(options, timestamp)
		req, err := c.newAPIRequest(ctx, http.MethodGet, "/minutes/api/space/list", query, nil)
		if err != nil {
			return nil, err
		}

		var data listResponse
		if err := c.doJSON(req, &data); err != nil {
			return nil, err
		}
		if data.List == nil {
			return nil, errors.New("minutes list response missing list")
		}

		all = append(all, data.List...)
		if !data.HasMore || len(data.List) == 0 {
			return all, nil
		}

		nextTimestamp := data.List[len(data.List)-1].ShareTime
		if nextTimestamp == 0 {
			return nil, errors.New("minutes list response missing next page share_time")
		}
		if nextTimestamp == timestamp {
			return nil, fmt.Errorf("minutes list pagination did not advance from timestamp %d", timestamp)
		}
		timestamp = nextTimestamp
	}
}

// ExportSubtitle exports subtitle content for a minute.
func (c *Client) ExportSubtitle(ctx context.Context, objectToken string, options SubtitleOptions) ([]byte, error) {
	if objectToken == "" {
		return nil, errors.New("object token is required")
	}

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

	return c.doRaw(req)
}

// GetStatus returns a minute status.
func (c *Client) GetStatus(ctx context.Context, objectToken string) (*MinuteStatus, error) {
	if objectToken == "" {
		return nil, errors.New("object token is required")
	}

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
		return nil, err
	}

	return &data.MinuteStatus, nil
}

// GetDownloadURL returns a minute video download URL.
func (c *Client) GetDownloadURL(ctx context.Context, objectToken string) (string, error) {
	status, err := c.GetStatus(ctx, objectToken)
	if err != nil {
		return "", err
	}

	if status.VideoInfo.VideoDownloadURL == "" {
		return "", errors.New("status response missing video_info.video_download_url")
	}

	return status.VideoInfo.VideoDownloadURL, nil
}

// DownloadFile streams a minute video into dst.
func (c *Client) DownloadFile(ctx context.Context, objectToken string, dst io.Writer) error {
	if dst == nil {
		return errors.New("destination writer is required")
	}

	downloadURL, err := c.GetDownloadURL(ctx, objectToken)
	if err != nil {
		return err
	}

	req, err := c.newRequest(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	return c.doStream(req, dst)
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
