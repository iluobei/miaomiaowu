import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'

// PWA(从妙妙屋X 移植):注册 / 注销 service worker,新版本接管时提示刷新。
//
// SW 本体是手写的 public/sw.js(缓存策略与放行规则都在那里),这里只管生命周期。
// 开关是系统设置里的 pwa_enabled,随首屏 HTML 的 <meta name="mmw-pwa"> 下发
// (internal/web/pwa.go),不多发请求。
//
// **只给已登录的会话注册**:未登录的访客(登录页)不装 SW、不留缓存。
// 登录后注册,退出登录注销并清缓存。

const readMeta = (name: string): string =>
  document
    .querySelector(`meta[name="${name}"]`)
    ?.getAttribute('content')
    ?.trim() ?? ''

const readPWAFlag = () => readMeta('mmw-pwa')

const CACHE_PREFIX = 'mmw-' // 与 public/sw.js 一致

async function deleteCaches(prefix: string) {
  if (!('caches' in window)) return
  const keys = await caches.keys()
  await Promise.all(
    keys.filter((k) => k.startsWith(prefix)).map((k) => caches.delete(k))
  )
}

// 上一个控制本页的 SW。controllerchange 时据此判断是不是「升级」:
// 首次安装时 clients.claim() 也会触发 controllerchange,那不是新版本,不该提示。
let lastController: ServiceWorker | null = null
let notified = false

// 注销本站所有 SW 并清掉它建的缓存。PWA 关闭时服务端 /sw.js 也会改发自我注销脚本,两边都兜。
async function unregisterAll() {
  try {
    const regs = await navigator.serviceWorker.getRegistrations()
    await Promise.all(regs.map((reg) => reg.unregister()))
    await deleteCaches(CACHE_PREFIX)
  } catch {
    // 注销失败不影响页面本身
  }
  // 注销后本页仍被旧 SW 控制到下次导航为止;不清掉的话,重新登录注册时的 claim 会被误判成升级。
  lastController = null
}

function register() {
  navigator.serviceWorker
    .register('/sw.js', { scope: '/', updateViaCache: 'none' })
    .catch((err) => {
      // eslint-disable-next-line no-console
      console.warn('[pwa] service worker 注册失败', err)
    })
}

// 浏览器只在导航时(以及至多 24 小时一次)检查 SW 更新;面板常常开着一整天不刷新,
// 升级后很久都发现不了。每小时主动查一次,切回标签页时也查一次。
function checkForUpdate() {
  if (!useAuthStore.getState().auth.accessToken) return
  navigator.serviceWorker
    .getRegistration('/')
    .then((reg) => reg?.update())
    .catch(() => {})
}

export function setupPWA() {
  // 开发环境不注册:Vite 开发服务器的模块请求会被缓存规则搅乱,改代码看不到效果。
  if (!import.meta.env.PROD) return
  if (!('serviceWorker' in navigator)) return
  // SW 只能在安全上下文里注册:https,或 localhost / 127.0.0.1。裸 IP + http 直接跳过。
  if (!window.isSecureContext) return

  if (readPWAFlag() === 'false') {
    void unregisterAll()
    return
  }

  lastController = navigator.serviceWorker.controller
  navigator.serviceWorker.addEventListener('controllerchange', () => {
    const prev = lastController
    lastController = navigator.serviceWorker.controller
    if (!prev || notified) return
    notified = true
    toast('有新版本,刷新后生效', {
      id: 'pwa-update',
      duration: Infinity,
      action: {
        label: '刷新',
        onClick: () => window.location.reload(),
      },
    })
  })

  // 登录态跟着 auth store 走(token 存在 cookie 里,store 初始化时已读出):
  // 有 token 就注册,token 被清掉(退出登录 / 会话过期)就注销。
  const sync = (loggedIn: boolean) => {
    if (loggedIn) register()
    else void unregisterAll()
  }
  // 等页面加载完再注册,别和首屏资源抢带宽。
  const start = () => {
    sync(Boolean(useAuthStore.getState().auth.accessToken))
    useAuthStore.subscribe((state, prevState) => {
      const now = Boolean(state.auth.accessToken)
      if (now !== Boolean(prevState.auth.accessToken)) sync(now)
    })
  }
  if (document.readyState === 'complete') start()
  else window.addEventListener('load', start, { once: true })

  window.setInterval(checkForUpdate, 60 * 60 * 1000)
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') checkForUpdate()
  })
}
