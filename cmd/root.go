/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var exit = os.Exit

func newRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "openminutes",
		Version: buildVersion(),
		Short:   "A brief description of your application",
		Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	}

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
