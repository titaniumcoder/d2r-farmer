package d2r

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type modelRecord struct {
	ID      string
	Created int64
}

func availableModels(apiKey string) ([]webModelOption, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("openai api key is empty")
	}

	client := openai.NewClient(option.WithAPIKey(apiKey))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	page, err := client.Models.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list openai models: %w", err)
	}

	records := make([]modelRecord, 0, len(page.Data))
	seen := map[string]bool{}
	for _, m := range page.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		records = append(records, modelRecord{ID: id, Created: m.Created})
	}

	sort.Slice(records, func(i, j int) bool {
		if records[i].Created == records[j].Created {
			return records[i].ID < records[j].ID
		}
		return records[i].Created > records[j].Created
	})

	options := make([]webModelOption, 0, len(records))
	for _, rec := range records {
		options = append(options, webModelOption{ID: rec.ID, Pricing: knownModelPricing(rec.ID)})
	}

	return options, nil
}

func fallbackModels() []webModelOption {
	return []webModelOption{
		{ID: "gpt-5", Pricing: knownModelPricing("gpt-5")},
		{ID: "gpt-5-mini", Pricing: knownModelPricing("gpt-5-mini")},
		{ID: "gpt-5-nano", Pricing: knownModelPricing("gpt-5-nano")},
		{ID: "gpt-4.1", Pricing: knownModelPricing("gpt-4.1")},
		{ID: "gpt-4.1-mini", Pricing: knownModelPricing("gpt-4.1-mini")},
		{ID: "gpt-4.1-nano", Pricing: knownModelPricing("gpt-4.1-nano")},
		{ID: "gpt-4o", Pricing: knownModelPricing("gpt-4o")},
		{ID: "gpt-4o-mini", Pricing: knownModelPricing("gpt-4o-mini")},
	}
}

func knownModelPricing(modelID string) string {
	prices := map[string]string{
		"gpt-5":      "$1.25/M input, $10.00/M output",
		"gpt-5-mini": "$0.25/M input, $2.00/M output",
		"gpt-5-nano": "$0.05/M input, $0.40/M output",
	}
	return prices[strings.TrimSpace(modelID)]
}

func ensureSelectedModel(options []webModelOption, selected string) []webModelOption {
	sel := strings.TrimSpace(selected)
	if sel == "" {
		return options
	}
	for _, opt := range options {
		if opt.ID == sel {
			return options
		}
	}
	return append([]webModelOption{{ID: sel, Pricing: knownModelPricing(sel)}}, options...)
}
