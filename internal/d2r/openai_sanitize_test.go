package d2r

import "testing"

func TestSanitizeRunewordSlotAndBases_InfersHeadFromHelmBases(t *testing.T) {
	slot, bases, details := sanitizeRunewordSlotAndBases(
		"armor",
		[]string{"Wolf Head", "Falcon Mask"},
		[]baseDetails{
			{Name: "Wolf Head", BaseClass: "helm"},
			{Name: "Falcon Mask", BaseClass: "helm"},
		},
	)

	if slot != "helm" {
		t.Fatalf("expected slot helm, got %q", slot)
	}
	if len(bases) != 2 {
		t.Fatalf("expected 2 bases, got %d", len(bases))
	}
	if len(details) != 2 {
		t.Fatalf("expected 2 base details, got %d", len(details))
	}
	for _, d := range details {
		if d.BaseClass != "helm" {
			t.Fatalf("expected normalized base class helm, got %q", d.BaseClass)
		}
	}
}

func TestSanitizeRunewordSlotAndBases_PreservesMixedLegalBaseClasses(t *testing.T) {
	slot, bases, details := sanitizeRunewordSlotAndBases(
		"weapon",
		[]string{"Crystal Sword", "Monarch"},
		[]baseDetails{
			{Name: "Crystal Sword", BaseClass: "melee_weapon"},
			{Name: "Monarch", BaseClass: "shield"},
		},
	)

	if slot != "weapon" {
		t.Fatalf("expected slot weapon, got %q", slot)
	}
	if len(bases) != 2 {
		t.Fatalf("expected 2 bases, got %d", len(bases))
	}
	if len(details) != 2 {
		t.Fatalf("expected 2 base details, got %d", len(details))
	}
}

func TestEnforceBaseDetailStatShape_WeaponHasNoDefense(t *testing.T) {
	details := enforceBaseDetailStatShape("weapon", []baseDetails{{
		Name:        "Phase Blade",
		BaseClass:   "melee_weapon",
		Hand:        "1h",
		Defense:     "123",
		Damage:      "31-35",
		WeaponSpeed: "-30",
	}})

	if len(details) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(details))
	}
	if details[0].Defense != "" {
		t.Fatalf("expected empty defense for weapon, got %q", details[0].Defense)
	}
	if details[0].Damage == "" || details[0].WeaponSpeed == "" {
		t.Fatalf("expected weapon damage and speed to be preserved, got damage=%q speed=%q", details[0].Damage, details[0].WeaponSpeed)
	}
}

func TestEnforceBaseDetailStatShape_ArmorHasNoDamageOrWeaponSpeed(t *testing.T) {
	details := enforceBaseDetailStatShape("armor", []baseDetails{{
		Name:        "Archon Plate",
		BaseClass:   "body_armor",
		Hand:        "",
		Defense:     "410-524",
		Damage:      "1-2",
		WeaponSpeed: "10",
	}})

	if len(details) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(details))
	}
	if details[0].Damage != "" || details[0].WeaponSpeed != "" {
		t.Fatalf("expected empty damage/speed for armor, got damage=%q speed=%q", details[0].Damage, details[0].WeaponSpeed)
	}
	if details[0].Defense == "" {
		t.Fatalf("expected armor defense to be preserved")
	}
	if details[0].Hand != "n/a" {
		t.Fatalf("expected non-weapon hand to default to n/a, got %q", details[0].Hand)
	}
}

func TestExpandGenericRunewordBases_HelmsAndPeltsToConcreteSocketBases(t *testing.T) {
	bases, details := expandGenericRunewordBases(
		nil,
		OpenAIConfig{},
		"Metamorphosis",
		"helm",
		[]string{"Io", "Cham", "Fal"},
		[]string{"Helms", "Pelts"},
		[]baseDetails{
			{Name: "Helms", BaseClass: "helm"},
			{Name: "Pelts", BaseClass: "helm"},
		},
	)

	if len(bases) == 0 {
		t.Fatalf("expected expanded base list")
	}
	if len(details) == 0 {
		t.Fatalf("expected expanded base details")
	}

	for _, d := range details {
		if d.BaseClass != "helm" {
			t.Fatalf("expected helm base class, got %q", d.BaseClass)
		}
		if d.Hand != "n/a" {
			t.Fatalf("expected hand n/a for helm base, got %q", d.Hand)
		}
	}
}

func TestExpandGenericRunewordBases_StaffCategoryToConcreteVariants(t *testing.T) {
	bases, details := expandGenericRunewordBases(
		nil,
		OpenAIConfig{},
		"Leaf",
		"weapon",
		[]string{"Tir", "Ral"},
		[]string{"Staves"},
		[]baseDetails{{Name: "Staves", BaseClass: "melee_weapon"}},
	)

	if len(bases) < 10 {
		t.Fatalf("expected many concrete staff bases, got %d", len(bases))
	}
	seen := map[string]bool{}
	for _, b := range bases {
		seen[b] = true
	}
	for _, expected := range []string{"Short Staff", "Jo Staff", "Archon Staff"} {
		if !seen[expected] {
			t.Fatalf("expected expanded bases to include %q", expected)
		}
	}
	for _, d := range details {
		if d.BaseClass != "melee_weapon" {
			t.Fatalf("expected staff base class melee_weapon, got %q", d.BaseClass)
		}
	}
}

func TestFilterBySocketRequirement_RemovesImpossibleWands(t *testing.T) {
	bases, details := filterBySocketRequirement(
		6,
		[]string{"Wand", "Archon Staff"},
		[]baseDetails{
			{Name: "Wand", BaseClass: "melee_weapon", Hand: "1h"},
			{Name: "Archon Staff", BaseClass: "melee_weapon", Hand: "2h"},
		},
	)

	for _, b := range bases {
		if b == "Wand" {
			t.Fatalf("expected Wand to be filtered out for required sockets=6")
		}
	}
	if len(details) != 1 || details[0].Name != "Archon Staff" {
		t.Fatalf("expected only Archon Staff detail after filtering, got %#v", details)
	}
}

