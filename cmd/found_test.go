package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFoundMarksTrackedGear(t *testing.T) {
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(prevWd) }()

	charsDir := filepath.Join("data", "chars")
	if err := os.MkdirAll(charsDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	seed := "name: \"fury\"\nclass: \"druid\"\ngear:\n  - exact_name: \"Harmony Bow\"\n    query: \"harmony bow\"\n    slot: \"weapon_swap\"\n    swap_role: \"main\"\n"
	if err := os.WriteFile(filepath.Join(charsDir, "fury.yaml"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}

	if err := runFound(foundCmd, []string{"fury", "harmony bow"}); err != nil {
		t.Fatalf("expected found command to succeed, got: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(charsDir, "fury.yaml"))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "found: true") {
		t.Fatalf("expected found flag in file, got: %s", text)
	}
	if !strings.Contains(text, "found_at:") {
		t.Fatalf("expected found_at timestamp in file, got: %s", text)
	}
}
