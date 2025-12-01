import { ReactNode } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

/**
 * 表格列配置
 */
export type DataTableColumn<T> = {
  /** 列标题 */
  header: string | ReactNode
  /** 数据访问器，可以是对象的key或自定义函数 */
  accessor?: keyof T | ((item: T) => ReactNode)
  /** 自定义单元格渲染函数 */
  cell?: (item: T) => ReactNode
  /** 表头单元格样式 */
  headerClassName?: string
  /** 数据单元格样式 */
  cellClassName?: string
  /** 列宽度 */
  width?: string
}

/**
 * 移动端卡片字段配置
 */
export type DataTableCardField<T> = {
  /** 字段标签 */
  label: string
  /** 字段值渲染函数 */
  value: (item: T) => ReactNode
  /** 是否隐藏此字段（可根据数据动态决定） */
  hidden?: (item: T) => boolean
  /** 字段容器样式 */
  className?: string
}

/**
 * 通用数据表格组件属性
 */
export type DataTableProps<T> = {
  /** 数据数组 */
  data: T[]

  /** 桌面端表格列配置 */
  columns: DataTableColumn<T>[]

  /** 移动端卡片配置 */
  mobileCard?: {
    /** 卡片头部内容渲染函数 */
    header?: (item: T) => ReactNode
    /** 卡片主要字段配置 */
    fields?: DataTableCardField<T>[]
    /** 卡片底部操作按钮渲染函数 */
    actions?: (item: T) => ReactNode
    /** 卡片容器样式 */
    cardClassName?: string
    /** 卡片内容样式 */
    contentClassName?: string
  }

  /** 表格行key生成函数（必须保证唯一性） */
  getRowKey: (item: T, index: number) => string | number

  /** 空状态提示文本 */
  emptyText?: string

  /** 表格容器样式 */
  containerClassName?: string

  /** 表格行点击事件 */
  onRowClick?: (item: T) => void

  /** 自定义表格行样式 */
  rowClassName?: (item: T) => string
}

/**
 * 通用数据表格组件
 *
 * 在桌面端显示为表格，在移动端显示为卡片列表
 * 支持完全自定义渲染内容和样式
 *
 * @example
 * ```tsx
 * <DataTable
 *   data={items}
 *   columns={[
 *     { header: 'Name', cell: (item) => item.name },
 *     { header: 'Status', cell: (item) => <Badge>{item.status}</Badge> }
 *   ]}
 *   mobileCard={{
 *     header: (item) => <h3>{item.name}</h3>,
 *     fields: [
 *       { label: 'Status', value: (item) => item.status }
 *     ],
 *     actions: (item) => <Button>Edit</Button>
 *   }}
 *   getRowKey={(item) => item.id}
 * />
 * ```
 */
export function DataTable<T>({
  data,
  columns,
  mobileCard,
  getRowKey,
  emptyText = '暂无数据',
  containerClassName = '',
  onRowClick,
  rowClassName
}: DataTableProps<T>) {

  const renderCellContent = (column: DataTableColumn<T>, item: T) => {
    // 优先使用 cell 函数
    if (column.cell) {
      return column.cell(item)
    }

    // 其次使用 accessor
    if (typeof column.accessor === 'function') {
      return column.accessor(item)
    }

    if (column.accessor && typeof column.accessor === 'string') {
      const value = item[column.accessor]
      return value !== null && value !== undefined ? String(value) : '-'
    }

    return '-'
  }

  return (
    <>
      {/* 移动端卡片视图 */}
      {mobileCard && (
        <div className='md:hidden space-y-3'>
          {data.length === 0 ? (
            <Card>
              <CardContent className='text-center text-muted-foreground py-8'>
                {emptyText}
              </CardContent>
            </Card>
          ) : (
            data.map((item, index) => (
              <Card
                key={getRowKey(item, index)}
                className={`overflow-hidden ${mobileCard.cardClassName || ''}`}
                onClick={() => onRowClick?.(item)}
              >
                <CardContent className={`p-4 space-y-3 ${mobileCard.contentClassName || ''}`}>
                  {/* 卡片头部 */}
                  {mobileCard.header && (
                    <div>{mobileCard.header(item)}</div>
                  )}

                  {/* 卡片字段 */}
                  {mobileCard.fields?.map((field, fieldIndex) => {
                    if (field.hidden?.(item)) {
                      return null
                    }
                    return (
                      <div key={fieldIndex} className={`space-y-2 ${field.className || ''}`}>
                        <div className='text-xs text-muted-foreground'>{field.label}</div>
                        <div>{field.value(item)}</div>
                      </div>
                    )
                  })}

                  {/* 卡片操作按钮 */}
                  {mobileCard.actions && (
                    <div className='flex items-center gap-2 pt-2 border-t flex-wrap'>
                      {mobileCard.actions(item)}
                    </div>
                  )}
                </CardContent>
              </Card>
            ))
          )}
        </div>
      )}

      {/* 桌面端表格视图 */}
      <div className={`hidden md:block rounded-md border overflow-auto ${containerClassName}`}>
        <Table>
          <TableHeader>
            <TableRow>
              {columns.map((column, index) => (
                <TableHead
                  key={index}
                  className={column.headerClassName}
                  style={column.width ? { width: column.width } : undefined}
                >
                  {column.header}
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={columns.length}
                  className='text-center text-muted-foreground py-8'
                >
                  {emptyText}
                </TableCell>
              </TableRow>
            ) : (
              data.map((item, index) => (
                <TableRow
                  key={getRowKey(item, index)}
                  className={rowClassName?.(item)}
                  onClick={() => onRowClick?.(item)}
                >
                  {columns.map((column, colIndex) => (
                    <TableCell
                      key={colIndex}
                      className={column.cellClassName}
                    >
                      {renderCellContent(column, item)}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    </>
  )
}
