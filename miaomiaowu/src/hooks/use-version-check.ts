import { useQuery } from '@tanstack/react-query'

const CURRENT_VERSION = '0.2.2'
const GITHUB_API_URL = 'https://api.github.com/repos/Jimleerx/miaomiaowu/releases/latest'

interface GitHubRelease {
  tag_name: string
  html_url: string
  name: string
}

function compareVersions(current: string, latest: string): boolean {
  // Remove 'v' prefix if present
  const cleanCurrent = current.replace(/^v/, '')
  const cleanLatest = latest.replace(/^v/, '')

  const currentParts = cleanCurrent.split('.').map(Number)
  const latestParts = cleanLatest.split('.').map(Number)

  for (let i = 0; i < Math.max(currentParts.length, latestParts.length); i++) {
    const currentPart = currentParts[i] || 0
    const latestPart = latestParts[i] || 0

    if (latestPart > currentPart) return true
    if (latestPart < currentPart) return false
  }

  return false
}

async function fetchLatestVersion(): Promise<{ version: string; hasUpdate: boolean; url: string }> {
  try {
    const response = await fetch(GITHUB_API_URL)
    if (!response.ok) {
      throw new Error('Failed to fetch latest version')
    }

    const data: GitHubRelease = await response.json()
    const latestVersion = data.tag_name.replace(/^v/, '')
    const hasUpdate = compareVersions(CURRENT_VERSION, latestVersion)

    return {
      version: latestVersion,
      hasUpdate,
      url: data.html_url
    }
  } catch (error) {
    console.error('Error fetching latest version:', error)
    return {
      version: CURRENT_VERSION,
      hasUpdate: false,
      url: 'https://github.com/Jimleerx/miaomiaowu/releases'
    }
  }
}

export function useVersionCheck() {
  const { data } = useQuery({
    queryKey: ['version-check'],
    queryFn: fetchLatestVersion,
    staleTime: 1000 * 60 * 60, // 1 hour
    gcTime: 1000 * 60 * 60 * 24, // 24 hours
    retry: 1,
    refetchOnWindowFocus: false,
  })

  return {
    currentVersion: CURRENT_VERSION,
    latestVersion: data?.version || CURRENT_VERSION,
    hasUpdate: data?.hasUpdate || false,
    releaseUrl: data?.url || 'https://github.com/Jimleerx/miaomiaowu/releases'
  }
}
