package d2r

import "strings"

var runeOrder = []string{
	"El", "Eld", "Tir", "Nef", "Eth", "Ith", "Tal", "Ral", "Ort", "Thul", "Amn", "Sol",
	"Shael", "Dol", "Hel", "Io", "Lum", "Ko", "Fal", "Lem", "Pul", "Um", "Mal", "Ist",
	"Gul", "Vex", "Ohm", "Lo", "Sur", "Ber", "Jah", "Cham", "Zod",
}

var runeOrderIndex = buildRuneOrderIndex()

func buildRuneOrderIndex() map[string]int {
	out := make(map[string]int, len(runeOrder))
	for idx, runeName := range runeOrder {
		out[strings.ToLower(runeName)] = idx
	}
	return out
}

func canonicalRuneName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	for _, runeName := range runeOrder {
		if strings.ToLower(runeName) == lower {
			return runeName
		}
	}
	if len(lower) == 1 {
		return strings.ToUpper(lower)
	}
	return strings.ToUpper(lower[:1]) + lower[1:]
}

func runeDifficultyOrder(runeName string) int {
	if idx, ok := runeOrderIndex[strings.ToLower(runeName)]; ok {
		return idx
	}
	return len(runeOrder) + 1000
}

func countessDifficultiesForRune(runeName string) []string {
	idx := runeDifficultyOrder(runeName)
	if idx > runeDifficultyOrder("Ist") {
		return nil
	}

	out := make([]string, 0, 3)
	if idx <= runeDifficultyOrder("Ral") {
		out = append(out, "normal")
	}
	if idx <= runeDifficultyOrder("Ko") {
		out = append(out, "nightmare")
	}
	out = append(out, "hell")
	return out
}
