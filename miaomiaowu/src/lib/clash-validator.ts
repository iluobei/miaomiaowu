/**
 * Clash配置校验器
 * 用于在保存订阅前检查配置的有效性，避免mihomo启动失败
 */

export type ValidationLevel = 'error' | 'warning' | 'info'

export interface ValidationIssue {
  level: ValidationLevel
  message: string
  location?: string // 例如："proxy-groups[0]", "proxies[5]"
  field?: string // 例如："name", "proxies"
  autoFixed?: boolean // 是否已自动修复
}

export interface ValidationResult {
  valid: boolean
  issues: ValidationIssue[]
  fixedConfig?: any // 修复后的配置（如果有自动修复）
}

/**
 * 校验Clash配置
 */
export function validateClashConfig(config: any): ValidationResult {
  const issues: ValidationIssue[] = []
  let fixedConfig = JSON.parse(JSON.stringify(config)) // 深拷贝

  // 1. 校验proxies
  const proxyIssues = validateProxies(fixedConfig.proxies || [])
  issues.push(...proxyIssues.issues)
  if (proxyIssues.fixed) {
    fixedConfig.proxies = proxyIssues.fixed
  }

  // 2. 校验proxy-groups
  const groupIssues = validateProxyGroups(
    fixedConfig['proxy-groups'] || [],
    fixedConfig.proxies || []
  )
  issues.push(...groupIssues.issues)
  if (groupIssues.fixed) {
    fixedConfig['proxy-groups'] = groupIssues.fixed
  }

  // 3. 检查循环引用
  const circularIssues = detectCircularReferences(fixedConfig['proxy-groups'] || [])
  issues.push(...circularIssues)

  // 判断是否有错误级别的问题
  const hasErrors = issues.some(issue => issue.level === 'error')

  return {
    valid: !hasErrors,
    issues,
    fixedConfig: issues.some(i => i.autoFixed) ? fixedConfig : undefined
  }
}

/**
 * 校验proxies数组
 */
function validateProxies(proxies: any[]): { issues: ValidationIssue[]; fixed?: any[] } {
  const issues: ValidationIssue[] = []
  const fixed: any[] = []
  const seenNames = new Set<string>()

  for (let i = 0; i < proxies.length; i++) {
    const proxy = proxies[i]
    const location = `proxies[${i}]`

    // 检查是否为对象
    if (!proxy || typeof proxy !== 'object') {
      issues.push({
        level: 'error',
        message: `代理节点 #${i + 1} 不是有效的对象`,
        location
      })
      continue
    }

    // 检查name字段是否存在
    if (!proxy.name || typeof proxy.name !== 'string' || proxy.name.trim() === '') {
      issues.push({
        level: 'error',
        message: `代理节点 #${i + 1} 缺少name字段或name为空`,
        location,
        field: 'name'
      })
      continue
    }

    const name = proxy.name.trim()

    // 检查name是否重复
    if (seenNames.has(name)) {
      issues.push({
        level: 'warning',
        message: `代理节点名称重复: "${name}"，已自动移除`,
        location,
        field: 'name',
        autoFixed: true
      })
      // 重复的节点不添加到fixed数组
      continue
    }
    seenNames.add(name)

    // 检查name是否为第一个字段
    const keys = Object.keys(proxy)
    if (keys.length > 0 && keys[0] !== 'name') {
      issues.push({
        level: 'warning',
        message: `代理节点 "${name}" 的name字段不是第一个字段，已自动调整`,
        location,
        field: 'name',
        autoFixed: true
      })
    }

    // 重新排序字段，确保name在第一位
    const orderedProxy = reorderProxyFields(proxy)
    fixed.push(orderedProxy)
  }

  return {
    issues,
    fixed: fixed.length > 0 ? fixed : undefined
  }
}

/**
 * 校验proxy-groups数组
 */
