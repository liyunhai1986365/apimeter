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
import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { ArrowRight } from 'lucide-react'
import { getLobeIcon } from '@/lib/lobe-icon'
import { useGroupDiscountLabels } from '@/hooks/use-group-discount-labels'
import { Badge } from '@/components/ui/badge'
import { DataTableColumnHeader } from '@/components/data-table/column-header'
import {
  formatLatency,
  formatThroughput,
  formatUptimePct,
} from '@/features/performance-metrics/lib/format'
import type { PerfModelSummary } from '@/features/performance-metrics/types'
import { DEFAULT_TOKEN_UNIT } from '../constants'
import {
  buildModelCardPriceDisplay,
  type ModelCardPriceDisplay,
  type ModelCardPriceEntry,
} from '../lib/model-card-price'
import { inferModelMetadata } from '../lib/model-metadata'
import type { PricingModel, TokenUnit } from '../types'
import { ModalityIcons } from './model-details-modalities'

// ----------------------------------------------------------------------------
// Pricing Table Columns
// ----------------------------------------------------------------------------

export interface PricingColumnsOptions {
  tokenUnit?: TokenUnit
  priceRate?: number
  usdExchangeRate?: number
  showRechargePrice?: boolean
  perfByModel?: Map<string, PerfModelSummary>
}

const EMPTY_CELL = <span className='text-muted-foreground/45 text-xs'>-</span>

function formatPriceUnit(unitLabel: string, t: (key: string) => string) {
  if (unitLabel === '1M' || unitLabel === '1K') return unitLabel
  return t(unitLabel)
}

function findPriceEntries(
  display: ModelCardPriceDisplay,
  keys: string[]
): ModelCardPriceEntry[] {
  return display.entries.filter((entry) => keys.includes(entry.key))
}

function renderPriceEntries(
  entries: ModelCardPriceEntry[],
  display: ModelCardPriceDisplay,
  t: (key: string) => string
): React.ReactNode {
  if (entries.length === 0) return EMPTY_CELL

  return (
    <div className='flex min-w-[8.5rem] flex-col items-end gap-1 text-right leading-5'>
      {entries.map((entry) => {
        const isRequestUnit = entry.unitLabel === 'request'
        const unit = isRequestUnit ? t('each') : formatPriceUnit(entry.unitLabel, t)
        const hasDiscount =
          display.hasDiscount && entry.original !== entry.current
        const specLabel = entry.specLabel || ''
        return (
          <div key={`${entry.key}:${specLabel}`} className='max-w-full'>
            {specLabel && (
              <span className='text-muted-foreground/65 mr-2 max-w-[5.5rem] truncate align-baseline text-[10px]'>
                {specLabel}
              </span>
            )}
            {hasDiscount && (
              <span className='text-muted-foreground/55 mr-2 font-mono text-[10px] line-through tabular-nums'>
                {entry.original}
              </span>
            )}
            <span className='text-foreground font-mono text-sm font-semibold tabular-nums'>
              {entry.current}
            </span>
            <span className='text-muted-foreground/55 ml-1.5 text-[10px]'>
              {isRequestUnit ? unit : `/ ${unit}`}
            </span>
          </div>
        )
      })}
    </div>
  )
}

function renderRequestPrice(
  display: ModelCardPriceDisplay,
  t: (key: string) => string
): React.ReactNode {
  const entries = findPriceEntries(display, ['request'])
  if (entries.length === 0) return EMPTY_CELL

  return (
    renderPriceEntries(entries, display, t)
  )
}

function renderRawExpression(
  display: ModelCardPriceDisplay,
  t: (key: string) => string
): React.ReactNode {
  if (!display.rawExpression) return EMPTY_CELL

  return (
    <div className='max-w-[11rem] text-right'>
      <div className='text-xs font-medium'>{t(display.billingLabelKey)}</div>
      <code className='text-muted-foreground/70 line-clamp-2 font-mono text-[10px] leading-4 break-all'>
        {display.rawExpression}
      </code>
    </div>
  )
}

function renderModalityFlow(model: PricingModel): React.ReactNode {
  const metadata = inferModelMetadata(model)
  return (
    <div className='flex min-w-[8rem] items-center gap-2.5'>
      <ModalityIcons
        modalities={metadata.input_modalities}
        className='size-3.5'
      />
      <ArrowRight className='text-muted-foreground/55 size-3.5 shrink-0' />
      <ModalityIcons
        modalities={metadata.output_modalities}
        className='size-3.5'
      />
    </div>
  )
}

function renderPerformance(
  perf: PerfModelSummary | undefined,
  t: (key: string) => string
): React.ReactNode {
  if (!perf) return EMPTY_CELL

  return (
    <div className='flex min-w-[16rem] items-center gap-3 text-xs'>
      {renderMetric(t('Success rate'), formatUptimePct(perf.success_rate))}
      {renderMetric(t('Latency'), formatLatency(perf.avg_latency_ms))}
      {renderMetric('TTFT', formatLatency(perf.avg_ttft_ms ?? 0))}
      {renderMetric('TPS', formatThroughput(perf.avg_tps))}
    </div>
  )
}

function renderMetric(label: string, value: string) {
  return (
    <div key={label} className='min-w-0'>
      <div className='text-muted-foreground/55 truncate text-[10px] leading-4'>
        {label}
      </div>
      <div className='text-foreground font-mono text-xs leading-5 whitespace-nowrap tabular-nums'>
        {value}
      </div>
    </div>
  )
}

