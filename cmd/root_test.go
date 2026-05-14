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

	configPath := writeConfig(t, `region = "feishu"
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

	for _, want := range []string{"openminutes", "Available Commands:", "get", "list", "upload"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want to contain %q", stdout, want)
		}
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

func TestRootCommandSubcommands(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
			return []minutes.Minute{{
				ObjectToken: "token-1",
				Topic:       "Root",
				URL:         "https://example.test/root",
			}}, nil
		}), nil
	})

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "get",
			args: []string{"get"},
			want: "get called\n",
		},
		{
			name: "list",
			args: []string{"list"},
			want: "token-1 Root https://example.test/root\n",
		},
		{
			name: "upload",
			args: []string{"upload"},
			want: "upload called\n",
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
	t.Setenv("OPENMINUTES_REGION", "larksuite")
	t.Setenv("OPENMINUTES_COOKIE", "session=env")
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
			return []minutes.Minute{{
				ObjectToken: "token-env",
				Topic:       "Env",
				URL:         "https://example.test/env",
			}}, nil
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

	configPath := writeConfig(t, `region = "larksuite"
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

	wantConfig := Config{Region: "larksuite", Cookie: "session=abc"}
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

func TestExecute(t *testing.T) {
	withoutOpenMinutesEnv(t)

	oldArgs := os.Args
	oldExit := exit
	configPath := writeConfig(t, `region = "feishu"
cookie = "session=abc"
`)
	os.Args = []string{"openminutes", "--config", configPath, "get"}
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
	configPath := writeConfig(t, `region = "feishu"
cookie = "session=abc"
`)
	cmd.SetArgs([]string{"--config", configPath, "get"})
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
