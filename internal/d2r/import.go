package d2r

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
)

type guideGearItem struct {
	Slot string `json:"slot"`
	Item string `json:"item"`
}

type importProgressUpdate struct {
	Current  int
	Total    int
	Item     string
	Imported int
	Skipped  int
}

var errImportCancelled = errors.New("import cancelled")

var extractGuideGearWithLLM = extractGuideGearFromURL

func importGuideForCharacter(character string, url string, progressf func(string, ...any), updatef func(importProgressUpdate), cancelledf func() bool) (int, int, error) {
	logf := func(format string, args ...any) {
		if progressf != nil {
			progressf(format, args...)
		}
	}

	if character == "" {
		return 0, 0, fmt.Errorf("character cannot be empty")
	}
	if url == "" {
		return 0, 0, fmt.Errorf("url cannot be empty")
	}

	logf("import start: character=%q url=%q\n", character, url)

	data, err := readCharacterData(character)
	if err != nil {
		logf("import failed reading character data: %v\n", err)
		return 0, 0, err
	}

	charClass := stringValue(data["class"])
	if charClass == "" {
		logf("import failed: class not set for character=%q\n", character)
		return 0, 0, fmt.Errorf("character %q has no class set", character)
	}
	logf("import context: class=%q\n", charClass)

	cfg, err := readConfig()
	if err != nil {
		logf("import failed reading config: %v\n", err)
		return 0, 0, err
	}

	items, err := extractGuideGearWithLLM(url, cfg)
	if err != nil {
		logf("import failed extracting guide gear: %v\n", err)
		return 0, 0, fmt.Errorf("extract guide gear: %w", err)
	}
	if len(items) == 0 {
		logf("import failed: extractor returned zero items\n")
		return 0, 0, fmt.Errorf("no gear items found in guide")
	}
	logf("import extracted items: count=%d\n", len(items))

	gearList := coerceGearEntries(data["gear"])
	imported := 0
	skipped := 0
	timeoutSkips := 0
	total := len(items)

	for idx, item := range items {
		if cancelledf != nil && cancelledf() {
			logf("import cancelled before %d/%d\n", idx+1, total)
			return imported, skipped, errImportCancelled
		}

		name := strings.TrimSpace(item.Item)
		if name == "" {
			logf("skip %d/%d: empty item name\n", idx+1, total)
			if updatef != nil {
				updatef(importProgressUpdate{Current: idx + 1, Total: total, Item: "", Imported: imported, Skipped: skipped})
			}
			continue
		}
		logf("resolving %d/%d: item=%q slot=%q\n", idx+1, total, name, strings.TrimSpace(item.Slot))
		if updatef != nil {
			updatef(importProgressUpdate{Current: idx + 1, Total: total, Item: name, Imported: imported, Skipped: skipped})
		}
		slotHint, weaponSwap, swapRole, merc := mapGuideSlot(item.Slot)
		logf("resolve hint %d/%d: item=%q slot_hint=%q weapon_swap=%t swap_role=%q merc=%t\n", idx+1, total, name, slotHint, weaponSwap, swapRole, merc)

		entry, err := resolveGearWithLLM(name, charClass, "", cfg)
		if err != nil {
			if isTimeoutError(err) {
				timeoutSkips++
			}
			logf("skip %d/%d: resolve failed for item=%q: %v\n", idx+1, total, name, err)
			skipped++
			if updatef != nil {
				updatef(importProgressUpdate{Current: idx + 1, Total: total, Item: name, Imported: imported, Skipped: skipped})
			}
			continue
		}

		if cancelledf != nil && cancelledf() {
			logf("import cancelled during %d/%d after resolve\n", idx+1, total)
			return imported, skipped, errImportCancelled
		}

		if stringValue(entry["exact_name"]) == "" {
			entry["exact_name"] = name
		}
		if stringValue(entry["query"]) == "" {
			entry["query"] = name
		}
		normalizeResolvedSlotAndRole(entry)

		if slotHint != "" && slotHint != "unknown" {
			switch slotHint {
			case "weapon":
				entry["slot"] = "weapon"
				entry["swap_role"] = normalizeSwapRole(swapRole)
			case "helm", "armor", "gloves", "boots", "belt", "ring", "amulet", "inventory":
				entry["slot"] = slotHint
				delete(entry, "swap_role")
				entry["weapon_swap"] = false
			}
		}
		if !weaponSwap && stringValue(entry["slot"]) == "weapon" && normalizeSwapRole(swapRole) == "offhand" {
			entry["swap_role"] = "offhand"
		}
		if weaponSwap {
			entry["weapon_swap"] = true
			entry["slot"] = "weapon"
			entry["swap_role"] = normalizeSwapRole(swapRole)
		}
		if merc {
			entry["merc"] = true
			applyMercEtherealIfIndestructible(entry)
		}

		entries := cloneEntryForPossibleSlots(entry)
		if slotHint != "" && slotHint != "unknown" {
			entries = nil // Guide explicitly provided the slot placement; do not fan out.
		}
		if len(entries) == 0 {
			entries = []map[string]any{entry}
		}

		addedThisItem := 0
		for _, expanded := range entries {
			if merc {
				expanded["merc"] = true
			}
			gearList = append(gearList, expanded)
			addedThisItem++
		}

		imported += addedThisItem
		logf("imported %d/%d: item=%q added=%d kind=%q slot=%q merc=%t\n", idx+1, total, stringValue(entry["exact_name"]), addedThisItem, stringValue(entry["kind"]), stringValue(entry["slot"]), merc)

		// Persist after each successful add so the UI can reflect live progress.
		setGearEntries(data, gearList)
		if err := writeCharacterData(character, data); err != nil {
			logf("import failed writing character data after item=%q: %v\n", name, err)
			return imported, skipped, err
		}
		if updatef != nil {
			updatef(importProgressUpdate{Current: idx + 1, Total: total, Item: name, Imported: imported, Skipped: skipped})
		}
	}

	logf("writing character data: total_entries=%d imported=%d skipped=%d\n", len(gearList), imported, skipped)
	setGearEntries(data, gearList)
	if err := writeCharacterData(character, data); err != nil {
		logf("import failed writing character data: %v\n", err)
		return 0, 0, err
	}
	if imported == 0 && timeoutSkips > 0 {
		return 0, skipped, fmt.Errorf("all item resolutions timed out (skipped %d item(s)); please retry", timeoutSkips)
	}
	logf("import done: imported=%d skipped=%d\n", imported, skipped)
	return imported, skipped, nil
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "timeout")
}

