package cmd

import "testing"

func TestBuildVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		commit  string
		want    string
	}{
		{
			name:    "default dev build",
			version: defaultVersion,
			commit:  defaultCommit,
			want:    defaultVersion,
		},
		{
			name:    "release build",
			version: "v0.1.2",
			commit:  "abcdef12",
			want:    "v0.1.2-abcdef12",
		},
		{
			name:    "release build with full commit",
			version: "v0.1.2",
			commit:  "abcdef1234567890",
			want:    "v0.1.2-abcdef12",
		},
		{
			name:    "empty version",
			version: "",
			commit:  "abcdef12",
			want:    defaultVersion,
		},
		{
			name:    "empty commit",
			version: "v0.1.2",
			commit:  "",
			want:    defaultVersion,
		},
		{
			name:    "short commit",
			version: "v0.1.2",
			commit:  "abcdef1",
			want:    defaultVersion,
		},
		{
			name:    "missing commit injection",
			version: "v0.1.2",
			commit:  defaultCommit,
			want:    defaultVersion,
		},
		{
			name:    "missing version injection",
			version: defaultVersion,
			commit:  "abcdef12",
			want:    defaultVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldVersion, oldCommit := version, commit
			t.Cleanup(func() {
				version = oldVersion
				commit = oldCommit
			})

			version = tt.version
			commit = tt.commit

			if got := buildVersion(); got != tt.want {
				t.Fatalf("buildVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}
