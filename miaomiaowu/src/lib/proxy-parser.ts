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
 * 安全的 URL 解码，解码失败时返回原字符串
 */
function safeDecodeURIComponent(str: string): string {
  if (!str) return str
  try {
    return decodeURIComponent(str)
  } catch {
    return str
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
        path: safeDecodeURIComponent(config.path) || '/',
        headers: config.host ? { Host: safeDecodeURIComponent(config.host) } : {}
      }
    }

    // HTTP/2
    if (config.net === 'h2') {
      const decodedHost = config.host ? safeDecodeURIComponent(config.host) : ''
      node['h2-opts'] = {
        host: decodedHost ? (Array.isArray(config.host) ? config.host.map(safeDecodeURIComponent) : [decodedHost]) : [],
        path: safeDecodeURIComponent(config.path) || '/'
      }
    }

    // gRPC
    if (config.net === 'grpc') {
      node['grpc-opts'] = {
        'grpc-service-name': safeDecodeURIComponent(config.path || config['grpc-service-name']) || ''
      }
    }

    // UDP - 如果配置中明确指定了 udp 参数，使用配置的值；否则默认为 true
    if (config.udp !== undefined) {
      node.udp = config.udp === true || config.udp === 'true' || config.udp === '1' || config.udp === 1
    } else {
      node.udp = true
    }

    return node
  } catch (e) {
    toast(`'Parse VMess error: '${e instanceof Error ? e.message : String(e)}`)
    return null
  }
}

/**
 * 解析 ShadowsocksR 协议
 * 格式: ssr://base64(server:port:protocol:method:obfs:base64(password)/?obfsparam=base64(obfs_param)&protoparam=base64(proto_param)&remarks=base64(name)&group=base64(group))
 */
function parseShadowsocksR(url: string): ProxyNode | null {
  try {
    const content = url.substring('ssr://'.length)
    const decoded = base64Decode(content)
    if (!decoded) return null

    // 分离主体和参数部分
    const parts = decoded.split('/?')
    const mainPart = parts[0]
    const paramsPart = parts.length > 1 ? parts[1] : ''

    // 解析主体部分: server:port:protocol:method:obfs:base64(password)
    // 注意：需要从右往左解析，因为 server 可能是 IPv6 地址（包含冒号）
    const mainSegments = mainPart.split(':')
    if (mainSegments.length < 6) return null

    // 从右往左提取固定的字段（最后5个字段）
    const passwordBase64 = mainSegments[mainSegments.length - 1]
    const obfs = mainSegments[mainSegments.length - 2]
    const method = mainSegments[mainSegments.length - 3]
    const protocol = mainSegments[mainSegments.length - 4]
    const portStr = mainSegments[mainSegments.length - 5]

    // server 是剩余的所有部分（可能包含冒号，如 IPv6）
    const server = mainSegments.slice(0, mainSegments.length - 5).join(':')
    const port = parseInt(portStr) || 0
    const password = base64Decode(passwordBase64)

    // 解析参数部分
    const params = parseQueryString(paramsPart)
    const name = params.remarks ? base64Decode(params.remarks) : 'SSR Node'
    const obfsParam = params.obfsparam ? base64Decode(params.obfsparam) : ''
    const protoParam = params.protoparam ? base64Decode(params.protoparam) : ''

    const node: ProxyNode = {
      name,
      type: 'ssr',
      server,
      port,
      cipher: method,
      password,
      protocol,
      obfs
    }

    if (obfsParam) {
      node['obfs-param'] = obfsParam
    }
    if (protoParam) {
      node['protocol-param'] = protoParam
    }

    node.udp = true  // SSR 协议默认支持 UDP

    return node
  } catch (e) {
    toast(`Parse ShadowsocksR error: ${e instanceof Error ? e.message : String(e)}`)
    return null
  }
}

