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
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { BarChart3, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatLogQuota } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getModelProfitStats } from '@/features/usage-logs/api'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import { usageLogsManualRefreshQueryOptions } from '@/features/usage-logs/lib/query-options'
import { getDefaultTimeRange } from '@/features/usage-logs/lib/utils'
import type { ModelProfitStats } from '@/features/usage-logs/types'

const EMPTY_MODEL_PROFIT_STATS: ModelProfitStats = {
  quota: 0,
  base_quota: 0,
  cost_quota: 0,
  profit_quota: 0,
  items: [],
}

function toSeconds(date?: Date): number | undefined {
  return date ? Math.floor(date.getTime() / 1000) : undefined
}

function profitTextClass(value: number): string {
  if (value > 0) return 'text-emerald-600 dark:text-emerald-400'
  if (value < 0) return 'text-red-600 dark:text-red-400'
  return 'text-muted-foreground'
}

function SummaryMetric(props: {
  label: string
  value: number
  accent: string
  valueClassName?: string
}) {
  return (
    <Card size='sm' className='rounded-lg'>
      <CardContent className='flex items-center justify-between gap-4 py-1'>
        <div className='flex min-w-0 items-center gap-2'>
          <span className={cn('h-7 w-1 rounded-full', props.accent)} />
          <span className='text-muted-foreground truncate text-sm'>
            {props.label}
          </span>
        </div>
        <span
          className={cn(
            'font-mono text-base font-semibold tabular-nums',
            props.valueClassName
          )}
        >
          {formatLogQuota(props.value)}
        </span>
      </CardContent>
    </Card>
  )
}

export function ModelProfit() {
  const { t } = useTranslation()
  const [range, setRange] = useState<{ start?: Date; end?: Date }>(
    getDefaultTimeRange
  )

  const {
    data: stats = EMPTY_MODEL_PROFIT_STATS,
    isLoading,
    isFetching,
    refetch,
  } = useQuery({
    ...usageLogsManualRefreshQueryOptions,
    queryKey: [
      'model-profit-stats',
      range.start?.getTime(),
      range.end?.getTime(),
    ],
    queryFn: async () => {
      const result = await getModelProfitStats({
        start_timestamp: toSeconds(range.start),
        end_timestamp: toSeconds(range.end),
      })
      return result.success
        ? result.data || EMPTY_MODEL_PROFIT_STATS
        : EMPTY_MODEL_PROFIT_STATS
    },
    placeholderData: (previousData) => previousData,
  })

  return (
    <div className='space-y-4'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <div className='space-y-1'>
          <h1 className='flex items-center gap-2 text-xl font-semibold'>
            <BarChart3 className='text-primary size-5' />
            {t('Model Profit')}
          </h1>
          <p className='text-muted-foreground text-sm'>
            {t('Track model revenue, supplier cost, and profit by date range.')}
          </p>
        </div>
        <div className='flex flex-col gap-2 sm:flex-row sm:items-center'>
          <CompactDateTimeRangePicker
            start={range.start}
            end={range.end}
            onChange={({ start, end }) => {
              setRange({ start, end })
            }}
            className='sm:w-[360px]'
          />
          <Button
            type='button'
            variant='outline'
            size='icon'
            onClick={() => refetch()}
            disabled={isFetching}
            aria-label={t('Refresh')}
          >
            <RefreshCw className={cn('size-4', isFetching && 'animate-spin')} />
          </Button>
        </div>
      </div>

      <div className='grid gap-3 md:grid-cols-3'>
        {isLoading ? (
          <>
            <Skeleton className='h-16 rounded-lg' />
            <Skeleton className='h-16 rounded-lg' />
            <Skeleton className='h-16 rounded-lg' />
          </>
        ) : (
          <>
            <SummaryMetric
              label={t('Revenue')}
              value={stats.quota}
              accent='bg-sky-500'
            />
            <SummaryMetric
              label={t('Cost')}
              value={stats.cost_quota}
              accent='bg-amber-500'
            />
            <SummaryMetric
              label={t('Profit')}
              value={stats.profit_quota}
              accent={stats.profit_quota < 0 ? 'bg-red-500' : 'bg-emerald-500'}
              valueClassName={profitTextClass(stats.profit_quota)}
            />
          </>
        )}
      </div>

      <Card className='rounded-lg'>
        <CardHeader className='pb-0'>
          <CardTitle>{t('Model Profit Details')}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className='max-h-[calc(100dvh-18rem)] overflow-auto rounded-md border'>
            <Table>
              <TableHeader className='bg-muted/30 sticky top-0 z-10'>
                <TableRow>
                  <TableHead>{t('Model')}</TableHead>
                  <TableHead className='text-right'>{t('Requests')}</TableHead>
                  <TableHead className='text-right'>{t('Revenue')}</TableHead>
                  <TableHead className='text-right'>{t('Cost')}</TableHead>
                  <TableHead className='text-right'>{t('Profit')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {isLoading ? (
                  Array.from({ length: 6 }).map((_, index) => (
                    <TableRow key={index}>
                      <TableCell colSpan={5}>
                        <Skeleton className='h-6 rounded-md' />
                      </TableCell>
                    </TableRow>
                  ))
                ) : stats.items.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={5}
                      className='text-muted-foreground h-24 text-center text-sm'
                    >
                      {t('No model profit data')}
                    </TableCell>
                  </TableRow>
                ) : (
                  stats.items.map((item) => (
                    <TableRow key={item.model_name}>
                      <TableCell className='max-w-[280px] truncate font-medium'>
                        {item.model_name}
                      </TableCell>
                      <TableCell className='text-right font-mono tabular-nums'>
                        {item.request_count}
                      </TableCell>
                      <TableCell className='text-right font-mono tabular-nums'>
                        {formatLogQuota(item.quota)}
                      </TableCell>
                      <TableCell className='text-right font-mono tabular-nums'>
                        {formatLogQuota(item.cost_quota)}
                      </TableCell>
                      <TableCell
                        className={cn(
                          'text-right font-mono tabular-nums',
                          profitTextClass(item.profit_quota)
                        )}
                      >
                        {formatLogQuota(item.profit_quota)}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
