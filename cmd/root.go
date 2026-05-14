/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"openminutes/internal/config"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var exit = os.Exit

func newRootCommand() *cobra.Command {
	configPath := config.DefaultPathFlagValue
	verbose := false

	rootCmd := &cobra.Command{
		Use:          "openminutes",
		Version:      buildVersion(),
		Short:        "A simple CLI for Feishu/Lark Minutes",
		SilenceUsage: true,
		Long:         `OpenMinutes is an easy-to-use CLI for managing Feishu/Lark Minutes.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			logger := zap.NewNop()
			if verbose {
				logger = newVerboseLogger(cmd.ErrOrStderr())
			}
			cmd.SetContext(contextWithLogger(cmd.Context(), logger))

			requiresConfig := commandRequiresConfig(cmd)
			logger.Debug("command started",
				zap.String("command", cmd.CommandPath()),
				zap.Bool("verbose", verbose),
				zap.Bool("requires_config", requiresConfig),
			)

			if !requiresConfig {
				return nil
			}
			if commandRequiresConfirmation(cmd) {
				confirmed, err := cmd.Flags().GetBool("yes")
				if err != nil {
					return err
				}
				if !confirmed {
					logger.Debug("config load skipped",
						zap.String("reason", "missing_confirmation"),
					)
					return nil
				}
			}

			configPathForLoad := configPath
			configPathSource := "flag"
			configFlag := cmd.Flag("config")
			if configFlag == nil || !configFlag.Changed {
				configPathForLoad = ""
				configPathSource = "default"
			}

			logger.Debug("config load requested",
				zap.String("path_source", configPathSource),
				zap.Bool("flag_changed", configFlag != nil && configFlag.Changed),
			)
			config, err := config.Load(configPathForLoad, logger)
			if err != nil {
				logger.Debug("config load failed", zap.Error(err))
				return err
			}

			cmd.SetContext(contextWithConfig(cmd.Context(), config))
			logger.Debug("config stored in command context",
				zap.String("base_url", config.BaseURL),
				zap.String("space_base_url", config.SpaceBaseURL),
				zap.Bool("cookie_present", config.Cookie != ""),
			)
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", config.DefaultPathFlagValue, "config file path")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose debug logging")
	rootCmd.AddCommand(newDeleteCommand())
	rootCmd.AddCommand(newGetCommand())
	rootCmd.AddCommand(newListCommand())
	rootCmd.AddCommand(newUploadCommand())

	return rootCmd
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main().
func Execute() {
	execute(newRootCommand())
}

func execute(cmd *cobra.Command) {
	if err := cmd.Execute(); err != nil {
		exit(1)
	}
}
