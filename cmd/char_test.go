package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddCharacterCreatesFile(t *testing.T) {
	prevClass := charClass
	prevMandatory := charMandatory
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(prevWd)
		charClass = prevClass
		charMandatory = prevMandatory
	})

	charClass = "sorceress"
	charMandatory = []string{"cannot be frozen"}

	if err := addCharacter(charCmd, []string{"Nova Sorc"}); err != nil {
		t.Fatalf("expected add character to succeed, got error: %v", err)
	}

	characterFile := filepath.Join(temp, "data", "chars", "nova-sorc.yaml")
	content, err := os.ReadFile(characterFile)
	if err != nil {
		t.Fatalf("expected character file to exist at %s, got error: %v", characterFile, err)
	}

	text := string(content)
	if !strings.Contains(text, "name: Nova Sorc") {
		t.Fatalf("expected name field in file, got: %s", text)
	}
	if !strings.Contains(text, "class: sorceress") {
		t.Fatalf("expected class field in file, got: %s", text)
	}
	if !strings.Contains(text, "mandatory:") || !strings.Contains(text, "cannot be frozen") {
		t.Fatalf("expected mandatory requirements in file, got: %s", text)
	}
}

func TestAddCharacterDuplicateFails(t *testing.T) {
	prevClass := charClass
	prevMandatory := charMandatory
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(prevWd)
		charClass = prevClass
		charMandatory = prevMandatory
	})

	charClass = "paladin"
	charMandatory = nil

	if err := addCharacter(charCmd, []string{"Hammerdin"}); err != nil {
		t.Fatalf("expected first create to succeed, got error: %v", err)
	}

	err = addCharacter(charCmd, []string{"Hammerdin"})
	if err == nil {
		t.Fatalf("expected duplicate create to fail")
	}
	if !strings.Contains(err.Error(), "character already exists") {
		t.Fatalf("expected duplicate error, got: %v", err)
	}
}
