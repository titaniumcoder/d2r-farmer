package d2r

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

type gearStatus struct {
	Key       string
	Name      string
	Slot      string
	Kind      string
	SwapRole  string
	Runes     []string
	Bases     []string
	BestBases []string
	Effects   []string
	IsSwap    bool
	Known     bool
	Needed    bool
	Prio      bool
}

// --- layout / coercion ---

func coerceGearEntries(v any) []map[string]any {
	bySlot := coerceGearBySlot(v)
	out := make([]map[string]any, 0)
	for _, slot := range supportedGearSlots {
		for _, entry := range bySlot[slot] {
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

// --- normalizeEntryMap / swap role / lookup (used across gear and import) ---

func normalizeEntryMap(v any) (map[string]any, bool) {
	switch entry := v.(type) {
	case map[string]any:
		return entry, true
	case map[string]string:
		out := make(map[string]any, len(entry))
		for k, val := range entry {
			out[k] = val
		}
		return out, true
	case map[any]any:
		out := make(map[string]any, len(entry))
		for k, val := range entry {
			key := strings.TrimSpace(fmt.Sprint(k))
			if key != "" {
				out[key] = val
			}
		}
		return out, true
	default:
		return nil, false
	}
}

func normalizeSwapRole(role string) string {
	if strings.ToLower(strings.TrimSpace(role)) == "offhand" {
		return "offhand"
	}
	return "main"
}

func findGearEntryIndex(entries []map[string]any, query string) int {
	needle := normalizeGearLookup(query)
	for i, entry := range entries {
		exact := normalizeGearLookup(fmt.Sprint(entry["exact_name"]))
		rawQuery := normalizeGearLookup(fmt.Sprint(entry["query"]))
		if needle != "" && (needle == exact || needle == rawQuery) {
			return i
		}
	}
	return -1
}

func normalizeGearLookup(s string) string {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(s)))
	return strings.Join(parts, " ")
}

// --- status computation ---

func buildGearStatuses(entries []map[string]any) []gearStatus {
	statuses := make([]gearStatus, 0, len(entries))
	foundBySlot := make(map[string]int)

	for _, entry := range entries {
		slot := slotForEntry(entry)
		if gearFound(entry) {
			foundBySlot[slot]++
		}
	}

	swapComplete := hasCompletedWeaponSwapOption(entries)

	for _, entry := range entries {
		slot := slotForEntry(entry)
		found := gearFound(entry)
		needed := !found
		if slot != "unknown" {
			needed = !found && foundBySlot[slot] < requiredSlotCount(slot)
		}

		prio := false
		if needed {
			switch {
			case isWeaponSwap(entry):
				prio = !swapComplete
			case slot == "unknown":
				prio = false
			default:
				prio = foundBySlot[slot] < requiredSlotCount(slot)
			}
		}

		statuses = append(statuses, gearStatus{
			Name:      gearName(entry),
			Slot:      slot,
			Kind:      gearKind(entry),
			SwapRole:  swapRoleValue(entry),
			Runes:     stringSliceValue(entry["runes"]),
			Bases:     stringSliceValue(entry["possible_bases"]),
			BestBases: bestBasesValue(entry), Effects: stringSliceValue(entry["effects"]), IsSwap: isWeaponSwap(entry),
			Known:  found,
			Needed: needed,
			Prio:   prio,
		})
	}
	return statuses
}

func requiredSlotCount(slot string) int {
	if slot == "ring" {
		return 2
	}
	return 1
}

func hasCompletedWeaponSwapOption(entries []map[string]any) bool {
	for _, entry := range entries {
		if isWeaponSwap(entry) && gearFound(entry) && swapRoleValue(entry) != "offhand" {
			return true
		}
	}
	return false
}

func gearName(entry map[string]any) string {
	if exact := stringValue(entry["exact_name"]); exact != "" {
		return exact
	}
	if q := stringValue(entry["query"]); q != "" {
		return q
	}
	return "unknown"
}

func gearKind(entry map[string]any) string {
	kind := strings.ToLower(stringValue(entry["kind"]))
	if kind == "" {
		return "unknown"
	}
	return kind
}

func gearFound(entry map[string]any) bool {
	value, ok := entry["found"]
	if !ok || value == nil {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(v)), "true")
	}
}

func swapRoleValue(entry map[string]any) string {
	return normalizeSwapRole(stringValue(entry["swap_role"]))
}

func bestBasesValue(entry map[string]any) []string {
	if values := stringSliceValue(entry["best_in_slot_bases"]); len(values) > 0 {
		return values
	}
	if best := stringValue(entry["best_in_slot_base"]); best != "" {
		return []string{best}
	}
	return nil
}

func filterStatuses(items []gearStatus, keep func(gearStatus) bool) []gearStatus {
	out := make([]gearStatus, 0)
	for _, item := range items {
		if keep(item) {
			out = append(out, item)
		}
	}
	return out
}

func readMandatoryRequirements(data map[string]any) []string {
	rawReq, ok := normalizeEntryMap(data["requirements"])
	if !ok {
		return nil
	}
	return stringSliceValue(rawReq["mandatory"])
}
