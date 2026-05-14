/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"openminutes/internal/minutes"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type getMinutesClient interface {
	ExportSubtitle(context.Context, string, minutes.SubtitleOptions) ([]byte, error)
}

type getOutputFile interface {
	Write([]byte) (int, error)
	Close() error
}

var newGetMinutesClient = func(config minutes.Config) (getMinutesClient, error) {
	return minutes.NewClient(config)
}

var openGetOutputFile = func(outputPath string) (getOutputFile, error) {
	return os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}

func newGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "get TOKEN",
		Short:        "Export Feishu minutes subtitles",
		Args:         validateGetArgs,
		SilenceUsage: true,
		Annotations: map[string]string{
			requiresConfigAnnotation: "true",
		},
		Long: `Export subtitle text from a Feishu Minutes object token.

The output file is created exclusively and existing files are never overwritten.`,
		RunE: runGetCommand,
	}
	cmd.Flags().String("file_type", "txt", "subtitle file type: txt or srt")
	cmd.Flags().Bool("speaker", false, "include speaker names")
	cmd.Flags().Bool("timestamp", false, "include timestamps")
	cmd.Flags().String("output", "", "output file path")

	return cmd
}

func validateGetArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return errors.New("object token is required")
	}
	if len(args) > 1 {
		return errors.New("get accepts exactly one token")
	}
	if strings.TrimSpace(args[0]) == "" {
		return errors.New("object token is required")
	}

	return nil
}

func runGetCommand(cmd *cobra.Command, args []string) error {
	if err := validateGetArgs(cmd, args); err != nil {
		return err
	}

	token := strings.TrimSpace(args[0])
	logger := loggerFromCommand(cmd)
	logger.Debug("get command started", zap.String("object_token", token))

	fileType, err := cmd.Flags().GetString("file_type")
	if err != nil {
		return err
	}
	fileType = strings.ToLower(strings.TrimSpace(fileType))
	switch fileType {
	case "txt", "srt":
	default:
		return fmt.Errorf("unsupported file_type %q: must be txt or srt", fileType)
	}

	outputPath, err := getOutputPath(cmd, token, fileType)
	if err != nil {
		return err
	}
	if err := ensureGetOutputDoesNotExist(outputPath); err != nil {
		return err
	}

	speaker, err := cmd.Flags().GetBool("speaker")
	if err != nil {
		return err
	}
	timestamp, err := cmd.Flags().GetBool("timestamp")
	if err != nil {
		return err
	}

	config, ok := configFromCommand(cmd)
	if !ok {
		logger.Debug("get command missing config")
		return errors.New("config is required")
	}

	clientConfig := minutes.Config{
		BaseURL:      config.BaseURL,
		SpaceBaseURL: config.SpaceBaseURL,
		Cookie:       config.Cookie,
	}
	if logger, ok := loggerFromContext(cmd.Context()); ok {
		clientConfig.Logger = logger
	}

	client, err := newGetMinutesClient(clientConfig)
	if err != nil {
		logger.Debug("get client initialization failed", zap.Error(err))
		return err
	}

	data, err := client.ExportSubtitle(cmd.Context(), token, minutes.SubtitleOptions{
		Format:       fileType,
		AddSpeaker:   speaker,
		AddTimestamp: timestamp,
	})
	if err != nil {
		logger.Debug("export subtitle failed",
			zap.String("object_token", token),
			zap.Error(err),
		)
		return err
	}

	if err := writeGetOutputFile(outputPath, data); err != nil {
		logger.Debug("write subtitle output failed",
			zap.String("path", outputPath),
			zap.Error(err),
		)
		return err
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Saved %s to %s\n", token, outputPath); err != nil {
		return err
	}

	logger.Debug("get command completed",
		zap.String("object_token", token),
		zap.String("path", outputPath),
		zap.String("file_type", fileType),
		zap.Int("bytes", len(data)),
	)
	return nil
}

func getOutputPath(cmd *cobra.Command, token, fileType string) (string, error) {
	outputPath, err := cmd.Flags().GetString("output")
	if err != nil {
		return "", err
	}

	outputFlag := cmd.Flags().Lookup("output")
	if outputFlag != nil && outputFlag.Changed {
		if strings.TrimSpace(outputPath) == "" {
			return "", errors.New("output path is required")
		}
		return outputPath, nil
	}

	return token + "." + fileType, nil
}

func ensureGetOutputDoesNotExist(outputPath string) error {
	if _, err := os.Stat(outputPath); err == nil {
		return fmt.Errorf("output file %q already exists", outputPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat output file %q: %w", outputPath, err)
	}

	return nil
}

func writeGetOutputFile(outputPath string, data []byte) (err error) {
	file, err := openGetOutputFile(outputPath)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := file.Close()
		if err != nil {
			_ = os.Remove(outputPath)
			return
		}
		if closeErr != nil {
			_ = os.Remove(outputPath)
			err = closeErr
		}
	}()

	n, err := file.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}

	return nil
}
