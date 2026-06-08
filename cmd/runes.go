package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var runesCmd = &cobra.Command{
	Use:   "runes [character]",
	Short: "List needed runes for a character",
	Long:  "Show all runes still needed for tracked gear that is not yet found.",
	Args:  cobra.ExactArgs(1),
	RunE:  runRunes,
}

func init() {
	rootCmd.AddCommand(runesCmd)
}

func runRunes(cmd *cobra.Command, args []string) error {
	character := strings.TrimSpace(args[0])
	if character == "" {
		return fmt.Errorf("character cannot be empty")
	}

	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	gearList := coerceGearEntries(data["gear"])
	counts := map[string]int{}
	order := make([]string, 0)

	for _, entry := range gearList {
		if gearFound(entry) {
			continue
		}

		for _, runeName := range stringSliceValue(entry["runes"]) {
			runeName = canonicalRuneName(runeName)
			if runeName == "" {
				continue
			}
			if _, seen := counts[runeName]; !seen {
				order = append(order, runeName)
			}
			counts[runeName]++
		}
	}

	if len(order) == 0 {
		cmd.Println("no needed runes")
		return nil
	}

	sort.Slice(order, func(i, j int) bool {
		left := runeDifficultyOrder(order[i])
		right := runeDifficultyOrder(order[j])
		if left == right {
			return order[i] < order[j]
		}
		return left < right
	})

	cmd.Println("needed runes:")
	for _, runeName := range order {
		count := counts[runeName]
		countess := countessDifficultiesForRune(runeName)
		countessText := ""
		if len(countess) > 0 {
			countessText = fmt.Sprintf("countess: %s", strings.Join(countess, ", "))
		}

		if count == 1 {
			if countessText == "" {
				cmd.Printf("- %s\n", runeName)
			} else {
				cmd.Printf("- %s (%s)\n", runeName, countessText)
			}
			continue
		}
		if countessText == "" {
			cmd.Printf("- %s x%d\n", runeName, count)
		} else {
			cmd.Printf("- %s x%d (%s)\n", runeName, count, countessText)
		}
	}

	return nil
}

var runeOrder = []string{
	"El", "Eld", "Tir", "Nef", "Eth", "Ith", "Tal", "Ral", "Ort", "Thul", "Amn", "Sol",
	"Shael", "Dol", "Hel", "Io", "Lum", "Ko", "Fal", "Lem", "Pul", "Um", "Mal", "Ist",
	"Gul", "Vex", "Ohm", "Lo", "Sur", "Ber", "Jah", "Cham", "Zod",
}

var runeOrderIndex = buildRuneOrderIndex()

func buildRuneOrderIndex() map[string]int {
	out := make(map[string]int, len(runeOrder))
	for idx, runeName := range runeOrder {
		out[strings.ToLower(runeName)] = idx
	}
	return out
}

func canonicalRuneName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	for _, runeName := range runeOrder {
		if strings.ToLower(runeName) == lower {
			return runeName
		}
	}
	if len(lower) == 1 {
		return strings.ToUpper(lower)
	}
	return strings.ToUpper(lower[:1]) + lower[1:]
}

func runeDifficultyOrder(runeName string) int {
	if idx, ok := runeOrderIndex[strings.ToLower(runeName)]; ok {
		return idx
	}
	return len(runeOrder) + 1000
}

func countessDifficultiesForRune(runeName string) []string {
	idx := runeDifficultyOrder(runeName)
	if idx > runeDifficultyOrder("Ist") {
		return nil
	}

	out := make([]string, 0, 3)
	if idx <= runeDifficultyOrder("Ral") {
		out = append(out, "normal")
	}
	if idx <= runeDifficultyOrder("Ko") {
		out = append(out, "nightmare")
	}
	out = append(out, "hell")
	return out
}
