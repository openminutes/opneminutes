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
	if err := os.WriteFile(configPath, []byte("base_url = \"https://meetings.example.test\"\nspace_base_url = \"https://space.example.test\"\ncookie = \"session=abc\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	os.Args = []string{"openminutes", "--config", configPath, "get"}
	main()
}

func withoutOpenMinutesEnv(t *testing.T) {
	t.Helper()

	baseURL, hadBaseURL := os.LookupEnv("OPENMINUTES_BASE_URL")
	spaceBaseURL, hadSpaceBaseURL := os.LookupEnv("OPENMINUTES_SPACE_BASE_URL")
	cookie, hadCookie := os.LookupEnv("OPENMINUTES_COOKIE")
	region, hadRegion := os.LookupEnv("OPENMINUTES_REGION")

	if err := os.Unsetenv("OPENMINUTES_BASE_URL"); err != nil {
		t.Fatalf("Unsetenv(OPENMINUTES_BASE_URL) error = %v", err)
	}
	if err := os.Unsetenv("OPENMINUTES_SPACE_BASE_URL"); err != nil {
		t.Fatalf("Unsetenv(OPENMINUTES_SPACE_BASE_URL) error = %v", err)
	}
	if err := os.Unsetenv("OPENMINUTES_COOKIE"); err != nil {
		t.Fatalf("Unsetenv(OPENMINUTES_COOKIE) error = %v", err)
	}
	if err := os.Unsetenv("OPENMINUTES_REGION"); err != nil {
		t.Fatalf("Unsetenv(OPENMINUTES_REGION) error = %v", err)
	}

	t.Cleanup(func() {
		if hadBaseURL {
			_ = os.Setenv("OPENMINUTES_BASE_URL", baseURL)
		} else {
			_ = os.Unsetenv("OPENMINUTES_BASE_URL")
		}

		if hadSpaceBaseURL {
			_ = os.Setenv("OPENMINUTES_SPACE_BASE_URL", spaceBaseURL)
		} else {
			_ = os.Unsetenv("OPENMINUTES_SPACE_BASE_URL")
		}

		if hadCookie {
			_ = os.Setenv("OPENMINUTES_COOKIE", cookie)
		} else {
			_ = os.Unsetenv("OPENMINUTES_COOKIE")
		}

		if hadRegion {
			_ = os.Setenv("OPENMINUTES_REGION", region)
		} else {
			_ = os.Unsetenv("OPENMINUTES_REGION")
		}
	})
}
