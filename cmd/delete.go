/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"openminutes/internal/minutes"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type deleteMinutesClient interface {
	DeleteMinute(context.Context, string, minutes.DeleteOptions) error
}

var newDeleteMinutesClient = func(config minutes.Config) (deleteMinutesClient, error) {
	return minutes.NewClient(config)
}

func newDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "delete TOKEN...",
		Short:        "Delete Feishu minutes",
		Args:         validateDeleteArgs,
		SilenceUsage: true,
		Annotations: map[string]string{
			requiresConfigAnnotation:       "true",
			requiresConfirmationAnnotation: "true",
		},
		RunE: runDeleteCommand,
	}
	cmd.Flags().Bool("yes", false, "confirm deletion")
	cmd.Flags().Bool("destroy", false, "permanently delete minutes after moving them to trash")

	return cmd
}

func validateDeleteArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return errors.New("at least one token is required")
	}
	for _, token := range args {
		if strings.TrimSpace(token) == "" {
			return errors.New("object token is required")
		}
	}

	return nil
}

func runDeleteCommand(cmd *cobra.Command, args []string) error {
	logger := loggerFromCommand(cmd)
	logger.Debug("delete command started", zap.Int("tokens", len(args)))

	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}
	if !yes {
		logger.Debug("delete command missing confirmation")
		return errors.New("delete requires --yes")
	}

	destroy, err := cmd.Flags().GetBool("destroy")
	if err != nil {
		return err
	}

	config, ok := configFromCommand(cmd)
	if !ok {
		logger.Debug("delete command missing config")
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

	client, err := newDeleteMinutesClient(clientConfig)
	if err != nil {
		logger.Debug("delete client initialization failed", zap.Error(err))
		return err
	}

	options := minutes.DeleteOptions{Destroy: destroy}
	for _, token := range args {
		token = strings.TrimSpace(token)
		if err := client.DeleteMinute(cmd.Context(), token, options); err != nil {
			logger.Debug("delete minute failed",
				zap.String("object_token", token),
				zap.Error(err),
			)
			return err
		}
		if destroy {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Permanently deleted %s\n", token); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Moved %s to trash\n", token); err != nil {
			return err
		}
	}

	logger.Debug("delete command completed",
		zap.Int("tokens", len(args)),
		zap.Bool("destroy", destroy),
	)
	return nil
}
