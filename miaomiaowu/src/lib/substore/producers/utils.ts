import { get as _get } from 'lodash'

// source: https://stackoverflow.com/a/36760050
const IPV4_REGEX = /^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)(\.(?!$)|$)){4}$/

// source: https://ihateregex.io/expr/ipv6/
const IPV6_REGEX =
  /^(([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|::(ffff(:0{1,4}){0,1}:){0,1}((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9]))$/

export class Result {
  proxy: any
  output: string[]

  constructor(proxy: any) {
    this.proxy = proxy
    this.output = []
  }

  append(data: string): void {
    if (typeof data === 'undefined') {
      throw new Error('required field is missing')
    }
    this.output.push(data)
  }

  appendIfPresent(data: string, attr: string): void {
    if (isPresent(this.proxy, attr)) {
      this.append(data)
    }
  }

  toString(): string {
    return this.output.join('')
  }
}

export function isPresent(obj: any, attr?: string): boolean {
  if (typeof attr === 'undefined') {
    // When called with single argument, check if obj itself is present
    return typeof obj !== 'undefined' && obj !== null
  }
  // When called with two arguments, use lodash get
  const data = _get(obj, attr)
  return typeof data !== 'undefined' && data !== null
}

export function isIPv4(ip: string): boolean {
  return IPV4_REGEX.test(ip)
}

export function isIPv6(ip: string): boolean {
  return IPV6_REGEX.test(ip)
}

export function isValidPortNumber(port: string | number): boolean {
  return /^((6553[0-5])|(655[0-2][0-9])|(65[0-4][0-9]{2})|(6[0-4][0-9]{3})|([1-5][0-9]{4})|([0-5]{0,5})|([0-9]{1,4}))$/.test(
    String(port)
  )
}

export function isNotBlank(str: string): boolean {
  return typeof str === 'string' && str.trim().length > 0
}

export function getIfNotBlank(str: string, defaultValue: string): string {
  return isNotBlank(str) ? str : defaultValue
}

export function getIfPresent<T>(obj: T | null | undefined, defaultValue: T): T {
  return isPresent(obj) ? obj! : defaultValue
}

export function getPolicyDescriptor(str: string): { 'policy-descriptor'?: string; policy?: string } {
  if (!str) return {}
  return /^.+?\s*?=\s*?.+?\s*?,.+?/.test(str)
    ? {
        'policy-descriptor': str,
      }
    : {
        policy: str,
      }
}

export function getRandomInt(min: number, max: number): number {
  min = Math.ceil(min)
  max = Math.floor(max)
  return Math.floor(Math.random() * (max - min + 1)) + min
}

export function getRandomPort(portString: string): number {
  const portParts = portString.split(/,|\//)
  const randomPart = portParts[Math.floor(Math.random() * portParts.length)]
  if (randomPart.includes('-')) {
    const [min, max] = randomPart.split('-').map(Number)
    return getRandomInt(min, max)
  } else {
    return Number(randomPart)
  }
}

export function numberToString(value: number | bigint): string {
  return Number.isSafeInteger(value) ? String(value) : BigInt(value).toString()
}

export function isValidUUID(uuid: string): boolean {
  return (
    typeof uuid === 'string' &&
    /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/.test(uuid)
  )
}

export function formatDateTime(date: Date | string | number, format = 'YYYY-MM-DD_HH-mm-ss'): string {
  const d = date instanceof Date ? date : new Date(date)

  if (isNaN(d.getTime())) {
    return ''
  }

  const pad = (num: number): string => String(num).padStart(2, '0')

  const replacements: Record<string, string | number> = {
    YYYY: d.getFullYear(),
    MM: pad(d.getMonth() + 1),
    DD: pad(d.getDate()),
    HH: pad(d.getHours()),
    mm: pad(d.getMinutes()),
    ss: pad(d.getSeconds()),
  }

  return format.replace(/YYYY|MM|DD|HH|mm|ss/g, (match) => String(replacements[match]))
}
