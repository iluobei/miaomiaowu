import * as React from 'react'
import { cn } from '@/lib/utils'

interface ButtonGroupProps extends React.HTMLAttributes<HTMLDivElement> {
  /**
   * 布局模式
   * - 'responsive': 桌面端右对齐，移动端自动宽度均匀分布
   * - 'responsive-wrap': 桌面端右对齐，移动端换行显示
   * - 'always-full': 始终占满宽度均匀分布
   * - 'always-auto': 始终自动宽度
   */
  mode?: 'responsive' | 'responsive-wrap' | 'always-full' | 'always-auto'
  /**
   * 按钮间距
   */
  gap?: 'sm' | 'md' | 'lg'
  /**
   * 移动端隐藏按钮图标
   */
  hideIconOnMobile?: boolean
}

/**
 * 按钮组组件 - 统一管理按钮的布局
 *
 * 使用示例:
 * ```tsx
 * <ButtonGroup mode="responsive" hideIconOnMobile>
 *   <Button variant="outline"><MapPin />按地区分组</Button>
 *   <Button variant="outline"><Layers />手动分组</Button>
 *   <Button><Save />保存为订阅</Button>
 * </ButtonGroup>
 * ```
 */
const ButtonGroup = React.forwardRef<HTMLDivElement, ButtonGroupProps>(
  ({ className, mode = 'responsive', gap = 'md', hideIconOnMobile = false, children, ...props }, ref) => {
    const gapClass = {
      sm: 'gap-1.5',
      md: 'gap-2',
      lg: 'gap-3',
    }[gap]

    const modeClass = {
      // 桌面端右对齐，移动端子元素均分宽度
      'responsive': 'flex justify-end [&>*]:flex-1 sm:[&>*]:flex-none',
      // 桌面端右对齐，移动端换行显示
      'responsive-wrap': 'flex flex-wrap justify-end',
      // 始终均分宽度
      'always-full': 'flex [&>*]:flex-1',
      // 始终自动宽度，右对齐
      'always-auto': 'flex justify-end',
    }[mode]

    // 移动端隐藏图标的样式
    const hideIconClass = hideIconOnMobile
      ? '[&_svg]:hidden sm:[&_svg]:inline [&_button]:gap-0 sm:[&_button]:gap-2'
      : ''

    return (
      <div
        ref={ref}
        className={cn(modeClass, gapClass, hideIconClass, className)}
        {...props}
      >
        {children}
      </div>
    )
  }
)
ButtonGroup.displayName = 'ButtonGroup'

export { ButtonGroup }
