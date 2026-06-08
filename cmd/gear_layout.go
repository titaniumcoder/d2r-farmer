package cmd

import (
	"fmt"
	"strings"
)

var supportedGearSlots = []string{"weapon", "head", "armor", "belt", "ring", "amulet", "inventory"}

type gearBuckets struct {
	Weapon    []map[string]any `yaml:"weapon"`
	Head      []map[string]any `yaml:"head"`
	Armor     []map[string]any `yaml:"armor"`
	Belt      []map[string]any `yaml:"belt"`
	Ring      []map[string]any `yaml:"ring"`
	Amulet    []map[string]any `yaml:"amulet"`
	Inventory []map[string]any `yaml:"inventory"`
}

func coerceGearEntries(v any) []map[string]any {
	bySlot := coerceGearBySlot(v)
	out := make([]map[string]any, 0)
	for _, slot := range supportedGearSlots {
		entries := bySlot[slot]
		for _, entry := range entries {
			out = append(out, entry)
		}
	}
	return out
}

func coerceGearBySlot(v any) map[string][]map[string]any {
	out := emptyGearBySlot()
	if v == nil {
		return out
	}

	appendEntry := func(entry map[string]any, slotOverride string) {
		slot := normalizeSlotName(slotOverride)
		if slot == "" {
			slot = slotForEntry(entry)
		}
		entry["slot"] = slot
		out[slot] = append(out[slot], entry)
	}

	switch items := v.(type) {
	case []map[string]any:
		for _, item := range items {
			appendEntry(item, "")
		}
		return out
	case []any:
		for _, item := range items {
			if entry, ok := normalizeEntryMap(item); ok {
				appendEntry(entry, "")
				continue
			}

			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				appendEntry(map[string]any{"exact_name": text, "query": text}, "")
			}
		}
		return out
	}

	if byKey, ok := normalizeEntryMap(v); ok {
		for key, rawEntries := range byKey {
			slot := normalizeSlotName(key)
			if slot == "" {
				continue
			}

			for _, entry := range coerceLegacyEntries(rawEntries) {
				appendEntry(entry, slot)
			}
		}
		return out
	}

	text := strings.TrimSpace(fmt.Sprint(v))
	if text != "" {
		appendEntry(map[string]any{"exact_name": text, "query": text}, "")
	}

	return out
}

func setGearEntries(data map[string]any, entries []map[string]any) {
	bySlot := emptyGearBySlot()
	for _, entry := range entries {
		slot := slotForEntry(entry)
		entry["slot"] = slot
		bySlot[slot] = append(bySlot[slot], entry)
	}
	data["gear"] = gearBuckets{
		Weapon:    bySlot["weapon"],
		Head:      bySlot["head"],
		Armor:     bySlot["armor"],
		Belt:      bySlot["belt"],
		Ring:      bySlot["ring"],
		Amulet:    bySlot["amulet"],
		Inventory: bySlot["inventory"],
	}
}

func coerceLegacyEntries(v any) []map[string]any {
	switch items := v.(type) {
	case []map[string]any:
		return append([]map[string]any{}, items...)
	case []any:
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if entry, ok := normalizeEntryMap(item); ok {
				out = append(out, entry)
				continue
			}

			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				out = append(out, map[string]any{"exact_name": text, "query": text})
			}
		}
		return out
	default:
		text := strings.TrimSpace(fmt.Sprint(v))
		if text == "" {
			return nil
		}
		return []map[string]any{{"exact_name": text, "query": text}}
	}
}

func emptyGearBySlot() map[string][]map[string]any {
	out := make(map[string][]map[string]any, len(supportedGearSlots))
	for _, slot := range supportedGearSlots {
		out[slot] = []map[string]any{}
	}
	return out
}

func normalizeSlotName(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")

	if value == "weapon_swap" {
		return "weapon"
	}

	for _, slot := range supportedGearSlots {
		if value == slot {
			return slot
		}
	}

	if value == "" {
		return ""
	}

	return "inventory"
}

func slotForEntry(entry map[string]any) string {
	if isWeaponSwap(entry) {
		return "weapon"
	}

	slot := normalizeSlotName(stringValue(entry["slot"]))
	if slot == "" {
		return "inventory"
	}
	return slot
}

func isWeaponSwap(entry map[string]any) bool {
	if v, ok := entry["weapon_swap"]; ok {
		switch val := v.(type) {
		case bool:
			if val {
				return true
			}
		case string:
			if strings.EqualFold(strings.TrimSpace(val), "true") {
				return true
			}
		default:
			if strings.EqualFold(strings.TrimSpace(fmt.Sprint(val)), "true") {
				return true
			}
		}
	}

	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(entry["slot"])), "weapon_swap")
}
