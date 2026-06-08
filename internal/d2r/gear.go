package d2r

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

var supportedGearSlots = []string{"weapon", "helm", "armor", "gloves", "boots", "belt", "ring", "amulet", "inventory", "merc_weapon", "merc_helm", "merc_armor"}

type gearBuckets struct {
	Weapon     []map[string]any `yaml:"weapon"`
	Helm       []map[string]any `yaml:"helm"`
	Armor      []map[string]any `yaml:"armor"`
	Gloves     []map[string]any `yaml:"gloves"`
	Boots      []map[string]any `yaml:"boots"`
	Belt       []map[string]any `yaml:"belt"`
	Ring       []map[string]any `yaml:"ring"`
	Amulet     []map[string]any `yaml:"amulet"`
	Inventory  []map[string]any `yaml:"inventory"`
	MercWeapon []map[string]any `yaml:"merc_weapon"`
	MercHelm   []map[string]any `yaml:"merc_helm"`
	MercArmor  []map[string]any `yaml:"merc_armor"`
}

type gearStatus struct {
	Key       string
	Name      string
	Slot      string
	Kind      string
	SwapRole  string
	Note      string
	Runes     []string
	Bases     []string
	BaseInfo  []gearBaseInfo
	BestBases []string
	Effects   []string
	IsSwap    bool
	Merc      bool
	Known     bool
	Needed    bool
	Prio      bool
}

type gearBaseInfo struct {
	Name        string
	BaseClass   string
	Hand        string
	Defense     string
	DefenseAvg  string
	Damage      string
	DamageAvg   string
	WeaponSpeed string
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
		Weapon:     bySlot["weapon"],
		Helm:       bySlot["helm"],
		Armor:      bySlot["armor"],
		Gloves:     bySlot["gloves"],
		Boots:      bySlot["boots"],
		Belt:       bySlot["belt"],
		Ring:       bySlot["ring"],
		Amulet:     bySlot["amulet"],
		Inventory:  bySlot["inventory"],
		MercWeapon: bySlot["merc_weapon"],
		MercHelm:   bySlot["merc_helm"],
		MercArmor:  bySlot["merc_armor"],
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
	if value == "merc_weapon" || value == "merc_helm" || value == "merc_armor" {
		return value
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
	if isMercEntry(entry) {
		slot := normalizeSlotName(stringValue(entry["slot"]))
		switch slot {
		case "weapon", "merc_weapon":
			return "merc_weapon"
		case "helm", "merc_helm":
			return "merc_helm"
		case "armor", "merc_armor":
			return "merc_armor"
		default:
			if slot == "" {
				return "inventory"
			}
			return slot
		}
	}
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

func normalizeResolvedSlotAndRole(entry map[string]any) {
	raw := strings.ToLower(strings.TrimSpace(stringValue(entry["slot"])))
	raw = strings.ReplaceAll(raw, "-", "_")
	raw = strings.ReplaceAll(raw, " ", "_")

	switch raw {
	case "offhand", "off_hand", "shield", "shields":
		entry["slot"] = "weapon"
		entry["swap_role"] = "offhand"
	case "helm", "helmet":
		entry["slot"] = "helm"
	case "gloves", "glove", "gauntlets", "gauntlet", "bracers", "vambraces":
		entry["slot"] = "gloves"
	case "boots", "boot", "greaves":
		entry["slot"] = "boots"
	case "armor", "body_armor", "bodyarmor", "chest", "breast":
		entry["slot"] = "armor"
	case "weapon", "melee_weapon", "missile_weapon", "bow", "crossbow", "amazon_bow":
		entry["slot"] = "weapon"
	default:
		entry["slot"] = normalizeSlotName(raw)
	}

	if strings.EqualFold(stringValue(entry["slot"]), "weapon") && stringValue(entry["swap_role"]) == "" {
		entry["swap_role"] = "main"
	}
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
			foundBySlot[slotRequirementKey(slot)]++
		}
	}

	swapComplete := hasCompletedWeaponSwapOption(entries)

	for _, entry := range entries {
		slot := slotForEntry(entry)
		merc := isMercEntry(entry)
		found := gearFound(entry)
		needed := !found
		required := requiredSlotCount(slot)
		slotKey := slotRequirementKey(slot)
		if slot != "unknown" {
			needed = !found && foundBySlot[slotKey] < required
		}

		prio := false
		if needed {
			switch {
			case isWeaponSwap(entry):
				prio = !swapComplete
			case slot == "unknown":
				prio = false
			default:
				prio = foundBySlot[slotKey] < required
			}
		}

		statuses = append(statuses, gearStatus{
			Name:      gearName(entry),
			Slot:      slot,
			Kind:      gearKind(entry),
			SwapRole:  swapRoleValue(entry),
			Note:      stringValue(entry["user_note"]),
			Runes:     stringSliceValue(entry["runes"]),
			Bases:     stringSliceValue(entry["possible_bases"]),
			BaseInfo:  baseInfoValue(entry),
			BestBases: bestBasesValue(entry), Effects: stringSliceValue(entry["effects"]), IsSwap: isWeaponSwap(entry),
			Merc:   merc,
			Known:  found,
			Needed: needed,
			Prio:   prio,
		})
	}
	return statuses
}

