package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigCreatesMissingDefaultConfig(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := filepath.Join(t.TempDir(), "openminutes", "config.toml")
	withDefaultConfigPath(t, configPath)

	config, err := loadConfig("")
	if err == nil {
		t.Fatal("loadConfig() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "cookie is required") {
		t.Fatalf("loadConfig() error = %v, want cookie required", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(data) != configTemplate {
		t.Fatalf("config file = %q, want %q", data, configTemplate)
	}

	if config != (Config{}) {
		t.Fatalf("config = %#v, want zero value", config)
	}
}

func TestLoadConfigCreatesMissingManualConfig(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := filepath.Join(t.TempDir(), "custom", "settings.toml")

	_, err := loadConfig(configPath)
	if err == nil {
		t.Fatal("loadConfig() error = nil, want error")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(data) != configTemplate {
		t.Fatalf("config file = %q, want %q", data, configTemplate)
	}
}

func TestLoadConfigReadsFileValues(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `region = "feishu"
cookie = "session=abc"
`)

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig() error = %v, want nil", err)
	}

	want := Config{Region: "feishu", Cookie: "session=abc"}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
}

func TestLoadConfigEnvOverridesFileValues(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `region = "feishu"
cookie = "session=file"
`)
	t.Setenv("OPENMINUTES_REGION", "larksuite")
	t.Setenv("OPENMINUTES_COOKIE", "session=env")

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig() error = %v, want nil", err)
	}

	want := Config{Region: "larksuite", Cookie: "session=env"}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
}

func TestLoadConfigEmptyEnvOverridesFileValue(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `region = "feishu"
cookie = "session=file"
`)
	t.Setenv("OPENMINUTES_COOKIE", "")

	_, err := loadConfig(configPath)
	if err == nil {
		t.Fatal("loadConfig() error = nil, want error")
	}

	if err.Error() != "cookie is required" {
		t.Fatalf("loadConfig() error = %q, want cookie is required", err.Error())
	}
}

func TestLoadConfigRejectsInvalidRegion(t *testing.T) {
	for _, region := range []string{"cn", "us"} {
		t.Run(region, func(t *testing.T) {
			withoutOpenMinutesEnv(t)

			configPath := writeConfig(t, `region = "`+region+`"
cookie = "session=abc"
`)

			_, err := loadConfig(configPath)
			if err == nil {
				t.Fatal("loadConfig() error = nil, want error")
			}

			want := `invalid region "` + region + `": must be one of feishu, larksuite`
			if err.Error() != want {
				t.Fatalf("loadConfig() error = %q, want %q", err.Error(), want)
			}
		})
	}
}

func TestLoadConfigRejectsEmptyCookie(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `region = "feishu"
cookie = ""
`)

	_, err := loadConfig(configPath)
	if err == nil {
		t.Fatal("loadConfig() error = nil, want error")
	}

	if err.Error() != "cookie is required" {
		t.Fatalf("loadConfig() error = %q, want cookie is required", err.Error())
	}
}

func TestLoadConfigAcceptsValidRegions(t *testing.T) {
	for _, region := range []string{"feishu", "larksuite"} {
		t.Run(region, func(t *testing.T) {
			withoutOpenMinutesEnv(t)

			configPath := writeConfig(t, `region = "`+region+`"
cookie = "session=abc"
`)

			config, err := loadConfig(configPath)
			if err != nil {
				t.Fatalf("loadConfig() error = %v, want nil", err)
			}

			if config.Region != region {
				t.Fatalf("config.Region = %q, want %q", config.Region, region)
			}
		})
	}
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

func withDefaultConfigPath(t *testing.T, configPath string) {
	t.Helper()

	oldDefaultConfigPathFunc := defaultConfigPathFunc
	defaultConfigPathFunc = func() string {
		return configPath
	}
	t.Cleanup(func() {
		defaultConfigPathFunc = oldDefaultConfigPathFunc
	})
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return configPath
}
