import { isPresent } from '@/lib/substore/producers/utils'

type IpVersion = 'dual' | 'v4-only' | 'v6-only' | 'prefer-v4' | 'prefer-v6'
type ClashIpVersion = 'dual' | 'ipv4' | 'ipv6' | 'ipv4-prefer' | 'ipv6-prefer'

const ipVersions: Record<IpVersion, ClashIpVersion> = {
  dual: 'dual',
  'v4-only': 'ipv4',
  'v6-only': 'ipv6',
  'prefer-v4': 'ipv4-prefer',
  'prefer-v6': 'ipv6-prefer',
}

interface ProduceOptions {
  'include-unsupported-proxy'?: boolean
  'delete-underscore-fields'?: boolean
}

interface Proxy {
  type: string
  name?: string
  server?: string
  port?: number
  cipher?: string
  password?: string
  version?: number
  network?: string
  aead?: boolean
  alterId?: number
  sni?: string
  servername?: string
  alpn?: string | string[]
  tfo?: boolean
  'fast-open'?: boolean
  token?: string
  'auth_str'?: string
  'auth-str'?: string
  keepalive?: number
  'persistent-keepalive'?: number
  'preshared-key'?: string
  'pre-shared-key'?: string
  udp?: boolean
  'shadow-tls-password'?: string
  'shadow-tls-sni'?: string
  'shadow-tls-version'?: number
  plugin?: string
  'plugin-opts'?: Record<string, unknown>
  'http-opts'?: {
    path?: string | string[]
    headers?: {
      Host?: string | string[]
    }
  }
  'h2-opts'?: {
    path?: string | string[]
    headers?: {
      host?: string | string[]
      Host?: string | string[]
    }
  }
  'ws-opts'?: {
    path?: string
    'early-data-header-name'?: string
    'max-early-data'?: number
  }
  'grpc-opts'?: {
    '_grpc-type'?: string
    '_grpc-authority'?: string
  }
  'reality-opts'?: Record<string, unknown>
  tls?: boolean | unknown
  'skip-cert-verify'?: boolean
  'tls-fingerprint'?: string
  fingerprint?: string
  'underlying-proxy'?: string
  'dialer-proxy'?: string
  'ip-version'?: string
  subName?: string
  collectionName?: string
  id?: string | number
  resolved?: boolean
  'no-resolve'?: boolean
  [key: string]: unknown
}

interface Producer {
  type: string
  produce: (proxies: Proxy[], type: string, opts?: ProduceOptions) => Proxy[] | string
}

