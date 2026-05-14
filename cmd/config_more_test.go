package cmd

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openminutes/internal/minutes"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func TestDefaultConfigPath(t *testing.T) {
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
			t.Setenv("XDG_CONFIG_HOME", tt.xdgConfigHome)
			withOSUserHomeDir(t, func() (string, error) {
				return tt.homeDir, tt.homeErr
			})
			withOSUserConfigDir(t, func() (string, error) {
				return tt.userConfigDir, tt.userConfigErr
			})

			if got := defaultConfigPath(); got != tt.want {
				t.Fatalf("defaultConfigPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeConfigPath(t *testing.T) {
	withDefaultConfigPath(t, "/default/config.toml")
	withOSUserHomeDir(t, func() (string, error) {
		return "/home/alice", nil
	})

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
			if got := normalizeConfigPath(tt.path); got != tt.want {
				t.Fatalf("normalizeConfigPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestNormalizeConfigPathKeepsPathWhenHomeLookupFails(t *testing.T) {
	withOSUserHomeDir(t, func() (string, error) {
		return "", errors.New("no home")
	})

	if got := normalizeConfigPath("~/config.toml"); got != "~/config.toml" {
		t.Fatalf("normalizeConfigPath() = %q, want original path", got)
	}
}

func TestEnsureConfigFile(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(existing, []byte("cookie = \"session=abc\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := ensureConfigFile(existing); err != nil {
		t.Fatalf("ensureConfigFile(existing) error = %v, want nil", err)
	}

	missing := filepath.Join(t.TempDir(), "nested", "config.toml")
	if err := ensureConfigFile(missing); err != nil {
		t.Fatalf("ensureConfigFile(missing) error = %v, want nil", err)
	}
	data, err := os.ReadFile(missing)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != configTemplate {
		t.Fatalf("created config = %q, want template", data)
	}
}

func TestEnsureConfigFileReturnsStatError(t *testing.T) {
	wantErr := errors.New("stat failed")
	withOSStat(t, func(string) (fs.FileInfo, error) {
		return nil, wantErr
	})

	if err := ensureConfigFileWithLogger("/tmp/config.toml", nil); !errors.Is(err, wantErr) {
		t.Fatalf("ensureConfigFileWithLogger() error = %v, want %v", err, wantErr)
	}
}

func TestEnsureConfigFileReturnsCreateErrors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, error)
	}{
		{
			name: "mkdir",
			setup: func(t *testing.T, wantErr error) {
				withOSMkdirAll(t, func(string, fs.FileMode) error {
					return wantErr
				})
			},
		},
		{
			name: "write",
			setup: func(t *testing.T, wantErr error) {
				withOSWriteFile(t, func(string, []byte, fs.FileMode) error {
					return wantErr
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantErr := errors.New(tt.name + " failed")
			withOSStat(t, func(string) (fs.FileInfo, error) {
				return nil, os.ErrNotExist
			})
			tt.setup(t, wantErr)

			err := ensureConfigFileWithLogger(filepath.Join(t.TempDir(), "config.toml"), zap.NewNop())
			if !errors.Is(err, wantErr) {
				t.Fatalf("ensureConfigFileWithLogger() error = %v, want %v", err, wantErr)
			}
		})
	}
}

func TestLoadConfigWithLoggerEdgeCases(t *testing.T) {
	t.Run("nil logger", func(t *testing.T) {
		withoutOpenMinutesEnv(t)
		configPath := writeConfig(t, `base_url = "https://meetings.example.test"
space_base_url = "https://space.example.test"
cookie = "session=abc"
`)

		config, err := loadConfigWithLogger(configPath, nil)
		if err != nil {
			t.Fatalf("loadConfigWithLogger() error = %v, want nil", err)
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

		if _, err := loadConfigWithLogger(configPath, zap.NewNop()); err == nil {
			t.Fatal("loadConfigWithLogger() error = nil, want invalid TOML error")
		}
	})

	t.Run("ensure failure", func(t *testing.T) {
		withoutOpenMinutesEnv(t)
		wantErr := errors.New("ensure failed")
		withOSStat(t, func(string) (fs.FileInfo, error) {
			return nil, wantErr
		})

		if _, err := loadConfigWithLogger("/tmp/config.toml", zap.NewNop()); !errors.Is(err, wantErr) {
			t.Fatalf("loadConfigWithLogger() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("missing urls default with cookie", func(t *testing.T) {
		withoutOpenMinutesEnv(t)
		configPath := writeConfig(t, `cookie = "session=abc"
`)

		config, err := loadConfigWithLogger(configPath, zap.NewNop())
		if err != nil {
			t.Fatalf("loadConfigWithLogger() error = %v, want nil", err)
		}
		want := Config{BaseURL: defaultBaseURL, SpaceBaseURL: defaultSpaceBaseURL, Cookie: "session=abc"}
		if config != want {
			t.Fatalf("config = %#v, want defaults", config)
		}
	})
}

func TestValidateConfigBaseURLRejectsQueryAndFragment(t *testing.T) {
	tests := []string{
		"https://meetings.example.test?token=secret",
		"https://meetings.example.test#section",
	}

	for _, rawURL := range tests {
		t.Run(rawURL, func(t *testing.T) {
			err := validateConfigBaseURL("base_url", rawURL)
			if err == nil {
				t.Fatal("validateConfigBaseURL() error = nil, want invalid URL error")
			}
			if !strings.Contains(err.Error(), "invalid base_url") {
				t.Fatalf("validateConfigBaseURL() error = %q, want invalid base_url", err.Error())
			}
		})
	}
}

func TestLoggingHelpersAcceptNilInputs(t *testing.T) {
	logger, ok := loggerFromContext(contextWithLogger(context.Background(), nil))
	if !ok || logger == nil {
		t.Fatalf("loggerFromContext() = %v, %v, want no-op logger", logger, ok)
	}

	newVerboseLogger(nil).Debug("discarded")
}

func TestDefaultCommandClientFactories(t *testing.T) {
	config := minutes.Config{
		Cookie: "session=abc; bv_csrf_token=csrf-token",
	}

	if _, err := newListMinutesClient(config); err != nil {
		t.Fatalf("newListMinutesClient() error = %v, want nil", err)
	}
	if _, err := newDeleteMinutesClient(config); err != nil {
		t.Fatalf("newDeleteMinutesClient() error = %v, want nil", err)
	}
}

func TestRunListCommandFlagGetterErrors(t *testing.T) {
	config := testCommandConfig()

	tests := []struct {
		name  string
		flags func(*cobra.Command)
	}{
		{
			name:  "missing size flag",
			flags: func(*cobra.Command) {},
		},
		{
			name: "missing timestamp flag",
			flags: func(cmd *cobra.Command) {
				cmd.Flags().Int("size", 20, "")
			},
		},
		{
			name: "missing json flag",
			flags: func(cmd *cobra.Command) {
				cmd.Flags().Int("size", 20, "")
				cmd.Flags().Int64("timestamp", 0, "")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withListMinutesClient(t, func(minutes.Config) (listMinutesClient, error) {
				return listMinutesClientFunc(func(context.Context, minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
					return &minutes.ListMinutesPageResult{}, nil
				}), nil
			})

			cmd := &cobra.Command{Use: "list"}
			tt.flags(cmd)
			cmd.SetContext(contextWithConfig(context.Background(), config))
			err := runListCommand(cmd, nil)
			if err == nil {
				t.Fatal("runListCommand() error = nil, want flag error")
			}
		})
	}
}

func TestRunDeleteCommandFlagGetterErrors(t *testing.T) {
	tests := []struct {
		name  string
		flags func(*cobra.Command)
	}{
		{
			name:  "missing yes flag",
			flags: func(*cobra.Command) {},
		},
		{
			name: "missing destroy flag",
			flags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("yes", true, "")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "delete"}
			tt.flags(cmd)
			cmd.SetContext(contextWithConfig(context.Background(), testCommandConfig()))
			err := runDeleteCommand(cmd, []string{"token-1"})
			if err == nil {
				t.Fatal("runDeleteCommand() error = nil, want flag error")
			}
		})
	}
}

func TestRootCommandConfirmationFlagGetterError(t *testing.T) {
	cmd := newRootCommand()
	cmd.AddCommand(&cobra.Command{
		Use: "confirm-without-flag",
		Annotations: map[string]string{
			requiresConfigAnnotation:       "true",
			requiresConfirmationAnnotation: "true",
		},
		Run: func(*cobra.Command, []string) {},
	})
	cmd.SetArgs([]string{"confirm-without-flag"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want flag error")
	}
	if !strings.Contains(err.Error(), `flag accessed but not defined: yes`) {
		t.Fatalf("Execute() error = %q, want missing yes flag error", err.Error())
	}
}

func TestRootCommandDefaultConfigFlagFallback(t *testing.T) {
	withoutOpenMinutesEnv(t)
	t.Setenv("OPENMINUTES_BASE_URL", "https://env.example.test")
	t.Setenv("OPENMINUTES_SPACE_BASE_URL", "https://env-space.example.test")
	t.Setenv("OPENMINUTES_COOKIE", "session=env")
	withDefaultConfigPath(t, filepath.Join(t.TempDir(), "config.toml"))

	root := newRootCommand()
	cmd := &cobra.Command{
		Use: "direct",
		Annotations: map[string]string{
			requiresConfigAnnotation: "true",
		},
	}
	cmd.SetContext(context.Background())

	if err := root.PersistentPreRunE(cmd, nil); err != nil {
		t.Fatalf("PersistentPreRunE() error = %v, want nil", err)
	}
	config, ok := configFromCommand(cmd)
	if !ok {
		t.Fatal("configFromCommand() ok = false, want true")
	}
	want := Config{
		BaseURL:      "https://env.example.test",
		SpaceBaseURL: "https://env-space.example.test",
		Cookie:       "session=env",
	}
	if config != want {
		t.Fatalf("config = %#v, want env values", config)
	}
}

func TestListCommandReturnsStdoutWriteErrors(t *testing.T) {
	tests := []struct {
		name   string
		result *minutes.ListMinutesPageResult
		writer *failingWriter
	}{
		{
			name: "row",
			result: &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{{ObjectToken: "token-1", Topic: "First", URL: "https://example.test/first"}},
			},
			writer: &failingWriter{failAt: 1},
		},
		{
			name: "next page footer",
			result: &minutes.ListMinutesPageResult{
				Items:         []minutes.Minute{{ObjectToken: "token-1", Topic: "First", URL: "https://example.test/first"}},
				HasMore:       true,
				NextTimestamp: 100,
			},
			writer: &failingWriter{failAt: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantErr := errors.New("write failed")
			tt.writer.err = wantErr
			withListMinutesClient(t, func(minutes.Config) (listMinutesClient, error) {
				return listMinutesClientFunc(func(context.Context, minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
					return tt.result, nil
				}), nil
			})

			cmd := newListCommand()
			cmd.SetOut(tt.writer)
			cmd.SetArgs([]string{})
			cmd.SetContext(contextWithConfig(context.Background(), testCommandConfig()))

			if err := cmd.Execute(); !errors.Is(err, wantErr) {
				t.Fatalf("Execute() error = %v, want %v", err, wantErr)
			}
		})
	}
}

func TestListCommandReturnsJSONWriteError(t *testing.T) {
	wantErr := errors.New("write failed")
	withListMinutesClient(t, func(minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(context.Context, minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			return &minutes.ListMinutesPageResult{}, nil
		}), nil
	})

	cmd := newListCommand()
	cmd.SetOut(&failingWriter{failAt: 1, err: wantErr})
	cmd.SetArgs([]string{"--json"})
	cmd.SetContext(contextWithConfig(context.Background(), Config{BaseURL: defaultBaseURL, SpaceBaseURL: defaultSpaceBaseURL, Cookie: "session=abc"}))

	if err := cmd.Execute(); !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestDeleteCommandReturnsStdoutWriteErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "soft delete", args: []string{"token-1", "--yes"}},
		{name: "destroy", args: []string{"token-1", "--yes", "--destroy"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantErr := errors.New("write failed")
			withDeleteMinutesClient(t, func(minutes.Config) (deleteMinutesClient, error) {
				return deleteMinutesClientFunc(func(context.Context, string, minutes.DeleteOptions) error {
					return nil
				}), nil
			})

			cmd := newDeleteCommand()
			cmd.SetOut(&failingWriter{failAt: 1, err: wantErr})
			cmd.SetArgs(tt.args)
			cmd.SetContext(contextWithConfig(context.Background(), testCommandConfig()))

			if err := cmd.Execute(); !errors.Is(err, wantErr) {
				t.Fatalf("Execute() error = %v, want %v", err, wantErr)
			}
		})
	}
}

type failingWriter struct {
	writes int
	failAt int
	err    error
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes >= w.failAt {
		return 0, w.err
	}

	return len(p), nil
}

func withOSUserHomeDir(t *testing.T, fn func() (string, error)) {
	t.Helper()

	old := osUserHomeDir
	osUserHomeDir = fn
	t.Cleanup(func() {
		osUserHomeDir = old
	})
}

func withOSUserConfigDir(t *testing.T, fn func() (string, error)) {
	t.Helper()

	old := osUserConfigDir
	osUserConfigDir = fn
	t.Cleanup(func() {
		osUserConfigDir = old
	})
}

func withOSStat(t *testing.T, fn func(string) (fs.FileInfo, error)) {
	t.Helper()

	old := osStat
	osStat = fn
	t.Cleanup(func() {
		osStat = old
	})
}

func withOSMkdirAll(t *testing.T, fn func(string, fs.FileMode) error) {
	t.Helper()

	old := osMkdirAll
	osMkdirAll = fn
	t.Cleanup(func() {
		osMkdirAll = old
	})
}

func withOSWriteFile(t *testing.T, fn func(string, []byte, fs.FileMode) error) {
	t.Helper()

	old := osWriteFile
	osWriteFile = fn
	t.Cleanup(func() {
		osWriteFile = old
	})
}