func requiredSlotCount(slot string) int {
	if slot == "merc_weapon" {
		return 2
	}
	if slot == "merc_helm" || slot == "merc_armor" {
		return 1
	}
	if slot == "ring" {
		return 2
	}
	return 1
}

func slotRequirementKey(slot string) string {
	return slot
}

func isMercEntry(entry map[string]any) bool {
	v, ok := entry["merc"]
	if !ok || v == nil {
		slot := normalizeSlotName(stringValue(entry["slot"]))
		return slot == "merc_weapon" || slot == "merc_helm" || slot == "merc_armor"
	}
	switch typed := v.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(v)), "true")
	}
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

func baseInfoValue(entry map[string]any) []gearBaseInfo {
	raw, ok := entry["possible_bases_details"]
	if !ok || raw == nil {
		return fallbackBaseInfo(stringSliceValue(entry["possible_bases"]))
	}

	items, ok := raw.([]any)
	if !ok {
		return fallbackBaseInfo(stringSliceValue(entry["possible_bases"]))
	}

	out := make([]gearBaseInfo, 0, len(items))
	for _, item := range items {
		m, ok := normalizeEntryMap(item)
		if !ok {
			continue
		}
		name := stringValue(m["name"])
		if name == "" {
			continue
		}
		out = append(out, gearBaseInfo{
			Name:        name,
			BaseClass:   normalizeBaseClass(stringValue(m["base_class"])),
			Hand:        strings.ToLower(stringValue(m["hand"])),
			Defense:     stringValue(m["defense"]),
			Damage:      stringValue(m["damage"]),
			WeaponSpeed: stringValue(m["weapon_speed"]),
		})
		last := &out[len(out)-1]
		if cat, ok := d2rBaseCatalog[normalizeGearLookup(stripEtherealPrefix(last.Name))]; ok {
			if last.BaseClass == "other" {
				last.BaseClass = normalizeBaseClass(cat.BaseClass)
			}
			if strings.TrimSpace(last.Hand) == "" {
				last.Hand = strings.ToLower(strings.TrimSpace(cat.Hand))
			}
			if strings.TrimSpace(last.Defense) == "" {
				last.Defense = strings.TrimSpace(cat.Defense)
			}
			if strings.TrimSpace(last.Damage) == "" {
				last.Damage = strings.TrimSpace(cat.Damage)
			}
			if strings.TrimSpace(last.WeaponSpeed) == "" {
				last.WeaponSpeed = strings.TrimSpace(cat.WeaponSpeed)
			}
			if isEtherealName(last.Name) {
				last.Defense = scaleRangeByHalf(last.Defense)
				last.Damage = scaleRangeByHalf(last.Damage)
			}
		}
		if avg, ok := averageDefense(last.Defense); ok {
			last.DefenseAvg = formatAvgValue(avg)
		}
		if avg, ok := averageDamage(last.Damage); ok {
			last.DamageAvg = formatAvgValue(avg)
		}
		if last.BaseClass == "melee_weapon" || last.BaseClass == "missile_weapon" {
			last.Defense = ""
			last.DefenseAvg = ""
			if last.WeaponSpeed == "" {
				last.WeaponSpeed = "?"
			}
		} else {
			last.DamageAvg = ""
		}
	}

	if len(out) == 0 {
		return fallbackBaseInfo(stringSliceValue(entry["possible_bases"]))
	}
	sortBasesByPower(out)
	return out
}

