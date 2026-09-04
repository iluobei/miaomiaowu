// 液态玻璃主题的「滑动指示器」:选中项背后那块会在选项之间流动的玻璃。
//
// 为什么需要 JS:纯 CSS 只能让高亮在不同元素上各自淡入淡出 —— 那是"闪现",不是"流动"。
// 要真的滑过去,必须知道选中项的位置和宽度,只能测量。
//
// 做成一个全局观察器而不是 React 组件:tabs 和顶栏导航分散在几十个页面里,
// 逐个改组件既啰嗦又容易漏;而这层纯装饰,不该侵入业务组件的结构。
//
// 只在液态玻璃主题下工作。其它主题各有自己的选中样式,插一块玻璃进去只会打架。

const INDICATOR_CLASS = 'glass-indicator'
const BLOB_CLASS = 'glass-indicator__blob'
const THEME_CLASS = 'theme-glass'

/** 一组「容器 → 选中项」的选择器。新增可滑动的控件只需往这里加一条。 */
const GROUPS = [
  { container: '[data-slot="tabs-list"]', active: '[data-state="active"]' },
  { container: '[data-glass-nav]', active: '[data-status="active"]' },
] as const

const attached = new WeakSet<HTMLElement>()

type Pos = { x: number; y: number; w: number; h: number }

/** 每块指示器上一次停在哪 —— 算这一程走了多远,决定抻多长。 */
const lastPos = new WeakMap<HTMLElement, Pos>()

/** 选中项四周还剩多少可见空间(相对真正裁剪它的那个祖先)。 */
type Room = { left: number; right: number; top: number; bottom: number }

/** 缓存每个容器的裁剪祖先:一次 DOM 上溯 + getComputedStyle,不该每帧重做。 */
const clipperOf = new WeakMap<HTMLElement, HTMLElement | null>()

/**
 * 找真正会裁掉溢出的那个祖先。
 *
 * 不能直接拿容器自己算:顶栏的 nav 确实就是裁剪者,但侧栏里挂 data-glass-nav 的
 * 是内层那个 flex 容器,真正裁剪的是外面带 padding 的 <nav>(overflow-y:auto)。
 * 拿错了会白白少掉一圈 padding 的可用空间。
 */
function findClipper(el: HTMLElement): HTMLElement | null {
  for (let n: HTMLElement | null = el; n; n = n.parentElement) {
    const s = getComputedStyle(n)
    if (s.overflowX !== 'visible' || s.overflowY !== 'visible') return n
  }
  return null
}

function prefersReducedMotion() {
  return (
    window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
  )
}

/**
 * 移动途中的挤压拉伸 —— 「水滴流动」的来源。
 *
 * 只做平移的话,看到的是一块形状不变的牌子滑过去;液体之所以像液体,是因为
 * 它在被拖动时会沿运动方向抻长、垂直方向变瘦,到位再弹回来。
 *
 * 这段必须用 Web Animations 而不是 CSS 过渡:过渡只在首尾两个状态之间插值,
 * 而形变的关键帧在**中途**(起点和终点都是不变形的)。
 */
