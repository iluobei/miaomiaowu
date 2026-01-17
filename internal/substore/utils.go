package substore

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// IPv4 regex pattern - simplified for Go regex (no negative lookahead)
	ipv4Regex = regexp.MustCompile(`^((25[0-5]|(2[0-4]|1[0-9]|[1-9])?[0-9])\.){3}(25[0-5]|(2[0-4]|1[0-9]|[1-9])?[0-9])$`)

	// IPv6 regex pattern - simplified version
	ipv6Regex = regexp.MustCompile(`^(([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|::)$`)

	// Port number validation regex
	portRegex = regexp.MustCompile(`^((6553[0-5])|(655[0-2][0-9])|(65[0-4][0-9]{2})|(6[0-4][0-9]{3})|([1-5][0-9]{4})|([0-5]{0,5})|([0-9]{1,4}))$`)

	// UUID validation regex
	uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// Result is a helper for building proxy configuration strings
type Result struct {
	Proxy  map[string]interface{}
	Output []string
}

// NewResult creates a new Result instance
func NewResult(proxy map[string]interface{}) *Result {
	return &Result{
		Proxy:  proxy,
		Output: make([]string, 0),
	}
}

// Append adds data to the output
func (r *Result) Append(data string) error {
	if data == "" {
		return fmt.Errorf("required field is missing")
	}
	r.Output = append(r.Output, data)
	return nil
}

// AppendIfPresent adds data to output if the attribute is present in proxy
func (r *Result) AppendIfPresent(format string, attr string) {
	if IsPresent(r.Proxy, attr) {
		val := r.Proxy[attr]
		formatted := fmt.Sprintf(format, val)
		r.Append(formatted)
	}
}

// String returns the joined output
func (r *Result) String() string {
	return strings.Join(r.Output, "")
}

// IsPresent checks if a value is present (not nil/empty)
func IsPresent(obj interface{}, attrs ...string) bool {
	if obj == nil {
		return false
	}

	// If no attributes specified, just check if obj is present
	if len(attrs) == 0 {
		return obj != nil
	}

	// Navigate through nested attributes
	current := obj
	for _, attr := range attrs {
		var m map[string]interface{}
		var ok bool

		// Try to convert to map[string]interface{} or Proxy
		if m, ok = current.(map[string]interface{}); !ok {
			var proxy Proxy
			if proxy, ok = current.(Proxy); ok {
				m = proxy
			}
		}

		if ok {
			val, exists := m[attr]
			if !exists || val == nil {
				return false
			}
			current = val
		} else {
			return false
		}
	}
	return true
}

// GetValue gets a nested value from a map using dot notation
func GetValue(obj map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	current := interface{}(obj)

	for _, part := range parts {
		if m, ok := current.(map[string]interface{}); ok {
			val, exists := m[part]
			if !exists {
				return nil, false
			}
			current = val
		} else {
			return nil, false
		}
	}
	return current, true
}

// IsIPv4 checks if a string is a valid IPv4 address
func IsIPv4(ip string) bool {
	return ipv4Regex.MatchString(ip)
}

// IsIPv6 checks if a string is a valid IPv6 address
func IsIPv6(ip string) bool {
	return ipv6Regex.MatchString(ip)
}

// IsValidPortNumber checks if a port number is valid
func IsValidPortNumber(port interface{}) bool {
	portStr := fmt.Sprintf("%v", port)
	return portRegex.MatchString(portStr)
}

// IsNotBlank checks if a string is not blank
func IsNotBlank(str string) bool {
	return strings.TrimSpace(str) != ""
}

// GetIfNotBlank returns str if it's not blank, otherwise returns defaultValue
func GetIfNotBlank(str, defaultValue string) string {
	if IsNotBlank(str) {
		return str
	}
	return defaultValue
}

// GetIfPresent returns obj if it's present, otherwise returns defaultValue
func GetIfPresent(obj, defaultValue interface{}) interface{} {
	if IsPresent(obj) {
		return obj
	}
	return defaultValue
}

// GetPolicyDescriptor returns policy descriptor or policy based on input format
func GetPolicyDescriptor(str string) map[string]string {
	if str == "" {
		return map[string]string{}
	}

	// Check if it matches policy descriptor format
	matched, _ := regexp.MatchString(`^.+?\s*?=\s*?.+?\s*?,.+?`, str)
	if matched {
		return map[string]string{"policy-descriptor": str}
	}
	return map[string]string{"policy": str}
}

