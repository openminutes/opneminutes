package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func executeCommand(args ...string) (string, string, error) {
	cmd := newRootCommand()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestRootCommandHelp(t *testing.T) {
	stdout, stderr, err := executeCommand()
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	for _, want := range []string{"openminutes", "Available Commands:", "get", "list", "upload"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want to contain %q", stdout, want)
		}
	}
}

func TestRootCommandUnknownCommand(t *testing.T) {
	stdout, stderr, err := executeCommand("missing")
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}

	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}

	if !strings.Contains(stderr, `unknown command "missing"`) {
		t.Fatalf("stderr = %q, want unknown command error", stderr)
	}
}

func TestRootCommandSubcommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "get",
			args: []string{"get"},
			want: "get called\n",
		},
		{
			name: "list",
			args: []string{"list"},
			want: "list called\n",
		},
		{
			name: "upload",
			args: []string{"upload"},
			want: "upload called\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := executeCommand(tt.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v, want nil", err)
			}

			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}

			if stdout != tt.want {
				t.Fatalf("stdout = %q, want %q", stdout, tt.want)
			}
		})
	}
}

func TestRootCommandVersion(t *testing.T) {
	oldVersion, oldCommit := version, commit
	t.Cleanup(func() {
		version = oldVersion
		commit = oldCommit
	})

	version = "v1.2.3"
	commit = "1234567890abcdef"

	stdout, stderr, err := executeCommand("--version")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	if want := "openminutes version v1.2.3-12345678\n"; stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestExecute(t *testing.T) {
	oldArgs := os.Args
	oldExit := exit
	os.Args = []string{"openminutes", "get"}
	exit = func(code int) {
		t.Fatalf("exit called with code %d", code)
	}
	t.Cleanup(func() {
		os.Args = oldArgs
		exit = oldExit
	})

	Execute()
}

func TestExecuteCommand(t *testing.T) {
	cmd := newRootCommand()
	cmd.SetArgs([]string{"get"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	execute(cmd)
}

func TestExecuteFailureExits(t *testing.T) {
	oldArgs := os.Args
	oldExit := exit
	exitCode := -1

	os.Args = []string{"openminutes", "missing"}
	exit = func(code int) {
		exitCode = code
	}
	t.Cleanup(func() {
		os.Args = oldArgs
		exit = oldExit
	})

	Execute()

	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
}