function deform(blob: HTMLElement, from: Pos, to: Pos, room: Room) {
  if (prefersReducedMotion() || typeof blob.animate !== 'function') return
  const dx = to.x - from.x
  const dy = to.y - from.y
  const dist = Math.hypot(dx, dy)
  if (dist < 2) return

  const horizontal = Math.abs(dx) >= Math.abs(dy)
  // 抻长量随距离增长但很快封顶:跨越整条导航时也不该被拉成一条面条。
  // 分母取 180px(约一个导航项的宽度),让最常见的「移到相邻项」就有明显形变 ——
  // 若按全程长度归一,近距离的那一下几乎看不出来,等于白做。
  let stretch = 1 + Math.min(dist / 180, 1) * 0.3

  // 容器会把溢出裁掉(顶栏 nav 是 overflow-x:clip),而形变默认以自身中心为轴、
  // 两头一起延展 —— 第一项往左抻、最后一项往右抻,都会越过裁剪边被切掉一块
  // (用户实报:第一个菜单选中效果的左侧被遮挡)。
  //
  // 与其把形变缩小,不如**把支点挪向有空间的那一侧**,让它只往里长:
  // 视觉幅度不变,却始终待在可见区内。两侧都不够时才退而缩小抻长量。
  const size = horizontal ? to.w : to.h
  const before = horizontal ? room.left : room.top
  const after = horizontal ? room.right : room.bottom
  let originFrac = 0.5
  const extra = size * (stretch - 1)
  if (extra > 0) {
    if (before + after < extra) {
      // 两侧加起来都放不下:收到刚好放得下
      stretch = 1 + Math.max(0, before + after) / size
      originFrac = before + after > 0 ? before / (before + after) : 0.5
    } else {
      // 支点取 f(0=左/上端,1=右/下端):向前溢出 extra*f,向后溢出 extra*(1-f)
      const lo = Math.max(0, 1 - after / extra)
      const hi = Math.min(1, before / extra)
      originFrac = Math.min(Math.max(0.5, lo), hi)
    }
  }
  if (stretch <= 1.001) return

  // 体积守恒:抻多长就变多瘦。指数取 0.8 而非 1,刻意留一点「胖」——
  // 严格守恒会瘦得像被压扁的纸,水滴是有表面张力的,不会那么听话。
  const squash = 1 / Math.pow(stretch, 0.8)
  const peak = horizontal
    ? `scale(${stretch}, ${squash})`
    : `scale(${squash}, ${stretch})`
  blob.style.transformOrigin = horizontal
    ? `${(originFrac * 100).toFixed(1)}% center`
    : `center ${(originFrac * 100).toFixed(1)}%`

  blob.animate(
    [
      { transform: 'scale(1, 1)' },
      // 峰值放在 42% 而不是正中:形变先到,然后一路松弛着落位,
      // 像尾巴追上了头部。放正中会显得对称、机械。
      { transform: peak, offset: 0.42 },
      { transform: 'scale(1, 1)' },
    ],
    { duration: 380, easing: 'ease-in-out' }
  )
}

function isGlassTheme() {
  return document.documentElement.classList.contains(THEME_CLASS)
}

function applyPos(indicator: HTMLElement, p: Pos) {
  indicator.style.width = `${p.w}px`
  indicator.style.height = `${p.h}px`
  indicator.style.transform = `translate3d(${p.x}px, ${p.y}px, 0)`
  indicator.style.opacity = '1'
}

/** 把指示器移到选中项底下。选中项不存在时把它藏起来。 */
function position(container: HTMLElement, activeSelector: string) {
  const indicator = container.querySelector<HTMLElement>(`.${INDICATOR_CLASS}`)
  if (!indicator) return
  const active = container.querySelector<HTMLElement>(activeSelector)
  if (!active) {
    indicator.style.opacity = '0'
    // 忘掉旧位置:再出现时若还按上次的算,会凭空抻出一段并不存在的行程。
    lastPos.delete(indicator)
    return
  }
  const c = container.getBoundingClientRect()
  const a = active.getBoundingClientRect()
  // 容器可能有滚动(选项卡多到要横向滚),用 scrollLeft 换算成内容坐标,
  // 否则滚动之后指示器会停在错的位置。
  const p: Pos = {
    x: a.left - c.left + container.scrollLeft,
    y: a.top - c.top + container.scrollTop,
    w: a.width,
    h: a.height,
  }
  const prev = lastPos.get(indicator)
  const blob = indicator.querySelector<HTMLElement>(`.${BLOB_CLASS}`)

  // 余量直接用视口坐标算:选中项的矩形 vs 裁剪祖先的矩形。
  // 这样不必操心容器滚动、padding、边框各自的坐标系差异 —— 滚动之后
  // 可见窗口就是裁剪者的矩形,减出来的就是真实可用空间。
  if (!clipperOf.has(container))
    clipperOf.set(container, findClipper(container))
  const clipper = clipperOf.get(container) ?? null
  const clip = clipper ? clipper.getBoundingClientRect() : null
  const room: Room = clip
    ? {
        left: a.left - clip.left,
        right: clip.right - a.right,
        top: a.top - clip.top,
        bottom: clip.bottom - a.bottom,
      }
    : { left: Infinity, right: Infinity, top: Infinity, bottom: Infinity }

  // 弹簧缓动会**冲过终点再收回**(那点回弹感就来自这里),可终点贴着容器边时,
  // 冲出去的那一截正好落在裁剪线外、被切掉一块 —— 第一个菜单项最明显,
  // 而且这早于水滴形变就存在(实测左越界 6.3px)。
  // 只在「前方确实没空间」的那几程换成不过冲的曲线,中间项照旧保留回弹:
  // 不为了一个边界情况把整体手感抹平。
  if (prev) {
    const dx = p.x - prev.x
    const dy = p.y - prev.y
    const ahead =
      Math.abs(dx) >= Math.abs(dy)
        ? dx < 0
          ? room.left
          : room.right
        : dy < 0
          ? room.top
          : room.bottom
    // 这条曲线的过冲约为行程的 3%,留一点余量按 5% 判。
    indicator.style.transitionTimingFunction =
      ahead < Math.hypot(dx, dy) * 0.05
        ? 'cubic-bezier(0.22, 0.9, 0.24, 1)'
        : ''
  }

  applyPos(indicator, p)
  lastPos.set(indicator, p)

  // 只有「上一程存在」才形变:首次就位是直接摆过去的,不该带形变。
  if (prev && blob) deform(blob, prev, p, room)
}

