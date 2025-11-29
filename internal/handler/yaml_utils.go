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
	quotedUnicodeRe := regexp.MustCompile(`"([^"]*\\[Uu][0-9A-Fa-f]{4,8}[^"]*)"`)
	result := quotedUnicodeRe.ReplaceAllStringFunc(yamlContent, func(match string) string {
		// Remove the outer quotes
		return strings.Trim(match, `"`)
	})

	// Step 2: Convert ALL Unicode escapes back to actual characters (quoted or not)
	// \U0001F4B0 -> 💰, \u4E2D -> 中, \U0001F1ED\U0001F1F0 -> 🇭🇰
	escapeRe := regexp.MustCompile(`\\U([0-9A-Fa-f]{8})|\\u([0-9A-Fa-f]{4})`)
	result = escapeRe.ReplaceAllStringFunc(result, func(escapeSeq string) string {
		var codepoint int
		if strings.HasPrefix(escapeSeq, `\U`) {
			fmt.Sscanf(escapeSeq, `\U%X`, &codepoint)
		} else {
			fmt.Sscanf(escapeSeq, `\u%X`, &codepoint)
		}
		return string(rune(codepoint))
	})

	// Step 3: Remove quotes from numeric values (like port: "443")
	// This fixes values that should be numbers but got quoted
	numericQuotesRe := regexp.MustCompile(`:\s+"(\d+)"`)
	result = numericQuotesRe.ReplaceAllString(result, `: $1`)

	// Step 4: Convert single quotes to double quotes for short-id field
	// Replace short-id: 'value' with short-id: "value"
	// Match only at line boundaries to avoid matching partial strings
	shortIdSingleQuoteRe := regexp.MustCompile(`(?m)^([ \t]+)short-id:[ \t]+'([^']*)'[ \t]*$`)
	result = shortIdSingleQuoteRe.ReplaceAllString(result, `$1short-id: "$2"`)

	// Step 5: Add quotes to empty short-id values (short-id: → short-id: "")
	// Match only complete lines ending after the colon
	emptyShortIdRe := regexp.MustCompile(`(?m)^([ \t]+)short-id:[ \t]*$`)
	result = emptyShortIdRe.ReplaceAllString(result, `$1short-id: ""`)

	return result
}
