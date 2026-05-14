/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
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
		Short:        "Export text from a Minute",
		Args:         validateGetArgs,
		SilenceUsage: true,
		Annotations: map[string]string{
			requiresConfigAnnotation: "true",
		},
		Long: `Export text from a Minute.

Export one Minute as txt or srt. Speaker names and timestamps can be included
with flags. Exported content is printed to stdout by default. Use --output or
-O to write to a file instead. Output files are created exclusively and existing
files are never overwritten. Use --json for structured output metadata and,
when --output is not used, inline exported content.`,
		Example: `  openminutes get m_abc123
  openminutes get m_abc123 --json
  openminutes get m_abc123 --file_type srt --speaker --timestamp -O meeting.srt`,
		RunE: runGetCommand,
	}
	cmd.Flags().String("file_type", "txt", "export format: txt or srt")
	cmd.Flags().Bool("speaker", false, "include speaker names in the exported text")
	cmd.Flags().Bool("timestamp", false, "include timestamps in the exported text")
	cmd.Flags().StringP("output", "O", "", "write exported content to this file path")
	cmd.Flags().Bool("json", false, "print structured JSON instead of plain text")

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
	if outputPath != "" {
		if err := ensureGetOutputDoesNotExist(outputPath); err != nil {
			return err
		}
	}

	speaker, err := cmd.Flags().GetBool("speaker")
	if err != nil {
		return err
	}
	timestamp, err := cmd.Flags().GetBool("timestamp")
	if err != nil {
		return err
	}
	jsonOutput, err := cmd.Flags().GetBool("json")
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

	if outputPath == "" {
		if jsonOutput {
			content := string(data)
			if err := writeGetJSON(cmd, getJSONOutput{
				ObjectToken: token,
				FileType:    fileType,
				Speaker:     speaker,
				Timestamp:   timestamp,
				Bytes:       len(data),
				Content:     &content,
			}); err != nil {
				logger.Debug("write subtitle JSON stdout failed",
					zap.String("object_token", token),
					zap.Error(err),
				)
				return err
			}

			logger.Debug("get command completed",
				zap.String("object_token", token),
				zap.String("output", "stdout"),
				zap.String("file_type", fileType),
				zap.Int("bytes", len(data)),
			)
			return nil
		}

		if err := writeGetStdout(cmd.OutOrStdout(), data); err != nil {
			logger.Debug("write subtitle stdout failed",
				zap.String("object_token", token),
				zap.Error(err),
			)
			return err
		}

		logger.Debug("get command completed",
			zap.String("object_token", token),
			zap.String("output", "stdout"),
			zap.String("file_type", fileType),
			zap.Int("bytes", len(data)),
		)
		return nil
	}

	if err := writeGetOutputFile(outputPath, data); err != nil {
		logger.Debug("write subtitle output failed",
			zap.String("path", outputPath),
			zap.Error(err),
		)
		return err
	}

	if jsonOutput {
		if err := writeGetJSON(cmd, getJSONOutput{
			ObjectToken: token,
			FileType:    fileType,
			Speaker:     speaker,
			Timestamp:   timestamp,
			Bytes:       len(data),
			OutputPath:  outputPath,
		}); err != nil {
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

type getJSONOutput struct {
	ObjectToken string  `json:"object_token"`
	FileType    string  `json:"file_type"`
	Speaker     bool    `json:"speaker"`
	Timestamp   bool    `json:"timestamp"`
	Bytes       int     `json:"bytes"`
	Content     *string `json:"content,omitempty"`
	OutputPath  string  `json:"output_path,omitempty"`
}

func writeGetJSON(cmd *cobra.Command, output getJSONOutput) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")

	return encoder.Encode(output)
}

func getOutputPath(cmd *cobra.Command, _ string, _ string) (string, error) {
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

	return "", nil
}

func writeGetStdout(stdout io.Writer, data []byte) error {
	n, err := stdout.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}

	if len(data) > 0 && data[len(data)-1] == '\n' {
		return nil
	}

	n, err = stdout.Write([]byte("\n"))
	if err != nil {
		return err
	}
	if n != 1 {
		return io.ErrShortWrite
	}

	return nil
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
