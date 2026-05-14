package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "openminutes/internal/errors"

	"go.uber.org/zap"
)

func TestLoadCreatesMissingDefaultConfig(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := filepath.Join(t.TempDir(), "openminutes", "config.toml")
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(filepath.Dir(configPath)))

	config, err := Load("", nil)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "cookie is required") {
		t.Fatalf("Load() error = %v, want cookie required", err)
	}
	if !apperrors.IsKind(err, apperrors.KindAuth) {
		t.Fatalf("Load() error kind = %q, want auth", apperrors.KindOf(err))
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(data) != Template {
		t.Fatalf("config file = %q, want %q", data, Template)
	}

	if config != (Config{}) {
		t.Fatalf("config = %#v, want zero value", config)
	}
}

func TestLoadCreatesMissingManualConfig(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := filepath.Join(t.TempDir(), "custom", "settings.toml")

	_, err := Load(configPath, nil)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(data) != Template {
		t.Fatalf("config file = %q, want %q", data, Template)
	}
}

func TestLoadReadsFileValues(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `base_url = "https://meetings.example.test/"
space_base_url = "https://space.example.test/"
cookie = "session=abc"
`)

	config, err := Load(configPath, nil)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	want := Config{
		BaseURL:      "https://meetings.example.test",
		SpaceBaseURL: "https://space.example.test",
		Cookie:       "session=abc",
	}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
}

func TestLoadEnvOverridesFileValues(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `base_url = "https://file.example.test"
space_base_url = "https://file-space.example.test"
cookie = "session=file"
`)
	t.Setenv("OPENMINUTES_BASE_URL", "https://env.example.test")
	t.Setenv("OPENMINUTES_SPACE_BASE_URL", "https://env-space.example.test")
	t.Setenv("OPENMINUTES_COOKIE", "session=env")

	config, err := Load(configPath, nil)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	want := Config{
		BaseURL:      "https://env.example.test",
		SpaceBaseURL: "https://env-space.example.test",
		Cookie:       "session=env",
	}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
}

func TestLoadEmptyURLEnvUsesDefaults(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `base_url = "https://file.example.test"
space_base_url = "https://file-space.example.test"
cookie = "session=file"
`)
	t.Setenv("OPENMINUTES_BASE_URL", "")
	t.Setenv("OPENMINUTES_SPACE_BASE_URL", "")

	config, err := Load(configPath, nil)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	want := Config{BaseURL: DefaultBaseURL, SpaceBaseURL: DefaultSpaceBaseURL, Cookie: "session=file"}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
}

func TestLoadEmptyCookieEnvRejectsCookie(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `base_url = "https://file.example.test"
space_base_url = "https://file-space.example.test"
cookie = "session=file"
`)
	t.Setenv("OPENMINUTES_BASE_URL", "")
	t.Setenv("OPENMINUTES_SPACE_BASE_URL", "")
	t.Setenv("OPENMINUTES_COOKIE", "")

	_, err := Load(configPath, nil)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	if err.Error() != "cookie is required" {
		t.Fatalf("Load() error = %q, want cookie is required", err.Error())
	}
	if !apperrors.IsKind(err, apperrors.KindAuth) {
		t.Fatalf("Load() error kind = %q, want auth", apperrors.KindOf(err))
	}
}

func TestLoadRejectsInvalidURLs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "base url missing scheme",
			content: `base_url = "meetings.example.test"
space_base_url = "https://space.example.test"
cookie = "session=abc"
`,
			want: `invalid base_url "meetings.example.test": must be an absolute http or https URL with a host`,
		},
		{
			name: "base url unsupported scheme",
			content: `base_url = "ftp://meetings.example.test"
space_base_url = "https://space.example.test"
cookie = "session=abc"
`,
			want: `invalid base_url "ftp://meetings.example.test": must be an absolute http or https URL with a host`,
		},
		{
			name: "space base url missing host",
			content: `base_url = "https://meetings.example.test"
space_base_url = "https://"
cookie = "session=abc"
`,
			want: `invalid space_base_url "https://": must be an absolute http or https URL with a host`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withoutOpenMinutesEnv(t)
			configPath := writeConfig(t, tt.content)
			_, err := Load(configPath, nil)
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}
			if err.Error() != tt.want {
				t.Fatalf("Load() error = %q, want %q", err.Error(), tt.want)
			}
			if !apperrors.IsKind(err, apperrors.KindConfig) {
				t.Fatalf("Load() error kind = %q, want config", apperrors.KindOf(err))
			}
		})
	}
}

func TestLoadRejectsEmptyCookie(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `base_url = "https://meetings.example.test"
space_base_url = "https://space.example.test"
cookie = ""
`)

	_, err := Load(configPath, nil)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	if err.Error() != "cookie is required" {
		t.Fatalf("Load() error = %q, want cookie is required", err.Error())
	}
}

func TestLoadDefaultsMissingURLs(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `cookie = "session=abc"
`)

	config, err := Load(configPath, nil)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	want := Config{BaseURL: DefaultBaseURL, SpaceBaseURL: DefaultSpaceBaseURL, Cookie: "session=abc"}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
}

