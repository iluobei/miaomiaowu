import { isPresent } from '@/lib/substore/producers/utils';

interface Proxy {
    type: string;
    name: string;
    server: string;
    port: number;
    cipher?: string;
    password?: string;
    uuid?: string;
    alterId?: number;
    aead?: boolean;
    plugin?: string;
    'plugin-opts'?: Record<string, any>;
    network?: string;
    'ws-opts'?: Record<string, any>;
    'http-opts'?: Record<string, any>;
    'h2-opts'?: Record<string, any>;
    tls?: boolean;
    'skip-cert-verify'?: boolean;
    sni?: string;
    'tls-fingerprint'?: string;
    'tls-pubkey-sha256'?: string;
    tfo?: boolean;
    'fast-open'?: boolean;
    udp?: boolean;
    udp_relay?: boolean;
    username?: string;
    flow?: string;
    'reality-opts'?: Record<string, any>;
    'shadow-tls-password'?: string;
    'shadow-tls-version'?: number;
    'shadow-tls-sni'?: string;
    'udp-port'?: number;
    xudp?: boolean;
    'obfs-password'?: string;
    obfs?: string;
    alpn?: string | string[];
    ports?: string;
    'hop-interval'?: string;
    token?: string;
    websocket?: Record<string, any>;
    transport?: Record<string, any>;
    subName?: string;
    collectionName?: string;
    id?: string;
    resolved?: any;
    'no-resolve'?: any;
    prev_hop?: string;
    'underlying-proxy'?: string;
    'dialer-proxy'?: string;
    detour?: string;
    next_hop?: string;
    [key: string]: any;
}

interface Producer {
    type: string;
    produce: (proxies: Proxy[], type?: string, opts?: Record<string, any>) => string | any[];
}

