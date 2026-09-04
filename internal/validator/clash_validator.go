package validator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidationLevel 校验级别
type ValidationLevel string

const (
	ErrorLevel   ValidationLevel = "error"
	WarningLevel ValidationLevel = "warning"
	InfoLevel    ValidationLevel = "info"
)

// ValidationIssue 校验问题
type ValidationIssue struct {
	Level     ValidationLevel `json:"level"`
	Message   string          `json:"message"`
	Location  string          `json:"location,omitempty"`
	Field     string          `json:"field,omitempty"`
	AutoFixed bool            `json:"auto_fixed,omitempty"`
}

// ValidationResult 校验结果
type ValidationResult struct {
	Valid       bool                   `json:"valid"`
	Issues      []ValidationIssue      `json:"issues"`
	FixedConfig map[string]interface{} `json:"fixed_config,omitempty"`
}

// ValidateClashConfig 校验Clash配置
func ValidateClashConfig(config map[string]interface{}) *ValidationResult {
	result := &ValidationResult{
		Valid:  true,
		Issues: []ValidationIssue{},
	}

	// 深拷贝配置
	fixedConfig := deepCopyMap(config)

	// 1. 校验proxies
	if proxies, ok := config["proxies"].([]interface{}); ok {
		proxyResult := validateProxies(proxies)
		result.Issues = append(result.Issues, proxyResult.Issues...)
		if proxyResult.Fixed != nil {
			fixedConfig["proxies"] = proxyResult.Fixed
		}
	}

	// 2. 校验proxy-groups
	proxies, _ := fixedConfig["proxies"].([]interface{})
	if groups, ok := config["proxy-groups"].([]interface{}); ok {
		groupResult := validateProxyGroups(groups, proxies)
		result.Issues = append(result.Issues, groupResult.Issues...)
		if groupResult.Fixed != nil {
			fixedConfig["proxy-groups"] = groupResult.Fixed
		}
	}

	// 3. 检测循环引用
	if groups, ok := fixedConfig["proxy-groups"].([]interface{}); ok {
		circularIssues := detectCircularReferences(groups)
		result.Issues = append(result.Issues, circularIssues...)
	}

	// 判断是否有错误
	for _, issue := range result.Issues {
		if issue.Level == ErrorLevel {
			result.Valid = false
			break
		}
	}

	// 如果有自动修复，返回修复后的配置
	hasAutoFix := false
	for _, issue := range result.Issues {
		if issue.AutoFixed {
			hasAutoFix = true
			break
		}
	}
	if hasAutoFix {
		result.FixedConfig = fixedConfig
	}

	return result
}

// ProxyValidationResult 代理节点校验结果
type ProxyValidationResult struct {
	Issues []ValidationIssue
	Fixed  []interface{}
}

// validateProxies 校验代理节点
func validateProxies(proxies []interface{}) *ProxyValidationResult {
	result := &ProxyValidationResult{
		Issues: []ValidationIssue{},
		Fixed:  []interface{}{},
	}

	seenNames := make(map[string]bool)

	for i, proxy := range proxies {
		proxyMap, ok := proxy.(map[string]interface{})
		if !ok {
			result.Issues = append(result.Issues, ValidationIssue{
				Level:    ErrorLevel,
				Message:  fmt.Sprintf("代理节点 #%d 不是有效的对象", i+1),
				Location: fmt.Sprintf("proxies[%d]", i),
			})
			continue
		}

		// 检查name字段
		name, ok := proxyMap["name"].(string)
		if !ok || strings.TrimSpace(name) == "" {
			result.Issues = append(result.Issues, ValidationIssue{
				Level:    ErrorLevel,
				Message:  fmt.Sprintf("代理节点 #%d 缺少name字段或name为空", i+1),
				Location: fmt.Sprintf("proxies[%d]", i),
				Field:    "name",
			})
			continue
		}

		name = strings.TrimSpace(name)

		// 检查name重复
		if seenNames[name] {
			result.Issues = append(result.Issues, ValidationIssue{
				Level:     WarningLevel,
				Message:   fmt.Sprintf("代理节点名称重复: \"%s\"，已自动移除", name),
				Location:  fmt.Sprintf("proxies[%d]", i),
				Field:     "name",
				AutoFixed: true,
			})
			continue
		}
		seenNames[name] = true

		// 检查name是否为第一个字段
		keys := getMapKeys(proxyMap)
		if len(keys) > 0 && keys[0] != "name" {
			result.Issues = append(result.Issues, ValidationIssue{
				Level:     WarningLevel,
				Message:   fmt.Sprintf("代理节点 \"%s\" 的name字段不是第一个字段，已自动调整", name),
				Location:  fmt.Sprintf("proxies[%d]", i),
				Field:     "name",
				AutoFixed: true,
			})
		}

		// 重新排序字段
		orderedProxy := reorderProxyFields(proxyMap)
		result.Fixed = append(result.Fixed, orderedProxy)
	}

	return result
}

