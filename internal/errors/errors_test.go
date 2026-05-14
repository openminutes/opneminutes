package errors

import (
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewCategorizesErrorWithoutChangingMessage(t *testing.T) {
	err := New(KindValidation, "object token is required")
	if err.Error() != "object token is required" {
		t.Fatalf("Error() = %q, want original message", err.Error())
	}
	if KindOf(err) != KindValidation {
		t.Fatalf("KindOf() = %q, want %q", KindOf(err), KindValidation)
	}
	if !IsKind(err, KindValidation) {
		t.Fatal("IsKind() = false, want true")
	}
}

func TestErrorfCategorizesFormattedError(t *testing.T) {
	err := Errorf(KindRemote, "server code %d", 123)
	if err.Error() != "server code 123" {
		t.Fatalf("Error() = %q, want formatted message", err.Error())
	}
	if !IsKind(err, KindRemote) {
		t.Fatal("IsKind() = false, want true")
	}
}

func TestWrapPreservesErrorsIs(t *testing.T) {
	sentinel := stderrors.New("write failed")
	err := Wrap(KindFileSystem, sentinel)
	if !stderrors.Is(err, sentinel) {
		t.Fatalf("errors.Is() = false, want true")
	}
	if !IsKind(err, KindFileSystem) {
		t.Fatal("IsKind() = false, want true")
	}
}

func TestWrapfPreservesErrorsIsAndAddsContext(t *testing.T) {
	sentinel := stderrors.New("stat failed")
	err := Wrapf(KindFileSystem, sentinel, "stat file %q", "config.toml")
	if !stderrors.Is(err, sentinel) {
		t.Fatalf("errors.Is() = false, want true")
	}
	if !strings.Contains(err.Error(), `stat file "config.toml": stat failed`) {
		t.Fatalf("Error() = %q, want context and wrapped message", err.Error())
	}
}

func TestNilAndUnknownErrors(t *testing.T) {
	if KindOf(nil) != KindUnknown {
		t.Fatalf("KindOf(nil) = %q, want unknown", KindOf(nil))
	}
	if IsKind(stderrors.New("plain"), KindValidation) {
		t.Fatal("IsKind(plain) = true, want false")
	}
	if Wrap(KindConfig, nil) != nil {
		t.Fatal("Wrap(nil) != nil")
	}
	if Wrapf(KindConfig, nil, "context") != nil {
		t.Fatal("Wrapf(nil) != nil")
	}
}

func TestKindOfInfersFileSystemErrors(t *testing.T) {
	_, err := os.Open(filepath.Join(t.TempDir(), "missing.txt"))
	if err == nil {
		t.Fatal("Open() error = nil, want error")
	}
	if KindOf(err) != KindFileSystem {
		t.Fatalf("KindOf() = %q, want file system", KindOf(err))
	}
}
