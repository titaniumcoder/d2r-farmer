package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var foundCmd = &cobra.Command{
	Use:   "found [character] [gear]",
	Short: "Mark a tracked gear item as found",
	Long:  "Mark a tracked gear item as found after verifying it exists in the character gear list.",
	Args:  cobra.ExactArgs(2),
	RunE:  runFound,
}

func init() {
	rootCmd.AddCommand(foundCmd)
}

func runFound(cmd *cobra.Command, args []string) error {
	character := strings.TrimSpace(args[0])
	if character == "" {
		return fmt.Errorf("character cannot be empty")
	}

	item := strings.TrimSpace(args[1])
	if item == "" {
		return fmt.Errorf("gear cannot be empty")
	}

	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	gearList := coerceGearEntries(data["gear"])
	idx := findGearEntryIndex(gearList, item)
	if idx < 0 {
		return fmt.Errorf("gear %q is not in %q list (add it first with `gear`)", item, character)
	}

	if gearFound(gearList[idx]) {
		cmd.Printf("gear %q is already marked as found for %q\n", item, character)
		return nil
	}

	gearList[idx]["found"] = true
	gearList[idx]["found_at"] = time.Now().UTC().Format(time.RFC3339)
	setGearEntries(data, gearList)

	if err := writeCharacterData(character, data); err != nil {
		return err
	}

	cmd.Printf("marked %q as found for %q\n", item, character)
	return nil
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
