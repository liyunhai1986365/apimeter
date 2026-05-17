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
import { StatusBadge } from '@/components/status-badge'
import type { ChannelOperationRecord } from '../../types'

function getActionVariant(action: string) {
  if (action === 'enable') return 'green'
  if (action === 'disable') return 'red'
  return 'grey'
}

export function useOperationRecordsColumns(): ColumnDef<ChannelOperationRecord>[] {
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
      accessorKey: 'channel_id',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Channel')} />
      ),
      cell: ({ row }) => {
        const record = row.original
        return (
          <div className='flex flex-col gap-0.5'>
            <span className='font-medium'>{record.channel_name || '-'}</span>
            <span className='text-muted-foreground font-mono text-xs'>
              #{record.channel_id}
            </span>
          </div>
        )
      },
      meta: { label: t('Channel') },
    },
    {
      accessorKey: 'action',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Action')} />
      ),
      cell: ({ row }) => {
        const action = row.original.action
        const label = action === 'enable' ? t('Enable') : t('Disable')
        return <StatusBadge variant={getActionVariant(action)}>{label}</StatusBadge>
      },
      meta: { label: t('Action') },
    },
    {
      accessorKey: 'source',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Source')} />
      ),
      cell: ({ row }) => (
        <StatusBadge variant='blue'>
          {row.original.source === 'auto' ? t('Automatic') : t('Manual')}
        </StatusBadge>
      ),
      meta: { label: t('Source') },
    },
    {
      accessorKey: 'model_name',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Model')} />
      ),
      cell: ({ row }) => (
        <span className='font-mono text-xs'>{row.original.model_name || '-'}</span>
      ),
      meta: { label: t('Model') },
    },
    {
      accessorKey: 'reason',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Reason')} />
      ),
      cell: ({ row }) => (
        <span className='line-clamp-2 max-w-[520px] text-sm'>
          {row.original.reason || '-'}
        </span>
      ),
      meta: { label: t('Reason') },
    },
  ]
}
