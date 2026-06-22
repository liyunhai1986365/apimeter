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
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { formatLatency, formatThroughput, formatUptimePct } from '../lib/format'
import type { PerfGroupSummary } from '../types'

export function GroupPerformanceInline(props: {
  perf?: PerfGroupSummary
  compact?: boolean
  className?: string
}) {
  const { t } = useTranslation()
  if (!props.perf || props.perf.request_count <= 0) return null

  const itemClass = props.compact
    ? 'rounded px-1.5 py-0.5'
    : 'rounded-md px-2 py-1'

  return (
    <div
      className={cn(
        'text-muted-foreground flex min-w-0 shrink-0 flex-wrap items-center gap-1.5 text-xs',
        props.className
      )}
    >
      <span className={cn('bg-muted/35 font-mono tabular-nums', itemClass)}>
        {t('Success rate')} {formatUptimePct(props.perf.success_rate)}
      </span>
      <span className={cn('bg-muted/35 font-mono tabular-nums', itemClass)}>
        {t('Latency short')} {formatLatency(props.perf.avg_latency_ms)}
      </span>
      <span className={cn('bg-muted/35 font-mono tabular-nums', itemClass)}>
        TPS {formatThroughput(props.perf.avg_tps)}
      </span>
    </div>
  )
}
