package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var removeSlot string

var removeCmd = &cobra.Command{
	Use:   "remove [character] [number]",
	Short: "Remove a tracked gear item by number",
	Long:  "Remove a tracked gear item by its index from `info` output.",
	Args:  cobra.ExactArgs(2),
	RunE:  runRemove,
}

func init() {
	rootCmd.AddCommand(removeCmd)
	removeCmd.Flags().StringVar(&removeSlot, "slot", "", "Remove by slot-specific index (weapon, head, armor, belt, ring, amulet, inventory)")
}

func runRemove(cmd *cobra.Command, args []string) error {
	character := strings.TrimSpace(args[0])
	if character == "" {
		return fmt.Errorf("character cannot be empty")
	}

	index, err := parsePositiveIndex(args[1])
	if err != nil {
		return err
	}

	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	gearList := coerceGearEntries(data["gear"])

	slot, ok := parseRemoveSlot(removeSlot)
	if removeSlot != "" && !ok {
		return fmt.Errorf("invalid slot %q (use: weapon, head, armor, belt, ring, amulet, inventory)", removeSlot)
	}

	if slot != "" {
		slotList := make([]map[string]any, 0)
		rest := make([]map[string]any, 0, len(gearList))
		for _, entry := range gearList {
			if slotForEntry(entry) == slot {
				slotList = append(slotList, entry)
			} else {
				rest = append(rest, entry)
			}
		}

		if index > len(slotList) {
			return fmt.Errorf("index %d out of range for slot %s (count: %d)", index, slot, len(slotList))
		}

		removed := gearName(slotList[index-1])
		slotList = append(slotList[:index-1], slotList[index:]...)
		updated := append(rest, slotList...)
		setGearEntries(data, updated)

		if err := writeCharacterData(character, data); err != nil {
			return err
		}

		cmd.Printf("removed %s #%d (%s) from %q\n", slot, index, removed, character)
		return nil
	}

	if index > len(gearList) {
		return fmt.Errorf("index %d out of range (gear count: %d)", index, len(gearList))
	}

	removed := gearName(gearList[index-1])
	updated := append(gearList[:index-1], gearList[index:]...)
	setGearEntries(data, updated)

	if err := writeCharacterData(character, data); err != nil {
		return err
	}

	cmd.Printf("removed #%d (%s) from %q\n", index, removed, character)
	return nil
}

func parseRemoveSlot(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", true
	}
	for _, slot := range supportedGearSlots {
		if value == slot {
			return slot, true
		}
	}
	return "", false
}
