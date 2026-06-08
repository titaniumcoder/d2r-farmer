package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddGearAppendsResolvedItem(t *testing.T) {
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	prevResolver := resolveGearWithLLM
	prevWeaponSwap := gearWeaponSwap
	prevSlotWeapon := gearSlotWeapon
	prevSlotHead := gearSlotHead
	prevSlotArmor := gearSlotArmor
	prevSlotBelt := gearSlotBelt
	prevSlotRing := gearSlotRing
	prevSlotAmulet := gearSlotAmulet
	prevSlotInv := gearSlotInv

	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(prevWd)
		resolveGearWithLLM = prevResolver
		gearWeaponSwap = prevWeaponSwap
		gearSlotWeapon = prevSlotWeapon
		gearSlotHead = prevSlotHead
		gearSlotArmor = prevSlotArmor
		gearSlotBelt = prevSlotBelt
		gearSlotRing = prevSlotRing
		gearSlotAmulet = prevSlotAmulet
		gearSlotInv = prevSlotInv
	})

	gearWeaponSwap = false
	gearSlotWeapon = false
	gearSlotHead = false
	gearSlotArmor = false
	gearSlotBelt = false
	gearSlotRing = false
	gearSlotAmulet = false
	gearSlotInv = false

	charsDir := filepath.Join("data", "chars")
	if err := os.MkdirAll(charsDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.MkdirAll("data", 0o755); err != nil {
		t.Fatalf("mkdir data failed: %v", err)
	}

	cfg := "provider: openai\nopenai:\n  api_key: test-key\n  model: gpt-4.1-mini\n"
	if err := os.WriteFile(filepath.Join("data", "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("config write failed: %v", err)
	}

	seed := "name: \"fury\"\nclass: \"druid\"\ncreated_at: \"2026-06-08T00:00:00Z\"\n"
	if err := os.WriteFile(filepath.Join(charsDir, "fury.yaml"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}

	resolveGearWithLLM = func(query string, className string, slotHint string, cfg Config) (map[string]any, error) {
		if query != "breath of fury" {
			t.Fatalf("unexpected query: %s", query)
		}
		if className != "druid" {
			t.Fatalf("unexpected class: %s", className)
		}
		if slotHint != "" {
			t.Fatalf("did not expect slot hint, got: %s", slotHint)
		}
		return map[string]any{
			"exact_name": "Breath of the Dying",
			"query":      "breath of fury",
			"slot":       "weapon",
			"kind":       "runeword",
			"runes":      []string{"Vex", "Hel", "El", "Eld", "Zod", "Eth"},
			"possible_bases": []string{
				"Ethereal Berserker Axe",
				"Ethereal Archon Staff",
			},
			"best_in_slot_base": "Ethereal Archon Staff",
			"notes":             "test",
			"sources":           []string{"https://example.com"},
		}, nil
	}

	if err := addGear(gearCmd, []string{"fury", "breath of fury"}); err != nil {
		t.Fatalf("expected add to succeed, got: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(charsDir, "fury.yaml"))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "gear:") {
		t.Fatalf("expected gear section in file, got: %s", text)
	}
	if !strings.Contains(text, "exact_name: Breath of the Dying") {
		t.Fatalf("expected exact name in file, got: %s", text)
	}
	if !strings.Contains(text, "kind: runeword") {
		t.Fatalf("expected kind in file, got: %s", text)
	}
	if !strings.Contains(text, "query: breath of fury") {
		t.Fatalf("expected original query in file, got: %s", text)
	}
	if !strings.Contains(text, "best_in_slot_base: Ethereal Archon Staff") {
		t.Fatalf("expected best base in file, got: %s", text)
	}
}

func TestAddGearHeadFlagForcesHeadSlot(t *testing.T) {
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	prevResolver := resolveGearWithLLM
	prevWeaponSwap := gearWeaponSwap
	prevSlotHead := gearSlotHead

	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(prevWd)
		resolveGearWithLLM = prevResolver
		gearWeaponSwap = prevWeaponSwap
		gearSlotHead = prevSlotHead
	})

	gearWeaponSwap = false
	gearSlotHead = true

	charsDir := filepath.Join("data", "chars")
	if err := os.MkdirAll(charsDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.MkdirAll("data", 0o755); err != nil {
		t.Fatalf("mkdir data failed: %v", err)
	}

	cfg := "provider: openai\nopenai:\n  api_key: test-key\n  model: gpt-4.1-mini\n"
	if err := os.WriteFile(filepath.Join("data", "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("config write failed: %v", err)
	}

	seed := "name: \"fury\"\nclass: \"druid\"\ncreated_at: \"2026-06-08T00:00:00Z\"\n"
	if err := os.WriteFile(filepath.Join(charsDir, "fury.yaml"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}

	resolveGearWithLLM = func(query string, className string, slotHint string, cfg Config) (map[string]any, error) {
		if slotHint != "head" {
			t.Fatalf("expected slot hint head, got: %s", slotHint)
		}
		return map[string]any{
			"exact_name":        "Wisdom",
			"query":             "wisdom",
			"slot":              "weapon",
			"kind":              "runeword",
			"runes":             []string{"Pul", "Ith", "Eld"},
			"possible_bases":    []string{"Dream Spirit", "Sky Spirit"},
			"best_in_slot_base": "Dream Spirit",
			"notes":             "test",
			"sources":           []string{"https://example.com"},
		}, nil
	}

	if err := addGear(gearCmd, []string{"fury", "wisdom"}); err != nil {
		t.Fatalf("expected add to succeed, got: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(charsDir, "fury.yaml"))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "head:") || !strings.Contains(text, "exact_name: Wisdom") {
		t.Fatalf("expected wisdom under head slot, got: %s", text)
	}
}

func TestAddGearFailsWithoutConfig(t *testing.T) {
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
	})

	charsDir := filepath.Join("data", "chars")
	if err := os.MkdirAll(charsDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	seed := "name: \"fury\"\nclass: \"druid\"\n"
	if err := os.WriteFile(filepath.Join(charsDir, "fury.yaml"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}

	err = addGear(gearCmd, []string{"fury", "some item"})
	if err == nil {
		t.Fatalf("expected missing config to fail")
	}
	if !strings.Contains(err.Error(), "run `d2r-farmer init` first") {
		t.Fatalf("expected missing config error, got: %v", err)
	}
}
