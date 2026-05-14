package cmd

import (
	"context"
	"testing"

	"openminutes/internal/minutes"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func TestRuntimeFromCommandBuildsClientConfig(t *testing.T) {
	logger := zap.NewNop()
	config := Config{
		BaseURL:      "https://meetings.example.test",
		SpaceBaseURL: "https://space.example.test",
		Cookie:       "session=abc",
	}
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(contextWithLogger(contextWithConfig(context.Background(), config), logger))

	runtime, err := runtimeFromCommand(cmd)
	if err != nil {
		t.Fatalf("runtimeFromCommand() error = %v, want nil", err)
	}
	if runtime.Config != config {
		t.Fatalf("runtime config = %#v, want %#v", runtime.Config, config)
	}
	wantClientConfig := minutes.Config{
		BaseURL:      config.BaseURL,
		SpaceBaseURL: config.SpaceBaseURL,
		Cookie:       config.Cookie,
		Logger:       logger,
	}
	if runtime.ClientConfig != wantClientConfig {
		t.Fatalf("client config = %#v, want %#v", runtime.ClientConfig, wantClientConfig)
	}
	if runtime.Logger != logger {
		t.Fatalf("runtime logger = %#v, want context logger", runtime.Logger)
	}
}

func TestRuntimeFromCommandRequiresConfig(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(context.Background())

	runtime, err := runtimeFromCommand(cmd)
	if err == nil {
		t.Fatal("runtimeFromCommand() error = nil, want missing config")
	}
	if err.Error() != "config is required" {
		t.Fatalf("runtimeFromCommand() error = %q, want config is required", err.Error())
	}
	if runtime.Logger == nil {
		t.Fatal("runtime logger = nil, want no-op logger")
	}
}
