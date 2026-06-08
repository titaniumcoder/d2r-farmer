package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

type gearResolution struct {
	ExactName      string   `json:"exact_name"`
	Slot           string   `json:"slot"`
	Kind           string   `json:"kind"`
	Runes          []string `json:"runes"`
	PossibleBases  []string `json:"possible_bases"`
	BestInSlotBase string   `json:"best_in_slot_base"`
	Notes          string   `json:"notes"`
	Sources        []string `json:"sources"`
}

var resolveGearWithLLM = resolveGearDetails

func resolveGearDetails(query string, className string, cfg Config) (map[string]any, error) {
	if cfg.Provider != "openai" {
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}

	return resolveWithOpenAI(query, className, cfg.OpenAI)
}

func resolveWithOpenAI(query string, className string, cfg OpenAIConfig) (map[string]any, error) {
	client := newOpenAIClient(cfg)
	systemPrompt := strings.TrimSpace(`You are a Diablo II: Resurrected item resolution engine.
Return strict JSON only.
Use current official D2R data and avoid outdated or modded content.
Ignore stale pre-warlock community data unless it is still current and verified.
Prefer the live item name, slot, item kind, runes, bases, and the best class-specific base.
If uncertain, return kind="unknown" with empty arrays and concise notes.`)

	userPrompt := fmt.Sprintf(
		"Resolve this requested item for class %q: %q. Return exact_name, slot, kind, runes, possible_bases, best_in_slot_base, notes, and sources.",
		className,
		query,
	)

	response, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model: shared.ChatModel(cfg.Model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			{
				OfSystem: &openai.ChatCompletionSystemMessageParam{
					Content: openai.ChatCompletionSystemMessageParamContentUnion{
						OfString: openai.String(systemPrompt),
					},
				},
			},
			{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{
						OfString: openai.String(userPrompt),
					},
				},
			},
		},
		Temperature: openai.Float(0.1),
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
		"exact_name":        strings.TrimSpace(resolution.ExactName),
		"slot":              strings.TrimSpace(resolution.Slot),
		"kind":              strings.TrimSpace(resolution.Kind),
		"runes":             resolution.Runes,
		"possible_bases":    resolution.PossibleBases,
		"best_in_slot_base": strings.TrimSpace(resolution.BestInSlotBase),
		"notes":             strings.TrimSpace(resolution.Notes),
		"sources":           resolution.Sources,
		"query":             query,
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

	return out, nil
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
			"runes": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"possible_bases": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"best_in_slot_base": map[string]any{"type": "string"},
			"notes":             map[string]any{"type": "string"},
			"sources": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required":             []string{"exact_name", "slot", "kind", "runes", "possible_bases", "best_in_slot_base", "notes", "sources"},
		"additionalProperties": false,
	}
}
