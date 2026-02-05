import { load as parseYAML, dump as dumpYAML } from 'js-yaml'

// Predefined region proxy groups with their comprehensive filter patterns
export const REGION_PROXY_GROUPS = [
  { name: '🇭🇰 香港节点', filter: '🇭🇰|港|\\bHK(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|hk|Hong Kong|HongKong|hongkong|HONG KONG|HONGKONG|深港|HKG|九龙|Kowloon|新界|沙田|荃湾|葵涌' },
  { name: '🇺🇸 美国节点', filter: '🇺🇸|美|波特兰|达拉斯|俄勒冈|凤凰城|费利蒙|硅谷|拉斯维加斯|洛杉矶|圣何塞|圣克拉拉|西雅图|芝加哥|纽约|纽纽|亚特兰大|迈阿密|华盛顿|\\bUS(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|United States|UnitedStates|UNITED STATES|USA|America|AMERICA|JFK|EWR|IAD|ATL|ORD|MIA|NYC|LAX|SFO|SEA|DFW|SJC' },
  { name: '🇯🇵 日本节点', filter: '🇯🇵|日本|川日|东京|大阪|泉日|埼玉|沪日|深日|(?<!尼|-)日|\\bJP(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|Japan|JAPAN|JPN|NRT|HND|KIX|TYO|OSA|关西|Kansai|KANSAI' },
  { name: '🇸🇬 新加坡节点', filter: '🇸🇬|新加坡|坡|狮城|\\bSG(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|Singapore|SINGAPORE|SIN' },
  { name: '🇼🇸 台湾节点', filter: '🇹🇼|🇼🇸|台|新北|彰化|\\bTW(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|Taiwan|TAIWAN|TWN|TPE|ROC' },
  { name: '🇰🇷 韩国节点', filter: '🇰🇷|\\bKR(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|Korea|KOREA|KOR|首尔|韩|韓|春川|Chuncheon|ICN' },
  { name: '🇨🇦 加拿大节点', filter: '🇨🇦|加拿大|\\bCA(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|Canada|CANADA|CAN|渥太华|温哥华|卡尔加里|蒙特利尔|Montreal|YVR|YYZ|YUL' },
  { name: '🇬🇧 英国节点', filter: '🇬🇧|英国|Britain|United Kingdom|UNITED KINGDOM|England|伦敦|曼彻斯特|Manchester|\\bUK(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|GBR|LHR|MAN' },
  { name: '🇫🇷 法国节点', filter: '🇫🇷|法国|\\bFR(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|France|FRANCE|FRA|巴黎|马赛|Marseille|CDG|MRS' },
  { name: '🇩🇪 德国节点', filter: '🇩🇪|德国|Germany|GERMANY|\\bDE(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|DEU|柏林|法兰克福|慕尼黑|Munich|MUC' },
  { name: '🇳🇱 荷兰节点', filter: '🇳🇱|荷兰|Netherlands|NETHERLANDS|\\bNL(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|NLD|阿姆斯特丹|AMS' },
  { name: '🇹🇷 土耳其节点', filter: '🇹🇷|土耳其|Turkey|TURKEY|Türkiye|\\bTR(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|TUR|IST|ANK' },
] as const

// Comprehensive exclude filter for "Other regions" group
export const OTHER_REGIONS_EXCLUDE_FILTER = '(^(?!.*(港|\\bHK(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|hk|Hong Kong|HongKong|hongkong|HONG KONG|HONGKONG|深港|HKG|🇭🇰|九龙|Kowloon|新界|沙田|荃湾|葵涌|美|波特兰|达拉斯|俄勒冈|凤凰城|费利蒙|硅谷|拉斯维加斯|洛杉矶|圣何塞|圣克拉拉|西雅图|芝加哥|纽约|纽纽|亚特兰大|迈阿密|华盛顿|\\bUS(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|United States|UnitedStates|UNITED STATES|USA|America|AMERICA|JFK|EWR|IAD|ATL|ORD|MIA|NYC|LAX|SFO|SEA|DFW|SJC|🇺🇸|日本|川日|东京|大阪|泉日|埼玉|沪日|深日|(?<!尼|-)日|\\bJP(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|Japan|JAPAN|JPN|NRT|HND|KIX|TYO|OSA|🇯🇵|关西|Kansai|KANSAI|新加坡|坡|狮城|\\bSG(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|Singapore|SINGAPORE|SIN|🇸🇬|台|新北|彰化|\\bTW(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|Taiwan|TAIWAN|TWN|TPE|ROC|🇹🇼|\\bKR(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|Korea|KOREA|KOR|首尔|韩|韓|春川|Chuncheon|ICN|🇰🇷|加拿大|\\bCA(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|Canada|CANADA|CAN|渥太华|温哥华|卡尔加里|蒙特利尔|Montreal|YVR|YYZ|YUL|🇨🇦|英国|Britain|United Kingdom|UNITED KINGDOM|England|伦敦|曼彻斯特|Manchester|\\bUK(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|GBR|LHR|MAN|🇬🇧|法国|\\bFR(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|France|FRANCE|FRA|巴黎|马赛|Marseille|CDG|MRS|🇫🇷|德国|Germany|GERMANY|\\bDE(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|DEU|柏林|法兰克福|慕尼黑|Munich|MUC|🇩🇪|荷兰|Netherlands|NETHERLANDS|\\bNL(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|NLD|阿姆斯特丹|AMS|🇳🇱|土耳其|Turkey|TURKEY|Türkiye|\\bTR(?:[-_ ]?\\d+(?:[-_ ]?[A-Za-z]{2,})?)?\\b|TUR|IST|ANK|🇹🇷)).*)'

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

