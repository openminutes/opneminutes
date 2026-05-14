package minutes

import "testing"

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "trims whitespace", raw: " https://example.test/root/ ", want: "https://example.test/root"},
		{name: "keeps host only", raw: "https://example.test", want: "https://example.test"},
		{name: "keeps http", raw: "http://example.test/path//", want: "http://example.test/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeBaseURL("base_url", tt.raw)
			if err != nil {
				t.Fatalf("NormalizeBaseURL() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeBaseURLRejectsInvalidValues(t *testing.T) {
	tests := []string{
		"example.test",
		"ftp://example.test",
		"https://",
		"https://example.test?token=secret",
		"https://example.test#fragment",
	}

	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			_, err := NormalizeBaseURL("base_url", raw)
			if err == nil {
				t.Fatal("NormalizeBaseURL() error = nil, want invalid URL error")
			}
			want := `invalid base_url "` + raw + `": must be an absolute http or https URL with a host`
			if err.Error() != want {
				t.Fatalf("NormalizeBaseURL() error = %q, want %q", err.Error(), want)
			}
		})
	}
}

func TestNormalizeBaseURLOrDefault(t *testing.T) {
	got, defaulted, err := NormalizeBaseURLOrDefault("base_url", "", "https://default.example.test/")
	if err != nil {
		t.Fatalf("NormalizeBaseURLOrDefault() error = %v, want nil", err)
	}
	if got != "https://default.example.test" || !defaulted {
		t.Fatalf("NormalizeBaseURLOrDefault() = %q, %v, want normalized default", got, defaulted)
	}

	got, defaulted, err = NormalizeBaseURLOrDefault("base_url", " https://custom.example.test/ ", "https://default.example.test")
	if err != nil {
		t.Fatalf("NormalizeBaseURLOrDefault() custom error = %v, want nil", err)
	}
	if got != "https://custom.example.test" || defaulted {
		t.Fatalf("NormalizeBaseURLOrDefault() custom = %q, %v, want normalized custom", got, defaulted)
	}
}
