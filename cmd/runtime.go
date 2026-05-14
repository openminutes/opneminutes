package cmd

import (
	"errors"

	"openminutes/internal/config"
	"openminutes/internal/minutes"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type commandRuntime struct {
	Config       config.Config
	Logger       *zap.Logger
	ClientConfig minutes.Config
}

func runtimeFromCommand(cmd *cobra.Command) (commandRuntime, error) {
	logger := loggerFromCommand(cmd)
	config, ok := configFromCommand(cmd)
	if !ok {
		return commandRuntime{Logger: logger}, errors.New("config is required")
	}

	clientConfig := minutes.Config{
		BaseURL:      config.BaseURL,
		SpaceBaseURL: config.SpaceBaseURL,
		Cookie:       config.Cookie,
	}
	if logger, ok := loggerFromContext(cmd.Context()); ok {
		clientConfig.Logger = logger
	}

	return commandRuntime{
		Config:       config,
		Logger:       logger,
		ClientConfig: clientConfig,
	}, nil
}