// 代理节点、代理集合、区域代理组占位符
export const PROXY_NODES_MARKER = '__PROXY_NODES__'
export const PROXY_PROVIDERS_MARKER = '__PROXY_PROVIDERS__'
export const REGION_PROXY_GROUPS_MARKER = '__REGION_PROXY_GROUPS__'

// Type for proxy order item
export type ProxyOrderItem = string // Can be group name, PROXY_NODES_MARKER, PROXY_PROVIDERS_MARKER, or REGION_PROXY_GROUPS_MARKER

// V3 Proxy Group configuration (matches backend ProxyGroupV3)
export interface ProxyGroupV3Config {
  name: string
  type: ProxyGroupType
  proxies?: string[]
  use?: string[]
  'include-all'?: boolean
  'include-all-proxies'?: boolean
  'include-all-providers'?: boolean
  'include-region-proxy-groups'?: boolean
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
  includeRegionProxyGroups: boolean
  includedProxyGroups: string[]
  proxyOrder: ProxyOrderItem[] // Order of proxy groups, nodes marker, providers marker
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
    includeRegionProxyGroups: false,
    includedProxyGroups: [],
    proxyOrder: [],
    staticProxies: [],
    url: 'https://www.gstatic.com/generate_204',
    interval: 300,
    tolerance: 50,
  }
}

// Check if proxy nodes should be shown (has filter/include-all-proxies/include-type)
export function hasProxyNodes(state: ProxyGroupFormState): boolean {
  return state.includeAll || state.includeAllProxies ||
         state.filterKeywords.trim() !== '' || state.includeTypes.length > 0
}

// Check if proxy providers should be shown (has use/include-all-providers)
export function hasProxyProviders(state: ProxyGroupFormState): boolean {
  return state.includeAll || state.includeAllProviders
}

