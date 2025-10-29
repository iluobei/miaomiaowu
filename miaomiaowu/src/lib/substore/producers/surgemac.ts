import { Base64 } from 'js-base64';
import Surge_Producer from '@/lib/substore/producers/surge';
import ClashMeta_Producer from '@/lib/substore/producers/clashmeta';
import { isIPv4, isIPv6, Result, isPresent } from '@/lib/substore/producers/utils';
import { toast } from 'sonner';

const targetPlatform = 'SurgeMac';

const surge_Producer = Surge_Producer();

interface Proxy {
    type: string;
    name: string;
    server?: string;
    port?: number;
    exec?: string;
    'local-port'?: string | number;
    args?: string[];
    addresses?: string[];
    'local-address'?: string;
    'local_address'?: string;
    cipher?: string;
    obfs?: string;
    'obfs-param'?: string;
    password?: string;
    protocol?: string;
    'protocol-param'?: string;
    'no-error-alert'?: string | boolean;
    tfo?: boolean;
    'fast-open'?: boolean;
    'test-url'?: string;
    'block-quic'?: string | boolean;
    'ip-version'?: string;
    _exec?: string;
    _localPort?: number;
    _defaultNameserver?: string[];
    _nameserver?: string[];
    [key: string]: any;
}

interface ProduceOptions {
    useMihomoExternal?: boolean;
    localPort?: number;
    defaultNameserver?: string[];
    nameserver?: string[];
    [key: string]: any;
}

interface Producer {
    produce: (proxy: Proxy, type?: string, opts?: ProduceOptions) => string;
}

export default function SurgeMac_Producer(): Producer {
    const produce = (proxy: Proxy, type?: string, opts: ProduceOptions = {}): string => {
        switch (proxy.type) {
            case 'external':
                return external(proxy);
            // case 'ssr':
            //     return shadowsocksr(proxy);
            default: {
                try {
                    return surge_Producer.produce(proxy as any, type, opts);
                } catch (e) {
                    if (opts.useMihomoExternal) {
                        console.log(
                            `${proxy.name} is not supported on ${targetPlatform}, try to use Mihomo(SurgeMac - External Proxy Program) instead`,
                        );
                        return mihomo(proxy, type, opts);
                    } else {
                        throw new Error(
                            `Surge for macOS 可手动指定链接参数 target=SurgeMac 或在 同步配置 中指定 SurgeMac 来启用 mihomo 支援 Surge 本身不支持的协议`,
                        );
                    }
                }
            }
        }
    };
    return { produce };
}

function external(proxy: Proxy): string {
    const result = new Result(proxy);
    if (!proxy.exec || !proxy['local-port']) {
        throw new Error(`${proxy.type}: exec and local-port are required`);
    }
    result.append(
        `${proxy.name}=external,exec="${proxy.exec}",local-port=${proxy['local-port']}`,
    );

    if (Array.isArray(proxy.args)) {
        proxy.args.map((args) => {
            result.append(`,args="${args}"`);
        });
    }
    if (Array.isArray(proxy.addresses)) {
        proxy.addresses.map((addresses) => {
            result.append(`,addresses=${addresses}`);
        });
    }

    result.appendIfPresent(
        `,no-error-alert=${proxy['no-error-alert']}`,
        'no-error-alert',
    );

    // tfo
    if (isPresent(proxy, 'tfo')) {
        result.append(`,tfo=${proxy['tfo']}`);
    } else if (isPresent(proxy, 'fast-open')) {
        result.append(`,tfo=${proxy['fast-open']}`);
    }

    // test-url
    result.appendIfPresent(`,test-url=${proxy['test-url']}`, 'test-url');

    // block-quic
    result.appendIfPresent(`,block-quic=${proxy['block-quic']}`, 'block-quic');

    return result.toString();
}

function mihomo(proxy: Proxy, _type?: string, opts: ProduceOptions = {}): string {
    const clashProxy = (ClashMeta_Producer().produce([proxy], 'internal') as Proxy[])?.[0];
    if (clashProxy) {
        const localPort = opts?.localPort || proxy._localPort || 65535;
        const ipv6 = ['ipv4', 'v4-only'].includes(proxy['ip-version'] || '')
            ? false
            : true;
        const external_proxy: Proxy = {
            name: proxy.name,
            type: 'external',
            exec: proxy._exec || '/usr/local/bin/mihomo',
            'local-port': localPort,
            args: [
                '-config',
                Base64.encode(
                    JSON.stringify({
                        'mixed-port': localPort,
                        ipv6,
                        mode: 'global',
                        dns: {
                            enable: true,
                            ipv6,
                            'default-nameserver': opts?.defaultNameserver ||
                                proxy._defaultNameserver || [
                                    '180.76.76.76',
                                    '52.80.52.52',
                                    '119.28.28.28',
                                    '223.6.6.6',
                                ],
                            nameserver: opts?.nameserver ||
                                proxy._nameserver || [
                                    'https://doh.pub/dns-query',
                                    'https://dns.alidns.com/dns-query',
                                    'https://doh-pure.onedns.net/dns-query',
                                ],
                        },
                        proxies: [
                            {
                                ...clashProxy,
                                name: 'proxy',
                            },
                        ],
                        'proxy-groups': [
                            {
                                name: 'GLOBAL',
                                type: 'select',
                                proxies: ['proxy'],
                            },
                        ],
                    }),
                ),
            ],
            addresses: [],
        };

        // https://manual.nssurge.com/policy/external-proxy.html
        if (isIP(proxy.server || '')) {
            external_proxy.addresses!.push(proxy.server!);
        } else {
            toast(
                `Platform ${targetPlatform}, proxy type ${proxy.type}: addresses should be an IP address, but got ${proxy.server}`,
            );
        }
        opts.localPort = localPort - 1;
        return external(external_proxy);
    }
    return '';
}

function isIP(ip: string): boolean {
    return isIPv4(ip) || isIPv6(ip);
}
