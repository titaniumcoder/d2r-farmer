package d2r

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

type gearResolution struct {
	ExactName      string   `json:"exact_name"`
	Slot           string   `json:"slot"`
	Kind           string   `json:"kind"`
	SwapRole       string   `json:"swap_role"`
	Runes          []string `json:"runes"`
	PossibleBases  []string `json:"possible_bases"`
	BestInSlotBase string   `json:"best_in_slot_base"`
	BestInSlotList []string `json:"best_in_slot_bases"`
	Effects        []string `json:"effects"`
	Notes          string   `json:"notes"`
	Sources        []string `json:"sources"`
}

var resolveGearWithLLM = resolveGearDetails

func resolveGearDetails(query string, className string, slotHint string, cfg Config) (map[string]any, error) {
	if cfg.Provider != "openai" {
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
	return resolveWithOpenAI(query, className, slotHint, cfg.OpenAI)
}

func resolveWithOpenAI(query string, className string, slotHint string, cfg OpenAIConfig) (map[string]any, error) {
	client := newOpenAIClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	systemPrompt := strings.TrimSpace(`You are a Diablo II: Resurrected item resolution engine.
Return strict JSON only. Use current official D2R data.

CRITICAL — determine kind carefully:
- "runeword": item made by inserting runes into a socketed base (has a rune recipe, e.g. Grief, Enigma, Fury)
- "unique": a specific named magic item drop (e.g. Shako, Ribcracker, Harlequin Crest, Thunderstroke)
- "set": part of a named set (e.g. Tal Rasha's Wrappings)
- "crafted": player-crafted via cube recipe
- "rare": rare base item
- "magic": magic base item
- "base": plain non-magic base (e.g. Monarch)
- "unknown": if truly uncertain

Unique items have runes=[] and possible_bases=[].
Runewords have a non-empty rune list and non-empty possible_bases.

If a slot hint is provided, honor it exactly.
For runewords: possible_bases lists all legal non-magic bases; best_in_slot_base is the preferred base for this class.
For weapon swap planning, infer swap_role (main|offhand) where applicable.
effects: list all notable stats and properties as concise strings, e.g. "Enhanced Damage: 340-400%", "Indestructible", "Prevent Monster Heal", "Cannot Be Frozen". Include weapon speed and damage range for weapons. Always populate this field.`)

	userPrompt := fmt.Sprintf(
		"Resolve this item for class %q: %q. Slot hint: %q. Return exact_name, slot, kind, swap_role, runes, possible_bases, best_in_slot_base, best_in_slot_bases (priority order), effects (all notable stats/properties as short strings), notes, sources.",
		className, query, slotHint,
	)

	response, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: shared.ChatModel(cfg.Model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			{
				OfSystem: &openai.ChatCompletionSystemMessageParam{
					Content: openai.ChatCompletionSystemMessageParamContentUnion{OfString: openai.String(systemPrompt)},
				},
			},
			{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{OfString: openai.String(userPrompt)},
				},
			},
		},
		Temperature: openai.Float(0),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				Type: "json_schema",
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:        "gear_enrichment",
					Description: openai.String("Structured gear enrichment output"),
					Strict:      openai.Bool(true),
					Schema:      gearSchema(),
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create openai chat completion: %w", err)
	}
	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no choices")
	}

	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		return nil, fmt.Errorf("openai returned empty content")
	}

	var resolution gearResolution
	if err := json.Unmarshal([]byte(content), &resolution); err != nil {
		return nil, fmt.Errorf("parse model JSON content: %w", err)
	}

	out := map[string]any{
		"exact_name":         strings.TrimSpace(resolution.ExactName),
		"slot":               strings.TrimSpace(resolution.Slot),
		"kind":               strings.TrimSpace(resolution.Kind),
		"swap_role":          strings.TrimSpace(resolution.SwapRole),
		"runes":              resolution.Runes,
		"possible_bases":     resolution.PossibleBases,
		"best_in_slot_base":  strings.TrimSpace(resolution.BestInSlotBase),
		"best_in_slot_bases": resolution.BestInSlotList,
		"effects":            resolution.Effects,
		"notes":              strings.TrimSpace(resolution.Notes),
		"sources":            resolution.Sources,
		"query":              query,
	}

	if out["exact_name"] == "" {
		out["exact_name"] = query
	}
	if out["slot"] == "" {
		out["slot"] = "unknown"
	}
	if out["kind"] == "" {
		out["kind"] = "unknown"
	}
	if out["swap_role"] == "" {
		out["swap_role"] = "main"
	}
	if len(stringSliceValue(out["best_in_slot_bases"])) == 0 {
		if best := stringValue(out["best_in_slot_base"]); best != "" {
			out["best_in_slot_bases"] = []string{best}
		}
	}

	applyRunewordBaseRules(out)
	return out, nil
}

func applyRunewordBaseRules(entry map[string]any) {
	if strings.ToLower(stringValue(entry["kind"])) != "runeword" {
		return
	}

	name := strings.ToLower(stringValue(entry["exact_name"]))
	query := strings.ToLower(stringValue(entry["query"]))

	if strings.Contains(name, "breath of the dying") || strings.Contains(query, "breath of the dying") || strings.Contains(query, "breath of fury") {
		entry["possible_bases"] = []string{"Any non-magic 6-socket weapon"}
		return
	}
	if strings.Contains(name, "wisdom") || strings.Contains(query, "wisdom") {
		entry["possible_bases"] = []string{"Any non-magic helm (including class-specific helms such as druid pelts)"}
	}
}

func newOpenAIClient(cfg OpenAIConfig) *openai.Client {
	client := openai.NewClient(option.WithAPIKey(cfg.APIKey))
	return &client
}

func gearSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"exact_name": map[string]any{"type": "string"},
			"slot":       map[string]any{"type": "string"},
			"kind":       map[string]any{"type": "string"},
			"swap_role":  map[string]any{"type": "string"},
			"runes": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"possible_bases": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"best_in_slot_base": map[string]any{"type": "string"},
			"best_in_slot_bases": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"effects": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"notes": map[string]any{"type": "string"},
			"sources": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required":             []string{"exact_name", "slot", "kind", "swap_role", "runes", "possible_bases", "best_in_slot_base", "best_in_slot_bases", "effects", "notes", "sources"},
		"additionalProperties": false,
	}
}