function attach(container: HTMLElement, activeSelector: string) {
  if (attached.has(container)) return
  attached.add(container)

  const indicator = document.createElement('span')
  indicator.className = INDICATOR_CLASS
  indicator.setAttribute('aria-hidden', 'true')
  indicator.style.transition = 'none'
  // 内层单独一块:外层走位、内层形变,两个动画各写各的 transform,互不覆盖。
  const blob = document.createElement('span')
  blob.className = BLOB_CLASS
  indicator.appendChild(blob)
  container.prepend(indicator)

  const update = () => position(container, activeSelector)

  // 首次直接就位,不开过渡 —— 否则每次容器出现,滑块都要从左上角飞过来一趟。
  // 下一帧再交还过渡,之后的选中变化才是"滑过去"。
  //
  // 顶栏和侧栏都常驻在 __root 里(顶栏原先写在每个页面自己的 JSX 里,切路由整个
  // 重建,滑块只能"瞬移";提到根布局后它跨路由存活,这里就不必再去记忆旧位置)。
  update()
  requestAnimationFrame(() => {
    indicator.style.transition = ''
  })

  // 选中态是改 data-state / data-status 属性,不是增删节点 —— 必须看属性。
  const mo = new MutationObserver(update)
  mo.observe(container, {
    subtree: true,
    childList: true,
    attributes: true,
    attributeFilter: ['data-state', 'data-status', 'aria-selected', 'class'],
  })

  // 容器尺寸变化(窗口缩放、侧栏折叠、字体加载完)都会让位置失效。
  if (typeof ResizeObserver !== 'undefined') {
    new ResizeObserver(update).observe(container)
  }
  container.addEventListener('scroll', update, { passive: true })
}

function scan() {
  if (!isGlassTheme()) return
  for (const g of GROUPS) {
    document
      .querySelectorAll<HTMLElement>(g.container)
      .forEach((el) => attach(el, g.active))
  }
}

/**
 * 启动滑动指示器。幂等,可以重复调用。
 *
 * 由 main.tsx 调一次即可:内部靠 MutationObserver 认领后来出现的容器
 * (路由切换会整片替换 DOM,一次性扫描是不够的)。
 */
export function initGlassIndicator() {
  if (typeof document === 'undefined') return
  scan()
  // 路由切换后新挂载的 tabs 也要接上。整树观察只看 childList,开销可以接受;
  // 真正频繁的属性变化只在各容器自己的观察器里看。
  new MutationObserver(scan).observe(document.body, {
    subtree: true,
    childList: true,
  })
  // 主题是切换后刷新页面生效的,但个人菜单里切主题会即时改 class ——
  // 从别的主题切到玻璃时也要能接上。
  new MutationObserver(scan).observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  })
}
