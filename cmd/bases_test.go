package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBasesUpdatesPossibleBasesAndBest(t *testing.T) {
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(prevWd) }()

	prevSet := basesSet
	prevBest := basesBest
	defer func() {
		basesSet = prevSet
		basesBest = prevBest
	}()

	charsDir := filepath.Join("data", "chars")
	if err := os.MkdirAll(charsDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	seed := "name: \"fury\"\nclass: \"druid\"\ngear:\n  - exact_name: \"Breath of the Dying\"\n    query: \"breath of fury\"\n    kind: \"runeword\"\n"
	if err := os.WriteFile(filepath.Join(charsDir, "fury.yaml"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}

	basesSet = "Ethereal Berserker Axe, Ethereal Archon Staff"
	basesBest = "Ethereal Archon Staff, Ethereal Berserker Axe"
	if err := runBases(basesCmd, []string{"fury", "1"}); err != nil {
		t.Fatalf("expected bases command to succeed, got: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(charsDir, "fury.yaml"))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "possible_bases:") || !strings.Contains(text, "Ethereal Berserker Axe") {
		t.Fatalf("expected updated bases in file, got: %s", text)
	}
	if !strings.Contains(text, "best_in_slot_base: Ethereal Archon Staff") {
		t.Fatalf("expected updated best base in file, got: %s", text)
	}
	if !strings.Contains(text, "best_in_slot_bases:") || !strings.Contains(text, "- Ethereal Berserker Axe") {
		t.Fatalf("expected prioritized best base list in file, got: %s", text)
	}
}

func TestBasesInteractiveUpdatesSelectedRuneword(t *testing.T) {
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

	seed := "name: \"fury\"\nclass: \"druid\"\ngear:\n  weapon:\n    - exact_name: \"Breath of the Dying\"\n      kind: \"runeword\"\n      possible_bases: [\"Old A\", \"Old B\"]\n    - exact_name: \"Shako\"\n      kind: \"unique\"\n"
	if err := os.WriteFile(filepath.Join(charsDir, "fury.yaml"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}

	in := bytes.NewBufferString("1\nEthereal Archon Staff, Ethereal Berserker Axe\nEthereal Archon Staff, Ethereal Berserker Axe\n")
	out := &bytes.Buffer{}
	basesCmd.SetIn(in)
	basesCmd.SetOut(out)
	basesCmd.SetErr(out)

	if err := runBases(basesCmd, []string{"fury"}); err != nil {
		t.Fatalf("expected interactive bases to succeed, got: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(charsDir, "fury.yaml"))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "best_in_slot_base: Ethereal Archon Staff") {
		t.Fatalf("expected first entered base as best, got: %s", text)
	}
	if !strings.Contains(text, "best_in_slot_bases:") {
		t.Fatalf("expected prioritized best-in-slot list, got: %s", text)
	}
	if !strings.Contains(text, "- Ethereal Archon Staff") || !strings.Contains(text, "- Ethereal Berserker Axe") {
		t.Fatalf("expected entered bases in file, got: %s", text)
	}
	if strings.Contains(text, "Old A") || strings.Contains(text, "Old B") {
		t.Fatalf("expected old bases to be removed, got: %s", text)
	}
}
