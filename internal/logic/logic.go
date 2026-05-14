package logic

import (
	"context"
	"strings"

	"openminutes/internal/config"
	apperrors "openminutes/internal/errors"
	"openminutes/internal/media"
	"openminutes/internal/minutes"

	"go.uber.org/zap"
)

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
		return nil, apperrors.New(apperrors.KindRemote, "upload result is empty")
	}

	return result, nil
}

func ValidateUploadFile(filePath string, logger *zap.Logger) error {
	return media.ValidateUploadFile(filePath, logger)
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