/**
 * 解析 SS plugin 参数
 * plugin 格式: plugin-name;param1=value1;param2=value2
 * 例如: obfs-local;obfs=http;obfs-host=example.com
 *
 * 支持的 plugin 类型:
 * - obfs/obfs-local: obfs=xxx -> mode, obfs-host -> host
 * - v2ray-plugin: mode, tls, host, path, mux, headers, fingerprint, skip-cert-verify, v2ray-http-upgrade
 * - gost-plugin: 类似 v2ray-plugin
 * - shadow-tls: host, password, version, client-fingerprint/fp
 * - restls: host, password, version-hint, restls-script, client-fingerprint/fp
 * - kcptun: key, crypt, mode, conn, autoexpire, scavengettl, mtu, sndwnd, rcvwnd, datashard, parityshard, dscp, nocomp, acknodelay, nodelay, interval, resend, sockbuf, smuxver, smuxbuf, streambuf, keepalive
 */
function parseSSPlugin(pluginStr: string): { plugin: string; pluginOpts: Record<string, unknown>; clientFingerprint?: string } | null {
  if (!pluginStr) return null

  // 先进行 URL 解码
  const decoded = decodeURIComponent(pluginStr)

  // 用 ; 分割参数
  const parts = decoded.split(';')
  if (parts.length === 0) return null

  // 第一个部分是 plugin 名称
  const pluginName = parts[0].trim()
  if (!pluginName) return null

  // 标准化 plugin 名称
  let plugin = pluginName
  if (pluginName === 'obfs-local' || pluginName === 'simple-obfs') {
    plugin = 'obfs'
  }

  const pluginOpts: Record<string, unknown> = {}
  let clientFingerprint: string | undefined

  // 解析剩余参数
  for (let i = 1; i < parts.length; i++) {
    const part = parts[i].trim()
    if (!part) continue

    const eqIndex = part.indexOf('=')
    if (eqIndex === -1) continue

    const key = part.substring(0, eqIndex).trim()
    const value = part.substring(eqIndex + 1).trim()

    // 根据 plugin 类型处理参数
    switch (plugin) {
      case 'obfs':
        if (key === 'obfs') {
          pluginOpts.mode = value
        } else if (key === 'obfs-host' || key === 'host') {
          pluginOpts.host = value
        }
        break

      case 'v2ray-plugin':
      case 'gost-plugin':
        if (key === 'mode') {
          pluginOpts.mode = value
        } else if (key === 'tls') {
          pluginOpts.tls = value === 'true' || value === '1'
        } else if (key === 'host') {
          pluginOpts.host = value
        } else if (key === 'path') {
          pluginOpts.path = value
        } else if (key === 'mux') {
          pluginOpts.mux = value === 'true' || value === '1'
        } else if (key === 'fingerprint') {
          pluginOpts.fingerprint = value
        } else if (key === 'skip-cert-verify') {
          pluginOpts['skip-cert-verify'] = value === 'true' || value === '1'
        } else if (key === 'v2ray-http-upgrade') {
          pluginOpts['v2ray-http-upgrade'] = value === 'true' || value === '1'
        } else if (key.startsWith('headers')) {
          // headers 参数处理 (headers.Custom=value)
          if (!pluginOpts.headers) {
            pluginOpts.headers = {}
          }
          const headerKey = key.replace(/^headers\.?/, '') || 'custom'
          ;(pluginOpts.headers as Record<string, string>)[headerKey] = value
        }
        break

      case 'shadow-tls':
        if (key === 'host') {
          pluginOpts.host = value
        } else if (key === 'password') {
          pluginOpts.password = value
        } else if (key === 'version') {
          pluginOpts.version = parseInt(value) || 2
        } else if (key === 'fp' || key === 'client-fingerprint') {
          // shadow-tls 的 client-fingerprint 需要放到顶层
          clientFingerprint = value
        }
        break

      case 'restls':
        if (key === 'host') {
          pluginOpts.host = value
        } else if (key === 'password') {
          pluginOpts.password = value
        } else if (key === 'version-hint') {
          pluginOpts['version-hint'] = value
        } else if (key === 'restls-script') {
          pluginOpts['restls-script'] = value
        } else if (key === 'fp' || key === 'client-fingerprint') {
          // restls 的 client-fingerprint 需要放到顶层
          clientFingerprint = value
        }
        break

      case 'kcptun':
        // kcptun 参数直接映射
        if (['conn', 'autoexpire', 'scavengettl', 'mtu', 'sndwnd', 'rcvwnd',
             'datashard', 'parityshard', 'dscp', 'nodelay', 'interval',
             'resend', 'sockbuf', 'smuxver', 'smuxbuf', 'streambuf', 'keepalive'].includes(key)) {
          pluginOpts[key] = parseInt(value)
        } else if (['nocomp', 'acknodelay'].includes(key)) {
          pluginOpts[key] = value === 'true' || value === '1'
        } else if (['key', 'crypt', 'mode'].includes(key)) {
          pluginOpts[key] = value
        }
        break

      default:
        // 未知 plugin 类型，直接映射参数
        pluginOpts[key] = value
    }
  }

  return {
    plugin,
    pluginOpts,
    clientFingerprint
  }
}

