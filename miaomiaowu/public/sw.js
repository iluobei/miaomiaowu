/*
 * 妙妙屋 service worker(从妙妙屋X 移植)。手写、原样拷进 dist,不经 Vite 处理。
 *
 * 只做两件事:
 *   1. 带哈希的静态资源(/assets/ 以及 /fonts/ /images/)缓存优先 —— 二次打开秒开;
 *   2. 页面导航网络优先,断网时退回最近一次缓存的首页壳。
 *
 * 其余一律不碰:接口(/api/)、临时订阅 /t/、流量接口 /traffic/、壁纸 /wallpapers/、
 * 非 GET、跨域 —— 不调用 respondWith,浏览器照常直连。
 * 订阅与接口的响应绝不能进缓存:它们带凭据、按用户变化,临时订阅还是一次性的。
 * 短链订阅挂在根路径(/{code}),与 SPA 路由没法区分,只能靠「导航网络优先」保证在线时直连。
 *
 * 缓存名里的版本号由 Go 在下发时替换(internal/web/pwa.go),前端每次重新构建它就变;
 * 新版本 install 后立即 skipWaiting,activate 时删掉旧版本的缓存并接管已打开的页面,
 * 页面那边收到 controllerchange 提示「有新版本,刷新」(src/lib/pwa.ts)。
 *
 * 不开 navigation preload:它对**所有**导航都会预先发一次请求,包括这里放行不接管的 /t/ ——
 * 临时订阅默认只能取一次,预请求会把它白白耗掉。
 */

const VERSION = '__MMW_SW_VERSION__'
// 与 internal/web/pwa.go 里自我注销脚本清理的前缀保持一致。
const CACHE_PREFIX = 'mmw-'
const STATIC_CACHE = CACHE_PREFIX + 'static-' + VERSION
const SHELL_CACHE = CACHE_PREFIX + 'shell-' + VERSION
const SHELL_URL = '/'

// 精确等于或以「前缀/」开头都算。不能只用 startsWith('/t'):那会误伤 /templates 这种 SPA 路由。
const BYPASS_PREFIXES = ['/api', '/t', '/traffic', '/wallpapers']
const BYPASS_EXACT = ['/sw.js', '/manifest.webmanifest']
const STATIC_PREFIXES = ['/assets/', '/fonts/', '/images/']

/**
 * 判断一个请求归哪一类:'static'(缓存优先)、'navigate'(网络优先 + 离线兜底)、
 * null(不接管)。
 */
function classifyRequest(url, method, mode, scopeOrigin) {
  if (method !== 'GET') return null
  let parsed
  try {
    parsed = new URL(url)
  } catch (_e) {
    return null
  }
  if (parsed.origin !== scopeOrigin) return null
  const path = parsed.pathname
  if (BYPASS_EXACT.indexOf(path) !== -1) return null
  for (let i = 0; i < BYPASS_PREFIXES.length; i++) {
    const prefix = BYPASS_PREFIXES[i]
    if (path === prefix || path.indexOf(prefix + '/') === 0) return null
  }
  if (mode === 'navigate') return 'navigate'
  for (let i = 0; i < STATIC_PREFIXES.length; i++) {
    if (path.indexOf(STATIC_PREFIXES[i]) === 0) return 'static'
  }
  return null
}

function isHTML(response) {
  return (response.headers.get('content-type') || '').indexOf('text/html') !== -1
}

// 只缓存同源、200、真正的 HTML 首页 —— 重定向、错误页都不进缓存。
function cacheShell(response) {
  if (!response || !response.ok || response.type !== 'basic') return
  if (!isHTML(response)) return
  const copy = response.clone()
  return caches
    .open(SHELL_CACHE)
    .then((cache) => cache.put(SHELL_URL, copy))
    .catch(() => {})
}

async function handleNavigate(request) {
  try {
    const response = await fetch(request)
    cacheShell(response)
    return response
  } catch (err) {
    const cache = await caches.open(SHELL_CACHE)
    const cached = await cache.match(SHELL_URL)
    if (cached) return cached
    throw err
  }
}

async function handleStatic(request) {
  const cache = await caches.open(STATIC_CACHE)
  const hit = await cache.match(request)
  if (hit) return hit
  const response = await fetch(request)
  // 不存在的 /assets/xxx.js 会被 SPA 兜底回成 200 的 index.html(升级后旧页面请求旧 chunk
  // 就是这样)。把它当成 JS 缓存下来,这个版本里就永远坏了,所以 HTML 一律不进静态缓存。
  if (response.ok && response.type === 'basic' && !isHTML(response)) {
    cache.put(request, response.clone()).catch(() => {})
  }
  return response
}

self.addEventListener('install', (event) => {
  self.skipWaiting()
  // 预先缓存一份首页壳:首次访问那一页还没有 SW,不预取的话要到第二次导航才有离线兜底。
  // 失败不影响安装。
  event.waitUntil(
    fetch(SHELL_URL, { cache: 'no-cache' })
      .then(cacheShell)
      .catch(() => {})
  )
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) =>
        Promise.all(
          keys
            .filter(
              (key) =>
                key.indexOf(CACHE_PREFIX) === 0 &&
                key !== STATIC_CACHE &&
                key !== SHELL_CACHE
            )
            .map((key) => caches.delete(key))
        )
      )
      .then(() => self.clients.claim())
  )
})

self.addEventListener('fetch', (event) => {
  const request = event.request
  const kind = classifyRequest(
    request.url,
    request.method,
    request.mode,
    self.location.origin
  )
  if (kind === 'navigate') {
    event.respondWith(handleNavigate(request))
  } else if (kind === 'static') {
    event.respondWith(handleStatic(request))
  }
})
