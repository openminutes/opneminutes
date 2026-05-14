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
		return apperrors.New(apperrors.KindValidation, "at least one token is required")
	}
	for _, token := range args {
		if strings.TrimSpace(token) == "" {
			return apperrors.New(apperrors.KindValidation, "object token is required")
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
		return apperrors.New(apperrors.KindConfirmation, "delete requires --yes")
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
	clientWithOutput := &deleteOutputClient{
		client:  client,
		destroy: destroy,
		out:     cmd.OutOrStdout(),
	}
	if err := logic.DeleteMinutes(cmd.Context(), clientWithOutput, args, options); err != nil {
		logger.Debug("delete minute failed",
			zap.String("object_token", clientWithOutput.currentToken),
			zap.Error(err),
		)
		return err
	}

	logger.Debug("delete command completed",
		zap.Int("tokens", len(args)),
		zap.Bool("destroy", destroy),
	)
	return nil
}

type deleteOutputClient struct {
	client  deleteMinutesClient
	destroy bool
	out     interface {
		Write([]byte) (int, error)
	}
	currentToken string
}

func (c *deleteOutputClient) DeleteMinute(ctx context.Context, token string, options minutes.DeleteOptions) error {
	c.currentToken = token
	if err := c.client.DeleteMinute(ctx, token, options); err != nil {
		return err
	}

	if c.destroy {
		_, err := fmt.Fprintf(c.out, "Permanently deleted %s\n", token)
		return apperrors.Wrap(apperrors.KindFileSystem, err)
	}

	_, err := fmt.Fprintf(c.out, "Moved %s to trash\n", token)
	return apperrors.Wrap(apperrors.KindFileSystem, err)
}
