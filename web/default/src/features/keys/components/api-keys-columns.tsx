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
import { type ColumnDef } from '@tanstack/react-table'
import { Ban } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getUserGroups } from '@/lib/api'
import { formatTimestampToDate } from '@/lib/format'
import { USER_FACING_GROUP_TERMS } from '@/lib/user-facing-group-terms'
import { cn } from '@/lib/utils'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { DataTableColumnHeader } from '@/components/data-table'
import { GroupBadge } from '@/components/group-badge'
import { StatusBadge } from '@/components/status-badge'
import { API_KEY_STATUSES } from '../constants'
import {
  getApiKeyGroupDisplayItems,
  getApiKeyRoutingStrategyLabel,
} from '../lib/api-key-form'
import { type ApiKey } from '../types'
import {
  ApiKeyCell,
  ApiKeyQuotaCell,
  ModelLimitsCell,
  IpRestrictionsCell,
} from './api-keys-cells'
import { DataTableRowActions } from './data-table-row-actions'

function useGroupRatios(): Record<string, number> {
  const { data } = useQuery({
    queryKey: ['user-self-groups'],
    queryFn: getUserGroups,
    staleTime: 5 * 60 * 1000,
    select: (res) => {
      if (!res.success || !res.data) return {}
      const ratios: Record<string, number> = {}
      for (const [group, info] of Object.entries(res.data)) {
        if (typeof info.ratio === 'number') {
          ratios[group] = info.ratio
        }
      }
      return ratios
    },
  })

  return data ?? {}
}

