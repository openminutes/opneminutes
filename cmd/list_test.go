package cmd

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"openminutes/internal/minutes"
)

type listMinutesClientFunc func(context.Context, minutes.ListOptions) ([]minutes.Minute, error)

func (f listMinutesClientFunc) ListMinutes(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
	return f(ctx, options)
}

func withListMinutesClient(t *testing.T, factory func(minutes.Config) (listMinutesClient, error)) {
	t.Helper()

	oldFactory := newListMinutesClient
	newListMinutesClient = factory
	t.Cleanup(func() {
		newListMinutesClient = oldFactory
	})
}

func executeListCommand(t *testing.T, config Config) (string, error) {
	t.Helper()

	cmd := newListCommand()
	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetArgs([]string{})
	cmd.SetContext(contextWithConfig(context.Background(), config))

	err := cmd.Execute()
	return stdout.String(), err
}

func TestListCommandReadsConfigAndCallsListAPI(t *testing.T) {
	wantConfig := minutes.Config{
		Region: "feishu",
		Cookie: "session=abc",
	}
	ctxKey := struct{}{}
	ctx := context.WithValue(context.Background(), ctxKey, "marker")
	var gotConfig minutes.Config
	var gotOptions minutes.ListOptions
	var gotMarker any
	calls := 0

	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		gotConfig = config
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
			calls++
			gotOptions = options
			gotMarker = ctx.Value(ctxKey)
			return []minutes.Minute{{
				ObjectToken: "token-1",
				Topic:       "First",
				URL:         "https://example.test/minutes/token-1",
			}}, nil
		}), nil
	})

	cmd := newListCommand()
	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetArgs([]string{})
	cmd.SetContext(contextWithConfig(ctx, Config{
		Region: wantConfig.Region,
		Cookie: wantConfig.Cookie,
	}))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if gotConfig != wantConfig {
		t.Fatalf("client config = %#v, want %#v", gotConfig, wantConfig)
	}
	if calls != 1 {
		t.Fatalf("ListMinutes() calls = %d, want 1", calls)
	}
	if gotMarker != "marker" {
		t.Fatalf("ListMinutes() context marker = %#v, want marker", gotMarker)
	}
	if !reflect.DeepEqual(gotOptions, minutes.ListOptions{}) {
		t.Fatalf("ListMinutes() options = %#v, want zero value", gotOptions)
	}
}

func TestListCommandPrintsMinutesInOrder(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
			return []minutes.Minute{
				{ObjectToken: "token-1", Topic: "First", URL: "https://example.test/first"},
				{ObjectToken: "token-2", Topic: "Second", URL: "https://example.test/second"},
			}, nil
		}), nil
	})

	stdout, err := executeListCommand(t, Config{Region: "feishu", Cookie: "session=abc"})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	want := strings.Join([]string{
		"token-1 First https://example.test/first",
		"token-2 Second https://example.test/second",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestListCommandPrintsEmptyMessage(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
			return nil, nil
		}), nil
	})

	stdout, err := executeListCommand(t, Config{Region: "feishu", Cookie: "session=abc"})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if stdout != "No minutes found.\n" {
		t.Fatalf("stdout = %q, want empty message", stdout)
	}
}

func TestListCommandPrintsFallbackTopicAndURL(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
			return []minutes.Minute{{ObjectToken: "token-1"}}, nil
		}), nil
	})

	stdout, err := executeListCommand(t, Config{Region: "feishu", Cookie: "session=abc"})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	want := "token-1 (untitled) https://meetings.feishu.cn/minutes/token-1\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestListCommandReturnsMissingConfigError(t *testing.T) {
	cmd := newListCommand()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if err.Error() != "config is required" {
		t.Fatalf("Execute() error = %q, want config is required", err.Error())
	}
}

func TestListCommandReturnsClientError(t *testing.T) {
	wantErr := errors.New("client failed")
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return nil, wantErr
	})

	_, err := executeListCommand(t, Config{Region: "feishu", Cookie: "session=abc"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestListCommandReturnsListError(t *testing.T) {
	wantErr := errors.New("list failed")
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
			return nil, wantErr
		}), nil
	})

	_, err := executeListCommand(t, Config{Region: "feishu", Cookie: "session=abc"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}
