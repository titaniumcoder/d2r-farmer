package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	gearWeaponSwap bool
	gearSlotWeapon bool
	gearSlotHead   bool
	gearSlotArmor  bool
	gearSlotBelt   bool
	gearSlotRing   bool
	gearSlotAmulet bool
	gearSlotInv    bool
)

var gearCmd = &cobra.Command{
	Use:   "gear [character] [item]",
	Short: "Add a gear item to a character",
	Long:  "Resolve item details using the configured LLM provider and append to the character file.",
	Args:  cobra.ExactArgs(2),
	RunE:  addGear,
}

func init() {
	rootCmd.AddCommand(gearCmd)
	gearCmd.Flags().BoolVar(&gearWeaponSwap, "weapon-swap", false, "Mark this gear as a weapon-swap item")
	gearCmd.Flags().BoolVar(&gearSlotWeapon, "weapon", false, "Force slot to weapon")
	gearCmd.Flags().BoolVar(&gearSlotHead, "head", false, "Force slot to head")
	gearCmd.Flags().BoolVar(&gearSlotArmor, "armor", false, "Force slot to armor")
	gearCmd.Flags().BoolVar(&gearSlotBelt, "belt", false, "Force slot to belt")
	gearCmd.Flags().BoolVar(&gearSlotRing, "ring", false, "Force slot to ring")
	gearCmd.Flags().BoolVar(&gearSlotAmulet, "amulet", false, "Force slot to amulet")
	gearCmd.Flags().BoolVar(&gearSlotInv, "inventory", false, "Force slot to inventory")
}

func addGear(cmd *cobra.Command, args []string) error {
	character := strings.TrimSpace(args[0])
	if character == "" {
		return fmt.Errorf("character cannot be empty")
	}

	gear := strings.TrimSpace(args[1])
	if gear == "" {
		return fmt.Errorf("gear cannot be empty")
	}

	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	charClass := stringValue(data["class"])
	if charClass == "" {
		return fmt.Errorf("character %q has no class set", character)
	}

	cfg, err := readConfig()
	if err != nil {
		return err
	}

	slotOverride, err := selectedGearSlot()
	if err != nil {
		return err
	}
	if gearWeaponSwap && slotOverride != "" && slotOverride != "weapon" {
		return fmt.Errorf("--weapon-swap can only be combined with --weapon")
	}

	entry, err := resolveGearWithLLM(gear, charClass, slotOverride, cfg)
	if err != nil {
		return fmt.Errorf("resolve gear details: %w", err)
	}

	if strings.TrimSpace(fmt.Sprint(entry["exact_name"])) == "" {
		entry["exact_name"] = gear
	}
	if strings.TrimSpace(fmt.Sprint(entry["query"])) == "" {
		entry["query"] = gear
	}

	entry["slot"] = normalizeSlotName(stringValue(entry["slot"]))
	if slotOverride != "" {
		entry["slot"] = slotOverride
	}

	if gearWeaponSwap {
		swapRole := normalizeSwapRole(stringValue(entry["swap_role"]))
		entry["slot"] = "weapon"
		entry["weapon_swap"] = true
		entry["swap_role"] = swapRole
	} else {
		entry["weapon_swap"] = false
	}

	gearList := coerceGearEntries(data["gear"])
	gearList = append(gearList, entry)
	setGearEntries(data, gearList)

	if err := writeCharacterData(character, data); err != nil {
		return err
	}

	cmd.Printf("added %q to %q\n", gear, character)
	return nil
}

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
	value := strings.ToLower(strings.TrimSpace(role))
	if value == "offhand" {
		return "offhand"
	}
	return "main"
}

func selectedGearSlot() (string, error) {
	selected := make([]string, 0, 7)
	if gearSlotWeapon {
		selected = append(selected, "weapon")
	}
	if gearSlotHead {
		selected = append(selected, "head")
	}
	if gearSlotArmor {
		selected = append(selected, "armor")
	}
	if gearSlotBelt {
		selected = append(selected, "belt")
	}
	if gearSlotRing {
		selected = append(selected, "ring")
	}
	if gearSlotAmulet {
		selected = append(selected, "amulet")
	}
	if gearSlotInv {
		selected = append(selected, "inventory")
	}

	if len(selected) > 1 {
		return "", fmt.Errorf("only one slot flag can be set")
	}
	if len(selected) == 0 {
		return "", nil
	}
	return selected[0], nil
}
