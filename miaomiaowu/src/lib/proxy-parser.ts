/**
 * 代理协议解析工具
 * 支持解析 tuic、trojan、hysteria、hysteria2、vmess、vless、socks、ss 等协议
 * 并转换为 Clash 节点格式
 */

import { toast } from "sonner"

// 通用代理节点接口
export interface ProxyNode {
  name: string
  type: string
  server: string
  port: number
  password?: string
  uuid?: string
  method?: string
  cipher?: string
  [key: string]: unknown,
  'spider-x'?: string
}

// Clash 节点格式
export interface ClashProxy {
  name: string
  type: string
  server: string
  port: number
  [key: string]: unknown
}

/**
 * Base64 解码（支持 URL Safe）
 */
function base64Decode(str: string): string {
  try {
    // 处理 URL Safe Base64
    let base64 = str.replace(/-/g, '+').replace(/_/g, '/')
    // 补齐 padding
    const pad = base64.length % 4
    if (pad) {
      base64 += '='.repeat(4 - pad)
    }
    return decodeURIComponent(escape(atob(base64)))
  } catch (e) {
    toast(`'Base64 decode error:' ${e instanceof Error ? e.message : String(e)}`)
    return ''
  }
}

/**
 * 解析 URL 查询参数
 */
function parseQueryString(query: string): Record<string, string> {
  const params: Record<string, string> = {}
  if (!query) return params

  const pairs = query.split('&')
  for (const pair of pairs) {
    const [key, value] = pair.split('=')
    if (key) {
      params[decodeURIComponent(key)] = value ? decodeURIComponent(value) : ''
    }
  }
  return params
}

/**
 * 解析 VMess 协议
 * 格式: vmess://base64(json)
 */
function parseVmess(url: string): ProxyNode | null {
  try {
    const base64Content = url.substring('vmess://'.length)
    const jsonStr = base64Decode(base64Content)
    if (!jsonStr) return null

    const config = JSON.parse(jsonStr)

    const node: ProxyNode = {
      name: config.ps || config.name || 'VMess Node',
      type: 'vmess',
      server: config.add || config.address || '',
      port: parseInt(config.port) || 0,
      uuid: config.id || '',
      alterId: parseInt(config.aid) || 0,
      cipher: config.scy || 'auto',
      network: config.net || 'tcp',
      tls: config.tls === 'tls' || config.tls === true
    }

    // SNI/Servername
    if (config.sni) {
      node.servername = config.sni
    } else if (config.host && config.tls) {
      node.servername = config.host
    }

    // ALPN
    if (config.alpn) {
      node.alpn = typeof config.alpn === 'string' ? config.alpn.split(',') : config.alpn
    }

    // Client Fingerprint
    if (config.fp) {
      node.fp = config.fp
    }

    // Skip cert verify
    if (config.allowInsecure !== undefined) {
      node.skipCertVerify = config.allowInsecure === true || config.allowInsecure === '1' || config.allowInsecure === 1
    }

    // WebSocket
    if (config.net === 'ws') {
      node['ws-opts'] = {
        path: config.path || '/',
        headers: config.host ? { Host: config.host } : {}
      }
    }

    // HTTP/2
    if (config.net === 'h2') {
      node['h2-opts'] = {
        host: config.host ? (Array.isArray(config.host) ? config.host : [config.host]) : [],
        path: config.path || '/'
      }
    }

    // gRPC
    if (config.net === 'grpc') {
      node['grpc-opts'] = {
        'grpc-service-name': config.path || config['grpc-service-name'] || ''
      }
    }

    return node
  } catch (e) {
    toast(`'Parse VMess error: '${e instanceof Error ? e.message : String(e)}`)
    return null
  }
}

/**
 * 解析 Shadowsocks 协议
 * 格式: ss://base64(method:password)@server:port#name
 * 或: ss://base64(method:password@server:port)#name
 */
