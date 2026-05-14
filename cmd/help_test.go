package cmd

import (
	"strings"
	"testing"
)

func TestCommandHelpIncludesDescriptionsAndExamples(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    []string
		notWant []string
	}{
		{
			name:    "delete",
			command: "delete",
			want: []string{
				"Delete Minutes from the current account.",
				"Tokens are removed from the authenticated account.",
				"moved to trash",
				"--destroy to permanently delete",
				"openminutes delete m_abc123 --yes",
				"openminutes delete m_abc123 m_def456 --yes --destroy",
				"--destroy",
				"permanently delete each Minute after moving it to trash",
				"--yes",
				"confirm deletion without prompting",
			},
		},
		{
			name:    "get",
			command: "get",
			want: []string{
				"Export text from a Minute.",
				"Export one Minute as txt or srt.",
				"Exported content is printed to stdout by default.",
				"Use --output or",
				"-O to write to a file instead.",
				"Output files are created exclusively",
				"files are never overwritten.",
				"Use --json for structured output metadata",
				"inline exported content",
				"openminutes get m_abc123",
				"openminutes get m_abc123 --json",
				"openminutes get m_abc123 --file_type srt --speaker --timestamp -O meeting.srt",
				"--file_type",
				"export format: txt or srt",
				"--json",
				"print structured JSON instead of plain text",
				"--speaker",
				"include speaker names in the exported text",
				"--timestamp",
				"include timestamps in the exported text",
				"-O, --output",
				"write exported content to this file path",
			},
			notWant: []string{
				"TOKEN.txt",
				"TOKEN.srt",
			},
		},
		{
			name:    "list",
			command: "list",
			want: []string{
				"List Minutes from the current account.",
				"By default, list requests one page",
				"Next page command",
				"with --timestamp to continue",
				"Use --json for structured output",
				"Use --all to follow",
				"openminutes list",
				"openminutes list --size 50 --timestamp 1710000000",
				"openminutes list --all --json",
				"--all",
				"follow pagination and list all Minutes",
				"--json",
				"print structured JSON instead of plain rows",
				"--size",
				"number of Minutes to request per page",
				"--timestamp",
				"pagination timestamp to start from",
			},
			notWant: []string{
				"A longer description",
				"Cobra is a CLI library",
			},
		},
		{
			name:    "upload",
			command: "upload",
			want: []string{
				"Upload media to Minutes for transcription.",
				"Upload one local audio or video file",
				"extension, size, and duration",
				"Minute URL/token",
				"openminutes upload ./meeting.mp4",
			},
			notWant: []string{
				"A longer description",
				"Cobra is a CLI library",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := executeCommand("help", tt.command)
			if err != nil {
				t.Fatalf("Execute() error = %v, want nil", err)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}

			for _, want := range tt.want {
				if !strings.Contains(stdout, want) {
					t.Fatalf("help output = %q, want to contain %q", stdout, want)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(stdout, notWant) {
					t.Fatalf("help output = %q, want not to contain %q", stdout, notWant)
				}
			}
		})
	}
}
