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
import { useCallback, useEffect, useState } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import { Gift, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useStatus } from '@/hooks/use-status'
import {
  formatInviteRegisterReward,
  formatInviteRewardRatio,
  getInviteRewardConfig,
  hasInviteRewards,
} from '../lib/reward-config'

type InvitePromoBannerProps = {
  className?: string
  onDismiss?: () => void
  visible?: boolean
}

const DISMISSED_STORAGE_KEY = 'invite_promo_banner_dismissed'

function getStoredDismissedState(): boolean {
  if (typeof window === 'undefined') {
    return false
  }

  return window.localStorage.getItem(DISMISSED_STORAGE_KEY) === '1'
}

export function useInvitePromoBanner() {
  const { status } = useStatus()
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

  useEffect(() => {
    setDismissed(getStoredDismissedState())
  }, [pathname])

  return {
    dismiss,
    visible: pathname === '/' && !dismissed && hasInviteRewards(config),
  }
}

export function InvitePromoBanner({
  className,
  onDismiss,
  visible,
}: InvitePromoBannerProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const config = getInviteRewardConfig(status)

  if (
    visible === false ||
    pathname === '/invite' ||
    !hasInviteRewards(config)
  ) {
    return null
  }

  const rewardLabel =
    config.topupRewardRatio > 0
      ? t('Earn {{ratio}} on invited top-ups', {
          ratio: formatInviteRewardRatio(config.topupRewardRatio),
        })
      : t('Earn {{amount}} for each friend who registers', {
          amount: formatInviteRegisterReward(config.inviterRegisterQuota),
        })

  return (
    <div
      className={cn(
        'relative z-30 h-[var(--invite-promo-banner-height)] border-b border-emerald-500/20 bg-[linear-gradient(90deg,#06281d,#0f5132_45%,#155e44)] text-white shadow-[0_12px_32px_-24px_rgba(16,185,129,0.9)]',
        className
      )}
    >
      <Link
        to='/invite'
        className='mx-auto flex h-full max-w-7xl items-center justify-center px-12 text-center text-sm font-semibold transition-colors hover:bg-white/8 sm:px-16'
      >
        <span className='flex min-w-0 items-center justify-center gap-2'>
          <Gift className='size-4 shrink-0' />
          <span className='truncate'>
            {t('Invite friends. Stack rewards.')}
            <span className='mx-2 text-emerald-100/70'>·</span>
            <span className='font-medium text-emerald-50/90'>
              {rewardLabel}
              {config.topupRewardLimit > 0
                ? ` · ${t('First {{count}} top-ups count', {
                    count: config.topupRewardLimit,
                  })}`
                : ''}
            </span>
          </span>
        </span>
      </Link>
      <button
        type='button'
        className='absolute top-1/2 right-3 inline-flex size-7 -translate-y-1/2 items-center justify-center rounded-full text-white/80 transition-colors hover:bg-white/12 hover:text-white'
        aria-label={t('Close')}
        onClick={onDismiss}
      >
        <X className='size-4' />
      </button>
    </div>
  )
}
