package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"openminutes/internal/minutes"
)

type listMinutesClientFunc func(context.Context, minutes.ListOptions) (*minutes.ListMinutesPageResult, error)

func (f listMinutesClientFunc) ListMinutesPage(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
	return f(ctx, options)
}

func (f listMinutesClientFunc) ListMinutes(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
	result, err := f(ctx, options)
	if err != nil || result == nil {
		return nil, err
	}

	return result.Items, nil
}

type listMinutesClientStub struct {
	listPage func(context.Context, minutes.ListOptions) (*minutes.ListMinutesPageResult, error)
	listAll  func(context.Context, minutes.ListOptions) ([]minutes.Minute, error)
}

func (s listMinutesClientStub) ListMinutesPage(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
	if s.listPage == nil {
		return nil, errors.New("ListMinutesPage() should not be called")
	}

	return s.listPage(ctx, options)
}

func (s listMinutesClientStub) ListMinutes(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
	if s.listAll == nil {
		return nil, errors.New("ListMinutes() should not be called")
	}

	return s.listAll(ctx, options)
}

func withListMinutesClient(t *testing.T, factory func(minutes.Config) (listMinutesClient, error)) {
	t.Helper()

	oldFactory := newListMinutesClient
	newListMinutesClient = factory
	t.Cleanup(func() {
		newListMinutesClient = oldFactory
	})
}

func executeListCommand(t *testing.T, config Config, args ...string) (string, error) {
	t.Helper()

	cmd := newListCommand()
	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetArgs(args)
	cmd.SetContext(contextWithConfig(context.Background(), config))

	err := cmd.Execute()
	return stdout.String(), err
}

func TestListCommandReadsConfigAndCallsListAPI(t *testing.T) {
	wantConfig := minutes.Config{
		BaseURL:      "https://meetings.example.test",
		SpaceBaseURL: "https://space.example.test",
		Cookie:       "session=abc",
	}
	ctxKey := struct{}{}
	ctx := context.WithValue(context.Background(), ctxKey, "marker")
	var gotConfig minutes.Config
	var gotOptions minutes.ListOptions
	var gotMarker any
	calls := 0

	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		gotConfig = config
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			calls++
			gotOptions = options
			gotMarker = ctx.Value(ctxKey)
			return &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{{
					ObjectToken: "token-1",
					Topic:       "First",
					URL:         "https://example.test/minutes/token-1",
				}},
			}, nil
		}), nil
	})

	cmd := newListCommand()
	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetArgs([]string{})
	cmd.SetContext(contextWithConfig(ctx, Config{
		BaseURL:      wantConfig.BaseURL,
		SpaceBaseURL: wantConfig.SpaceBaseURL,
		Cookie:       wantConfig.Cookie,
	}))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if gotConfig != wantConfig {
		t.Fatalf("client config = %#v, want %#v", gotConfig, wantConfig)
	}
	if calls != 1 {
		t.Fatalf("ListMinutesPage() calls = %d, want 1", calls)
	}
	if gotMarker != "marker" {
		t.Fatalf("ListMinutesPage() context marker = %#v, want marker", gotMarker)
	}
	if !reflect.DeepEqual(gotOptions, minutes.ListOptions{Size: 20}) {
		t.Fatalf("ListMinutesPage() options = %#v, want default size", gotOptions)
	}
}

func TestListCommandPassesCustomPaginationOptions(t *testing.T) {
	var gotOptions minutes.ListOptions
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			gotOptions = options
			return &minutes.ListMinutesPageResult{}, nil
		}), nil
	})

	_, err := executeListCommand(t, testCommandConfig(), "--size", "50", "--timestamp", "100")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	want := minutes.ListOptions{Size: 50, Timestamp: 100}
	if !reflect.DeepEqual(gotOptions, want) {
		t.Fatalf("ListMinutesPage() options = %#v, want %#v", gotOptions, want)
	}
}

func TestListCommandPrintsMinutesInOrder(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			return &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{
					{ObjectToken: "token-1", Topic: "First", URL: "https://example.test/first"},
					{ObjectToken: "token-2", Topic: "Second", URL: "https://example.test/second"},
				},
			}, nil
		}), nil
	})

	stdout, err := executeListCommand(t, testCommandConfig())
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

