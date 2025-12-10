/**
 * 将国家代码转换为对应的旗帜 emoji
 * 使用 Unicode 区域指示符号 (Regional Indicator Symbol)
 * @param countryCode 两字母国家代码，如 "US", "CN", "JP"
 * @returns 对应的旗帜 emoji，如 "🇺🇸", "🇨🇳", "🇯🇵"
 */
export function countryCodeToFlag(countryCode: string): string {
  if (!countryCode || countryCode.length !== 2) {
    return ''
  }

  // 将每个字母转换为对应的区域指示符号
  // 区域指示符号 A-Z 对应 Unicode 码点 127462-127487 (0x1F1E6-0x1F1FF)
  // 计算方式: 字母的 ASCII 码 + 127397 = 区域指示符号码点
  const codePoints = countryCode
    .toUpperCase()
    .split('')
    .map(char => 127397 + char.charCodeAt(0))

  return String.fromCodePoint(...codePoints)
}

/**
 * 从旗帜 emoji 反向解析国家代码
 * 例如 "🇺🇸" -> "US", "🇭🇰" -> "HK"
 */
export function flagToCountryCode(flag: string): string | null {
  if (!flag) return null

  const codePoints = [...flag].map(char => char.codePointAt(0) || 0)
  if (codePoints.length !== 2) return null

  // 区域指示符号范围: 0x1F1E6 (A) - 0x1F1FF (Z)
  const isRegionalIndicator = (cp: number) => cp >= 0x1F1E6 && cp <= 0x1F1FF
  if (!codePoints.every(isRegionalIndicator)) return null

  return codePoints.map(cp => String.fromCharCode(cp - 127397)).join('')
}

/**
 * 从节点名称提取地区 emoji 和国家代码
 */
export function extractRegionFromNodeName(nodeName: string): { emoji: string, countryCode: string } | null {
  if (!nodeName) return null

  const emojiRegex = /^([\u{1F1E6}-\u{1F1FF}]{2})/u
  const match = nodeName.match(emojiRegex)
  if (!match) return null

  const emoji = match[1]
  const countryCode = flagToCountryCode(emoji)
  if (!countryCode) return null

  return { emoji, countryCode }
}

/**
 * 代理组名称到国家代码的映射
 */
export const REGION_GROUP_MAP: Record<string, string[]> = {
  '🇭🇰 香港节点': ['HK'],
  '🇺🇸 美国节点': ['US'],
  '🇯🇵 日本节点': ['JP'],
  '🇸🇬 新加坡节点': ['SG'],
  '🇹🇼 台湾节点': ['TW'],
  '🇰🇷 韩国节点': ['KR'],
  '🇨🇦 加拿大节点': ['CA'],
  '🇬🇧 英国节点': ['GB'],
  '🇫🇷 法国节点': ['FR'],
  '🇩🇪 德国节点': ['DE'],
  '🇳🇱 荷兰节点': ['NL'],
  '🇹🇷 土耳其节点': ['TR'],
}

/**
 * 国家代码到代理组名称的反向映射
 */
export const COUNTRY_TO_GROUP_MAP: Record<string, string> = {
  'HK': '🇭🇰 香港节点',
  'US': '🇺🇸 美国节点',
  'JP': '🇯🇵 日本节点',
  'SG': '🇸🇬 新加坡节点',
  'TW': '🇹🇼 台湾节点',
  'KR': '🇰🇷 韩国节点',
  'CA': '🇨🇦 加拿大节点',
  'GB': '🇬🇧 英国节点',
  'FR': '🇫🇷 法国节点',
  'DE': '🇩🇪 德国节点',
  'NL': '🇳🇱 荷兰节点',
  'TR': '🇹🇷 土耳其节点',
}

/**
 * 根据国家代码查找对应的代理组名称
 */
export function findRegionGroupName(countryCode: string): string | null {
  return COUNTRY_TO_GROUP_MAP[countryCode.toUpperCase()] || null
}

/**
 * 检查字符串开头是否已有 emoji
 * 包括旗帜 emoji、表情符号等
 */
export function hasEmojiPrefix(text: string): boolean {
  if (!text) return false

  // 匹配开头的 emoji 字符
  // 包括：
  // - Emoji_Presentation: 默认以 emoji 形式显示的字符
  // - Extended_Pictographic: 扩展象形文字（包括旗帜）
  // - 区域指示符号对（旗帜 emoji）
  const emojiRegex = /^(?:[\u{1F1E6}-\u{1F1FF}]{2}|[\u{1F300}-\u{1F9FF}]|[\u{2600}-\u{26FF}]|[\u{2700}-\u{27BF}]|[\u{1F600}-\u{1F64F}]|[\u{1F680}-\u{1F6FF}]|[\u{1F900}-\u{1F9FF}])/u

  return emojiRegex.test(text)
}

/**
 * 从 ipinfo.io 获取 IP 地理位置信息
 */
export interface GeoIPInfo {
  ip: string
  country_code: string
  country: string
  continent_code?: string
  continent?: string
  asn?: string
  as_name?: string
  as_domain?: string
}

const IPINFO_TOKEN = 'cddae164b36656'

export async function getGeoIPInfo(ip: string): Promise<GeoIPInfo> {
  const response = await fetch(`https://api.ipinfo.io/lite/${ip}?token=${IPINFO_TOKEN}`)

  if (!response.ok) {
    throw new Error(`Failed to get GeoIP info: ${response.status}`)
  }

  return response.json()
}
