package minutes

import (
	"io"
	"net/http"

	"go.uber.org/zap"
)

// Config configures a Feishu Minutes HTTP client.
type Config struct {
	Cookie       string
	HTTPClient   *http.Client
	BaseURL      string
	SpaceBaseURL string
	UserAgent    string
	Logger       *zap.Logger
}

// Client provides low-level Feishu Minutes HTTP operations.
type Client struct {
	httpClient   *http.Client
	baseURL      string
	spaceBaseURL string
	cookie       string
	csrfToken    string
	userAgent    string
	referer      string
	logger       *zap.Logger
}

// ListOptions controls listing minutes from the minutes space.
type ListOptions struct {
	Size      int
	SpaceName int
	Rank      int
	Asc       *bool
	NoteInfo  *bool
	OwnerType int
	Language  string
	Timestamp int64
}

// ListMinutesPageResult contains one page of minutes from the list API.
type ListMinutesPageResult struct {
	Items         []Minute
	HasMore       bool
	NextTimestamp int64
}

// DeleteOptions controls deleting a minute from the minutes space.
type DeleteOptions struct {
	Language  string
	SpaceName int
	Destroy   bool
}

// Minute is a Feishu Minutes list item.
type Minute struct {
	ObjectToken string `json:"object_token"`
	ObjectType  int    `json:"object_type"`
	Topic       string `json:"topic"`
	URL         string `json:"url"`
	MediaType   string `json:"media_type"`
	OwnerName   string `json:"owner_name"`
	Duration    int64  `json:"duration"`
	ShareTime   int64  `json:"share_time"`
	StartTime   int64  `json:"start_time"`
	StopTime    int64  `json:"stop_time"`
	CreateTime  int64  `json:"create_time"`
	Status      int    `json:"status"`
}

// SubtitleOptions controls the export subtitle request.
type SubtitleOptions struct {
	Format       string
	AddSpeaker   bool
	AddTimestamp bool
	Language     string
}

// MinuteStatus is the status payload returned by /minutes/api/status.
type MinuteStatus struct {
	ObjectToken        string             `json:"object_token"`
	ObjectStatus       int                `json:"object_status"`
	TranscriptProgress TranscriptProgress `json:"transcript_progress"`
	VideoInfo          VideoInfo          `json:"video_info"`
}

// TranscriptProgress describes transcription progress for uploaded minutes.
type TranscriptProgress struct {
	Current string `json:"current"`
	Total   string `json:"total"`
}

// VideoInfo contains video download metadata.
type VideoInfo struct {
	VideoDownloadURL string `json:"video_download_url"`
}

// UploadOptions controls a file upload.
type UploadOptions struct {
	FilePath           string
	Reader             io.ReadSeeker
	Size               int64
	Name               string
	FileID             string
	Language           string
	TranscribeLanguage string
	AutoTranscribe     *bool
}

// UploadResult contains identifiers created by an upload.
type UploadResult struct {
	ObjectToken string
	UploadID    string
	VHID        string
	UploadToken string
	NumBlocks   int
}
