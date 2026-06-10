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
import { useQuery } from '@tanstack/react-query'
import { KeyRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatQuota } from '@/lib/format'
import { Skeleton } from '@/components/ui/skeleton'
import { getWorkspaceUsageStats } from '../api'
import { type WorkspaceUsageStats } from '../types'
import { ApiKeysPrimaryButtons } from './api-keys-primary-buttons'
import { useApiKeys } from './api-keys-provider'
import { ApiKeysTable } from './api-keys-table'
import { WorkspaceQuotaManagement } from './workspace-quota-management'

function WorkspaceUsageStatsStrip({
  stats,
  isLoading,
  isError,
}: {
  stats?: WorkspaceUsageStats
  isLoading: boolean
  isError: boolean
}) {
  const { t } = useTranslation()

  if (isLoading) {
    return (
      <div className='flex max-w-full min-w-0 flex-nowrap items-center justify-start gap-2 overflow-x-auto'>
        {Array.from({ length: 4 }).map((_, index) => (
          <Skeleton
            key={index}
            className='h-12 w-[5.75rem] shrink-0 rounded-md'
          />
        ))}
      </div>
    )
  }

  if (isError) {
    return (
      <span className='text-muted-foreground bg-muted/40 rounded-md border px-2.5 py-1.5 text-xs'>
        {t('Failed to load workspace usage')}
      </span>
    )
  }

  const items = [
    { label: t('Today usage'), value: stats?.today_quota ?? 0 },
    { label: t('This week usage'), value: stats?.week_quota ?? 0 },
    { label: t('This month usage'), value: stats?.month_quota ?? 0 },
    { label: t('Total used'), value: stats?.total_used_quota ?? 0 },
  ]

  return (
    <dl
      aria-label={t('Workspace usage stats')}
      className='flex max-w-full min-w-0 flex-nowrap items-center justify-start gap-2 overflow-x-auto'
    >
      {items.map((item) => (
        <div
          key={item.label}
          className='bg-background text-foreground flex min-h-12 min-w-[5.75rem] shrink-0 flex-col items-center justify-center rounded-md border px-2.5 py-1.5 text-center shadow-xs'
        >
          <dd className='font-mono text-sm leading-5 font-semibold tracking-normal tabular-nums sm:text-base'>
            {formatQuota(item.value)}
          </dd>
          <dt className='text-muted-foreground truncate text-[11px] leading-4'>
            {item.label}
          </dt>
        </div>
      ))}
    </dl>
  )
}

export function ApiKeyWorkspaceDetail() {
  const { t } = useTranslation()
  const { selectedWorkspace, isLoadingWorkspaces } = useApiKeys()
  const usageStatsQuery = useQuery({
    queryKey: ['workspace-usage-stats', selectedWorkspace?.id],
    queryFn: async () => {
      if (!selectedWorkspace?.id) return undefined
      const res = await getWorkspaceUsageStats(selectedWorkspace.id)
      if (!res.success) {
        throw new Error(res.message || 'Failed to load workspace usage')
      }
      return res.data
    },
    enabled: !!selectedWorkspace?.id,
    staleTime: 60 * 1000,
  })

  if (isLoadingWorkspaces && !selectedWorkspace) {
    return (
      <section className='min-w-0 space-y-3'>
        <div className='flex flex-col gap-3 px-1 py-1 sm:flex-row sm:items-center sm:justify-between'>
          <div className='min-w-0 space-y-2'>
            <Skeleton className='h-5 w-44' />
            <Skeleton className='h-4 w-72 max-w-full' />
          </div>
          <Skeleton className='h-8 w-32' />
        </div>
        <ApiKeysTable />
      </section>
    )
  }

  return (
    <section className='min-w-0 space-y-3'>
      <div className='flex flex-col gap-3 px-1 py-1 lg:flex-row lg:items-start lg:justify-between'>
        <div className='flex min-w-0 flex-1 items-start gap-3'>
          <div className='bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-md border'>
            <KeyRound className='size-4' />
          </div>
          <div className='flex min-w-0 flex-1 flex-col gap-2 md:flex-row md:items-center'>
            <div className='min-w-0 md:max-w-72 md:shrink-0'>
              <h2 className='truncate text-sm font-semibold'>
                {selectedWorkspace
                  ? t('API Keys in {{workspace}}', {
                      workspace: selectedWorkspace.name,
                    })
                  : t('API Keys in selected workspace')}
              </h2>
              <p className='text-muted-foreground mt-1 truncate text-xs'>
                {t('Workspace totals are calculated from linked API keys.')}
              </p>
            </div>
            {selectedWorkspace && (
              <div className='min-w-0 md:flex-1'>
                <WorkspaceUsageStatsStrip
                  stats={usageStatsQuery.data}
                  isLoading={usageStatsQuery.isLoading}
                  isError={usageStatsQuery.isError}
                />
              </div>
            )}
          </div>
        </div>
        <div className='flex shrink-0 items-start gap-5 lg:ml-auto'>
          {selectedWorkspace && (
            <WorkspaceQuotaManagement workspaceId={selectedWorkspace.id} />
          )}
          <ApiKeysPrimaryButtons />
        </div>
      </div>

      <ApiKeysTable />
    </section>
  )
}
