import { api } from '@/lib/api'
import type { PredefinedRuleSetType, ProxyGroupCategory } from '@/lib/sublink/types'

/**
 * Fetch proxy group categories from the API
 * @returns Promise of proxy group categories array
 */
export async function fetchProxyGroupCategories(): Promise<ProxyGroupCategory[]> {
  try {
    const response = await api.get<ProxyGroupCategory[]>('/api/proxy-groups')
    return response.data ?? []
  } catch (error) {
    console.error('Failed to fetch proxy group categories:', error)
    throw error
  }
}

/**
 * Sync proxy group categories from remote source (GitHub)
 * @param sourceUrl - Optional custom source URL, defaults to environment config
 * @returns Promise of sync result
 */
export async function syncProxyGroupCategories(sourceUrl?: string): Promise<{ message: string; timestamp: string }> {
  try {
    const response = await api.post<{ message: string; timestamp: string }>(
      '/api/admin/proxy-groups/sync',
      sourceUrl ? { source_url: sourceUrl } : {}
    )
    return response.data
  } catch (error) {
    console.error('Failed to sync proxy group categories:', error)
    throw error
  }
}

/**
 * Filter categories by preset type
 * @param categories - All available categories
 * @param preset - Preset type to filter by
 * @returns Filtered categories for the given preset
 */
export function filterCategoriesByPreset(
  categories: ProxyGroupCategory[],
  preset: PredefinedRuleSetType
): ProxyGroupCategory[] {
  if (preset === 'custom') {
    return categories
  }
  return categories.filter((category) => category.presets.includes(preset))
}

/**
 * Create a map of category name to category object for quick lookup
 * @param categories - Array of categories
 * @returns Map with category name as key
 */
export function createCategoryNameMap(
  categories: ProxyGroupCategory[]
): Map<string, ProxyGroupCategory> {
  return new Map(categories.map((category) => [category.name, category]))
}

/**
 * Create a map of preset to category names
 * @param categories - Array of categories
 * @returns Object with preset as key and array of category names as value
 */
export function createPresetMap(categories: ProxyGroupCategory[]): Record<string, string[]> {
  const map: Record<string, string[]> = {}

  for (const category of categories) {
    for (const preset of category.presets) {
      if (!map[preset]) {
        map[preset] = []
      }
      map[preset].push(category.name)
    }
  }

  return map
}

/**
 * Get category names for a given preset, maintaining the order from categories array
 * @param categories - All available categories
 * @param preset - Preset type
 * @returns Array of category names in order
 */
export function getCategoryNamesForPreset(
  categories: ProxyGroupCategory[],
  preset: PredefinedRuleSetType
): string[] {
  if (preset === 'custom') {
    return []
  }
  return categories
    .filter((category) => category.presets.includes(preset))
    .map((category) => category.name)
}

/**
 * Extract all unique group labels from categories
 * Useful for quick-select buttons in UI
 * @param categories - Array of categories
 * @returns Array of unique group labels
 */
export function extractGroupLabels(categories: ProxyGroupCategory[]): string[] {
  const labels = new Set<string>()
  for (const category of categories) {
    if (category.group_label) {
      labels.add(category.group_label)
    }
  }
  return Array.from(labels)
}
