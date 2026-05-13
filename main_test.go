package main

import (
	"os"
	"testing"
)

func TestMainCommand(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() {
		os.Args = oldArgs
	})

	os.Args = []string{"openminutes", "get"}
	main()
}
