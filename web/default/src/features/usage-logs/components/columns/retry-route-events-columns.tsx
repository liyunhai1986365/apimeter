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
import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { formatTimestampToDate } from '@/lib/format'
import { DataTableColumnHeader } from '@/components/data-table'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import type { RetryRouteEvent } from '../../types'

function getActionVariant(action: string): StatusVariant {
  if (action === 'failover') return 'blue'
  if (action === 'retry') return 'orange'
  if (action === 'skip_retry') return 'grey'
  return 'neutral'
}

function getFinalStatusVariant(event: RetryRouteEvent): StatusVariant {
  if (event.final_success) return 'green'
  if (event.final_status === 'failed') return 'red'
  return 'grey'
}

function formatChannel(id: number, name: string) {
  if (!id) return '-'
  return name ? `${name} #${id}` : `#${id}`
}

export function useRetryRouteEventsColumns(): ColumnDef<RetryRouteEvent>[] {
  const { t } = useTranslation()

  return [
    {
      accessorKey: 'created_at',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Time')} />
      ),
      cell: ({ row }) => (
        <span className='font-mono text-xs tabular-nums'>
          {formatTimestampToDate(row.original.created_at, 'seconds')}
        </span>
      ),
      meta: { label: t('Time') },
    },
    {
      accessorKey: 'request_id',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Request ID')} />
      ),
      cell: ({ row }) => (
        <div className='flex max-w-[220px] flex-col gap-0.5'>
          <span className='truncate font-mono text-xs'>
            {row.original.request_id || '-'}
          </span>
          <span className='text-muted-foreground text-xs'>
            {t('Attempt')} {row.original.attempt_index + 1}
          </span>
        </div>
      ),
      meta: { label: t('Request ID') },
    },
    {
      accessorKey: 'rule_name',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Rule')} />
      ),
      cell: ({ row }) => (
        <div className='flex flex-col gap-0.5'>
          <span className='font-medium'>{row.original.rule_name || '-'}</span>
          <span className='text-muted-foreground text-xs'>
            {row.original.rule_source || '-'}
          </span>
        </div>
      ),
      meta: { label: t('Rule') },
    },
    {
      accessorKey: 'action',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Action')} />
      ),
      cell: ({ row }) => (
        <StatusBadge variant={getActionVariant(row.original.action)}>
          {t(row.original.action || 'Unknown')}
        </StatusBadge>
      ),
      meta: { label: t('Action') },
    },
    {
      accessorKey: 'original_model',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Model')} />
      ),
      cell: ({ row }) => (
        <span className='font-mono text-xs'>
          {row.original.original_model || '-'}
        </span>
      ),
      meta: { label: t('Model') },
    },
    {
      accessorKey: 'source_channel_id',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Source')} />
      ),
      cell: ({ row }) => (
        <div className='flex flex-col gap-0.5 text-sm'>
          <span>
            {formatChannel(
              row.original.source_channel_id,
              row.original.source_channel_name
            )}
          </span>
          <span className='text-muted-foreground text-xs'>
            {row.original.source_group || '-'}
          </span>
        </div>
      ),
      meta: { label: t('Source') },
    },
    {
      accessorKey: 'target_channel_id',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Target')} />
      ),
      cell: ({ row }) => (
        <div className='flex flex-col gap-0.5 text-sm'>
          <span>
            {formatChannel(
              row.original.target_channel_id,
              row.original.target_channel_name
            )}
          </span>
          <span className='text-muted-foreground text-xs'>
            {row.original.target_group || '-'}
          </span>
        </div>
      ),
      meta: { label: t('Target') },
    },
    {
      accessorKey: 'status_code',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Error')} />
      ),
      cell: ({ row }) => (
        <div className='max-w-[360px] text-sm'>
          <div className='flex flex-wrap items-center gap-2'>
            {row.original.status_code > 0 && (
              <StatusBadge variant='red'>
                {row.original.status_code}
              </StatusBadge>
            )}
            {row.original.error_code && (
              <span className='font-mono text-xs'>
                {row.original.error_code}
              </span>
            )}
          </div>
          {row.original.error_message && (
            <p className='text-muted-foreground mt-1 line-clamp-2 text-xs'>
              {row.original.error_message}
            </p>
          )}
        </div>
      ),
      meta: { label: t('Error') },
    },
    {
      accessorKey: 'final_status',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Final Status')} />
      ),
      cell: ({ row }) => (
        <StatusBadge variant={getFinalStatusVariant(row.original)}>
          {row.original.final_status || t('Pending')}
        </StatusBadge>
      ),
      meta: { label: t('Final Status') },
    },
  ]
}
