package d2r

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func readCharacterData(character string) (map[string]any, error) {
	characterFile := filepath.Join("data", "chars", slugifyName(character)+".yaml")
	content, err := os.ReadFile(characterFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("character does not exist: %s", characterFile)
		}
		return nil, fmt.Errorf("read character file: %w", err)
	}

	var data map[string]any
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("parse character file: %w", err)
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

func writeCharacterData(character string, data map[string]any) error {
	characterFile := filepath.Join("data", "chars", slugifyName(character)+".yaml")
	updated, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal character file: %w", err)
	}
	if err := os.WriteFile(characterFile, updated, 0o644); err != nil {
		return fmt.Errorf("write character file: %w", err)
	}
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
		doc.Requirements = requirements{Mandatory: mandatory}
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
