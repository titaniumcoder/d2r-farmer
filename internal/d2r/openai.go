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
	ExactName      string        `json:"exact_name"`
	Slot           string        `json:"slot"`
	Kind           string        `json:"kind"`
	SwapRole       string        `json:"swap_role"`
	Runes          []string      `json:"runes"`
	PossibleBases  []string      `json:"possible_bases"`
	BaseDetails    []baseDetails `json:"possible_bases_details"`
	BestInSlotBase string        `json:"best_in_slot_base"`
	BestInSlotList []string      `json:"best_in_slot_bases"`
	Effects        []string      `json:"effects"`
	Notes          string        `json:"notes"`
	Sources        []string      `json:"sources"`
}

type baseDetails struct {
	Name        string `json:"name"`
	BaseClass   string `json:"base_class"`
	Hand        string `json:"hand"`
	Defense     string `json:"defense"`
	Damage      string `json:"damage"`
	WeaponSpeed string `json:"weapon_speed"`
}

type runewordVerification struct {
	Slot        string        `json:"slot"`
	Runes       []string      `json:"runes"`
	Bases       []string      `json:"possible_bases"`
	BaseDetails []baseDetails `json:"possible_bases_details"`
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

Assume modern Diablo II: Resurrected rules where ladder runewords are available. Do not downgrade or remap runewords based on old ladder-only restrictions.
The rune recipe and legal base class must match the resolved runeword exactly.

Unique items have runes=[] and possible_bases=[].
Runewords have a non-empty rune list and non-empty possible_bases.

If a slot hint is provided, honor it exactly.
For runewords: possible_bases must contain the FULL legal non-magic base list for that exact runeword (do not return only top picks).
possible_bases_details must include one object per legal base with: name, base_class, hand ("1h"|"2h"|"n/a"), defense (armor/shields/helms), damage (weapons), weapon_speed (weapons).
base_class must be one of: melee_weapon, missile_weapon, shield, helm, body_armor, other.
For Harmony-like runewords, do not include melee weapon bases; use missile_weapon bases only.
Runeword slot must be accurate and can be weapon, offhand, head, or armor depending on legal base class.
For non-weapons use hand="n/a" and leave damage/weapon_speed empty.
For non-armor use defense empty.
best_in_slot_base is optional context only.
For weapon swap planning, infer swap_role (main|offhand) where applicable.
effects: list all notable stats and properties as concise strings, e.g. "Enhanced Damage: 340-400%", "Indestructible", "Prevent Monster Heal", "Cannot Be Frozen". Include weapon speed and damage range for weapons. Always populate this field.`)

	userPrompt := fmt.Sprintf(
		"Resolve this item for class %q: %q. Slot hint: %q. Return exact_name, slot, kind, swap_role, runes, possible_bases, possible_bases_details, best_in_slot_base, best_in_slot_bases (priority order), effects (all notable stats/properties as short strings), notes, sources.",
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
		"exact_name":             strings.TrimSpace(resolution.ExactName),
		"slot":                   strings.TrimSpace(resolution.Slot),
		"kind":                   strings.TrimSpace(resolution.Kind),
		"swap_role":              strings.TrimSpace(resolution.SwapRole),
		"runes":                  resolution.Runes,
		"possible_bases":         resolution.PossibleBases,
		"possible_bases_details": resolution.BaseDetails,
		"best_in_slot_base":      strings.TrimSpace(resolution.BestInSlotBase),
		"best_in_slot_bases":     resolution.BestInSlotList,
		"effects":                resolution.Effects,
		"notes":                  strings.TrimSpace(resolution.Notes),
		"sources":                resolution.Sources,
		"query":                  query,
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

	if strings.EqualFold(stringValue(out["kind"]), "runeword") {
		verified, verifyErr := verifyRunewordRecipeWithOpenAI(client, cfg, query, stringValue(out["exact_name"]), slotHint)
		if verifyErr == nil {
			if verified.Slot != "" {
				out["slot"] = strings.TrimSpace(verified.Slot)
			}
			if len(verified.Runes) > 0 {
				out["runes"] = verified.Runes
			}
			if len(verified.Bases) > 0 {
				out["possible_bases"] = verified.Bases
			}
			if len(verified.BaseDetails) > 0 {
				out["possible_bases_details"] = verified.BaseDetails
			}
		}
		normalizedSlot, normalizedBases, normalizedDetails := sanitizeRunewordSlotAndBases(stringValue(out["slot"]), stringSliceValue(out["possible_bases"]), baseDetailsSliceValue(out["possible_bases_details"]))
		out["slot"] = normalizedSlot
		out["possible_bases"] = normalizedBases
		out["possible_bases_details"] = normalizedDetails
	}
	return out, nil
}

func verifyRunewordRecipeWithOpenAI(client *openai.Client, cfg OpenAIConfig, query string, exactName string, slotHint string) (runewordVerification, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	systemPrompt := strings.TrimSpace(`You are an independent Diablo II: Resurrected runeword verifier.
Return strict JSON only.
Assume modern D2R rules where ladder runewords are allowed.
Validate the exact runeword recipe and legal base class for the requested runeword.
Do not return old/legacy mismatched recipes.
If slot hint is provided, honor it.
Return full legal non-magic base list and base details.
Runeword slot must be one of weapon, offhand, head, armor.
Use base_class in each base detail as one of: melee_weapon, missile_weapon, shield, helm, body_armor, other.
Do not include base classes that are incompatible with the slot.`)

	userPrompt := fmt.Sprintf(
		"Verify the runeword recipe and legal base list for item query=%q exact_name=%q slot_hint=%q. Return JSON with slot, runes, possible_bases, possible_bases_details including base_class.",
		query, exactName, slotHint,
	)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"slot": map[string]any{"type": "string"},
			"runes": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"possible_bases": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"possible_bases_details": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":         map[string]any{"type": "string"},
						"base_class":   map[string]any{"type": "string"},
						"hand":         map[string]any{"type": "string"},
						"defense":      map[string]any{"type": "string"},
						"damage":       map[string]any{"type": "string"},
						"weapon_speed": map[string]any{"type": "string"},
					},
					"required":             []string{"name", "base_class", "hand", "defense", "damage", "weapon_speed"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"slot", "runes", "possible_bases", "possible_bases_details"},
		"additionalProperties": false,
	}

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
					Name:        "runeword_verification",
					Description: openai.String("Runeword verification output"),
					Strict:      openai.Bool(true),
					Schema:      schema,
				},
			},
		},
	})
	if err != nil {
		return runewordVerification{}, fmt.Errorf("verify runeword with openai: %w", err)
	}
	if len(response.Choices) == 0 {
		return runewordVerification{}, fmt.Errorf("runeword verification returned no choices")
	}

	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		return runewordVerification{}, fmt.Errorf("runeword verification returned empty content")
	}

	var verified runewordVerification
	if err := json.Unmarshal([]byte(content), &verified); err != nil {
		return runewordVerification{}, fmt.Errorf("parse runeword verification JSON: %w", err)
	}

	return verified, nil
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
			"possible_bases_details": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":         map[string]any{"type": "string"},
						"base_class":   map[string]any{"type": "string"},
						"hand":         map[string]any{"type": "string"},
						"defense":      map[string]any{"type": "string"},
						"damage":       map[string]any{"type": "string"},
						"weapon_speed": map[string]any{"type": "string"},
					},
					"required":             []string{"name", "base_class", "hand", "defense", "damage", "weapon_speed"},
					"additionalProperties": false,
				},
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
		"required":             []string{"exact_name", "slot", "kind", "swap_role", "runes", "possible_bases", "possible_bases_details", "best_in_slot_base", "best_in_slot_bases", "effects", "notes", "sources"},
		"additionalProperties": false,
	}
}

func baseDetailsSliceValue(v any) []baseDetails {
	if v == nil {
		return nil
	}

	items, ok := v.([]baseDetails)
	if ok {
		out := make([]baseDetails, len(items))
		copy(out, items)
		return out
	}

	raw, ok := v.([]any)
	if !ok {
		return nil
	}

	out := make([]baseDetails, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(stringValue(m["name"]))
		if name == "" {
			continue
		}
		out = append(out, baseDetails{
			Name:        name,
			BaseClass:   strings.TrimSpace(stringValue(m["base_class"])),
			Hand:        strings.TrimSpace(stringValue(m["hand"])),
			Defense:     strings.TrimSpace(stringValue(m["defense"])),
			Damage:      strings.TrimSpace(stringValue(m["damage"])),
			WeaponSpeed: strings.TrimSpace(stringValue(m["weapon_speed"])),
		})
	}
	return out
}

func normalizeRunewordSlot(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.ReplaceAll(v, "-", "_")
	v = strings.ReplaceAll(v, " ", "_")
	switch v {
	case "offhand", "off_hand", "shield":
		return "offhand"
	case "head", "helm", "helmet":
		return "head"
	case "armor", "body_armor", "bodyarmor", "breast", "chest":
		return "armor"
	case "weapon", "melee_weapon", "missile_weapon", "bow", "crossbow":
		return "weapon"
	default:
		return "weapon"
	}
}

func normalizeBaseClass(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.ReplaceAll(v, "-", "_")
	v = strings.ReplaceAll(v, " ", "_")
	switch v {
	case "melee_weapon", "missile_weapon", "shield", "helm", "body_armor", "other":
		return v
	case "armor", "bodyarmor":
		return "body_armor"
	default:
		return "other"
	}
}

func slotAllowsBaseClass(slot string, baseClass string) bool {
	s := normalizeRunewordSlot(slot)
	b := normalizeBaseClass(baseClass)
	if b == "other" {
		return true
	}
	switch s {
	case "weapon":
		return b == "melee_weapon" || b == "missile_weapon"
	case "offhand":
		return b == "shield"
	case "head":
		return b == "helm"
	case "armor":
		return b == "body_armor"
	default:
		return true
	}
}

func sanitizeRunewordSlotAndBases(slot string, bases []string, details []baseDetails) (string, []string, []baseDetails) {
	normalizedSlot := normalizeRunewordSlot(slot)
	if len(details) == 0 {
		return normalizedSlot, bases, details
	}

	keptDetails := make([]baseDetails, 0, len(details))
	nameSet := map[string]bool{}
	allowedWeaponClasses := map[string]bool{}
	for _, d := range details {
		if !slotAllowsBaseClass(normalizedSlot, d.BaseClass) {
			continue
		}
		d.BaseClass = normalizeBaseClass(d.BaseClass)
		keptDetails = append(keptDetails, d)
		nameSet[normalizeGearLookup(d.Name)] = true
		if d.BaseClass == "melee_weapon" || d.BaseClass == "missile_weapon" {
			allowedWeaponClasses[d.BaseClass] = true
		}
	}

	if len(keptDetails) == 0 {
		return normalizedSlot, bases, details
	}

	keptBases := make([]string, 0, len(bases))
	for _, b := range bases {
		lookup := normalizeGearLookup(b)
		if nameSet[lookup] {
			keptBases = append(keptBases, b)
			continue
		}
		if normalizedSlot == "weapon" && len(allowedWeaponClasses) == 1 {
			guessed := guessWeaponBaseClass(b)
			if guessed != "" && !allowedWeaponClasses[guessed] {
				continue
			}
		}
		keptBases = append(keptBases, b)
	}
	if len(keptBases) == 0 {
		for _, d := range keptDetails {
			keptBases = append(keptBases, d.Name)
		}
	}

	return normalizedSlot, keptBases, keptDetails
}

func guessWeaponBaseClass(baseName string) string {
	v := strings.ToLower(strings.TrimSpace(baseName))
	if v == "" {
		return ""
	}

	missileTokens := []string{" bow", "crossbow", "javelin", "spear", "amazon", "matriarchal", "ceremonial", "maiden", "grand matron", "hydra bow", "ward bow", "blade bow", "short bow", "long bow", "composite bow", "recurve bow", "stag bow", "gothic bow", "rune bow", "ashwood", "reflex", "ceremonial bow", "matron bow"}
	for _, token := range missileTokens {
		if strings.Contains(v, token) {
			return "missile_weapon"
		}
	}

	meleeTokens := []string{"sword", "axe", "mace", "club", "hammer", "maul", "flail", "staff", "orb", "wand", "dagger", "knife", "katar", "claw", "scythe", "poleaxe", "polearm", "halberd", "pike", "trident", "scepter"}
	for _, token := range meleeTokens {
		if strings.Contains(v, token) {
			return "melee_weapon"
		}
	}

	return ""
}
