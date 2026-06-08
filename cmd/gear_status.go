package cmd

import (
	"fmt"
	"strings"
)

type gearStatus struct {
	Name      string
	Slot      string
	Kind      string
	SwapRole  string
	Runes     []string
	Bases     []string
	BestBases []string
	IsSwap    bool
	Known     bool
	Needed    bool
	Prio      bool
}

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
			BestBases: bestBasesValue(entry),
			IsSwap:    isWeaponSwap(entry),
			Known:     found,
			Needed:    needed,
			Prio:      prio,
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
		if !isWeaponSwap(entry) {
			continue
		}

		if gearFound(entry) && swapRoleValue(entry) != "offhand" {
			return true
		}
	}

	return false
}

func gearName(entry map[string]any) string {
	exact := stringValue(entry["exact_name"])
	if exact != "" {
		return exact
	}
	query := stringValue(entry["query"])
	if query != "" {
		return query
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
	values := stringSliceValue(entry["best_in_slot_bases"])
	if len(values) > 0 {
		return values
	}

	best := stringValue(entry["best_in_slot_base"])
	if best == "" {
		return nil
	}
	return []string{best}
}
