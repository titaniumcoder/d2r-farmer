package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info [character]",
	Short: "Show detailed character info",
	Long:  "Show class and gear details with needed/known/prio flags.",
	Args:  cobra.ExactArgs(1),
	RunE:  runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)
}

func runInfo(cmd *cobra.Command, args []string) error {
	character := strings.TrimSpace(args[0])
	if character == "" {
		return fmt.Errorf("character cannot be empty")
	}

	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	className := stringValue(data["class"])
	if className == "" {
		className = "unknown"
	}

	cmd.Printf("class: %s\n", className)

	mandatory := readMandatoryRequirements(data)
	if len(mandatory) == 0 {
		cmd.Println("mandatory requirements: none")
	} else {
		cmd.Printf("mandatory requirements: %s\n", strings.Join(mandatory, ", "))
	}

	cmd.Println("gear:")

	gearList := coerceGearEntries(data["gear"])
	statuses := buildGearStatuses(gearList)
	if len(statuses) == 0 {
		for _, slot := range supportedGearSlots {
			cmd.Printf("%s:\n", slot)
			cmd.Println("  - none")
		}
		return nil
	}

	bySlot := map[string][]gearStatus{}
	for _, status := range statuses {
		bySlot[status.Slot] = append(bySlot[status.Slot], status)
	}

	globalIndex := 0
	weapon := bySlot["weapon"]
	mainWeapon := filterStatuses(weapon, func(s gearStatus) bool { return !s.IsSwap && s.SwapRole != "offhand" })
	offhand := filterStatuses(weapon, func(s gearStatus) bool { return !s.IsSwap && s.SwapRole == "offhand" })
	swapMain := filterStatuses(weapon, func(s gearStatus) bool { return s.IsSwap && s.SwapRole != "offhand" })
	swapOffhand := filterStatuses(weapon, func(s gearStatus) bool { return s.IsSwap && s.SwapRole == "offhand" })

	globalIndex = printInfoSection(cmd, "weapon", mainWeapon, globalIndex)
	globalIndex = printInfoSection(cmd, "offhand", offhand, globalIndex)
	globalIndex = printInfoSection(cmd, "weapon_swap_weapon", swapMain, globalIndex)
	globalIndex = printInfoSection(cmd, "weapon_swap_offhand", swapOffhand, globalIndex)

	for _, slot := range []string{"head", "armor", "belt", "ring", "amulet", "inventory"} {
		globalIndex = printInfoSection(cmd, slot, bySlot[slot], globalIndex)
	}

	return nil
}

func printInfoSection(cmd *cobra.Command, label string, items []gearStatus, start int) int {
	cmd.Printf("%s:\n", label)
	if len(items) == 0 {
		cmd.Println("  - none")
		return start
	}

	idx := start
	for _, status := range items {
		idx++

		flags := make([]string, 0, 3)
		if status.Needed {
			flags = append(flags, "needed")
		}
		if status.Prio {
			flags = append(flags, "prio")
		}
		if status.Known {
			flags = append(flags, "known")
		}

		flagText := strings.Join(flags, ", ")
		if flagText == "" {
			flagText = "none"
		}

		cmd.Printf("  %d. %s [%s]\n", idx, status.Name, flagText)
		cmd.Printf("     Type: %s\n", status.Kind)

		if status.Kind == "runeword" {
			runes := "none"
			if len(status.Runes) > 0 {
				runes = strings.Join(status.Runes, " -> ")
			}
			cmd.Printf("     Runes: %s\n", runes)
		}

		cmd.Println("     Bases:")
		if len(status.Bases) == 0 {
			cmd.Println("     - none")
		} else {
			for _, base := range status.Bases {
				cmd.Printf("     - %s\n", base)
			}
		}

		cmd.Println("     Best in Slot (priority):")
		if len(status.BestBases) == 0 {
			cmd.Println("     - none")
		} else {
			for i, base := range status.BestBases {
				cmd.Printf("     %d. %s\n", i+1, base)
			}
		}
	}

	return idx
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