func TestListCommandAllPrintsAllMinutesWithoutNextPageFooter(t *testing.T) {
	var gotOptions minutes.ListOptions
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientStub{
			listAll: func(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
				gotOptions = options
				return []minutes.Minute{
					{ObjectToken: "token-1", Topic: "First", URL: "https://example.test/first"},
					{ObjectToken: "token-2", Topic: "Second", URL: "https://example.test/second"},
				}, nil
			},
		}, nil
	})

	stdout, err := executeListCommand(t, testCommandConfig(), "--all")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	wantOptions := minutes.ListOptions{Size: 20}
	if !reflect.DeepEqual(gotOptions, wantOptions) {
		t.Fatalf("ListMinutes() options = %#v, want %#v", gotOptions, wantOptions)
	}
	want := strings.Join([]string{
		"token-1 First https://example.test/first",
		"token-2 Second https://example.test/second",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if strings.Contains(stdout, "Next page:") {
		t.Fatalf("stdout = %q, want no next page footer", stdout)
	}
}

func TestListCommandAllPassesCustomPaginationOptions(t *testing.T) {
	var gotOptions minutes.ListOptions
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientStub{
			listAll: func(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
				gotOptions = options
				return nil, nil
			},
		}, nil
	})

	_, err := executeListCommand(t, testCommandConfig(), "--all", "--size", "50", "--timestamp", "100")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	want := minutes.ListOptions{Size: 50, Timestamp: 100}
	if !reflect.DeepEqual(gotOptions, want) {
		t.Fatalf("ListMinutes() options = %#v, want %#v", gotOptions, want)
	}
}

func TestListCommandAllPrintsJSONWithoutPaginationMetadata(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientStub{
			listAll: func(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
				return []minutes.Minute{
					{
						ObjectToken: "token-1",
						Topic:       "First",
						URL:         "https://example.test/first",
						ShareTime:   200,
					},
					{
						ObjectToken: "token-2",
						Topic:       "Second",
						URL:         "https://example.test/second",
						ShareTime:   100,
					},
				}, nil
			},
		}, nil
	})

	stdout, err := executeListCommand(t, testCommandConfig(), "--all", "--json")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	var got struct {
		Items         []minutes.Minute `json:"items"`
		HasMore       bool             `json:"has_more"`
		NextTimestamp int64            `json:"next_timestamp"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, stdout = %q", err, stdout)
	}

	wantItems := []minutes.Minute{
		{
			ObjectToken: "token-1",
			Topic:       "First",
			URL:         "https://example.test/first",
			ShareTime:   200,
		},
		{
			ObjectToken: "token-2",
			Topic:       "Second",
			URL:         "https://example.test/second",
			ShareTime:   100,
		},
	}
	if !reflect.DeepEqual(got.Items, wantItems) {
		t.Fatalf("items = %#v, want %#v", got.Items, wantItems)
	}
	if got.HasMore {
		t.Fatal("has_more = true, want false")
	}
	if got.NextTimestamp != 0 {
		t.Fatalf("next_timestamp = %d, want 0", got.NextTimestamp)
	}
}

func TestListCommandPrintsJSON(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			return &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{
					{
						ObjectToken: "token-1",
						Topic:       "First",
						URL:         "https://example.test/first",
						ShareTime:   200,
					},
					{
						ObjectToken: "token-2",
						Topic:       "Second",
						URL:         "https://example.test/second",
						ShareTime:   100,
					},
				},
				HasMore:       true,
				NextTimestamp: 100,
			}, nil
		}), nil
	})

	stdout, err := executeListCommand(t, testCommandConfig(), "--json")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	var got struct {
		Items         []minutes.Minute `json:"items"`
		HasMore       bool             `json:"has_more"`
		NextTimestamp int64            `json:"next_timestamp"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, stdout = %q", err, stdout)
	}

	want := struct {
		Items         []minutes.Minute `json:"items"`
		HasMore       bool             `json:"has_more"`
		NextTimestamp int64            `json:"next_timestamp"`
	}{
		Items: []minutes.Minute{
			{
				ObjectToken: "token-1",
				Topic:       "First",
				URL:         "https://example.test/first",
				ShareTime:   200,
			},
			{
				ObjectToken: "token-2",
				Topic:       "Second",
				URL:         "https://example.test/second",
				ShareTime:   100,
			},
		},
		HasMore:       true,
		NextTimestamp: 100,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("json output = %#v, want %#v", got, want)
	}
}