// Get default proxy order based on include options
export function getDefaultProxyOrder(state: ProxyGroupFormState): ProxyOrderItem[] {
  const order: ProxyOrderItem[] = []

  // Add region proxy groups marker if enabled
  if (state.includeRegionProxyGroups) {
    order.push(REGION_PROXY_GROUPS_MARKER)
  }

  // For include-all, providers come before nodes
  if (state.includeAll) {
    order.push(PROXY_PROVIDERS_MARKER)
    order.push(PROXY_NODES_MARKER)
  } else {
    if (hasProxyNodes(state)) {
      order.push(PROXY_NODES_MARKER)
    }
    if (hasProxyProviders(state)) {
      order.push(PROXY_PROVIDERS_MARKER)
    }
  }

  return order
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
  if (state.includeRegionProxyGroups) config['include-region-proxy-groups'] = true

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

  // Build proxies list from proxyOrder (including markers for backend) and staticProxies
  // Only include markers if the corresponding include option is set
  const proxiesFromOrder = state.proxyOrder.filter(item => {
    if (item === PROXY_NODES_MARKER) return hasProxyNodes(state)
    if (item === PROXY_PROVIDERS_MARKER) return hasProxyProviders(state)
    if (item === REGION_PROXY_GROUPS_MARKER) return state.includeRegionProxyGroups
    return true
  })
  const allProxies = [...proxiesFromOrder, ...state.staticProxies]
  if (allProxies.length > 0) {
    config.proxies = allProxies
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
export function configToFormState(config: ProxyGroupV3Config, allGroupNames: string[] = []): ProxyGroupFormState {
  // Separate proxy groups, markers, and static proxies
  const proxies = config.proxies || []
  const proxyOrder: string[] = []
  const staticProxies: string[] = []

  for (const p of proxies) {
    if (p === PROXY_NODES_MARKER || p === PROXY_PROVIDERS_MARKER || p === REGION_PROXY_GROUPS_MARKER) {
      proxyOrder.push(p)
    } else if (allGroupNames.includes(p)) {
      proxyOrder.push(p)
    } else {
      staticProxies.push(p)
    }
  }

  // include-all 等同于同时开启 include-all-proxies 和 include-all-providers
  const includeAll = config['include-all'] || false
  const includeAllProxies = config['include-all-proxies'] || includeAll
  const includeAllProviders = config['include-all-providers'] || includeAll

  const state: ProxyGroupFormState = {
    name: config.name,
    type: config.type,
    filterKeywords: regexToKeywords(config.filter || ''),
    excludeFilterKeywords: regexToKeywords(config['exclude-filter'] || ''),
    includeTypes: (config['include-type']?.split('|').filter(t => PROXY_TYPES.includes(t as ProxyType)) || []) as ProxyType[],
    excludeTypes: (config['exclude-type']?.split('|').filter(t => PROXY_TYPES.includes(t as ProxyType)) || []) as ProxyType[],
    includeAll,
    includeAllProxies,
    includeAllProviders,
    includeRegionProxyGroups: config['include-region-proxy-groups'] || false,
    includedProxyGroups: proxyOrder.filter(p => p !== PROXY_NODES_MARKER && p !== PROXY_PROVIDERS_MARKER && p !== REGION_PROXY_GROUPS_MARKER),
    proxyOrder,
    staticProxies,
    url: config.url || 'https://www.gstatic.com/generate_204',
    interval: config.interval || 300,
    tolerance: config.tolerance || 50,
  }

  // Add default markers if not present but should be shown
  const defaultOrder = getDefaultProxyOrder(state)
  for (const marker of defaultOrder) {
    if (!state.proxyOrder.includes(marker)) {
      state.proxyOrder.push(marker)
    }
  }

  return state
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
  const allGroupNames = template['proxy-groups'].map(g => g.name)
  return template['proxy-groups'].map(config => configToFormState(config, allGroupNames))
}

// Update proxy-groups in template content
export function updateProxyGroups(content: string, groups: ProxyGroupFormState[]): string {
  const template = parseTemplate(content)
  if (!template) return content

  template['proxy-groups'] = groups.map(formStateToConfig)
  return serializeTemplate(template)
}

// Display names for markers in preview (Chinese for better user understanding)
export const PROXY_NODES_DISPLAY = '⛓️‍💥 代理节点'
export const PROXY_PROVIDERS_DISPLAY = '📦 代理集合'
export const REGION_PROXY_GROUPS_DISPLAY = '🌏 区域代理组'

// Generate proxy-groups YAML preview from form states
export function generateProxyGroupsPreview(groups: ProxyGroupFormState[]): string {
  const configs = groups.map(formStateToConfig).map(config => {
    // Replace markers with Chinese display names for preview
    if (config.proxies) {
      config.proxies = config.proxies.map(p => {
        if (p === PROXY_NODES_MARKER) return PROXY_NODES_DISPLAY
        if (p === PROXY_PROVIDERS_MARKER) return PROXY_PROVIDERS_DISPLAY
        if (p === REGION_PROXY_GROUPS_MARKER) return REGION_PROXY_GROUPS_DISPLAY
        return p
      })
    }
    return config
  })
  return dumpYAML({ 'proxy-groups': configs }, { indent: 2, lineWidth: -1, noRefs: true })
}

// Generate region proxy groups as ProxyGroupFormState array
export function generateRegionProxyGroups(type: ProxyGroupType = 'url-test'): ProxyGroupFormState[] {
  const groups: ProxyGroupFormState[] = REGION_PROXY_GROUPS.map(region => {
    const state = {
      ...createDefaultFormState(region.name),
      type,
      filterKeywords: region.filter, // Keep original regex filter as-is
      includeAllProxies: true,
    }
    state.proxyOrder = getDefaultProxyOrder(state)
    return state
  })

  // Add "Other regions" group
  const otherState = {
    ...createDefaultFormState('🌐 其他地区'),
    type,
    filterKeywords: OTHER_REGIONS_EXCLUDE_FILTER, // Keep original regex filter as-is
    includeAllProxies: true,
  }
  otherState.proxyOrder = getDefaultProxyOrder(otherState)
  groups.push(otherState)

  return groups
}

// Get region proxy group names
export function getRegionProxyGroupNames(): string[] {
  return [...REGION_PROXY_GROUPS.map(r => r.name), '🌐 其他地区']
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
