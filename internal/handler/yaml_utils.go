package handler

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// MarshalYAMLWithIndent marshals a YAML node with 2-space indentation
func MarshalYAMLWithIndent(node *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(node); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalWithIndent marshals any value to YAML with 2-space indentation
func MarshalWithIndent(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RemoveUnicodeEscapeQuotes removes quotes from strings that contain Unicode escape sequences
// and converts the escape sequences back to actual Unicode characters (like emoji).
// This also removes quotes from numeric values.
func RemoveUnicodeEscapeQuotes(yamlContent string) string {
	// Step 1: Remove quotes from strings that contain Unicode escape sequences
	// Pattern: "...\U000XXXXX..." or "...\uXXXX..."
	// But keep quotes if the unquoted string would start with YAML special characters
	quotedUnicodeRe := regexp.MustCompile(`"([^"]*\\[Uu][0-9A-Fa-f]{4,8}[^"]*)"`)
	result := quotedUnicodeRe.ReplaceAllStringFunc(yamlContent, func(match string) string {
		// Get the content without quotes
		content := strings.Trim(match, `"`)

		// First convert Unicode escapes to actual characters to check the real first character
		tempContent := convertUnicodeEscapes(content)

		// Check if the unquoted string would start with YAML special characters that need quoting
		// These characters have special meaning in YAML and need to be quoted
		if len(tempContent) > 0 {
			firstChar := tempContent[0]
			// Characters that need quoting at the start: [ ] { } * & ! | > ' " % @ ` # , ? : -
			if firstChar == '[' || firstChar == ']' || firstChar == '{' || firstChar == '}' ||
				firstChar == '*' || firstChar == '&' || firstChar == '!' || firstChar == '|' ||
				firstChar == '>' || firstChar == '\'' || firstChar == '"' || firstChar == '%' ||
				firstChar == '@' || firstChar == '`' || firstChar == '#' || firstChar == ',' ||
				firstChar == '?' || firstChar == ':' || firstChar == '-' {
				// Keep the quotes but still convert Unicode escapes inside
				return `"` + convertUnicodeEscapes(content) + `"`
			}
		}

		// Safe to remove quotes
		return content
	})

	// Step 2: Convert ALL Unicode escapes back to actual characters (quoted or not)
	// \U0001F4B0 -> 💰, \u4E2D -> 中, \U0001F1ED\U0001F1F0 -> 🇭🇰
	result = convertUnicodeEscapes(result)

	// Step 3: Remove quotes from numeric values (like port: "443")
	// This fixes values that should be numbers but got quoted
	numericQuotesRe := regexp.MustCompile(`:\s+"(\d+)"`)
	result = numericQuotesRe.ReplaceAllString(result, `: $1`)

	return result
}

// convertUnicodeEscapes converts Unicode escape sequences to actual characters
func convertUnicodeEscapes(s string) string {
	escapeRe := regexp.MustCompile(`\\U([0-9A-Fa-f]{8})|\\u([0-9A-Fa-f]{4})`)
	return escapeRe.ReplaceAllStringFunc(s, func(escapeSeq string) string {
		var codepoint int
		if strings.HasPrefix(escapeSeq, `\U`) {
			fmt.Sscanf(escapeSeq, `\U%X`, &codepoint)
		} else {
			fmt.Sscanf(escapeSeq, `\u%X`, &codepoint)
		}
		return string(rune(codepoint))
	})
}
