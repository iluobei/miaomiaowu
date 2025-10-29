 
import { Base64 } from 'js-base64';
import URI_Producer from './uri';
import { toast } from 'sonner';

const URI = URI_Producer();

interface Proxy {
    [key: string]: any;
}

interface Producer {
    type: string;
    produce: (proxies: Proxy[]) => string;
}

export default function V2Ray_Producer(): Producer {
    const type = 'ALL';
    const produce = (proxies: Proxy[]): string => {
        const result: string[] = [];
        proxies.map((proxy) => {
            try {
                result.push(URI.produce(proxy as any));
            } catch (err) {
                toast(
                    `Cannot produce proxy: ${JSON.stringify(
                        proxy,
                        null,
                        2,
                    )}\nReason: ${err}`,
                );
            }
        });

        return Base64.encode(result.join('\n'));
    };

    return { type, produce };
}