export function useApiKeysColumns(): ColumnDef<ApiKey>[] {
  const { t } = useTranslation()
  const groupRatios = useGroupRatios()
  return [
    {
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllPageRowsSelected()}
          indeterminate={table.getIsSomePageRowsSelected()}
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label='Select all'
          className='translate-y-[2px]'
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label='Select row'
          className='translate-y-[2px]'
        />
      ),
      enableSorting: false,
      enableHiding: false,
      meta: { label: t('Select') },
    },
    {
      accessorKey: 'name',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Name')} />
      ),
      cell: ({ row }) => (
        <div className='max-w-[200px] truncate font-medium'>
          {row.getValue('name')}
        </div>
      ),
      meta: { label: t('Name'), mobileTitle: true },
    },
    {
      accessorKey: 'status',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Status')} />
      ),
      cell: ({ row }) => {
        const statusConfig = API_KEY_STATUSES[row.getValue('status') as number]
        if (!statusConfig) return null
        return (
          <StatusBadge
            label={t(statusConfig.label)}
            variant={statusConfig.variant}
            showDot={statusConfig.showDot}
            copyable={false}
          />
        )
      },
      filterFn: (row, id, value) => value.includes(String(row.getValue(id))),
      meta: { label: t('Status'), mobileBadge: true },
    },
    {
      id: 'key',
      accessorKey: 'key',
      header: t('API Key'),
      cell: ({ row }) => <ApiKeyCell apiKey={row.original} />,
      enableSorting: false,
      meta: { label: t('API Key') },
    },
    {
      id: 'quota',
      accessorKey: 'remain_quota',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Quota')} />
      ),
      cell: ({ row }) => {
        return <ApiKeyQuotaCell apiKey={row.original} />
      },
      meta: { label: t('Quota') },
    },
    {
      accessorKey: 'group',
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t(USER_FACING_GROUP_TERMS.single)}
        />
      ),
      cell: ({ row }) => {
        const apiKey = row.original
        const groupDisplay = getApiKeyGroupDisplayItems(apiKey)
        if (groupDisplay.routingStrategy) {
          const excludedGroups = groupDisplay.excludedGroups
          return (
            <span className='inline-flex max-w-[260px] items-center gap-1.5 overflow-hidden'>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <span className='inline-flex items-center gap-1.5 text-xs' />
                  }
                >
                  <span className='border-primary/20 bg-primary/10 text-primary inline-flex rounded-md border px-2 py-0.5 text-xs font-medium'>
                    {t(
                      getApiKeyRoutingStrategyLabel(
                        groupDisplay.routingStrategy
                      )
                    )}
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  <span className='text-xs'>{t('Smart routing')}</span>
                </TooltipContent>
              </Tooltip>
              {excludedGroups.length > 0 && (
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <span className='border-destructive/25 bg-destructive/10 text-destructive inline-flex min-w-0 items-center gap-1 rounded-md border px-1.5 py-0.5 text-xs font-medium' />
                    }
                  >
                    <Ban className='size-3 shrink-0' />
                    <span className='truncate'>{excludedGroups[0]}</span>
                    {excludedGroups.length > 1 && (
                      <span className='shrink-0'>
                        +{excludedGroups.length - 1}
                      </span>
                    )}
                  </TooltipTrigger>
                  <TooltipContent side='bottom' className='max-w-64'>
                    <div className='space-y-2'>
                      <div className='flex items-center gap-1.5 text-xs font-medium'>
                        <Ban className='size-3.5' />
                        {t('Ignored suppliers')}
                      </div>
                      <div className='flex max-w-56 flex-wrap gap-1.5'>
                        {excludedGroups.map((group) => (
                          <GroupBadge
                            key={group}
                            group={group}
                            ratio={groupRatios[group]}
                          />
                        ))}
                      </div>
                    </div>
                  </TooltipContent>
                </Tooltip>
              )}
            </span>
          )
        }

        const primaryGroup = groupDisplay.allGroups[0]

        if (primaryGroup === 'auto') {
          return (
            <Tooltip>
              <TooltipTrigger
                render={
                  <span className='inline-flex items-center gap-1.5 text-xs' />
                }
              >
                <GroupBadge group='auto' />
                {apiKey.cross_group_retry && (
                  <>
                    <span className='text-muted-foreground/30'>·</span>
                    <span className='text-muted-foreground/60'>
                      {t(USER_FACING_GROUP_TERMS.crossRetryShort)}
                    </span>
                  </>
                )}
              </TooltipTrigger>
              <TooltipContent>
                <span className='text-xs'>
                  {t(USER_FACING_GROUP_TERMS.autoDescription)}
                </span>
              </TooltipContent>
            </Tooltip>
          )
        }

        const content = (
          <span className='inline-flex max-w-[220px] items-center gap-1.5 overflow-hidden'>
            {groupDisplay.visibleGroups.map((group) => (
              <GroupBadge
                key={group}
                group={group}
                ratio={groupRatios[group]}
              />
            ))}
            {groupDisplay.hiddenCount > 0 && (
              <span className='text-muted-foreground inline-flex shrink-0 rounded-md border px-1.5 py-0.5 text-xs font-medium'>
                +{groupDisplay.hiddenCount}
              </span>
            )}
          </span>
        )

        if (groupDisplay.allGroups.length <= 1) {
          return content
        }

        return (
          <Tooltip>
            <TooltipTrigger render={<span className='inline-flex' />}>
              {content}
            </TooltipTrigger>
            <TooltipContent>
              <div className='space-y-1.5'>
                {groupDisplay.allGroups.map((group, index) => (
                  <div key={group} className='flex items-center gap-2 text-xs'>
                    <span className='text-muted-foreground w-4 text-right font-mono tabular-nums'>
                      {index + 1}
                    </span>
                    <GroupBadge group={group} ratio={groupRatios[group]} />
                  </div>
                ))}
              </div>
            </TooltipContent>
          </Tooltip>
        )
      },
      meta: { label: t(USER_FACING_GROUP_TERMS.single), mobileHidden: true },
    },
    {
      id: 'model_limits',
      accessorKey: 'model_limits',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Models')} />
      ),
      cell: ({ row }) => <ModelLimitsCell apiKey={row.original} />,
      enableSorting: false,
      meta: { label: t('Models'), mobileHidden: true },
    },
    {
      id: 'allow_ips',
      accessorKey: 'allow_ips',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('IP Restriction')} />
      ),
      cell: ({ row }) => <IpRestrictionsCell apiKey={row.original} />,
      enableSorting: false,
      meta: { label: t('IP Restriction'), mobileHidden: true },
    },
    {
      accessorKey: 'created_time',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Created')} />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground font-mono text-xs tabular-nums'>
          {formatTimestampToDate(row.getValue('created_time'))}
        </span>
      ),
      meta: { label: t('Created'), mobileHidden: true },
    },
    {
      accessorKey: 'accessed_time',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Last Used')} />
      ),
      cell: ({ row }) => {
        const accessedTime = row.getValue('accessed_time') as number
        if (!accessedTime) {
          return <span className='text-muted-foreground text-xs'>-</span>
        }
        return (
          <span className='text-muted-foreground font-mono text-xs tabular-nums'>
            {formatTimestampToDate(accessedTime)}
          </span>
        )
      },
      meta: { label: t('Last Used'), mobileHidden: true },
    },
    {
      accessorKey: 'expired_time',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Expires')} />
      ),
      cell: ({ row }) => {
        const expiredTime = row.getValue('expired_time') as number
        if (expiredTime === -1) {
          return (
            <StatusBadge
              label={t('Never')}
              variant='neutral'
              copyable={false}
            />
          )
        }
        const isExpired = expiredTime * 1000 < Date.now()
        return (
          <span
            className={cn(
              'font-mono text-xs tabular-nums',
              isExpired ? 'text-destructive' : 'text-muted-foreground'
            )}
          >
            {formatTimestampToDate(expiredTime)}
          </span>
        )
      },
      meta: { label: t('Expires'), mobileHidden: true },
    },
    {
      id: 'actions',
      cell: ({ row }) => <DataTableRowActions row={row} />,
      meta: { label: t('Actions') },
      size: 88,
    },
  ]
}
