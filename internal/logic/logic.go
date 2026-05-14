package logic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"openminutes/internal/config"
	"openminutes/internal/minutes"

	"go.uber.org/zap"
)

const (
	maxUploadFileSize = 6 * 1024 * 1024 * 1024
	maxUploadDuration = 6 * time.Hour
)

var supportedUploadExtensions = map[string]struct{}{
	".wav":  {},
	".mp3":  {},
	".m4a":  {},
	".aac":  {},
	".ogg":  {},
	".wma":  {},
	".amr":  {},
	".avi":  {},
	".wmv":  {},
	".mov":  {},
	".mp4":  {},
	".m4v":  {},
	".mpeg": {},
	".flv":  {},
}

type ListClient interface {
	ListMinutesPage(context.Context, minutes.ListOptions) (*minutes.ListMinutesPageResult, error)
	ListMinutes(context.Context, minutes.ListOptions) ([]minutes.Minute, error)
}

type GetClient interface {
	ExportSubtitle(context.Context, string, minutes.SubtitleOptions) ([]byte, error)
}

type DeleteClient interface {
	DeleteMinute(context.Context, string, minutes.DeleteOptions) error
}

type UploadClient interface {
	UploadFile(context.Context, minutes.UploadOptions) (*minutes.UploadResult, error)
}

func ListMinutes(ctx context.Context, client ListClient, options minutes.ListOptions, all bool) (*minutes.ListMinutesPageResult, error) {
	if !all {
		return client.ListMinutesPage(ctx, options)
	}

	items, err := client.ListMinutes(ctx, options)
	if err != nil {
		return nil, err
	}

	return &minutes.ListMinutesPageResult{Items: items}, nil
}

func ExportSubtitle(ctx context.Context, client GetClient, token string, options minutes.SubtitleOptions) ([]byte, error) {
	return client.ExportSubtitle(ctx, token, options)
}

func DeleteMinutes(ctx context.Context, client DeleteClient, tokens []string, options minutes.DeleteOptions) error {
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if err := client.DeleteMinute(ctx, token, options); err != nil {
			return err
		}
	}

	return nil
}

func UploadFile(ctx context.Context, client UploadClient, filePath string, logger *zap.Logger) (*minutes.UploadResult, error) {
	if err := ValidateUploadFile(filePath, logger); err != nil {
		return nil, err
	}

	result, err := client.UploadFile(ctx, minutes.UploadOptions{FilePath: filePath})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("upload result is empty")
	}

	return result, nil
}

func ValidateUploadFile(filePath string, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("file %q does not exist", filePath)
		}
		return fmt.Errorf("stat file %q: %w", filePath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("file %q is a directory", filePath)
	}

	extension := strings.ToLower(filepath.Ext(filePath))
	if _, ok := supportedUploadExtensions[extension]; !ok {
		return fmt.Errorf("unsupported file extension %q", extension)
	}

	if info.Size() > maxUploadFileSize {
		return fmt.Errorf("file size %d exceeds maximum %d bytes", info.Size(), int64(maxUploadFileSize))
	}

	duration, known, err := probeUploadDuration(filePath, extension)
	if err != nil {
		logger.Debug("upload duration probe failed",
			zap.String("path", filePath),
			zap.String("extension", extension),
			zap.Error(err),
		)
		return nil
	}
	if !known {
		logger.Debug("upload duration probe skipped",
			zap.String("path", filePath),
			zap.String("extension", extension),
		)
		return nil
	}
	if duration > maxUploadDuration {
		return fmt.Errorf("file duration %s exceeds maximum %s", duration, maxUploadDuration)
	}

	logger.Debug("upload duration probe completed",
		zap.String("path", filePath),
		zap.String("extension", extension),
		zap.Duration("duration", duration),
	)
	return nil
}

func MinuteURL(baseURL, objectToken string) string {
	baseURL, _, err := minutes.NormalizeBaseURLOrDefault("base_url", baseURL, config.DefaultBaseURL)
	if err != nil {
		baseURL = config.DefaultBaseURL
	}
	return baseURL + "/minutes/" + objectToken
}

func MinuteTopic(topic string) string {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "(untitled)"
	}

	return topic
}
