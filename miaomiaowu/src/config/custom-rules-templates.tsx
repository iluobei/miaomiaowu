// 规则集模板对应的规则内容
export const RULE_PROVIDER_RULES = {
	'loyalsoldier-blacklist': `- RULE-SET,applications,DIRECT
- DOMAIN,clash.razord.top,DIRECT
- DOMAIN,yacd.haishan.me,DIRECT
- RULE-SET,private,DIRECT
- RULE-SET,reject,REJECT
- RULE-SET,tld-not-cn,PROXY
- RULE-SET,gfw,PROXY
- RULE-SET,telegramcidr,PROXY
- MATCH,DIRECT`,
	'loyalsoldier-whitelist': `- RULE-SET,applications,DIRECT
- DOMAIN,clash.razord.top,DIRECT
- DOMAIN,yacd.haishan.me,DIRECT
- RULE-SET,private,DIRECT
- RULE-SET,reject,REJECT
- RULE-SET,icloud,DIRECT
- RULE-SET,apple,DIRECT
- RULE-SET,google,PROXY
- RULE-SET,proxy,PROXY
- RULE-SET,direct,DIRECT
- RULE-SET,lancidr,DIRECT
- RULE-SET,cncidr,DIRECT
- RULE-SET,telegramcidr,PROXY
- GEOIP,LAN,DIRECT
- GEOIP,CN,DIRECT
- MATCH,PROXY`,
	'acl4ssr-standard': `- RULE-SET,LocalAreaNetwork,🎯 全球直连
- RULE-SET,BanAD,🛑 全球拦截
- RULE-SET,BanProgramAD,🍃 应用净化
- RULE-SET,GoogleCN,🎯 全球直连
- RULE-SET,SteamCN,🎯 全球直连
- RULE-SET,Microsoft,Ⓜ️ 微软服务
- RULE-SET,Apple,🍎 苹果服务
- RULE-SET,ProxyMedia,🌍 国外媒体
- RULE-SET,Telegram,📲 电报信息
- RULE-SET,ProxyLite,🚀 节点选择
- RULE-SET,ChinaDomain,🎯 全球直连
- RULE-SET,ChinaCompanyIp,🎯 全球直连
- GEOIP,CN,🎯 全球直连
- MATCH,🐟 漏网之鱼`,
	'acl4ssr-lite': `- RULE-SET,LocalAreaNetwork,🎯 全球直连
- RULE-SET,BanAD,🛑 全球拦截
- RULE-SET,BanProgramAD,🛑 全球拦截
- RULE-SET,GoogleCN,🎯 全球直连
- RULE-SET,SteamCN,🎯 全球直连
- RULE-SET,Telegram,🚀 节点选择
- RULE-SET,ProxyMedia,🚀 节点选择
- RULE-SET,ProxyLite,🚀 节点选择
- RULE-SET,ChinaDomain,🎯 全球直连
- RULE-SET,ChinaCompanyIp,🎯 全球直连
- GEOIP,CN,🎯 全球直连
- MATCH,🐟 漏网之鱼`,
	'aethersailor-standard': `- GEOSITE,private,🎯 全球直连
- GEOIP,private,🎯 全球直连,no-resolve
- RULE-SET,Custom_Direct_Classical,🎯 全球直连
- RULE-SET,Custom_Proxy_Classical,🚀 手动选择
- GEOSITE,google-cn,🎯 全球直连
- GEOSITE,category-games@cn,🎯 全球直连
- RULE-SET,Steam_CDN_Classical,🎯 全球直连
- GEOSITE,category-game-platforms-download,🎯 全球直连
- GEOSITE,category-public-tracker,🎯 全球直连
- GEOSITE,category-communication,💬 即时通讯
- GEOSITE,category-social-media-!cn,🌐 社交媒体
- GEOSITE,openai,🤖 ChatGPT
- GEOSITE,bing,🤖 Copilot
- GEOSITE,category-ai-!cn,🤖 AI服务
- GEOSITE,github,🚀 GitHub
- GEOSITE,category-speedtest,🚀 测速工具
- GEOSITE,steam,🎮 Steam
- GEOSITE,youtube,📹 YouTube
- GEOSITE,apple-tvplus,🎥 AppleTV+
- GEOSITE,apple,🍎 苹果服务
- GEOSITE,microsoft,Ⓜ️ 微软服务
- GEOSITE,googlefcm,📢 谷歌FCM
- GEOSITE,google,🇬 谷歌服务
- GEOSITE,tiktok,🎶 TikTok
- GEOSITE,netflix,🎥 Netflix
- GEOSITE,disney,🎥 DisneyPlus
- GEOSITE,hbo,🎥 HBO
- GEOSITE,primevideo,🎥 PrimeVideo
- GEOSITE,category-emby,🎥 Emby
- GEOSITE,spotify,🎻 Spotify
- GEOSITE,bahamut,📺 Bahamut
- GEOSITE,category-games,🎮 游戏平台
- GEOSITE,category-entertainment,🌎 国外媒体
- GEOSITE,category-ecommerce,🛒 国外电商
- GEOSITE,gfw,🚀 手动选择
- GEOIP,telegram,💬 即时通讯,no-resolve
- GEOIP,twitter,🌐 社交媒体,no-resolve
- GEOIP,facebook,🌐 社交媒体,no-resolve
- GEOIP,google,🇬 谷歌服务,no-resolve
- GEOIP,netflix,🎥 Netflix,no-resolve
- GEOSITE,cn,🎯 全球直连
- GEOIP,cn,🎯 全球直连,no-resolve
- RULE-SET,Custom_Port_Direct,🔀 非标端口
- MATCH,🐟 漏网之鱼`,
	'aethersailor-lite': `- GEOSITE,private,🎯 全球直连
- GEOIP,private,🎯 全球直连,no-resolve
- RULE-SET,Custom_Direct_Classical,🎯 全球直连
- RULE-SET,Custom_Proxy_Classical,🚀 手动选择
- GEOSITE,google-cn,🎯 全球直连
- GEOSITE,category-games@cn,🎯 全球直连
- RULE-SET,Steam_CDN_Classical,🎯 全球直连
- GEOSITE,category-game-platforms-download,🎯 全球直连
- GEOSITE,category-public-tracker,🎯 全球直连
- GEOSITE,github,🚀 GitHub
- GEOSITE,category-speedtest,🚀 测速工具
- GEOSITE,apple,🍎 苹果服务
- GEOSITE,steam,🎮 Steam
- GEOSITE,microsoft,Ⓜ️ 微软服务
- GEOSITE,googlefcm,📢 谷歌FCM
- GEOSITE,google,🇬 谷歌服务
- GEOSITE,gfw,🚀 手动选择
- GEOSITE,category-games,🎮 游戏平台
- GEOIP,google,🇬 谷歌服务,no-resolve
- GEOSITE,cn,🎯 全球直连
- GEOIP,cn,🎯 全球直连,no-resolve
- RULE-SET,Custom_Port_Direct,🔀 非标端口
- MATCH,🐟 漏网之鱼`
}

