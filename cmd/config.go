package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const configTemplate = "region = \"\"\ncookie = \"\"\n"
const defaultConfigFlagValue = "~/.config/openminutes/config.toml"
const requiresConfigAnnotation = "openminutes.requires_config"

type Config struct {
	Region string
	Cookie string
}

type configContextKey struct{}

var defaultConfigPathFunc = defaultConfigPath

func defaultConfigPath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil && homeDir != "" {
			configDir = filepath.Join(homeDir, ".config")
		}
	}
	if configDir == "" {
		userConfigDir, err := os.UserConfigDir()
		if err == nil && userConfigDir != "" {
			configDir = userConfigDir
		}
	}
	if configDir == "" {
		configDir = ".config"
	}

	return filepath.Join(configDir, "openminutes", "config.toml")
}

func loadConfig(configPath string) (Config, error) {
	configPath = normalizeConfigPath(configPath)
	if err := ensureConfigFile(configPath); err != nil {
		return Config{}, err
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")
	v.SetEnvPrefix("OPENMINUTES")
	v.AllowEmptyEnv(true)
	v.AutomaticEnv()

	for _, key := range []string{"region", "cookie"} {
		if err := v.BindEnv(key); err != nil {
			return Config{}, err
		}
	}

	if err := v.ReadInConfig(); err != nil {
		return Config{}, err
	}

	config := Config{
		Region: strings.TrimSpace(v.GetString("region")),
		Cookie: v.GetString("cookie"),
	}
	if err := validateConfig(config); err != nil {
		return Config{}, err
	}

	return config, nil
}

func normalizeConfigPath(configPath string) string {
	if configPath == "" {
		return defaultConfigPathFunc()
	}

	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return configPath
	}

	if configPath == "~" {
		return homeDir
	}
	if strings.HasPrefix(configPath, "~/") {
		return filepath.Join(homeDir, strings.TrimPrefix(configPath, "~/"))
	}

	return configPath
}

func ensureConfigFile(configPath string) error {
	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return err
	}

	return os.WriteFile(configPath, []byte(configTemplate), 0o600)
}

func validateConfig(config Config) error {
	if config.Region != "" && config.Region != "feishu" && config.Region != "larksuite" {
		return fmt.Errorf("invalid region %q: must be one of feishu, larksuite", config.Region)
	}

	if strings.TrimSpace(config.Cookie) == "" {
		return errors.New("cookie is required")
	}

	if config.Region == "" {
		return fmt.Errorf("invalid region %q: must be one of feishu, larksuite", config.Region)
	}

	return nil
}

func contextWithConfig(ctx context.Context, config Config) context.Context {
	return context.WithValue(ctx, configContextKey{}, config)
}

func configFromCommand(cmd *cobra.Command) (Config, bool) {
	config, ok := cmd.Context().Value(configContextKey{}).(Config)
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
