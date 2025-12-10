import { useEffect, useRef, memo } from 'react'
import twemoji from 'twemoji'

interface TwemojiProps {
  children: React.ReactNode
  className?: string
}

/**
 * Twemoji 组件 - 将文本中的 emoji 转换为 Twitter 风格的 SVG 图片
 * 使用 CDN 加载 SVG 格式的 emoji，确保跨平台显示一致
 */
export const Twemoji = memo(function Twemoji({ children, className }: TwemojiProps) {
  const containerRef = useRef<HTMLSpanElement>(null)

  useEffect(() => {
    if (containerRef.current) {
      twemoji.parse(containerRef.current, {
        folder: 'svg',
        ext: '.svg',
        base: 'https://cdn.jsdelivr.net/gh/twitter/twemoji@14.0.2/assets/',
      })
    }
  }, [children])

  return (
    <span ref={containerRef} className={className}>
      {children}
    </span>
  )
})

export default Twemoji
