package cmd

import (
	"fmt"
	"strconv"
	"strings"
)

func stringValue(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func stringSliceValue(v any) []string {
	if v == nil {
		return nil
	}

	switch items := v.(type) {
	case []string:
		out := make([]string, 0, len(items))
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			text := stringValue(item)
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		text := stringValue(v)
		if text == "" {
			return nil
		}
		return []string{text}
	}
}

func parsePositiveIndex(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	idx, err := strconv.Atoi(value)
	if err != nil || idx <= 0 {
		return 0, fmt.Errorf("index must be a positive number")
	}
	return idx, nil
}
