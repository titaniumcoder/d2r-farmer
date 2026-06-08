package d2r

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

type addItemToolCall struct {
	Tool string          `json:"tool"`
	Args addItemToolArgs `json:"args"`
}

type addItemToolArgs struct {
	ExactName      string        `json:"exact_name"`
	Slot           string        `json:"slot"`
	PossibleSlots  []string      `json:"possible_slots"`
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
	Slot          string        `json:"slot"`
	PossibleSlots []string      `json:"possible_slots"`
	Runes         []string      `json:"runes"`
	Bases         []string      `json:"possible_bases"`
	BaseDetails   []baseDetails `json:"possible_bases_details"`
}

var resolveGearWithLLM = resolveGearDetails

func resolveGearDetails(query string, className string, slotHint string, cfg Config) (map[string]any, error) {
	_ = slotHint
	if cfg.Provider != "openai" {
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
	return resolveWithOpenAI(query, className, cfg.OpenAI)
}

func resolveWithOpenAI(query string, className string, cfg OpenAIConfig) (map[string]any, error) {
	client := newOpenAIClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	log.Printf("[llm.resolve.request] query=%q class=%q model=%q", query, className, cfg.Model)

	systemPrompt := strings.TrimSpace(`You are a Diablo II: Resurrected item resolution engine with one tool.
You MUST respond with strict JSON matching the schema for a single tool call:
tool="add_item" and args={...}.
Never output prose, markdown, or extra fields.
Use current official D2R data.

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

For runewords: possible_bases must contain legal non-magic base names OR base-type labels (examples: "Monarch", "swords", "staves", "helms").
possible_bases_details is optional context only and may be empty.
base_class must be one of: melee_weapon, missile_weapon, shield, helm, body_armor, other.
For Harmony-like runewords, do not include melee weapon bases; use missile_weapon bases only.
Runeword slot must be accurate and can be weapon, offhand, helm, or armor depending on legal base class.
possible_slots must list all legal placement slots for this item in this app: weapon, offhand, helm, armor, belt, ring, amulet, inventory.
For non-weapons use hand="n/a" and leave damage/weapon_speed empty.
For non-armor use defense empty.
best_in_slot_base is optional context only.
For weapon swap planning, infer swap_role (main|offhand) where applicable.
effects: list all notable stats and properties as concise strings, e.g. "Enhanced Damage: 340-400%", "Indestructible", "Prevent Monster Heal", "Cannot Be Frozen". Include weapon speed and damage range for weapons. Always populate this field.

Allowed enum values:
- slot/possible_slots: weapon, offhand, helm, armor, belt, ring, amulet, inventory, unknown
- kind: runeword, unique, set, crafted, rare, magic, base, unknown
- swap_role: main, offhand`)

	userPrompt := fmt.Sprintf(
		"Resolve this item for class %q: %q. Return one tool call only: add_item(args).",
		className, query,
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
	log.Printf("[llm.resolve.raw] %s", content)

	var call addItemToolCall
	if err := json.Unmarshal([]byte(content), &call); err != nil {
		return nil, fmt.Errorf("parse model JSON content: %w", err)
	}
	if strings.TrimSpace(strings.ToLower(call.Tool)) != "add_item" {
		return nil, fmt.Errorf("model returned unsupported tool %q", call.Tool)
	}
	args := call.Args
	log.Printf(
		"[llm.resolve.tool] tool=%q exact_name=%q kind=%q slot=%q possible_slots=%v runes=%v bases=%d",
		call.Tool,
		args.ExactName,
		args.Kind,
		args.Slot,
		args.PossibleSlots,
		args.Runes,
		len(args.PossibleBases),
	)
	args.BaseDetails = enforceBaseDetailStatShape(args.Slot, args.BaseDetails)

	out := map[string]any{
		"exact_name":             strings.TrimSpace(args.ExactName),
		"slot":                   strings.TrimSpace(args.Slot),
		"possible_slots":         args.PossibleSlots,
		"kind":                   strings.TrimSpace(args.Kind),
		"swap_role":              strings.TrimSpace(args.SwapRole),
		"runes":                  args.Runes,
		"possible_bases":         args.PossibleBases,
		"possible_bases_details": args.BaseDetails,
		"best_in_slot_base":      strings.TrimSpace(args.BestInSlotBase),
		"best_in_slot_bases":     args.BestInSlotList,
		"effects":                args.Effects,
		"notes":                  strings.TrimSpace(args.Notes),
		"sources":                args.Sources,
		"query":                  query,
	}

	if out["exact_name"] == "" {
		out["exact_name"] = query
	}
	if out["slot"] == "" {
		out["slot"] = "unknown"
	}
	if len(stringSliceValue(out["possible_slots"])) == 0 {
		out["possible_slots"] = []string{stringValue(out["slot"])}
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

	if strings.EqualFold(stringValue(out["kind"]), "base") {
		lookup := normalizeGearLookup(stringValue(out["exact_name"]))
		if cat, ok := d2rBaseCatalog[lookup]; ok {
			d := baseDetailFromCatalog(cat)
			out["possible_bases"] = []string{cat.Name}
			out["possible_bases_details"] = []baseDetails{d}
			switch normalizeBaseClass(cat.BaseClass) {
			case "melee_weapon", "missile_weapon":
				out["slot"] = "weapon"
			case "shield":
				out["slot"] = "offhand"
			case "helm":
				out["slot"] = "helm"
			case "body_armor":
				out["slot"] = "armor"
			}
			out["possible_slots"] = derivePossibleSlotsFromRuneword(stringValue(out["slot"]), []baseDetails{d})
		}
	}

	if strings.EqualFold(stringValue(out["kind"]), "runeword") {
		verified, verifyErr := verifyRunewordRecipeWithOpenAI(client, cfg, query, stringValue(out["exact_name"]))
		if verifyErr == nil {
			if shouldApplyRunewordVerification(verified) {
				if verified.Slot != "" {
					out["slot"] = strings.TrimSpace(verified.Slot)
				}
				if len(verified.PossibleSlots) > 0 {
					out["possible_slots"] = verified.PossibleSlots
				}
				if len(verified.Runes) > 0 {
					out["runes"] = verified.Runes
				}
				if len(verified.Bases) > 0 {
					out["possible_bases"] = verified.Bases
				}
			} else {
				log.Printf("[llm.verify.reject] query=%q exact_name=%q reason=verification_inconsistent", query, stringValue(out["exact_name"]))
			}
		} else {
			log.Printf("[llm.verify.reject] query=%q exact_name=%q reason=%v", query, stringValue(out["exact_name"]), verifyErr)
		}

		catalogBases, catalogDetails := resolveRunewordBasesFromCatalog(
			stringValue(out["slot"]),
			stringSliceValue(out["runes"]),
			stringSliceValue(out["possible_bases"]),
		)

		normalizedSlot, normalizedBases, normalizedDetails := sanitizeRunewordSlotAndBases(stringValue(out["slot"]), catalogBases, catalogDetails)
		normalizedBases, normalizedDetails = enrichRunewordBasesFromCatalog(normalizedSlot, stringSliceValue(out["runes"]), normalizedBases, normalizedDetails)
		out["slot"] = normalizedSlot
		out["possible_bases"] = normalizedBases
		out["possible_bases_details"] = normalizedDetails
		if len(stringSliceValue(out["possible_slots"])) == 0 {
			out["possible_slots"] = derivePossibleSlotsFromRuneword(normalizedSlot, normalizedDetails)
		}
	}
	out["possible_slots"] = normalizePossibleSlots(stringSliceValue(out["possible_slots"]))
	log.Printf("[llm.resolve.final] query=%q exact_name=%q kind=%q slot=%q possible_slots=%v bases=%d", query, stringValue(out["exact_name"]), stringValue(out["kind"]), stringValue(out["slot"]), stringSliceValue(out["possible_slots"]), len(stringSliceValue(out["possible_bases"])))
	return out, nil
}

func verifyRunewordRecipeWithOpenAI(client *openai.Client, cfg OpenAIConfig, query string, exactName string) (runewordVerification, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	log.Printf("[llm.verify.request] query=%q exact_name=%q model=%q", query, exactName, cfg.Model)

	systemPrompt := strings.TrimSpace(`You are an independent Diablo II: Resurrected runeword verifier.
Return strict JSON only.
Assume modern Diablo II: Resurrected current-season data where ladder runewords are available.
Validate the exact runeword recipe and legal base class for the requested runeword.
Do not return old/legacy mismatched recipes.
Never invent or guess.
If you are not fully certain, return slot="unknown", possible_slots=[], runes=[], possible_bases=[], possible_bases_details=[].
Return legal non-magic base names and/or base-type labels.
Runeword slot must be one of weapon, offhand, helm, armor.
possible_slots must list all legal placement slots for this runeword in this app.
possible_bases_details may be empty.`)

	userPrompt := fmt.Sprintf(
		"Verify the runeword recipe and legal base list for item query=%q exact_name=%q. Return JSON with slot, possible_slots, runes, possible_bases, and optionally possible_bases_details.",
		query, exactName,
	)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"slot": map[string]any{"type": "string"},
			"possible_slots": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
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
		"required":             []string{"slot", "possible_slots", "runes", "possible_bases", "possible_bases_details"},
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
	log.Printf("[llm.verify.raw] %s", content)

	var verified runewordVerification
	if err := json.Unmarshal([]byte(content), &verified); err != nil {
		return runewordVerification{}, fmt.Errorf("parse runeword verification JSON: %w", err)
	}
	verified.BaseDetails = enforceBaseDetailStatShape(verified.Slot, verified.BaseDetails)
	log.Printf("[llm.verify.final] slot=%q possible_slots=%v runes=%v bases=%d", verified.Slot, verified.PossibleSlots, verified.Runes, len(verified.Bases))

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
			"tool": map[string]any{"type": "string", "enum": []string{"add_item"}},
			"args": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"exact_name": map[string]any{"type": "string"},
					"slot": map[string]any{
						"type": "string",
						"enum": []string{"weapon", "offhand", "helm", "armor", "belt", "ring", "amulet", "inventory", "unknown"},
					},
					"possible_slots": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "string",
							"enum": []string{"weapon", "offhand", "helm", "armor", "belt", "ring", "amulet", "inventory", "unknown"},
						},
					},
					"kind": map[string]any{
						"type": "string",
						"enum": []string{"runeword", "unique", "set", "crafted", "rare", "magic", "base", "unknown"},
					},
					"swap_role": map[string]any{"type": "string", "enum": []string{"main", "offhand"}},
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
								"name": map[string]any{"type": "string"},
								"base_class": map[string]any{
									"type": "string",
									"enum": []string{"melee_weapon", "missile_weapon", "shield", "helm", "body_armor", "other"},
								},
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
				"required":             []string{"exact_name", "slot", "possible_slots", "kind", "swap_role", "runes", "possible_bases", "possible_bases_details", "best_in_slot_base", "best_in_slot_bases", "effects", "notes", "sources"},
				"additionalProperties": false,
			},
		},
		"required":             []string{"tool", "args"},
		"additionalProperties": false,
	}
}

