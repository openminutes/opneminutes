package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const (
	defaultBaseURL      = "https://meetings.feishu.cn"
	defaultSpaceBaseURL = "https://internal-api-space.feishu.cn"

	configTemplate = "base_url = \"https://meetings.feishu.cn\"\nspace_base_url = \"https://internal-api-space.feishu.cn\"\ncookie = \"\"\n"

	defaultConfigFlagValue         = "~/.config/openminutes/config.toml"
	requiresConfigAnnotation       = "openminutes.requires_config"
	requiresConfirmationAnnotation = "openminutes.requires_confirmation"
)

type Config struct {
	BaseURL      string
	SpaceBaseURL string
	Cookie       string
}

type configContextKey struct{}

var (
	defaultConfigPathFunc = defaultConfigPath
	osUserHomeDir         = os.UserHomeDir
	osUserConfigDir       = os.UserConfigDir
	osStat                = os.Stat
	osMkdirAll            = os.MkdirAll
	osWriteFile           = os.WriteFile
)

func defaultConfigPath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		homeDir, err := osUserHomeDir()
		if err == nil && homeDir != "" {
			configDir = filepath.Join(homeDir, ".config")
		}
	}
	if configDir == "" {
		userConfigDir, err := osUserConfigDir()
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
	return loadConfigWithLogger(configPath, zap.NewNop())
}

func loadConfigWithLogger(configPath string, logger *zap.Logger) (Config, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	rawConfigPath := configPath
	configPath = normalizeConfigPath(configPath)
	logger.Debug("config path resolved",
		zap.String("path", configPath),
		zap.Bool("explicit_path", strings.TrimSpace(rawConfigPath) != ""),
		zap.Bool("base_url_env_present", envPresent("OPENMINUTES_BASE_URL")),
		zap.Bool("space_base_url_env_present", envPresent("OPENMINUTES_SPACE_BASE_URL")),
		zap.Bool("cookie_env_present", envPresent("OPENMINUTES_COOKIE")),
	)
	if err := ensureConfigFileWithLogger(configPath, logger); err != nil {
		logger.Debug("config file ensure failed",
			zap.String("path", configPath),
			zap.Error(err),
		)
		return Config{}, err
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")
	v.SetEnvPrefix("OPENMINUTES")
	v.AllowEmptyEnv(true)
	v.AutomaticEnv()

	for _, key := range []string{"base_url", "space_base_url", "cookie"} {
		if err := v.BindEnv(key); err != nil {
			return Config{}, err
		}
	}

	if err := v.ReadInConfig(); err != nil {
		logger.Debug("config read failed",
			zap.String("path", configPath),
			zap.Error(err),
		)
		return Config{}, err
	}

	config := Config{
		BaseURL:      configBaseURLOrDefault(v.GetString("base_url"), defaultBaseURL),
		SpaceBaseURL: configBaseURLOrDefault(v.GetString("space_base_url"), defaultSpaceBaseURL),
		Cookie:       v.GetString("cookie"),
	}
	if err := validateConfig(config); err != nil {
		logger.Debug("config validation failed",
			zap.String("path", configPath),
			zap.String("base_url", config.BaseURL),
			zap.String("space_base_url", config.SpaceBaseURL),
			zap.Bool("cookie_present", strings.TrimSpace(config.Cookie) != ""),
			zap.Bool("base_url_env_override", envPresent("OPENMINUTES_BASE_URL")),
			zap.Bool("space_base_url_env_override", envPresent("OPENMINUTES_SPACE_BASE_URL")),
			zap.Bool("cookie_env_override", envPresent("OPENMINUTES_COOKIE")),
			zap.Error(err),
		)
		return Config{}, err
	}

	logger.Debug("config loaded",
		zap.String("path", configPath),
		zap.String("base_url", config.BaseURL),
		zap.String("space_base_url", config.SpaceBaseURL),
		zap.Bool("cookie_present", strings.TrimSpace(config.Cookie) != ""),
		zap.Bool("base_url_env_override", envPresent("OPENMINUTES_BASE_URL")),
		zap.Bool("space_base_url_env_override", envPresent("OPENMINUTES_SPACE_BASE_URL")),
		zap.Bool("cookie_env_override", envPresent("OPENMINUTES_COOKIE")),
	)

	return config, nil
}

func normalizeConfigPath(configPath string) string {
	if configPath == "" {
		return defaultConfigPathFunc()
	}

	homeDir, err := osUserHomeDir()
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
	return ensureConfigFileWithLogger(configPath, zap.NewNop())
}

func ensureConfigFileWithLogger(configPath string, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}

	if _, err := osStat(configPath); err == nil {
		logger.Debug("config file found", zap.String("path", configPath))
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	logger.Debug("config file missing, creating template", zap.String("path", configPath))
	if err := osMkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return err
	}

	return osWriteFile(configPath, []byte(configTemplate), 0o600)
}

func envPresent(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}

func validateConfig(config Config) error {
	if err := validateConfigBaseURL("base_url", config.BaseURL); err != nil {
		return err
	}
	if err := validateConfigBaseURL("space_base_url", config.SpaceBaseURL); err != nil {
		return err
	}

	if strings.TrimSpace(config.Cookie) == "" {
		return errors.New("cookie is required")
	}

	return nil
}

func configBaseURLOrDefault(rawURL, defaultURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return defaultURL
	}

	return trimBaseURLTrailingSlash(rawURL)
}

func validateConfigBaseURL(fieldName, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return invalidConfigBaseURLError(fieldName, rawURL)
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return invalidConfigBaseURLError(fieldName, rawURL)
	}

	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return invalidConfigBaseURLError(fieldName, rawURL)
	}

	return nil
}

func invalidConfigBaseURLError(fieldName, rawURL string) error {
	return fmt.Errorf("invalid %s %q: must be an absolute http or https URL with a host", fieldName, rawURL)
}

func trimBaseURLTrailingSlash(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || parsed.Path == "" {
		return rawURL
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String()
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

func commandRequiresConfirmation(cmd *cobra.Command) bool {
	for current := cmd; current != nil; current = current.Parent() {
		if current.Annotations[requiresConfirmationAnnotation] == "true" {
			return true
		}
	}

	return false
}
