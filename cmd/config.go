package cmd

import (
	"context"

	"openminutes/internal/config"

	"github.com/spf13/cobra"
)

const (
	requiresConfigAnnotation       = "openminutes.requires_config"
	requiresConfirmationAnnotation = "openminutes.requires_confirmation"
)

type configContextKey struct{}

func contextWithConfig(ctx context.Context, config config.Config) context.Context {
	return context.WithValue(ctx, configContextKey{}, config)
}

func configFromCommand(cmd *cobra.Command) (config.Config, bool) {
	config, ok := cmd.Context().Value(configContextKey{}).(config.Config)
	return config, ok
}

func commandRequiresConfig(cmd *cobra.Command) bool {
	for current := cmd; current != nil; current = current.Parent() {
		if current.Annotations[requiresConfigAnnotation] == "true" {
			return true
		}
	}

	return false
}

func commandRequiresConfirmation(cmd *cobra.Command) bool {
	for current := cmd; current != nil; current = current.Parent() {
		if current.Annotations[requiresConfirmationAnnotation] == "true" {
			return true
		}
	}

	return false
}
