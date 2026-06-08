package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
	if data == nil {
		data = map[string]any{}
	}

	charClass := strings.TrimSpace(fmt.Sprint(data["class"]))
	if charClass == "" || charClass == "<nil>" {
		return fmt.Errorf("character %q has no class set", character)
	}

	cfg, err := readConfig()
	if err != nil {
		return err
	}

	entry, err := resolveGearWithLLM(gear, charClass, cfg)
	if err != nil {
		return fmt.Errorf("resolve gear details: %w", err)
	}

	if strings.TrimSpace(fmt.Sprint(entry["exact_name"])) == "" {
		entry["exact_name"] = gear
	}
	if strings.TrimSpace(fmt.Sprint(entry["query"])) == "" {
		entry["query"] = gear
	}

	gearList := coerceGearEntries(data["gear"])
	data["gear"] = append(gearList, entry)

	updated, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal character file: %w", err)
	}

	if err := os.WriteFile(characterFile, updated, 0o644); err != nil {
		return fmt.Errorf("write character file: %w", err)
	}

	cmd.Printf("added %q to %q\n", gear, character)
	return nil
}

func coerceGearEntries(v any) []map[string]any {
	if v == nil {
		return []map[string]any{}
	}

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
			return []map[string]any{}
		}
		return []map[string]any{{"exact_name": text, "query": text}}
	}
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
