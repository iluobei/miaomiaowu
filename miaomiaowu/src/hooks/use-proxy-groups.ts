import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchProxyGroupCategories, syncProxyGroupCategories } from '@/lib/sublink/proxy-groups'
import type { ProxyGroupCategory } from '@/lib/sublink/types'

/**
 * Hook to fetch and cache proxy group categories
 * Automatically refetches on window focus and keeps data fresh
 */
export function useProxyGroupCategories() {
  return useQuery<ProxyGroupCategory[], Error>({
    queryKey: ['proxy-group-categories'],
    queryFn: fetchProxyGroupCategories,
    staleTime: 1000 * 60 * 5,      // Consider data fresh for 5 minutes
    gcTime: 1000 * 60 * 60,         // Keep in cache for 1 hour
    retry: 2,                       // Retry failed requests twice
    refetchOnWindowFocus: true,     // Refetch when window regains focus
  })
}

/**
 * Hook to sync proxy group categories from remote source
 * Invalidates cache on success to trigger refetch
 */
export function useSyncProxyGroupCategories() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (sourceUrl?: string) => syncProxyGroupCategories(sourceUrl),
    onSuccess: () => {
      // Invalidate and refetch proxy group categories after successful sync
      queryClient.invalidateQueries({ queryKey: ['proxy-group-categories'] })
    },
  })
}
