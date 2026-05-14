/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

func newGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "A brief description of your command",
		Annotations: map[string]string{
			requiresConfigAnnotation: "true",
		},
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("get called")
		},
	}
}
