package minutes

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

func TestNewFileIDUUIDFormatVersionAndVariant(t *testing.T) {
	withRandRead(t, func(p []byte) (int, error) {
		for i := range p {
			p[i] = byte(i)
		}
		return len(p), nil
	})

	got := newFileID()
	parts := strings.Split(got, "-")
	if len(parts) != 5 {
		t.Fatalf("newFileID() = %q, want UUID with 5 parts", got)
	}
	wantLengths := []int{8, 4, 4, 4, 12}
	for i, want := range wantLengths {
		if len(parts[i]) != want {
			t.Fatalf("part %d length = %d, want %d in %q", i, len(parts[i]), want, got)
		}
		if _, err := strconv.ParseUint(parts[i], 16, 64); err != nil {
			t.Fatalf("part %d = %q is not hex: %v", i, parts[i], err)
		}
	}
	if got[14] != '4' {
		t.Fatalf("version nibble = %q, want 4 in %q", got[14], got)
	}
	if !strings.ContainsRune("89ab", rune(got[19])) {
		t.Fatalf("variant nibble = %q, want RFC 4122 variant in %q", got[19], got)
	}
}

func TestNewFileIDFallsBackWhenRandomReadFails(t *testing.T) {
	withRandRead(t, func([]byte) (int, error) {
		return 0, errors.New("random failed")
	})

	got := newFileID()
	if _, err := strconv.ParseInt(got, 10, 64); err != nil {
		t.Fatalf("newFileID() = %q, want decimal fallback timestamp", got)
	}
	if got == "" {
		t.Fatal("newFileID() = empty, want fallback timestamp")
	}
}

func TestFallbackFileID(t *testing.T) {
	got := fallbackFileID()
	if _, err := strconv.ParseInt(got, 10, 64); err != nil {
		t.Fatalf("fallbackFileID() = %q, want decimal timestamp", got)
	}
}

func withRandRead(t *testing.T, fn func([]byte) (int, error)) {
	t.Helper()

	old := randRead
	randRead = fn
	t.Cleanup(func() {
		randRead = old
	})
}
