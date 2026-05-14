/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"openminutes/internal/minutes"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type listMinutesClient interface {
	ListMinutesPage(context.Context, minutes.ListOptions) (*minutes.ListMinutesPageResult, error)
}

var newListMinutesClient = func(config minutes.Config) (listMinutesClient, error) {
	return minutes.NewClient(config)
}

func newListCommand() *cobra.Command {
	var size int
	var timestamp int64
	var jsonOutput bool

	cmd := &cobra.Command{
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
	cmd.Flags().IntVar(&size, "size", 20, "number of minutes to request")
	cmd.Flags().Int64Var(&timestamp, "timestamp", 0, "pagination timestamp")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output result as JSON")

	return cmd
}

func runListCommand(cmd *cobra.Command, args []string) error {
	logger := loggerFromCommand(cmd)
	logger.Debug("list command started")

	config, ok := configFromCommand(cmd)
	if !ok {
		logger.Debug("list command missing config")
		return errors.New("config is required")
	}

	size, err := cmd.Flags().GetInt("size")
	if err != nil {
		return err
	}
	if size <= 0 {
		return errors.New("size must be greater than 0")
	}
	timestamp, err := cmd.Flags().GetInt64("timestamp")
	if err != nil {
		return err
	}
	if timestamp < 0 {
		return errors.New("timestamp must be greater than or equal to 0")
	}

	clientConfig := minutes.Config{
		BaseURL:      config.BaseURL,
		SpaceBaseURL: config.SpaceBaseURL,
		Cookie:       config.Cookie,
	}
	if logger, ok := loggerFromContext(cmd.Context()); ok {
		clientConfig.Logger = logger
	}

	client, err := newListMinutesClient(clientConfig)
	if err != nil {
		logger.Debug("list client initialization failed", zap.Error(err))
		return err
	}

	result, err := client.ListMinutesPage(cmd.Context(), minutes.ListOptions{
		Size:      size,
		Timestamp: timestamp,
	})
	if err != nil {
		logger.Debug("list minutes failed", zap.Error(err))
		return err
	}

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}
	if jsonOutput {
		if err := writeListJSON(cmd, result); err != nil {
			return err
		}
		logger.Debug("list command completed",
			zap.Int("count", len(result.Items)),
			zap.Bool("has_more", result.HasMore),
			zap.Int64("next_timestamp", result.NextTimestamp),
		)
		return nil
	}

	items := result.Items
	if len(items) == 0 {
		logger.Debug("list command completed", zap.Int("count", 0))
		cmd.Println("No minutes found.")
		return nil
	}

	for _, item := range items {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", item.ObjectToken, listTopic(item.Topic), listURL(item, config.BaseURL)); err != nil {
			return err
		}
	}
	if result.HasMore {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Next page: openminutes list --size %d --timestamp %d\n", size, result.NextTimestamp); err != nil {
			return err
		}
	}

	logger.Debug("list command completed",
		zap.Int("count", len(items)),
		zap.Bool("has_more", result.HasMore),
		zap.Int64("next_timestamp", result.NextTimestamp),
	)
	return nil
}

type listJSONOutput struct {
	Items         []minutes.Minute `json:"items"`
	HasMore       bool             `json:"has_more"`
	NextTimestamp int64            `json:"next_timestamp"`
}

func writeListJSON(cmd *cobra.Command, result *minutes.ListMinutesPageResult) error {
	items := result.Items
	if items == nil {
		items = []minutes.Minute{}
	}

	output := listJSONOutput{
		Items:         items,
		HasMore:       result.HasMore,
		NextTimestamp: result.NextTimestamp,
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")

	return encoder.Encode(output)
}

func listTopic(topic string) string {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "(untitled)"
	}

	return topic
}

func listURL(item minutes.Minute, baseURL string) string {
	rawURL := strings.TrimSpace(item.URL)
	if rawURL != "" {
		return rawURL
	}

	baseURL = configBaseURLOrDefault(baseURL, defaultBaseURL)
	return baseURL + "/minutes/" + item.ObjectToken
}
