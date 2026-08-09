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
import { getStatus } from '@/lib/api'
import { isAgentSiteStatus } from '@/lib/server-address'

const INVITE_FEATURE_PATHS = new Set(['/invite', '/partner', '/invite-rewards'])
let resolvedAgentSiteStatus: boolean | undefined

export function isInviteFeaturePath(path: string): boolean {
  const normalizedPath = path.split(/[?#]/, 1)[0].replace(/\/+$/, '') || '/'
  return INVITE_FEATURE_PATHS.has(normalizedPath)
}

function getStoredAgentSiteStatus(): boolean | undefined {
  if (typeof window === 'undefined') return undefined

  try {
    const saved = window.localStorage.getItem('status')
    return saved ? isAgentSiteStatus(JSON.parse(saved)) : undefined
  } catch {
    return undefined
  }
}

export async function isCurrentAgentSite(): Promise<boolean> {
  if (resolvedAgentSiteStatus !== undefined) return resolvedAgentSiteStatus

  const storedStatus = getStoredAgentSiteStatus()

  try {
    resolvedAgentSiteStatus = isAgentSiteStatus(await getStatus())
    return resolvedAgentSiteStatus
  } catch {
    return storedStatus ?? false
  }
}
