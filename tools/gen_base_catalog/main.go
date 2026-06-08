// tools/gen_base_catalog — downloads D2R Excel data and generates base_catalog_gen.go
//
// Run from repository root:
//
//	go run ./tools/gen_base_catalog/
package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	weaponsURL = "https://raw.githubusercontent.com/pinkufairy/D2R-Excel/main/weapons.txt"
	armorURL   = "https://raw.githubusercontent.com/pinkufairy/D2R-Excel/main/armor.txt"
	outPath    = "internal/d2r/base_catalog_gen.go"
)

type baseEntry struct {
	Name        string
	TypeCode    string // original d2 type code (e.g. swor, staf, helm, tors)
	BaseClass   string // melee_weapon | missile_weapon | helm | body_armor | shield
	Hand        string // 1h | 2h | n/a
	Damage      string // "min-max" for weapons
	Defense     string // "min-max" for armor
	WeaponSpeed string // numeric string e.g. "0", "-10"
	MaxSockets  int
}

func main() {
	entries := map[string]baseEntry{}

	log.Printf("[weapons] downloading %s", weaponsURL)
	n, err := parseWeapons(weaponsURL, entries)
	if err != nil {
		log.Fatalf("parse weapons: %v", err)
	}
	log.Printf("[weapons] added %d bases", n)

	log.Printf("[armor] downloading %s", armorURL)
	n, err = parseArmor(armorURL, entries)
	if err != nil {
		log.Fatalf("parse armor: %v", err)
	}
	log.Printf("[armor] added %d bases", n)

	if err := writeOutput(entries); err != nil {
		log.Fatalf("write output: %v", err)
	}
	log.Printf("[done] wrote %s with %d entries", outPath, len(entries))
}

// ---------------------------------------------------------------------------
// TSV helpers
// ---------------------------------------------------------------------------

func fetchLines(url string) ([]string, error) {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	var lines []string
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

// parseHeader returns a map of lowercase-trimmed column name → zero-based index.
func parseHeader(line string) map[string]int {
	cols := strings.Split(line, "\t")
	m := make(map[string]int, len(cols))
	for i, c := range cols {
		k := strings.ToLower(strings.TrimSpace(c))
		if _, dup := m[k]; !dup {
			m[k] = i
		}
	}
	return m
}

func get(cols []string, idx int) string {
	if idx < 0 || idx >= len(cols) {
		return ""
	}
	return strings.TrimSpace(cols[idx])
}

func must(hdr map[string]int, name string) int {
	idx, ok := hdr[strings.ToLower(name)]
	if !ok {
		log.Fatalf("column %q not found in header", name)
	}
	return idx
}

// ---------------------------------------------------------------------------
// Weapons
// ---------------------------------------------------------------------------

func parseWeapons(url string, out map[string]baseEntry) (int, error) {
	lines, err := fetchLines(url)
	if err != nil {
		return 0, err
	}
	if len(lines) < 2 {
		return 0, fmt.Errorf("weapons.txt too short")
	}

	hdr := parseHeader(lines[0])
	iName := must(hdr, "name")
	iType := must(hdr, "type")
	iSpawn := must(hdr, "spawnable")
	iMinDam := must(hdr, "mindam")
	iMaxDam := must(hdr, "maxdam")
	iMin2h := must(hdr, "2handmindam")
	iMax2h := must(hdr, "2handmaxdam")
	iMinMis := must(hdr, "minmisdam")
	iMaxMis := must(hdr, "maxmisdam")
	i2handed := must(hdr, "2handed")
	iSpeed := must(hdr, "speed")
	iWclass := must(hdr, "wclass")
	iGems := must(hdr, "gemsockets")

	log.Printf("[weapons] columns: name=%d type=%d spawnable=%d mindam=%d maxdam=%d 2handed=%d speed=%d wclass=%d gemsockets=%d",
		iName, iType, iSpawn, iMinDam, iMaxDam, i2handed, iSpeed, iWclass, iGems)

	count := 0
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")

		name := get(cols, iName)
		if name == "" || strings.EqualFold(name, "Expansion") {
			continue
		}
		// Only spawnable base items
		if get(cols, iSpawn) != "1" {
			continue
		}

		itemType := get(cols, iType)
		baseClass := weaponClass(itemType)
		if baseClass == "" {
			continue
		}

		wclass := get(cols, iWclass)
		is2h := get(cols, i2handed) == "1"
		hand := weaponHand(itemType, wclass, is2h)

		// Damage: prefer 2h when the weapon is primarily 2h
		minD, maxD := get(cols, iMinDam), get(cols, iMaxDam)
		min2h, max2h := get(cols, iMin2h), get(cols, iMax2h)
		minM, maxM := get(cols, iMinMis), get(cols, iMaxMis)

		var damage string
		switch {
		case hand == "2h" && min2h != "" && max2h != "":
			damage = min2h + "-" + max2h
		case minD != "" && maxD != "":
			damage = minD + "-" + maxD
		case min2h != "" && max2h != "":
			damage = min2h + "-" + max2h
			hand = "2h"
		case minM != "" && maxM != "":
			// Throwing-only weapons (e.g. pure javelin with no melee damage)
			damage = minM + "-" + maxM
		}
		if damage == "" {
			continue
		}

		speed := get(cols, iSpeed)
		if speed == "" {
			speed = "0"
		}

		maxSockets, _ := strconv.Atoi(get(cols, iGems))
		if maxSockets == 0 {
			continue // Cannot be socketed, not useful for runewords
		}

		key := normKey(name)
		if _, exists := out[key]; !exists {
			out[key] = baseEntry{
				Name:        name,
				TypeCode:    itemType,
				BaseClass:   baseClass,
				Hand:        hand,
				Damage:      damage,
				WeaponSpeed: speed,
				MaxSockets:  maxSockets,
			}
			count++
		}
	}
	return count, nil
}

