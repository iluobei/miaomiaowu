import type { ProxyConfig, CustomRule, ProxyGroupCategory } from './types'
import { deepCopy } from './utils'
import { DEFAULT_CLASH_CONFIG, CLASH_SITE_RULE_SET_BASE_URL, CLASH_IP_RULE_SET_BASE_URL } from './clash-config'
import { RULE_CATEGORIES } from './predefined-rules'
import { translateOutbound, CATEGORY_TO_RULE_NAME } from './translations'
// import { ProxyNode } from '../proxy-parser'

export class ClashConfigBuilder {
  private proxies: ProxyConfig[] = []
  private config: Record<string, unknown>
  private categoryMap: Map<string, ProxyGroupCategory>

  constructor(
    private proxyConfigs: ProxyConfig[],
    private selectedCategories: string[] = [],
    private customRules: CustomRule[] = [],
    ruleCategories?: ProxyGroupCategory[]
  ) {
    this.config = deepCopy(DEFAULT_CLASH_CONFIG)

    // Use provided categories or fall back to static RULE_CATEGORIES
    const categories = ruleCategories || this.convertLegacyCategories()
    this.categoryMap = new Map(categories.map((c) => [c.name, c]))
  }

  /**
   * Convert legacy RULE_CATEGORIES to new ProxyGroupCategory format
   * Used as fallback when ruleCategories is not provided
   */
  private convertLegacyCategories(): ProxyGroupCategory[] {
    return RULE_CATEGORIES.map((category) => ({
      name: category.name,
      label: category.label,
      emoji: category.icon,
      icon: category.icon,
      rule_name: CATEGORY_TO_RULE_NAME[category.name] || category.name,
      group_label: translateOutbound(CATEGORY_TO_RULE_NAME[category.name] || category.name),
      presets: [], // Legacy doesn't have preset info
      site_rules: category.site_rules.map((rule) => ({
        key: rule,
        behavior: 'domain',
        type: 'http',
        format: 'mrs',
        url: `${CLASH_SITE_RULE_SET_BASE_URL}${rule}.mrs`,
        path: `./ruleset/${rule}.mrs`,
        interval: 86400,
      })),
      ip_rules: category.ip_rules.map((rule) => ({
        key: rule,
        behavior: 'ipcidr',
        type: 'http',
        format: 'mrs',
        url: `${CLASH_IP_RULE_SET_BASE_URL}${rule}.mrs`,
        path: `./ruleset/${rule}.mrs`,
        interval: 86400,
      })),
    }))
  }

  build(): string {
    this.convertProxies()
    this.buildRuleProviders()
    this.buildProxyGroups()
    this.buildRules()

    // 重新排序 config，确保 rule-providers 在最后
    const orderedConfig: Record<string, unknown> = {}
    const ruleProviders = this.config['rule-providers']

    // 先添加除 rule-providers 外的所有字段
    for (const [key, value] of Object.entries(this.config)) {
      if (key !== 'rule-providers') {
        orderedConfig[key] = value
      }
    }

    // 最后添加 rule-providers
    if (ruleProviders) {
      orderedConfig['rule-providers'] = ruleProviders
    }

    // Convert to YAML
    return this.toYAML(orderedConfig)
  }

  private convertProxies(): void {
    // 重新排序代理节点的字段：name, type, server, port 在最前面
    this.config.proxies = this.proxyConfigs.map(proxy => this.reorderProxyFields(proxy))
    this.proxies = this.proxyConfigs
  }

