import type { ProxyConfig, CustomRule } from './types'
import { deepCopy } from './utils'
import { DEFAULT_CLASH_CONFIG, CLASH_SITE_RULE_SET_BASE_URL, CLASH_IP_RULE_SET_BASE_URL } from './clash-config'
import { RULE_CATEGORIES } from './predefined-rules'
import { translateOutbound, CATEGORY_TO_RULE_NAME } from './translations'
// import { ProxyNode } from '../proxy-parser'

export class ClashConfigBuilder {
  private proxies: ProxyConfig[] = []
  private config: Record<string, unknown>

  constructor(
    private proxyConfigs: ProxyConfig[],
    private selectedCategories: string[] = [],
    private customRules: CustomRule[] = []
  ) {
    this.config = deepCopy(DEFAULT_CLASH_CONFIG)
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
    const siteRules = new Set<string>()
    const ipRules = new Set<string>()

    // Collect rules from selected categories
    for (const categoryName of this.selectedCategories) {
      const category = RULE_CATEGORIES.find((c) => c.name === categoryName)
      if (!category) continue

      category.site_rules.forEach((rule) => siteRules.add(rule))
      category.ip_rules.forEach((rule) => ipRules.add(rule))
    }

    // Build site rule providers
    siteRules.forEach((rule) => {
      ruleProviders[rule] = {
        type: 'http',
        format: 'mrs',
        behavior: 'domain',
        url: `${CLASH_SITE_RULE_SET_BASE_URL}${rule}.mrs`,
        path: `./ruleset/${rule}.mrs`,
        interval: 86400,
      }
    })

    // Build IP rule providers
    ipRules.forEach((rule) => {
      ruleProviders[rule] = {
        type: 'http',
        format: 'mrs',
        behavior: 'ipcidr',
        url: `${CLASH_IP_RULE_SET_BASE_URL}${rule}.mrs`,
        path: `./ruleset/${rule}.mrs`,
        interval: 86400,
      }
    })

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
      proxies: [...proxyNames],
      url: 'https://www.gstatic.com/generate_204',
      interval: 300,
      lazy: false,
    })

    // 3. Category-specific groups
    for (const categoryName of this.selectedCategories) {
      const ruleName = CATEGORY_TO_RULE_NAME[categoryName]
      if (!ruleName) continue

      groups.push({
        name: translateOutbound(ruleName),
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
      const category = RULE_CATEGORIES.find((c) => c.name === categoryName)
      if (!category) continue

      const ruleName = CATEGORY_TO_RULE_NAME[categoryName]
      if (!ruleName) continue

      const outbound = translateOutbound(ruleName)

      // Site rules
      for (const siteRule of category.site_rules) {
        rules.push(`RULE-SET,${siteRule},${outbound}`)
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
      const category = RULE_CATEGORIES.find((c) => c.name === categoryName)
      if (!category) continue

      const ruleName = CATEGORY_TO_RULE_NAME[categoryName]
      if (!ruleName) continue

      const outbound = translateOutbound(ruleName)

      // IP rules
      for (const ipRule of category.ip_rules) {
        rules.push(`RULE-SET,${ipRule},${outbound},no-resolve`)
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
      }
      return value
    }
    return String(value)
  }
}
