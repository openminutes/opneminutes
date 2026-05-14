/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var exit = os.Exit

func newRootCommand() *cobra.Command {
	configPath := defaultConfigFlagValue
	verbose := false

	rootCmd := &cobra.Command{
		Use:          "openminutes",
		Version:      buildVersion(),
		Short:        "A brief description of your application",
		SilenceUsage: true,
		Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
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
			config, err := loadConfigWithLogger(configPathForLoad, logger)
			if err != nil {
				logger.Debug("config load failed", zap.Error(err))
				return err
			}

			cmd.SetContext(contextWithConfig(cmd.Context(), config))
			logger.Debug("config stored in command context",
				zap.String("region", config.Region),
				zap.Bool("cookie_present", config.Cookie != ""),
			)
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", defaultConfigFlagValue, "config file path")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose debug logging")
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
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