  // 重新排序代理节点字段，将 name, type, server, port 放在最前面
  private reorderProxyFields(proxy: ProxyConfig): ProxyConfig {
    const ordered: any = {}
    const priorityKeys = ['name', 'type', 'server', 'port']

    // 先添加优先字段
    for (const key of priorityKeys) {
      if (key in proxy) {
        ordered[key] = (proxy as any)[key]
      }
    }

    // 再添加其他字段
    for (const [key, value] of Object.entries(proxy)) {
      if (!priorityKeys.includes(key)) {
        ordered[key] = value
      }
    }

    return ordered as ProxyConfig
  }
  private buildRuleProviders(): void {
    const ruleProviders: Record<string, unknown> = {}

    // Collect providers from selected categories
    for (const categoryName of this.selectedCategories) {
      const category = this.categoryMap.get(categoryName)
      if (!category) continue

      // Add site rule providers
      for (const provider of category.site_rules) {
        if (ruleProviders[provider.key]) {
          console.warn(`Duplicate rule provider key: ${provider.key}`)
          continue
        }
        ruleProviders[provider.key] = {
          type: provider.type,
          format: provider.format,
          behavior: provider.behavior,
          url: provider.url,
          path: provider.path,
          interval: provider.interval,
        }
      }

      // Add IP rule providers
      for (const provider of category.ip_rules) {
        if (ruleProviders[provider.key]) {
          console.warn(`Duplicate rule provider key: ${provider.key}`)
          continue
        }
        ruleProviders[provider.key] = {
          type: provider.type,
          format: provider.format,
          behavior: provider.behavior,
          url: provider.url,
          path: provider.path,
          interval: provider.interval,
        }
      }
    }

    this.config['rule-providers'] = ruleProviders
  }
  
  public buildProxyGroups(): void {
    const proxyNames = this.proxies.map((p) => p.name)
    const groups: Record<string, unknown>[] = []

    // 1. Node Select group
    groups.push({
      name: translateOutbound('Node Select'),
      type: 'select',
      proxies: ['DIRECT', 'REJECT', translateOutbound('Auto Select'), ...proxyNames],
    })

    // 2. Auto Select group
    groups.push({
      name: translateOutbound('Auto Select'),
      type: 'url-test',
      url: 'https://www.gstatic.com/generate_204',
      interval: 300,
      lazy: false,
      proxies: [...proxyNames],
    })

    // 3. Category-specific groups
    for (const categoryName of this.selectedCategories) {
      const category = this.categoryMap.get(categoryName)
      if (!category) continue

      // Skip categories without rule_name (they won't have corresponding proxy groups)
      if (!category.rule_name) continue

      // Use group_label from category, fallback to translated rule_name
      const groupName = category.group_label || translateOutbound(category.rule_name)

      groups.push({
        name: groupName,
        type: 'select',
        proxies: [
          translateOutbound('Node Select'),
          'DIRECT',
          'REJECT',
          translateOutbound('Auto Select'),
          ...proxyNames,
        ],
      })
    }

    // 4. Custom rule groups
    for (const rule of this.customRules) {
      if (!rule.name) continue

      groups.push({
        name: translateOutbound(rule.name),
        type: 'select',
        proxies: [
          translateOutbound('Node Select'),
          'DIRECT',
          'REJECT',
          translateOutbound('Auto Select'),
          ...proxyNames,
        ],
      })
    }

    // 5. Fall Back group
    groups.push({
      name: translateOutbound('Fall Back'),
      type: 'select',
      proxies: [
        translateOutbound('Node Select'),
        'DIRECT',
        'REJECT',
        translateOutbound('Auto Select'),
        ...proxyNames,
      ],
    })

    this.config['proxy-groups'] = groups
  }