func normalizePossibleSlots(slots []string) []string {
	if len(slots) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(slots))
	for _, slot := range slots {
		norm := normalizeRunewordSlot(slot)
		if norm == "" {
			continue
		}
		if !seen[norm] {
			seen[norm] = true
			out = append(out, norm)
		}
	}
	return out
}

func derivePossibleSlotsFromRuneword(slot string, details []baseDetails) []string {
	if len(details) == 0 {
		return []string{normalizeRunewordSlot(slot)}
	}
	possible := map[string]bool{}
	for _, d := range details {
		switch normalizeBaseClass(d.BaseClass) {
		case "melee_weapon", "missile_weapon":
			possible["weapon"] = true
		case "shield":
			possible["offhand"] = true
		case "helm":
			possible["helm"] = true
		case "body_armor":
			possible["armor"] = true
		}
	}
	out := make([]string, 0, 4)
	for _, slotName := range []string{"weapon", "offhand", "helm", "armor"} {
		if possible[slotName] {
			out = append(out, slotName)
		}
	}
	if len(out) == 0 {
		out = append(out, normalizeRunewordSlot(slot))
	}
	return out
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
	case "helm", "helmet":
		return "helm"
	case "armor", "body_armor", "bodyarmor", "breast", "chest":
		return "armor"
	case "weapon", "melee_weapon", "missile_weapon", "bow", "crossbow":
		return "weapon"
	case "unknown", "":
		return "unknown"
	default:
		return "unknown"
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

func sanitizeRunewordSlotAndBases(slot string, bases []string, details []baseDetails) (string, []string, []baseDetails) {
	normalizedSlot := normalizeRunewordSlot(slot)
	if len(details) == 0 {
		return normalizedSlot, bases, details
	}

	// Prefer slot evidence from legal base classes over model slot labels.
	derivedSlots := derivePossibleSlotsFromRuneword(normalizedSlot, details)
	if len(derivedSlots) > 0 {
		match := false
		for _, s := range derivedSlots {
			if s == normalizedSlot {
				match = true
				break
			}
		}
		if !match {
			normalizedSlot = derivedSlots[0]
		}
	}

	keptDetails := make([]baseDetails, 0, len(details))
	nameSet := map[string]bool{}
	allowedWeaponClasses := map[string]bool{}
	for _, d := range details {
		d.BaseClass = normalizeBaseClass(d.BaseClass)
		keptDetails = append(keptDetails, d)
		nameSet[normalizeGearLookup(d.Name)] = true
		if d.BaseClass == "melee_weapon" || d.BaseClass == "missile_weapon" {
			allowedWeaponClasses[d.BaseClass] = true
		}
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

	keptDetails = enforceBaseDetailStatShape(normalizedSlot, keptDetails)

	return normalizedSlot, keptBases, keptDetails
}

func enforceBaseDetailStatShape(slot string, details []baseDetails) []baseDetails {
	if len(details) == 0 {
		return details
	}

	normSlot := normalizeRunewordSlot(slot)
	out := make([]baseDetails, 0, len(details))
	for _, d := range details {
		d.Name = strings.TrimSpace(d.Name)
		d.BaseClass = normalizeBaseClass(d.BaseClass)
		d.Hand = strings.TrimSpace(d.Hand)
		d.Defense = strings.TrimSpace(d.Defense)
		d.Damage = strings.TrimSpace(d.Damage)
		d.WeaponSpeed = strings.TrimSpace(d.WeaponSpeed)

		isWeapon := d.BaseClass == "melee_weapon" || d.BaseClass == "missile_weapon" || (d.BaseClass == "other" && normSlot == "weapon")
		if isWeapon {
			d.Defense = ""
		} else {
			d.Damage = ""
			d.WeaponSpeed = ""
			if d.Hand == "" {
				d.Hand = "n/a"
			}
		}

		out = append(out, d)
	}
	return out
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

func shouldApplyRunewordVerification(verified runewordVerification) bool {
	normSlot := normalizeRunewordSlot(verified.Slot)
	if normSlot == "unknown" || strings.TrimSpace(verified.Slot) == "" {
		return false
	}
	if len(verified.Runes) == 0 {
		return false
	}
	if len(verified.Bases) == 0 && len(verified.BaseDetails) == 0 {
		return false
	}

	if len(verified.Bases) > 0 && len(verified.BaseDetails) > 0 {
		matched := 0
		nameSet := map[string]bool{}
		for _, d := range verified.BaseDetails {
			nameSet[normalizeGearLookup(d.Name)] = true
		}
		for _, b := range verified.Bases {
			if nameSet[normalizeGearLookup(b)] {
				matched++
			}
		}
		if matched == 0 {
			return false
		}
	}

	if len(verified.BaseDetails) > 0 {
		allowed := derivePossibleSlotsFromRuneword(normSlot, verified.BaseDetails)
		if len(allowed) == 0 {
			return false
		}
	}

	return true
}

type socketedBase struct {
	Name       string
	BaseClass  string
	MaxSockets int
}

var helmSocketCatalog = []socketedBase{
	{Name: "Cap", BaseClass: "helm", MaxSockets: 2},
	{Name: "Skull Cap", BaseClass: "helm", MaxSockets: 2},
	{Name: "Helm", BaseClass: "helm", MaxSockets: 2},
	{Name: "Full Helm", BaseClass: "helm", MaxSockets: 2},
	{Name: "Great Helm", BaseClass: "helm", MaxSockets: 3},
	{Name: "Crown", BaseClass: "helm", MaxSockets: 3},
	{Name: "Mask", BaseClass: "helm", MaxSockets: 3},
	{Name: "Bone Helm", BaseClass: "helm", MaxSockets: 2},
	{Name: "War Hat", BaseClass: "helm", MaxSockets: 2},
	{Name: "Sallet", BaseClass: "helm", MaxSockets: 2},
	{Name: "Casque", BaseClass: "helm", MaxSockets: 2},
	{Name: "Basinet", BaseClass: "helm", MaxSockets: 2},
	{Name: "Winged Helm", BaseClass: "helm", MaxSockets: 2},
	{Name: "Grand Crown", BaseClass: "helm", MaxSockets: 3},
	{Name: "Death Mask", BaseClass: "helm", MaxSockets: 3},
	{Name: "Grim Helm", BaseClass: "helm", MaxSockets: 2},
	{Name: "Bone Visage", BaseClass: "helm", MaxSockets: 2},
	{Name: "Shako", BaseClass: "helm", MaxSockets: 2},
	{Name: "Hydraskull", BaseClass: "helm", MaxSockets: 2},
	{Name: "Armet", BaseClass: "helm", MaxSockets: 2},
	{Name: "Giant Conch", BaseClass: "helm", MaxSockets: 2},
	{Name: "Spired Helm", BaseClass: "helm", MaxSockets: 2},
	{Name: "Corona", BaseClass: "helm", MaxSockets: 3},
	{Name: "Demonhead", BaseClass: "helm", MaxSockets: 3},
	{Name: "Wolf Head", BaseClass: "helm", MaxSockets: 2},
	{Name: "Hawk Helm", BaseClass: "helm", MaxSockets: 2},
	{Name: "Antlers", BaseClass: "helm", MaxSockets: 2},
	{Name: "Falcon Mask", BaseClass: "helm", MaxSockets: 2},
	{Name: "Spirit Mask", BaseClass: "helm", MaxSockets: 2},
	{Name: "Alpha Helm", BaseClass: "helm", MaxSockets: 2},
	{Name: "Griffon Headdress", BaseClass: "helm", MaxSockets: 2},
	{Name: "Hunter's Guise", BaseClass: "helm", MaxSockets: 2},
	{Name: "Sacred Feathers", BaseClass: "helm", MaxSockets: 2},
	{Name: "Totemic Mask", BaseClass: "helm", MaxSockets: 2},
	{Name: "Blood Spirit", BaseClass: "helm", MaxSockets: 3},
	{Name: "Sun Spirit", BaseClass: "helm", MaxSockets: 3},
	{Name: "Earth Spirit", BaseClass: "helm", MaxSockets: 3},
	{Name: "Sky Spirit", BaseClass: "helm", MaxSockets: 3},
	{Name: "Dream Spirit", BaseClass: "helm", MaxSockets: 3},
}

var staffSocketCatalog = []socketedBase{
	{Name: "Short Staff", BaseClass: "melee_weapon", MaxSockets: 6},
	{Name: "Long Staff", BaseClass: "melee_weapon", MaxSockets: 6},
	{Name: "Gnarled Staff", BaseClass: "melee_weapon", MaxSockets: 6},
	{Name: "Battle Staff", BaseClass: "melee_weapon", MaxSockets: 6},
	{Name: "War Staff", BaseClass: "melee_weapon", MaxSockets: 6},
	{Name: "Jo Staff", BaseClass: "melee_weapon", MaxSockets: 6},
	{Name: "Quarterstaff", BaseClass: "melee_weapon", MaxSockets: 6},
	{Name: "Cedar Staff", BaseClass: "melee_weapon", MaxSockets: 6},
	{Name: "Gothic Staff", BaseClass: "melee_weapon", MaxSockets: 6},
	{Name: "Rune Staff", BaseClass: "melee_weapon", MaxSockets: 6},
	{Name: "Walking Stick", BaseClass: "melee_weapon", MaxSockets: 6},
	{Name: "Stalagmite", BaseClass: "melee_weapon", MaxSockets: 6},
	{Name: "Elder Staff", BaseClass: "melee_weapon", MaxSockets: 6},
	{Name: "Shillelagh", BaseClass: "melee_weapon", MaxSockets: 6},
	{Name: "Archon Staff", BaseClass: "melee_weapon", MaxSockets: 6},
}

func expandGenericRunewordBases(client *openai.Client, cfg OpenAIConfig, exactName string, slot string, runes []string, bases []string, details []baseDetails) ([]string, []baseDetails) {
	requiredSockets := len(runes)
	if requiredSockets <= 0 {
		return bases, details
	}

	normSlot := normalizeRunewordSlot(slot)
	if normSlot != "helm" && normSlot != "weapon" {
		return bases, details
	}

	categories := detectGenericBaseCategories(bases, details)
	if len(categories) == 0 {
		return bases, details
	}

	if client != nil {
		llmBases, llmDetails, err := expandGenericBasesWithOpenAI(client, cfg, exactName, normSlot, categories, requiredSockets)
		if err == nil && len(llmBases) > 0 {
			log.Printf("[llm.expand.final] exact_name=%q slot=%q categories=%v required_sockets=%d bases=%d", exactName, normSlot, categories, requiredSockets, len(llmBases))
			return llmBases, llmDetails
		}
		if err != nil {
			log.Printf("[llm.expand.reject] exact_name=%q slot=%q reason=%v", exactName, normSlot, err)
		}
	}

	catalog := []socketedBase{}
	switch normSlot {
	case "helm":
		if !(categories["helm"] || categories["pelt"]) {
			return bases, details
		}
		catalog = helmSocketCatalog
	case "weapon":
		if !(categories["staff"] || categories["staves"]) {
			return bases, details
		}
		catalog = staffSocketCatalog
	default:
		return bases, details
	}

	expandedBases := make([]string, 0)
	expandedDetails := make([]baseDetails, 0)
	for _, candidate := range catalog {
		if candidate.MaxSockets < requiredSockets {
			continue
		}
		expandedBases = append(expandedBases, candidate.Name)
		hand := "n/a"
		if candidate.BaseClass == "melee_weapon" || candidate.BaseClass == "missile_weapon" {
			hand = "2h"
		}
		expandedDetails = append(expandedDetails, baseDetails{
			Name:      candidate.Name,
			BaseClass: candidate.BaseClass,
			Hand:      hand,
		})
	}

	if len(expandedBases) == 0 {
		return bases, details
	}
	return expandedBases, expandedDetails
}

func detectGenericBaseCategories(bases []string, details []baseDetails) map[string]bool {
	out := map[string]bool{}
	mark := func(raw string) {
		v := strings.ToLower(strings.TrimSpace(raw))
		switch v {
		case "helm", "helms", "helmet", "helmets":
			out["helm"] = true
		case "pelt", "pelts":
			out["pelt"] = true
		case "staff", "staves":
			out["staff"] = true
			out["staves"] = true
		}
	}
	for _, b := range bases {
		mark(b)
	}
	for _, d := range details {
		mark(d.Name)
	}
	return out
}

func expandGenericBasesWithOpenAI(client *openai.Client, cfg OpenAIConfig, exactName string, slot string, categories map[string]bool, requiredSockets int) ([]string, []baseDetails, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	categoryList := make([]string, 0, len(categories))
	for c := range categories {
		categoryList = append(categoryList, c)
	}
	sort.Strings(categoryList)

	log.Printf("[llm.expand.request] exact_name=%q slot=%q categories=%v required_sockets=%d model=%q", exactName, slot, categoryList, requiredSockets, cfg.Model)

	systemPrompt := strings.TrimSpace(`You expand Diablo II: Resurrected generic base categories into concrete non-magical base names.
Return strict JSON only.
Never return category labels like "Helms", "Pelts", "Staves".
Return concrete base item names only.
Only include bases that can naturally roll at least the required socket count.
	Do not include any base with max sockets below required_sockets (example: wand max 2, so never include wand for required_sockets=6).
For slot=helm, use base_class=helm and hand="n/a".
For slot=weapon and staff-like bases, use base_class=melee_weapon and hand="2h".
For each concrete weapon base, provide exact D2R base stats for that exact variant:
- damage must be exact base damage range (e.g. "83-99"), never placeholders like "varies" or "n/a".
- weapon_speed must be exact base speed value (e.g. "0"), never placeholders.
If exact stats are unknown for a base, exclude that base from the result.
Do not include magic/rare/unique/set items.`)

	userPrompt := fmt.Sprintf("Expand generic base categories for runeword=%q slot=%q categories=%v required_sockets=%d.", exactName, slot, categoryList, requiredSockets)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
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
						"base_class":   map[string]any{"type": "string", "enum": []string{"melee_weapon", "missile_weapon", "shield", "helm", "body_armor", "other"}},
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
		"required":             []string{"possible_bases", "possible_bases_details"},
		"additionalProperties": false,
	}

	response, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: shared.ChatModel(cfg.Model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			{OfSystem: &openai.ChatCompletionSystemMessageParam{Content: openai.ChatCompletionSystemMessageParamContentUnion{OfString: openai.String(systemPrompt)}}},
			{OfUser: &openai.ChatCompletionUserMessageParam{Content: openai.ChatCompletionUserMessageParamContentUnion{OfString: openai.String(userPrompt)}}},
		},
		Temperature: openai.Float(0),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
			Type: "json_schema",
			JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:        "runeword_base_expansion",
				Description: openai.String("Expanded concrete runeword base list"),
				Strict:      openai.Bool(true),
				Schema:      schema,
			},
		}},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("expand generic bases with openai: %w", err)
	}
	if len(response.Choices) == 0 {
		return nil, nil, fmt.Errorf("expand generic bases returned no choices")
	}

	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		return nil, nil, fmt.Errorf("expand generic bases returned empty content")
	}
	log.Printf("[llm.expand.raw] %s", content)

	var parsed struct {
		PossibleBases       []string      `json:"possible_bases"`
		PossibleBaseDetails []baseDetails `json:"possible_bases_details"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, nil, fmt.Errorf("parse generic base expansion JSON: %w", err)
	}

	bases := make([]string, 0, len(parsed.PossibleBases))
	for _, b := range parsed.PossibleBases {
		name := strings.TrimSpace(b)
		if name == "" {
			continue
		}
		low := strings.ToLower(name)
		if low == "helms" || low == "helm" || low == "pelts" || low == "pelt" || low == "staff" || low == "staves" {
			continue
		}
		bases = append(bases, name)
	}

	details := enforceBaseDetailStatShape(slot, parsed.PossibleBaseDetails)
	if normalizeRunewordSlot(slot) == "weapon" {
		details = filterConcreteWeaponStatDetails(details)
	}
	bases, details = filterBySocketRequirement(requiredSockets, bases, details)
	if len(bases) == 0 && len(details) > 0 {
		for _, d := range details {
			if strings.TrimSpace(d.Name) != "" {
				bases = append(bases, strings.TrimSpace(d.Name))
			}
		}
		bases, details = filterBySocketRequirement(requiredSockets, bases, details)
	}

	if normalizeRunewordSlot(slot) == "weapon" {
		enriched, enrichErr := enrichWeaponBaseStatsWithOpenAI(client, cfg, bases, details)
		if enrichErr != nil {
			log.Printf("[llm.expand.enrich.reject] exact_name=%q reason=%v", exactName, enrichErr)
		} else if len(enriched) > 0 {
			details = enriched
			bases, details = filterBySocketRequirement(requiredSockets, bases, details)
		}
	} else {
		enriched, enrichErr := enrichDefensiveBaseStatsWithOpenAI(client, cfg, bases, details)
		if enrichErr != nil {
			log.Printf("[llm.expand.enrich.reject] exact_name=%q reason=%v", exactName, enrichErr)
		} else if len(enriched) > 0 {
			details = enriched
			bases, details = filterBySocketRequirement(requiredSockets, bases, details)
		}
	}

	return bases, details, nil
}

func enrichWeaponBaseStatsWithOpenAI(client *openai.Client, cfg OpenAIConfig, bases []string, details []baseDetails) ([]baseDetails, error) {
	if len(bases) == 0 {
		return details, nil
	}

	byName := map[string]baseDetails{}
	for _, d := range details {
		key := normalizeGearLookup(d.Name)
		if key == "" {
			continue
		}
		byName[key] = d
	}

	missing := make([]string, 0)
	for _, base := range bases {
		key := normalizeGearLookup(base)
		d, ok := byName[key]
		if !ok || strings.TrimSpace(d.Damage) == "" || strings.TrimSpace(d.WeaponSpeed) == "" {
			missing = append(missing, base)
		}
	}
	if len(missing) == 0 {
		return details, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	log.Printf("[llm.expand.enrich.request] bases=%v model=%q", missing, cfg.Model)

	systemPrompt := strings.TrimSpace(`You provide exact Diablo II: Resurrected non-magical weapon base stats.
Return strict JSON only.
For each requested base name, return exact base damage range and exact base speed.
Use weapon_speed as the exact numeric base speed string (e.g. "0", "-10").
If uncertain for any base, omit that base from the response.
Do not invent non-requested bases.`)
	userPrompt := fmt.Sprintf("Return exact stats for these weapon bases: %v", missing)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bases": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":         map[string]any{"type": "string"},
						"damage":       map[string]any{"type": "string"},
						"weapon_speed": map[string]any{"type": "string"},
						"hand":         map[string]any{"type": "string"},
					},
					"required":             []string{"name", "damage", "weapon_speed", "hand"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"bases"},
		"additionalProperties": false,
	}

	response, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: shared.ChatModel(cfg.Model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			{OfSystem: &openai.ChatCompletionSystemMessageParam{Content: openai.ChatCompletionSystemMessageParamContentUnion{OfString: openai.String(systemPrompt)}}},
			{OfUser: &openai.ChatCompletionUserMessageParam{Content: openai.ChatCompletionUserMessageParamContentUnion{OfString: openai.String(userPrompt)}}},
		},
		Temperature: openai.Float(0),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
			Type: "json_schema",
			JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:        "weapon_base_stats",
				Description: openai.String("Exact weapon base stats"),
				Strict:      openai.Bool(true),
				Schema:      schema,
			},
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("enrich weapon base stats with openai: %w", err)
	}
	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("weapon stat enrichment returned no choices")
	}

	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		return nil, fmt.Errorf("weapon stat enrichment returned empty content")
	}
	log.Printf("[llm.expand.enrich.raw] %s", content)

	var parsed struct {
		Bases []struct {
			Name        string `json:"name"`
			Damage      string `json:"damage"`
			WeaponSpeed string `json:"weapon_speed"`
			Hand        string `json:"hand"`
		} `json:"bases"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, fmt.Errorf("parse weapon stat enrichment JSON: %w", err)
	}

	for _, b := range parsed.Bases {
		name := strings.TrimSpace(b.Name)
		if name == "" {
			continue
		}
		key := normalizeGearLookup(name)
		d := byName[key]
		d.Name = name
		d.BaseClass = "melee_weapon"
		d.Hand = strings.TrimSpace(b.Hand)
		d.Damage = strings.TrimSpace(b.Damage)
		d.WeaponSpeed = strings.TrimSpace(b.WeaponSpeed)
		byName[key] = d
	}

	out := make([]baseDetails, 0, len(bases))
	for _, base := range bases {
		key := normalizeGearLookup(base)
		if d, ok := byName[key]; ok {
			out = append(out, d)
		}
	}
	out = enforceBaseDetailStatShape("weapon", filterConcreteWeaponStatDetails(out))
	if len(out) == 0 {
		return details, nil
	}
	return out, nil
}

