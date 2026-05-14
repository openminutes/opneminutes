/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"openminutes/internal/minutes"

	"github.com/spf13/cobra"
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

type uploadMinutesClient interface {
	UploadFile(context.Context, minutes.UploadOptions) (*minutes.UploadResult, error)
}

var newUploadMinutesClient = func(config minutes.Config) (uploadMinutesClient, error) {
	return minutes.NewClient(config)
}

func newUploadCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "upload FILE",
		Short:        "Upload media to Minutes for transcription",
		Args:         validateUploadArgs,
		SilenceUsage: true,
		Annotations: map[string]string{
			requiresConfigAnnotation: "true",
		},
		Long: `Upload media to Minutes for transcription.

Upload one local audio or video file for Minutes transcription. The file
extension, size, and duration are validated before upload. The result is a new
Minute URL/token in the current account.`,
		Example: `  openminutes upload ./meeting.mp4`,
		RunE:    runUploadCommand,
	}
}

func validateUploadArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return errors.New("file path is required")
	}
	if len(args) > 1 {
		return errors.New("upload accepts exactly one file path")
	}
	if strings.TrimSpace(args[0]) == "" {
		return errors.New("file path is required")
	}

	return nil
}

func runUploadCommand(cmd *cobra.Command, args []string) error {
	if err := validateUploadArgs(cmd, args); err != nil {
		return err
	}

	logger := loggerFromCommand(cmd)
	logger.Debug("upload command started", zap.String("path", args[0]))

	config, ok := configFromCommand(cmd)
	if !ok {
		logger.Debug("upload command missing config")
		return errors.New("config is required")
	}

	filePath := args[0]
	if err := validateUploadFile(filePath, logger); err != nil {
		logger.Debug("upload file validation failed",
			zap.String("path", filePath),
			zap.Error(err),
		)
		return err
	}

	clientConfig := minutes.Config{
		BaseURL:      config.BaseURL,
		SpaceBaseURL: config.SpaceBaseURL,
		Cookie:       config.Cookie,
	}
	if logger, ok := loggerFromContext(cmd.Context()); ok {
		clientConfig.Logger = logger
	}

	client, err := newUploadMinutesClient(clientConfig)
	if err != nil {
		logger.Debug("upload client initialization failed", zap.Error(err))
		return err
	}

	result, err := client.UploadFile(cmd.Context(), minutes.UploadOptions{FilePath: filePath})
	if err != nil {
		logger.Debug("upload file failed",
			zap.String("path", filePath),
			zap.Error(err),
		)
		return err
	}
	if result == nil {
		return errors.New("upload result is empty")
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Uploaded %s %s\n", result.ObjectToken, uploadMinutesURL(config.BaseURL, result.ObjectToken)); err != nil {
		return err
	}

	logger.Debug("upload command completed",
		zap.String("path", filePath),
		zap.String("object_token", result.ObjectToken),
	)
	return nil
}

func validateUploadFile(filePath string, logger *zap.Logger) error {
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

func uploadMinutesURL(baseURL, objectToken string) string {
	return configBaseURLOrDefault(baseURL, defaultBaseURL) + "/minutes/" + objectToken
}