  private buildRules(): void {
    const rules: string[] = []

    // Custom rules first (domain-based)
    for (const rule of this.customRules) {
      if (!rule.name) continue

      const outbound = translateOutbound(rule.name)

      if (rule.domain_suffix) {
        rule.domain_suffix.split(',').forEach((domain) => {
          const trimmed = domain.trim()
          if (trimmed) rules.push(`DOMAIN-SUFFIX,${trimmed},${outbound}`)
        })
      }

      if (rule.domain_keyword) {
        rule.domain_keyword.split(',').forEach((keyword) => {
          const trimmed = keyword.trim()
          if (trimmed) rules.push(`DOMAIN-KEYWORD,${trimmed},${outbound}`)
        })
      }
    }

    // Category rules (RULE-SET format)
    for (const categoryName of this.selectedCategories) {
      const category = this.categoryMap.get(categoryName)
      if (!category) continue

      // Skip categories without rule_name
      if (!category.rule_name) continue

      // Use group_label for outbound, fallback to translated rule_name
      const outbound = category.group_label || translateOutbound(category.rule_name)

      // Site rules - use provider key from configuration
      for (const provider of category.site_rules) {
        rules.push(`RULE-SET,${provider.key},${outbound}`)
      }
    }

    // Custom rules (IP-based) after site rules
    for (const rule of this.customRules) {
      if (!rule.name) continue

      const outbound = translateOutbound(rule.name)

      if (rule.ip_cidr) {
        rule.ip_cidr.split(',').forEach((cidr) => {
          const trimmed = cidr.trim()
          if (trimmed) rules.push(`IP-CIDR,${trimmed},${outbound},no-resolve`)
        })
      }
    }

    // Category IP rules
    for (const categoryName of this.selectedCategories) {
      const category = this.categoryMap.get(categoryName)
      if (!category) continue

      // Skip categories without rule_name
      if (!category.rule_name) continue

      // Use group_label for outbound, fallback to translated rule_name
      const outbound = category.group_label || translateOutbound(category.rule_name)

      // IP rules - use provider key from configuration
      for (const provider of category.ip_rules) {
        rules.push(`RULE-SET,${provider.key},${outbound},no-resolve`)
      }
    }

    // Final MATCH rule
    rules.push(`MATCH,${translateOutbound('Fall Back')}`)

    this.config.rules = rules
  }

  private toYAML(obj: unknown, indent: number = 0): string {
    const spaces = '  '.repeat(indent)
    let yaml = ''

    if (Array.isArray(obj)) {
      for (const item of obj) {
        if (typeof item === 'object' && item !== null) {
          const entries = Object.entries(item).filter(([_, v]) => v !== undefined)
          if (entries.length > 0) {
            const [firstKey, firstValue] = entries[0]
            const restEntries = entries.slice(1)

            if (Array.isArray(firstValue)) {
              yaml += `${spaces}- ${firstKey}:\n${this.toYAML(firstValue, indent + 2)}`
            } else if (typeof firstValue === 'object' && firstValue !== null) {
              yaml += `${spaces}- ${firstKey}:\n${this.toYAML(firstValue, indent + 2)}`
            } else {
              yaml += `${spaces}- ${firstKey}: ${this.formatValue(firstValue, firstKey)}\n`
            }

            for (const [key, value] of restEntries) {
              if (Array.isArray(value)) {
                yaml += `${spaces}  ${key}:\n${this.toYAML(value, indent + 2)}`
              } else if (typeof value === 'object' && value !== null) {
                yaml += `${spaces}  ${key}:\n${this.toYAML(value, indent + 2)}`
              } else {
                yaml += `${spaces}  ${key}: ${this.formatValue(value, key)}\n`
              }
            }
          }
        } else {
          yaml += `${spaces}- ${this.formatValue(item)}\n`
        }
      }
    } else if (typeof obj === 'object' && obj !== null) {
      for (const [key, value] of Object.entries(obj)) {
        if (value === undefined) continue

        if (Array.isArray(value)) {
          yaml += `${spaces}${key}:\n${this.toYAML(value, indent + 1)}`
        } else if (typeof value === 'object' && value !== null) {
          yaml += `${spaces}${key}:\n${this.toYAML(value, indent + 1)}`
        } else {
          yaml += `${spaces}${key}: ${this.formatValue(value, key)}\n`
        }
      }
    }

    return yaml
  }

  private formatValue(value: unknown, key?: string): string {
    if (typeof value === 'string') {
      // 空字符串或 short-id 字段强制使用引号
      if (value === '' || key === 'short-id') {
        return `"${value}"`
      }
      if (
        value.includes(':') ||
        value.includes('#') ||
        value.includes('[') ||
        value.includes(']') ||
        value.includes(',')
      ) {
        return `"${value}"`
      } else if (value.startsWith('@')) {
        return `"${value}"`
      }
      return value
    }
    return String(value)
  }
}
