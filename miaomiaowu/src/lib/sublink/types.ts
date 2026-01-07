// Type definitions for sublink-worker
export interface KanbanObject { 
  id: string
  name: string
  features: KanbanFeatrure[]
}

export interface KanbanFeatrure { 
  name: string
  id: string
  pid: string
}


export interface ProxyConfig {
  tag?: string
  server_port: number
  alter_id?: number
  security?: string
  network?: string
  tcp_fast_open?: boolean
  tls?: boolean
  transport?: TransportConfig
  name?: string
  type: string
  server: string
  port?: number
  password?: string
  uuid?: string
  method?: string
  flow?: string
  cipher?: string
  [key: string]: unknown
}

export interface TlsConfig {
  enabled: boolean
  server_name?: string
  insecure?: boolean
  alpn?: string[]
}

export interface TransportConfig {
  type: string
  path?: string
  headers?: Record<string, string>
  host?: string[]
  service_name?: string
}

export interface CustomRule {
  name: string
  site?: string
  ip?: string
  domain_suffix?: string
  domain_keyword?: string
  ip_cidr?: string
  protocol?: string
}

export interface ClashProxy {
  name: string
  type: string
  server: string
  port: number
  cipher?: string
  password?: string
  uuid?: string
  alterId?: number
  tls?: boolean
  servername?: string
  'skip-cert-verify'?: boolean
  network?: string
  'ws-opts'?: Record<string, unknown>
  'grpc-opts'?: Record<string, unknown>
  'http-opts'?: Record<string, unknown>
  flow: string
}

export interface ClashConfig {
  proxies: ClashProxy[]
  'proxy-groups': any[]
  rules: string[]
  [key: string]: any
}

export interface RuleSet {
  name: string
  outbound: string
  rules: string[]
}

export type PredefinedRuleSetType = 'minimal' | 'balanced' | 'comprehensive' | 'custom'

export interface GeneratedLinks {
  singbox: string
  clash: string
  xray: string
  surge: string
}

// Rule provider configuration for Clash Meta
export interface RuleProviderConfig {
  key: string          // Unique key for this rule provider
  behavior: string     // Rule behavior: domain, ipcidr, classical
  type: string         // Provider type: http, file
  format: string       // Format: yaml, mrs, text
  url: string          // Remote URL for downloading rules
  path: string         // Local cache path
  interval: number     // Update interval in seconds
}

// Proxy group category with rule providers
export interface ProxyGroupCategory {
  name: string              // Internal identifier (e.g., "ai", "youtube")
  label: string             // Display label (e.g., "AI 服务", "油管视频")
  emoji: string             // Emoji for UI display
  icon: string              // Icon identifier
  rule_name: string         // Rule name for Clash config
  group_label: string       // Label for proxy group (e.g., "💬 AI 服务")
  presets: string[]         // Which presets include this category
  site_rules: RuleProviderConfig[]  // Domain-based rule providers
  ip_rules: RuleProviderConfig[]    // IP-based rule providers
}
