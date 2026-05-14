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

type deleteMinutesClientFunc func(context.Context, string, minutes.DeleteOptions) error

func (f deleteMinutesClientFunc) DeleteMinute(ctx context.Context, objectToken string, options minutes.DeleteOptions) error {
	return f(ctx, objectToken, options)
}

func withDeleteMinutesClient(t *testing.T, factory func(minutes.Config) (deleteMinutesClient, error)) {
	t.Helper()

	oldFactory := newDeleteMinutesClient
	newDeleteMinutesClient = factory
	t.Cleanup(func() {
		newDeleteMinutesClient = oldFactory
	})
}

func executeDeleteCommand(t *testing.T, config Config, args ...string) (string, error) {
	t.Helper()

	cmd := newDeleteCommand()
	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetArgs(args)
	cmd.SetContext(contextWithConfig(context.Background(), config))

	err := cmd.Execute()
	return stdout.String(), err
}

func TestDeleteCommandReadsConfigAndCallsDeleteAPI(t *testing.T) {
	wantConfig := minutes.Config{
		Region: "feishu",
		Cookie: "session=abc",
	}
	ctxKey := struct{}{}
	ctx := context.WithValue(context.Background(), ctxKey, "marker")
	var gotConfig minutes.Config
	var gotToken string
	var gotOptions minutes.DeleteOptions
	var gotMarker any
	calls := 0

	withDeleteMinutesClient(t, func(config minutes.Config) (deleteMinutesClient, error) {
		gotConfig = config
		return deleteMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.DeleteOptions) error {
			calls++
			gotToken = objectToken
			gotOptions = options
			gotMarker = ctx.Value(ctxKey)
			return nil
		}), nil
	})

	cmd := newDeleteCommand()
	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetArgs([]string{"token-1", "--yes"})
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
		t.Fatalf("DeleteMinute() calls = %d, want 1", calls)
	}
	if gotToken != "token-1" {
		t.Fatalf("DeleteMinute() token = %q, want token-1", gotToken)
	}
	if !reflect.DeepEqual(gotOptions, minutes.DeleteOptions{}) {
		t.Fatalf("DeleteMinute() options = %#v, want zero options", gotOptions)
	}
	if gotMarker != "marker" {
		t.Fatalf("DeleteMinute() context marker = %#v, want marker", gotMarker)
	}
	if stdout.String() != "Moved token-1 to trash\n" {
		t.Fatalf("stdout = %q, want moved message", stdout.String())
	}
}

func TestDeleteCommandPassesDestroyOption(t *testing.T) {
	var gotOptions minutes.DeleteOptions
	withDeleteMinutesClient(t, func(config minutes.Config) (deleteMinutesClient, error) {
		return deleteMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.DeleteOptions) error {
			gotOptions = options
			return nil
		}), nil
	})

	stdout, err := executeDeleteCommand(t, Config{Region: "feishu", Cookie: "session=abc"}, "token-1", "--yes", "--destroy")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if !reflect.DeepEqual(gotOptions, minutes.DeleteOptions{Destroy: true}) {
		t.Fatalf("DeleteMinute() options = %#v, want destroy", gotOptions)
	}
	if stdout != "Permanently deleted token-1\n" {
		t.Fatalf("stdout = %q, want permanent delete message", stdout)
	}
}

func TestDeleteCommandDeletesMultipleTokensInOrder(t *testing.T) {
	var tokens []string
	withDeleteMinutesClient(t, func(config minutes.Config) (deleteMinutesClient, error) {
		return deleteMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.DeleteOptions) error {
			tokens = append(tokens, objectToken)
			return nil
		}), nil
	})

	stdout, err := executeDeleteCommand(t, Config{Region: "feishu", Cookie: "session=abc"}, "token-1", "token-2", "--yes")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if got := strings.Join(tokens, ","); got != "token-1,token-2" {
		t.Fatalf("DeleteMinute() tokens = %s, want token-1,token-2", got)
	}
	wantStdout := "Moved token-1 to trash\nMoved token-2 to trash\n"
	if stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", stdout, wantStdout)
	}
}

func TestDeleteCommandStopsOnFirstFailure(t *testing.T) {
	wantErr := errors.New("delete failed")
	var tokens []string
	withDeleteMinutesClient(t, func(config minutes.Config) (deleteMinutesClient, error) {
		return deleteMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.DeleteOptions) error {
			tokens = append(tokens, objectToken)
			if objectToken == "token-2" {
				return wantErr
			}
			return nil
		}), nil
	})

	stdout, err := executeDeleteCommand(t, Config{Region: "feishu", Cookie: "session=abc"}, "token-1", "token-2", "token-3", "--yes")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
	if got := strings.Join(tokens, ","); got != "token-1,token-2" {
		t.Fatalf("DeleteMinute() tokens = %s, want token-1,token-2", got)
	}
	if stdout != "Moved token-1 to trash\n" {
		t.Fatalf("stdout = %q, want first success only", stdout)
	}
}

func TestDeleteCommandRequiresConfirmationBeforeClient(t *testing.T) {
	clientCreated := false
	withDeleteMinutesClient(t, func(config minutes.Config) (deleteMinutesClient, error) {
		clientCreated = true
		return deleteMinutesClientFunc(func(ctx context.Context, objectToken string, options minutes.DeleteOptions) error {
			return nil
		}), nil
	})

	_, err := executeDeleteCommand(t, Config{Region: "feishu", Cookie: "session=abc"}, "token-1")
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if err.Error() != "delete requires --yes" {
		t.Fatalf("Execute() error = %q, want delete requires --yes", err.Error())
	}
	if clientCreated {
		t.Fatal("client created without --yes")
	}
}

func TestDeleteCommandRejectsMissingToken(t *testing.T) {
	clientCreated := false
	withDeleteMinutesClient(t, func(config minutes.Config) (deleteMinutesClient, error) {
		clientCreated = true
		return nil, errors.New("client should not be created")
	})

	_, err := executeDeleteCommand(t, Config{Region: "feishu", Cookie: "session=abc"}, "--yes")
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if err.Error() != "at least one token is required" {
		t.Fatalf("Execute() error = %q, want missing token error", err.Error())
	}
	if clientCreated {
		t.Fatal("client created without token")
	}
}

func TestDeleteCommandRejectsEmptyToken(t *testing.T) {
	_, err := executeDeleteCommand(t, Config{Region: "feishu", Cookie: "session=abc"}, " ", "--yes")
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if err.Error() != "object token is required" {
		t.Fatalf("Execute() error = %q, want object token error", err.Error())
	}
}

func TestDeleteCommandReturnsMissingConfigError(t *testing.T) {
	cmd := newDeleteCommand()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"token-1", "--yes"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if err.Error() != "config is required" {
		t.Fatalf("Execute() error = %q, want config is required", err.Error())
	}
}

func TestDeleteCommandReturnsClientError(t *testing.T) {
	wantErr := errors.New("client failed")
	withDeleteMinutesClient(t, func(config minutes.Config) (deleteMinutesClient, error) {
		return nil, wantErr
	})

	_, err := executeDeleteCommand(t, Config{Region: "feishu", Cookie: "session=abc"}, "token-1", "--yes")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}
