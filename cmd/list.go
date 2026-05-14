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

type listMinutesClient interface {
	ListMinutes(context.Context, minutes.ListOptions) ([]minutes.Minute, error)
}

var newListMinutesClient = func(config minutes.Config) (listMinutesClient, error) {
	return minutes.NewClient(config)
}

func newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Feishu minutes",
		Annotations: map[string]string{
			requiresConfigAnnotation: "true",
		},
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		RunE: runListCommand,
	}
}

func runListCommand(cmd *cobra.Command, args []string) error {
	logger := loggerFromCommand(cmd)
	logger.Debug("list command started")

	config, ok := configFromCommand(cmd)
	if !ok {
		logger.Debug("list command missing config")
		return errors.New("config is required")
	}

	clientConfig := minutes.Config{
		Region: config.Region,
		Cookie: config.Cookie,
	}
	if logger, ok := loggerFromContext(cmd.Context()); ok {
		clientConfig.Logger = logger
	}

	client, err := newListMinutesClient(clientConfig)
	if err != nil {
		logger.Debug("list client initialization failed", zap.Error(err))
		return err
	}

	items, err := client.ListMinutes(cmd.Context(), minutes.ListOptions{})
	if err != nil {
		logger.Debug("list minutes failed", zap.Error(err))
		return err
	}

	if len(items) == 0 {
		logger.Debug("list command completed", zap.Int("count", 0))
		cmd.Println("No minutes found.")
		return nil
	}

	for _, item := range items {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", item.ObjectToken, listTopic(item.Topic), listURL(item)); err != nil {
			return err
		}
	}

	logger.Debug("list command completed", zap.Int("count", len(items)))
	return nil
}

func listTopic(topic string) string {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "(untitled)"
	}

	return topic
}

func listURL(item minutes.Minute) string {
	rawURL := strings.TrimSpace(item.URL)
	if rawURL != "" {
		return rawURL
	}

	return "https://meetings.feishu.cn/minutes/" + item.ObjectToken
}
