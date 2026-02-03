import { load as parseYAML, dump as dumpYAML } from 'js-yaml'

// Proxy types supported by mihomo/clash
export const PROXY_TYPES = [
  'ss', 'ssr', 'vmess', 'vless', 'trojan', 'hysteria', 'hysteria2',
  'tuic', 'wireguard', 'socks5', 'http', 'snell', 'anytls', 'ssh'
] as const

export type ProxyType = typeof PROXY_TYPES[number]

// Proxy group types
export const PROXY_GROUP_TYPES = [
  'select', 'url-test', 'fallback', 'load-balance', 'relay'
] as const

export type ProxyGroupType = typeof PROXY_GROUP_TYPES[number]

// V3 Proxy Group configuration (matches backend ProxyGroupV3)
export interface ProxyGroupV3Config {
  name: string
  type: ProxyGroupType
  proxies?: string[]
  use?: string[]
  'include-all'?: boolean
  'include-all-proxies'?: boolean
  'include-all-providers'?: boolean
  'include-type'?: string
  'exclude-type'?: string
  filter?: string
  'exclude-filter'?: string
  url?: string
  interval?: number
  tolerance?: number
  lazy?: boolean
  'disable-udp'?: boolean
  strategy?: string
  'interface-name'?: string
  'routing-mark'?: number
}

// Parsed template structure
export interface ParsedTemplate {
  port?: number
  'socks-port'?: number
  'allow-lan'?: boolean
  mode?: string
  'log-level'?: string
  'external-controller'?: string
  dns?: Record<string, unknown>
  proxies?: Array<Record<string, unknown>>
  'proxy-groups'?: ProxyGroupV3Config[]
  rules?: string[]
  'rule-providers'?: Record<string, unknown>
}

// Form state for proxy group editor
export interface ProxyGroupFormState {
  name: string
  type: ProxyGroupType
  filterKeywords: string
  excludeFilterKeywords: string
  includeTypes: ProxyType[]
  excludeTypes: ProxyType[]
  includeAll: boolean
  includeAllProxies: boolean
  includeAllProviders: boolean
  staticProxies: string[]
  url: string
  interval: number
  tolerance: number
}

// Convert comma-separated keywords to regex pattern
export function keywordsToRegex(keywords: string): string {
  if (!keywords.trim()) return ''
  return keywords
    .split(/[,，]/)
    .map(k => k.trim())
    .filter(k => k.length > 0)
    .join('|')
}

// Convert regex pattern back to keywords (best effort)
export function regexToKeywords(regex: string): string {
  if (!regex) return ''
  return regex.split('|').join(', ')
}

// Create default form state for a new proxy group
export function createDefaultFormState(name = '新代理组'): ProxyGroupFormState {
  return {
    name,
    type: 'select',
    filterKeywords: '',
    excludeFilterKeywords: '',
    includeTypes: [],
    excludeTypes: [],
    includeAll: false,
    includeAllProxies: false,
    includeAllProviders: false,
    staticProxies: [],
    url: 'https://www.gstatic.com/generate_204',
    interval: 300,
    tolerance: 50,
  }
}

// Convert ProxyGroupFormState to ProxyGroupV3Config
export function formStateToConfig(state: ProxyGroupFormState): ProxyGroupV3Config {
  const config: ProxyGroupV3Config = {
    name: state.name,
    type: state.type,
  }

  // Include options
  if (state.includeAll) config['include-all'] = true
  if (state.includeAllProxies) config['include-all-proxies'] = true
  if (state.includeAllProviders) config['include-all-providers'] = true

  // Filter patterns
  const filter = keywordsToRegex(state.filterKeywords)
  if (filter) config.filter = filter

  const excludeFilter = keywordsToRegex(state.excludeFilterKeywords)
  if (excludeFilter) config['exclude-filter'] = excludeFilter

  // Type filters
  if (state.includeTypes.length > 0) {
    config['include-type'] = state.includeTypes.join('|')
  }

  if (state.excludeTypes.length > 0) {
    config['exclude-type'] = state.excludeTypes.join('|')
  }

  // Static proxies
  if (state.staticProxies.length > 0) {
    config.proxies = state.staticProxies
  }

  // URL test options
  if (state.type === 'url-test' || state.type === 'fallback' || state.type === 'load-balance') {
    if (state.url) config.url = state.url
    if (state.interval) config.interval = state.interval
    if (state.tolerance && state.type !== 'load-balance') config.tolerance = state.tolerance
  }

  return config
}

