package d2r

import "testing"

func TestApplyRunewordBaseRules_BreathOfTheDying(t *testing.T) {
	entry := map[string]any{
		"exact_name":     "Breath of the Dying",
		"query":          "breath of fury",
		"kind":           "runeword",
		"possible_bases": []string{"Ethereal Berserker Axe", "Ethereal Colossus Blade"},
	}

	applyRunewordBaseRules(entry)
	bases := stringSliceValue(entry["possible_bases"])
	if len(bases) != 1 || bases[0] != "Any non-magic 6-socket weapon" {
		t.Fatalf("expected canonical BOTD bases rule, got: %#v", bases)
	}
}

func TestApplyRunewordBaseRules_Wisdom(t *testing.T) {
	entry := map[string]any{
		"exact_name":     "Wisdom",
		"query":          "wisdom pelt",
		"kind":           "runeword",
		"possible_bases": []string{"Pelt"},
	}

	applyRunewordBaseRules(entry)
	bases := stringSliceValue(entry["possible_bases"])
	if len(bases) != 1 || bases[0] != "Any non-magic helm (including class-specific helms such as druid pelts)" {
		t.Fatalf("expected canonical Wisdom bases rule, got: %#v", bases)
	}
}
