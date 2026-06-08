package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List characters",
	Long:  "List all tracked characters.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listCharacters(cmd)
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
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
