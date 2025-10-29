// Utility functions for sublink-worker

export function decodeBase64(str: string): string {
  try {
    return decodeURIComponent(
      atob(str)
        .split('')
        .map((c) => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2))
        .join('')
    )
  } catch (e) {
    console.error('Failed to decode base64:', e)
    return ''
  }
}

export function encodeBase64(str: string): string {
  try {
    return btoa(
      encodeURIComponent(str).replace(/%([0-9A-F]{2})/g, (_match, p1) =>
        String.fromCharCode(parseInt(p1, 16))
      )
    )
  } catch (e) {
    console.error('Failed to encode base64:', e)
    return ''
  }
}

export function base64ToBinary(base64: string): string {
  try {
    return atob(base64.replace(/-/g, '+').replace(/_/g, '/'))
  } catch (e) {
    console.error('Failed to decode base64 to binary:', e)
    return ''
  }
}

export function generateRandomPath(length: number = 6): string {
  const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'
  let result = ''
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  return result
}

export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i]
}

export function parseServerInfo(serverInfo: string): {
  server: string
  port: number
} {
  // Match IPv6 address
  const match = serverInfo.match(/\[([^\]]+)\]:(\d+)/)
  if (match) {
    return { server: match[1], port: parseInt(match[2]) }
  }
  const [server, port] = serverInfo.split(':')
  return { server, port: parseInt(port) }
}

export function parseUrlParams(url: string): Record<string, string> {
  const params: Record<string, string> = {}
  const urlObj = new URL(url)
  urlObj.searchParams.forEach((value, key) => {
    params[key] = value
  })
  return params
}

export function createTlsConfig(params: Record<string, string>): any {
  if (params.security !== 'tls' && params.sni === undefined) {
    return { enabled: false }
  }
  return {
    enabled: true,
    server_name: params.sni || params.host || '',
    insecure: params.allowInsecure === '1' || params.skip_cert_verify === '1',
    alpn: params.alpn ? params.alpn.split(',') : [],
  }
}

export function createTransportConfig(
  type: string,
  params: Record<string, string>
): any {
  if (type === 'ws') {
    return {
      type: 'ws',
      path: params.path || '/',
      headers: {
        Host: params.host || params.sni || '',
      },
    }
  } else if (type === 'grpc') {
    return {
      type: 'grpc',
      service_name: params.serviceName || params.service_name || '',
    }
  } else if (type === 'http') {
    return {
      type: 'http',
      host: [params.host || params.sni || ''],
      path: params.path || '/',
    }
  }
  return { type: 'tcp' }
}

export function deepCopy<T>(obj: T): T {
  return JSON.parse(JSON.stringify(obj))
}
