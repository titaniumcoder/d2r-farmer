package cmd

import (
	"fmt"
	"os"
	"path/filepath"

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