func enrichDefensiveBaseStatsWithOpenAI(client *openai.Client, cfg OpenAIConfig, bases []string, details []baseDetails) ([]baseDetails, error) {
	if len(bases) == 0 {
		return details, nil
	}

	byName := map[string]baseDetails{}
	for _, d := range details {
		key := normalizeGearLookup(d.Name)
		if key == "" {
			continue
		}
		byName[key] = d
	}

	missing := make([]string, 0)
	for _, base := range bases {
		key := normalizeGearLookup(base)
		d, ok := byName[key]
		if !ok || strings.TrimSpace(d.Defense) == "" {
			missing = append(missing, base)
		}
	}
	if len(missing) == 0 {
		return details, nil
	}

	log.Printf("[llm.expand.enrich.request] defensive_bases=%v model=%q", missing, cfg.Model)
	parsed, err := requestDefensiveBaseStats(client, cfg, missing, false)
	if err != nil {
		return nil, fmt.Errorf("enrich defensive base stats with openai: %w", err)
	}
	if hasSuspiciousUniformDefense(parsed) {
		log.Printf("[llm.expand.enrich.retry] reason=uniform_defense_ranges")
		retryParsed, retryErr := requestDefensiveBaseStats(client, cfg, missing, true)
		if retryErr == nil && len(retryParsed.Bases) > 0 {
			parsed = retryParsed
		}
	}

	for _, b := range parsed.Bases {
		name := strings.TrimSpace(b.Name)
		if name == "" {
			continue
		}
		if b.DefenseMin <= 0 || b.DefenseMax <= 0 || b.DefenseMax < b.DefenseMin {
			continue
		}
		key := normalizeGearLookup(name)
		d := byName[key]
		d.Name = name
		if d.BaseClass == "" || d.BaseClass == "other" {
			d.BaseClass = "helm"
		}
		d.Hand = strings.TrimSpace(b.Hand)
		d.Defense = fmt.Sprintf("%d-%d", b.DefenseMin, b.DefenseMax)
		byName[key] = d
	}

	out := make([]baseDetails, 0, len(bases))
	for _, base := range bases {
		key := normalizeGearLookup(base)
		if d, ok := byName[key]; ok {
			out = append(out, d)
		}
	}
	out = enforceBaseDetailStatShape("helm", filterConcreteDefensiveStatDetails(out))
	if len(out) == 0 {
		return details, nil
	}
	return out, nil
}

