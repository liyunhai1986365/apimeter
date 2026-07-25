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
import { useMemo } from 'react'
import { ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatNumber, formatQuota } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { TopupInfo } from '../types'

interface AffiliateRulesCardProps {
  topupInfo: TopupInfo | null
  loading?: boolean
}

export function AffiliateRulesCard({
  topupInfo,
  loading,
}: AffiliateRulesCardProps) {
  const { t } = useTranslation()

  const rules = useMemo(() => {
    const items: string[] = []
    const rewardRatio = topupInfo?.affiliate_topup_reward_ratio ?? 0
    const rewardLimit = topupInfo?.affiliate_topup_reward_limit ?? 0
    const inviterQuota = topupInfo?.quota_for_inviter ?? 0

    if (rewardRatio > 0) {
      items.push(
        t(
          'Invite a friend to register and top up. You can earn {{ratio}} of their top-up amount as a reward.',
          { ratio: formatPercentText(rewardRatio) }
        )
      )

      items.push(
        rewardLimit > 0
          ? t(
              'Each invited user brings you rewards for their first {{count}} top-ups.',
              { count: rewardLimit }
            )
          : t(
              'Each invited user can bring you top-up rewards without a count limit.'
            )
      )
    }

    if (inviterQuota > 0) {
      items.push(
        t('Invite a friend to register and instantly receive {{amount}}.', {
          amount: formatQuota(inviterQuota),
        })
      )
    }

    items.push(
      t(
        'Do not invite yourself with alternate accounts. Violations will result in reward recovery and serious cases may be banned.'
      )
    )

    return items
  }, [t, topupInfo])

  if (loading) {
    return (
      <Card className='bg-muted/20 py-0'>
        <CardContent className='space-y-3 p-3 sm:p-4'>
          <Skeleton className='h-5 w-24' />
          <div className='space-y-2'>
            <Skeleton className='h-9 rounded-lg' />
            <Skeleton className='h-9 rounded-lg' />
            <Skeleton className='h-9 rounded-lg' />
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card className='bg-muted/20 py-0'>
      <CardContent className='space-y-3 p-3 sm:p-4'>
        <div className='flex items-center gap-2.5'>
          <div className='bg-background flex size-8 shrink-0 items-center justify-center rounded-lg border'>
            <ShieldCheck className='text-muted-foreground size-4' />
          </div>
          <div className='flex min-w-0 flex-1 flex-wrap items-center gap-2'>
            <h3 className='text-sm font-semibold'>{t('Reward Rules')}</h3>
            <Badge variant='secondary'>
              {topupInfo?.affiliate_policy?.uses_default_role
                ? t('System default')
                : topupInfo?.affiliate_policy?.role_name || t('System default')}
            </Badge>
          </div>
        </div>

        <ol className='space-y-2'>
          {rules.map((rule, index) => (
            <li key={rule} className='flex gap-2 text-sm leading-5'>
              <span className='bg-background text-muted-foreground flex size-5 shrink-0 items-center justify-center rounded-full border text-[11px] font-semibold tabular-nums'>
                {index + 1}
              </span>
              <span className='text-muted-foreground'>{rule}</span>
            </li>
          ))}
        </ol>
      </CardContent>
    </Card>
  )
}

function formatPercentText(value: number) {
  return `${formatNumber(value)}%`
}
