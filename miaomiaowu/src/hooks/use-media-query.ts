import { useEffect, useState } from 'react'

export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(false)

  useEffect(() => {
    const media = window.matchMedia(query)

    // 设置初始值
    setMatches(media.matches)

    // 创建监听器
    const listener = (e: MediaQueryListEvent) => {
      setMatches(e.matches)
    }

    // 添加监听器
    media.addEventListener('change', listener)

    // 清理
    return () => media.removeEventListener('change', listener)
  }, [query])

  return matches
}
