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
		Short:        "Delete Minutes from the current account",
		Args:         validateDeleteArgs,
		SilenceUsage: true,
		Annotations: map[string]string{
			requiresConfigAnnotation:       "true",
			requiresConfirmationAnnotation: "true",
		},
		Long: `Delete Minutes from the current account.

Tokens are removed from the authenticated account. By default, each Minute is
moved to trash. Use --destroy to permanently delete a Minute after moving it to
trash. The command always requires --yes to avoid accidental deletion.`,
		Example: `  openminutes delete m_abc123 --yes
  openminutes delete m_abc123 m_def456 --yes --destroy`,
		RunE: runDeleteCommand,
	}
	cmd.Flags().Bool("yes", false, "confirm deletion without prompting")
	cmd.Flags().Bool("destroy", false, "permanently delete each Minute after moving it to trash")

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

	runtime, err := runtimeFromCommand(cmd)
	if err != nil {
		logger.Debug("delete command missing config")
		return err
	}

	client, err := newDeleteMinutesClient(runtime.ClientConfig)
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
