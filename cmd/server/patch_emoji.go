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

// needsPatching checks if a file contains Unicode escape sequences or quoted numbers that need fixing
func needsPatching(content string) bool {
	// Check for patterns like \U0001F4B0 or \u4E2D
	if strings.Contains(content, "\\U") || strings.Contains(content, "\\u") {
		return true
	}

	// Check for quoted numeric values like port: "443"
	numericQuotesRe := regexp.MustCompile(`:\s+"(\d+)"`)
	return numericQuotesRe.MatchString(content)
}

// removeUnicodeEscapeQuotesFromFile removes quotes from strings that contain Unicode escape sequences
// and converts the escape sequences back to actual Unicode characters (like emoji)
func removeUnicodeEscapeQuotesFromFile(yamlContent string) string {
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

	return result
}
