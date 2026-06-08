package d2r

import "testing"

func TestRequiredSlotCount_MercSlots(t *testing.T) {
	if got := requiredSlotCount("merc_weapon"); got != 2 {
		t.Fatalf("expected merc_weapon required count 2, got %d", got)
	}
	if got := requiredSlotCount("merc_helm"); got != 1 {
		t.Fatalf("expected merc_helm required count 1, got %d", got)
	}
	if got := requiredSlotCount("merc_armor"); got != 1 {
		t.Fatalf("expected merc_armor required count 1, got %d", got)
	}
}

func TestApplyMercEtherealIfIndestructible_RequiresRunewordIndestructible(t *testing.T) {
	entry := map[string]any{
		"kind":                   "runeword",
		"runes":                  []string{"Vex", "Hel", "El", "Eld", "Zod", "Eth"},
		"possible_bases":         []string{"Berserker Axe"},
		"possible_bases_details": []baseDetails{{Name: "Berserker Axe", BaseClass: "melee_weapon", Hand: "1h", Damage: "24-71", WeaponSpeed: "0"}},
	}

	applyMercEtherealIfIndestructible(entry)

	bases := stringSliceValue(entry["possible_bases"])
	seen := map[string]bool{}
	for _, b := range bases {
		seen[b] = true
	}
	if !seen["Ethereal Berserker Axe"] {
		t.Fatalf("expected ethereal berserker axe to be added")
	}
}

func TestApplyMercEtherealIfIndestructible_DoesNotApplyForNonIndestructible(t *testing.T) {
	entry := map[string]any{
		"kind":                   "runeword",
		"runes":                  []string{"Ral", "Tir", "Tal", "Sol"},
		"possible_bases":         []string{"Monarch"},
		"possible_bases_details": []baseDetails{{Name: "Monarch", BaseClass: "shield", Hand: "n/a", Defense: "133-148"}},
	}

	applyMercEtherealIfIndestructible(entry)

	bases := stringSliceValue(entry["possible_bases"])
	for _, b := range bases {
		if b == "Ethereal Monarch" {
			t.Fatalf("did not expect ethereal monarch to be added")
		}
	}
}

func TestSlotForEntry_MercOnlySpecialForWeaponHelmArmor(t *testing.T) {
	if got := slotForEntry(map[string]any{"merc": true, "slot": "weapon"}); got != "merc_weapon" {
		t.Fatalf("expected merc weapon slot, got %q", got)
	}
	if got := slotForEntry(map[string]any{"merc": true, "slot": "helm"}); got != "merc_helm" {
		t.Fatalf("expected merc helm slot, got %q", got)
	}
	if got := slotForEntry(map[string]any{"merc": true, "slot": "armor"}); got != "merc_armor" {
		t.Fatalf("expected merc armor slot, got %q", got)
	}
	if got := slotForEntry(map[string]any{"merc": true, "slot": "ring"}); got != "ring" {
		t.Fatalf("expected merc ring to remain normal ring slot, got %q", got)
	}
}
