package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"openminutes/internal/minutes"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type getMinutesClientFunc func(context.Context, string, minutes.SubtitleOptions) ([]byte, error)

func (f getMinutesClientFunc) ExportSubtitle(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
	return f(ctx, objectToken, options)
}

func withGetMinutesClient(t *testing.T, factory func(minutes.Config) (getMinutesClient, error)) {
	t.Helper()

	oldFactory := newGetMinutesClient
	newGetMinutesClient = factory
	t.Cleanup(func() {
		newGetMinutesClient = oldFactory
	})
}

func withOpenGetOutputFile(t *testing.T, opener func(string) (getOutputFile, error)) {
	t.Helper()

	oldOpen := openGetOutputFile
	openGetOutputFile = opener
	t.Cleanup(func() {
		openGetOutputFile = oldOpen
	})
}

func executeGetCommand(t *testing.T, config Config, args ...string) (string, error) {
	t.Helper()

	cmd := newGetCommand()
	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetArgs(args)
	cmd.SetContext(contextWithConfig(context.Background(), config))

	err := cmd.Execute()
	return stdout.String(), err
}

func TestNewGetMinutesClientInitializesRealClient(t *testing.T) {
	client, err := newGetMinutesClient(minutes.Config{Cookie: "bv_csrf_token=token"})
	if err != nil {
		t.Fatalf("newGetMinutesClient() error = %v, want nil", err)
	}
	if client == nil {
		t.Fatal("newGetMinutesClient() client = nil, want client")
	}
}

func TestGetCommandDefaultExportWritesStdout(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	var gotToken string
	var gotOptions minutes.SubtitleOptions
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		return getMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
			gotToken = objectToken
			gotOptions = options
			return []byte("hello\nworld\n"), nil
		}), nil
	})

	stdout, err := executeGetCommand(t, testCommandConfig(), "token-1")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if gotToken != "token-1" {
		t.Fatalf("ExportSubtitle() token = %q, want token-1", gotToken)
	}
	if !reflect.DeepEqual(gotOptions, minutes.SubtitleOptions{Format: "txt"}) {
		t.Fatalf("ExportSubtitle() options = %#v, want default txt options", gotOptions)
	}
	if stdout != "hello\nworld\n" {
		t.Fatalf("stdout = %q, want exported text", stdout)
	}

	if _, err := os.Stat(filepath.Join(tempDir, "token-1.txt")); !os.IsNotExist(err) {
		t.Fatalf("implicit output file stat error = %v, want not exist", err)
	}
}

func TestGetCommandJSONExportWritesInlineContent(t *testing.T) {
	var gotToken string
	var gotOptions minutes.SubtitleOptions
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		return getMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
			gotToken = objectToken
			gotOptions = options
			return []byte("hello\nworld\n"), nil
		}), nil
	})

	stdout, err := executeGetCommand(t, testCommandConfig(),
		" token-1 ",
		"--file_type", " SRT ",
		"--speaker",
		"--timestamp",
		"--json",
	)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if gotToken != "token-1" {
		t.Fatalf("ExportSubtitle() token = %q, want token-1", gotToken)
	}
	wantOptions := minutes.SubtitleOptions{Format: "srt", AddSpeaker: true, AddTimestamp: true}
	if !reflect.DeepEqual(gotOptions, wantOptions) {
		t.Fatalf("ExportSubtitle() options = %#v, want %#v", gotOptions, wantOptions)
	}

	var got struct {
		ObjectToken string  `json:"object_token"`
		FileType    string  `json:"file_type"`
		Speaker     bool    `json:"speaker"`
		Timestamp   bool    `json:"timestamp"`
		Bytes       int     `json:"bytes"`
		Content     *string `json:"content"`
		OutputPath  string  `json:"output_path"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, stdout = %q", err, stdout)
	}

	if got.ObjectToken != "token-1" {
		t.Fatalf("object_token = %q, want token-1", got.ObjectToken)
	}
	if got.FileType != "srt" {
		t.Fatalf("file_type = %q, want srt", got.FileType)
	}
	if !got.Speaker {
		t.Fatal("speaker = false, want true")
	}
	if !got.Timestamp {
		t.Fatal("timestamp = false, want true")
	}
	if got.Bytes != len("hello\nworld\n") {
		t.Fatalf("bytes = %d, want %d", got.Bytes, len("hello\nworld\n"))
	}
	if got.Content == nil {
		t.Fatal("content = nil, want inline content")
	}
	if *got.Content != "hello\nworld\n" {
		t.Fatalf("content = %q, want exported content", *got.Content)
	}
	if got.OutputPath != "" {
		t.Fatalf("output_path = %q, want empty", got.OutputPath)
	}
}

func TestGetCommandJSONOutputWritesFileAndMetadata(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "subtitle.srt")
	data := []byte("1\n00:00:00,000 --> 00:00:01,000\nHi\n")
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		return getMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
			return data, nil
		}), nil
	})

	stdout, err := executeGetCommand(t, testCommandConfig(),
		"token-1",
		"--file_type", "srt",
		"--speaker",
		"--timestamp",
		"--json",
		"--output", outputPath,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	fileData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v, want nil", err)
	}
	if string(fileData) != string(data) {
		t.Fatalf("output file = %q, want exported bytes", fileData)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, stdout = %q", err, stdout)
	}
	if _, ok := raw["content"]; ok {
		t.Fatalf("content present in JSON metadata, want omitted: %q", stdout)
	}

	var got struct {
		ObjectToken string `json:"object_token"`
		FileType    string `json:"file_type"`
		Speaker     bool   `json:"speaker"`
		Timestamp   bool   `json:"timestamp"`
		Bytes       int    `json:"bytes"`
		OutputPath  string `json:"output_path"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, stdout = %q", err, stdout)
	}

	want := struct {
		ObjectToken string `json:"object_token"`
		FileType    string `json:"file_type"`
		Speaker     bool   `json:"speaker"`
		Timestamp   bool   `json:"timestamp"`
		Bytes       int    `json:"bytes"`
		OutputPath  string `json:"output_path"`
	}{
		ObjectToken: "token-1",
		FileType:    "srt",
		Speaker:     true,
		Timestamp:   true,
		Bytes:       len(data),
		OutputPath:  outputPath,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("json output = %#v, want %#v", got, want)
	}
}

