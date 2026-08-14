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
import {
  ApiIcon,
  BarChartIcon,
  Clock01Icon,
  MoneyReceiveCircleIcon,
  Wallet01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { BalanceForecast, UserWalletData } from '../types'

interface WalletStatsCardProps {
  user: UserWalletData | null
  totalUsedQuota?: number
  forecast?: BalanceForecast | null
  forecastLoading?: boolean
  loading?: boolean
}

function formatRunway(
  estimatedHours: number,
  t: (key: string, options?: Record<string, unknown>) => string
) {
  if (estimatedHours < 24) {
    return t('{{hours}} hours', {
      hours: Math.max(1, Math.ceil(estimatedHours)),
    })
  }
  return t('{{days}} days', {
    days: (estimatedHours / 24).toFixed(1),
  })
}

export function WalletStatsCard(props: WalletStatsCardProps) {
  const { t } = useTranslation()
  if (props.loading) {
    return (
      <div className='overflow-hidden rounded-lg border'>
        <div className='divide-border/60 grid grid-cols-2 divide-x md:grid-cols-4'>
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className='px-3 py-3 sm:px-5 sm:py-4'>
              <Skeleton className='h-3.5 w-20' />
              <Skeleton className='mt-2 h-7 w-28' />
              <Skeleton className='mt-1.5 h-3.5 w-24' />
            </div>
          ))}
        </div>
      </div>
    )
  }

  const hasCreditQuota = (props.user?.credit_quota ?? 0) > 0
  const forecast = props.forecast
  let forecastValue: string | null = '-'
  if (props.forecastLoading) {
    forecastValue = null
  } else if (forecast?.status === 'ready') {
    forecastValue = formatRunway(forecast.estimated_hours, t)
  } else if (forecast?.status === 'depleted') {
    forecastValue = t('Balance depleted')
  }

  let forecastDetails = (
    <div className='flex flex-col gap-1'>
      <p>{t('Not enough wallet usage data yet')}</p>
      <p>
        {t(
          'Keep using the wallet to see an estimate based on recent consumption.'
        )}
      </p>
    </div>
  )
  if (forecast?.status === 'ready') {
    forecastDetails = (
      <div className='flex flex-col gap-2'>
        <p>{t('Based on wallet usage over the last 24 hours and 7 days.')}</p>
        <dl className='grid grid-cols-[auto_1fr] gap-x-3 gap-y-1'>
          <dt>{t('Average per hour')}</dt>
          <dd className='text-right tabular-nums'>
            {formatQuota(forecast.hourly_consumption)}
          </dd>
          <dt>{t('Average per day')}</dt>
          <dd className='text-right tabular-nums'>
            {formatQuota(forecast.daily_consumption)}
          </dd>
          <dt>{t('Estimated depletion')}</dt>
          <dd className='text-right tabular-nums'>
            {formatTimestampToDate(forecast.estimated_exhausted_at)}
          </dd>
          <dt>{t('Updated')}</dt>
          <dd className='text-right tabular-nums'>
            {formatTimestampToDate(forecast.calculated_at)}
          </dd>
        </dl>
      </div>
    )
  } else if (forecast?.status === 'depleted') {
    forecastDetails = <p>{t('Balance depleted')}</p>
  }
  const stats = [
    {
      label: t('Current Balance'),
      value: formatQuota(props.user?.quota ?? 0),
      description: t('Remaining quota'),
      icon: Wallet01Icon,
    },
    ...(hasCreditQuota
      ? [
          {
            label: t('Credit Quota'),
            value: formatQuota(props.user?.credit_quota ?? 0),
            description: t('Credit amount awaiting repayment'),
            icon: MoneyReceiveCircleIcon,
          },
        ]
      : []),
    {
      label: t('Total Usage'),
      value: formatQuota(props.totalUsedQuota ?? props.user?.used_quota ?? 0),
      description: t('Total consumed quota'),
      icon: BarChartIcon,
    },
    {
      label: t('Estimated balance runway'),
      value: forecastValue,
      description: null,
      icon: Clock01Icon,
      loading: props.forecastLoading,
      tooltip: props.forecastLoading ? null : forecastDetails,
    },
    {
      label: t('API Requests'),
      value: (props.user?.request_count ?? 0).toLocaleString(),
      description: t('Total requests made'),
      icon: ApiIcon,
    },
  ]

  return (
    <div className='overflow-hidden rounded-lg border'>
      <TooltipProvider delay={200}>
        <div
          className={cn(
            'divide-border/60 grid grid-cols-2 divide-x',
            hasCreditQuota ? 'md:grid-cols-5' : 'md:grid-cols-4'
          )}
        >
          {stats.map((item) => {
            const content = (
              <>
                <div className='flex items-center gap-2'>
                  <HugeiconsIcon
                    icon={item.icon}
                    className='text-muted-foreground/60 size-3.5 shrink-0'
                  />
                  <div className='text-muted-foreground truncate text-xs font-medium tracking-wider uppercase'>
                    {item.label}
                  </div>
                </div>

                {item.loading ? (
                  <Skeleton className='mt-2 h-7 w-28' />
                ) : (
                  <div className='text-foreground mt-1.5 font-mono text-base font-bold tracking-tight break-all tabular-nums sm:mt-2 sm:text-2xl'>
                    {item.value}
                  </div>
                )}
                {item.description ? (
                  <div className='text-muted-foreground/60 mt-1 hidden text-xs md:block'>
                    {item.description}
                  </div>
                ) : null}
              </>
            )

            if (item.tooltip) {
              return (
                <Tooltip key={item.label}>
                  <TooltipTrigger
                    type='button'
                    className='focus-visible:ring-ring/50 cursor-help px-3 py-3 text-left focus-visible:ring-[3px] focus-visible:outline-none sm:px-5 sm:py-4'
                  >
                    {content}
                  </TooltipTrigger>
                  <TooltipContent
                    side='bottom'
                    align='start'
                    className='max-w-sm'
                  >
                    {item.tooltip}
                  </TooltipContent>
                </Tooltip>
              )
            }

            return (
              <div key={item.label} className='px-3 py-3 sm:px-5 sm:py-4'>
                {content}
              </div>
            )
          })}
        </div>
      </TooltipProvider>
    </div>
  )
}
