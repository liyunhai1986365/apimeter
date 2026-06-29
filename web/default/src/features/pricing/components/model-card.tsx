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
import { memo } from 'react'
import { ArrowRight, Copy, Percent } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { useGroupDiscountLabels } from '@/hooks/use-group-discount-labels'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { DEFAULT_TOKEN_UNIT } from '../constants'
import { buildModelCardPriceDisplay } from '../lib/model-card-price'
import { inferModelMetadata } from '../lib/model-metadata'
import type { PricingModel, TokenUnit } from '../types'
import { ModalityIcons } from './model-details-modalities'
import type { ModelPerfBadgeData } from './model-perf-badge'

export interface ModelCardProps {
  model: PricingModel
  onClick: () => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  perf?: ModelPerfBadgeData
}

export const ModelCard = memo(function ModelCard(props: ModelCardProps) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const discountLabels = useGroupDiscountLabels()
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT
  const priceRate = props.priceRate ?? 1
  const usdExchangeRate = props.usdExchangeRate ?? 1
  const showRechargePrice = props.showRechargePrice ?? false
  const vendorIcon = props.model.vendor_icon
    ? getLobeIcon(props.model.vendor_icon, 18)
    : null
  const initial = props.model.model_name?.charAt(0).toUpperCase() || '?'
  const metadata = inferModelMetadata(props.model)
  const priceDisplay = buildModelCardPriceDisplay(props.model, {
    tokenUnit,
    showRechargePrice,
    priceRate,
    usdExchangeRate,
    discountLabels,
  })
  const featuredEntries = priceDisplay.entries.filter((entry) => entry.featured)
  const secondaryEntries = priceDisplay.entries.filter(
    (entry) => !entry.featured
  )

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    copyToClipboard(props.model.model_name || '')
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      props.onClick()
    }
  }

  const formatPriceUnit = (unitLabel: string) => {
    if (unitLabel === '1M' || unitLabel === '1K') {
      return unitLabel
    }
    return t(unitLabel)
  }

  return (
    <div
      role='button'
      tabIndex={0}
      onClick={props.onClick}
      onKeyDown={handleKeyDown}
      className={cn(
        'group bg-background relative flex min-h-[238px] cursor-pointer flex-col overflow-hidden rounded-xl border px-3 pt-3 transition-all sm:px-4 sm:pt-4',
        'hover:border-foreground/18 hover:bg-muted/10 focus-visible:ring-ring/50 hover:shadow-sm focus-visible:ring-2 focus-visible:outline-none'
      )}
    >
      <div className='flex min-h-8 items-center justify-between gap-2'>
        <div className='flex min-w-0 items-center gap-2.5'>
          <div className='flex size-5 shrink-0 items-center justify-center'>
            {vendorIcon || (
              <span className='text-muted-foreground text-[11px] font-semibold'>
                {initial}
              </span>
            )}
          </div>
          <div className='min-w-0'>
            <h3 className='text-foreground line-clamp-2 font-mono text-[13px] leading-snug font-semibold break-all sm:text-sm'>
              {props.model.model_name}
            </h3>
          </div>
        </div>

        <div className='flex shrink-0 items-center gap-1.5'>
          <Tooltip>
            <TooltipTrigger
              render={
                <button
                  type='button'
                  onClick={handleCopy}
                  className='text-muted-foreground hover:text-foreground hover:bg-muted focus-visible:ring-ring/50 bg-background/90 rounded-md border p-1.5 opacity-0 shadow-sm transition-all group-hover:opacity-100 focus:opacity-100 focus-visible:ring-2 focus-visible:outline-none'
                  aria-label={t('Copy')}
                />
              }
            >
              <Copy className='size-3.5' />
            </TooltipTrigger>
            <TooltipContent side='top' className='text-xs'>
              {t('Copy')}
            </TooltipContent>
          </Tooltip>
        </div>
      </div>

      <div
        className='bg-muted/25 mt-3 flex min-w-0 items-center gap-2 rounded-lg px-2.5 py-2'
        aria-label={`${t('Input modalities')} → ${t('Output modalities')}`}
      >
        <ModalityIcons
          modalities={metadata.input_modalities}
          className='size-3.5'
        />
        <ArrowRight className='text-muted-foreground/70 size-3.5 shrink-0' />
        <ModalityIcons
          modalities={metadata.output_modalities}
          className='size-3.5'
        />
      </div>

      <div className='mt-3 flex flex-1 flex-col justify-end'>
        <div className='bg-muted/30 border-border/70 -mx-3 min-h-[5.25rem] border-t px-3 py-2.5 sm:-mx-4 sm:px-4'>
          <div className='flex items-center justify-between gap-2'>
            <span className='text-muted-foreground text-[11px] font-semibold tracking-wide uppercase'>
              {t(priceDisplay.billingLabelKey)}
            </span>
            <div className='flex min-w-0 shrink-0 items-center gap-1.5'>
              {priceDisplay.discountLabel && (
                <span className='inline-flex max-w-[92px] items-center gap-1 truncate rounded-full bg-emerald-500/10 px-1.5 py-0.5 text-[11px] font-semibold text-emerald-700 ring-1 ring-emerald-500/15 dark:text-emerald-300'>
                  <Percent className='size-3 shrink-0' />
                  <span className='truncate'>{priceDisplay.discountLabel}</span>
                </span>
              )}
            </div>
          </div>

          {priceDisplay.rawExpression ? (
            <code className='text-muted-foreground mt-1.5 line-clamp-2 font-mono text-[11px] leading-4 break-all'>
              {priceDisplay.rawExpression}
            </code>
          ) : (
            <div className='mt-1.5 flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1'>
              {featuredEntries.map((entry) => (
                <span
                  key={entry.key}
                  className='inline-flex min-w-0 items-baseline gap-1.5'
                >
                  <span className='text-muted-foreground text-[11px] font-medium whitespace-nowrap'>
                    {t(entry.labelKey)}
                  </span>
                  {priceDisplay.hasDiscount && (
                    <span className='text-muted-foreground/55 max-w-[4.25rem] truncate font-mono text-[11px] line-through'>
                      {entry.original}
                    </span>
                  )}
                  <span className='text-foreground font-mono text-[15px] leading-none font-bold whitespace-nowrap tabular-nums'>
                    {entry.current}
                  </span>
                  <span className='text-muted-foreground/65 text-[11px] whitespace-nowrap'>
                    /{formatPriceUnit(entry.unitLabel)}
                  </span>
                </span>
              ))}

              {secondaryEntries.map((entry) => (
                <span
                  key={entry.key}
                  className='text-muted-foreground inline-flex min-w-0 items-baseline gap-1 truncate text-[11px]'
                >
                  <span className='truncate'>{t(entry.labelKey)}</span>
                  <span className='font-mono whitespace-nowrap'>
                    {entry.current}
                  </span>
                  <span className='text-muted-foreground/60 whitespace-nowrap'>
                    /{formatPriceUnit(entry.unitLabel)}
                  </span>
                </span>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
})
