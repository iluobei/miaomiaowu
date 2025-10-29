import { isPresent } from '@/lib/substore/producers/utils';
import { toast } from 'sonner';

interface Proxy {
    type: string;
    name?: string;
    cipher?: string;
    version?: number;
    'reality-opts'?: any;
    flow?: string;
    'underlying-proxy'?: string;
    'dialer-proxy'?: string;
    aead?: boolean;
    alterId?: number;
    sni?: string;
    servername?: string;
    alpn?: string | string[];
    token?: string;
    'auth_str'?: string;
    'auth-str'?: string;
    tfo?: boolean;
    'fast-open'?: boolean;
    down?: string;
    'down-speed'?: string;
    up?: string;
    'up-speed'?: string;
    password?: string;
    auth?: string;
    keepalive?: number;
    'persistent-keepalive'?: number;
    'preshared-key'?: string;
    'pre-shared-key'?: string;
    udp?: boolean;
    network?: string;
    'http-opts'?: {
        path?: string | string[];
        headers?: {
            Host?: string | string[];
        };
    };
    'h2-opts'?: {
        path?: string | string[];
        headers?: {
            host?: string | string[];
            Host?: string | string[];
        };
    };
    'ws-opts'?: {
        path?: string;
        'early-data-header-name'?: string;
        'max-early-data'?: number;
    };
    'plugin-opts'?: {
        tls?: boolean;
        'skip-cert-verify'?: boolean;
    };
    'skip-cert-verify'?: boolean;
    tls?: boolean | unknown;
    'grpc-opts'?: {
        '_grpc-type'?: string;
        '_grpc-authority'?: string;
    };
    'tls-fingerprint'?: string;
    'server-cert-fingerprint'?: string;
    'test-url'?: string;
    'benchmark-url'?: string;
    'test-timeout'?: number;
    'benchmark-timeout'?: number;
    subName?: string;
    collectionName?: string;
    id?: string | number;
    resolved?: boolean;
    'no-resolve'?: boolean;
    [key: string]: any;
}

interface ProduceOptions {
    [key: string]: any;
}

interface Producer {
    type: string;
    produce: (proxies: Proxy[], type?: string, opts?: ProduceOptions) => Proxy[] | string;
}