func TestGetCommandJSONExportIncludesEmptyContent(t *testing.T) {
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		return getMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
			return []byte{}, nil
		}), nil
	})

	stdout, err := executeGetCommand(t, testCommandConfig(), "token-1", "--json")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	var got struct {
		Bytes   int     `json:"bytes"`
		Content *string `json:"content"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, stdout = %q", err, stdout)
	}
	if got.Bytes != 0 {
		t.Fatalf("bytes = %d, want 0", got.Bytes)
	}
	if got.Content == nil {
		t.Fatal("content = nil, want empty string")
	}
	if *got.Content != "" {
		t.Fatalf("content = %q, want empty string", *got.Content)
	}
}

func TestGetCommandDefaultStdoutAddsTrailingNewlineWhenMissing(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "missing newline", data: []byte("subtitle"), want: "subtitle\n"},
		{name: "already has newline", data: []byte("subtitle\n"), want: "subtitle\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
				return getMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
					return tt.data, nil
				}), nil
			})

			stdout, err := executeGetCommand(t, testCommandConfig(), "token-1")
			if err != nil {
				t.Fatalf("Execute() error = %v, want nil", err)
			}
			if stdout != tt.want {
				t.Fatalf("stdout = %q, want %q", stdout, tt.want)
			}
		})
	}
}

func TestGetCommandPassesCustomSubtitleOptions(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "subtitle.srt")
	var gotOptions minutes.SubtitleOptions
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		return getMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
			gotOptions = options
			return []byte("1\n00:00:00,000 --> 00:00:01,000\nHi\n"), nil
		}), nil
	})

	stdout, err := executeGetCommand(t, testCommandConfig(),
		"token-1",
		"--file_type", " SRT ",
		"--speaker",
		"--timestamp",
		"--output", outputPath,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	wantOptions := minutes.SubtitleOptions{Format: "srt", AddSpeaker: true, AddTimestamp: true}
	if !reflect.DeepEqual(gotOptions, wantOptions) {
		t.Fatalf("ExportSubtitle() options = %#v, want %#v", gotOptions, wantOptions)
	}
	if stdout != "Saved token-1 to "+outputPath+"\n" {
		t.Fatalf("stdout = %q, want saved message", stdout)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v, want nil", err)
	}
	if string(data) != "1\n00:00:00,000 --> 00:00:01,000\nHi\n" {
		t.Fatalf("output file = %q, want exported bytes", data)
	}
}

func TestGetCommandOutputShorthandWritesTextFile(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "subtitle.txt")
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		return getMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
			return []byte("shorthand subtitle\n"), nil
		}), nil
	})

	stdout, err := executeGetCommand(t, testCommandConfig(), "token-1", "-O", outputPath)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	if stdout != "Saved token-1 to "+outputPath+"\n" {
		t.Fatalf("stdout = %q, want saved message", stdout)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v, want nil", err)
	}
	if string(data) != "shorthand subtitle\n" {
		t.Fatalf("output file = %q, want exported bytes", data)
	}
}

func TestGetCommandReadsConfigLoggerAndCallsExportAPI(t *testing.T) {
	wantConfig := minutes.Config{
		BaseURL:      "https://meetings.example.test",
		SpaceBaseURL: "https://space.example.test",
		Cookie:       "session=abc",
	}
	outputPath := filepath.Join(t.TempDir(), "subtitle.txt")
	ctxKey := struct{}{}
	ctx := context.WithValue(context.Background(), ctxKey, "marker")
	logger := zap.NewNop()
	var gotConfig minutes.Config
	var gotToken string
	var gotMarker any
	calls := 0

	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		gotConfig = config
		return getMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
			calls++
			gotToken = objectToken
			gotMarker = ctx.Value(ctxKey)
			return []byte("subtitle"), nil
		}), nil
	})

	cmd := newGetCommand()
	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetArgs([]string{" token-1 ", "--output", outputPath})
	cmd.SetContext(contextWithLogger(contextWithConfig(ctx, Config{
		BaseURL:      wantConfig.BaseURL,
		SpaceBaseURL: wantConfig.SpaceBaseURL,
		Cookie:       wantConfig.Cookie,
	}), logger))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if gotConfig.Logger != logger {
		t.Fatalf("client logger = %#v, want context logger", gotConfig.Logger)
	}
	gotConfig.Logger = nil
	if gotConfig != wantConfig {
		t.Fatalf("client config = %#v, want %#v", gotConfig, wantConfig)
	}
	if calls != 1 {
		t.Fatalf("ExportSubtitle() calls = %d, want 1", calls)
	}
	if gotToken != "token-1" {
		t.Fatalf("ExportSubtitle() token = %q, want trimmed token", gotToken)
	}
	if gotMarker != "marker" {
		t.Fatalf("ExportSubtitle() context marker = %#v, want marker", gotMarker)
	}
	if stdout.String() != "Saved token-1 to "+outputPath+"\n" {
		t.Fatalf("stdout = %q, want saved message", stdout.String())
	}
}

func TestGetCommandRejectsInvalidArgsBeforeClientCreation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing token", args: []string{}, wantErr: "object token is required"},
		{name: "blank token", args: []string{" "}, wantErr: "object token is required"},
		{name: "extra token", args: []string{"token-1", "token-2"}, wantErr: "get accepts exactly one token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientCreated := false
			withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
				clientCreated = true
				return nil, errors.New("client should not be created")
			})

			_, err := executeGetCommand(t, testCommandConfig(), tt.args...)
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("Execute() error = %q, want %q", err.Error(), tt.wantErr)
			}
			if clientCreated {
				t.Fatal("client created for invalid args")
			}
		})
	}
}

func TestGetCommandRejectsInvalidFileTypeBeforeClientCreation(t *testing.T) {
	clientCreated := false
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		clientCreated = true
		return nil, errors.New("client should not be created")
	})

	_, err := executeGetCommand(t, testCommandConfig(), "token-1", "--file_type", "pdf", "--output", filepath.Join(t.TempDir(), "out.txt"))
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if err.Error() != `unsupported file_type "pdf": must be txt or srt` {
		t.Fatalf("Execute() error = %q, want unsupported file_type", err.Error())
	}
	if clientCreated {
		t.Fatal("client created for invalid file_type")
	}
}

func TestGetCommandRejectsExplicitEmptyOutputBeforeClientCreation(t *testing.T) {
	clientCreated := false
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		clientCreated = true
		return nil, errors.New("client should not be created")
	})

	_, err := executeGetCommand(t, testCommandConfig(), "token-1", "--output", " ")
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if err.Error() != "output path is required" {
		t.Fatalf("Execute() error = %q, want output path is required", err.Error())
	}
	if clientCreated {
		t.Fatal("client created for empty output")
	}
}

func TestGetCommandRejectsExistingOutputBeforeClientCreation(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(outputPath, []byte("existing"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	clientCreated := false
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		clientCreated = true
		return nil, errors.New("client should not be created")
	})

	_, err := executeGetCommand(t, testCommandConfig(), "token-1", "--output", outputPath)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Execute() error = %q, want already exists", err.Error())
	}
	if clientCreated {
		t.Fatal("client created for existing output")
	}

	data, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if string(data) != "existing" {
		t.Fatalf("existing output = %q, want unchanged", data)
	}
}

func TestGetCommandReturnsMissingConfigError(t *testing.T) {
	clientCreated := false
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		clientCreated = true
		return nil, errors.New("client should not be created")
	})

	cmd := newGetCommand()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"token-1", "--output", filepath.Join(t.TempDir(), "out.txt")})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if err.Error() != "config is required" {
		t.Fatalf("Execute() error = %q, want config is required", err.Error())
	}
	if clientCreated {
		t.Fatal("client created without config")
	}
}

func TestGetCommandReturnsClientFactoryError(t *testing.T) {
	wantErr := errors.New("client failed")
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		return nil, wantErr
	})

	outputPath := filepath.Join(t.TempDir(), "out.txt")
	_, err := executeGetCommand(t, testCommandConfig(), "token-1", "--output", outputPath)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("output file stat error = %v, want not exist", statErr)
	}
}

func TestGetCommandReturnsExportError(t *testing.T) {
	wantErr := errors.New("export failed")
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		return getMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
			return nil, wantErr
		}), nil
	})

	outputPath := filepath.Join(t.TempDir(), "out.txt")
	_, err := executeGetCommand(t, testCommandConfig(), "token-1", "--output", outputPath)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("output file stat error = %v, want not exist", statErr)
	}
}

func TestGetCommandReturnsOutputWriteError(t *testing.T) {
	clientCalled := false
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		return getMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
			clientCalled = true
			return []byte("subtitle"), nil
		}), nil
	})

	outputPath := filepath.Join(t.TempDir(), "missing", "out.txt")
	_, err := executeGetCommand(t, testCommandConfig(), "token-1", "--output", outputPath)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("Execute() error = %v, want not exist error", err)
	}
	if !clientCalled {
		t.Fatal("client was not called before output write error")
	}
}

func TestRunGetCommandReturnsValidationError(t *testing.T) {
	err := runGetCommand(&cobra.Command{}, []string{})
	if err == nil {
		t.Fatal("runGetCommand() error = nil, want error")
	}
	if err.Error() != "object token is required" {
		t.Fatalf("runGetCommand() error = %q, want object token is required", err.Error())
	}
}

func TestRunGetCommandReturnsFlagErrors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T) *cobra.Command
		want  string
	}{
		{
			name: "missing file_type flag",
			setup: func(t *testing.T) *cobra.Command {
				return &cobra.Command{}
			},
			want: "file_type",
		},
		{
			name: "missing speaker flag",
			setup: func(t *testing.T) *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().String("file_type", "txt", "")
				cmd.Flags().String("output", "", "")
				if err := cmd.Flags().Set("output", filepath.Join(t.TempDir(), "out.txt")); err != nil {
					t.Fatalf("Set(output) error = %v", err)
				}
				return cmd
			},
			want: "speaker",
		},
		{
			name: "missing timestamp flag",
			setup: func(t *testing.T) *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().String("file_type", "txt", "")
				cmd.Flags().String("output", "", "")
				cmd.Flags().Bool("speaker", false, "")
				if err := cmd.Flags().Set("output", filepath.Join(t.TempDir(), "out.txt")); err != nil {
					t.Fatalf("Set(output) error = %v", err)
				}
				return cmd
			},
			want: "timestamp",
		},
		{
			name: "missing json flag",
			setup: func(t *testing.T) *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().String("file_type", "txt", "")
				cmd.Flags().String("output", "", "")
				cmd.Flags().Bool("speaker", false, "")
				cmd.Flags().Bool("timestamp", false, "")
				return cmd
			},
			want: "json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setup(t)
			cmd.SetContext(context.Background())

			err := runGetCommand(cmd, []string{"token-1"})
			if err == nil {
				t.Fatal("runGetCommand() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("runGetCommand() error = %q, want to contain %q", err.Error(), tt.want)
			}
		})
	}
}

func TestGetCommandReturnsDefaultStdoutError(t *testing.T) {
	wantErr := errors.New("stdout failed")
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		return getMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
			return []byte("subtitle"), nil
		}), nil
	})

	cmd := newGetCommand()
	cmd.SetOut(errorWriter{err: wantErr})
	cmd.SetArgs([]string{"token-1"})
	cmd.SetContext(contextWithConfig(context.Background(), testCommandConfig()))

	err := cmd.Execute()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestGetCommandReturnsJSONStdoutError(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "inline content", args: []string{"token-1", "--json"}},
		{name: "output metadata", args: []string{"token-1", "--json", "--output", filepath.Join(t.TempDir(), "out.txt")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantErr := errors.New("stdout failed")
			withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
				return getMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
					return []byte("subtitle"), nil
				}), nil
			})

			cmd := newGetCommand()
			cmd.SetOut(errorWriter{err: wantErr})
			cmd.SetArgs(tt.args)
			cmd.SetContext(contextWithConfig(context.Background(), testCommandConfig()))

			err := cmd.Execute()
			if !errors.Is(err, wantErr) {
				t.Fatalf("Execute() error = %v, want %v", err, wantErr)
			}
		})
	}
}

func TestGetCommandReturnsSaveMessageStdoutError(t *testing.T) {
	wantErr := errors.New("stdout failed")
	outputPath := filepath.Join(t.TempDir(), "out.txt")
	withGetMinutesClient(t, func(config minutes.Config) (getMinutesClient, error) {
		return getMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.SubtitleOptions) ([]byte, error) {
			return []byte("subtitle"), nil
		}), nil
	})

	cmd := newGetCommand()
	cmd.SetOut(errorWriter{err: wantErr})
	cmd.SetArgs([]string{"token-1", "--output", outputPath})
	cmd.SetContext(contextWithConfig(context.Background(), testCommandConfig()))

	err := cmd.Execute()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestGetOutputPathReturnsMissingFlagError(t *testing.T) {
	_, err := getOutputPath(&cobra.Command{}, "token-1", "txt")
	if err == nil {
		t.Fatal("getOutputPath() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "output") {
		t.Fatalf("getOutputPath() error = %q, want output flag error", err.Error())
	}
}

func TestGetOutputPathReturnsEmptyWhenOutputNotExplicit(t *testing.T) {
	outputPath, err := getOutputPath(newGetCommand(), "token-1", "txt")
	if err != nil {
		t.Fatalf("getOutputPath() error = %v, want nil", err)
	}
	if outputPath != "" {
		t.Fatalf("getOutputPath() = %q, want empty", outputPath)
	}
}

func TestWriteGetStdoutReturnsWriteErrors(t *testing.T) {
	writeErr := errors.New("write failed")

	tests := []struct {
		name    string
		writer  *scriptedWriter
		wantErr error
	}{
		{
			name: "short data write",
			writer: &scriptedWriter{writes: []scriptedWrite{
				{n: 1},
			}},
			wantErr: io.ErrShortWrite,
		},
		{
			name: "newline write error",
			writer: &scriptedWriter{writes: []scriptedWrite{
				{full: true},
				{err: writeErr},
			}},
			wantErr: writeErr,
		},
		{
			name: "short newline write",
			writer: &scriptedWriter{writes: []scriptedWrite{
				{full: true},
				{n: 0},
			}},
			wantErr: io.ErrShortWrite,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := writeGetStdout(tt.writer, []byte("subtitle"))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("writeGetStdout() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestEnsureGetOutputDoesNotExistReturnsStatError(t *testing.T) {
	err := ensureGetOutputDoesNotExist("bad\x00path")
	if err == nil {
		t.Fatal("ensureGetOutputDoesNotExist() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "stat output file") {
		t.Fatalf("ensureGetOutputDoesNotExist() error = %q, want stat error", err.Error())
	}
}

func TestWriteGetOutputFileReturnsWriteAndCloseErrors(t *testing.T) {
	writeErr := errors.New("write failed")
	closeErr := errors.New("close failed")

	tests := []struct {
		name    string
		file    *fakeGetOutputFile
		wantErr error
	}{
		{
			name:    "write error",
			file:    &fakeGetOutputFile{writeErr: writeErr},
			wantErr: writeErr,
		},
		{
			name:    "short write",
			file:    &fakeGetOutputFile{writeN: 1},
			wantErr: io.ErrShortWrite,
		},
		{
			name:    "close error",
			file:    &fakeGetOutputFile{writeN: len("subtitle"), closeErr: closeErr},
			wantErr: closeErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withOpenGetOutputFile(t, func(string) (getOutputFile, error) {
				return tt.file, nil
			})

			err := writeGetOutputFile(filepath.Join(t.TempDir(), "out.txt"), []byte("subtitle"))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("writeGetOutputFile() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type scriptedWrite struct {
	n    int
	full bool
	err  error
}

type scriptedWriter struct {
	writes []scriptedWrite
	calls  int
}

func (w *scriptedWriter) Write(p []byte) (int, error) {
	if w.calls >= len(w.writes) {
		return len(p), nil
	}

	result := w.writes[w.calls]
	w.calls++
	if result.full {
		return len(p), result.err
	}
	return result.n, result.err
}

type fakeGetOutputFile struct {
	writeN   int
	writeErr error
	closeErr error
}

func (f *fakeGetOutputFile) Write([]byte) (int, error) {
	return f.writeN, f.writeErr
}

func (f *fakeGetOutputFile) Close() error {
	return f.closeErr
}
