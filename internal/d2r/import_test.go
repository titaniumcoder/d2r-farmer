package d2r

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMapGuideSlot_MercAndKnownSlots(t *testing.T) {
	slot, weaponSwap, swapRole, merc := mapGuideSlot("Mercenary Weapon")
	if slot != "weapon" || weaponSwap || swapRole != "main" || !merc {
		t.Fatalf("unexpected merc weapon mapping: slot=%q weaponSwap=%t swapRole=%q merc=%t", slot, weaponSwap, swapRole, merc)
	}

	slot, weaponSwap, swapRole, merc = mapGuideSlot("Weapon Swap Off-Hand")
	if slot != "weapon" || !weaponSwap || swapRole != "offhand" || merc {
		t.Fatalf("unexpected swap mapping: slot=%q weaponSwap=%t swapRole=%q merc=%t", slot, weaponSwap, swapRole, merc)
	}

	slot, _, _, merc = mapGuideSlot("Some Unknown Section")
	if slot != "unknown" || merc {
		t.Fatalf("unexpected unknown mapping: slot=%q merc=%t", slot, merc)
	}

	slot, _, _, merc = mapGuideSlot("Mercenary Ring")
	if slot != "ring" || merc {
		t.Fatalf("expected merc ring to stay normal ring slot, got slot=%q merc=%t", slot, merc)
	}

	slot, _, _, merc = mapGuideSlot("Gloves")
	if slot != "gloves" || merc {
		t.Fatalf("expected gloves mapping, got slot=%q merc=%t", slot, merc)
	}

	slot, _, _, merc = mapGuideSlot("Boots")
	if slot != "boots" || merc {
		t.Fatalf("expected boots mapping, got slot=%q merc=%t", slot, merc)
	}
}

func TestImportGuideForCharacter_MercIndestructibleAddsEthereal(t *testing.T) {
	if err := os.MkdirAll(filepath.Join("data", "chars"), 0o755); err != nil {
		t.Fatalf("mkdir data/chars: %v", err)
	}
	if err := os.MkdirAll("data", 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}

	configFile := configPath()
	originalConfig, configReadErr := os.ReadFile(configFile)
	hadConfig := configReadErr == nil
	if err := os.WriteFile(configFile, []byte("provider: openai\nopenai:\n  api_key: test-key\n  model: gpt-5-mini\n"), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	t.Cleanup(func() {
		if hadConfig {
			_ = os.WriteFile(configFile, originalConfig, 0o600)
			return
		}
		_ = os.Remove(configFile)
	})

	charName := "test-merc-import"
	content, err := buildCharacterYAML(charName, "barbarian", nil)
	if err != nil {
		t.Fatalf("build character yaml: %v", err)
	}
	charPath := filepath.Join("data", "chars", slugifyName(charName)+".yaml")
	if err := os.WriteFile(charPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write character yaml: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(charPath) })

	origExtract := extractGuideGearWithLLM
	origResolve := resolveGearWithLLM
	t.Cleanup(func() {
		extractGuideGearWithLLM = origExtract
		resolveGearWithLLM = origResolve
	})

	extractGuideGearWithLLM = func(url string, cfg Config) ([]guideGearItem, error) {
		return []guideGearItem{{Slot: "Mercenary Weapon", Item: "Breath of the Dying"}}, nil
	}
	resolveGearWithLLM = func(query string, className string, slotHint string, cfg Config) (map[string]any, error) {
		return map[string]any{
			"exact_name":             "Breath of the Dying",
			"kind":                   "runeword",
			"slot":                   "weapon",
			"possible_slots":         []string{"weapon"},
			"runes":                  []string{"Vex", "Hel", "El", "Eld", "Zod", "Eth"},
			"possible_bases":         []string{"Berserker Axe"},
			"possible_bases_details": []baseDetails{{Name: "Berserker Axe", BaseClass: "melee_weapon", Hand: "1h", Damage: "24-71", WeaponSpeed: "0"}},
		}, nil
	}

	imported, skipped, err := importGuideForCharacter(charName, "https://example.com/guide", nil, nil, nil)
	if err != nil {
		t.Fatalf("import guide: %v", err)
	}
	if imported != 1 || skipped != 0 {
		t.Fatalf("unexpected import counts: imported=%d skipped=%d", imported, skipped)
	}

	data, err := readCharacterData(charName)
	if err != nil {
		t.Fatalf("read character data: %v", err)
	}
	entries := coerceGearEntries(data["gear"])
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	entry := entries[0]
	if !isMercEntry(entry) {
		t.Fatalf("expected imported entry to be marked as merc")
	}
	if slotForEntry(entry) != "merc_weapon" {
		t.Fatalf("expected merc weapon slot, got %q", slotForEntry(entry))
	}

	bases := stringSliceValue(entry["possible_bases"])
	seenEth := false
	for _, b := range bases {
		if b == "Ethereal Berserker Axe" {
			seenEth = true
			break
		}
	}
	if !seenEth {
		t.Fatalf("expected merc indestructible import to include Ethereal Berserker Axe")
	}
}