export default function Egern_Producer(): Producer {
    const type = 'ALL';
    const produce = (proxies: Proxy[], type?: string, opts: Record<string, any> = {}): string | any[] => {
        // https://egernapp.com/zh-CN/docs/configuration/proxies
        const list = proxies
            .filter((proxy) => {
                if (
                    ![
                        'http',
                        'socks5',
                        'ss',
                        'trojan',
                        'hysteria2',
                        'vless',
                        'vmess',
                        'tuic',
                    ].includes(proxy.type) ||
                    (proxy.type === 'ss' &&
                        ((proxy.plugin === 'obfs' &&
                            !['http', 'tls'].includes(
                                proxy['plugin-opts']?.mode,
                            )) ||
                            ![
                                'chacha20-ietf-poly1305',
                                'chacha20-poly1305',
                                'aes-256-gcm',
                                'aes-128-gcm',
                                'none',
                                'tbale',
                                'rc4',
                                'rc4-md5',
                                'aes-128-cfb',
                                'aes-192-cfb',
                                'aes-256-cfb',
                                'aes-128-ctr',
                                'aes-192-ctr',
                                'aes-256-ctr',
                                'bf-cfb',
                                'camellia-128-cfb',
                                'camellia-192-cfb',
                                'camellia-256-cfb',
                                'cast5-cfb',
                                'des-cfb',
                                'idea-cfb',
                                'rc2-cfb',
                                'seed-cfb',
                                'salsa20',
                                'chacha20',
                                'chacha20-ietf',
                                '2022-blake3-aes-128-gcm',
                                '2022-blake3-aes-256-gcm',
                            ].includes(proxy.cipher!))) ||
                    (proxy.type === 'vmess' &&
                        !['http', 'ws', 'tcp'].includes(proxy.network!) &&
                        proxy.network) ||
                    (proxy.type === 'trojan' &&
                        !['http', 'ws', 'tcp'].includes(proxy.network!) &&
                        proxy.network) ||
                    (proxy.type === 'vless' &&
                        ((!opts['include-unsupported-proxy'] &&
                            (typeof proxy.flow !== 'undefined' ||
                                proxy['reality-opts'])) ||
                            (opts['include-unsupported-proxy'] &&
                                typeof proxy.flow !== 'undefined' &&
                                proxy.flow !== 'xtls-rprx-vision') ||
                            (!['http', 'ws', 'tcp'].includes(proxy.network!) &&
                                proxy.network))) ||
                    (proxy.type === 'tuic' &&
                        proxy.token &&
                        proxy.token.length !== 0)
                ) {
                    return false;
                }
                return true;
            })
            .map((proxy) => {
                const original = { ...proxy };
                let flow: string | undefined;
                if (proxy.tls && !proxy.sni) {
                    proxy.sni = proxy.server;
                }
                const prev_hop =
                    proxy.prev_hop ||
                    proxy['underlying-proxy'] ||
                    proxy['dialer-proxy'] ||
                    proxy.detour;

                if (proxy.type === 'http') {
                    proxy = {
                        type: 'http',
                        name: proxy.name,
                        server: proxy.server,
                        port: proxy.port,
                        username: proxy.username,
                        password: proxy.password,
                        tfo: proxy.tfo || proxy['fast-open'],
                        next_hop: proxy.next_hop,
                    };
                } else if (proxy.type === 'socks5') {
                    proxy = {
                        type: 'socks5',
                        name: proxy.name,
                        server: proxy.server,
                        port: proxy.port,
                        username: proxy.username,
                        password: proxy.password,
                        tfo: proxy.tfo || proxy['fast-open'],
                        udp_relay:
                            proxy.udp || proxy.udp_relay || proxy.udp_relay,
                        next_hop: proxy.next_hop,
                    };
                } else if (proxy.type === 'ss') {
                    proxy = {
                        type: 'shadowsocks',
                        name: proxy.name,
                        method:
                            proxy.cipher === 'chacha20-ietf-poly1305'
                                ? 'chacha20-poly1305'
                                : proxy.cipher,
                        server: proxy.server,
                        port: proxy.port,
                        password: proxy.password,
                        tfo: proxy.tfo || proxy['fast-open'],
                        udp_relay:
                            proxy.udp || proxy.udp_relay || proxy.udp_relay,
                        next_hop: proxy.next_hop,
                    };
                    if (original.plugin === 'obfs') {
                        proxy.obfs = original['plugin-opts']!.mode;
                        proxy.obfs_host = original['plugin-opts']!.host;
                        proxy.obfs_uri = original['plugin-opts']!.path;
                    }
                } else if (proxy.type === 'hysteria2') {
                    proxy = {
                        type: 'hysteria2',
                        name: proxy.name,
                        server: proxy.server,
                        port: proxy.port,
                        auth: proxy.password,
                        tfo: proxy.tfo || proxy['fast-open'],
                        udp_relay:
                            proxy.udp || proxy.udp_relay || proxy.udp_relay,
                        next_hop: proxy.next_hop,
                        sni: proxy.sni,
                        skip_tls_verify: proxy['skip-cert-verify'],
                        port_hopping: proxy.ports,
                        port_hopping_interval: proxy['hop-interval'],
                    };
                    if (
                        original['obfs-password'] &&
                        original.obfs == 'salamander'
                    ) {
                        proxy.obfs = 'salamander';
                        proxy.obfs_password = original['obfs-password'];
                    }
                } else if (proxy.type === 'tuic') {
                    proxy = {
                        type: 'tuic',
                        name: proxy.name,
                        server: proxy.server,
                        port: proxy.port,
                        uuid: proxy.uuid,
                        password: proxy.password,
                        next_hop: proxy.next_hop,
                        sni: proxy.sni,
                        alpn: Array.isArray(proxy.alpn)
                            ? proxy.alpn
                            : [proxy.alpn || 'h3'],
                        skip_tls_verify: proxy['skip-cert-verify'],
                        port_hopping: proxy.ports,
                        port_hopping_interval: proxy['hop-interval'],
                    };
                } else if (proxy.type === 'trojan') {
                    if (proxy.network === 'ws') {
                        proxy.websocket = {
                            path: proxy['ws-opts']?.path,
                            host: proxy['ws-opts']?.headers?.Host,
                        };
                    }
                    proxy = {
                        type: 'trojan',
                        name: proxy.name,
                        server: proxy.server,
                        port: proxy.port,
                        password: proxy.password,
                        tfo: proxy.tfo || proxy['fast-open'],
                        udp_relay:
                            proxy.udp || proxy.udp_relay || proxy.udp_relay,
                        next_hop: proxy.next_hop,
                        sni: proxy.sni,
                        skip_tls_verify: proxy['skip-cert-verify'],
                        websocket: proxy.websocket,
                    };
                } else if (proxy.type === 'vmess') {
                    // Egern：传输层，支持 ws/wss/http1/http2/tls，不配置则为 tcp
                    let security = proxy.cipher;
                    if (
                        security &&
                        ![
                            'auto',
                            'none',
                            'zero',
                            'aes-128-gcm',
                            'chacha20-poly1305',
                        ].includes(security)
                    ) {
                        security = 'auto';
                    }
                    if (proxy.network === 'ws') {
                        proxy.transport = {
                            [proxy.tls ? 'wss' : 'ws']: {
                                path: proxy['ws-opts']?.path,
                                headers: {
                                    Host: proxy['ws-opts']?.headers?.Host,
                                },
                                sni: proxy.tls ? proxy.sni : undefined,
                                skip_tls_verify: proxy.tls
                                    ? proxy['skip-cert-verify']
                                    : undefined,
                            },
                        };
                    } else if (proxy.network === 'http') {
                        proxy.transport = {
                            http1: {
                                method: proxy['http-opts']?.method,
                                path: Array.isArray(proxy['http-opts']?.path)
                                    ? proxy['http-opts']?.path[0]
                                    : proxy['http-opts']?.path,
                                headers: {
                                    Host: Array.isArray(
                                        proxy['http-opts']?.headers?.Host,
                                    )
                                        ? proxy['http-opts']?.headers?.Host[0]
                                        : proxy['http-opts']?.headers?.Host,
                                },
                                skip_tls_verify: proxy['skip-cert-verify'],
                            },
                        };
                    } else if (proxy.network === 'h2') {
                        proxy.transport = {
                            http2: {
                                method: proxy['h2-opts']?.method,
                                path: Array.isArray(proxy['h2-opts']?.path)
                                    ? proxy['h2-opts']?.path[0]
                                    : proxy['h2-opts']?.path,
                                headers: {
                                    Host: Array.isArray(
                                        proxy['h2-opts']?.headers?.Host,
                                    )
                                        ? proxy['h2-opts']?.headers?.Host[0]
                                        : proxy['h2-opts']?.headers?.Host,
                                },
                                skip_tls_verify: proxy['skip-cert-verify'],
                            },
                        };
                    } else if (
                        (proxy.network === 'tcp' || !proxy.network) &&
                        proxy.tls
                    ) {
                        proxy.transport = {
                            tls: {
                                sni: proxy.tls ? proxy.sni : undefined,
                                skip_tls_verify: proxy.tls
                                    ? proxy['skip-cert-verify']
                                    : undefined,
                            },
                        };
                    }
                    let legacy: boolean | undefined;
                    if (isPresent(proxy, 'aead') && !proxy.aead) {
                        legacy = true;
                    } else if (proxy.alterId !== 0) {
                        legacy = true;
                    }
                    proxy = {
                        type: 'vmess',
                        name: proxy.name,
                        server: proxy.server,
                        port: proxy.port,
                        user_id: proxy.uuid,
                        security,
                        tfo: proxy.tfo || proxy['fast-open'],
                        legacy,
                        udp_relay:
                            proxy.udp || proxy.udp_relay || proxy.udp_relay,
                        next_hop: proxy.next_hop,
                        transport: proxy.transport,
                        // sni: proxy.sni,
                        // skip_tls_verify: proxy['skip-cert-verify'],
                    };
                } else if (proxy.type === 'vless') {
                    if (proxy.network === 'ws') {
                        proxy.transport = {
                            [proxy.tls ? 'wss' : 'ws']: {
                                path: proxy['ws-opts']?.path,
                                headers: {
                                    Host: proxy['ws-opts']?.headers?.Host,
                                },
                                sni: proxy.tls ? proxy.sni : undefined,
                                skip_tls_verify: proxy.tls
                                    ? proxy['skip-cert-verify']
                                    : undefined,
                            },
                        };
                    } else if (proxy.network === 'http') {
                        proxy.transport = {
                            http: {
                                method: proxy['http-opts']?.method,
                                path: Array.isArray(proxy['http-opts']?.path)
                                    ? proxy['http-opts']?.path[0]
                                    : proxy['http-opts']?.path,
                                headers: {
                                    Host: Array.isArray(
                                        proxy['http-opts']?.headers?.Host,
                                    )
                                        ? proxy['http-opts']?.headers?.Host[0]
                                        : proxy['http-opts']?.headers?.Host,
                                },
                                skip_tls_verify: proxy['skip-cert-verify'],
                            },
                        };
                    } else if (proxy.network === 'tcp' || !proxy.network) {
                        let reality: Record<string, any> | undefined;
                        if (
                            proxy['reality-opts']?.['short-id'] ||
                            proxy['reality-opts']?.['public-key']
                        ) {
                            reality = {
                                short_id: proxy['reality-opts']['short-id'],
                                public_key: proxy['reality-opts']['public-key'],
                            };
                        }
                        proxy.transport = {
                            [proxy.tls ? 'tls' : 'tcp']: {
                                sni: proxy.tls ? proxy.sni : undefined,
                                skip_tls_verify: proxy.tls
                                    ? proxy['skip-cert-verify']
                                    : undefined,
                                reality,
                            },
                        };

                        flow = proxy.flow;
                    }
                    proxy = {
                        type: 'vless',
                        name: proxy.name,
                        server: proxy.server,
                        port: proxy.port,
                        user_id: proxy.uuid,
                        security: proxy.cipher,
                        tfo: proxy.tfo || proxy['fast-open'],
                        udp_relay:
                            proxy.udp || proxy.udp_relay || proxy.udp_relay,
                        next_hop: proxy.next_hop,
                        transport: proxy.transport,
                        flow,
                        // sni: proxy.sni,
                        // skip_tls_verify: proxy['skip-cert-verify'],
                    };
                }
                if (
                    [
                        'http',
                        'socks5',
                        'ss',
                        'trojan',
                        'vless',
                        'vmess',
                    ].includes(original.type)
                ) {
                    if (isPresent(original, 'shadow-tls-password')) {
                        if (original['shadow-tls-version'] != 3)
                            throw new Error(
                                `shadow-tls version ${original['shadow-tls-version']} is not supported`,
                            );
                        proxy.shadow_tls = {
                            password: original['shadow-tls-password'],
                            sni: original['shadow-tls-sni'],
                        };
                    } else if (
                        ['shadow-tls'].includes(original.plugin!) &&
                        original['plugin-opts']
                    ) {
                        if (original['plugin-opts'].version != 3)
                            throw new Error(
                                `shadow-tls version ${original['plugin-opts'].version} is not supported`,
                            );
                        proxy.shadow_tls = {
                            password: original['plugin-opts'].password,
                            sni: original['plugin-opts'].host,
                        };
                    }
                }
                if (
                    ['ss'].includes(original.type) &&
                    proxy.shadow_tls &&
                    (original['udp-port'] ?? 0) > 0 &&
                    (original['udp-port'] ?? 0) <= 65535
                ) {
                    proxy['udp_port'] = original['udp-port'];
                }

                delete proxy.subName;
                delete proxy.collectionName;
                delete proxy.id;
                delete proxy.resolved;
                delete proxy['no-resolve'];

                if (proxy.transport) {
                    for (const key in proxy.transport) {
                        if (
                            Object.keys(proxy.transport[key]).length === 0 ||
                            Object.values(proxy.transport[key]).every(
                                (v) => v == null,
                            )
                        ) {
                            delete proxy.transport[key];
                        }
                    }
                    if (Object.keys(proxy.transport).length === 0) {
                        delete proxy.transport;
                    }
                }

                if (type !== 'internal') {
                    for (const key in proxy) {
                        if (proxy[key] == null || /^_/i.test(key)) {
                            delete proxy[key];
                        }
                    }
                }
                return {
                    [proxy.type]: {
                        ...proxy,
                        type: undefined,
                        prev_hop,
                    },
                };
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