export default function ClashMeta_Producer(): Producer {
  const type = 'ALL'
  const produce = (proxies: Proxy[], type: string, opts: ProduceOptions = {}): Proxy[] | string => {
    const list = proxies
      .filter((proxy) => {
        if (opts['include-unsupported-proxy']) return true
        if (proxy.type === 'snell' && (proxy.version ?? 0) >= 4) {
          return false
        } else if (['juicity'].includes(proxy.type)) {
          return false
        } else if (
          ['ss'].includes(proxy.type) &&
          ![
            'aes-128-ctr',
            'aes-192-ctr',
            'aes-256-ctr',
            'aes-128-cfb',
            'aes-192-cfb',
            'aes-256-cfb',
            'aes-128-gcm',
            'aes-192-gcm',
            'aes-256-gcm',
            'aes-128-ccm',
            'aes-192-ccm',
            'aes-256-ccm',
            'aes-128-gcm-siv',
            'aes-256-gcm-siv',
            'chacha20-ietf',
            'chacha20',
            'xchacha20',
            'chacha20-ietf-poly1305',
            'xchacha20-ietf-poly1305',
            'chacha8-ietf-poly1305',
            'xchacha8-ietf-poly1305',
            '2022-blake3-aes-128-gcm',
            '2022-blake3-aes-256-gcm',
            '2022-blake3-chacha20-poly1305',
            'lea-128-gcm',
            'lea-192-gcm',
            'lea-256-gcm',
            'rabbit128-poly1305',
            'aegis-128l',
            'aegis-256',
            'aez-384',
            'deoxys-ii-256-128',
            'rc4-md5',
            'none',
          ].includes(proxy.cipher as string)
        ) {
          // https://wiki.metacubex.one/config/proxies/ss/#cipher
          return false
        } else if (
          ['anytls'].includes(proxy.type) &&
          proxy.network &&
          (!['tcp'].includes(proxy.network) ||
            (['tcp'].includes(proxy.network) && proxy['reality-opts']))
        ) {
          return false
        } else if (['xhttp'].includes(proxy.network as string)) {
          return false
        }
        return true
      })
      .map((proxy) => {
        if (proxy.type === 'vmess') {
          // handle vmess aead
          if (isPresent(proxy, 'aead')) {
            if (proxy.aead) {
              proxy.alterId = 0
            }
            delete proxy.aead
          }
          if (isPresent(proxy, 'sni')) {
            proxy.servername = proxy.sni
            delete proxy.sni
          }
          // https://github.com/MetaCubeX/Clash.Meta/blob/Alpha/docs/config.yaml#L400
          // https://stash.wiki/proxy-protocols/proxy-types#vmess
          if (
            isPresent(proxy, 'cipher') &&
            !['auto', 'none', 'zero', 'aes-128-gcm', 'chacha20-poly1305'].includes(
              proxy.cipher as string
            )
          ) {
            proxy.cipher = 'auto'
          }
        } else if (proxy.type === 'tuic') {
          if (isPresent(proxy, 'alpn')) {
            proxy.alpn = Array.isArray(proxy.alpn) ? proxy.alpn : [proxy.alpn as string]
          }
          //  else {
          //     proxy.alpn = ['h3'];
          // }
          if (isPresent(proxy, 'tfo') && !isPresent(proxy, 'fast-open')) {
            proxy['fast-open'] = proxy.tfo
          }
          // https://github.com/MetaCubeX/Clash.Meta/blob/Alpha/adapter/outbound/tuic.go#L197
          if (
            (!proxy.token || (proxy.token as string).length === 0) &&
            !isPresent(proxy, 'version')
          ) {
            proxy.version = 5
          }
        } else if (proxy.type === 'hysteria') {
          // auth_str 将会在未来某个时候删除 但是有的机场不规范
          if (isPresent(proxy, 'auth_str') && !isPresent(proxy, 'auth-str')) {
            proxy['auth-str'] = proxy['auth_str']
          }
          if (isPresent(proxy, 'alpn')) {
            proxy.alpn = Array.isArray(proxy.alpn) ? proxy.alpn : [proxy.alpn as string]
          }
          if (isPresent(proxy, 'tfo') && !isPresent(proxy, 'fast-open')) {
            proxy['fast-open'] = proxy.tfo
          }
        } else if (proxy.type === 'wireguard') {
          proxy.keepalive = proxy.keepalive ?? proxy['persistent-keepalive']
          proxy['persistent-keepalive'] = proxy.keepalive
          proxy['preshared-key'] = proxy['preshared-key'] ?? proxy['pre-shared-key']
          proxy['pre-shared-key'] = proxy['preshared-key']
        } else if (proxy.type === 'snell' && (proxy.version ?? 0) < 3) {
          delete proxy.udp
        } else if (proxy.type === 'vless') {
          if (isPresent(proxy, 'sni')) {
            proxy.servername = proxy.sni
            delete proxy.sni
          }
        } else if (proxy.type === 'ss') {
          if (isPresent(proxy, 'shadow-tls-password') && !isPresent(proxy, 'plugin')) {
            proxy.plugin = 'shadow-tls'
            proxy['plugin-opts'] = {
              host: proxy['shadow-tls-sni'],
              password: proxy['shadow-tls-password'],
              version: proxy['shadow-tls-version'],
            }
            delete proxy['shadow-tls-password']
            delete proxy['shadow-tls-sni']
            delete proxy['shadow-tls-version']
          }
        }

        if (['vmess', 'vless'].includes(proxy.type) && proxy.network === 'http') {
          const httpPath = proxy['http-opts']?.path
          if (isPresent(proxy, 'http-opts.path') && !Array.isArray(httpPath)) {
            proxy['http-opts']!.path = [httpPath as string]
          }
          const httpHost = proxy['http-opts']?.headers?.Host
          if (isPresent(proxy, 'http-opts.headers.Host') && !Array.isArray(httpHost)) {
            proxy['http-opts']!.headers!.Host = [httpHost as string]
          }
        }
        if (['vmess', 'vless'].includes(proxy.type) && proxy.network === 'h2') {
          const path = proxy['h2-opts']?.path
          if (isPresent(proxy, 'h2-opts.path') && Array.isArray(path)) {
            proxy['h2-opts']!.path = path[0]
          }
          const host = proxy['h2-opts']?.headers?.host
          if (isPresent(proxy, 'h2-opts.headers.Host') && !Array.isArray(host)) {
            proxy['h2-opts']!.headers!.host = [host as string]
          }
        }
        if (['ws'].includes(proxy.network as string)) {
          const networkPath = (proxy[`${proxy.network}-opts`] as Record<string, unknown>)?.path as
            | string
            | undefined
          if (networkPath) {
            const reg = /^(.*?)(?:\?ed=(\d+))?$/
             
            const [_, path = '', ed = ''] = reg.exec(networkPath) ?? []
            ;(proxy[`${proxy.network}-opts`] as Record<string, unknown>).path = path
            if (ed !== '') {
              proxy['ws-opts']!['early-data-header-name'] = 'Sec-WebSocket-Protocol'
              proxy['ws-opts']!['max-early-data'] = parseInt(ed, 10)
            }
          } else {
            proxy[`${proxy.network}-opts`] = proxy[`${proxy.network}-opts`] || {}
            ;(proxy[`${proxy.network}-opts`] as Record<string, unknown>).path = '/'
          }
        }

        if ((proxy['plugin-opts'] as Record<string, unknown>)?.tls) {
          if (isPresent(proxy, 'skip-cert-verify')) {
            ;(proxy['plugin-opts'] as Record<string, unknown>)['skip-cert-verify'] =
              proxy['skip-cert-verify']
          }
        }
        if (
          ['trojan', 'tuic', 'hysteria', 'hysteria2', 'juicity', 'anytls'].includes(proxy.type)
        ) {
          delete proxy.tls
        }

        if (proxy['tls-fingerprint']) {
          proxy.fingerprint = proxy['tls-fingerprint']
        }
        delete proxy['tls-fingerprint']

        if (proxy['underlying-proxy']) {
          proxy['dialer-proxy'] = proxy['underlying-proxy']
        }
        delete proxy['underlying-proxy']

        if (isPresent(proxy, 'tls') && typeof proxy.tls !== 'boolean') {
          delete proxy.tls
        }
        delete proxy.subName
        delete proxy.collectionName
        delete proxy.id
        delete proxy.resolved
        delete proxy['no-resolve']
        if (type !== 'internal' || opts['delete-underscore-fields']) {
          for (const key in proxy) {
            if (proxy[key] == null || /^_/i.test(key)) {
              delete proxy[key]
            }
          }
        }
        if (['grpc'].includes(proxy.network as string) && proxy[`${proxy.network}-opts`]) {
          delete (proxy[`${proxy.network}-opts`] as Record<string, unknown>)['_grpc-type']
          delete (proxy[`${proxy.network}-opts`] as Record<string, unknown>)['_grpc-authority']
        }

        if (proxy['ip-version']) {
          proxy['ip-version'] =
            ipVersions[proxy['ip-version'] as IpVersion] || proxy['ip-version']
        }
        return proxy
      })

    return type === 'internal'
      ? list
      : 'proxies:\n' + list.map((proxy) => '  - ' + JSON.stringify(proxy) + '\n').join('')
  }
  return { type, produce }
}