// Convert ProxyGroupV3Config to ProxyGroupFormState
export function configToFormState(config: ProxyGroupV3Config): ProxyGroupFormState {
  return {
    name: config.name,
    type: config.type,
    filterKeywords: regexToKeywords(config.filter || ''),
    excludeFilterKeywords: regexToKeywords(config['exclude-filter'] || ''),
    includeTypes: (config['include-type']?.split('|').filter(t => PROXY_TYPES.includes(t as ProxyType)) || []) as ProxyType[],
    excludeTypes: (config['exclude-type']?.split('|').filter(t => PROXY_TYPES.includes(t as ProxyType)) || []) as ProxyType[],
    includeAll: config['include-all'] || false,
    includeAllProxies: config['include-all-proxies'] || false,
    includeAllProviders: config['include-all-providers'] || false,
    staticProxies: config.proxies || [],
    url: config.url || 'https://www.gstatic.com/generate_204',
    interval: config.interval || 300,
    tolerance: config.tolerance || 50,
  }
}

// Parse YAML template to structured object
export function parseTemplate(content: string): ParsedTemplate | null {
  try {
    return parseYAML(content) as ParsedTemplate
  } catch {
    return null
  }
}

// Serialize structured object back to YAML
export function serializeTemplate(template: ParsedTemplate): string {
  return dumpYAML(template, { indent: 2, lineWidth: -1, noRefs: true })
}

// Extract proxy groups from template content
export function extractProxyGroups(content: string): ProxyGroupFormState[] {
  const template = parseTemplate(content)
  if (!template || !template['proxy-groups']) return []
  return template['proxy-groups'].map(configToFormState)
}

// Update proxy-groups in template content
export function updateProxyGroups(content: string, groups: ProxyGroupFormState[]): string {
  const template = parseTemplate(content)
  if (!template) return content

  template['proxy-groups'] = groups.map(formStateToConfig)
  return serializeTemplate(template)
}

// Generate proxy-groups YAML preview from form states
export function generateProxyGroupsPreview(groups: ProxyGroupFormState[]): string {
  const configs = groups.map(formStateToConfig)
  return dumpYAML({ 'proxy-groups': configs }, { indent: 2, lineWidth: -1, noRefs: true })
}

// Create a blank v3 template
export function createBlankTemplate(): string {
  const template: ParsedTemplate = {
    port: 7890,
    'socks-port': 7891,
    'allow-lan': true,
    mode: 'rule',
    'log-level': 'info',
    'external-controller': '127.0.0.1:9090',
    dns: {
      enable: true,
      ipv6: true,
      'enhanced-mode': 'fake-ip',
      nameserver: [
        'https://223.5.5.5/dns-query',
        'https://120.53.53.53/dns-query',
      ],
    },
    proxies: [],
    'proxy-groups': [
      {
        name: '🚀 节点选择',
        type: 'select',
        'include-all-proxies': true,
      },
      {
        name: '♻️ 自动选择',
        type: 'url-test',
        'include-all-proxies': true,
        url: 'https://www.gstatic.com/generate_204',
        interval: 300,
        tolerance: 50,
      },
      {
        name: '🎯 全球直连',
        type: 'select',
        proxies: ['DIRECT'],
      },
    ],
    rules: [
      'GEOSITE,private,🎯 全球直连',
      'GEOIP,private,🎯 全球直连,no-resolve',
      'GEOSITE,cn,🎯 全球直连',
      'GEOIP,cn,🎯 全球直连,no-resolve',
      'MATCH,🚀 节点选择',
    ],
  }
  return serializeTemplate(template)
}
