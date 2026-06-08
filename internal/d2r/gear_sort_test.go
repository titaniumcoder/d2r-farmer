package d2r

import "testing"

func TestSortWeaponBasesByAverageDamage(t *testing.T) {
	items := []gearBaseInfo{
		{Name: "Jo Staff", BaseClass: "melee_weapon", Damage: "25-35"},
		{Name: "Archon Staff", BaseClass: "melee_weapon", Damage: "83-99"},
		{Name: "Short Staff", BaseClass: "melee_weapon", Damage: "1-8"},
	}

	sortBasesByPower(items)

	if items[0].Name != "Archon Staff" {
		t.Fatalf("expected highest average damage first, got %q", items[0].Name)
	}
}

func TestSortDefensiveBasesByAverageDefense(t *testing.T) {
	items := []gearBaseInfo{
		{Name: "Bone Visage", BaseClass: "helm", Defense: "100-157"},
		{Name: "Dream Spirit", BaseClass: "helm", Defense: "109-159"},
		{Name: "Shako", BaseClass: "helm", Defense: "98-141"},
	}

	sortBasesByPower(items)

	if items[0].Name != "Dream Spirit" {
		t.Fatalf("expected highest average defense first, got %q", items[0].Name)
	}
}

func TestAverageDamage(t *testing.T) {
	v, ok := averageDamage("83-99")
	if !ok {
		t.Fatalf("expected parse to succeed")
	}
	if v != 91 {
		t.Fatalf("expected average 91, got %v", v)
	}

	if _, ok := averageDamage("varies"); ok {
		t.Fatalf("expected varies to fail parse")
	}
}

func TestFormatAvgValue_RoundsHalfUpToInteger(t *testing.T) {
	if got := formatAvgValue(91.5); got != "92" {
		t.Fatalf("expected 92, got %q", got)
	}
	if got := formatAvgValue(91.49); got != "91" {
		t.Fatalf("expected 91, got %q", got)
	}
}

func TestNormalizeResolvedSlotAndRole_GlovesBoots(t *testing.T) {
	e := map[string]any{"slot": "gauntlets"}
	normalizeResolvedSlotAndRole(e)
	if got := stringValue(e["slot"]); got != "gloves" {
		t.Fatalf("expected gloves slot, got %q", got)
	}

	e = map[string]any{"slot": "greaves"}
	normalizeResolvedSlotAndRole(e)
	if got := stringValue(e["slot"]); got != "boots" {
		t.Fatalf("expected boots slot, got %q", got)
	}
}
