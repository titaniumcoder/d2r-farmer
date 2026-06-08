package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var charClass string
var charMandatory []string

var charCmd = &cobra.Command{
	Use:   "char [name]",
	Short: "Create a new character",
	Long:  "Create a new character YAML file under data/chars.",
	Args:  cobra.ExactArgs(1),
	RunE:  addCharacter,
}

func init() {
	rootCmd.AddCommand(charCmd)
	charCmd.Flags().StringVarP(&charClass, "class", "c", "", "Character class (e.g. sorceress, paladin)")
	charCmd.Flags().StringSliceVar(&charMandatory, "mandatory", nil, "Mandatory requirement (repeatable), e.g. cannot be frozen")
	_ = charCmd.MarkFlagRequired("class")
}

func addCharacter(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if name == "" {
		return fmt.Errorf("character name cannot be empty")
	}

	class := strings.TrimSpace(charClass)
	if class == "" {
		return fmt.Errorf("class cannot be empty")
	}

	mandatory := make([]string, 0, len(charMandatory))
	for _, req := range charMandatory {
		req = strings.TrimSpace(req)
		if req != "" {
			mandatory = append(mandatory, req)
		}
	}

	fileName := slugifyName(name)
	if fileName == "" {
		return fmt.Errorf("character name %q has no valid filename characters", name)
	}

	charsDir := filepath.Join("data", "chars")
	if err := os.MkdirAll(charsDir, 0o755); err != nil {
		return fmt.Errorf("create chars directory: %w", err)
	}

	filePath := filepath.Join(charsDir, fileName+".yaml")
	if _, err := os.Stat(filePath); err == nil {
		return fmt.Errorf("character already exists: %s", filePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check character file: %w", err)
	}

	content, err := buildCharacterYAML(name, class, mandatory)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write character file: %w", err)
	}

	cmd.Printf("initialized character %q at %s\n", name, filePath)
	return nil
}

func buildCharacterYAML(name string, class string, mandatory []string) (string, error) {
	createdAt := time.Now().UTC().Format(time.RFC3339)

	type requirements struct {
		Mandatory []string `yaml:"mandatory,omitempty"`
	}

	type characterDocument struct {
		Name         string       `yaml:"name"`
		Class        string       `yaml:"class"`
		CreatedAt    string       `yaml:"created_at"`
		Requirements requirements `yaml:"requirements,omitempty"`
	}

	doc := characterDocument{
		Name:      name,
		Class:     class,
		CreatedAt: createdAt,
	}

	if len(mandatory) > 0 {
		doc.Requirements = requirements{
			Mandatory: mandatory,
		}
	}

	content, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal character file: %w", err)
	}

	return string(content), nil
}

var invalidSlugChars = regexp.MustCompile(`[^a-z0-9_-]+`)

func slugifyName(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = invalidSlugChars.ReplaceAllString(slug, "")
	slug = strings.Trim(slug, "-")
	return slug
}
