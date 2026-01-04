import { useMemo, memo } from 'react'
import twemoji from 'twemoji'

interface TwemojiProps {
  children: React.ReactNode
  className?: string
}

// 全局缓存，避免重复解析相同内容
const parseCache = new Map<string, string>()

/**
 * Twemoji 组件 - 将文本中的 emoji 转换为 Twitter 风格的 SVG 图片
 * 使用 CDN 加载 SVG 格式的 emoji，确保跨平台显示一致
 * 使用缓存优化性能，避免重复解析相同内容
 */
export const Twemoji = memo(function Twemoji({ children, className }: TwemojiProps) {
  const parsedHtml = useMemo(() => {
    const text = String(children || '')

    // 检查缓存
    if (parseCache.has(text)) {
      return parseCache.get(text)!
    }

    // 创建临时元素进行解析
    const temp = document.createElement('span')
    temp.textContent = text
    twemoji.parse(temp, {
      folder: 'svg',
      ext: '.svg',
      base: 'https://cdn.jsdelivr.net/gh/twitter/twemoji@14.0.2/assets/',
    })

    const result = temp.innerHTML
    parseCache.set(text, result)
    return result
  }, [children])

  return (
    <span
      className={className}
      dangerouslySetInnerHTML={{ __html: parsedHtml }}
    />
  )
})

export default Twemoji