export default function Stash_Producer(): Producer {
    const type = 'ALL';
    const produce = (proxies: Proxy[], _type?: string, _opts: ProduceOptions = {}): Proxy[] | string => {
        // https://stash.wiki/proxy-protocols/proxy-types#shadowsocks
        const list = proxies
            .filter((proxy) => {
                if (
                    ![
                        'ss',
                        'ssr',
                        'vmess',
                        'socks5',
                        'http',
                        'snell',
                        'trojan',
                        'tuic',
                        'vless',
                        'wireguard',
                        'hysteria',
                        'hysteria2',
                        'ssh',
                        'juicity',
                    ].includes(proxy.type) ||
                    (proxy.type === 'ss' &&
                        ![
                            'aes-128-gcm',
                            'aes-192-gcm',
                            'aes-256-gcm',
                            'aes-128-cfb',
                            'aes-192-cfb',
                            'aes-256-cfb',
                            'aes-128-ctr',
                            'aes-192-ctr',
                            'aes-256-ctr',
                            'rc4-md5',
                            'chacha20-ietf',
                            'xchacha20',
                            'chacha20-ietf-poly1305',
                            'xchacha20-ietf-poly1305',
                            '2022-blake3-aes-128-gcm',
                            '2022-blake3-aes-256-gcm',
                        ].includes(proxy.cipher || '')) ||
                    (proxy.type === 'snell' && (proxy.version ?? 0) >= 4) ||
                    (proxy.type === 'vless' &&
                        proxy['reality-opts'] &&
                        !['xtls-rprx-vision'].includes(proxy.flow || ''))
                ) {
                    return false;
                } else if (proxy['underlying-proxy'] || proxy['dialer-proxy']) {
                    toast(
                        `Stash 暂不支持前置代理字段. 已过滤节点 ${proxy.name}. 请使用 代理的转发链 https://stash.wiki/proxy-protocols/proxy-groups#relay`,
                    );
                    return false;
                }
                return true;
            })
            .map((proxy) => {
                if (proxy.type === 'vmess') {
                    // handle vmess aead
                    if (isPresent(proxy, 'aead')) {
                        if (proxy.aead) {
                            proxy.alterId = 0;
                        }
                        delete proxy.aead;
                    }
                    if (isPresent(proxy, 'sni')) {
                        proxy.servername = proxy.sni;
                        delete proxy.sni;
                    }
                    // https://github.com/MetaCubeX/Clash.Meta/blob/Alpha/docs/config.yaml#L400
                    // https://stash.wiki/proxy-protocols/proxy-types#vmess
                    if (
                        isPresent(proxy, 'cipher') &&
                        ![
                            'auto',
                            'aes-128-gcm',
                            'chacha20-poly1305',
                            'none',
                        ].includes(proxy.cipher || '')
                    ) {
                        proxy.cipher = 'auto';
                    }
                } else if (proxy.type === 'tuic') {
                    if (isPresent(proxy, 'alpn')) {
                        proxy.alpn = Array.isArray(proxy.alpn)
                            ? proxy.alpn
                            : [proxy.alpn as string];
                    } else {
                        proxy.alpn = ['h3'];
                    }
                    if (
                        isPresent(proxy, 'tfo') &&
                        !isPresent(proxy, 'fast-open')
                    ) {
                        proxy['fast-open'] = proxy.tfo;
                        delete proxy.tfo;
                    }
                    // https://github.com/MetaCubeX/Clash.Meta/blob/Alpha/adapter/outbound/tuic.go#L197
                    if (
                        (!proxy.token || proxy.token.length === 0) &&
                        !isPresent(proxy, 'version')
                    ) {
                        proxy.version = 5;
                    }
                } else if (proxy.type === 'hysteria') {
                    // auth_str 将会在未来某个时候删除 但是有的机场不规范
                    if (
                        isPresent(proxy, 'auth_str') &&
                        !isPresent(proxy, 'auth-str')
                    ) {
                        proxy['auth-str'] = proxy['auth_str'];
                    }
                    if (isPresent(proxy, 'alpn')) {
                        proxy.alpn = Array.isArray(proxy.alpn)
                            ? proxy.alpn
                            : [proxy.alpn as string];
                    }
                    if (
                        isPresent(proxy, 'tfo') &&
                        !isPresent(proxy, 'fast-open')
                    ) {
                        proxy['fast-open'] = proxy.tfo;
                        delete proxy.tfo;
                    }
                    if (
                        isPresent(proxy, 'down') &&
                        !isPresent(proxy, 'down-speed')
                    ) {
                        proxy['down-speed'] = proxy.down;
                        delete proxy.down;
                    }
                    if (
                        isPresent(proxy, 'up') &&
                        !isPresent(proxy, 'up-speed')
                    ) {
                        proxy['up-speed'] = proxy.up;
                        delete proxy.up;
                    }
                    if (isPresent(proxy, 'down-speed')) {
                        proxy['down-speed'] =
                            `${proxy['down-speed']}`.match(/\d+/)?.[0] || '0';
                    }
                    if (isPresent(proxy, 'up-speed')) {
                        proxy['up-speed'] =
                            `${proxy['up-speed']}`.match(/\d+/)?.[0] || '0';
                    }
                } else if (proxy.type === 'hysteria2') {
                    if (
                        isPresent(proxy, 'password') &&
                        !isPresent(proxy, 'auth')
                    ) {
                        proxy.auth = proxy.password;
                        delete proxy.password;
                    }
                    if (
                        isPresent(proxy, 'tfo') &&
                        !isPresent(proxy, 'fast-open')
                    ) {
                        proxy['fast-open'] = proxy.tfo;
                        delete proxy.tfo;
                    }
                    if (
                        isPresent(proxy, 'down') &&
                        !isPresent(proxy, 'down-speed')
                    ) {
                        proxy['down-speed'] = proxy.down;
                        delete proxy.down;
                    }
                    if (
                        isPresent(proxy, 'up') &&
                        !isPresent(proxy, 'up-speed')
                    ) {
                        proxy['up-speed'] = proxy.up;
                        delete proxy.up;
                    }
                    if (isPresent(proxy, 'down-speed')) {
                        proxy['down-speed'] =
                            `${proxy['down-speed']}`.match(/\d+/)?.[0] || '0';
                    }
                    if (isPresent(proxy, 'up-speed')) {
                        proxy['up-speed'] =
                            `${proxy['up-speed']}`.match(/\d+/)?.[0] || '0';
                    }
                } else if (proxy.type === 'wireguard') {
                    proxy.keepalive =
                        proxy.keepalive ?? proxy['persistent-keepalive'];
                    proxy['persistent-keepalive'] = proxy.keepalive;
                    proxy['preshared-key'] =
                        proxy['preshared-key'] ?? proxy['pre-shared-key'];
                    proxy['pre-shared-key'] = proxy['preshared-key'];
                } else if (proxy.type === 'snell' && (proxy.version ?? 0) < 3) {
                    delete proxy.udp;
                } else if (proxy.type === 'vless') {
                    if (isPresent(proxy, 'sni')) {
                        proxy.servername = proxy.sni;
                        delete proxy.sni;
                    }
                }

                if (
                    ['vmess', 'vless'].includes(proxy.type) &&
                    proxy.network === 'http'
                ) {
                    const httpPath = proxy['http-opts']?.path;
                    if (
                        isPresent(proxy, 'http-opts.path') &&
                        !Array.isArray(httpPath)
                    ) {
                        proxy['http-opts']!.path = [httpPath as string];
                    }
                    const httpHost = proxy['http-opts']?.headers?.Host;
                    if (
                        isPresent(proxy, 'http-opts.headers.Host') &&
                        !Array.isArray(httpHost)
                    ) {
                        proxy['http-opts']!.headers!.Host = [httpHost as string];
                    }
                }
                if (
                    ['vmess', 'vless'].includes(proxy.type) &&
                    proxy.network === 'h2'
                ) {
                    const path = proxy['h2-opts']?.path;
                    if (
                        isPresent(proxy, 'h2-opts.path') &&
                        Array.isArray(path)
                    ) {
                        proxy['h2-opts']!.path = path[0];
                    }
                    const host = proxy['h2-opts']?.headers?.host;
                    if (
                        isPresent(proxy, 'h2-opts.headers.Host') &&
                        !Array.isArray(host)
                    ) {
                        proxy['h2-opts']!.headers!.host = [host as string];
                    }
                }
                if (['ws'].includes(proxy.network || '')) {
                    const networkPath = proxy[`${proxy.network}-opts`]?.path;
                    if (networkPath) {
                        const reg = /^(.*?)(?:\?ed=(\d+))?$/;
                         
                        const [_, path = '', ed = ''] = reg.exec(networkPath) || [];
                        proxy[`${proxy.network}-opts`].path = path;
                        if (ed !== '') {
                            proxy['ws-opts']!['early-data-header-name'] =
                                'Sec-WebSocket-Protocol';
                            proxy['ws-opts']!['max-early-data'] = parseInt(
                                ed,
                                10,
                            );
                        }
                    } else {
                        proxy[`${proxy.network}-opts`] =
                            proxy[`${proxy.network}-opts`] || {};
                        proxy[`${proxy.network}-opts`].path = '/';
                    }
                }
                if (proxy['plugin-opts']?.tls) {
                    if (isPresent(proxy, 'skip-cert-verify')) {
                        proxy['plugin-opts']['skip-cert-verify'] =
                            proxy['skip-cert-verify'];
                    }
                }
                if (
                    [
                        'trojan',
                        'tuic',
                        'hysteria',
                        'hysteria2',
                        'juicity',
                        'anytls',
                    ].includes(proxy.type)
                ) {
                    delete proxy.tls;
                }
                if (proxy['tls-fingerprint']) {
                    proxy['server-cert-fingerprint'] = proxy['tls-fingerprint'];
                }
                delete proxy['tls-fingerprint'];

                if (isPresent(proxy, 'tls') && typeof proxy.tls !== 'boolean') {
                    delete proxy.tls;
                }

                if (proxy['test-url']) {
                    proxy['benchmark-url'] = proxy['test-url'];
                    delete proxy['test-url'];
                }
                if (proxy['test-timeout']) {
                    proxy['benchmark-timeout'] = proxy['test-timeout'];
                    delete proxy['test-timeout'];
                }

                delete proxy.subName;
                delete proxy.collectionName;
                delete proxy.id;
                delete proxy.resolved;
                delete proxy['no-resolve'];
                if (_type !== 'internal') {
                    for (const key in proxy) {
                        if (proxy[key] == null || /^_/i.test(key)) {
                            delete proxy[key];
                        }
                    }
                }
                if (
                    ['grpc'].includes(proxy.network || '') &&
                    proxy[`${proxy.network}-opts`]
                ) {
                    delete proxy[`${proxy.network}-opts`]['_grpc-type'];
                    delete proxy[`${proxy.network}-opts`]['_grpc-authority'];
                }
                return proxy;
            });
        return _type === 'internal'
            ? list
            : 'proxies:\n' +
                  list
                      .map((proxy) => '  - ' + JSON.stringify(proxy) + '\n')
                      .join('');
    };
    return { type, produce };
}
