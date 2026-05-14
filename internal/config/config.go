package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"openminutes/internal/minutes"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const (
	DefaultBaseURL       = minutes.DefaultBaseURL
	DefaultSpaceBaseURL  = minutes.DefaultSpaceBaseURL
	DefaultPathFlagValue = "~/.config/openminutes/config.toml"

	Template = "base_url = \"https://meetings.feishu.cn\"\nspace_base_url = \"https://internal-api-space.feishu.cn\"\ncookie = \"\"\n"
)

type Config struct {
	BaseURL      string
	SpaceBaseURL string
	Cookie       string
}

func DefaultPath() string {
	return defaultPath(os.Getenv("XDG_CONFIG_HOME"), os.UserHomeDir, os.UserConfigDir)
}

func defaultPath(xdgConfigHome string, userHomeDir func() (string, error), userConfigDirFunc func() (string, error)) string {
	configDir := xdgConfigHome
	if configDir == "" {
		homeDir, err := userHomeDir()
		if err == nil && homeDir != "" {
			configDir = filepath.Join(homeDir, ".config")
		}
	}
	if configDir == "" {
		userConfigDir, err := userConfigDirFunc()
		if err == nil && userConfigDir != "" {
			configDir = userConfigDir
		}
	}
	if configDir == "" {
		configDir = ".config"
	}

	return filepath.Join(configDir, "openminutes", "config.toml")
}

func Load(path string, logger *zap.Logger) (Config, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	rawPath := path
	path = NormalizePath(path)
	logger.Debug("config path resolved",
		zap.String("path", path),
		zap.Bool("explicit_path", strings.TrimSpace(rawPath) != ""),
		zap.Bool("base_url_env_present", envPresent("OPENMINUTES_BASE_URL")),
		zap.Bool("space_base_url_env_present", envPresent("OPENMINUTES_SPACE_BASE_URL")),
		zap.Bool("cookie_env_present", envPresent("OPENMINUTES_COOKIE")),
	)
	if err := EnsureFile(path, logger); err != nil {
		logger.Debug("config file ensure failed",
			zap.String("path", path),
			zap.Error(err),
		)
		return Config{}, err
	}

	v := viper.New()
	v.SetConfigFile(path)
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
			zap.String("path", path),
			zap.Error(err),
		)
		return Config{}, err
	}

	config := Config{
		Cookie: v.GetString("cookie"),
	}
	var err error
	config.BaseURL, _, err = minutes.NormalizeBaseURLOrDefault("base_url", v.GetString("base_url"), DefaultBaseURL)
	if err != nil {
		logger.Debug("config validation failed",
			zap.String("path", path),
			zap.String("base_url", strings.TrimSpace(v.GetString("base_url"))),
			zap.Bool("cookie_present", strings.TrimSpace(config.Cookie) != ""),
			zap.Bool("base_url_env_override", envPresent("OPENMINUTES_BASE_URL")),
			zap.Bool("space_base_url_env_override", envPresent("OPENMINUTES_SPACE_BASE_URL")),
			zap.Bool("cookie_env_override", envPresent("OPENMINUTES_COOKIE")),
			zap.Error(err),
		)
		return Config{}, err
	}
	config.SpaceBaseURL, _, err = minutes.NormalizeBaseURLOrDefault("space_base_url", v.GetString("space_base_url"), DefaultSpaceBaseURL)
	if err != nil {
		logger.Debug("config validation failed",
			zap.String("path", path),
			zap.String("space_base_url", strings.TrimSpace(v.GetString("space_base_url"))),
			zap.Bool("cookie_present", strings.TrimSpace(config.Cookie) != ""),
			zap.Bool("base_url_env_override", envPresent("OPENMINUTES_BASE_URL")),
			zap.Bool("space_base_url_env_override", envPresent("OPENMINUTES_SPACE_BASE_URL")),
			zap.Bool("cookie_env_override", envPresent("OPENMINUTES_COOKIE")),
			zap.Error(err),
		)
		return Config{}, err
	}
	if err := Validate(config); err != nil {
		logger.Debug("config validation failed",
			zap.String("path", path),
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
		zap.String("path", path),
		zap.String("base_url", config.BaseURL),
		zap.String("space_base_url", config.SpaceBaseURL),
		zap.Bool("cookie_present", strings.TrimSpace(config.Cookie) != ""),
		zap.Bool("base_url_env_override", envPresent("OPENMINUTES_BASE_URL")),
		zap.Bool("space_base_url_env_override", envPresent("OPENMINUTES_SPACE_BASE_URL")),
		zap.Bool("cookie_env_override", envPresent("OPENMINUTES_COOKIE")),
	)

	return config, nil
}

func NormalizePath(path string) string {
	return normalizePath(path, DefaultPath, os.UserHomeDir)
}

func normalizePath(path string, defaultPath func() string, userHomeDir func() (string, error)) string {
	if path == "" {
		return defaultPath()
	}

	homeDir, err := userHomeDir()
	if err != nil || homeDir == "" {
		return path
	}

	if path == "~" {
		return homeDir
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, strings.TrimPrefix(path, "~/"))
	}

	return path
}

func EnsureFile(path string, logger *zap.Logger) error {
	return ensureFile(path, logger, os.Stat, os.MkdirAll, os.WriteFile)
}

func ensureFile(
	path string,
	logger *zap.Logger,
	stat func(string) (fs.FileInfo, error),
	mkdirAll func(string, fs.FileMode) error,
	writeFile func(string, []byte, fs.FileMode) error,
) error {
	if logger == nil {
		logger = zap.NewNop()
	}

	if _, err := stat(path); err == nil {
		logger.Debug("config file found", zap.String("path", path))
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	logger.Debug("config file missing, creating template", zap.String("path", path))
	if err := mkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	return writeFile(path, []byte(Template), 0o600)
}

func Validate(config Config) error {
	if err := validateBaseURL("base_url", config.BaseURL); err != nil {
		return err
	}
	if err := validateBaseURL("space_base_url", config.SpaceBaseURL); err != nil {
		return err
	}

	if strings.TrimSpace(config.Cookie) == "" {
		return errors.New("cookie is required")
	}

	return nil
}

func envPresent(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}

func validateBaseURL(fieldName, rawURL string) error {
	_, err := minutes.NormalizeBaseURL(fieldName, rawURL)
	return err
}