func fallbackBaseInfo(bases []string) []gearBaseInfo {
	if len(bases) == 0 {
		return nil
	}
	out := make([]gearBaseInfo, 0, len(bases))
	for _, name := range bases {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		info := gearBaseInfo{Name: trimmed}
		lookup := normalizeGearLookup(stripEtherealPrefix(trimmed))
		if cat, ok := d2rBaseCatalog[lookup]; ok {
			info.BaseClass = normalizeBaseClass(cat.BaseClass)
			info.Hand = strings.ToLower(strings.TrimSpace(cat.Hand))
			if info.Hand == "" {
				info.Hand = "n/a"
			}
			info.Defense = strings.TrimSpace(cat.Defense)
			info.Damage = strings.TrimSpace(cat.Damage)
			info.WeaponSpeed = strings.TrimSpace(cat.WeaponSpeed)

			if isEtherealName(trimmed) {
				info.Defense = scaleRangeByHalf(info.Defense)
				info.Damage = scaleRangeByHalf(info.Damage)
			}

			if avg, ok := averageDefense(info.Defense); ok {
				info.DefenseAvg = formatAvgValue(avg)
			}
			if avg, ok := averageDamage(info.Damage); ok {
				info.DamageAvg = formatAvgValue(avg)
			}

			if info.BaseClass == "melee_weapon" || info.BaseClass == "missile_weapon" {
				info.Defense = ""
				info.DefenseAvg = ""
				if info.WeaponSpeed == "" {
					info.WeaponSpeed = "?"
				}
			} else {
				info.DamageAvg = ""
			}
		}
		out = append(out, info)
	}
	sortBasesByPower(out)
	return out
}

func sortBasesByPower(items []gearBaseInfo) {
	if len(items) < 2 {
		return
	}

	sort.SliceStable(items, func(i, j int) bool {
		isWeaponI := items[i].BaseClass == "melee_weapon" || items[i].BaseClass == "missile_weapon"
		isWeaponJ := items[j].BaseClass == "melee_weapon" || items[j].BaseClass == "missile_weapon"
		if isWeaponI != isWeaponJ {
			return isWeaponI
		}

		if isWeaponI {
			ai, okI := averageDamage(items[i].Damage)
			aj, okJ := averageDamage(items[j].Damage)
			if okI != okJ {
				return okI
			}
			if okI && ai != aj {
				return ai > aj
			}
		} else {
			di, okI := averageDefense(items[i].Defense)
			dj, okJ := averageDefense(items[j].Defense)
			if okI != okJ {
				return okI
			}
			if okI && di != dj {
				return di > dj
			}
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
}

func averageDamage(raw string) (float64, bool) {
	v := strings.TrimSpace(strings.ToLower(raw))
	if v == "" || v == "n/a" || v == "varies" || strings.Contains(v, "unknown") {
		return 0, false
	}

	nums := make([]int, 0, 2)
	current := 0
	inNumber := false
	for _, r := range v {
		if r >= '0' && r <= '9' {
			current = current*10 + int(r-'0')
			inNumber = true
			continue
		}
		if inNumber {
			nums = append(nums, current)
			current = 0
			inNumber = false
		}
	}
	if inNumber {
		nums = append(nums, current)
	}

	if len(nums) == 0 {
		return 0, false
	}
	if len(nums) == 1 {
		return float64(nums[0]), true
	}
	return float64(nums[0]+nums[1]) / 2.0, true
}

func averageDefense(raw string) (float64, bool) {
	return averageDamage(raw)
}

func formatAvgValue(v float64) string {
	// D2-style display: integer with half-up rounding.
	return fmt.Sprintf("%d", int(math.Round(v)))
}

func isEtherealName(name string) bool {
	v := strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(v, "ethereal ") || strings.HasPrefix(v, "eth ")
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
