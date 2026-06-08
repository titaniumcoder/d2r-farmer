package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
	"github.com/spf13/cobra"
)

type guideGearItem struct {
	Slot string `json:"slot"`
	Item string `json:"item"`
}

var extractGuideGearWithLLM = extractGuideGearFromURL

var importCmd = &cobra.Command{
	Use:   "import [character] [url]",
	Short: "Import gear from a Maxroll build guide",
	Long:  "Fetches a Maxroll guide URL and imports only gear options into the character.",
	Args:  cobra.ExactArgs(2),
	RunE:  runImport,
}

func init() {
	rootCmd.AddCommand(importCmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	character := strings.TrimSpace(args[0])
	if character == "" {
		return fmt.Errorf("character cannot be empty")
	}

	url := strings.TrimSpace(args[1])
	if url == "" {
		return fmt.Errorf("url cannot be empty")
	}

	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	charClass := stringValue(data["class"])
	if charClass == "" {
		return fmt.Errorf("character %q has no class set", character)
	}

	cfg, err := readConfig()
	if err != nil {
		return err
	}

	items, err := extractGuideGearWithLLM(url, cfg)
	if err != nil {
		return fmt.Errorf("extract guide gear: %w", err)
	}
	if len(items) == 0 {
		return fmt.Errorf("no gear items found in guide")
	}

	gearList := coerceGearEntries(data["gear"])
	imported := 0
	skipped := 0

	for _, item := range items {
		name := strings.TrimSpace(item.Item)
		if name == "" {
			continue
		}

		if findGearEntryIndex(gearList, name) >= 0 {
			skipped++
			continue
		}

		slotHint, weaponSwap, swapRole := mapGuideSlot(item.Slot)

		entry, err := resolveGearWithLLM(name, charClass, slotHint, cfg)
		if err != nil {
			cmd.Printf("skipped %q: %v\n", name, err)
			skipped++
			continue
		}

		if stringValue(entry["exact_name"]) == "" {
			entry["exact_name"] = name
		}
		if stringValue(entry["query"]) == "" {
			entry["query"] = name
		}

		if slotHint != "" {
			entry["slot"] = slotHint
		} else {
			entry["slot"] = normalizeSlotName(stringValue(entry["slot"]))
		}

		if weaponSwap {
			entry["weapon_swap"] = true
			entry["slot"] = "weapon"
			entry["swap_role"] = normalizeSwapRole(swapRole)
		}

		gearList = append(gearList, entry)
		imported++
	}

	setGearEntries(data, gearList)
	if err := writeCharacterData(character, data); err != nil {
		return err
	}

	cmd.Printf("imported %d gear items from %s (skipped %d)\n", imported, url, skipped)
	return nil
}

func mapGuideSlot(raw string) (slot string, weaponSwap bool, swapRole string) {
	value := strings.ToLower(strings.TrimSpace(raw))

	if strings.Contains(value, "weapon-swap") || strings.Contains(value, "swap") {
		if strings.Contains(value, "off-hand") || strings.Contains(value, "off hand") {
			return "weapon", true, "offhand"
		}
		return "weapon", true, "main"
	}

	if strings.Contains(value, "weapon") || strings.Contains(value, "off-hand") || strings.Contains(value, "off hand") {
		return "weapon", false, "main"
	}
	if strings.Contains(value, "helmet") || strings.Contains(value, "helm") {
		return "head", false, "main"
	}
	if strings.Contains(value, "body armor") || strings.Contains(value, "armor") {
		return "armor", false, "main"
	}
	if strings.Contains(value, "belt") {
		return "belt", false, "main"
	}
	if strings.Contains(value, "ring") {
		return "ring", false, "main"
	}
	if strings.Contains(value, "amulet") {
		return "amulet", false, "main"
	}
	if strings.Contains(value, "charm") || strings.Contains(value, "inventory") {
		return "inventory", false, "main"
	}

	return "inventory", false, "main"
}

func extractGuideGearFromURL(url string, cfg Config) ([]guideGearItem, error) {
	if cfg.Provider != "openai" {
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}

	body, err := fetchGuidePage(url)
	if err != nil {
		return nil, err
	}

	text := cleanupGuideHTML(body)
	if text == "" {
		return nil, fmt.Errorf("empty page content")
	}

	return extractGuideGearOpenAI(url, text, cfg.OpenAI)
}

func fetchGuidePage(url string) (string, error) {
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetch url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	return string(raw), nil
}

var (
	scriptStyleRE = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	tagRE         = regexp.MustCompile(`(?s)<[^>]+>`)
	spaceRE       = regexp.MustCompile(`[ \t\r\f\v]+`)
	newLineRE     = regexp.MustCompile(`\n{3,}`)
)

func cleanupGuideHTML(raw string) string {
	clean := scriptStyleRE.ReplaceAllString(raw, " ")
	clean = tagRE.ReplaceAllString(clean, "\n")
	clean = strings.ReplaceAll(clean, "&nbsp;", " ")
	clean = strings.ReplaceAll(clean, "&amp;", "&")
	clean = strings.ReplaceAll(clean, "&quot;", "\"")
	clean = strings.ReplaceAll(clean, "&#39;", "'")
	clean = spaceRE.ReplaceAllString(clean, " ")
	clean = strings.ReplaceAll(clean, "\r\n", "\n")
	clean = strings.ReplaceAll(clean, "\r", "\n")
	clean = newLineRE.ReplaceAllString(clean, "\n\n")
	clean = strings.TrimSpace(clean)

	idx := strings.Index(strings.ToLower(clean), "gear options")
	if idx >= 0 {
		start := idx
		if start > 2500 {
			start -= 2500
		} else {
			start = 0
		}
		end := idx + 25000
		if end > len(clean) {
			end = len(clean)
		}
		clean = clean[start:end]
	}

	if len(clean) > 45000 {
		clean = clean[:45000]
	}
	return clean
}

func extractGuideGearOpenAI(url string, pageText string, cfg OpenAIConfig) ([]guideGearItem, error) {
	client := newOpenAIClient(cfg)

	systemPrompt := strings.TrimSpace(`You extract Diablo II gear options from guide page text.
Return strict JSON only.
Extract only the Gear Options section items.
Ignore stats, skills, mercenary, farming spots, and explanatory prose.
Return slot labels and item names.`)

	userPrompt := fmt.Sprintf("URL: %s\nPage text:\n%s\n\nReturn JSON with items: [{slot,item}] from Gear Options only.", url, pageText)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"slot": map[string]any{"type": "string"},
						"item": map[string]any{"type": "string"},
					},
					"required":             []string{"slot", "item"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"items"},
		"additionalProperties": false,
	}

	response, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
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
					Name:        "gear_import",
					Description: openai.String("Gear import extraction"),
					Strict:      openai.Bool(true),
					Schema:      schema,
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("extract guide with openai: %w", err)
	}
	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no choices")
	}

	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		return nil, fmt.Errorf("openai returned empty content")
	}

	var parsed struct {
		Items []guideGearItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, fmt.Errorf("parse extracted gear JSON: %w", err)
	}

	seen := map[string]bool{}
	items := make([]guideGearItem, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		slot := strings.TrimSpace(item.Slot)
		name := strings.TrimSpace(item.Item)
		if slot == "" || name == "" {
			continue
		}
		key := strings.ToLower(slot + "|" + name)
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, guideGearItem{Slot: slot, Item: name})
	}

	return items, nil
}