type defensiveStatsResponse struct {
	Bases []struct {
		Name       string `json:"name"`
		DefenseMin int    `json:"defense_min"`
		DefenseMax int    `json:"defense_max"`
		Hand       string `json:"hand"`
	} `json:"bases"`
}

func requestDefensiveBaseStats(client *openai.Client, cfg OpenAIConfig, missing []string, strict bool) (defensiveStatsResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	systemPrompt := strings.TrimSpace(`You provide exact Diablo II: Resurrected non-magical defensive base stats.
Return strict JSON only.
For each requested base name, return exact base defense MIN and MAX integers.
Do not return placeholders or estimates.
If uncertain for any base, omit that base from the response.
Do not invent non-requested bases.`)
	if strict {
		systemPrompt += "\nEvery base must be computed independently. Do not reuse one range for many distinct bases unless that is truly exact in D2R data."
	}
	userPrompt := fmt.Sprintf("Return exact defense min/max for these non-magical defensive bases: %v", missing)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bases": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":        map[string]any{"type": "string"},
						"defense_min": map[string]any{"type": "integer"},
						"defense_max": map[string]any{"type": "integer"},
						"hand":        map[string]any{"type": "string"},
					},
					"required":             []string{"name", "defense_min", "defense_max", "hand"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"bases"},
		"additionalProperties": false,
	}

	response, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: shared.ChatModel(cfg.Model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			{OfSystem: &openai.ChatCompletionSystemMessageParam{Content: openai.ChatCompletionSystemMessageParamContentUnion{OfString: openai.String(systemPrompt)}}},
			{OfUser: &openai.ChatCompletionUserMessageParam{Content: openai.ChatCompletionUserMessageParamContentUnion{OfString: openai.String(userPrompt)}}},
		},
		Temperature: openai.Float(0),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
			Type: "json_schema",
			JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:        "defensive_base_stats",
				Description: openai.String("Exact defensive base stats"),
				Strict:      openai.Bool(true),
				Schema:      schema,
			},
		}},
	})
	if err != nil {
		return defensiveStatsResponse{}, err
	}
	if len(response.Choices) == 0 {
		return defensiveStatsResponse{}, fmt.Errorf("defensive stat enrichment returned no choices")
	}

	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		return defensiveStatsResponse{}, fmt.Errorf("defensive stat enrichment returned empty content")
	}
	log.Printf("[llm.expand.enrich.raw] %s", content)

	var parsed defensiveStatsResponse
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return defensiveStatsResponse{}, fmt.Errorf("parse defensive stat enrichment JSON: %w", err)
	}

	return parsed, nil
}

