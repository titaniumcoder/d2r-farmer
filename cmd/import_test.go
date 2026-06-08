package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportAddsGuideGear(t *testing.T) {
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	prevResolver := resolveGearWithLLM
	prevExtractor := extractGuideGearWithLLM

	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(prevWd)
		resolveGearWithLLM = prevResolver
		extractGuideGearWithLLM = prevExtractor
	})

	if err := os.MkdirAll(filepath.Join("data", "chars"), 0o755); err != nil {
		t.Fatalf("mkdir chars failed: %v", err)
	}
	if err := os.MkdirAll("data", 0o755); err != nil {
		t.Fatalf("mkdir data failed: %v", err)
	}

	cfg := "provider: openai\nopenai:\n  api_key: test-key\n  model: gpt-4.1-mini\n"
	if err := os.WriteFile(filepath.Join("data", "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("config write failed: %v", err)
	}

	seed := "name: \"fury\"\nclass: \"druid\"\n"
	if err := os.WriteFile(filepath.Join("data", "chars", "fury.yaml"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}

	extractGuideGearWithLLM = func(url string, cfg Config) ([]guideGearItem, error) {
		return []guideGearItem{
			{Slot: "Weapons", Item: "Breath of the Dying"},
			{Slot: "Helmets", Item: "Wisdom"},
		}, nil
	}

	resolveGearWithLLM = func(query string, className string, slotHint string, cfg Config) (map[string]any, error) {
		entry := map[string]any{
			"exact_name": query,
			"query":      query,
			"slot":       slotHint,
			"kind":       "runeword",
			"runes":      []string{"El"},
		}
		if strings.EqualFold(query, "wisdom") {
			entry["possible_bases"] = []string{"Pelt"}
			entry["best_in_slot_base"] = "Pelt"
		} else {
			entry["possible_bases"] = []string{"Any non-magic 6-socket weapon"}
			entry["best_in_slot_base"] = "Any non-magic 6-socket weapon"
		}
		return entry, nil
	}

	if err := runImport(importCmd, []string{"fury", "https://maxroll.gg/d2/guides/werewolf-fury-druid"}); err != nil {
		t.Fatalf("expected import to succeed, got: %v", err)
	}

	content, err := os.ReadFile(filepath.Join("data", "chars", "fury.yaml"))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	text := string(content)
	for _, expected := range []string{"Breath of the Dying", "Wisdom", "weapon:", "head:"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in imported character file, got: %s", expected, text)
		}
	}
}