// GroupValidationResult 代理组校验结果
type GroupValidationResult struct {
	Issues []ValidationIssue
	Fixed  []interface{}
}

// validateProxyGroups 校验代理组
func validateProxyGroups(groups []interface{}, proxies []interface{}) *GroupValidationResult {
	result := &GroupValidationResult{
		Issues: []ValidationIssue{},
		Fixed:  []interface{}{},
	}

	// 构建代理名称集合
	proxyNames := make(map[string]bool)
	for _, proxy := range proxies {
		if proxyMap, ok := proxy.(map[string]interface{}); ok {
			if name, ok := proxyMap["name"].(string); ok {
				proxyNames[name] = true
			}
		}
	}

	// 构建代理组名称集合
	groupNames := make(map[string]bool)
	for _, group := range groups {
		if groupMap, ok := group.(map[string]interface{}); ok {
			if name, ok := groupMap["name"].(string); ok {
				groupNames[name] = true
			}
		}
	}

	seenNames := make(map[string]bool)
	specialNodes := map[string]bool{
		"DIRECT":      true,
		"REJECT":      true,
		"REJECT-DROP": true,
		"PROXY":       true,
		"PASS":        true,
	}

	// 常见拼写错误修正
	spellingCorrections := map[string]string{
		"DIRCT":  "DIRECT",
		"REJET":  "REJECT",
		"REJCT":  "REJECT",
		"PROXXY": "PROXY",
	}

	for i, group := range groups {
		groupMap, ok := group.(map[string]interface{})
		if !ok {
			result.Issues = append(result.Issues, ValidationIssue{
				Level:    ErrorLevel,
				Message:  fmt.Sprintf("代理组 #%d 不是有效的对象", i+1),
				Location: fmt.Sprintf("proxy-groups[%d]", i),
			})
			continue
		}

		// 检查name字段
		name, ok := groupMap["name"].(string)
		if !ok || strings.TrimSpace(name) == "" {
			result.Issues = append(result.Issues, ValidationIssue{
				Level:    ErrorLevel,
				Message:  fmt.Sprintf("代理组 #%d 缺少name字段或name为空", i+1),
				Location: fmt.Sprintf("proxy-groups[%d]", i),
				Field:    "name",
			})
			continue
		}

		name = strings.TrimSpace(name)

		// 检查name重复
		if seenNames[name] {
			result.Issues = append(result.Issues, ValidationIssue{
				Level:    ErrorLevel,
				Message:  fmt.Sprintf("代理组名称重复: \"%s\"", name),
				Location: fmt.Sprintf("proxy-groups[%d]", i),
				Field:    "name",
			})
			continue
		}
		seenNames[name] = true

		// 检查name是否为第一个字段
		keys := getMapKeys(groupMap)
		if len(keys) > 0 && keys[0] != "name" {
			result.Issues = append(result.Issues, ValidationIssue{
				Level:     WarningLevel,
				Message:   fmt.Sprintf("代理组 \"%s\" 的name字段不是第一个字段，已自动调整", name),
				Location:  fmt.Sprintf("proxy-groups[%d]", i),
				Field:     "name",
				AutoFixed: true,
			})
		}

		// 检查proxies、use、filter和include-all字段
		groupProxies, hasProxies := groupMap["proxies"].([]interface{})
		groupUse, hasUse := groupMap["use"].([]interface{})
		groupFilter, hasFilter := groupMap["filter"].(string)
		groupIncludeAll, hasIncludeAll := groupMap["include-all"].(bool)

		hasValidProxies := hasProxies && len(groupProxies) > 0
		hasValidUse := hasUse && len(groupUse) > 0
		hasValidFilter := hasFilter && strings.TrimSpace(groupFilter) != ""
		hasValidIncludeAll := hasIncludeAll && groupIncludeAll

		if !hasValidProxies && !hasValidUse && !hasValidFilter && !hasValidIncludeAll {
			result.Issues = append(result.Issues, ValidationIssue{
				Level:    ErrorLevel,
				Message:  fmt.Sprintf("代理组 \"%s\" 的proxies、use、filter和include-all字段都为空或不存在", name),
				Location: fmt.Sprintf("proxy-groups[%d]", i),
				Field:    "proxies",
			})
			continue
		}

		// 校验proxies引用
		if hasValidProxies {
			validProxies := []interface{}{}
			seenProxies := make(map[string]bool)
			hasDuplicates := false

			for _, proxyRef := range groupProxies {
				proxyName, ok := proxyRef.(string)
				if !ok {
					continue
				}

				// 检查重复
				if seenProxies[proxyName] {
					hasDuplicates = true
					continue
				}

				// 修正拼写错误
				correctedName := proxyName
				if corrected, ok := spellingCorrections[proxyName]; ok {
					correctedName = corrected
					result.Issues = append(result.Issues, ValidationIssue{
						Level:     WarningLevel,
						Message:   fmt.Sprintf("代理组 \"%s\" 中的节点引用 \"%s\" 已自动修正为 \"%s\"", name, proxyName, correctedName),
						Location:  fmt.Sprintf("proxy-groups[%d]", i),
						Field:     "proxies",
						AutoFixed: true,
					})
				}

				// 检查节点是否存在
				isSpecial := specialNodes[correctedName]
				isProxy := proxyNames[correctedName]
				isGroup := groupNames[correctedName] && correctedName != name

				if !isSpecial && !isProxy && !isGroup {
					result.Issues = append(result.Issues, ValidationIssue{
						Level:    ErrorLevel,
						Message:  fmt.Sprintf("代理组 \"%s\" 引用了不存在的节点: \"%s\"", name, correctedName),
						Location: fmt.Sprintf("proxy-groups[%d]", i),
						Field:    "proxies",
					})
					continue
				}

				seenProxies[correctedName] = true
				validProxies = append(validProxies, correctedName)
			}

			if hasDuplicates {
				result.Issues = append(result.Issues, ValidationIssue{
					Level:     WarningLevel,
					Message:   fmt.Sprintf("代理组 \"%s\" 的proxies字段包含重复引用，已自动去重", name),
					Location:  fmt.Sprintf("proxy-groups[%d]", i),
					Field:     "proxies",
					AutoFixed: true,
				})
			}

			groupMap["proxies"] = validProxies
		}

		// 重新排序字段
		orderedGroup := reorderGroupFields(groupMap)
		result.Fixed = append(result.Fixed, orderedGroup)
	}

	return result
}