/**
 * 解析 Shadowsocks 协议
 * 格式: ss://base64(method:password)@server:port#name
 * 或: ss://base64(method:password@server:port)#name
 * 或: ss://base64(method:password)@server:port/?plugin=xxx#name
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

    // 提取查询参数（处理 ?plugin=xxx&group=xxx 格式）
    let queryParams: Record<string, string> = {}
    if (mainPart.includes('?')) {
      const qIndex = mainPart.indexOf('?')
      const queryString = mainPart.substring(qIndex + 1)
      mainPart = mainPart.substring(0, qIndex)
      queryParams = parseQueryString(queryString)
    }

    // 去掉 mainPart 尾部的 /
    mainPart = mainPart.replace(/\/$/, '')

    let server = ''
    let port = 0
    let method = ''
    let password = ''

    // 格式1: base64(method:password)@server:port
    if (mainPart.includes('@')) {
      const [encodedPart, serverPart] = mainPart.split('@')
      // 修复ss password含有:号, urlencode格式转换
      const decoded = base64Decode(encodedPart.indexOf('%') == -1 ? encodedPart : decodeURIComponent(encodedPart))
      const colonIndex = decoded.indexOf(':')
      const m = decoded.substring(0, colonIndex)
      const p = decoded.substring(colonIndex + 1)
      method = m
      password = p

      // 从最后一个冒号分割，支持IPv6地址
      const lastColonIndex = serverPart.lastIndexOf(':')
      if (lastColonIndex === -1) return null
      server = serverPart.substring(0, lastColonIndex)
      port = parseInt(serverPart.substring(lastColonIndex + 1)) || 0
    } else {
      // 格式2: base64(method:password@server:port)
      const decoded = base64Decode(mainPart)
      const atIndex = decoded.lastIndexOf('@')
      if (atIndex === -1) return null

      const authPart = decoded.substring(0, atIndex)
      const serverPart = decoded.substring(atIndex + 1)

      const colonIndex = authPart.indexOf(':')
      const m = authPart.substring(0, colonIndex)
      const p = authPart.substring(colonIndex + 1)
      method = m
      password = p

      // 从最后一个冒号分割，支持IPv6地址
      const lastColonIndex = serverPart.lastIndexOf(':')
      if (lastColonIndex === -1) return null
      server = serverPart.substring(0, lastColonIndex)
      port = parseInt(serverPart.substring(lastColonIndex + 1)) || 0
    }

    const node: ProxyNode = {
      name,
      type: 'ss',
      server,
      port,
      cipher: method,
      password
    }

    // SS 协议默认支持 UDP
    node.udp = true

    // 解析 plugin 参数
    if (queryParams.plugin) {
      const pluginInfo = parseSSPlugin(queryParams.plugin)
      if (pluginInfo) {
        node.plugin = pluginInfo.plugin
        if (Object.keys(pluginInfo.pluginOpts).length > 0) {
          node['plugin-opts'] = pluginInfo.pluginOpts
        }
        // shadow-tls 和 restls 的 client-fingerprint 需要放到顶层
        if (pluginInfo.clientFingerprint) {
          node['client-fingerprint'] = pluginInfo.clientFingerprint
        }
      }
    }

    return node
  } catch (e) {
    toast(`'Parse Shadowsocks error:' ${e instanceof Error ? e.message : String(e)}`)
    return null
  }
}

/**
 * 解析 SOCKS 协议
 * 支持两种格式:
 * 1. socks://base64(user:password)@server:port?#name (x-ray 内核格式)
 * 2. socks5://user:password@server:port#name (通用格式，自动转换)
 */
