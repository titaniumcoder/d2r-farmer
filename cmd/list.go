package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var listCmd = &cobra.Command{
	Use:   "list [character]",
	Short: "List characters or character gear",
	Long:  "Without args, list all characters. With a character, list gear entries.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return listCharacters(cmd)
	}
	return listCharacterGear(cmd, args[0])
}

func listCharacters(cmd *cobra.Command) error {
	charsDir := filepath.Join("data", "chars")
	entries, err := os.ReadDir(charsDir)
	if err != nil {
		if os.IsNotExist(err) {
			cmd.Println("no characters found")
			return nil
		}
		return fmt.Errorf("read chars directory: %w", err)
	}

	var chars []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if name != "" {
			chars = append(chars, name)
		}
	}
	sort.Strings(chars)

	if len(chars) == 0 {
		cmd.Println("no characters found")
		return nil
	}

	for _, name := range chars {
		cmd.Println(name)
	}
	return nil
}

func listCharacterGear(cmd *cobra.Command, character string) error {
	characterFile := filepath.Join("data", "chars", slugifyName(character)+".yaml")
	content, err := os.ReadFile(characterFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("character does not exist: %s", characterFile)
		}
		return fmt.Errorf("read character file: %w", err)
	}

	var data map[string]any
	if err := yaml.Unmarshal(content, &data); err != nil {
		return fmt.Errorf("parse character file: %w", err)
	}

	gear := coerceGearEntries(data["gear"])
	if len(gear) == 0 {
		cmd.Printf("no gear for %q\n", character)
		return nil
	}

	for i, entry := range gear {
		exact := strings.TrimSpace(fmt.Sprint(entry["exact_name"]))
		if exact == "" || exact == "<nil>" {
			exact = strings.TrimSpace(fmt.Sprint(entry["query"]))
		}
		slot := strings.TrimSpace(fmt.Sprint(entry["slot"]))
		kind := strings.TrimSpace(fmt.Sprint(entry["kind"]))
		cmd.Printf("%d. %s (%s, %s)\n", i+1, exact, slot, kind)
	}
	return nil
}
