package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openminutes/internal/minutes"

	"github.com/spf13/cobra"
)

func executeCommand(args ...string) (string, string, error) {
	cmd := newRootCommand()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func executeCommandWithConfig(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `base_url = "https://meetings.example.test"
space_base_url = "https://space.example.test"
cookie = "session=abc"
`)

	return executeCommand(append([]string{"--config", configPath}, args...)...)
}

func TestRootCommandHelp(t *testing.T) {
	stdout, stderr, err := executeCommand()
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	for _, want := range []string{"openminutes", "Available Commands:", "delete", "get", "list", "upload", "--verbose"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want to contain %q", stdout, want)
		}
	}
	for _, notWant := range []string{"--toggle", "Help message for toggle"} {
		if strings.Contains(stdout, notWant) {
			t.Fatalf("stdout = %q, want not to contain %q", stdout, notWant)
		}
	}
}

func TestRootCommandVerboseHelpDoesNotRequireConfig(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := filepath.Join(t.TempDir(), "openminutes", "config.toml")
	withDefaultConfigPath(t, configPath)

	stdout, stderr, err := executeCommand("--verbose", "help")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if !strings.Contains(stderr, "command started") {
		t.Fatalf("stderr = %q, want command debug log", stderr)
	}
	if strings.Contains(stderr, "config load") || strings.Contains(stderr, "config loaded") {
		t.Fatalf("stderr = %q, want no config logs", stderr)
	}

	if !strings.Contains(stdout, "--verbose") {
		t.Fatalf("stdout = %q, want verbose flag", stdout)
	}

	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("config file stat error = %v, want not exist", statErr)
	}
}

func TestRootCommandHelpSubcommandDoesNotRequireConfig(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := filepath.Join(t.TempDir(), "openminutes", "config.toml")
	withDefaultConfigPath(t, configPath)

	stdout, stderr, err := executeCommand("help")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	if !strings.Contains(stdout, "Available Commands:") {
		t.Fatalf("stdout = %q, want available commands", stdout)
	}

	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("config file stat error = %v, want not exist", statErr)
	}
}

func TestRootCommandDeleteWithoutConfirmationDoesNotRequireConfig(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := filepath.Join(t.TempDir(), "openminutes", "config.toml")
	withDefaultConfigPath(t, configPath)

	stdout, stderr, err := executeCommand("delete", "token-1")
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}

	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "delete requires --yes") {
		t.Fatalf("stderr = %q, want delete requires --yes", stderr)
	}

	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("config file stat error = %v, want not exist", statErr)
	}
}

func TestRootCommandUnknownCommand(t *testing.T) {
	stdout, stderr, err := executeCommand("missing")
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}

	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}

	if !strings.Contains(stderr, `unknown command "missing"`) {
		t.Fatalf("stderr = %q, want unknown command error", stderr)
	}
}

func TestRootCommandVerboseUnknownCommandDoesNotRequireConfig(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := filepath.Join(t.TempDir(), "openminutes", "config.toml")
	withDefaultConfigPath(t, configPath)

	stdout, stderr, err := executeCommand("--verbose", "missing")
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}

	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}

	if !strings.Contains(stderr, `unknown command "missing"`) {
		t.Fatalf("stderr = %q, want unknown command error", stderr)
	}
	if strings.Contains(stderr, "DEBUG") {
		t.Fatalf("stderr = %q, want no debug logs for unknown command", stderr)
	}

	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("config file stat error = %v, want not exist", statErr)
	}
}

func TestRootCommandSubcommands(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			return &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{{
					ObjectToken: "token-1",
					Topic:       "Root",
					URL:         "https://example.test/root",
				}},
			}, nil
		}), nil
	})
	withDeleteMinutesClient(t, func(config minutes.Config) (deleteMinutesClient, error) {
		return deleteMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.DeleteOptions) error {
			return nil
		}), nil
	})
	withUploadMinutesClient(t, func(config minutes.Config) (uploadMinutesClient, error) {
		return uploadMinutesClientFunc(func(ctx context.Context, options minutes.UploadOptions) (*minutes.UploadResult, error) {
			return &minutes.UploadResult{ObjectToken: "object-root"}, nil
		}), nil
	})
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		return getMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
			return []byte("root subtitle\n"), nil
		}), nil
	})
	uploadPath := writeUploadFile(t, t.TempDir(), "root.aac", []byte("audio"))
	getOutputPath := filepath.Join(t.TempDir(), "root.txt")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "delete",
			args: []string{"delete", "token-root", "--yes"},
			want: "Moved token-root to trash\n",
		},
		{
			name: "get",
			args: []string{"get", "token-root", "--output", getOutputPath},
			want: "Saved token-root to " + getOutputPath + "\n",
		},
		{
			name: "list",
			args: []string{"list"},
			want: "token-1 Root https://example.test/root\n",
		},
		{
			name: "upload",
			args: []string{"upload", uploadPath},
			want: "Uploaded object-root https://meetings.example.test/minutes/object-root\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := executeCommandWithConfig(t, tt.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v, want nil", err)
			}

			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}

			if stdout != tt.want {
				t.Fatalf("stdout = %q, want %q", stdout, tt.want)
			}
		})
	}
}