// detectCircularReferences 检测循环引用
func detectCircularReferences(groups []interface{}) []ValidationIssue {
	issues := []ValidationIssue{}

	// 构建引用图
	graph := make(map[string][]string)
	for _, group := range groups {
		if groupMap, ok := group.(map[string]interface{}); ok {
			name, ok1 := groupMap["name"].(string)
			proxies, ok2 := groupMap["proxies"].([]interface{})
			if !ok1 || !ok2 {
				continue
			}

			refs := []string{}
			for _, proxy := range proxies {
				if proxyName, ok := proxy.(string); ok {
					// 只记录对其他代理组的引用
					for _, g := range groups {
						if gMap, ok := g.(map[string]interface{}); ok {
							if gName, ok := gMap["name"].(string); ok && gName == proxyName {
								refs = append(refs, proxyName)
								break
							}
						}
					}
				}
			}
			graph[name] = refs
		}
	}

	// DFS检测循环
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(node string, path []string) bool
	dfs = func(node string, path []string) bool {
		visited[node] = true
		recStack[node] = true
		path = append(path, node)

		for _, neighbor := range graph[node] {
			if !visited[neighbor] {
				if dfs(neighbor, path) {
					return true
				}
			} else if recStack[neighbor] {
				// 找到循环
				cycleStart := -1
				for i, p := range path {
					if p == neighbor {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := append(path[cycleStart:], neighbor)
					issues = append(issues, ValidationIssue{
						Level:    ErrorLevel,
						Message:  fmt.Sprintf("检测到代理组循环引用: %s", strings.Join(cycle, " → ")),
						Location: fmt.Sprintf("proxy-groups[%s]", node),
					})
				}
				return true
			}
		}

		recStack[node] = false
		return false
	}

	for node := range graph {
		if !visited[node] {
			dfs(node, []string{})
		}
	}

	return issues
}

// FormatValidationIssues 格式化校验结果为用户友好的消息
func FormatValidationIssues(issues []ValidationIssue) string {
	if len(issues) == 0 {
		return "✅ 配置校验通过"
	}

	var errors []ValidationIssue
	var warnings []ValidationIssue
	var autoFixed []ValidationIssue

	for _, issue := range issues {
		switch issue.Level {
		case ErrorLevel:
			errors = append(errors, issue)
		case WarningLevel:
			warnings = append(warnings, issue)
		}
		if issue.AutoFixed {
			autoFixed = append(autoFixed, issue)
		}
	}

	var message strings.Builder

	if len(errors) > 0 {
		message.WriteString(fmt.Sprintf("❌ 发现 %d 个错误:\n", len(errors)))
		for i, issue := range errors {
			message.WriteString(fmt.Sprintf("  %d. %s\n", i+1, issue.Message))
			if issue.Location != "" {
				message.WriteString(fmt.Sprintf("     位置: %s\n", issue.Location))
			}
		}
	}

	if len(warnings) > 0 {
		if message.Len() > 0 {
			message.WriteString("\n")
		}
		message.WriteString(fmt.Sprintf("⚠️ 发现 %d 个警告:\n", len(warnings)))
		for i, issue := range warnings {
			message.WriteString(fmt.Sprintf("  %d. %s\n", i+1, issue.Message))
		}
	}

	if len(autoFixed) > 0 {
		if message.Len() > 0 {
			message.WriteString("\n")
		}
		message.WriteString(fmt.Sprintf("🔧 已自动修复 %d 个问题", len(autoFixed)))
	}

	return message.String()
}

// 辅助函数

func deepCopyMap(src map[string]interface{}) map[string]interface{} {
	// 使用JSON序列化/反序列化进行深拷贝
	data, err := json.Marshal(src)
	if err != nil {
		return src
	}
	var dst map[string]interface{}
	if err := json.Unmarshal(data, &dst); err != nil {
		return src
	}
	return dst
}

func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	// 注意：Go的map遍历是无序的，这里需要特殊处理
	// 实际应该保留原始顺序，这里简化处理
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func reorderProxyFields(proxy map[string]interface{}) map[string]interface{} {
	ordered := make(map[string]interface{})
	priorityKeys := []string{"name", "type", "server", "port"}

	// 先添加优先字段
	for _, key := range priorityKeys {
		if val, ok := proxy[key]; ok {
			ordered[key] = val
		}
	}

	// 再添加其他字段
	for key, val := range proxy {
		if !contains(priorityKeys, key) {
			ordered[key] = val
		}
	}

	return ordered
}

func reorderGroupFields(group map[string]interface{}) map[string]interface{} {
	ordered := make(map[string]interface{})
	priorityKeys := []string{"name", "type", "proxies", "use", "url", "interval", "strategy", "lazy", "hidden"}

	// 先添加优先字段
	for _, key := range priorityKeys {
		if val, ok := group[key]; ok {
			ordered[key] = val
		}
	}

	// 再添加其他字段
	for key, val := range group {
		if !contains(priorityKeys, key) {
			ordered[key] = val
		}
	}

	return ordered
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
