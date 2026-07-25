import type { AuthUser } from '@/stores/auth-store'

const WORKSPACE_ACCOUNT_ALLOWED_PATHS = new Set([
  '/keys',
  '/dashboard/overview',
  '/usage-logs/common',
  '/profile',
])

function normalizePathname(pathname: string): string {
  if (pathname.length > 1 && pathname.endsWith('/')) {
    return pathname.slice(0, -1)
  }
  return pathname
}

export function canManageTeamSettings(user: AuthUser | null): boolean {
  return Boolean(user && !user.workspace_subaccount)
}

export function getWorkspaceAccountRedirect(
  user: AuthUser | null,
  pathname: string
): '/change-password' | '/keys' | null {
  if (!user?.workspace_subaccount) return null

  const normalizedPathname = normalizePathname(pathname)

  if (user.must_change_password) {
    return normalizedPathname === '/change-password' ? null : '/change-password'
  }

  if (normalizedPathname === '/change-password') return '/keys'

  return WORKSPACE_ACCOUNT_ALLOWED_PATHS.has(normalizedPathname)
    ? null
    : '/keys'
}