func hasSuspiciousUniformDefense(parsed defensiveStatsResponse) bool {
	if len(parsed.Bases) < 5 {
		return false
	}
	counts := map[string]int{}
	for _, b := range parsed.Bases {
		if b.DefenseMin <= 0 || b.DefenseMax <= 0 || b.DefenseMax < b.DefenseMin {
			continue
		}
		k := fmt.Sprintf("%d-%d", b.DefenseMin, b.DefenseMax)
		counts[k]++
	}
	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}
	return maxCount >= (len(parsed.Bases)*2)/3
}

func filterConcreteWeaponStatDetails(details []baseDetails) []baseDetails {
	if len(details) == 0 {
		return details
	}

	out := make([]baseDetails, 0, len(details))
	for _, d := range details {
		if d.BaseClass != "melee_weapon" && d.BaseClass != "missile_weapon" {
			out = append(out, d)
			continue
		}

		dmg := strings.ToLower(strings.TrimSpace(d.Damage))
		spd := strings.ToLower(strings.TrimSpace(d.WeaponSpeed))
		if dmg == "" || spd == "" {
			continue
		}
		if dmg == "varies" || dmg == "n/a" || strings.Contains(dmg, "unknown") {
			continue
		}
		if spd == "varies" || spd == "n/a" || strings.Contains(spd, "unknown") {
			continue
		}
		out = append(out, d)
	}
	return out
}