func mapGuideSlot(raw string) (slot string, weaponSwap bool, swapRole string, merc bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	mercSection := strings.Contains(value, "merc") || strings.Contains(value, "hireling")

	if strings.Contains(value, "weapon-swap") || strings.Contains(value, "swap") {
		if strings.Contains(value, "off-hand") || strings.Contains(value, "off hand") {
			return "weapon", true, "offhand", mercSection
		}
		return "weapon", true, "main", mercSection
	}
	if strings.Contains(value, "off-hand") || strings.Contains(value, "off hand") {
		return "weapon", false, "offhand", mercSection
	}
	if strings.Contains(value, "weapon") {
		return "weapon", false, "main", mercSection
	}
	if strings.Contains(value, "helmet") || strings.Contains(value, "helm") {
		return "helm", false, "main", mercSection
	}
	if strings.Contains(value, "body armor") || strings.Contains(value, "armor") {
		return "armor", false, "main", mercSection
	}
	if strings.Contains(value, "glove") || strings.Contains(value, "gauntlet") || strings.Contains(value, "bracer") || strings.Contains(value, "vambrace") {
		return "gloves", false, "main", false
	}
	if strings.Contains(value, "boot") || strings.Contains(value, "greaves") {
		return "boots", false, "main", false
	}
	if strings.Contains(value, "belt") {
		return "belt", false, "main", false
	}
	if strings.Contains(value, "ring") {
		return "ring", false, "main", false
	}
	if strings.Contains(value, "amulet") {
		return "amulet", false, "main", false
	}
	if strings.Contains(value, "charm") || strings.Contains(value, "inventory") {
		return "inventory", false, "main", false
	}
	return "unknown", false, "main", false
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
	resp, err := client.Get(url) //nolint:gosec
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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	systemPrompt := strings.TrimSpace(`You extract Diablo II gear options from guide page text.
Return strict JSON only.
Extract items from the build's gear sections, including both player gear and Mercenary gear sections.
If a guide has a dedicated Mercenary section, include those items with slot labels that preserve merc context (e.g. "Mercenary Weapon", "Mercenary Helm", "Mercenary Armor").
Ignore stats, skills, farming spots, and explanatory prose.
Return slot labels and item names.`)

	userPrompt := fmt.Sprintf("URL: %s\nPage text:\n%s\n\nReturn JSON with items: [{slot,item}] from gear sections, including Mercenary gear sections when present.", url, pageText)

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
