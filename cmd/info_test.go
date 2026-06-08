package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildGearStatusesWeaponSwapPriority(t *testing.T) {
	entries := []map[string]any{
		{"exact_name": "Call to Arms", "slot": "weapon_swap", "swap_role": "main", "found": false},
		{"exact_name": "Monarch Spirit", "slot": "weapon_swap", "swap_role": "offhand", "found": true},
		{"exact_name": "Harmony Bow", "slot": "weapon_swap", "swap_role": "main", "found": true},
	}

	statuses := buildGearStatuses(entries)
	if len(statuses) != 3 {
		t.Fatalf("expected 3 statuses, got %d", len(statuses))
	}

	for _, s := range statuses {
		if s.Name == "Call to Arms" && s.Prio {
			t.Fatalf("expected CTA to stop being priority once a main swap item is found")
		}
	}
}

func TestBuildGearStatusesRingNeedsTwoSlots(t *testing.T) {
	entries := []map[string]any{
		{"exact_name": "Raven Frost", "slot": "ring", "found": true},
		{"exact_name": "Bul-Kathos' Wedding Band", "slot": "ring", "found": false},
	}

	statuses := buildGearStatuses(entries)
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}

	for _, s := range statuses {
		if s.Name == "Bul-Kathos' Wedding Band" && !s.Prio {
			t.Fatalf("expected second ring to remain priority until two rings are found")
		}
		if s.Name == "Bul-Kathos' Wedding Band" && !s.Needed {
			t.Fatalf("expected second ring to remain needed until two rings are found")
		}
	}

	entries = append(entries, map[string]any{"exact_name": "Stone of Jordan", "slot": "ring", "found": true})
	statuses = buildGearStatuses(entries)
	for _, s := range statuses {
		if s.Name == "Bul-Kathos' Wedding Band" && s.Needed {
			t.Fatalf("expected extra unfound rings to stop being needed once two rings are found")
		}
	}
}

func TestInfoShowsRequirementsAndRunewordDetails(t *testing.T) {
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

	seed := "name: \"fury\"\nclass: \"druid\"\nrequirements:\n  mandatory:\n    - \"cannot be frozen\"\ngear:\n  - exact_name: \"Breath of the Dying\"\n    kind: \"runeword\"\n    runes: [\"Vex\", \"Hel\", \"El\", \"Eld\", \"Zod\", \"Eth\"]\n    best_in_slot_base: \"Ethereal Archon Staff\"\n    possible_bases: [\"Ethereal Berserker Axe\", \"Ethereal Archon Staff\"]\n"
	if err := os.WriteFile(filepath.Join(charsDir, "fury.yaml"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}

	buf := &bytes.Buffer{}
	infoCmd.SetOut(buf)
	infoCmd.SetErr(buf)

	if err := runInfo(infoCmd, []string{"fury"}); err != nil {
		t.Fatalf("expected info command to succeed, got: %v", err)
	}

	out := buf.String()
	for _, expected := range []string{
		"mandatory requirements: cannot be frozen",
		"weapon:",
		"offhand:",
		"weapon_swap_weapon:",
		"weapon_swap_offhand:",
		"head:",
		"armor:",
		"belt:",
		"ring:",
		"amulet:",
		"inventory:",
		"Type: runeword",
		"Runes: Vex -> Hel -> El -> Eld -> Zod -> Eth",
		"Best in Slot (priority):",
		"1. Ethereal Archon Staff",
		"- Ethereal Berserker Axe",
		"- Ethereal Archon Staff",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected %q in output, got: %s", expected, out)
		}
	}
}