function parseShadowsocks(url: string): ProxyNode | null {
  try {
    const content = url.substring('ss://'.length)
    let name = 'SS Node'
    let mainPart = content

    // 提取节点名称
    if (content.includes('#')) {
      const parts = content.split('#')
      mainPart = parts[0]
      name = decodeURIComponent(parts[1])
    }

    let server = ''
    let port = 0
    let method = ''
    let password = ''

    // 格式1: base64(method:password)@server:port
    if (mainPart.includes('@')) {
      const [encodedPart, serverPart] = mainPart.split('@')
      const decoded = base64Decode(encodedPart)
      const [m, p] = decoded.split(':')
      method = m
      password = p

      const [s, po] = serverPart.split(':')
      server = s
      port = parseInt(po) || 0
    } else {
      // 格式2: base64(method:password@server:port)
      const decoded = base64Decode(mainPart)
      const atIndex = decoded.lastIndexOf('@')
      if (atIndex === -1) return null

      const authPart = decoded.substring(0, atIndex)
      const serverPart = decoded.substring(atIndex + 1)

      const [m, p] = authPart.split(':')
      method = m
      password = p

      const [s, po] = serverPart.split(':')
      server = s
      port = parseInt(po) || 0
    }

    return {
      name,
      type: 'ss',
      server,
      port,
      cipher: method,
      password
    }
  } catch (e) {
    toast(`'Parse Shadowsocks error:' ${e instanceof Error ? e.message : String(e)}`)
    return null
  }
}

/**
 * 解析 SOCKS 协议
 * 格式: socks://base64(user:password)@server:port#name
 */
function parseSocks(url: string): ProxyNode | null {
  try {
    const content = url.substring('socks://'.length)
    let name = 'SOCKS Node'
    let mainPart = content

    if (content.includes('#')) {
      const parts = content.split('#')
      mainPart = parts[0]
      name = decodeURIComponent(parts[1])
    }

    const [encodedAuth, serverPart] = mainPart.split('@')
    const decoded = base64Decode(encodedAuth)
    const [username, password] = decoded.split(':')

    const [server, portStr] = serverPart.split(':')
    const port = parseInt(portStr) || 0

    return {
      name,
      type: 'socks5',
      server,
      port,
      username,
      password
    }
  } catch (e) {
    toast(`'Parse SOCKS error:' ${e instanceof Error ? e.message : String(e)}`)
    return null
  }
}

/**
 * 解析 Snell 协议
 * 格式: snell://password@server:port?obfs=http&obfs-host=example.com&version=4#name
 */
// function parseSnell(url: string): ProxyNode | null {
//   try {
//     const content = url.substring('snell://'.length)
//     let name = 'Snell Node'
//     let mainPart = content

//     // 提取节点名称
//     if (content.includes('#')) {
//       const hashIndex = content.lastIndexOf('#')
//       mainPart = content.substring(0, hashIndex)
//       name = decodeURIComponent(content.substring(hashIndex + 1))
//     }

//     // 提取查询参数
//     let queryParams: Record<string, string> = {}
//     let authAndServer = mainPart
//     if (mainPart.includes('?')) {
//       const [main, query] = mainPart.split('?')
//       authAndServer = main
//       queryParams = parseQueryString(query)
//     }

//     // 解析 password@server:port
//     const atIndex = authAndServer.lastIndexOf('@')
//     if (atIndex === -1) return null

//     const password = authAndServer.substring(0, atIndex)
//     const serverPart = authAndServer.substring(atIndex + 1)

//     // 解析 server:port
//     const colonIndex = serverPart.lastIndexOf(':')
//     if (colonIndex === -1) return null

//     const server = serverPart.substring(0, colonIndex)
//     const port = parseInt(serverPart.substring(colonIndex + 1)) || 0

//     const node: ProxyNode = {
//       name,
//       type: 'snell',
//       server,
//       port,
//       psk: password,  // Snell 使用 psk (pre-shared key)
//       version: parseInt(queryParams.version) || 4  // 默认版本 4
//     }

//     // 混淆设置
//     if (queryParams.obfs && queryParams.obfs !== 'none') {
//       node['obfs-opts'] = {
//         mode: queryParams.obfs,  // http, tls
//         host: queryParams['obfs-host'] || queryParams['obfs-hostname'] || ''
//       }
//     }

