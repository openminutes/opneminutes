package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"openminutes/internal/config"
	"openminutes/internal/minutes"

	"github.com/spf13/cobra"
)

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
	if _, err := newUploadMinutesClient(config); err != nil {
		t.Fatalf("newUploadMinutesClient() error = %v, want nil", err)
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
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

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
	got, ok := configFromCommand(cmd)
	if !ok {
		t.Fatal("configFromCommand() ok = false, want true")
	}
	want := config.Config{
		BaseURL:      "https://env.example.test",
		SpaceBaseURL: "https://env-space.example.test",
		Cookie:       "session=env",
	}
	if got != want {
		t.Fatalf("config = %#v, want env values", got)
	}
}

func TestListCommandReturnsStdoutWriteErrors(t *testing.T) {
	tests := []struct {
		name   string
		result *minutes.ListMinutesPageResult
		writer *failingWriter
	}{
		{
			name: "header",
			result: &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{{ObjectToken: "token-1", Topic: "First", URL: "https://example.test/first"}},
			},
			writer: &failingWriter{failAt: 1},
		},
		{
			name: "row",
			result: &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{{ObjectToken: "token-1", Topic: "First", URL: "https://example.test/first"}},
			},
			writer: &failingWriter{failAt: 2},
		},
		{
			name: "footer spacer",
			result: &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{{ObjectToken: "token-1", Topic: "First", URL: "https://example.test/first"}},
			},
			writer: &failingWriter{failAt: 3},
		},
		{
			name: "next page footer",
			result: &minutes.ListMinutesPageResult{
				Items:         []minutes.Minute{{ObjectToken: "token-1", Topic: "First", URL: "https://example.test/first"}},
				HasMore:       true,
				NextTimestamp: 100,
			},
			writer: &failingWriter{failAt: 4},
		},
		{
			name: "get content footer",
			result: &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{{ObjectToken: "token-1", Topic: "First", URL: "https://example.test/first"}},
			},
			writer: &failingWriter{failAt: 4},
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
	cmd.SetContext(contextWithConfig(context.Background(), config.Config{BaseURL: config.DefaultBaseURL, SpaceBaseURL: config.DefaultSpaceBaseURL, Cookie: "session=abc"}))

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
