package d2r

import "testing"

func TestBuildRuneNeeds_CompleteRunesMoveToBottom(t *testing.T) {
	data := map[string]any{
		"gear": []map[string]any{
			{"exact_name": "Spirit", "kind": "runeword", "runes": []string{"Tal"}},
			{"exact_name": "Infinity", "kind": "runeword", "runes": []string{"Ber"}},
			{"exact_name": "Fortitude", "kind": "runeword", "runes": []string{"Lo"}},
		},
		"runes_owned": map[string]any{"Tal": 1, "Lo": 1},
	}

	needs := buildRuneNeeds(data)
	if len(needs) != 3 {
		t.Fatalf("expected 3 rune needs, got %d", len(needs))
	}
	if needs[0].Name != "Ber" || needs[0].Complete {
		t.Fatalf("expected first rune to be incomplete Ber, got name=%q complete=%t", needs[0].Name, needs[0].Complete)
	}
	if !needs[1].Complete || !needs[2].Complete {
		t.Fatalf("expected complete runes to be at bottom")
	}
	if needs[1].Name != "Tal" || needs[2].Name != "Lo" {
		t.Fatalf("expected completed runes sorted by rune order (Tal, Lo), got (%s, %s)", needs[1].Name, needs[2].Name)
	}
}

func TestSplitMercWeaponSlots_AssignsSecondMainToOffhand(t *testing.T) {
	items := []webGearItem{
		{Key: "0", Status: gearStatus{Name: "Insight", Slot: "merc_weapon", Merc: true, SwapRole: "main"}},
		{Key: "1", Status: gearStatus{Name: "Infinity", Slot: "merc_weapon", Merc: true, SwapRole: "main"}},
	}

	main, offhand := splitMercWeaponSlots(items)
	if len(main) != 1 || main[0].Status.Name != "Insight" {
		t.Fatalf("expected main merc weapon Insight, got %+v", main)
	}
	if len(offhand) != 1 || offhand[0].Status.Name != "Infinity" {
		t.Fatalf("expected offhand merc weapon Infinity, got %+v", offhand)
	}
}

func TestSplitMercWeaponSlots_PrefersExplicitOffhand(t *testing.T) {
	items := []webGearItem{
		{Key: "0", Status: gearStatus{Name: "Grief", Slot: "merc_weapon", Merc: true, SwapRole: "main"}},
		{Key: "1", Status: gearStatus{Name: "Lawbringer", Slot: "merc_weapon", Merc: true, SwapRole: "offhand"}},
	}

	main, offhand := splitMercWeaponSlots(items)
	if len(main) != 1 || main[0].Status.Name != "Grief" {
		t.Fatalf("expected main merc weapon Grief, got %+v", main)
	}
	if len(offhand) != 1 || offhand[0].Status.Name != "Lawbringer" {
		t.Fatalf("expected explicit offhand Lawbringer, got %+v", offhand)
	}
}
