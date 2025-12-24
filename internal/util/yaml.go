package util

import (
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

// ProxyPriorityFields defines the fields that should appear first in proxy configuration
var ProxyPriorityFields = []string{"name", "type", "server", "port"}

// ReorderProxyFieldsToNode reorders proxy configuration to put key fields first
// Returns a yaml.Node to preserve field order
func ReorderProxyFieldsToNode(proxy map[string]any) *yaml.Node {
	node := &yaml.Node{
		Kind: yaml.MappingNode,
	}

	// First add priority keys in order
	for _, key := range ProxyPriorityFields {
		if val, exists := proxy[key]; exists {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
			valNode := &yaml.Node{}
			valNode.Encode(val)
			node.Content = append(node.Content, keyNode, valNode)
		}
	}

	// Then add remaining keys (in original order from map iteration)
	for key, val := range proxy {
		if !isPriorityField(key) {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
			valNode := &yaml.Node{}
			valNode.Encode(val)
			node.Content = append(node.Content, keyNode, valNode)
		}
	}

	return node
}

// ReorderProxyNode reorders fields in an existing yaml.Node proxy
// Preserves existing field values and order for non-priority fields
func ReorderProxyNode(proxyNode *yaml.Node) *yaml.Node {
	if proxyNode == nil || proxyNode.Kind != yaml.MappingNode {
		return proxyNode
	}

	result := &yaml.Node{
		Kind: yaml.MappingNode,
	}

	// Build a map of key -> value node pairs
	fieldMap := make(map[string]*yaml.Node)
	for i := 0; i < len(proxyNode.Content)-1; i += 2 {
		keyNode := proxyNode.Content[i]
		valueNode := proxyNode.Content[i+1]
		if keyNode.Kind == yaml.ScalarNode {
			fieldMap[keyNode.Value] = valueNode
		}
	}

	// First add priority fields in order
	for _, key := range ProxyPriorityFields {
		if valNode, exists := fieldMap[key]; exists {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
			result.Content = append(result.Content, keyNode, valNode)
		}
	}

	// Then add remaining fields in original order
	for i := 0; i < len(proxyNode.Content)-1; i += 2 {
		keyNode := proxyNode.Content[i]
		valueNode := proxyNode.Content[i+1]
		if keyNode.Kind == yaml.ScalarNode && !isPriorityField(keyNode.Value) {
			newKeyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: keyNode.Value}
			result.Content = append(result.Content, newKeyNode, valueNode)
		}
	}

	return result
}

// isPriorityField checks if a field name is in the priority list
func isPriorityField(key string) bool {
	for _, pf := range ProxyPriorityFields {
		if key == pf {
			return true
		}
	}
	return false
}

// ValueToYAMLNode converts a Go value to a yaml.Node with proper type tags
func ValueToYAMLNode(value any) *yaml.Node {
	switch v := value.(type) {
	case bool:
		node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool"}
		if v {
			node.Value = "true"
		} else {
			node.Value = "false"
		}
		return node
	case int:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(v)}
	case int64:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(v, 10)}
	case float64:
		// Check if it's actually an integer
		if v == float64(int64(v)) {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(int64(v), 10)}
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: strconv.FormatFloat(v, 'f', -1, 64)}
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Value: v}
	default:
		// For complex types, marshal and unmarshal to get proper node
		data, _ := yaml.Marshal(value)
		var node yaml.Node
		_ = yaml.Unmarshal(data, &node)
		if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
			return node.Content[0]
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("%v", value)}
	}
}

// GetNodeFieldValue gets a string field value from a mapping node
func GetNodeFieldValue(node *yaml.Node, fieldName string) string {
	if node == nil || node.Kind != yaml.MappingNode {
		return ""
	}

	for i := 0; i < len(node.Content)-1; i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == fieldName {
			if valueNode.Kind == yaml.ScalarNode {
				return valueNode.Value
			}
		}
	}

	return ""
}

// SetNodeField sets or updates a field in a mapping node
func SetNodeField(node *yaml.Node, fieldName string, value any) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}

	// Find existing field
	for i := 0; i < len(node.Content)-1; i += 2 {
		keyNode := node.Content[i]
		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == fieldName {
			// Update existing field
			node.Content[i+1] = ValueToYAMLNode(value)
			return
		}
	}

	// Add new field
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: fieldName},
		ValueToYAMLNode(value),
	)
}