function validateProxyGroups(
  groups: any[],
  proxies: any[]
): { issues: ValidationIssue[]; fixed?: any[] } {
  const issues: ValidationIssue[] = []
  const fixed: any[] = []
  const seenNames = new Set<string>()
  const proxyNames = new Set(proxies.map(p => p.name))
  const groupNames = new Set(groups.map(g => g?.name).filter(Boolean))

  // 特殊节点名称
  const specialNodes = new Set([
    'DIRECT',
    'REJECT',
    'REJECT-DROP',
    'PROXY',
    'PASS',
  ])
  // 常见的拼写错误
  const spellingCorrections: Record<string, string> = {
    'DIRCT': 'DIRECT',
    'REJET': 'REJECT',
    'REJCT': 'REJECT',
  }

  for (let i = 0; i < groups.length; i++) {
    const group = groups[i]
    const location = `proxy-groups[${i}]`

    // 检查是否为对象
    if (!group || typeof group !== 'object') {
      issues.push({
        level: 'error',
        message: `代理组 #${i + 1} 不是有效的对象`,
        location
      })
      continue
    }

    // 检查name字段是否存在
    if (!group.name || typeof group.name !== 'string' || group.name.trim() === '') {
      issues.push({
        level: 'error',
        message: `代理组 #${i + 1} 缺少name字段或name为空`,
        location,
        field: 'name'
      })
      continue
    }

    const name = group.name.trim()

    // 检查name是否重复
    if (seenNames.has(name)) {
      issues.push({
        level: 'error',
        message: `代理组名称重复: "${name}"`,
        location,
        field: 'name'
      })
      continue
    }
    seenNames.add(name)

    // 检查name是否为第一个字段
    const keys = Object.keys(group)
    if (keys.length > 0 && keys[0] !== 'name') {
      issues.push({
        level: 'warning',
        message: `代理组 "${name}" 的name字段不是第一个字段，已自动调整`,
        location,
        field: 'name',
        autoFixed: true
      })
    }

    // 检查proxies、use、filter和include-all字段
    const hasProxies = Array.isArray(group.proxies) && group.proxies.length > 0
    const hasUse = Array.isArray(group.use) && group.use.length > 0
    const hasFilter = typeof group.filter === 'string' && group.filter.trim() !== ''
    const hasIncludeAll = group['include-all'] === true

    if (!hasProxies && !hasUse && !hasFilter && !hasIncludeAll) {
      issues.push({
        level: 'error',
        message: `代理组 "${name}" 的proxies、use、filter和include-all字段都为空或不存在`,
        location,
        field: 'proxies'
      })
      continue
    }

    // 处理proxies字段
    let fixedProxies = group.proxies || []
    if (hasProxies) {
      const uniqueProxies = new Set<string>()
      const validProxies: string[] = []
      let hasDuplicates = false

      for (const proxy of group.proxies) {
        if (typeof proxy !== 'string') {
          continue
        }

        // 检查重复
        if (uniqueProxies.has(proxy)) {
          hasDuplicates = true
          continue
        }

        // 修正常见拼写错误
        let correctedProxy = proxy
        if (spellingCorrections[proxy]) {
          correctedProxy = spellingCorrections[proxy]
          issues.push({
            level: 'warning',
            message: `代理组 "${name}" 中的节点引用 "${proxy}" 已自动修正为 "${correctedProxy}"`,
            location,
            field: 'proxies',
            autoFixed: true
          })
        }

        // 检查节点是否存在
        const isSpecial = specialNodes.has(correctedProxy)
        const isProxy = proxyNames.has(correctedProxy)
        const isGroup = groupNames.has(correctedProxy) && correctedProxy !== name // 不能引用自己

        if (!isSpecial && !isProxy && !isGroup) {
          issues.push({
            level: 'error',
            message: `代理组 "${name}" 引用了不存在的节点: "${correctedProxy}"`,
            location,
            field: 'proxies'
          })
          continue
        }

        uniqueProxies.add(correctedProxy)
        validProxies.push(correctedProxy)
      }

      if (hasDuplicates) {
        issues.push({
          level: 'warning',
          message: `代理组 "${name}" 的proxies字段包含重复引用，已自动去重`,
          location,
          field: 'proxies',
          autoFixed: true
        })
      }

      fixedProxies = validProxies
    }

    // 重新排序字段
    const orderedGroup = reorderGroupFields({ ...group, proxies: fixedProxies })
    fixed.push(orderedGroup)
  }

  return {
    issues,
    fixed: fixed.length > 0 ? fixed : undefined
  }
}

/**
 * 检测循环引用
 */
function detectCircularReferences(groups: any[]): ValidationIssue[] {
  const issues: ValidationIssue[] = []
  const groupMap = new Map<string, string[]>()

  // 构建引用图
  for (const group of groups) {
    if (!group.name) continue
    const refs = (group.proxies || [])
      .filter((p: any) => typeof p === 'string')
      .filter((p: string) => groups.some(g => g.name === p))
    groupMap.set(group.name, refs)
  }

  // DFS检测循环
  function hasCycle(node: string, visited: Set<string>, recStack: Set<string>, path: string[]): boolean {
    visited.add(node)
    recStack.add(node)
    path.push(node)

    const neighbors = groupMap.get(node) || []
    for (const neighbor of neighbors) {
      if (!visited.has(neighbor)) {
        if (hasCycle(neighbor, visited, recStack, path)) {
          return true
        }
      } else if (recStack.has(neighbor)) {
        // 找到循环
        const cycleStart = path.indexOf(neighbor)
        const cycle = [...path.slice(cycleStart), neighbor].join(' → ')
        issues.push({
          level: 'error',
          message: `检测到代理组循环引用: ${cycle}`,
          location: `proxy-groups[${node}]`
        })
        return true
      }
    }

    recStack.delete(node)
    path.pop()
    return false
  }

  const visited = new Set<string>()
  for (const [node] of groupMap) {
    if (!visited.has(node)) {
      hasCycle(node, visited, new Set(), [])
    }
  }

  return issues
}