func TestRootCommandVerboseListWritesDebugLogsToStderr(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		if config.Logger == nil {
			t.Fatal("config.Logger = nil, want verbose logger")
		}
		config.Logger.Debug("mock list client received logger")
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			return &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{{
					ObjectToken: "token-1",
					Topic:       "Verbose",
					URL:         "https://example.test/verbose",
				}},
			}, nil
		}), nil
	})

	stdout, stderr, err := executeCommandWithConfig(t, "--verbose", "list")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if stdout != "token-1 Verbose https://example.test/verbose\n" {
		t.Fatalf("stdout = %q, want list output only", stdout)
	}

	for _, want := range []string{"DEBUG", "command started", "config loaded", "list command completed", "mock list client received logger"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr = %q, want to contain %q", stderr, want)
		}
	}
	if strings.Contains(stderr, "session=abc") {
		t.Fatalf("stderr = %q, want no cookie value", stderr)
	}
}

func TestRootCommandVerboseListFlagAfterSubcommand(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		if config.Logger == nil {
			t.Fatal("config.Logger = nil, want verbose logger")
		}
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			return &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{{ObjectToken: "token-1", Topic: "After", URL: "https://example.test/after"}},
			}, nil
		}), nil
	})

	stdout, stderr, err := executeCommandWithConfig(t, "list", "--verbose")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if stdout != "token-1 After https://example.test/after\n" {
		t.Fatalf("stdout = %q, want list output only", stdout)
	}
	if !strings.Contains(stderr, "command started") {
		t.Fatalf("stderr = %q, want debug logs", stderr)
	}
}

func TestRootCommandSubcommandCreatesManualConfig(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := filepath.Join(t.TempDir(), "custom", "config.toml")

	stdout, stderr, err := executeCommand("--config", configPath, "list")
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}

	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}

	if !strings.Contains(stderr, "cookie is required") {
		t.Fatalf("stderr = %q, want cookie required", stderr)
	}

	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}

	if string(data) != configTemplate {
		t.Fatalf("config file = %q, want %q", data, configTemplate)
	}
}

func TestRootCommandSubcommandUsesEnvWithMissingManualConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "custom", "config.toml")
	t.Setenv("OPENMINUTES_BASE_URL", "https://env.example.test")
	t.Setenv("OPENMINUTES_SPACE_BASE_URL", "https://env-space.example.test")
	t.Setenv("OPENMINUTES_COOKIE", "session=env")
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			return &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{{
					ObjectToken: "token-env",
					Topic:       "Env",
					URL:         "https://example.test/env",
				}},
			}, nil
		}), nil
	})

	stdout, stderr, err := executeCommand("--config", configPath, "list")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	if stdout != "token-env Env https://example.test/env\n" {
		t.Fatalf("stdout = %q, want mocked list output", stdout)
	}

	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}

	if string(data) != configTemplate {
		t.Fatalf("config file = %q, want %q", data, configTemplate)
	}
}

func TestRootCommandStoresConfigInContext(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `base_url = "https://meetings.context.test"
space_base_url = "https://space.context.test"
cookie = "session=abc"
`)
	var gotConfig Config
	var hasConfig bool

	cmd := newRootCommand()
	cmd.AddCommand(&cobra.Command{
		Use: "inspect-config",
		Annotations: map[string]string{
			requiresConfigAnnotation: "true",
		},
		Run: func(cmd *cobra.Command, args []string) {
			gotConfig, hasConfig = configFromCommand(cmd)
		},
	})
	cmd.SetArgs([]string{"--config", configPath, "inspect-config"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if !hasConfig {
		t.Fatal("configFromCommand() ok = false, want true")
	}

	wantConfig := Config{
		BaseURL:      "https://meetings.context.test",
		SpaceBaseURL: "https://space.context.test",
		Cookie:       "session=abc",
	}
	if gotConfig != wantConfig {
		t.Fatalf("config = %#v, want %#v", gotConfig, wantConfig)
	}
}

func TestRootCommandVersion(t *testing.T) {
	oldVersion, oldCommit := version, commit
	t.Cleanup(func() {
		version = oldVersion
		commit = oldCommit
	})

	version = "v1.2.3"
	commit = "1234567890abcdef"

	stdout, stderr, err := executeCommand("--version")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	if want := "openminutes version v1.2.3-12345678\n"; stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRootCommandVerboseVersionDoesNotRequireConfig(t *testing.T) {
	withoutOpenMinutesEnv(t)

	oldVersion, oldCommit := version, commit
	t.Cleanup(func() {
		version = oldVersion
		commit = oldCommit
	})
	version = "v1.2.3"
	commit = "1234567890abcdef"

	configPath := filepath.Join(t.TempDir(), "openminutes", "config.toml")
	withDefaultConfigPath(t, configPath)

	stdout, stderr, err := executeCommand("--verbose", "--version")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if stdout != "openminutes version v1.2.3-12345678\n" {
		t.Fatalf("stdout = %q, want version output", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("config file stat error = %v, want not exist", statErr)
	}
}

func TestExecute(t *testing.T) {
	withoutOpenMinutesEnv(t)

	oldArgs := os.Args
	oldExit := exit
	os.Args = []string{"openminutes", "--version"}
	exit = func(code int) {
		t.Fatalf("exit called with code %d", code)
	}
	t.Cleanup(func() {
		os.Args = oldArgs
		exit = oldExit
	})

	Execute()
}

func TestExecuteCommand(t *testing.T) {
	withoutOpenMinutesEnv(t)

	cmd := newRootCommand()
	cmd.SetArgs([]string{"--version"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	execute(cmd)
}

func TestExecuteFailureExits(t *testing.T) {
	oldArgs := os.Args
	oldExit := exit
	exitCode := -1

	os.Args = []string{"openminutes", "missing"}
	exit = func(code int) {
		exitCode = code
	}
	t.Cleanup(func() {
		os.Args = oldArgs
		exit = oldExit
	})

	Execute()

	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
}