func filterConcreteDefensiveStatDetails(details []baseDetails) []baseDetails {
	if len(details) == 0 {
		return details
	}

	out := make([]baseDetails, 0, len(details))
	for _, d := range details {
		if d.BaseClass == "melee_weapon" || d.BaseClass == "missile_weapon" {
			out = append(out, d)
			continue
		}

		def := strings.ToLower(strings.TrimSpace(d.Defense))
		if def == "" {
			continue
		}
		if def == "varies" || def == "n/a" || strings.Contains(def, "unknown") {
			continue
		}
		out = append(out, d)
	}
	return out
}

func filterBySocketRequirement(requiredSockets int, bases []string, details []baseDetails) ([]string, []baseDetails) {
	if requiredSockets <= 0 {
		return bases, details
	}

	allowedByName := map[string]bool{}
	if len(details) > 0 {
		filteredDetails := make([]baseDetails, 0, len(details))
		for _, d := range details {
			name := strings.TrimSpace(d.Name)
			if name == "" {
				continue
			}
			maxSockets := maxSocketsForBaseName(name)
			if maxSockets > 0 && maxSockets < requiredSockets {
				continue
			}
			allowedByName[normalizeGearLookup(name)] = true
			filteredDetails = append(filteredDetails, d)
		}
		details = filteredDetails
	}

	if len(bases) > 0 {
		filteredBases := make([]string, 0, len(bases))
		for _, b := range bases {
			name := strings.TrimSpace(b)
			if name == "" {
				continue
			}
			if len(allowedByName) > 0 {
				if !allowedByName[normalizeGearLookup(name)] {
					continue
				}
				filteredBases = append(filteredBases, name)
				continue
			}
			maxSockets := maxSocketsForBaseName(name)
			if maxSockets > 0 && maxSockets < requiredSockets {
				continue
			}
			filteredBases = append(filteredBases, name)
		}
		bases = filteredBases
	}

	return bases, details
}

