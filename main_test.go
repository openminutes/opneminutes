package main

import (
	"os"
	"testing"
)

func TestMainCommand(t *testing.T) {
	withoutOpenMinutesEnv(t)

	oldArgs := os.Args
	t.Cleanup(func() {
		os.Args = oldArgs
	})

	os.Args = []string{"openminutes", "--version"}
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
