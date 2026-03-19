// Package discovery provides agent discovery, capability tag standardization,
// and reputation-weighted matching for the ClawNet agent network.
package discovery

import (
	"encoding/json"
	"sort"
	"strings"
)

// Standard capability tag categories.
// Aligned with A2A Agent Card "capabilities" taxonomy.
var StandardTags = map[string][]string{
	"development": {
		"code-review", "debugging", "testing", "devops", "ci-cd",
		"frontend", "backend", "fullstack", "api-design", "database",
	},
	"languages": {
		"python", "javascript", "typescript", "go", "rust", "java",
		"c", "cpp", "ruby", "swift", "kotlin", "php", "shell",
	},
	"ai-ml": {
		"machine-learning", "deep-learning", "nlp", "computer-vision",
		"data-science", "data-analysis", "llm", "fine-tuning", "rag",
	},
	"content": {
		"writing", "translation", "copywriting", "documentation",
		"editing", "proofreading", "summarization",
	},
	"research": {
		"research", "literature-review", "data-collection",
		"competitive-analysis", "market-research",
	},
	"design": {
		"ui-design", "ux-design", "graphic-design", "prototyping",
	},
	"ops": {
		"deployment", "monitoring", "security", "cloud", "networking",
	},
}

// tagAliases maps common variations to canonical tag names.
var tagAliases = map[string]string{
	"js":              "javascript",
	"ts":              "typescript",
	"py":              "python",
	"golang":          "go",
	"c++":             "cpp",
	"ml":              "machine-learning",
	"dl":              "deep-learning",
	"cv":              "computer-vision",
	"translate":       "translation",
	"write":           "writing",
	"edit":            "editing",
	"review":          "code-review",
	"test":            "testing",
	"deploy":          "deployment",
	"ops":             "devops",
	"frontend-dev":    "frontend",
	"backend-dev":     "backend",
	"full-stack":      "fullstack",
	"data-analytics":  "data-analysis",
	"data-collection": "data-collection",
	"web-scraping":    "data-collection",
	"scraping":        "data-collection",
}

// NormalizeTag lowercases, trims, and resolves aliases for a single tag.
func NormalizeTag(raw string) string {
	t := strings.TrimSpace(strings.ToLower(raw))
	t = strings.ReplaceAll(t, " ", "-")
	if canonical, ok := tagAliases[t]; ok {
		return canonical
	}
	return t
}

// NormalizeTags normalizes a slice of tags, deduplicates, and sorts.
func NormalizeTags(tags []string) []string {
	seen := make(map[string]bool, len(tags))
	var result []string
	for _, raw := range tags {
		t := NormalizeTag(raw)
		if t != "" && !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}
	sort.Strings(result)
	return result
}

// ParseTagsJSON parses a JSON array string into normalized tags.
func ParseTagsJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		// Comma-separated fallback
		tags = strings.Split(raw, ",")
	}
	return NormalizeTags(tags)
}

// TagOverlap computes the fraction of required tags that the agent possesses.
// Returns 0.0-1.0 and the list of matched tags.
func TagOverlap(agentTags, requiredTags []string) (float64, []string) {
	if len(requiredTags) == 0 {
		return 1.0, nil
	}
	agentSet := make(map[string]bool, len(agentTags))
	for _, t := range agentTags {
		agentSet[NormalizeTag(t)] = true
	}
	var matched []string
	for _, t := range requiredTags {
		if agentSet[NormalizeTag(t)] {
			matched = append(matched, t)
		}
	}
	return float64(len(matched)) / float64(len(requiredTags)), matched
}

// CategoryFor returns the category a tag belongs to (empty if unknown).
func CategoryFor(tag string) string {
	t := NormalizeTag(tag)
	for category, tags := range StandardTags {
		for _, ct := range tags {
			if ct == t {
				return category
			}
		}
	}
	return ""
}

// AllStandardTags returns all registered standard tags in sorted order.
func AllStandardTags() []string {
	var all []string
	for _, tags := range StandardTags {
		all = append(all, tags...)
	}
	sort.Strings(all)
	return all
}

// InferTagsFromText extracts likely capability tags from free text
// (e.g. task title + description). Returns only tags that match standard taxonomy.
func InferTagsFromText(text string) []string {
	lower := strings.ToLower(text)
	var found []string
	seen := make(map[string]bool)

	// Check aliases first
	for alias, canonical := range tagAliases {
		if strings.Contains(lower, alias) && !seen[canonical] {
			seen[canonical] = true
			found = append(found, canonical)
		}
	}
	// Check standard tags
	for _, tags := range StandardTags {
		for _, tag := range tags {
			if !seen[tag] && strings.Contains(lower, tag) {
				seen[tag] = true
				found = append(found, tag)
			}
		}
	}
	sort.Strings(found)
	return found
}