func maxSocketsForBaseName(name string) int {
	v := normalizeGearLookup(stripEtherealPrefix(name))
	if v == "" {
		return 0
	}

	if item, ok := d2rBaseCatalog[v]; ok {
		return item.MaxSockets
	}

	known := map[string]int{
		"wand": 2, "yew wand": 2, "bone wand": 2, "grim wand": 2,
		"burnt wand": 2, "petrified wand": 2, "tomb wand": 2,
		"grave wand": 2, "polished wand": 2, "ghost wand": 2, "lich wand": 2,
		"unearthed wand": 2,
	}
	if max, ok := known[v]; ok {
		return max
	}

	if strings.Contains(v, "staff") || strings.Contains(v, "staves") {
		return 6
	}
	if strings.Contains(v, "wand") {
		return 2
	}
	return 0
}

func resolveRunewordBasesFromCatalog(slot string, runes []string, hints []string) ([]string, []baseDetails) {
	normSlot := normalizeRunewordSlot(slot)
	requiredSockets := len(runes)
	if len(hints) == 0 {
		return nil, nil
	}

	selected := map[string]catalogBase{}
	for _, hint := range hints {
		collectCatalogBasesForHint(normSlot, requiredSockets, hint, selected)
	}

	if len(selected) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(selected))
	for k := range selected {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	bases := make([]string, 0, len(keys))
	details := make([]baseDetails, 0, len(keys))
	for _, k := range keys {
		cat := selected[k]
		bases = append(bases, cat.Name)
		details = append(details, baseDetailFromCatalog(cat))
	}

	return bases, details
}

func collectCatalogBasesForHint(slot string, requiredSockets int, hint string, out map[string]catalogBase) {
	n := normalizeGearLookup(hint)
	if n == "" {
		return
	}

	if cat, ok := d2rBaseCatalog[n]; ok {
		if catalogBaseFitsRunewordSlot(slot, cat) && (requiredSockets <= 0 || cat.MaxSockets == 0 || cat.MaxSockets >= requiredSockets) {
			out[n] = cat
		}
		return
	}

	token := singularBaseToken(n)
	addIfMatch := func(key string, cat catalogBase) {
		if !catalogBaseFitsRunewordSlot(slot, cat) {
			return
		}
		if requiredSockets > 0 && cat.MaxSockets > 0 && cat.MaxSockets < requiredSockets {
			return
		}
		out[key] = cat
	}

	for key, cat := range d2rBaseCatalog {
		name := normalizeGearLookup(cat.Name)
		typeCode := strings.ToLower(strings.TrimSpace(cat.TypeCode))
		switch token {
		case "helm", "helmet", "pelt":
			if normalizeBaseClass(cat.BaseClass) == "helm" || typeCode == "helm" || typeCode == "pelt" || typeCode == "phlm" || typeCode == "circ" {
				addIfMatch(key, cat)
			}
		case "sword", "one handed sword", "one-handed sword", "two handed sword", "two-handed sword", "1h sword", "2h sword":
			if typeCode == "swor" {
				addIfMatch(key, cat)
			}
		case "armor", "body_armor", "bodyarmor", "chest":
			if normalizeBaseClass(cat.BaseClass) == "body_armor" || typeCode == "tors" {
				addIfMatch(key, cat)
			}
		case "shield":
			if normalizeBaseClass(cat.BaseClass) == "shield" || typeCode == "shie" || typeCode == "ashd" || typeCode == "head" {
				addIfMatch(key, cat)
			}
		case "staf", "staff", "staves":
			if typeCode == "staf" {
				addIfMatch(key, cat)
			}
		case "wand", "wands":
			if typeCode == "wand" {
				addIfMatch(key, cat)
			}
		case "axe", "axes":
			if typeCode == "axe" {
				addIfMatch(key, cat)
			}
		case "mace", "maces":
			if typeCode == "mace" || typeCode == "hamm" || typeCode == "club" {
				addIfMatch(key, cat)
			}
		case "scepter", "scepters":
			if typeCode == "scep" {
				addIfMatch(key, cat)
			}
		case "polearm", "polearms":
			if typeCode == "pole" {
				addIfMatch(key, cat)
			}
		case "spear", "spears":
			if typeCode == "spea" || typeCode == "aspe" {
				addIfMatch(key, cat)
			}
		case "bow", "bows":
			if typeCode == "bow" || typeCode == "abow" {
				addIfMatch(key, cat)
			}
		case "crossbow", "crossbows":
			if typeCode == "xbow" {
				addIfMatch(key, cat)
			}
		case "orb", "orbs":
			if typeCode == "orb" {
				addIfMatch(key, cat)
			}
		case "claw", "claws":
			if typeCode == "h2h" || typeCode == "h2h2" {
				addIfMatch(key, cat)
			}
		case "weapon":
			if normalizeBaseClass(cat.BaseClass) == "melee_weapon" || normalizeBaseClass(cat.BaseClass) == "missile_weapon" {
				addIfMatch(key, cat)
			}
		default:
			if strings.Contains(name, token) || strings.Contains(name, n) {
				addIfMatch(key, cat)
			}
		}
	}
}

func singularBaseToken(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasSuffix(v, "ies") && len(v) > 3 {
		return strings.TrimSuffix(v, "ies") + "y"
	}
	if strings.HasSuffix(v, "ves") && len(v) > 3 {
		return strings.TrimSuffix(v, "ves") + "f"
	}
	if strings.HasSuffix(v, "s") && len(v) > 1 {
		return strings.TrimSuffix(v, "s")
	}
	return v
}