/**
 * 重新排序代理节点字段
 */
function reorderProxyFields(proxy: any): any {
  const ordered: any = {}
  const priorityKeys = ['name', 'type', 'server', 'port']

  // 先添加优先字段
  for (const key of priorityKeys) {
    if (key in proxy) {
      ordered[key] = proxy[key]
    }
  }

  // 再添加其他字段
  for (const [key, value] of Object.entries(proxy)) {
    if (!priorityKeys.includes(key)) {
      ordered[key] = value
    }
  }

  return ordered
}

/**
 * 重新排序代理组字段
 */
function reorderGroupFields(group: any): any {
  const ordered: any = {}
  const priorityKeys = ['name', 'type', 'proxies', 'use', 'url', 'interval', 'strategy', 'lazy', 'hidden']

  // 先添加优先字段
  for (const key of priorityKeys) {
    if (key in group) {
      ordered[key] = group[key]
    }
  }

  // 再添加其他字段
  for (const [key, value] of Object.entries(group)) {
    if (!priorityKeys.includes(key)) {
      ordered[key] = value
    }
  }

  return ordered
}

/**
 * 格式化校验结果为用户友好的消息
 */
export function formatValidationIssues(issues: ValidationIssue[]): string {
  if (issues.length === 0) {
    return '✅ 配置校验通过'
  }

  const errors = issues.filter(i => i.level === 'error')
  const warnings = issues.filter(i => i.level === 'warning')
  const autoFixed = issues.filter(i => i.autoFixed)

  let message = ''

  // 辅助函数：提取错误消息的模式（去掉引号中的内容）
  const extractPattern = (msg: string): string => {
    return msg.replace(/"[^"]+"/g, '"{name}"')
  }

  // 辅助函数：从消息中提取名称
  const extractName = (msg: string): string | null => {
    const match = msg.match(/"([^"]+)"/)
    return match ? match[1] : null
  }

  // 辅助函数：格式化分组的问题
  const formatGroupedIssues = (issueList: ValidationIssue[], maxDisplay = 3): string => {
    // 按错误模式分组
    const grouped = new Map<string, ValidationIssue[]>()

    issueList.forEach(issue => {
      const pattern = extractPattern(issue.message)
      if (!grouped.has(pattern)) {
        grouped.set(pattern, [])
      }
      grouped.get(pattern)!.push(issue)
    })

    let result = ''
    let itemIndex = 1

    grouped.forEach((items, pattern) => {
      if (items.length === 1) {
        // 单个错误，直接显示
        const issue = items[0]
        result += `  ${itemIndex}. ${issue.message}`
        if (issue.location) {
          result += ` (位置: ${issue.location})`
        }
        result += '\n'
        itemIndex++
      } else {
        // 多个相同模式的错误，合并显示
        const names = items.map(i => extractName(i.message)).filter(Boolean)

        // 重建消息，将第一个名称替换为计数
        let baseMessage = pattern.replace('"{name}"', `${items.length} 个项目`)

        // 如果是关于"name字段位置"的警告，简化描述
        if (baseMessage.includes('name字段不是第一个字段')) {
          baseMessage = `${items.length} 个代理组的 name 字段位置需要调整`
        }

        result += `  ${itemIndex}. ${baseMessage}\n`

        // 只显示前几个受影响的项目名称
        if (names.length > 0) {
          const displayNames = names.slice(0, maxDisplay)
          const remaining = names.length - maxDisplay
          result += `     受影响: ${displayNames.join(', ')}`
          if (remaining > 0) {
            result += ` 等 ${remaining} 个`
          }
          result += '\n'
        }

        itemIndex++
      }
    })

    return result
  }

  if (errors.length > 0) {
    message += `❌ 发现 ${errors.length} 个错误:\n`
    message += formatGroupedIssues(errors, 5)
  }

  if (warnings.length > 0) {
    if (message) message += '\n'
    message += `⚠️ 发现 ${warnings.length} 个警告:\n`
    message += formatGroupedIssues(warnings, 5)
  }

  if (autoFixed.length > 0) {
    if (message) message += '\n'
    message += `🔧 已自动修复 ${autoFixed.length} 个问题`
  }

  return message
}
