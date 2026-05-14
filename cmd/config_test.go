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

	configPath := writeConfig(t, `base_url = "https://meetings.example.test/"
space_base_url = "https://space.example.test/"
cookie = "session=abc"
`)

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig() error = %v, want nil", err)
	}

	want := Config{
		BaseURL:      "https://meetings.example.test",
		SpaceBaseURL: "https://space.example.test",
		Cookie:       "session=abc",
	}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
}

func TestLoadConfigEnvOverridesFileValues(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `base_url = "https://file.example.test"
space_base_url = "https://file-space.example.test"
cookie = "session=file"
`)
	t.Setenv("OPENMINUTES_BASE_URL", "https://env.example.test")
	t.Setenv("OPENMINUTES_SPACE_BASE_URL", "https://env-space.example.test")
	t.Setenv("OPENMINUTES_COOKIE", "session=env")

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig() error = %v, want nil", err)
	}

	want := Config{
		BaseURL:      "https://env.example.test",
		SpaceBaseURL: "https://env-space.example.test",
		Cookie:       "session=env",
	}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
}

func TestLoadConfigEmptyURLEnvUsesDefaults(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `base_url = "https://file.example.test"
space_base_url = "https://file-space.example.test"
cookie = "session=file"
`)
	t.Setenv("OPENMINUTES_BASE_URL", "")
	t.Setenv("OPENMINUTES_SPACE_BASE_URL", "")

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig() error = %v, want nil", err)
	}

	want := Config{BaseURL: defaultBaseURL, SpaceBaseURL: defaultSpaceBaseURL, Cookie: "session=file"}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
}

func TestLoadConfigEmptyCookieEnvRejectsCookie(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `base_url = "https://file.example.test"
space_base_url = "https://file-space.example.test"
cookie = "session=file"
`)
	t.Setenv("OPENMINUTES_BASE_URL", "")
	t.Setenv("OPENMINUTES_SPACE_BASE_URL", "")
	t.Setenv("OPENMINUTES_COOKIE", "")

	_, err := loadConfig(configPath)
	if err == nil {
		t.Fatal("loadConfig() error = nil, want error")
	}

	if err.Error() != "cookie is required" {
		t.Fatalf("loadConfig() error = %q, want cookie is required", err.Error())
	}
}

func TestLoadConfigRejectsInvalidURLs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "base url missing scheme",
			content: `base_url = "meetings.example.test"
space_base_url = "https://space.example.test"
cookie = "session=abc"
`,
			want: `invalid base_url "meetings.example.test": must be an absolute http or https URL with a host`,
		},
		{
			name: "base url unsupported scheme",
			content: `base_url = "ftp://meetings.example.test"
space_base_url = "https://space.example.test"
cookie = "session=abc"
`,
			want: `invalid base_url "ftp://meetings.example.test": must be an absolute http or https URL with a host`,
		},
		{
			name: "space base url missing host",
			content: `base_url = "https://meetings.example.test"
space_base_url = "https://"
cookie = "session=abc"
`,
			want: `invalid space_base_url "https://": must be an absolute http or https URL with a host`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withoutOpenMinutesEnv(t)
			configPath := writeConfig(t, tt.content)
			_, err := loadConfig(configPath)
			if err == nil {
				t.Fatal("loadConfig() error = nil, want error")
			}
			if err.Error() != tt.want {
				t.Fatalf("loadConfig() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestLoadConfigRejectsEmptyCookie(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `base_url = "https://meetings.example.test"
space_base_url = "https://space.example.test"
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

func TestLoadConfigDefaultsMissingURLs(t *testing.T) {
	withoutOpenMinutesEnv(t)

	configPath := writeConfig(t, `cookie = "session=abc"
`)

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig() error = %v, want nil", err)
	}

	want := Config{BaseURL: defaultBaseURL, SpaceBaseURL: defaultSpaceBaseURL, Cookie: "session=abc"}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
}

func TestLoadConfigIgnoresRegionFileAndEnv(t *testing.T) {
	withoutOpenMinutesEnv(t)
	t.Setenv("OPENMINUTES_REGION", "larksuite")

	configPath := writeConfig(t, `region = "invalid"
cookie = "session=abc"
`)

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig() error = %v, want nil", err)
	}

	want := Config{BaseURL: defaultBaseURL, SpaceBaseURL: defaultSpaceBaseURL, Cookie: "session=abc"}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
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

func testCommandConfig() Config {
	return Config{BaseURL: defaultBaseURL, SpaceBaseURL: defaultSpaceBaseURL, Cookie: "session=abc"}
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