func enrichRunewordBasesFromCatalog(slot string, runes []string, bases []string, details []baseDetails) ([]string, []baseDetails) {
	normSlot := normalizeRunewordSlot(slot)
	if normSlot != "weapon" && normSlot != "offhand" && normSlot != "helm" && normSlot != "armor" {
		return bases, details
	}

	byName := map[string]baseDetails{}
	ordered := make([]string, 0, len(bases)+len(details))
	push := func(name string) {
		k := normalizeGearLookup(name)
		if k == "" {
			return
		}
		if _, ok := byName[k]; ok {
			return
		}
		ordered = append(ordered, k)
		byName[k] = baseDetails{Name: strings.TrimSpace(name)}
	}

	for _, b := range bases {
		push(b)
	}
	for _, d := range details {
		push(d.Name)
		k := normalizeGearLookup(d.Name)
		if k == "" {
			continue
		}
		byName[k] = d
	}

	requiredSockets := len(runes)
	resolved := make([]baseDetails, 0, len(ordered))
	seen := map[string]bool{}
	for _, key := range ordered {
		d := byName[key]
		name := strings.TrimSpace(d.Name)
		if name == "" {
			name = key
		}

		if cat, ok := d2rBaseCatalog[normalizeGearLookup(stripEtherealPrefix(name))]; ok {
			if !catalogBaseFitsRunewordSlot(normSlot, cat) {
				continue
			}
			if requiredSockets > 0 && cat.MaxSockets > 0 && cat.MaxSockets < requiredSockets {
				continue
			}
			d = baseDetailFromCatalog(cat)
			d.Name = name
		} else {
			if !baseDetailFitsRunewordSlot(normSlot, d) {
				continue
			}
		}

		lookup := normalizeGearLookup(d.Name)
		if lookup == "" || seen[lookup] {
			continue
		}
		seen[lookup] = true
		resolved = append(resolved, d)
	}

	resolved = enforceBaseDetailStatShape(normSlot, resolved)
	basesOut, resolved := filterBySocketRequirement(requiredSockets, baseNamesFromDetails(resolved), resolved)

	return basesOut, resolved
}

func baseNamesFromDetails(details []baseDetails) []string {
	out := make([]string, 0, len(details))
	seen := map[string]bool{}
	for _, d := range details {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			continue
		}
		key := normalizeGearLookup(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}

func baseDetailFromCatalog(cat catalogBase) baseDetails {
	hand := strings.TrimSpace(cat.Hand)
	if hand == "" {
		hand = "n/a"
	}
	return baseDetails{
		Name:        strings.TrimSpace(cat.Name),
		BaseClass:   normalizeBaseClass(cat.BaseClass),
		Hand:        hand,
		Defense:     strings.TrimSpace(cat.Defense),
		Damage:      strings.TrimSpace(cat.Damage),
		WeaponSpeed: strings.TrimSpace(cat.WeaponSpeed),
	}
}

func catalogBaseFitsRunewordSlot(slot string, cat catalogBase) bool {
	baseClass := normalizeBaseClass(cat.BaseClass)
	switch slot {
	case "weapon":
		return baseClass == "melee_weapon" || baseClass == "missile_weapon"
	case "offhand":
		return baseClass == "shield"
	case "helm":
		return baseClass == "helm"
	case "armor":
		return baseClass == "body_armor"
	default:
		return false
	}
}

func baseDetailFitsRunewordSlot(slot string, d baseDetails) bool {
	baseClass := normalizeBaseClass(d.BaseClass)
	switch slot {
	case "weapon":
		return baseClass == "melee_weapon" || baseClass == "missile_weapon"
	case "offhand":
		return baseClass == "shield"
	case "helm":
		return baseClass == "helm"
	case "armor":
		return baseClass == "body_armor"
	default:
		return true
	}
}

func hasRune(runes []string, runeName string) bool {
	needle := normalizeGearLookup(runeName)
	for _, r := range runes {
		if normalizeGearLookup(r) == needle {
			return true
		}
	}
	return false
}

func appendEtherealVariants(bases []string, details []baseDetails) ([]string, []baseDetails) {
	ethDetails := make([]baseDetails, 0, len(details)+len(bases))
	ethSeen := map[string]bool{}
	appendEth := func(source baseDetails) {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(source.Name)), "ethereal ") {
			return
		}
		if !canBeEthereal(source) {
			return
		}
		ed := source
		ed.Name = "Ethereal " + strings.TrimSpace(source.Name)
		ed.Damage = scaleRangeByHalf(ed.Damage)
		ed.Defense = scaleRangeByHalf(ed.Defense)
		key := normalizeGearLookup(ed.Name)
		if key == "" || ethSeen[key] {
			return
		}
		ethSeen[key] = true
		ethDetails = append(ethDetails, ed)
	}

	for _, d := range details {
		appendEth(d)
	}

	for _, name := range bases {
		trimmed := strings.TrimSpace(name)
		lower := strings.ToLower(trimmed)
		if trimmed == "" || strings.HasPrefix(lower, "ethereal ") || strings.HasPrefix(lower, "eth ") {
			continue
		}
		lookup := normalizeGearLookup(stripEtherealPrefix(trimmed))
		cat, ok := d2rBaseCatalog[lookup]
		if !ok {
			continue
		}
		d := baseDetailFromCatalog(cat)
		d.Name = trimmed
		appendEth(d)
	}

	if len(ethDetails) == 0 {
		return bases, details
	}

	detailsOut := make([]baseDetails, 0, len(details)+len(ethDetails))
	detailsOut = append(detailsOut, details...)
	detailsOut = append(detailsOut, ethDetails...)

	basesOut := make([]string, 0, len(bases)+len(ethDetails))
	seen := map[string]bool{}
	for _, b := range bases {
		name := strings.TrimSpace(b)
		if name == "" {
			continue
		}
		key := normalizeGearLookup(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		basesOut = append(basesOut, name)
	}
	for _, d := range ethDetails {
		key := normalizeGearLookup(d.Name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		basesOut = append(basesOut, d.Name)
	}

	return basesOut, detailsOut
}

func canBeEthereal(d baseDetails) bool {
	baseClass := normalizeBaseClass(d.BaseClass)
	if baseClass == "melee_weapon" || baseClass == "missile_weapon" || baseClass == "shield" || baseClass == "helm" || baseClass == "body_armor" {
		lookup := normalizeGearLookup(d.Name)
		if lookup == "phase blade" {
			return false
		}
		return true
	}
	return false
}

func scaleRangeByHalf(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	parts := strings.Split(v, "-")
	if len(parts) != 2 {
		return raw
	}
	minV, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return raw
	}
	maxV, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return raw
	}
	return fmt.Sprintf("%d-%d", (minV*3)/2, (maxV*3)/2)
}

func stripEtherealPrefix(name string) string {
	v := strings.TrimSpace(name)
	lower := strings.ToLower(v)
	if strings.HasPrefix(lower, "ethereal ") {
		return strings.TrimSpace(v[len("ethereal "):])
	}
	if strings.HasPrefix(lower, "eth ") {
		return strings.TrimSpace(v[len("eth "):])
	}
	return v
}