export function usePricingColumns(
  options: PricingColumnsOptions = {}
): ColumnDef<PricingModel>[] {
  const { t } = useTranslation()
  const discountLabels = useGroupDiscountLabels()
  const {
    tokenUnit = DEFAULT_TOKEN_UNIT,
    priceRate = 1,
    usdExchangeRate = 1,
    showRechargePrice = false,
    perfByModel,
  } = options

  const priceOptions = {
    tokenUnit,
    showRechargePrice,
    priceRate,
    usdExchangeRate,
    discountLabels,
    includeAllDynamicTiers: true,
  }

  return [
    {
      accessorKey: 'model_name',
      meta: { label: t('Model') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Model')} />
      ),
      cell: ({ row }) => {
        const model = row.original
        const vendorIcon = model.vendor_icon
          ? getLobeIcon(model.vendor_icon, 14)
          : null

        return (
          <div className='flex min-w-[15rem] items-center gap-2.5'>
            <div className='flex size-4 shrink-0 items-center justify-center'>
              {vendorIcon || (
                <span className='text-muted-foreground text-[10px] font-semibold'>
                  {model.model_name?.charAt(0).toUpperCase() || '?'}
                </span>
              )}
            </div>
            <div className='min-w-0'>
              <div className='text-foreground truncate font-mono text-sm leading-5 font-semibold'>
                {model.model_name}
              </div>
            </div>
          </div>
        )
      },
      minSize: 250,
    },
    {
      id: 'input_price',
      meta: { label: t('Input'), align: 'right' },
      header: t('Input'),
      cell: ({ row }) => {
        const display = buildModelCardPriceDisplay(row.original, priceOptions)
        if (display.rawExpression) return renderRawExpression(display, t)
        if (display.kind === 'request') return renderRequestPrice(display, t)
        return renderPriceEntries(
          findPriceEntries(display, ['input', 'p']),
          display,
          t
        )
      },
      size: 168,
      enableSorting: false,
    },
    {
      id: 'output_price',
      meta: { label: t('Output'), align: 'right' },
      header: t('Output'),
      cell: ({ row }) => {
        const display = buildModelCardPriceDisplay(row.original, priceOptions)
        if (display.kind === 'request') return EMPTY_CELL
        if (display.rawExpression) return renderRawExpression(display, t)
        return renderPriceEntries(
          findPriceEntries(display, ['output', 'c']),
          display,
          t
        )
      },
      size: 168,
      enableSorting: false,
    },
    {
      id: 'cache_write_price',
      meta: { label: t('Cache Write'), align: 'right' },
      header: t('Cache Write'),
      cell: ({ row }) => {
        const display = buildModelCardPriceDisplay(row.original, priceOptions)
        if (display.rawExpression) return EMPTY_CELL
        return renderPriceEntries(
          findPriceEntries(display, [
            'create_cache',
            'cc',
            'cc1h',
            'cacheWritePrice',
            'cacheCreatePrice',
            'cacheCreate1hPrice',
          ]),
          display,
          t
        )
      },
      size: 168,
      enableSorting: false,
    },
    {
      id: 'cache_read_price',
      meta: { label: t('Cache Read'), align: 'right' },
      header: t('Cache Read'),
      cell: ({ row }) => {
        const display = buildModelCardPriceDisplay(row.original, priceOptions)
        if (display.rawExpression) return EMPTY_CELL
        return renderPriceEntries(
          findPriceEntries(display, ['cache', 'cr', 'cacheReadPrice']),
          display,
          t
        )
      },
      size: 168,
      enableSorting: false,
    },
    {
      id: 'discount',
      meta: { label: t('Discount') },
      header: t('Discount'),
      cell: ({ row }) => {
        const display = buildModelCardPriceDisplay(row.original, priceOptions)
        return display.discountLabel ? (
          <Badge variant='secondary' className='rounded-md px-2 text-[11px]'>
            {display.discountLabel}
          </Badge>
        ) : (
          <span className='text-muted-foreground/45 text-xs'>-</span>
        )
      },
      size: 120,
      enableSorting: false,
    },
    {
      id: 'modalities',
      meta: { label: t('Modalities') },
      header: t('Modalities'),
      cell: ({ row }) => renderModalityFlow(row.original),
      size: 152,
      enableSorting: false,
    },
    {
      id: 'performance',
      meta: { label: t('Performance') },
      header: t('Performance'),
      cell: ({ row }) =>
        renderPerformance(perfByModel?.get(row.original.model_name || ''), t),
      size: 260,
      enableSorting: false,
    },
    {
      accessorKey: 'vendor_name',
      meta: { label: t('Vendor') },
      header: t('Vendor'),
      cell: ({ row }) => {
        const model = row.original
        if (!model.vendor_name) return EMPTY_CELL
        const vendorIcon = model.vendor_icon
          ? getLobeIcon(model.vendor_icon, 14)
          : null
        return (
          <span className='text-muted-foreground flex min-w-[9rem] items-center gap-2 text-xs'>
            {vendorIcon}
            <span className='truncate'>{model.vendor_name}</span>
          </span>
        )
      },
      size: 156,
      enableSorting: false,
    },
  ]
}
