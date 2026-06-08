package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunesListsNeededRunesFromUnfoundGear(t *testing.T) {
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

	seed := "name: \"fury\"\nclass: \"druid\"\ngear:\n  - exact_name: \"Breath of the Dying\"\n    query: \"breath of fury\"\n    runes: [\"Vex\", \"Hel\", \"El\", \"Eld\", \"Zod\", \"Eth\"]\n  - exact_name: \"Call to Arms\"\n    runes: [\"Amn\", \"Ral\", \"Mal\", \"Ist\", \"Ohm\"]\n    found: true\n"
	if err := os.WriteFile(filepath.Join(charsDir, "fury.yaml"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}

	buf := &bytes.Buffer{}
	runesCmd.SetOut(buf)
	runesCmd.SetErr(buf)

	if err := runRunes(runesCmd, []string{"fury"}); err != nil {
		t.Fatalf("expected runes command to succeed, got: %v", err)
	}

	out := buf.String()
	for _, runeName := range []string{"El", "Eld", "Eth", "Hel", "Vex", "Zod"} {
		if !strings.Contains(out, runeName) {
			t.Fatalf("expected rune %q in output, got: %s", runeName, out)
		}
	}

	if strings.Index(out, "El") > strings.Index(out, "Hel") {
		t.Fatalf("expected El before Hel in difficulty order, got: %s", out)
	}
	if strings.Index(out, "Hel") > strings.Index(out, "Vex") {
		t.Fatalf("expected Hel before Vex in difficulty order, got: %s", out)
	}

	for _, expected := range []string{
		"El (countess: normal, nightmare, hell)",
		"Hel (countess: nightmare, hell)",
		"Vex",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected %q in output, got: %s", expected, out)
		}
	}

	if strings.Contains(out, "Ohm") {
		t.Fatalf("did not expect runes from found gear, got: %s", out)
	}
}
