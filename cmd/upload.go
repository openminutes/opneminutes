package cmd

import (
	"context"
	"fmt"
	"strings"

	apperrors "openminutes/internal/errors"
	"openminutes/internal/logic"
	"openminutes/internal/minutes"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

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
		return apperrors.New(apperrors.KindValidation, "file path is required")
	}
	if len(args) > 1 {
		return apperrors.New(apperrors.KindValidation, "upload accepts exactly one file path")
	}
	if strings.TrimSpace(args[0]) == "" {
		return apperrors.New(apperrors.KindValidation, "file path is required")
	}

	return nil
}

func runUploadCommand(cmd *cobra.Command, args []string) error {
	if err := validateUploadArgs(cmd, args); err != nil {
		return err
	}

	logger := loggerFromCommand(cmd)
	logger.Debug("upload command started", zap.String("path", args[0]))

	runtime, err := runtimeFromCommand(cmd)
	if err != nil {
		logger.Debug("upload command missing config")
		return err
	}

	filePath := args[0]
	client, err := newUploadMinutesClient(runtime.ClientConfig)
	if err != nil {
		logger.Debug("upload client initialization failed", zap.Error(err))
		return err
	}

	result, err := logic.UploadFile(cmd.Context(), client, filePath, logger)
	if err != nil {
		logger.Debug("upload file failed",
			zap.String("path", filePath),
			zap.Error(err),
		)
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Uploaded %s %s\n", result.ObjectToken, logic.MinuteURL(runtime.Config.BaseURL, result.ObjectToken)); err != nil {
		return apperrors.Wrap(apperrors.KindFileSystem, err)
	}

	logger.Debug("upload command completed",
		zap.String("path", filePath),
		zap.String("object_token", result.ObjectToken),
	)
	return nil
}
