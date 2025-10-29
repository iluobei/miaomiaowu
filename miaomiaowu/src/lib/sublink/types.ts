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
