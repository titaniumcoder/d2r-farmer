package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

var (
	basesSet  string
	basesBest string
)

var basesCmd = &cobra.Command{
	Use:   "bases [character] [number]",
	Short: "Edit possible bases for a tracked gear item",
	Long:  "Set possible bases for a tracked runeword. Interactive mode supports selecting by number.",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runBases,
}

func init() {
	rootCmd.AddCommand(basesCmd)
	basesCmd.Flags().StringVar(&basesSet, "set", "", "Comma-separated possible bases list")
	basesCmd.Flags().StringVar(&basesBest, "best", "", "Comma-separated prioritized best-in-slot bases (first = top priority)")
}

func runBases(cmd *cobra.Command, args []string) error {
	character := strings.TrimSpace(args[0])
	if character == "" {
		return fmt.Errorf("character cannot be empty")
	}

	if len(args) == 1 {
		return runBasesInteractive(cmd, character)
	}

	index, err := parsePositiveIndex(args[1])
	if err != nil {
		return err
	}

	setBases := splitCSV(basesSet)
	bestBase := strings.TrimSpace(basesBest)
	if len(setBases) == 0 && bestBase == "" {
		return fmt.Errorf("provide --set and/or --best")
	}

	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	gearList := coerceGearEntries(data["gear"])
	if index > len(gearList) {
		return fmt.Errorf("index %d out of range (gear count: %d)", index, len(gearList))
	}

	entry := gearList[index-1]
	if len(setBases) > 0 {
		entry["possible_bases"] = setBases
	}

	bestList := splitCSV(bestBase)
	if len(bestList) == 0 {
		if len(setBases) > 0 {
			bestList = append([]string{}, setBases...)
		} else if single := stringValue(entry["best_in_slot_base"]); single != "" {
			bestList = []string{single}
		}
	}

	if len(bestList) > 0 {
		entry["best_in_slot_bases"] = bestList
		entry["best_in_slot_base"] = bestList[0]
	}

	setGearEntries(data, gearList)
	if err := writeCharacterData(character, data); err != nil {
		return err
	}

	cmd.Printf("updated bases for #%d (%s) on %q\n", index, gearName(entry), character)
	return nil
}

func runBasesInteractive(cmd *cobra.Command, character string) error {
	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	gearList := coerceGearEntries(data["gear"])
	type candidate struct {
		Index int
		Entry map[string]any
	}

	runewords := make([]candidate, 0)
	for i, entry := range gearList {
		if strings.EqualFold(stringValue(entry["kind"]), "runeword") {
			runewords = append(runewords, candidate{Index: i, Entry: entry})
		}
	}

	if len(runewords) == 0 {
		return fmt.Errorf("no runewords found for %q", character)
	}

	cmd.Println("select runeword:")
	for i, item := range runewords {
		cmd.Printf("%d. %s\n", i+1, gearName(item.Entry))
	}
	cmd.Print("number: ")

	reader := bufio.NewReader(cmd.InOrStdin())
	rawSelection, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("read selection: %w", err)
	}

	selection, err := parsePositiveIndex(rawSelection)
	if err != nil {
		return fmt.Errorf("invalid selection: %w", err)
	}
	if selection > len(runewords) {
		return fmt.Errorf("selection %d out of range", selection)
	}

	target := runewords[selection-1]
	cmd.Printf("selected: %s\n", gearName(target.Entry))

	cmd.Println("current bases:")
	for _, base := range stringSliceValue(target.Entry["possible_bases"]) {
		cmd.Printf("- %s\n", base)
	}
	cmd.Println("enter comma-separated bases (first is best):")
	cmd.Print("> ")

	rawBases, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("read bases: %w", err)
	}

	bases := splitCSV(rawBases)
	if len(bases) == 0 {
		return fmt.Errorf("at least one base is required")
	}

	cmd.Println("enter prioritized best-in-slot bases (comma-separated, blank = use bases order):")
	cmd.Print("> ")

	rawBest, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("read best bases: %w", err)
	}

	bestList := splitCSV(rawBest)
	if len(bestList) == 0 {
		bestList = append([]string{}, bases...)
	}

	entry := target.Entry
	entry["possible_bases"] = bases
	entry["best_in_slot_bases"] = bestList
	entry["best_in_slot_base"] = bestList[0]

	gearList[target.Index] = entry
	setGearEntries(data, gearList)
	if err := writeCharacterData(character, data); err != nil {
		return err
	}

	cmd.Printf("updated bases for %s (best: %s)\n", gearName(entry), bestList[0])
	return nil
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