func TestFilterConcreteWeaponStatDetails_DropsPlaceholderStats(t *testing.T) {
	filtered := filterConcreteWeaponStatDetails([]baseDetails{
		{Name: "Archon Staff", BaseClass: "melee_weapon", Damage: "83-99", WeaponSpeed: "0"},
		{Name: "Elder Staff", BaseClass: "melee_weapon", Damage: "varies", WeaponSpeed: "n/a"},
		{Name: "Jo Staff", BaseClass: "melee_weapon", Damage: "", WeaponSpeed: "0"},
	})

	if len(filtered) != 1 {
		t.Fatalf("expected 1 concrete weapon detail, got %d", len(filtered))
	}
	if filtered[0].Name != "Archon Staff" {
		t.Fatalf("expected Archon Staff to remain, got %q", filtered[0].Name)
	}
}

func TestFilterConcreteDefensiveStatDetails_DropsPlaceholderDefense(t *testing.T) {
	filtered := filterConcreteDefensiveStatDetails([]baseDetails{
		{Name: "Dream Spirit", BaseClass: "helm", Defense: "109-159"},
		{Name: "Sky Spirit", BaseClass: "helm", Defense: "varies"},
		{Name: "Earth Spirit", BaseClass: "helm", Defense: ""},
	})

	if len(filtered) != 1 {
		t.Fatalf("expected 1 concrete defensive detail, got %d", len(filtered))
	}
	if filtered[0].Name != "Dream Spirit" {
		t.Fatalf("expected Dream Spirit to remain, got %q", filtered[0].Name)
	}
}

func TestEnrichRunewordBasesFromCatalog_DoesNotInjectEtherealVariants(t *testing.T) {
	bases, details := enrichRunewordBasesFromCatalog(
		"weapon",
		[]string{"Vex", "Hel", "El", "Eld", "Zod", "Eth"},
		[]string{"Berserker Axe"},
		nil,
	)

	seenBase := map[string]bool{}
	for _, b := range bases {
		seenBase[b] = true
	}
	if !seenBase["Berserker Axe"] {
		t.Fatalf("expected base list to include Berserker Axe")
	}
	if seenBase["Ethereal Berserker Axe"] {
		t.Fatalf("did not expect resolver enrichment to include Ethereal Berserker Axe")
	}

	seenDetails := map[string]baseDetails{}
	for _, d := range details {
		seenDetails[d.Name] = d
	}
	normal, ok := seenDetails["Berserker Axe"]
	if !ok {
		t.Fatalf("expected normal base detail for Berserker Axe")
	}
	if normal.Damage != "24-71" {
		t.Fatalf("expected catalog damage 24-71, got %q", normal.Damage)
	}
	if _, ok := seenDetails["Ethereal Berserker Axe"]; ok {
		t.Fatalf("did not expect resolver enrichment to include ethereal base detail")
	}
}

func TestMaxSocketsForBaseName_EtherealPrefixUsesCatalog(t *testing.T) {
	if got := maxSocketsForBaseName("Ethereal Monarch"); got != 4 {
		t.Fatalf("expected 4 sockets for Ethereal Monarch, got %d", got)
	}
}

func TestAppendEtherealVariants_FromBaseNameCatalog(t *testing.T) {
	bases, details := appendEtherealVariants([]string{"Archon Staff"}, nil)

	seen := map[string]bool{}
	for _, b := range bases {
		seen[b] = true
	}
	if !seen["Ethereal Archon Staff"] {
		t.Fatalf("expected Ethereal Archon Staff in base list")
	}

	var eth baseDetails
	found := false
	for _, d := range details {
		if d.Name == "Ethereal Archon Staff" {
			eth = d
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected Ethereal Archon Staff detail")
	}
	if eth.Damage != "124-148" {
		t.Fatalf("expected scaled damage 124-148, got %q", eth.Damage)
	}
}

func TestResolveRunewordBasesFromCatalog_ExpandsTypeHints(t *testing.T) {
	bases, details := resolveRunewordBasesFromCatalog("weapon", []string{"Amn", "Ral", "Mal", "Ist", "Ohm"}, []string{"staves"})

	if len(bases) == 0 || len(details) == 0 {
		t.Fatalf("expected expanded bases/details from staves hint")
	}

	seen := false
	for _, d := range details {
		if d.Name == "Archon Staff" {
			seen = true
			if d.Damage == "" || d.WeaponSpeed == "" {
				t.Fatalf("expected catalog stats for Archon Staff detail, got damage=%q speed=%q", d.Damage, d.WeaponSpeed)
			}
		}
	}
	if !seen {
		t.Fatalf("expected Archon Staff to be included for staves hint")
	}
}

func TestResolveRunewordBasesFromCatalog_RespectsSockets(t *testing.T) {
	bases, _ := resolveRunewordBasesFromCatalog("weapon", []string{"Zod", "Zod", "Zod", "Zod", "Zod", "Zod"}, []string{"wands"})
	if len(bases) != 0 {
		t.Fatalf("expected no wand bases for six-socket requirement, got %v", bases)
	}
}

func TestResolveRunewordBasesFromCatalog_SwordsIncludePhaseBlade(t *testing.T) {
	bases, _ := resolveRunewordBasesFromCatalog("weapon", []string{"Shael", "Um", "Tir"}, []string{"swords"})

	seen := false
	for _, b := range bases {
		if b == "Phase Blade" {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatalf("expected swords hint to include Phase Blade, got %v", bases)
	}
}
