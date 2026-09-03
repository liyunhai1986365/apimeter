export function sanitizeStatusForCache(
  status: Record<string, unknown>
): Record<string, unknown> {
  const cacheable = { ...status }
  delete cacheable.announcements
  return cacheable
}

export function readCachedStatus(): Record<string, unknown> | null {
  try {
    if (typeof window === 'undefined') return null
    const raw = window.localStorage.getItem('status')
    if (!raw) return null
    return sanitizeStatusForCache(JSON.parse(raw) as Record<string, unknown>)
  } catch {
    return null
  }
}

export function writeCachedStatus(status: Record<string, unknown>): void {
  try {
    if (typeof window === 'undefined') return
    window.localStorage.setItem(
      'status',
      JSON.stringify(sanitizeStatusForCache(status))
    )
  } catch {
    /* empty */
  }
}