//     return node
//   } catch (e) {
//     toast(`Parse Snell error: ${e instanceof Error ? e.message : String(e)}`)
//     return null
//   }
// }

/**
 * 解析通用协议 (trojan, vless, tuic, hysteria, hysteria2)
 * 格式: protocol://password@server:port?key1=value1&key2=value2#name
 */
function parseGenericProtocol(url: string, protocol: string): ProxyNode | null {
  try {
    const content = url.substring(`${protocol}://`.length)
    let name = `${protocol.toUpperCase()} Node`
    let mainPart = content

    // 提取节点名称
    if (content.includes('#')) {
      const hashIndex = content.lastIndexOf('#')
      mainPart = content.substring(0, hashIndex)
      name = decodeURIComponent(content.substring(hashIndex + 1))
    }

    // 提取查询参数
    let queryParams: Record<string, string> = {}
    let authAndServer = mainPart
    if (mainPart.includes('?')) {
      const [main, query] = mainPart.split('?')
      authAndServer = main
      queryParams = parseQueryString(query)
    }

    // 解析 password@server:port (支持 IPv6)
    const atIndex = authAndServer.lastIndexOf('@')
    if (atIndex === -1) return null

    const password = authAndServer.substring(0, atIndex)
    const serverPart = authAndServer.substring(atIndex + 1)

    let server = ''
    let port = 0

    // 检查是否是 IPv6 地址 (格式: [ipv6]:port)
    if (serverPart.startsWith('[')) {
      const closeBracketIndex = serverPart.indexOf(']')
      if (closeBracketIndex !== -1) {
        // 保留完整的 [ipv6]:port 格式给 Hysteria2（Clash 需要）
        // 但同时提取纯 IPv6 地址用于其他协议
        const ipv6Address = serverPart.substring(1, closeBracketIndex)
        const portPart = serverPart.substring(closeBracketIndex + 1)
        port = parseInt(portPart.replace(':', '')) || 0

        // 对于 Hysteria2，保留方括号；其他协议去掉方括号
        if (protocol === 'hysteria2' || protocol === 'hysteria') {
          server = serverPart.substring(0, closeBracketIndex + 1) // 包含方括号 [ipv6]
        } else {
          server = ipv6Address // 去掉方括号
        }
      }
    } else {
      // IPv4 或域名
      const parts = serverPart.split(':')
      server = parts[0]
      port = parseInt(parts[parts.length - 1]) || 0
    }

    const node: ProxyNode = {
      name,
      type: protocol,
      server,
      port
    }

    // 根据协议类型添加特定字段
    switch (protocol) {
      case 'trojan':
        node.password = password
        node.sni = queryParams.sni || queryParams.peer || queryParams.host || server
        node.network = queryParams.type || 'tcp'

        // TLS 设置
        if (queryParams.security) {
          node.security = queryParams.security
        }

        // 传输层配置
        if (queryParams.type === 'ws') {
          node['ws-opts'] = {
            path: queryParams.path || '/',
            headers: queryParams.host ? { Host: queryParams.host } : {}
          }
        } else if (queryParams.type === 'grpc') {
          node['grpc-opts'] = {
            'grpc-service-name': queryParams.serviceName || queryParams.path || ''
          }
        } else if (queryParams.type === 'h2' || queryParams.type === 'http') {
          node['h2-opts'] = {
            host: queryParams.host ? [queryParams.host] : [],
            path: queryParams.path || '/'
          }
        }

        // 其他参数
        if (queryParams.alpn) {
          node.alpn = queryParams.alpn.split(',')
        }
        if (queryParams.fp) {
          node.fp = queryParams.fp
        }
        node.skipCertVerify = queryParams.allowInsecure === '1' || queryParams['skip-cert-verify'] === '1'
        break

      case 'vless':
        node.password = password
        node.uuid = password
        node.flow = queryParams.flow || ''
        node.encryption = queryParams.encryption || 'none' // 加密方式，默认为 none
        node.security = queryParams.security || 'none'
        node.tls = queryParams.security === 'tls' || queryParams.security === 'reality'
        node.network = queryParams.type || 'tcp'
        node.sni = queryParams.sni || server
        node.servername = queryParams.sni || server
        node.skipCertVerify = queryParams.allowInsecure === '1'
        node['spider-x'] = queryParams.spx

        // Reality 协议专用参数
        if (queryParams.security === 'reality') {
          node.pbk = queryParams.pbk || ''
          node.sid = queryParams.sid || ''
          node.spx = queryParams.spx || ''
          node.fp = queryParams.fp || ''
          node['public-key'] = queryParams.pbk || ''
          node['short-id'] = queryParams.sid || ''
        }

        // 传输层配置
        if (queryParams.type === 'ws') {
          node['ws-opts'] = {
            path: queryParams.path || '/',
            headers: queryParams.host ? { Host: queryParams.host } : {}
          }
        } else if (queryParams.type === 'grpc') {
          node['grpc-opts'] = {
            'grpc-service-name': queryParams.serviceName || queryParams.path || ''
          }
        } else if (queryParams.type === 'h2' || queryParams.type === 'http') {
          node['h2-opts'] = {
            host: queryParams.host ? [queryParams.host] : [],
            path: queryParams.path || '/'
          }
        }

        // 其他常见参数
        if (queryParams.alpn) {
          node.alpn = queryParams.alpn.split(',')
        }
        if (queryParams.host) {
          node.host = queryParams.host
        }
        if (queryParams.path) {
          node.path = queryParams.path
        }
        if (queryParams.headerType) {
          node.headerType = queryParams.headerType
        }
        break

      case 'hysteria':
      case 'hysteria2':
        node.password = password // Hysteria2 使用 password 字段
        node.auth = password // 内部临时字段，用于传递认证信息
        // node.ports = queryParams.mport || port.toString()
        node.obfs = queryParams.obfs
        node['obfs-password'] = queryParams.obfsParam
        node.sni = queryParams.peer || queryParams.sni || (server.startsWith('[') ? '' : server)
        node.alpn = queryParams.alpn ? queryParams.alpn.split(',') : undefined
        // insecure=1 表示跳过证书验证
        node.skipCertVerify = queryParams.insecure === '1' || queryParams.allowInsecure === '1' || queryParams['skip-cert-verify'] === '1'
        node.up = queryParams.up || queryParams.upmbps
        node.down = queryParams.down || queryParams.downmbps
        // 只有在 URL 中明确指定了 fp 参数时才添加 client-fingerprint
        if (queryParams.fp) {
          node.fp = queryParams.fp
        }
        break

      case 'tuic':
        node.uuid = password
        node.password = queryParams.password || ''
        node.sni = queryParams.sni || server
        node.alpn = queryParams.alpn ? queryParams.alpn.split(',') : ['h3']
        node.skipCertVerify = queryParams.allowInsecure === '1'
        node['congestion-controller'] = queryParams.congestion_control || 'bbr'
        node['udp-relay-mode'] = queryParams.udp_relay_mode || 'native'
        break
    }

    return node
  } catch (e) {
    toast(`Parse ${protocol} error: ${e instanceof Error ? e.message : String(e)}`)
    return null
  }
}