func TestLoadIgnoresRegionFileAndEnv(t *testing.T) {
	withoutOpenMinutesEnv(t)
	t.Setenv("OPENMINUTES_REGION", "larksuite")

	configPath := writeConfig(t, `region = "invalid"
cookie = "session=abc"
`)

	config, err := Load(configPath, nil)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	want := Config{BaseURL: DefaultBaseURL, SpaceBaseURL: DefaultSpaceBaseURL, Cookie: "session=abc"}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
}

func TestDefaultPath(t *testing.T) {
	tests := []struct {
		name          string
		xdgConfigHome string
		homeDir       string
		homeErr       error
		userConfigDir string
		userConfigErr error
		want          string
	}{
		{
			name:          "xdg config home",
			xdgConfigHome: "/xdg",
			homeErr:       errors.New("home should not be used"),
			userConfigErr: errors.New("user config should not be used"),
			want:          filepath.Join("/xdg", "openminutes", "config.toml"),
		},
		{
			name:          "home fallback",
			xdgConfigHome: "",
			homeDir:       "/home/alice",
			userConfigErr: errors.New("user config should not be used"),
			want:          filepath.Join("/home/alice", ".config", "openminutes", "config.toml"),
		},
		{
			name:          "user config fallback",
			xdgConfigHome: "",
			homeErr:       errors.New("no home"),
			userConfigDir: "/user-config",
			want:          filepath.Join("/user-config", "openminutes", "config.toml"),
		},
		{
			name:          "relative fallback",
			xdgConfigHome: "",
			homeErr:       errors.New("no home"),
			userConfigErr: errors.New("no config"),
			want:          filepath.Join(".config", "openminutes", "config.toml"),
		},
		{
			name:          "empty home uses user config",
			xdgConfigHome: "",
			homeDir:       "",
			userConfigDir: "/user-config",
			want:          filepath.Join("/user-config", "openminutes", "config.toml"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultPath(tt.xdgConfigHome, func() (string, error) {
				return tt.homeDir, tt.homeErr
			}, func() (string, error) {
				return tt.userConfigDir, tt.userConfigErr
			})
			if got != tt.want {
				t.Fatalf("defaultPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizePath(t *testing.T) {
	defaultPath := func() string { return "/default/config.toml" }
	userHomeDir := func() (string, error) { return "/home/alice", nil }

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "empty path", path: "", want: "/default/config.toml"},
		{name: "home only", path: "~", want: "/home/alice"},
		{name: "home relative", path: "~/openminutes/config.toml", want: filepath.Join("/home/alice", "openminutes", "config.toml")},
		{name: "plain path", path: "/tmp/config.toml", want: "/tmp/config.toml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePath(tt.path, defaultPath, userHomeDir); got != tt.want {
				t.Fatalf("normalizePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestNormalizePathKeepsPathWhenHomeLookupFails(t *testing.T) {
	got := normalizePath("~/config.toml", func() string { return "/default/config.toml" }, func() (string, error) {
		return "", errors.New("no home")
	})
	if got != "~/config.toml" {
		t.Fatalf("normalizePath() = %q, want original path", got)
	}
}

func TestEnsureFile(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(existing, []byte("cookie = \"session=abc\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := EnsureFile(existing, nil); err != nil {
		t.Fatalf("EnsureFile(existing) error = %v, want nil", err)
	}

	missing := filepath.Join(t.TempDir(), "nested", "config.toml")
	if err := EnsureFile(missing, zap.NewNop()); err != nil {
		t.Fatalf("EnsureFile(missing) error = %v, want nil", err)
	}
	data, err := os.ReadFile(missing)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != Template {
		t.Fatalf("created config = %q, want template", data)
	}
}

func TestEnsureFileReturnsStatError(t *testing.T) {
	wantErr := errors.New("stat failed")
	stat := func(string) (fs.FileInfo, error) {
		return nil, wantErr
	}

	if err := ensureFile("/tmp/config.toml", nil, stat, os.MkdirAll, os.WriteFile); !errors.Is(err, wantErr) {
		t.Fatalf("ensureFile() error = %v, want %v", err, wantErr)
	}
}

func TestEnsureFileReturnsCreateErrors(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "mkdir"},
		{name: "write"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantErr := errors.New(tt.name + " failed")
			stat := func(string) (fs.FileInfo, error) {
				return nil, os.ErrNotExist
			}
			mkdirAll := os.MkdirAll
			writeFile := os.WriteFile
			switch tt.name {
			case "mkdir":
				mkdirAll = func(string, fs.FileMode) error {
					return wantErr
				}
			case "write":
				writeFile = func(string, []byte, fs.FileMode) error {
					return wantErr
				}
			}

			err := ensureFile(filepath.Join(t.TempDir(), "config.toml"), zap.NewNop(), stat, mkdirAll, writeFile)
			if !errors.Is(err, wantErr) {
				t.Fatalf("ensureFile() error = %v, want %v", err, wantErr)
			}
		})
	}
}

func TestLoadEdgeCases(t *testing.T) {
	t.Run("nil logger", func(t *testing.T) {
		withoutOpenMinutesEnv(t)
		configPath := writeConfig(t, `base_url = "https://meetings.example.test"
space_base_url = "https://space.example.test"
cookie = "session=abc"
`)

		config, err := Load(configPath, nil)
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}
		want := Config{
			BaseURL:      "https://meetings.example.test",
			SpaceBaseURL: "https://space.example.test",
			Cookie:       "session=abc",
		}
		if config != want {
			t.Fatalf("config = %#v, want file values", config)
		}
	})

	t.Run("invalid toml", func(t *testing.T) {
		withoutOpenMinutesEnv(t)
		configPath := writeConfig(t, "base_url = [")

		if _, err := Load(configPath, zap.NewNop()); err == nil {
			t.Fatal("Load() error = nil, want invalid TOML error")
		}
	})

	t.Run("ensure failure", func(t *testing.T) {
		withoutOpenMinutesEnv(t)
		if _, err := Load("\x00", zap.NewNop()); err == nil {
			t.Fatal("Load() error = nil, want ensure failure")
		}
	})

	t.Run("missing urls default with cookie", func(t *testing.T) {
		withoutOpenMinutesEnv(t)
		configPath := writeConfig(t, `cookie = "session=abc"
`)

		config, err := Load(configPath, zap.NewNop())
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}
		want := Config{BaseURL: DefaultBaseURL, SpaceBaseURL: DefaultSpaceBaseURL, Cookie: "session=abc"}
		if config != want {
			t.Fatalf("config = %#v, want defaults", config)
		}
	})
}

func TestValidateBaseURLRejectsQueryAndFragment(t *testing.T) {
	tests := []string{
		"https://meetings.example.test?token=secret",
		"https://meetings.example.test#section",
	}

	for _, rawURL := range tests {
		t.Run(rawURL, func(t *testing.T) {
			err := validateBaseURL("base_url", rawURL)
			if err == nil {
				t.Fatal("validateBaseURL() error = nil, want invalid URL error")
			}
			if !strings.Contains(err.Error(), "invalid base_url") {
				t.Fatalf("validateBaseURL() error = %q, want invalid base_url", err.Error())
			}
		})
	}
}

func TestValidateRejectsInvalidURLs(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{
			name: "invalid base url",
			config: Config{
				BaseURL:      "meetings.example.test",
				SpaceBaseURL: DefaultSpaceBaseURL,
				Cookie:       "session=abc",
			},
			want: "invalid base_url",
		},
		{
			name: "invalid space base url",
			config: Config{
				BaseURL:      DefaultBaseURL,
				SpaceBaseURL: "space.example.test",
				Cookie:       "session=abc",
			},
			want: "invalid space_base_url",
		},
		{
			name: "empty cookie",
			config: Config{
				BaseURL:      DefaultBaseURL,
				SpaceBaseURL: DefaultSpaceBaseURL,
				Cookie:       "",
			},
			want: "cookie is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.config)
			if err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func withoutOpenMinutesEnv(t *testing.T) {
	t.Helper()

	baseURL, hadBaseURL := os.LookupEnv("OPENMINUTES_BASE_URL")
	spaceBaseURL, hadSpaceBaseURL := os.LookupEnv("OPENMINUTES_SPACE_BASE_URL")
	cookie, hadCookie := os.LookupEnv("OPENMINUTES_COOKIE")
	region, hadRegion := os.LookupEnv("OPENMINUTES_REGION")

	if err := os.Unsetenv("OPENMINUTES_BASE_URL"); err != nil {
		t.Fatalf("Unsetenv(OPENMINUTES_BASE_URL) error = %v", err)
	}
	if err := os.Unsetenv("OPENMINUTES_SPACE_BASE_URL"); err != nil {
		t.Fatalf("Unsetenv(OPENMINUTES_SPACE_BASE_URL) error = %v", err)
	}
	if err := os.Unsetenv("OPENMINUTES_COOKIE"); err != nil {
		t.Fatalf("Unsetenv(OPENMINUTES_COOKIE) error = %v", err)
	}
	if err := os.Unsetenv("OPENMINUTES_REGION"); err != nil {
		t.Fatalf("Unsetenv(OPENMINUTES_REGION) error = %v", err)
	}

	t.Cleanup(func() {
		if hadBaseURL {
			_ = os.Setenv("OPENMINUTES_BASE_URL", baseURL)
		} else {
			_ = os.Unsetenv("OPENMINUTES_BASE_URL")
		}

		if hadSpaceBaseURL {
			_ = os.Setenv("OPENMINUTES_SPACE_BASE_URL", spaceBaseURL)
		} else {
			_ = os.Unsetenv("OPENMINUTES_SPACE_BASE_URL")
		}

		if hadCookie {
			_ = os.Setenv("OPENMINUTES_COOKIE", cookie)
		} else {
			_ = os.Unsetenv("OPENMINUTES_COOKIE")
		}

		if hadRegion {
			_ = os.Setenv("OPENMINUTES_REGION", region)
		} else {
			_ = os.Unsetenv("OPENMINUTES_REGION")
		}
	})
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return configPath
}
