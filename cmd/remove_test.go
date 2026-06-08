package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveDeletesGearByIndex(t *testing.T) {
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	prevSlot := removeSlot

	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(prevWd)
		removeSlot = prevSlot
	}()

	removeSlot = ""

	charsDir := filepath.Join("data", "chars")
	if err := os.MkdirAll(charsDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	seed := "name: \"fury\"\nclass: \"druid\"\ngear:\n  - exact_name: \"Breath of the Dying\"\n    query: \"breath of fury\"\n  - exact_name: \"Raven Frost\"\n    query: \"raven frost\"\n"
	if err := os.WriteFile(filepath.Join(charsDir, "fury.yaml"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}

	if err := runRemove(removeCmd, []string{"fury", "1"}); err != nil {
		t.Fatalf("expected remove command to succeed, got: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(charsDir, "fury.yaml"))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	text := string(content)
	if strings.Contains(text, "Breath of the Dying") {
		t.Fatalf("expected first item removed, got: %s", text)
	}
	if !strings.Contains(text, "Raven Frost") {
		t.Fatalf("expected second item to remain, got: %s", text)
	}
}

func TestRemoveDeletesGearBySlotAndIndex(t *testing.T) {
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	prevSlot := removeSlot

	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(prevWd)
		removeSlot = prevSlot
	}()

	charsDir := filepath.Join("data", "chars")
	if err := os.MkdirAll(charsDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	seed := "name: \"fury\"\nclass: \"druid\"\ngear:\n  ring:\n    - exact_name: \"Raven Frost\"\n      query: \"raven frost\"\n    - exact_name: \"Bul-Kathos' Wedding Band\"\n      query: \"bk ring\"\n  weapon:\n    - exact_name: \"Breath of the Dying\"\n      query: \"breath of fury\"\n"
	if err := os.WriteFile(filepath.Join(charsDir, "fury.yaml"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}

	removeSlot = "ring"
	if err := runRemove(removeCmd, []string{"fury", "1"}); err != nil {
		t.Fatalf("expected remove --slot to succeed, got: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(charsDir, "fury.yaml"))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	text := string(content)
	if strings.Contains(text, "Raven Frost") {
		t.Fatalf("expected first ring removed, got: %s", text)
	}
	if !strings.Contains(text, "Bul-Kathos") || !strings.Contains(text, "Breath of the Dying") {
		t.Fatalf("expected other items to remain, got: %s", text)
	}
}
