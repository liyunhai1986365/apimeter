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
export type AuthRedirectTarget = {
  target: string
  external: boolean
}

export function resolveAuthRedirect(
  redirectTo?: string,
  allowedExternalRedirect?: string
): AuthRedirectTarget {
  if (redirectTo?.startsWith('/') && !redirectTo.startsWith('//')) {
    return { target: redirectTo, external: false }
  }

  if (redirectTo && redirectTo === allowedExternalRedirect) {
    return { target: redirectTo, external: true }
  }

  return { target: '/dashboard', external: false }
}