func weaponClass(t string) string {
	switch t {
	case "axe", "swor", "knif", "mace", "hamm", "club",
		"spea", "pole", "staf", "scep", "wand", "h2h", "h2h2", "orb":
		return "melee_weapon"
	case "bow", "abow", "xbow", "jave", "ajav", "taxe", "tkni", "aspe":
		return "missile_weapon"
	default:
		return ""
	}
}

// weaponHand decides whether a weapon is primarily used 1-handed or 2-handed.
// D2 wclass tokens starting with "2h" or equal to "stf"/"bow"/"xbw" are 2h.
func weaponHand(itemType, wclass string, is2h bool) string {
	if is2h {
		return "2h"
	}
	// Purely 2h item types
	switch itemType {
	case "bow", "abow", "xbow", "aspe", "spea", "pole":
		return "2h"
	}
	// 2h weapon classes
	switch wclass {
	case "stf", "2hs", "2hsl", "2ht", "2hss", "bow", "xbw":
		return "2h"
	}
	return "1h"
}

// ---------------------------------------------------------------------------
// Armor
// ---------------------------------------------------------------------------

func parseArmor(url string, out map[string]baseEntry) (int, error) {
	lines, err := fetchLines(url)
	if err != nil {
		return 0, err
	}
	if len(lines) < 2 {
		return 0, fmt.Errorf("armor.txt too short")
	}

	hdr := parseHeader(lines[0])
	iName := must(hdr, "name")
	iSpawn := must(hdr, "spawnable")
	iMinAC := must(hdr, "minac")
	iMaxAC := must(hdr, "maxac")
	iGems := must(hdr, "gemsockets")
	iType := must(hdr, "type")

	log.Printf("[armor] columns: name=%d spawnable=%d minac=%d maxac=%d gemsockets=%d type=%d",
		iName, iSpawn, iMinAC, iMaxAC, iGems, iType)

	count := 0
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")

		name := get(cols, iName)
		if name == "" || strings.EqualFold(name, "Expansion") {
			continue
		}
		if get(cols, iSpawn) != "1" {
			continue
		}

		armorType := get(cols, iType)
		baseClass := armorClass(armorType)
		if baseClass == "" {
			continue // gloves, boots, belts, misc → skip
		}

		minAC := get(cols, iMinAC)
		maxAC := get(cols, iMaxAC)
		if minAC == "" || maxAC == "" {
			continue
		}

		maxSockets, _ := strconv.Atoi(get(cols, iGems))
		// Keep 0-socket items too — they still appear in gear lists even if not runeword bases

		key := normKey(name)
		if _, exists := out[key]; !exists {
			out[key] = baseEntry{
				Name:       name,
				TypeCode:   armorType,
				BaseClass:  baseClass,
				Hand:       "n/a",
				Defense:    minAC + "-" + maxAC,
				MaxSockets: maxSockets,
			}
			count++
		}
	}
	return count, nil
}

func armorClass(t string) string {
	switch t {
	case "helm", "pelt", "phlm", "circ":
		return "helm"
	case "tors":
		return "body_armor"
	case "shie", "ashd", "head":
		return "shield"
	default:
		return "" // glov, boot, belt, misc → not included
	}
}

// ---------------------------------------------------------------------------
// Key normalisation
// ---------------------------------------------------------------------------

func normKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

// ---------------------------------------------------------------------------
// Output
// ---------------------------------------------------------------------------

func writeOutput(entries map[string]baseEntry) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	out := filepath.Join(wd, outPath)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}

	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	header(w)

	fmt.Fprintln(w, "// d2rBaseCatalog maps normalised item name → base stats.")
	fmt.Fprintln(w, "// All three tiers (normal / exceptional / elite) are included.")
	fmt.Fprintln(w, "// Unique, set, and quest items are excluded.")
	fmt.Fprintln(w, "var d2rBaseCatalog = map[string]catalogBase{")
	for _, k := range keys {
		e := entries[k]
		fmt.Fprintf(w,
			"\t%q: {Name: %q, TypeCode: %q, BaseClass: %q, Hand: %q, Damage: %q, Defense: %q, WeaponSpeed: %q, MaxSockets: %d},\n",
			k, e.Name, e.TypeCode, e.BaseClass, e.Hand, e.Damage, e.Defense, e.WeaponSpeed, e.MaxSockets)
	}
	fmt.Fprintln(w, "}")

	return w.Flush()
}

func header(w io.Writer) {
	fmt.Fprintln(w, "package d2r")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "// Code generated by tools/gen_base_catalog — DO NOT EDIT.")
	fmt.Fprintln(w, "// Re-generate:  go run ./tools/gen_base_catalog/")
	fmt.Fprintln(w, "// Source:       https://github.com/pinkufairy/D2R-Excel")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "// catalogBase holds deterministic D2R stats for a base item.")
	fmt.Fprintln(w, "type catalogBase struct {")
	fmt.Fprintln(w, "\tName        string")
	fmt.Fprintln(w, "\tTypeCode    string // source item type code, e.g. swor, staf, helm, tors")
	fmt.Fprintln(w, "\tBaseClass   string // melee_weapon | missile_weapon | helm | body_armor | shield")
	fmt.Fprintln(w, "\tHand        string // 1h | 2h | n/a")
	fmt.Fprintln(w, "\tDamage      string // \"min-max\" for weapons, empty for armor")
	fmt.Fprintln(w, "\tDefense     string // \"min-max\" for armor, empty for weapons")
	fmt.Fprintln(w, "\tWeaponSpeed string // numeric base speed, e.g. \"0\", \"-10\", \"10\"")
	fmt.Fprintln(w, "\tMaxSockets  int")
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w)
}
