// Translation mappings from sublink-worker's i18n
// Maps category internal names to display names with emoji

export const OUTBOUND_NAMES: Record<string, string> = {
  'Auto Select': '♻️ 自动选择',
  'Node Select': '🚀 节点选择',
  'Fall Back': '🐟 漏网之鱼',
  'Ad Block': '🛑 广告拦截',
  'AI Services': '💬 AI 服务',
  Bilibili: '📺 哔哩哔哩',
  Youtube: '📹 油管视频',
  Google: '🔍 谷歌服务',
  Private: '🏠 私有网络',
  'Location:CN': '🔒 国内服务',
  Telegram: '📲 电报消息',
  Github: '🐱 Github',
  Microsoft: 'Ⓜ️ 微软服务',
  Apple: '🍏 苹果服务',
  'Social Media': '🌐 社交媒体',
  Streaming: '🎬 流媒体',
  Gaming: '🎮 游戏平台',
  Education: '📚 教育资源',
  Financial: '💰 金融服务',
  'Cloud Services': '☁️ 云服务',
  'Non-China': '🌐 非中国',
}

// Map internal category names to unified rule names
export const CATEGORY_TO_RULE_NAME: Record<string, string> = {
  ads: 'Ad Block',
  ai: 'AI Services',
  bilibili: 'Bilibili',
  youtube: 'Youtube',
  google: 'Google',
  private: 'Private',
  domestic: 'Location:CN',
  telegram: 'Telegram',
  github: 'Github',
  microsoft: 'Microsoft',
  apple: 'Apple',
  social: 'Social Media',
  streaming: 'Streaming',
  gaming: 'Gaming',
  education: 'Education',
  finance: 'Financial',
  cloud: 'Cloud Services',
  overseas: 'Non-China',
}

export function translateOutbound(name: string): string {
  return OUTBOUND_NAMES[name] || name
}
