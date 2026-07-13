/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
const OAUTH_REDIRECT_STORAGE_PREFIX = 'oauth-login-redirect:'

export function storeOAuthRedirect(state: string, redirectTo?: string): void {
  if (!redirectTo || typeof window === 'undefined') return
  try {
    window.sessionStorage.setItem(
      `${OAUTH_REDIRECT_STORAGE_PREFIX}${state}`,
      redirectTo
    )
  } catch {
    /* empty */
  }
}

export function consumeOAuthRedirect(state?: string): string | undefined {
  if (!state || typeof window === 'undefined') return undefined
  const key = `${OAUTH_REDIRECT_STORAGE_PREFIX}${state}`
  try {
    const redirectTo = window.sessionStorage.getItem(key) ?? undefined
    window.sessionStorage.removeItem(key)
    return redirectTo
  } catch {
    return undefined
  }
}
