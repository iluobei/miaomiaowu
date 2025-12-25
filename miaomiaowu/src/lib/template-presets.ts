// 模板预设常量定义

export interface ACL4SSRPreset {
  name: string
  url: string
  label: string
}

// 内置 ACL4SSR 预设列表
export const ACL4SSR_PRESETS: ACL4SSRPreset[] = [
  // 作者自用
  { name: 'sublinkPro作者自用', url: 'https://raw.githubusercontent.com/ZeroDeng01/ACL4SSR/master/Clash/config/ACL4SSR_Online_Full_NoCountry.ini', label: 'sublinkPro作者自用 - 不区分国家' },
  // 标准版
  { name: 'ACL4SSR', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR.ini', label: '标准版 - 典型分组' },
  { name: 'ACL4SSR_AdblockPlus', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_AdblockPlus.ini', label: '标准版 - 典型分组+去广告' },
  // 回国版
  { name: 'ACL4SSR_BackCN', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_BackCN.ini', label: '回国版 - 回国专用' },
  // 精简版
  { name: 'ACL4SSR_Mini', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Mini.ini', label: '精简版 - 少量分组' },
  { name: 'ACL4SSR_Mini_Fallback', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Mini_Fallback.ini', label: '精简版 - 故障转移' },
  { name: 'ACL4SSR_Mini_MultiMode', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Mini_MultiMode.ini', label: '精简版 - 多模式 (自动/手动)' },
  { name: 'ACL4SSR_Mini_NoAuto', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Mini_NoAuto.ini', label: '精简版 - 无自动测速' },
  // 无苹果/微软分流版
  { name: 'ACL4SSR_NoApple', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_NoApple.ini', label: '无苹果 - 无苹果分流' },
  { name: 'ACL4SSR_NoAuto', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_NoAuto.ini', label: '无测速 - 无自动测速' },
  { name: 'ACL4SSR_NoAuto_NoApple', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_NoAuto_NoApple.ini', label: '无测速&苹果 - 无测速&无苹果分流' },
  { name: 'ACL4SSR_NoAuto_NoApple_NoMicrosoft', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_NoAuto_NoApple_NoMicrosoft.ini', label: '无测速&苹果&微软 - 无测速&无苹果&无微软分流' },
  { name: 'ACL4SSR_NoMicrosoft', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_NoMicrosoft.ini', label: '无微软 - 无微软分流' },
  // 在线版
  { name: 'ACL4SSR_Online', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online.ini', label: '在线版 - 典型分组' },
  { name: 'ACL4SSR_Online_AdblockPlus', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_AdblockPlus.ini', label: '在线版 - 典型分组+去广告' },
  // 在线全分组版
  { name: 'ACL4SSR_Online_Full', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Full.ini', label: '在线全分组 - 比较全' },
  { name: 'ACL4SSR_Online_Full_AdblockPlus', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Full_AdblockPlus.ini', label: '在线全分组 - 带广告拦截' },
  { name: 'ACL4SSR_Online_Full_Google', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Full_Google.ini', label: '在线全分组 - 谷歌分流' },
  { name: 'ACL4SSR_Online_Full_MultiMode', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Full_MultiMode.ini', label: '在线全分组 - 多模式' },
  { name: 'ACL4SSR_Online_Full_Netflix', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Full_Netflix.ini', label: '在线全分组 - 奈飞分流' },
  { name: 'ACL4SSR_Online_Full_NoAuto', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Full_NoAuto.ini', label: '在线全分组 - 无自动测速' },
  // 在线精简版
  { name: 'ACL4SSR_Online_Mini', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Mini.ini', label: '在线精简版 - 少量分组' },
  { name: 'ACL4SSR_Online_Mini_AdblockPlus', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Mini_AdblockPlus.ini', label: '在线精简版 - 带广告拦截' },
  { name: 'ACL4SSR_Online_Mini_Ai', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Mini_Ai.ini', label: '在线精简版 - AI' },
  { name: 'ACL4SSR_Online_Mini_Fallback', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Mini_Fallback.ini', label: '在线精简版 - 故障转移' },
  { name: 'ACL4SSR_Online_Mini_MultiCountry', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Mini_MultiCountry.ini', label: '在线精简版 - 多国家' },
  { name: 'ACL4SSR_Online_Mini_MultiMode', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Mini_MultiMode.ini', label: '在线精简版 - 多模式' },
  { name: 'ACL4SSR_Online_Mini_NoAuto', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Mini_NoAuto.ini', label: '在线精简版 - 无自动测速' },
  { name: 'ACL4SSR_Online_MultiCountry', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_MultiCountry.ini', label: '在线版 - 多国家' },
  { name: 'ACL4SSR_Online_NoAuto', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_NoAuto.ini', label: '在线版 - 无自动测速' },
  { name: 'ACL4SSR_Online_NoReject', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_NoReject.ini', label: '在线版 - 无拒绝规则' },
  // 特殊版
  { name: 'ACL4SSR_WithChinaIp', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_WithChinaIp.ini', label: '特殊版 - 包含回国IP' },
  { name: 'ACL4SSR_WithChinaIp_WithGFW', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_WithChinaIp_WithGFW.ini', label: '特殊版 - 包含回国IP&GFW列表' },
  { name: 'ACL4SSR_WithGFW', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_WithGFW.ini', label: '特殊版 - 包含GFW列表' },
]

// Aethersailor 预设列表
export const Aethersailor_PRESETS: ACL4SSRPreset[] = [
  { name: 'Custom_Clash', url: 'https://raw.githubusercontent.com//Aethersailor/Custom_OpenClash_Rules/main/cfg/Custom_Clash.ini', label: 'Aethersailor - 标准 (推荐使用)' },
  { name: 'Custom_Clash_Full', url: 'https://raw.githubusercontent.com//Aethersailor/Custom_OpenClash_Rules/main/cfg/Custom_Clash_Full.ini', label: 'Aethersailor - 全分组 (节点较多)' },
  { name: 'Custom_Clash_GFW', url: 'https://raw.githubusercontent.com//Aethersailor/Custom_OpenClash_Rules/main/cfg/Custom_Clash_GFW.ini', label: 'Aethersailor - 极简 (GFW)' },
  { name: 'Custom_Clash_Lite', url: 'https://raw.githubusercontent.com//Aethersailor/Custom_OpenClash_Rules/main/cfg/Custom_Clash_Lite.ini', label: 'Aethersailor - 轻量 (国内直连，国外代理)' },
]

// 合并所有预设模板
export const ALL_TEMPLATE_PRESETS: ACL4SSRPreset[] = [
  ...Aethersailor_PRESETS,
  ...ACL4SSR_PRESETS,
]