// 预设模板
export const RULE_TEMPLATES = {
	dns: {
		proxy: {
			name: '使用代理解析DNS',
			content: `enable: true
nameserver:
  - https://8.8.8.8/dns-query#♻️ 自动选择
direct-nameserver:
  - https://1.12.12.12/dns-query
nameserver-policy:
  geosite:gfw,greatfire:
    - https://8.8.8.8/dns-query#♻️ 自动选择
  geosite:cn,apple,private,steam,onedrive:
    - https://1.12.12.12/dns-query
  geosite:category-games@cn:
    - https://1.12.12.12/dns-query
  geosite:google:
    - https://1.0.0.1/dns-query#♻️ 自动选择
  geosite:geolocation-!cn:
    - https://1.0.0.1/dns-query#♻️ 自动选择
proxy-server-nameserver:
  - https://1.12.12.12/dns-query
ipv6: false
listen: 0.0.0.0:7874
default-nameserver:
  - https://1.1.1.1/dns-query#♻️ 自动选择
fallback:
  - https://1.1.1.1/dns-query#♻️ 自动选择
  - https://120.53.53.53/dns-query
  - https://223.5.5.5/dns-query
use-hosts: true`
		},
		local: {
			name: '本地解析DNS',
			content: `enable: true
nameserver:
  - https://1.12.12.12/dns-query
direct-nameserver:
  - https://1.12.12.12/dns-query
nameserver-policy:
  'geosite:gfw,greatfire':
    - 'https://8.8.8.8/dns-query'
  "geosite:cn, private":
    - https://1.12.12.12/dns-query
  "geosite:category-games@cn":
    - https://1.12.12.12/dns-query
  "geosite:google":
    - https://1.0.0.1/dns-query
  "geosite:apple":
    - https://1.0.0.1/dns-query
  "geosite:geolocation-!cn":
    - https://1.0.0.1/dns-query
proxy-server-nameserver:
  - https://1.12.12.12/dns-query
ipv6: false
listen: 0.0.0.0:7874
default-nameserver:
  - https://1.1.1.1/dns-query
fallback:
  - https://120.53.53.53/dns-query
  - https://223.5.5.5/dns-query
  - https://1.1.1.1/dns-query
use-hosts: true`
		}
	},
	rules: {
		'loyalsoldier-blacklist': {
			name: 'Loyalsoldier 规则（黑名单）',
			content: `- RULE-SET,applications,DIRECT
- DOMAIN,clash.razord.top,DIRECT
- DOMAIN,yacd.haishan.me,DIRECT
- RULE-SET,private,DIRECT
- RULE-SET,reject,REJECT
- RULE-SET,tld-not-cn,PROXY
- RULE-SET,gfw,PROXY
- RULE-SET,telegramcidr,PROXY
- MATCH,DIRECT`
		},
		'loyalsoldier-whitelist': {
			name: 'Loyalsoldier 规则（白名单）',
			content: `- RULE-SET,applications,DIRECT
- DOMAIN,clash.razord.top,DIRECT
- DOMAIN,yacd.haishan.me,DIRECT
- RULE-SET,private,DIRECT
- RULE-SET,reject,REJECT
- RULE-SET,icloud,DIRECT
- RULE-SET,apple,DIRECT
- RULE-SET,google,PROXY
- RULE-SET,proxy,PROXY
- RULE-SET,direct,DIRECT
- RULE-SET,lancidr,DIRECT
- RULE-SET,cncidr,DIRECT
- RULE-SET,telegramcidr,PROXY
- GEOIP,LAN,DIRECT
- GEOIP,CN,DIRECT
- MATCH,PROXY`
		},
		'acl4ssr-standard': {
			name: 'ACL4SSR 规则（标准版）',
			content: `- RULE-SET,LocalAreaNetwork,🎯 全球直连
- RULE-SET,BanAD,🛑 全球拦截
- RULE-SET,BanProgramAD,🍃 应用净化
- RULE-SET,GoogleCN,🎯 全球直连
- RULE-SET,SteamCN,🎯 全球直连
- RULE-SET,Microsoft,Ⓜ️ 微软服务
- RULE-SET,Apple,🍎 苹果服务
- RULE-SET,ProxyMedia,🌍 国外媒体
- RULE-SET,Telegram,📲 电报信息
- RULE-SET,ProxyLite,🚀 节点选择
- RULE-SET,ChinaDomain,🎯 全球直连
- RULE-SET,ChinaCompanyIp,🎯 全球直连
- GEOIP,CN,🎯 全球直连
- MATCH,🐟 漏网之鱼`
		},
		'acl4ssr-lite': {
			name: 'ACL4SSR 规则（轻量版）',
			content: `- RULE-SET,LocalAreaNetwork,🎯 全球直连
- RULE-SET,BanAD,🛑 全球拦截
- RULE-SET,BanProgramAD,🛑 全球拦截
- RULE-SET,GoogleCN,🎯 全球直连
- RULE-SET,SteamCN,🎯 全球直连
- RULE-SET,Telegram,🚀 节点选择
- RULE-SET,ProxyMedia,🚀 节点选择
- RULE-SET,ProxyLite,🚀 节点选择
- RULE-SET,ChinaDomain,🎯 全球直连
- RULE-SET,ChinaCompanyIp,🎯 全球直连
- GEOIP,CN,🎯 全球直连
- MATCH,🐟 漏网之鱼`
		},
		'aethersailor-standard': {
			name: 'Aethersailor 规则（标准版）',
			content: `- GEOSITE,private,🎯 全球直连
- GEOIP,private,🎯 全球直连,no-resolve
- RULE-SET,Custom_Direct_Classical,🎯 全球直连
- RULE-SET,Custom_Proxy_Classical,🚀 手动选择
- GEOSITE,google-cn,🎯 全球直连
- GEOSITE,category-games@cn,🎯 全球直连
- RULE-SET,Steam_CDN_Classical,🎯 全球直连
- GEOSITE,category-game-platforms-download,🎯 全球直连
- GEOSITE,category-public-tracker,🎯 全球直连
- GEOSITE,category-communication,💬 即时通讯
- GEOSITE,category-social-media-!cn,🌐 社交媒体
- GEOSITE,openai,🤖 ChatGPT
- GEOSITE,bing,🤖 Copilot
- GEOSITE,category-ai-!cn,🤖 AI服务
- GEOSITE,github,🚀 GitHub
- GEOSITE,category-speedtest,🚀 测速工具
- GEOSITE,steam,🎮 Steam
- GEOSITE,youtube,📹 YouTube
- GEOSITE,apple-tvplus,🎥 AppleTV+
- GEOSITE,apple,🍎 苹果服务
- GEOSITE,microsoft,Ⓜ️ 微软服务
- GEOSITE,googlefcm,📢 谷歌FCM
- GEOSITE,google,🇬 谷歌服务
- GEOSITE,tiktok,🎶 TikTok
- GEOSITE,netflix,🎥 Netflix
- GEOSITE,disney,🎥 DisneyPlus
- GEOSITE,hbo,🎥 HBO
- GEOSITE,primevideo,🎥 PrimeVideo
- GEOSITE,category-emby,🎥 Emby
- GEOSITE,spotify,🎻 Spotify
- GEOSITE,bahamut,📺 Bahamut
- GEOSITE,category-games,🎮 游戏平台
- GEOSITE,category-entertainment,🌎 国外媒体
- GEOSITE,category-ecommerce,🛒 国外电商
- GEOSITE,gfw,🚀 手动选择
- GEOIP,telegram,💬 即时通讯,no-resolve
- GEOIP,twitter,🌐 社交媒体,no-resolve
- GEOIP,facebook,🌐 社交媒体,no-resolve
- GEOIP,google,🇬 谷歌服务,no-resolve
- GEOIP,netflix,🎥 Netflix,no-resolve
- GEOSITE,cn,🎯 全球直连
- GEOIP,cn,🎯 全球直连,no-resolve
- RULE-SET,Custom_Port_Direct,🔀 非标端口
- MATCH,🐟 漏网之鱼`
		},
		'aethersailor-lite': {
			name: 'Aethersailor 规则（轻量版）',
			content: `- GEOSITE,private,🎯 全球直连
- GEOIP,private,🎯 全球直连,no-resolve
- RULE-SET,Custom_Direct_Classical,🎯 全球直连
- RULE-SET,Custom_Proxy_Classical,🚀 手动选择
- GEOSITE,google-cn,🎯 全球直连
- GEOSITE,category-games@cn,🎯 全球直连
- RULE-SET,Steam_CDN_Classical,🎯 全球直连
- GEOSITE,category-game-platforms-download,🎯 全球直连
- GEOSITE,category-public-tracker,🎯 全球直连
- GEOSITE,github,🚀 GitHub
- GEOSITE,category-speedtest,🚀 测速工具
- GEOSITE,apple,🍎 苹果服务
- GEOSITE,steam,🎮 Steam
- GEOSITE,microsoft,Ⓜ️ 微软服务
- GEOSITE,googlefcm,📢 谷歌FCM
- GEOSITE,google,🇬 谷歌服务
- GEOSITE,gfw,🚀 手动选择
- GEOSITE,category-games,🎮 游戏平台
- GEOIP,google,🇬 谷歌服务,no-resolve
- GEOSITE,cn,🎯 全球直连
- GEOIP,cn,🎯 全球直连,no-resolve
- RULE-SET,Custom_Port_Direct,🔀 非标端口
- MATCH,🐟 漏网之鱼`
		}
	},
	'rule-providers': {
		'loyalsoldier-whitelist': {
			name: 'Loyalsoldier 规则集（白名单）',
			content: `reject:
  type: http
  behavior: domain
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/reject.txt"
  path: ./ruleset/reject.yaml
  interval: 86400

icloud:
  type: http
  behavior: domain
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/icloud.txt"
  path: ./ruleset/icloud.yaml
  interval: 86400

apple:
  type: http
  behavior: domain
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/apple.txt"
  path: ./ruleset/apple.yaml
  interval: 86400

google:
  type: http
  behavior: domain
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/google.txt"
  path: ./ruleset/google.yaml
  interval: 86400

proxy:
  type: http
  behavior: domain
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/proxy.txt"
  path: ./ruleset/proxy.yaml
  interval: 86400

direct:
  type: http
  behavior: domain
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/direct.txt"
  path: ./ruleset/direct.yaml
  interval: 86400

private:
  type: http
  behavior: domain
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/private.txt"
  path: ./ruleset/private.yaml
  interval: 86400

gfw:
  type: http
  behavior: domain
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/gfw.txt"
  path: ./ruleset/gfw.yaml
  interval: 86400

tld-not-cn:
  type: http
  behavior: domain
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/tld-not-cn.txt"
  path: ./ruleset/tld-not-cn.yaml
  interval: 86400

telegramcidr:
  type: http
  behavior: ipcidr
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/telegramcidr.txt"
  path: ./ruleset/telegramcidr.yaml
  interval: 86400

cncidr:
  type: http
  behavior: ipcidr
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/cncidr.txt"
  path: ./ruleset/cncidr.yaml
  interval: 86400

lancidr:
  type: http
  behavior: ipcidr
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/lancidr.txt"
  path: ./ruleset/lancidr.yaml
  interval: 86400

applications:
  type: http
  behavior: classical
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/applications.txt"
  path: ./ruleset/applications.yaml
  interval: 86400`
		},
		'loyalsoldier-blacklist': {
			name: 'Loyalsoldier 规则集（黑名单）',
			content: `reject:
  type: http
  behavior: domain
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/reject.txt"
  path: ./ruleset/reject.yaml
  interval: 86400

private:
  type: http
  behavior: domain
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/private.txt"
  path: ./ruleset/private.yaml
  interval: 86400

gfw:
  type: http
  behavior: domain
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/gfw.txt"
  path: ./ruleset/gfw.yaml
  interval: 86400

tld-not-cn:
  type: http
  behavior: domain
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/tld-not-cn.txt"
  path: ./ruleset/tld-not-cn.yaml
  interval: 86400

telegramcidr:
  type: http
  behavior: ipcidr
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/telegramcidr.txt"
  path: ./ruleset/telegramcidr.yaml
  interval: 86400

applications:
  type: http
  behavior: classical
  url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/applications.txt"
  path: ./ruleset/applications.yaml
  interval: 86400`
		},
		'aethersailor-standard': {
			name: 'Aethersailor 规则集（标准版）',
			content: `Custom_Direct_Classical:
  type: http
  behavior: classical
  url: https://testingcf.jsdelivr.net/gh/Aethersailor/Custom_OpenClash_Rules@main/rule/Custom_Direct_Classical.yaml
  path: ./providers/15519759398106057482.yaml
  interval: 28800

Custom_Proxy_Classical:
  type: http
  behavior: classical
  url: https://testingcf.jsdelivr.net/gh/Aethersailor/Custom_OpenClash_Rules@main/rule/Custom_Proxy_Classical.yaml
  path: ./providers/5958306251279867197.yaml
  interval: 28800

Steam_CDN_Classical:
  type: http
  behavior: classical
  url: https://testingcf.jsdelivr.net/gh/Aethersailor/Custom_OpenClash_Rules@main/rule/Steam_CDN_Classical.yaml
  path: ./providers/13268481674871578153.yaml
  interval: 28800

Custom_Port_Direct:
  type: http
  behavior: classical
  url: https://testingcf.jsdelivr.net/gh/Aethersailor/Custom_OpenClash_Rules@main/rule/Custom_Port_Direct.yaml
  path: ./providers/2451647452112462632.yaml
  interval: 28800`
		},
		'aethersailor-lite': {
			name: 'Aethersailor 规则集（轻量版）',
			content: `Custom_Direct_Classical:
  type: http
  behavior: classical
  url: https://testingcf.jsdelivr.net/gh/Aethersailor/Custom_OpenClash_Rules@main/rule/Custom_Direct_Classical.yaml
  path: ./providers/15519759398106057482.yaml
  interval: 28800

Custom_Proxy_Classical:
  type: http
  behavior: classical
  url: https://testingcf.jsdelivr.net/gh/Aethersailor/Custom_OpenClash_Rules@main/rule/Custom_Proxy_Classical.yaml
  path: ./providers/5958306251279867197.yaml
  interval: 28800

Steam_CDN_Classical:
  type: http
  behavior: classical
  url: https://testingcf.jsdelivr.net/gh/Aethersailor/Custom_OpenClash_Rules@main/rule/Steam_CDN_Classical.yaml
  path: ./providers/13268481674871578153.yaml
  interval: 28800

Custom_Port_Direct:
  type: http
  behavior: classical
  url: https://testingcf.jsdelivr.net/gh/Aethersailor/Custom_OpenClash_Rules@main/rule/Custom_Port_Direct.yaml
  path: ./providers/2451647452112462632.yaml
  interval: 28800`
		},
		'acl4ssr-standard': {
			name: 'ACL4SSR 规则集（标准版）',
			content: `LocalAreaNetwork:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9Mb2NhbEFyZWFOZXR3b3JrLmxpc3Q
  path: ./providers/8402706212293704900.yaml
  interval: 86400

BanAD:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9CYW5BRC5saXN0
  path: ./providers/2929890640486968208.yaml
  interval: 86400

BanProgramAD:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9CYW5Qcm9ncmFtQUQubGlzdA
  path: ./providers/8331643685869654068.yaml
  interval: 86400

GoogleCN:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9Hb29nbGVDTi5saXN0
  path: ./providers/1915104033986474024.yaml
  interval: 86400

SteamCN:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9SdWxlc2V0L1N0ZWFtQ04ubGlzdA
  path: ./providers/12662494171829552811.yaml
  interval: 86400

Microsoft:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9NaWNyb3NvZnQubGlzdA
  path: ./providers/8612810905479681943.yaml
  interval: 86400

Apple:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9BcHBsZS5saXN0
  path: ./providers/18320548137865118588.yaml
  interval: 86400

ProxyMedia:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9Qcm94eU1lZGlhLmxpc3Q
  path: ./providers/2993815002304293589.yaml
  interval: 86400

Telegram:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9UZWxlZ3JhbS5saXN0
  path: ./providers/8557577971298535803.yaml
  interval: 86400

ProxyLite:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9Qcm94eUxpdGUubGlzdA
  path: ./providers/9032795626629285706.yaml
  interval: 86400

ChinaDomain:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9DaGluYURvbWFpbi5saXN0
  path: ./providers/16015304399768979015.yaml
  interval: 86400

ChinaCompanyIp:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9DaGluYUNvbXBhbnlJcC5saXN0
  path: ./providers/8094975577528505650.yaml
  interval: 86400`
		},
		'acl4ssr-lite': {
			name: 'ACL4SSR 规则集（轻量版）',
			content: `LocalAreaNetwork:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9Mb2NhbEFyZWFOZXR3b3JrLmxpc3Q
  path: ./providers/8402706212293704900.yaml
  interval: 86400

BanAD:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9CYW5BRC5saXN0
  path: ./providers/2929890640486968208.yaml
  interval: 86400

BanProgramAD:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9CYW5Qcm9ncmFtQUQubGlzdA
  path: ./providers/8331643685869654068.yaml
  interval: 86400

GoogleCN:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9Hb29nbGVDTi5saXN0
  path: ./providers/1915104033986474024.yaml
  interval: 86400

SteamCN:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9SdWxlc2V0L1N0ZWFtQ04ubGlzdA
  path: ./providers/12662494171829552811.yaml
  interval: 86400

Telegram:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9UZWxlZ3JhbS5saXN0
  path: ./providers/8557577971298535803.yaml
  interval: 86400

ProxyMedia:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9Qcm94eU1lZGlhLmxpc3Q
  path: ./providers/2993815002304293589.yaml
  interval: 86400

ProxyLite:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9Qcm94eUxpdGUubGlzdA
  path: ./providers/9032795626629285706.yaml
  interval: 86400

ChinaDomain:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9DaGluYURvbWFpbi5saXN0
  path: ./providers/16015304399768979015.yaml
  interval: 86400

ChinaCompanyIp:
  type: http
  behavior: classical
  url: https://api.dler.io/getruleset?type=6&url=cnVsZXMvQUNMNFNTUi9DbGFzaC9DaGluYUNvbXBhbnlJcC5saXN0
  path: ./providers/8094975577528505650.yaml
  interval: 86400`
		}
	}
}
