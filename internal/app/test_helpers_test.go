package app

import (
	"os"
	"path/filepath"
	"testing"
)

// Go runs package tests in internal/app; repository fixtures stay at the root.
func testProjectRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("locate project fixtures: %v", err)
	}
	return root
}
