// 登录后 layout 的动态四角星背景。定时器持续在随机位置生成星星,每颗闪一次(淡入→亮→淡出)即被移除。
// 保证每颗都是全新随机位置、此起彼伏,不会卡在固定点。仅 anime 主题显示(样式作用域见 anime-theme.css)。
import { useEffect, useState } from 'react'

// 刻晴配色:紫罗兰 / 薰衣草 / 靛蓝 / 金 / 品红粉 / 白
const COLORS = [
  '#a78bfa',
  '#8b5cf6',
  '#c4b5fd',
  '#6a5ae0',
  '#f0c674',
  '#ec6fb0',
  '#eede9f',
  '#efe6ff',
]

const rand = (min: number, max: number) => min + Math.random() * (max - min)
const pick = <T,>(arr: T[]) => arr[Math.floor(Math.random() * arr.length)]

type Star = {
  id: number
  top: string
  left: string
  size: number
  color: string
  dur: number
}

let gid = 0
function makeStar(): Star {
  return {
    id: ++gid,
    top: `${rand(2, 96).toFixed(2)}%`,
    left: `${rand(1, 98).toFixed(2)}%`,
    size: Math.round(rand(12, 26)),
    color: pick(COLORS),
    dur: Number(rand(1.4, 2.8).toFixed(2)),
  }
}

const SPAWN_MS = 70 // 生成间隔(越小越密)
const MAX = 160 // 并发上限(防 animationEnd 万一不触发导致堆积)

export function AnimeStarfield() {
  const [stars, setStars] = useState<Star[]>([])

  useEffect(() => {
    const timer = setInterval(() => {
      setStars((prev) => {
        const next =
          prev.length >= MAX ? prev.slice(prev.length - MAX + 1) : prev
        return [...next, makeStar()]
      })
    }, SPAWN_MS)
    return () => clearInterval(timer)
  }, [])

  const remove = (id: number) =>
    setStars((prev) => prev.filter((s) => s.id !== id))

  return (
    <div className='anime-starfield' aria-hidden='true'>
      {stars.map((s) => (
        <span
          key={s.id}
          className='anime-star'
          style={{
            top: s.top,
            left: s.left,
            width: s.size,
            height: s.size,
            color: s.color,
            animationDuration: `${s.dur}s`,
          }}
          onAnimationEnd={() => remove(s.id)}
        />
      ))}
    </div>
  )
}
