import { toast } from 'sonner';
import { isPresent } from './utils';

type Proxy = Record<string, any>;

interface ProduceOptions {
    'include-unsupported-proxy'?: boolean;
    [key: string]: any;
}

interface Producer {
    type: string;
    produce: (proxies: Proxy[], type?: string, opts?: ProduceOptions) => Proxy[] | string;
}

export default function Clash_Producer(): Producer {
    const type = 'ALL';
    const produce = (proxies: Proxy[], type?: string, opts: ProduceOptions = {}): Proxy[] | string => {
        const list = proxies
            .filter((proxy) => {
                if (opts['include-unsupported-proxy']) return true;
                if (
                    ![
                        'ss',
                        'ssr',
                        'vmess',
                        'vless',
                        'socks5',
                        'http',
                        'snell',
                        'trojan',
                        'wireguard',
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
                        ].includes(proxy.cipher)) ||
                    (proxy.type === 'snell' && (proxy.version ?? 0) >= 4) ||
                    (proxy.type === 'vless' &&
                        (typeof proxy.flow !== 'undefined' ||
                            proxy['reality-opts']))
                ) {
                    return false;
                } else if (proxy['underlying-proxy'] || proxy['dialer-proxy']) {
                    toast(
                        `Clash 不支持前置代理字段. 已过滤节点 ${proxy.name}`,
                    );
                    return false;
                }
                return true;
            })
            .map((proxy) => {
                if (proxy.type === 'vmess') {
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
                    if (
                        isPresent(proxy, 'cipher') &&
                        ![
                            'auto',
                            'aes-128-gcm',
                            'chacha20-poly1305',
                            'none',
                        ].includes(proxy.cipher)
                    ) {
                        proxy.cipher = 'auto';
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
                        proxy['http-opts'].path = [httpPath];
                    }
                    const httpHost = proxy['http-opts']?.headers?.Host;
                    if (
                        isPresent(proxy, 'http-opts.headers.Host') &&
                        !Array.isArray(httpHost)
                    ) {
                        proxy['http-opts'].headers.Host = [httpHost];
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
                        proxy['h2-opts'].path = path[0];
                    }
                    const host = proxy['h2-opts']?.headers?.host;
                    if (
                        isPresent(proxy, 'h2-opts.headers.Host') &&
                        !Array.isArray(host)
                    ) {
                        proxy['h2-opts'].headers.host = [host];
                    }
                }
                if (['ws'].includes(proxy.network)) {
                    const networkPath = proxy[`${proxy.network}-opts`]?.path;
                    if (networkPath) {
                        const reg = /^(.*?)(?:\?ed=(\d+))?$/;
                         
                        const [_, path = '', ed = ''] = reg.exec(networkPath) || [];
                        proxy[`${proxy.network}-opts`].path = path;
                        if (ed !== '') {
                            proxy['ws-opts']['early-data-header-name'] =
                                'Sec-WebSocket-Protocol';
                            proxy['ws-opts']['max-early-data'] = parseInt(
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
                    proxy.fingerprint = proxy['tls-fingerprint'];
                }
                delete proxy['tls-fingerprint'];

                if (isPresent(proxy, 'tls') && typeof proxy.tls !== 'boolean') {
                    delete proxy.tls;
                }

                delete proxy.subName;
                delete proxy.collectionName;
                delete proxy.id;
                delete proxy.resolved;
                delete proxy['no-resolve'];
                if (type !== 'internal') {
                    for (const key in proxy) {
                        if (proxy[key] == null || /^_/i.test(key)) {
                            delete proxy[key];
                        }
                    }
                }
                if (
                    ['grpc'].includes(proxy.network) &&
                    proxy[`${proxy.network}-opts`]
                ) {
                    delete proxy[`${proxy.network}-opts`]['_grpc-type'];
                    delete proxy[`${proxy.network}-opts`]['_grpc-authority'];
                }
                return proxy;
            });
        return type === 'internal'
            ? list
            : 'proxies:\n' +
                  list
                      .map((proxy) => '  - ' + JSON.stringify(proxy) + '\n')
                      .join('');
    };
    return { type, produce };
}