func TestListCommandPrintsJSONForEmptyResult(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			return &minutes.ListMinutesPageResult{}, nil
		}), nil
	})

	stdout, err := executeListCommand(t, testCommandConfig(), "--json")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if strings.Contains(stdout, "No minutes found.") {
		t.Fatalf("stdout = %q, want JSON without empty message", stdout)
	}

	var got struct {
		Items         []minutes.Minute `json:"items"`
		HasMore       bool             `json:"has_more"`
		NextTimestamp int64            `json:"next_timestamp"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, stdout = %q", err, stdout)
	}
	if got.Items == nil {
		t.Fatal("items = nil, want empty slice")
	}
	if len(got.Items) != 0 {
		t.Fatalf("items = %#v, want empty slice", got.Items)
	}
	if got.HasMore {
		t.Fatal("has_more = true, want false")
	}
	if got.NextTimestamp != 0 {
		t.Fatalf("next_timestamp = %d, want 0", got.NextTimestamp)
	}
}

func TestListCommandPrintsEmptyMessage(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			return &minutes.ListMinutesPageResult{}, nil
		}), nil
	})

	stdout, err := executeListCommand(t, testCommandConfig())
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if stdout != "No minutes found.\n" {
		t.Fatalf("stdout = %q, want empty message", stdout)
	}
}

func TestListCommandPrintsFallbackTopicAndURL(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			return &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{{ObjectToken: "token-1"}},
			}, nil
		}), nil
	})

	config := testCommandConfig()
	config.BaseURL = "https://meetings.custom.test"

	stdout, err := executeListCommand(t, config)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	want := "token-1 (untitled) https://meetings.custom.test/minutes/token-1\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestListCommandPrintsNextPageFooter(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			return &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{{
					ObjectToken: "token-1",
					Topic:       "First",
					URL:         "https://example.test/first",
				}},
				HasMore:       true,
				NextTimestamp: 123,
			}, nil
		}), nil
	})

	stdout, err := executeListCommand(t, testCommandConfig(), "--size", "50")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	want := strings.Join([]string{
		"token-1 First https://example.test/first",
		"Next page: openminutes list --size 50 --timestamp 123",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestListCommandDoesNotPrintNextPageFooterWhenHasMoreFalse(t *testing.T) {
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			return &minutes.ListMinutesPageResult{
				Items: []minutes.Minute{{
					ObjectToken: "token-1",
					Topic:       "First",
					URL:         "https://example.test/first",
				}},
				NextTimestamp: 123,
			}, nil
		}), nil
	})

	stdout, err := executeListCommand(t, testCommandConfig())
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	if strings.Contains(stdout, "Next page:") {
		t.Fatalf("stdout = %q, want no next page footer", stdout)
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

	_, err := executeListCommand(t, testCommandConfig())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestListCommandReturnsListError(t *testing.T) {
	wantErr := errors.New("list failed")
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
			return nil, wantErr
		}), nil
	})

	_, err := executeListCommand(t, testCommandConfig())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestListCommandAllReturnsListError(t *testing.T) {
	wantErr := errors.New("list all failed")
	withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
		return listMinutesClientStub{
			listAll: func(ctx context.Context, options minutes.ListOptions) ([]minutes.Minute, error) {
				return nil, wantErr
			},
		}, nil
	})

	_, err := executeListCommand(t, testCommandConfig(), "--all")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestListCommandRejectsInvalidPaginationOptions(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "zero size",
			args:    []string{"--size", "0"},
			wantErr: "size must be greater than 0",
		},
		{
			name:    "negative size",
			args:    []string{"--size", "-1"},
			wantErr: "size must be greater than 0",
		},
		{
			name:    "negative timestamp",
			args:    []string{"--timestamp", "-1"},
			wantErr: "timestamp must be greater than or equal to 0",
		},
		{
			name:    "zero size with all",
			args:    []string{"--all", "--size", "0"},
			wantErr: "size must be greater than 0",
		},
		{
			name:    "negative timestamp with all",
			args:    []string{"--all", "--timestamp", "-1"},
			wantErr: "timestamp must be greater than or equal to 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientCreated := false
			withListMinutesClient(t, func(config minutes.Config) (listMinutesClient, error) {
				clientCreated = true
				return listMinutesClientFunc(func(ctx context.Context, options minutes.ListOptions) (*minutes.ListMinutesPageResult, error) {
					return nil, fmt.Errorf("ListMinutesPage() should not be called")
				}), nil
			})

			_, err := executeListCommand(t, testCommandConfig(), tt.args...)
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("Execute() error = %q, want %q", err.Error(), tt.wantErr)
			}
			if clientCreated {
				t.Fatal("client created for invalid pagination options")
			}
		})
	}
}