function parseSocks(url: string): ProxyNode | null {
  try {
    let content: string
    let isPlainAuth = false  // 标记是否是明文认证格式 (socks5://)

    // 检测是 socks:// 还是 socks5:// 格式
    if (url.startsWith('socks5://')) {
      content = url.substring('socks5://'.length)
      isPlainAuth = true
    } else {
      content = url.substring('socks://'.length)
    }

    let mainPart = content
    let name = ''

    // 提取节点名称 (# 后面的部分)
    if (content.includes('#')) {
      const hashIndex = content.lastIndexOf('#')
      mainPart = content.substring(0, hashIndex)
      name = decodeURIComponent(content.substring(hashIndex + 1))
    }

    // 移除可能存在的查询参数 (? 后面的部分，在 # 之前)
    if (mainPart.includes('?')) {
      mainPart = mainPart.split('?')[0]
    }

    // 解析 auth@server:port
    const atIndex = mainPart.lastIndexOf('@')
    if (atIndex === -1) {
      // 没有认证信息的情况
      const [server, portStr] = mainPart.split(':')
      const port = parseInt(portStr) || 0

      // 如果没有名称，自动生成
      if (!name) {
        name = `${server}:${port}`
      }

      return {
        name,
        type: 'socks5',
        server,
        port,
        udp: true
      }
    }

    const authPart = mainPart.substring(0, atIndex)
    const serverPart = mainPart.substring(atIndex + 1)

    let username = ''
    let password = ''

    if (isPlainAuth) {
      // socks5:// 格式: user:password 是明文
      const colonIndex = authPart.indexOf(':')
      if (colonIndex !== -1) {
        username = decodeURIComponent(authPart.substring(0, colonIndex))
        password = decodeURIComponent(authPart.substring(colonIndex + 1))
      } else {
        username = decodeURIComponent(authPart)
      }
    } else {
      // socks:// 格式: user:password 是 base64 编码的
      const decoded = base64Decode(authPart)
      const colonIndex = decoded.indexOf(':')
      if (colonIndex !== -1) {
        username = decoded.substring(0, colonIndex)
        password = decoded.substring(colonIndex + 1)
      } else {
        username = decoded
      }
    }

    // 解析 server:port (支持 IPv6)
    let server = ''
    let port = 0

    if (serverPart.startsWith('[')) {
      // IPv6 地址格式: [ipv6]:port
      const closeBracketIndex = serverPart.indexOf(']')
      if (closeBracketIndex !== -1) {
        server = serverPart.substring(1, closeBracketIndex)
        const portPart = serverPart.substring(closeBracketIndex + 1)
        port = parseInt(portPart.replace(':', '')) || 0
      }
    } else {
      // IPv4 或域名
      const lastColonIndex = serverPart.lastIndexOf(':')
      if (lastColonIndex !== -1) {
        server = serverPart.substring(0, lastColonIndex)
        port = parseInt(serverPart.substring(lastColonIndex + 1)) || 0
      } else {
        server = serverPart
      }
    }

    // 如果没有名称，自动生成
    if (!name) {
      name = `${server}:${port}`
    }

    return {
      name,
      type: 'socks5',
      server,
      port,
      username,
      password,
      udp: true  // SOCKS5 默认支持 UDP
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
    // 去掉尾部的 / (anytls URL 格式: anytls://password@server:port/?params)
    authAndServer = authAndServer.replace(/\/$/, '')

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
        const parsedPort = parseInt(portPart.replace(':', '')) || 0
        // 如果没有端口，根据协议设置默认端口
        port = parsedPort || ((protocol === 'anytls' || protocol === 'trojan' || protocol === 'vless') ? 443 : 0)

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
      // 如果有端口部分则解析，否则根据协议设置默认端口
      if (parts.length > 1) {
        port = parseInt(parts[1]) || 0
      } else {
        // 默认端口：anytls/trojan/vless 等 TLS 协议默认 443
        port = (protocol === 'anytls' || protocol === 'trojan' || protocol === 'vless') ? 443 : 0
      }
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

        // 传输层配置（对 path 和 host 额外解码，处理双重编码情况）
        if (queryParams.type === 'ws') {
          node['ws-opts'] = {
            path: safeDecodeURIComponent(queryParams.path) || '/',
            headers: queryParams.host ? { Host: safeDecodeURIComponent(queryParams.host) } : {}
          }
        } else if (queryParams.type === 'grpc') {
          node['grpc-opts'] = {
            'grpc-service-name': safeDecodeURIComponent(queryParams.serviceName || queryParams.path) || ''
          }
        } else if (queryParams.type === 'h2' || queryParams.type === 'http') {
          node['h2-opts'] = {
            host: queryParams.host ? [safeDecodeURIComponent(queryParams.host)] : [],
            path: safeDecodeURIComponent(queryParams.path) || '/'
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

        // UDP 支持 - 如果 URL 中明确指定了 udp 参数，使用指定的值；否则默认为 true
        if (queryParams.udp !== undefined) {
          node.udp = queryParams.udp === 'true' || queryParams.udp === '1'
        } else {
          node.udp = true
        }
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

        // 传输层配置（对 path 和 host 额外解码，处理双重编码情况）
        if (queryParams.type === 'ws') {
          node['ws-opts'] = {
            path: safeDecodeURIComponent(queryParams.path) || '/',
            headers: queryParams.host ? { Host: safeDecodeURIComponent(queryParams.host) } : {}
          }
        } else if (queryParams.type === 'grpc') {
          node['grpc-opts'] = {
            'grpc-service-name': safeDecodeURIComponent(queryParams.serviceName || queryParams.path) || ''
          }
        } else if (queryParams.type === 'h2' || queryParams.type === 'http') {
          node['h2-opts'] = {
            host: queryParams.host ? [safeDecodeURIComponent(queryParams.host)] : [],
            path: safeDecodeURIComponent(queryParams.path) || '/'
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

        // UDP 支持 - 如果 URL 中明确指定了 udp 参数，使用指定的值；否则默认为 true
        if (queryParams.udp !== undefined) {
          node.udp = queryParams.udp === 'true' || queryParams.udp === '1'
        } else {
          node.udp = true
        }
        break

      case 'hysteria':
      case 'hysteria2':
        node.password = password // Hysteria2 使用 password 字段
        node.auth = password // 内部临时字段，用于传递认证信息
        // node.ports = queryParams.mport || port.toString()
        node.obfs = queryParams.obfs
        node['obfs-password'] = queryParams['obfs-password'] || queryParams.obfsParam
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

        // UDP 支持 - 如果 URL 中明确指定了 udp 参数，使用指定的值；否则默认为 true（基于 QUIC）
        if (queryParams.udp !== undefined) {
          node.udp = queryParams.udp === 'true' || queryParams.udp === '1'
        } else {
          node.udp = true
        }
        break

      case 'tuic':
        // TUIC 格式: tuic://uuid:password@server:port?params#name
        // 注意: `:` 可能被 URL 编码为 `%3A`
        {
          const decodedAuth = safeDecodeURIComponent(password)
          const colonIndex = decodedAuth.indexOf(':')
          if (colonIndex !== -1) {
            node.uuid = decodedAuth.substring(0, colonIndex)
            node.password = decodedAuth.substring(colonIndex + 1)
          } else {
            // 兼容旧格式：只有 uuid，password 在查询参数中
            node.uuid = decodedAuth
            node.password = queryParams.password || ''
          }
        }
        node.sni = queryParams.sni || server
        node.alpn = queryParams.alpn ? queryParams.alpn.split(',') : ['h3']
        node.skipCertVerify = queryParams.allowInsecure === '1' || queryParams.allow_insecure === '1'
        node['congestion-controller'] = queryParams.congestion_control || 'bbr'
        node['udp-relay-mode'] = queryParams.udp_relay_mode || 'native'

        // UDP 支持 - 如果 URL 中明确指定了 udp 参数，使用指定的值；否则默认为 true（基于 QUIC）
        if (queryParams.udp !== undefined) {
          node.udp = queryParams.udp === 'true' || queryParams.udp === '1'
        } else {
          node.udp = true
        }
        break

      case 'anytls':
        node.password = password
        node.sni = queryParams.peer || queryParams.sni || (server.startsWith('[') ? '' : server)
        node.alpn = queryParams.alpn ? queryParams.alpn.split(',') : undefined
        node.skipCertVerify = queryParams.insecure === '1' || queryParams.allowInsecure === '1' || queryParams['skip-cert-verify'] === '1'
        // client-fingerprint
        if (queryParams.fp) {
          node.fp = queryParams.fp
        }
        // UDP 支持 - 如果 URL 中明确指定了 udp 参数，使用指定的值；否则默认为 true
        if (queryParams.udp !== undefined) {
          node.udp = queryParams.udp === 'true' || queryParams.udp === '1'
        } else {
          node.udp = true
        }
        // anytls 特有参数 - idle session 相关
        if (queryParams.idleSessionCheckInterval) {
          node['idle-session-check-interval'] = parseInt(queryParams.idleSessionCheckInterval)
        }
        if (queryParams.idleSessionTimeout) {
          node['idle-session-timeout'] = parseInt(queryParams.idleSessionTimeout)
        }
        if (queryParams.minIdleSession) {
          node['min-idle-session'] = parseInt(queryParams.minIdleSession)
        }
        break
    }

    return node
  } catch (e) {
    toast(`Parse ${protocol} error: ${e instanceof Error ? e.message : String(e)}`)
    return null
  }
}

/**
 * 检查是否是 IPv4 地址
 */
function isIPv4(str: string): boolean {
  const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}$/
  if (!ipv4Regex.test(str)) return false
  const parts = str.split('.')
  return parts.every(part => {
    const num = parseInt(part, 10)
    return num >= 0 && num <= 255
  })
}

/**
 * 检查是否是 IPv6 地址
 */
function isIPv6(str: string): boolean {
  // 简化的 IPv6 检测
  return /^([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}$/.test(str) ||
         /^::([0-9a-fA-F]{1,4}:){0,5}[0-9a-fA-F]{1,4}$/.test(str) ||
         /^([0-9a-fA-F]{1,4}:){1,6}:$/.test(str) ||
         /^::$/.test(str)
}

/**
 * 解析 WireGuard 协议
 * 格式: wireguard://privateKey@server:port/?publickey=xxx&address=xxx&reserved=xxx#name
 * 或: wg://privateKey@server:port/?publickey=xxx&address=xxx&reserved=xxx#name
 */
function parseWireGuard(url: string): ProxyNode | null {
  try {
    // 移除协议前缀
    const content = url.replace(/^(wireguard|wg):\/\//, '')

    // 使用正则解析 URL 各部分
    const match = /^((.*?)@)?(.*?)(:(\d+))?\/?(\?(.*?))?(?:#(.*?))?$/.exec(content)
    if (!match) return null

    let privateKey = match[2] || ''
    const server = match[3] || ''
    let port = parseInt(match[5], 10)
    const addons = match[7] || ''
    let name = match[8]

    // 默认端口
    if (isNaN(port)) {
      port = 51820
    }

    // 解码私钥
    privateKey = decodeURIComponent(privateKey)

    // 解码名称
    if (name != null) {
      name = decodeURIComponent(name)
    }
    name = name ?? `WireGuard ${server}:${port}`

    const node: ProxyNode = {
      type: 'wireguard',
      name,
      server,
      port,
      'private-key': privateKey,
      udp: true,
    }

    // 解析附加参数
    for (const addon of addons.split('&')) {
      if (addon) {
        let [key, value] = addon.split('=')
        key = key.replace(/_/g, '-')
        value = decodeURIComponent(value || '')

        if (['reserved'].includes(key)) {
          // reserved 参数是一个数组，格式：reserved=1,2,3
          const parsed = value
            .split(',')
            .map(i => parseInt(i.trim(), 10))
            .filter(i => Number.isInteger(i))
          if (parsed.length === 3) {
            node[key] = parsed
          }
        } else if (['address', 'ip'].includes(key)) {
          // address 参数可能包含 IPv4 和 IPv6，格式：address=10.0.0.1/32,fd00::1/128
          value.split(',').forEach(i => {
            const ip = i
              .trim()
              .replace(/\/\d+$/, '')  // 移除 CIDR 后缀
              .replace(/^\[/, '')     // 移除 IPv6 方括号
              .replace(/\]$/, '')
            if (isIPv4(ip)) {
              node.ip = ip
            } else if (isIPv6(ip)) {
              node.ipv6 = ip
            }
          })
        } else if (['mtu'].includes(key)) {
          const parsed = parseInt(value.trim(), 10)
          if (Number.isInteger(parsed)) {
            node[key] = parsed
          }
        } else if (/publickey/i.test(key)) {
          node['public-key'] = value
        } else if (/privatekey/i.test(key)) {
          node['private-key'] = value
        } else if (['udp'].includes(key)) {
          // UDP 参数：支持 true/1 或 false/0
          node.udp = /(true)|1/i.test(value)
        } else if (!['name', 'type', 'server', 'port', 'private-key', 'flag'].includes(key)) {
          // 其他未知参数直接添加
          node[key] = value
        }
      }
    }

    return node
  } catch (e) {
    toast(`Parse WireGuard error: ${e instanceof Error ? e.message : String(e)}`)
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
  } else if (url.startsWith('ssr://')) {
    return parseShadowsocksR(url)
  } else if (url.startsWith('ss://')) {
    return parseShadowsocks(url)
  } else if (url.startsWith('socks://') || url.startsWith('socks5://')) {
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
  } else if (url.startsWith('anytls://')) {
    return parseGenericProtocol(url, 'anytls')
  } else if (url.startsWith('wireguard://') || url.startsWith('wg://')) {
    return parseWireGuard(url)
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
    'fingerprint', // 已转换为 client-fingerprint
    'client-fingerprint' // SS plugin 中的 client-fingerprint，已单独处理
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
  } else if (node.type === 'hysteria2' || node.type === 'hysteria' || node.type === 'anytls') {
    // Hysteria/Hysteria2/AnyTLS 使用 password
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
  if (node.type === 'hysteria' || node.type === 'hysteria2' || node.type === 'trojan' || node.type === 'tuic' || node.type === 'anytls') {
    if (node.sni && node.sni !== node.server) {
      clash.sni = node.sni
    }
  }

  // Client Fingerprint (注意是 client-fingerprint 不是 fingerprint)
  if (node.fp || node.fingerprint || node['client-fingerprint']) {
    clash['client-fingerprint'] = (node['client-fingerprint'] || node.fingerprint || node.fp) as string
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

  // AnyTLS 专用字段
  if (node.type === 'anytls') {
    if (node['idle-session-check-interval'] !== undefined) {
      clash['idle-session-check-interval'] = node['idle-session-check-interval']
    }
    if (node['idle-session-timeout'] !== undefined) {
      clash['idle-session-timeout'] = node['idle-session-timeout']
    }
    if (node['min-idle-session'] !== undefined) {
      clash['min-idle-session'] = node['min-idle-session']
    }
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
