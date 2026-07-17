package service

import (
	"regexp"
	"strings"
)

var templatePattern = regexp.MustCompile(`\{\{(\w+)}}`)

// RenderTemplate replaces {{key}} placeholders with values from the variables map.
// Unresolved placeholders are left as-is, matching Java TemplateEngine behavior.
func RenderTemplate(template string, variables map[string]string) string {
	if variables == nil {
		return template
	}
	return templatePattern.ReplaceAllStringFunc(template, func(match string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}")
		if val, ok := variables[key]; ok {
			return val
		}
		return match
	})
}
