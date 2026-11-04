import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'

// 自定义品牌(从妙妙屋X 移植,无许可证门控)。/api/branding 是公开接口(登录页也要能拿),
// 值为空 = 没设自定义,前端回落内置默认。
export interface Branding {
  site_title: string
  brand_title: string
  logo_url: string
  icon_url: string
}

export const DEFAULT_SITE_TITLE = '妙妙屋'

// 首屏注入的左上角品牌 —— 由 Go 把 index.html 里 <meta name="brand-*"> 的占位符替换成
// 当前设置(internal/web/handler.go)。
//
// 存在的意义只有一个:让**第一帧**就画对。/api/branding 是个网络往返,在它回来之前
// brandTitle/logoUrl 都是空,于是左上角先渲染内置的「妙妙屋」+ 内置 logo,接口回来
// 再换掉 —— 用户每次刷新都看到闪一下。
//
// Vite 开发服务器不做替换,content 仍是 __ 开头的占位符,这里当成没配置。
const readInjectedBrand = (name: string): string => {
  if (typeof document === 'undefined') return ''
  const value = document
    .querySelector(`meta[name="${name}"]`)
    ?.getAttribute('content')
    ?.trim()
  if (!value || value.startsWith('__')) return ''
  return value
}

const injectedBrandTitle = readInjectedBrand('brand-title')
const injectedBrandLogo = readInjectedBrand('brand-logo')

const brandingQueryFn = async (): Promise<Branding> => {
  const res = await api.get('/api/branding')
  return (res.data?.branding ?? {}) as Branding
}

export function useBranding() {
  const { data } = useQuery({
    queryKey: ['branding'],
    queryFn: brandingQueryFn,
    staleTime: 5 * 60 * 1000,
    // 品牌是公开的,不依赖登录;失败静默(回落默认)。
    retry: false,
  })
  // 拿到接口数据之前一律用注入值兜底;有 data 之后以接口为准:
  // 管理员刚改完设置时,注入值可能比接口旧一拍。
  return {
    siteTitle: (data?.site_title || '').trim(),
    brandTitle: (data?.brand_title ?? injectedBrandTitle).trim(),
    logoUrl: (data?.logo_url ?? injectedBrandLogo).trim(),
    iconUrl: (data?.icon_url || '').trim(),
    // 判据是 data 有没有,不是 isFetched:请求失败时 isFetched 同样为 true 而 data 是 undefined,
    // 用 isFetched 判就会在接口挂掉时把标签页标题 / 图标打回内置的,盖掉服务端注入的自定义值。
    loaded: !!data,
  }
}

// 把标签页标题 / 图标套到 document 上。没自定义图标时:anime 主题用它自己的,其余用内置的。
export function applyDocumentBranding(title: string, iconUrl: string) {
  document.title = title.trim() || DEFAULT_SITE_TITLE

  let icon = document.querySelector<HTMLLinkElement>("link[rel~='icon']")
  if (!icon) {
    icon = document.createElement('link')
    icon.rel = 'icon'
    document.head.appendChild(icon)
  }
  const base = import.meta.env.BASE_URL
  icon.href =
    iconUrl.trim() ||
    (document.documentElement.classList.contains('theme-anime')
      ? `${base}images/anime-favicon.png`
      : `${base}images/favicon.ico`)
}