func TestImportGuideForCharacter_AllowsDuplicateEntries(t *testing.T) {
	if err := os.MkdirAll(filepath.Join("data", "chars"), 0o755); err != nil {
		t.Fatalf("mkdir data/chars: %v", err)
	}
	if err := os.MkdirAll("data", 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}

	configFile := configPath()
	originalConfig, configReadErr := os.ReadFile(configFile)
	hadConfig := configReadErr == nil
	if err := os.WriteFile(configFile, []byte("provider: openai\nopenai:\n  api_key: test-key\n  model: gpt-5-mini\n"), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	t.Cleanup(func() {
		if hadConfig {
			_ = os.WriteFile(configFile, originalConfig, 0o600)
			return
		}
		_ = os.Remove(configFile)
	})

	charName := "test-import-duplicates"
	content, err := buildCharacterYAML(charName, "barbarian", nil)
	if err != nil {
		t.Fatalf("build character yaml: %v", err)
	}
	charPath := filepath.Join("data", "chars", slugifyName(charName)+".yaml")
	if err := os.WriteFile(charPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write character yaml: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(charPath) })

	origExtract := extractGuideGearWithLLM
	origResolve := resolveGearWithLLM
	t.Cleanup(func() {
		extractGuideGearWithLLM = origExtract
		resolveGearWithLLM = origResolve
	})

	extractGuideGearWithLLM = func(url string, cfg Config) ([]guideGearItem, error) {
		return []guideGearItem{
			{Slot: "Weapon", Item: "Grief"},
			{Slot: "Weapon", Item: "Grief"},
		}, nil
	}
	resolveGearWithLLM = func(query string, className string, slotHint string, cfg Config) (map[string]any, error) {
		return map[string]any{
			"exact_name":             "Grief",
			"kind":                   "runeword",
			"slot":                   "weapon",
			"possible_slots":         []string{"weapon"},
			"runes":                  []string{"Eth", "Tir", "Lo", "Mal", "Ral"},
			"possible_bases":         []string{"Phase Blade"},
			"possible_bases_details": []baseDetails{{Name: "Phase Blade", BaseClass: "melee_weapon", Hand: "1h", Damage: "31-35", WeaponSpeed: "-30"}},
		}, nil
	}

	imported, skipped, err := importGuideForCharacter(charName, "https://example.com/guide", nil, nil, nil)
	if err != nil {
		t.Fatalf("import guide: %v", err)
	}
	if imported != 2 || skipped != 0 {
		t.Fatalf("unexpected import counts: imported=%d skipped=%d", imported, skipped)
	}

	data, err := readCharacterData(charName)
	if err != nil {
		t.Fatalf("read character data: %v", err)
	}
	entries := coerceGearEntries(data["gear"])
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}