// GetRandomInt returns a random integer between min and max (inclusive)
func GetRandomInt(min, max int) int {
	return rand.Intn(max-min+1) + min
}

// GetRandomPort returns a random port from a port string (e.g., "80,443" or "8000-9000")
func GetRandomPort(portString string) int {
	portParts := regexp.MustCompile(`[,/]`).Split(portString, -1)
	randomPart := portParts[rand.Intn(len(portParts))]

	if strings.Contains(randomPart, "-") {
		parts := strings.Split(randomPart, "-")
		if len(parts) == 2 {
			min, _ := strconv.Atoi(parts[0])
			max, _ := strconv.Atoi(parts[1])
			return GetRandomInt(min, max)
		}
	}

	port, _ := strconv.Atoi(randomPart)
	return port
}

// NumberToString converts a number to string
func NumberToString(value int64) string {
	return strconv.FormatInt(value, 10)
}

// IsValidUUID checks if a string is a valid UUID
func IsValidUUID(uuid string) bool {
	return uuidRegex.MatchString(uuid)
}

// FormatDateTime formats a time.Time to a string with the given format
func FormatDateTime(t time.Time, format string) string {
	if format == "" {
		format = "2006-01-02_15-04-05"
	}

	// Replace format tokens
	result := format
	result = strings.ReplaceAll(result, "YYYY", t.Format("2006"))
	result = strings.ReplaceAll(result, "MM", t.Format("01"))
	result = strings.ReplaceAll(result, "DD", t.Format("02"))
	result = strings.ReplaceAll(result, "HH", t.Format("15"))
	result = strings.ReplaceAll(result, "mm", t.Format("04"))
	result = strings.ReplaceAll(result, "ss", t.Format("05"))

	return result
}

// GetString safely gets a string value from a map
func GetString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// GetAnyString safely gets a string value from a map, handling various types
func GetAnyString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case string:
			return v
		case int:
			return fmt.Sprintf("%d", v)
		case int64:
			return fmt.Sprintf("%d", v)
		case float64:
			// 如果是整数但以 float64 形式存储
			if v == float64(int64(v)) {
				return fmt.Sprintf("%d", int64(v))
			}
			return fmt.Sprintf("%g", v)
		case bool:
			if v {
				return "true"
			}
			return "false"
		case []byte:
			return string(v)
		default:
			// 尝试转换为字符串
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

// GetInt safely gets an int value from a map
func GetInt(m map[string]interface{}, key string) int {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		case string:
			if i, err := strconv.Atoi(v); err == nil {
				return i
			}
		}
	}
	return 0
}

// GetBool safely gets a bool value from a map
func GetBool(m map[string]interface{}, key string) bool {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// GetStringSlice safely gets a string slice from a map
func GetStringSlice(m map[string]interface{}, key string) []string {
	if val, ok := m[key]; ok {
		if slice, ok := val.([]string); ok {
			return slice
		}
		if slice, ok := val.([]interface{}); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
	}
	return nil
}

// GetMap safely gets a map from a map
func GetMap(m map[string]interface{}, key string) map[string]interface{} {
	if val, ok := m[key]; ok {
		if subMap, ok := val.(map[string]interface{}); ok {
			return subMap
		}
	}
	return nil
}

// Base64Encode encodes a string to base64 (standard encoding, no padding removed)
func Base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// Base64EncodeURLSafe encodes a string to URL-safe base64
func Base64EncodeURLSafe(s string) string {
	return base64.URLEncoding.EncodeToString([]byte(s))
}

// URLEncode encodes a string for use in URL query parameters
func URLEncode(s string) string {
	return url.QueryEscape(s)
}

// ArrayToString converts an array to a comma-separated string
func ArrayToString(arr interface{}) string {
	switch v := arr.(type) {
	case []string:
		return strings.Join(v, ",")
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, fmt.Sprintf("%v", item))
		}
		return strings.Join(parts, ",")
	case string:
		return v
	default:
		return fmt.Sprintf("%v", arr)
	}
}

// GetSNI retrieves the SNI (Server Name Indication) value from proxy
// Compatible with both SubStore's "sni" field and miaomiaowu's "servername" field
// Priority: sni > servername
func GetSNI(proxy Proxy) string {
	// First try SubStore standard field "sni"
	if sni := GetString(proxy, "sni"); sni != "" {
		return sni
	}
	// Fallback to miaomiaowu's "servername" field (imported from Clash)
	if servername := GetString(proxy, "servername"); servername != "" {
		return servername
	}
	return ""
}
