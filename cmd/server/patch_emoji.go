package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// patchSubscribeFilesEmoji fixes emoji escape sequences and removes unnecessary quotes
// from all YAML files in the subscribes directory.
// This is a one-time patch to fix issues introduced in previous versions.
func patchSubscribeFilesEmoji(subscribeDir string) {
	log.Printf("[Emoji Patch] Starting emoji patch for subscribe files in %s", subscribeDir)

	entries, err := os.ReadDir(subscribeDir)
	if err != nil {
		log.Printf("[Emoji Patch] Warning: failed to read subscribes directory: %v", err)
		return
	}

	patchedCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		ext := filepath.Ext(filename)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		// Skip the .keep.yaml placeholder file
		if filename == ".keep.yaml" {
			continue
		}

		filePath := filepath.Join(subscribeDir, filename)
		if patchFile(filePath) {
			patchedCount++
		}
	}

	if patchedCount > 0 {
		log.Printf("[Emoji Patch] Successfully patched %d subscribe file(s)", patchedCount)
	} else {
		log.Printf("[Emoji Patch] No files needed patching")
	}
}

// patchFile fixes emoji escapes and quotes in a single file
func patchFile(filePath string) bool {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Printf("[Emoji Patch] Warning: failed to read file %s: %v", filePath, err)
		return false
	}

	originalContent := string(content)

	// Check if file needs patching (contains Unicode escape sequences)
	if !needsPatching(originalContent) {
		return false
	}

	log.Printf("[Emoji Patch] Patching file: %s", filepath.Base(filePath))

	// Apply the fix
	fixedContent := removeUnicodeEscapeQuotesFromFile(originalContent)

	// Only write if content actually changed
	if fixedContent == originalContent {
		return false
	}

	// Write the fixed content back
	if err := ioutil.WriteFile(filePath, []byte(fixedContent), 0644); err != nil {
		log.Printf("[Emoji Patch] Warning: failed to write patched file %s: %v", filePath, err)
		return false
	}

	return true
}

// needsPatching checks if a file needs patching
func needsPatching(content string) bool {
	// Check for Unicode escape sequences
	if strings.Contains(content, "\\U") || strings.Contains(content, "\\u") {
		return true
	}

	// Check for quoted port values in proxies section
	if strings.Contains(content, "proxies:") {
		portQuotesRe := regexp.MustCompile(`(?m)^\s+port:\s+"(\d+)"`)
		if portQuotesRe.MatchString(content) {
			return true
		}
	}

	// Check for short-id that needs fixing (empty, <nil>, null, or unquoted values)
	// Match patterns like: "short-id: <nil>", "short-id: null", "short-id:", "short-id: abc"
	shortIdNeedsFixRe := regexp.MustCompile(`(?m)^\s+short-id:\s*(?:$|<nil>|null|[^"\n])`)
	return shortIdNeedsFixRe.MatchString(content)
}

// removeUnicodeEscapeQuotesFromFile applies the 3 fixes:
// 1. Remove quotes from port values in proxies section
// 2. Fix short-id values to use double quotes
// 3. Convert Unicode escapes back to emoji
func removeUnicodeEscapeQuotesFromFile(yamlContent string) string {
	result := yamlContent

	// Fix 1: Remove quotes from port values in proxies section only
	// Match: port: "443" → port: 443 (only under proxies)
	inProxiesSection := false
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		// Detect if we're in proxies section
		if strings.HasPrefix(strings.TrimSpace(line), "proxies:") {
			inProxiesSection = true
			continue
		}
		// Exit proxies section when we hit another top-level key
		if inProxiesSection && len(line) > 0 && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(line, ":") {
			inProxiesSection = false
		}

		// Fix port values only in proxies section
		if inProxiesSection {
			portQuotesRe := regexp.MustCompile(`^([ \t]+port:[ \t]*)"(\d+)"`)
			if portQuotesRe.MatchString(line) {
				lines[i] = portQuotesRe.ReplaceAllString(line, `$1$2`)
			}
		}
	}
	result = strings.Join(lines, "\n")

	// Fix 2: Fix short-id values to always use double quotes
	// Handle different cases:
	// - short-id: → short-id: ""
	// - short-id: <nil> → short-id: ""
	// - short-id: null → short-id: ""
	// - short-id: value → short-id: "value" (if not already quoted)
	// - short-id: 'value' → short-id: "value" (single quotes to double)

	// 2.1: Fix <nil> and null values
	nilShortIdRe := regexp.MustCompile(`(?m)^([ \t]+short-id:[ \t]*)<nil>([ \t]*)$`)
	result = nilShortIdRe.ReplaceAllString(result, `$1""$2`)

	nullShortIdRe := regexp.MustCompile(`(?m)^([ \t]+short-id:[ \t]*)null([ \t]*)$`)
	result = nullShortIdRe.ReplaceAllString(result, `$1""$2`)

	// 2.2: Fix empty values (just colon with nothing after)
	emptyShortIdRe := regexp.MustCompile(`(?m)^([ \t]+short-id:)([ \t]*)$`)
	result = emptyShortIdRe.ReplaceAllString(result, `$1 ""$2`)

	// 2.3: Convert single quotes to double quotes
	singleQuoteShortIdRe := regexp.MustCompile(`(?m)^([ \t]+short-id:[ \t]*)'([^']*)'([ \t]*)$`)
	result = singleQuoteShortIdRe.ReplaceAllString(result, `$1"$2"$3`)

	// 2.4: Add double quotes to unquoted non-empty values (but skip if already has quotes)
	// Match: short-id: abc123 (not starting with quote)
	unquotedShortIdRe := regexp.MustCompile(`(?m)^([ \t]+short-id:[ \t]*)([^"'\s][^ \t\n]*)([ \t]*)$`)
	result = unquotedShortIdRe.ReplaceAllString(result, `$1"$2"$3`)

	// Fix 3: Convert Unicode escape sequences back to actual emoji/characters
	// First, remove quotes from strings containing Unicode escapes to avoid double-encoding
	quotedUnicodeRe := regexp.MustCompile(`"([^"]*\\[Uu][0-9A-Fa-f]{4,8}[^"]*)"`)
	result = quotedUnicodeRe.ReplaceAllStringFunc(result, func(match string) string {
		return strings.Trim(match, `"`)
	})

	// Then convert all Unicode escapes: \U0001F4B0 → 💰, \u4E2D → 中
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

	return result
}
