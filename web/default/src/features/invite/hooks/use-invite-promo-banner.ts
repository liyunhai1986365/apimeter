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
import { useCallback, useState } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { isAgentSiteStatus } from '@/lib/server-address'
import { useStatus } from '@/hooks/use-status'
import { getInviteRewardConfig, hasInviteRewards } from '../lib/reward-config'

const DISMISSED_STORAGE_KEY = 'invite_promo_banner_dismissed'

function getStoredDismissedState(): boolean {
  if (typeof window === 'undefined') return false
  return window.localStorage.getItem(DISMISSED_STORAGE_KEY) === '1'
}

export function useInvitePromoBanner() {
  const { status, loading } = useStatus()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const [dismissed, setDismissed] = useState(getStoredDismissedState)
  const config = getInviteRewardConfig(status)
  const dismiss = useCallback(() => {
    setDismissed(true)
    if (typeof window !== 'undefined') {
      window.localStorage.setItem(DISMISSED_STORAGE_KEY, '1')
    }
  }, [])

  return {
    dismiss,
    visible:
      !loading &&
      !isAgentSiteStatus(status) &&
      pathname === '/' &&
      !dismissed &&
      hasInviteRewards(config),
  }
}
