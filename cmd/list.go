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
	ListMinutes(context.Context, minutes.ListOptions) ([]minutes.Minute, error)
}

var newListMinutesClient = func(config minutes.Config) (listMinutesClient, error) {
	return minutes.NewClient(config)
}

func newListCommand() *cobra.Command {
	var size int
	var timestamp int64
	var jsonOutput bool
	var all bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Minutes from the current account",
		Annotations: map[string]string{
			requiresConfigAnnotation: "true",
		},
		Long: `List Minutes from the current account.

By default, list requests one page and prints a Next page command when more
results are available. Pass the printed timestamp with --timestamp to continue
from the next page. Use --json for structured output. Use --all to follow
pagination and list all Minutes, starting from --timestamp when provided.`,
		Example: `  openminutes list
  openminutes list --size 50 --timestamp 1710000000
  openminutes list --all --json`,
		RunE: runListCommand,
	}
	cmd.Flags().IntVar(&size, "size", 20, "number of Minutes to request per page")
	cmd.Flags().Int64Var(&timestamp, "timestamp", 0, "pagination timestamp to start from")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print structured JSON instead of plain rows")
	cmd.Flags().BoolVar(&all, "all", false, "follow pagination and list all Minutes")

	return cmd
}

func runListCommand(cmd *cobra.Command, args []string) error {
	logger := loggerFromCommand(cmd)
	logger.Debug("list command started")

	runtime, err := runtimeFromCommand(cmd)
	if err != nil {
		logger.Debug("list command missing config")
		return err
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
	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}

	client, err := newListMinutesClient(runtime.ClientConfig)
	if err != nil {
		logger.Debug("list client initialization failed", zap.Error(err))
		return err
	}

	options := minutes.ListOptions{
		Size:      size,
		Timestamp: timestamp,
	}
	result, err := listMinutes(cmd.Context(), client, options, all)
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

	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintln(out, "Columns: token name URL"); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(out, "%s %s %s\n", item.ObjectToken, listTopic(item.Topic), listURL(item, runtime.Config.BaseURL)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if result.HasMore {
		if _, err := fmt.Fprintf(out, "Next page: openminutes list --size %d --timestamp %d\n", size, result.NextTimestamp); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(out, "Get content: openminutes get <token>"); err != nil {
		return err
	}

	logger.Debug("list command completed",
		zap.Int("count", len(items)),
		zap.Bool("has_more", result.HasMore),
		zap.Int64("next_timestamp", result.NextTimestamp),
	)
	return nil
}

func listMinutes(ctx context.Context, client listMinutesClient, options minutes.ListOptions, all bool) (*minutes.ListMinutesPageResult, error) {
	if !all {
		return client.ListMinutesPage(ctx, options)
	}

	items, err := client.ListMinutes(ctx, options)
	if err != nil {
		return nil, err
	}

	return &minutes.ListMinutesPageResult{Items: items}, nil
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

	baseURL, _, err := minutes.NormalizeBaseURLOrDefault("base_url", baseURL, defaultBaseURL)
	if err != nil {
		baseURL = defaultBaseURL
	}
	return baseURL + "/minutes/" + item.ObjectToken
}