/**
 * 解析单个代理 URL
 */
export function parseProxyUrl(url: string): ProxyNode | null {
  if (!url || typeof url !== 'string') {
    return null
  }

  url = url.trim()

  if (url.startsWith('vmess://')) {
    return parseVmess(url)
  } else if (url.startsWith('ss://')) {
    return parseShadowsocks(url)
  } else if (url.startsWith('socks://')) {
    return parseSocks(url)
  // } else if (url.startsWith('snell://')) {
  //   return parseSnell(url)
  } else if (url.startsWith('trojan://')) {
    return parseGenericProtocol(url, 'trojan')
  } else if (url.startsWith('vless://')) {
    return parseGenericProtocol(url, 'vless')
  } else if (url.startsWith('hysteria://')) {
    return parseGenericProtocol(url, 'hysteria')
  } else if (url.startsWith('hy2://')) {
    return parseGenericProtocol(url.replace('hy2://', 'hysteria2://'), 'hysteria2')
  } else if (url.startsWith('hysteria2://')) {
    return parseGenericProtocol(url, 'hysteria2')
  } else if (url.startsWith('tuic://')) {
    return parseGenericProtocol(url, 'tuic')
  }

  return null
}

/**
 * 转换为 Clash 节点格式
 */
export function toClashProxy(node: ProxyNode): ClashProxy {
  // 参数名映射表：将缩写转换为 Clash 标准格式
  const paramMapping: Record<string, string> = {
    // VLESS Reality 参数
    'pbk': 'public-key',
    'sid': 'short-id',
    'spx': 'spider-x',
    'fp': 'client-fingerprint',

    // 通用参数映射
    'sni': 'servername',
    'alpn': 'alpn',
    'allowInsecure': 'skip-cert-verify',
    'skipCertVerify': 'skip-cert-verify',

    // 保持原样的参数
    'servername': 'servername',
    'public-key': 'public-key',
    'short-id': 'short-id',
    'spider-x': 'spider-x',
    'fingerprint': 'fingerprint',
    'skip-cert-verify': 'skip-cert-verify'
  }

  // 需要排除的中间参数（不输出到 Clash）
  const excludeKeys = new Set([
    'name', 'type', 'server', 'port',
    // 原始缩写参数（已转换为标准格式）
    'pbk', 'sid', 'spx', 'fp',
    // Reality 参数（已转换为 reality-opts）
    'public-key', 'short-id', 'spider-x', '_spider-x',
    // 中间状态参数
    'allowInsecure', 'skipCertVerify',
    'sni', // 已转换为 servername
    // 'servername', // 与 server 重复，不需要输出
    'auth', // Hysteria2 内部使用的中间字段，已转换为 password
    'password', // 认证字段，已在第530-541行根据协议类型单独处理
    'uuid', // 认证字段，已在第526-528行单独处理
    // 'psk', // Snell 认证字段，需单独处理
    // 'version', // Snell 版本字段，需单独处理
    // 已处理的参数
    'security', // 已转换为 tls 和 reality-opts
    'fingerprint' // 已转换为 client-fingerprint
  ])

  const clash: ClashProxy = {
    name: node.name,
    type: node.type,
    server: node.server,
    port: node.port
  }

  // 首先处理标准字段（按 Clash 推荐顺序）
  if (node.uuid) {
    clash.uuid = node.uuid
  }

  // 根据协议类型设置认证字段
  if (node.type === 'vless') {
    // VLESS 只使用 uuid，不需要 password
    // 添加 encryption 字段（VLESS 特有）
    if (node.encryption) {
      clash.encryption = node.encryption
    }
  // } else if (node.type === 'snell') {
  //   // Snell 使用 psk (pre-shared key)
  //   if (node.psk) {
  //     clash.psk = node.psk
  //   }
  //   // Snell version
  //   if (node.version) {
  //     clash.version = node.version
  //   }
  } else if (node.type === 'hysteria2' || node.type === 'hysteria') {
    // Hysteria/Hysteria2 使用 password
    if (node.password) {
      clash.password = node.password
    }
  } else if (node.password) {
    // 其他协议（trojan、ss 等）使用 password
    clash.password = node.password
  }

  // SOCKS5 协议专用字段
  if (node.type === 'socks5' || node.type === 'socks') {
    if (node.username) clash.username = node.username as string
    if (node.password) clash.password = node.password as string
  }

  // TLS 设置
  if (node.security) {
    if (node.type === 'vless') {
      clash.tls = node.security === 'tls' || node.security === 'reality'
    } else if (node.type === 'trojan') {
      // Trojan 默认使用 TLS
      clash.tls = true
    } else if (node.tls !== undefined) {
      clash.tls = node.tls
    }
  } else if (node.tls !== undefined) {
    clash.tls = node.tls
  } else if (node.type === 'trojan') {
    // Trojan 默认启用 TLS
    clash.tls = true
  }

  // Flow 控制
  if (node.flow) clash.flow = node.flow

  // Skip cert verify - Reality 协议默认为 true
  if (node.security === 'reality') {
    clash['skip-cert-verify'] = true
  } else if (node.skipCertVerify !== undefined) {
    clash['skip-cert-verify'] = node.skipCertVerify
  } else if (node.allowInsecure !== undefined) {
    clash['skip-cert-verify'] = node.allowInsecure
  }

  // Reality 协议选项
  if (node.security === 'reality') {
    const realityOpts: Record<string, unknown> = {}
    if (node.pbk || node['public-key']) {
      realityOpts['public-key'] = (node['public-key'] || node.pbk) as string
    }
    if (node.sid !== undefined || node['short-id'] !== undefined) {
      realityOpts['short-id'] = (node['short-id'] || node.sid || '') as string
    }
    // 添加 spider-x 参数
    if (node.spx || node['spider-x'] || node['_spider-x']) {
      realityOpts['spider-x'] = (node['spider-x'] || node['_spider-x'] || node.spx || '') as string
    }
    clash['reality-opts'] = realityOpts
  }

  // SNI 设置 - 特定协议需要输出 sni 字段
  if (node.type === 'hysteria' || node.type === 'hysteria2' || node.type === 'trojan' || node.type === 'tuic') {
    if (node.sni && node.sni !== node.server) {
      clash.sni = node.sni
    }
  }

  // Client Fingerprint (注意是 client-fingerprint 不是 fingerprint)
  if (node.fp || node.fingerprint) {
    clash['client-fingerprint'] = (node.fingerprint || node.fp) as string
  }

  // 网络类型
  if (node.network) clash.network = node.network

  // ALPN
  if (node.alpn) {
    clash.alpn = node.alpn
  }

  // 其他加密设置
  if (node.cipher) clash.cipher = node.cipher

  // VMess 专用字段
  if (node.type === 'vmess') {
    if (node.alterId !== undefined) clash.alterId = node.alterId as number
    // VMess 默认添加 tfo: false（除非明确指定）
    if (clash.tfo === undefined) clash.tfo = false
  }

  // 复制其他属性（传输层配置等）
  for (const [key, value] of Object.entries(node)) {
    if (value !== undefined &&
        !excludeKeys.has(key) &&
        !Object.prototype.hasOwnProperty.call(clash, key) &&
        key !== 'tls' && // tls 已经被处理过了
        key !== 'security' && // security 已经被处理过了
        key !== 'sni' // sni 已经映射为 servername
    ) {
      // 检查是否需要映射参数名
      const mappedKey = paramMapping[key] || key

      // 避免重复添加已处理的参数
      if (!Object.prototype.hasOwnProperty.call(clash, mappedKey)) {
        clash[mappedKey] = value
      }
    }
  }

  return clash
}

/**
 * 解析订阅内容（多个代理 URL，每行一个或 base64 编码）
 */
export function parseSubscription(content: string): ClashProxy[] {
  if (!content) return []

  let lines: string[] = []

  // 尝试 base64 解码
  try {
    const decoded = base64Decode(content)
    if (decoded && decoded.includes('://')) {
      lines = decoded.split('\n')
    } else {
      lines = content.split('\n')
    }
  } catch {
    lines = content.split('\n')
  }

  const proxies: ClashProxy[] = []

  for (const line of lines) {
    const trimmed = line.trim()
    if (!trimmed || !trimmed.includes('://')) continue

    const node = parseProxyUrl(trimmed)
    if (node) {
      proxies.push(toClashProxy(node))
    }
  }

  return proxies
}

/**
 * 生成 Clash 配置的代理部分
 */
export function generateClashProxiesConfig(proxies: ClashProxy[]): string {
  return `proxies:\n${proxies.map(p => '  - ' + JSON.stringify(p)).join('\n')}`
}
