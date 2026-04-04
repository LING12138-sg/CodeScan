package report

import (
	"encoding/json"
	"strings"
)

func parseFindings(raw json.RawMessage, fallback string) ([]map[string]any, string, bool) {
	if findings, ok := decodeJSONArray(raw); ok {
		return findings, "", true
	}
	if findings, ok := decodeJSONArray([]byte(extractJSONBlock(fallback))); ok {
		return findings, "", true
	}

	trimmed := strings.TrimSpace(fallback)
	if trimmed != "" {
		return nil, trimmed, true
	}

	if trimmedRaw := strings.TrimSpace(string(raw)); trimmedRaw != "" && trimmedRaw != "{}" && trimmedRaw != "null" {
		return nil, trimmedRaw, true
	}

	return nil, "", false
}

func parseRouteCount(raw json.RawMessage, fallback string) int {
	if routes, ok := decodeJSONArray(raw); ok {
		return len(routes)
	}
	if routes, ok := decodeJSONArray([]byte(extractJSONBlock(fallback))); ok {
		return len(routes)
	}
	return 0
}

func decodeJSONArray(raw []byte) ([]map[string]any, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return nil, false
	}

	var findings []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &findings); err == nil {
		return findings, true
	}

	return nil, false
}

func extractJSONBlock(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if json.Valid([]byte(input)) {
		return input
	}

	if start := strings.Index(input, "```"); start != -1 {
		end := strings.LastIndex(input, "```")
		if end > start {
			content := strings.TrimSpace(input[start+3 : end])
			content = strings.TrimPrefix(content, "json")
			content = strings.TrimSpace(content)
			if json.Valid([]byte(content)) {
				return content
			}
		}
	}

	if start := strings.Index(input, "["); start != -1 {
		if end := strings.LastIndex(input, "]"); end > start {
			candidate := input[start : end+1]
			if json.Valid([]byte(candidate)) {
				return candidate
			}
		}
	}

	if start := strings.Index(input, "{"); start != -1 {
		if end := strings.LastIndex(input, "}"); end > start {
			candidate := input[start : end+1]
			if json.Valid([]byte(candidate)) {
				return candidate
			}
		}
	}

	return input
}
