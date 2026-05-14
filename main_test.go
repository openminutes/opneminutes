package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMainCommand(t *testing.T) {
	withoutOpenMinutesEnv(t)

	oldArgs := os.Args
	t.Cleanup(func() {
		os.Args = oldArgs
	})

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("region = \"feishu\"\ncookie = \"session=abc\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	os.Args = []string{"openminutes", "--config", configPath, "get"}
	main()
}

func withoutOpenMinutesEnv(t *testing.T) {
	t.Helper()

	region, hadRegion := os.LookupEnv("OPENMINUTES_REGION")
	cookie, hadCookie := os.LookupEnv("OPENMINUTES_COOKIE")

	if err := os.Unsetenv("OPENMINUTES_REGION"); err != nil {
		t.Fatalf("Unsetenv(OPENMINUTES_REGION) error = %v", err)
	}
	if err := os.Unsetenv("OPENMINUTES_COOKIE"); err != nil {
		t.Fatalf("Unsetenv(OPENMINUTES_COOKIE) error = %v", err)
	}

	t.Cleanup(func() {
		if hadRegion {
			_ = os.Setenv("OPENMINUTES_REGION", region)
		} else {
			_ = os.Unsetenv("OPENMINUTES_REGION")
		}

		if hadCookie {
			_ = os.Setenv("OPENMINUTES_COOKIE", cookie)
		} else {
			_ = os.Unsetenv("OPENMINUTES_COOKIE")
		}
	})
}
