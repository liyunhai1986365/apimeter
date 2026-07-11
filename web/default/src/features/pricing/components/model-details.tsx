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
import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate, useParams, useSearch } from '@tanstack/react-router'
import {
  CircleDollarSign,
  ArrowLeft,
  ChevronDown,
  Code2,
  Info,
} from 'lucide-react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { formatGroupDiscount } from '@/lib/group-discount'
import { getLobeIcon } from '@/lib/lobe-icon'
import { USER_FACING_GROUP_TERMS } from '@/lib/user-facing-group-terms'
import { cn } from '@/lib/utils'
import { useGroupDiscountLabels } from '@/hooks/use-group-discount-labels'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { CopyButton } from '@/components/copy-button'
import { PublicLayout } from '@/components/layout'
import { getPerfMetrics } from '@/features/performance-metrics/api'
import {
  formatLatency,
  formatThroughput,
} from '@/features/performance-metrics/lib/format'
import type { PerformanceGroup } from '@/features/performance-metrics/types'
import { DEFAULT_TOKEN_UNIT, QUOTA_TYPE_VALUES } from '../constants'
import { usePricingData } from '../hooks/use-pricing-data'
import {
  buildDynamicTierPriceDisplayRows,
  getDynamicPriceUnitLabelKey,
  getDynamicPricingTiers,
  isDynamicPricingModel,
  type DynamicPriceEntry,
  type DynamicTierPriceDisplayRow,
} from '../lib/dynamic-price'
import { parseTags } from '../lib/filters'
import { isModelPriceFreeForRatio } from '../lib/model-card-price'
import { getAvailableGroups, isTokenBasedModel } from '../lib/model-helpers'
import { inferModelMetadata } from '../lib/model-metadata'
import { toGroupUptimeSeries } from '../lib/performance-series'
import { formatFixedPrice, formatGroupPrice } from '../lib/price'
import type {
  Modality,
  ModelCapability,
  PriceType,
  PricingGroupDisplayConfig,
  PricingModel,
  TokenUnit,
} from '../types'
import { ModelDetailsApi, ModelDetailsProviderInfo } from './model-details-api'
import {
  getUsableGroupDescription,
  type PricingUsableGroupMap,
} from './model-details-group-name'
import { ModalityIcons } from './model-details-modalities'
import { ModelDetailsPerformance } from './model-details-performance'
import { ModelDetailsQuickStats } from './model-details-quick-stats'
import {
  MODEL_DETAILS_STICKY_BAR_CLASS,
  MODEL_DETAILS_STICKY_INFO_CLASS,
} from './model-details-sticky-layout'
import { UptimeSparkline } from './model-details-uptime-sparkline'
import {
  getFloatingPublicTopChromeHeight,
  PRICING_DETAILS_TOP_PADDING_CLASS,
} from './pricing-layout'

// ----------------------------------------------------------------------------
// Local UI helpers
// ----------------------------------------------------------------------------

type ModelDetailsVariant = 'compact' | 'page'

function SectionTitle(props: {
  children: React.ReactNode
  className?: string
}) {
  return (
    <h2
      className={cn(
        'text-muted-foreground mb-3 text-xs font-semibold tracking-wider uppercase',
        props.className
      )}
    >
      {props.children}
    </h2>
  )
}

const CAPABILITY_LABEL_KEYS: Record<ModelCapability, string> = {
  function_calling: 'Function calling',
  streaming: 'Streaming',
  vision: 'Vision',
  json_mode: 'JSON mode',
  structured_output: 'Structured output',
  reasoning: 'Reasoning',
  tools: 'Tools',
  system_prompt: 'System prompt',
  web_search: 'Web search',
  code_interpreter: 'Code interpreter',
  caching: 'Prompt caching',
  embeddings: 'Embeddings',
}

function CompactCapabilityList(props: { capabilities: ModelCapability[] }) {
  const { t } = useTranslation()

  if (props.capabilities.length === 0) {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('No capabilities reported for this model.')}
      </span>
    )
  }

  return (
    <div className='flex flex-wrap gap-1.5'>
      {props.capabilities.map((capability) => (
        <span
          key={capability}
          className='bg-muted text-muted-foreground rounded-md px-2 py-1 text-xs font-medium'
        >
          {t(CAPABILITY_LABEL_KEYS[capability] ?? capability)}
        </span>
      ))}
    </div>
  )
}

function CompactModalities(props: { input: Modality[]; output: Modality[] }) {
  const { t } = useTranslation()

  return (
    <div className='grid gap-2 sm:grid-cols-2'>
      <div className='flex items-center justify-between gap-3 rounded-lg border px-3 py-2'>
        <span className='text-muted-foreground text-xs font-medium'>
          {t('Input')}
        </span>
        <ModalityIcons modalities={props.input} />
      </div>
      <div className='flex items-center justify-between gap-3 rounded-lg border px-3 py-2'>
        <span className='text-muted-foreground text-xs font-medium'>
          {t('Output')}
        </span>
        <ModalityIcons modalities={props.output} />
      </div>
    </div>
  )
}

function ModelAliasesPanel(props: { aliases: string[] }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  const primaryAlias = props.aliases[0]
  const hasMultipleAliases = props.aliases.length > 1

  useEffect(() => {
    if (!open) return

    const handlePointerDown = (event: PointerEvent) => {
      const container = containerRef.current

      if (container && !container.contains(event.target as Node)) {
        setOpen(false)
      }
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpen(false)
      }
    }

    document.addEventListener('pointerdown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)

    return () => {
      document.removeEventListener('pointerdown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [open])

  if (!primaryAlias) return null

  return (
    <div ref={containerRef} className='relative mt-1 inline-block max-w-full'>
      <div className='flex min-w-0 items-center gap-1 text-xs'>
        <span className='text-muted-foreground shrink-0'>{t('Alias')}:</span>
        <code className='text-muted-foreground min-w-0 truncate font-mono'>
          {primaryAlias}
        </code>
        <CopyButton
          value={primaryAlias}
          className='text-muted-foreground hover:text-foreground size-4 p-0'
          iconClassName='size-3'
          tooltip={t('Copy alias')}
          successTooltip={t('Copied!')}
          aria-label={t('Copy alias')}
        />
        {hasMultipleAliases && (
          <button
            type='button'
            className='text-muted-foreground hover:text-foreground flex size-4 shrink-0 items-center justify-center rounded transition-colors'
            onClick={() => setOpen((current) => !current)}
            aria-expanded={open}
            aria-label={t('Expand aliases')}
          >
            <ChevronDown
              className={cn(
                'size-3 transition-transform',
                open && 'rotate-180'
              )}
            />
          </button>
        )}
      </div>

      {hasMultipleAliases && open && (
        <div className='bg-popover text-popover-foreground ring-foreground/10 absolute top-full left-0 z-50 mt-1 grid w-fit max-w-[min(28rem,calc(100vw-2rem))] min-w-full gap-1 rounded-lg p-2 shadow-md ring-1'>
          {props.aliases.map((alias) => (
            <div
              key={alias}
              className='hover:bg-muted/60 flex min-w-0 items-center gap-2 rounded-md px-2 py-1.5'
            >
              <code className='text-foreground min-w-0 flex-1 truncate font-mono text-xs'>
                {alias}
              </code>
              <CopyButton
                value={alias}
                className='size-5 p-0'
                iconClassName='size-3.5'
                tooltip={t('Copy alias')}
                successTooltip={t('Copied!')}
                aria-label={t('Copy alias')}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function ModelSignalsSection(props: {
  capabilities: ModelCapability[]
  input: Modality[]
  output: Modality[]
  variant?: ModelDetailsVariant
}) {
  const { t } = useTranslation()
  const isPage = props.variant === 'page'

  return (
    <section
      className={cn(
        isPage && 'bg-background/70 rounded-2xl border p-5 shadow-sm md:p-6'
      )}
    >
      <SectionTitle className={cn(isPage && 'mb-4')}>
        {t('Capabilities')} / {t('Supported modalities')}
      </SectionTitle>
      <div
        className={cn(
          'grid gap-3 rounded-xl border p-3 @2xl/details:grid-cols-[minmax(0,1.5fr)_minmax(260px,1fr)]',
          isPage && 'bg-muted/10 p-4'
        )}
      >
        <CompactCapabilityList capabilities={props.capabilities} />
        <CompactModalities input={props.input} output={props.output} />
      </div>
    </section>
  )
}

// ----------------------------------------------------------------------------
// Model header (always visible above the detail sections)
// ----------------------------------------------------------------------------

function ModelHeader(props: {
  model: PricingModel
  variant?: ModelDetailsVariant
}) {
  const { t } = useTranslation()
  const [expandedDescriptionModel, setExpandedDescriptionModel] = useState<
    string | null
  >(null)
  const [descriptionExpandable, setDescriptionExpandable] = useState(false)
  const descriptionRef = useRef<HTMLParagraphElement>(null)
  const model = props.model
  const isPage = props.variant === 'page'
  const vendorIcon = model.vendor_icon
    ? getLobeIcon(model.vendor_icon, isPage ? 32 : 20)
    : null
  const description = model.description || model.vendor_description || null
  const tags = parseTags(model.tags)
  const aliasModels = model.alias_models || []
  const isSpecialExpression =
    model.billing_mode === 'tiered_expr' &&
    Boolean(model.billing_expr) &&
    getDynamicPricingTiers(model).length === 0
  const descriptionExpanded = expandedDescriptionModel === model.model_name

  useEffect(() => {
    if (!isPage || !description || descriptionExpanded) return

    const paragraph = descriptionRef.current
    if (!paragraph) return

    const measure = () => {
      setDescriptionExpandable(
        paragraph.scrollHeight > paragraph.clientHeight + 1
      )
    }

    measure()

    const resizeObserver = new ResizeObserver(measure)
    resizeObserver.observe(paragraph)

    return () => resizeObserver.disconnect()
  }, [description, descriptionExpanded, isPage])

  return (
    <header className={cn('pb-4', isPage && 'pb-7')}>
      <div
        className={cn(
          isPage &&
            'grid gap-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start'
        )}
      >
        <div className='min-w-0'>
          <div className={cn('flex items-center gap-2.5', isPage && 'gap-3')}>
            {vendorIcon}
            <h1
              className={cn(
                'font-mono text-xl font-bold tracking-tight sm:text-2xl',
                isPage &&
                  'min-w-0 text-[clamp(2rem,5vw,3.75rem)] leading-[1.08] break-all'
              )}
            >
              {model.model_name}
            </h1>
            <CopyButton
              value={model.model_name || ''}
              className={cn('size-6 shrink-0', isPage && 'size-8')}
              iconClassName={cn('size-3', isPage && 'size-4')}
              tooltip={t('Copy model name')}
              successTooltip={t('Copied!')}
              aria-label={t('Copy model name')}
            />
          </div>
          <ModelAliasesPanel aliases={aliasModels} />
        </div>

        <div
          className={cn(
            'mt-1 flex flex-wrap items-center gap-1.5 text-xs',
            isPage &&
              'mt-0 justify-start gap-2 text-sm lg:max-w-xs lg:justify-end lg:text-right'
          )}
        >
          {model.vendor_name && (
            <span className='text-muted-foreground'>{model.vendor_name}</span>
          )}
          <span className='text-muted-foreground/30'>·</span>
          <span className='text-muted-foreground/70'>
            {model.quota_type === QUOTA_TYPE_VALUES.TOKEN
              ? t('Token-based')
              : t('Per Request')}
          </span>
          {model.billing_mode === 'tiered_expr' && model.billing_expr && (
            <>
              <span className='text-muted-foreground/30'>·</span>
              <span className='rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-500/20 dark:text-amber-300'>
                {isSpecialExpression
                  ? t('Special billing expression')
                  : t('Dynamic Pricing')}
              </span>
            </>
          )}
        </div>
      </div>
      {description && (
        <div className={cn('mt-2', isPage && 'mt-4 w-full')}>
          <p
            ref={descriptionRef}
            className={cn(
              'text-muted-foreground text-sm leading-relaxed',
              isPage && !descriptionExpanded && 'line-clamp-3'
            )}
          >
            {description}
          </p>
          {isPage && descriptionExpandable && (
            <button
              type='button'
              className='text-muted-foreground hover:text-foreground mt-2 text-xs font-medium transition-colors'
              onClick={() =>
                setExpandedDescriptionModel((current) =>
                  current === model.model_name ? null : model.model_name
                )
              }
              aria-expanded={descriptionExpanded}
            >
              {descriptionExpanded ? t('Collapse') : t('More...')}
            </button>
          )}
        </div>
      )}
      {tags.length > 0 && (
        <div className={cn('mt-2.5 flex flex-wrap gap-1', isPage && 'mt-4')}>
          {tags.map((tag) => (
            <span
              key={tag}
              className={cn(
                'bg-muted text-muted-foreground rounded px-2 py-0.5 text-[11px] font-medium',
                isPage && 'rounded-md px-2.5 py-1 text-xs'
              )}
            >
              {tag}
            </span>
          ))}
        </div>
      )}
    </header>
  )
}

// ----------------------------------------------------------------------------
// Group pricing cards
// ----------------------------------------------------------------------------

function getDiscountBadgeClass() {
  return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/15 dark:text-emerald-300'
}

function getDiscountLabel(
  ratio: number,
  labels: ReturnType<typeof useGroupDiscountLabels>
) {
  if (!(ratio > 0 && ratio < 1)) return undefined
  return formatGroupDiscount(ratio, labels)
}

function ProviderGroupTitle(props: { group: string; desc?: string }) {
  const { t } = useTranslation()
  const label = (
    <span className='text-foreground inline-flex max-w-full truncate text-base font-semibold'>
      {props.group}
    </span>
  )

  if (!props.desc) return label

  return (
    <TooltipProvider delay={120}>
      <Tooltip>
        <TooltipTrigger render={label} />
        <TooltipContent side='top' className='max-w-72 leading-relaxed'>
          {t(props.desc)}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

function ProviderPerformanceInline(props: {
  performance?: PerformanceGroup
  isLoading?: boolean
}) {
  if (props.isLoading) {
    return <Skeleton className='h-7 w-64 max-w-full rounded-md' />
  }

  if (!props.performance) {
    return null
  }

  const uptimeSeries = toGroupUptimeSeries(props.performance)

  return (
    <div className='text-muted-foreground flex min-w-0 shrink-0 items-center gap-2 overflow-hidden text-xs'>
      <span className='bg-muted/35 rounded-md px-2 py-1 font-mono font-semibold tabular-nums'>
        TPS {formatThroughput(props.performance.avg_tps)}
      </span>
      <span className='bg-muted/35 rounded-md px-2 py-1 font-mono font-semibold tabular-nums'>
        TTFT {formatLatency(props.performance.avg_ttft_ms)}
      </span>
      <span className='bg-muted/35 rounded-md px-2 py-1 font-mono font-semibold tabular-nums'>
        Latency {formatLatency(props.performance.avg_latency_ms)}
      </span>
      <UptimeSparkline size='sm' series={uptimeSeries} />
    </div>
  )
}

function ProviderPriceItem(props: {
  label: string
  value: React.ReactNode
  originalValue?: React.ReactNode
  featured?: boolean
  unit?: string
}) {
  return (
    <div
      className={cn(
        'flex min-w-52 shrink-0 items-center justify-start gap-2 px-1 py-1'
      )}
    >
      {props.label ? (
        <span
          className={cn(
            'text-muted-foreground min-w-0 truncate text-xs font-medium',
            props.featured && 'text-sm'
          )}
        >
          {props.label}
        </span>
      ) : null}
      <span
        className={cn(
          'text-foreground flex shrink-0 items-baseline gap-1.5 font-mono text-sm font-semibold tabular-nums',
          props.featured && 'text-base md:text-lg'
        )}
      >
        {props.originalValue ? (
          <span className='text-muted-foreground/45 text-xs font-medium line-through'>
            {props.originalValue}
          </span>
        ) : null}
        {props.value}
        {props.unit ? (
          <span className='text-muted-foreground/40 font-sans text-[10px]'>
            / {props.unit}
          </span>
        ) : null}
      </span>
    </div>
  )
}

function ProviderFreePriceItem() {
  const { t } = useTranslation()

  return (
    <div className='flex min-w-52 shrink-0 items-center justify-start gap-1.5 px-1 py-1'>
      <span className='bg-primary/10 text-primary inline-flex size-6 shrink-0 items-center justify-center rounded-full'>
        <CircleDollarSign className='size-3.5' aria-hidden='true' />
      </span>
      <span className='text-foreground text-sm leading-none font-bold'>
        {t('Free')}
      </span>
    </div>
  )
}

function ProviderCacheWritePriceGroup(props: {
  renderTokenPrice: (type: PriceType) => string
  renderOriginalTokenPrice: (type: PriceType) => string
  hasDiscount: boolean
  unit: string
}) {
  const { t } = useTranslation()
  const rows: Array<{ label: string; type: PriceType }> = [
    { label: t('Cache Write (5m)'), type: 'create_cache' },
    { label: t('Cache Write (1h)'), type: 'create_cache_1h' },
  ]

  return (
    <div className='flex min-w-52 shrink-0 flex-col justify-center gap-1 px-1 py-1'>
      {rows.map((row) => (
        <div
          key={row.type}
          className='flex items-baseline justify-between gap-3'
        >
          <span className='text-muted-foreground min-w-0 truncate text-xs font-medium'>
            {row.label}
          </span>
          <span className='text-foreground flex shrink-0 items-baseline gap-1.5 font-mono text-sm font-semibold tabular-nums'>
            {props.hasDiscount ? (
              <span className='text-muted-foreground/45 text-xs font-medium line-through'>
                {props.renderOriginalTokenPrice(row.type)}
              </span>
            ) : null}
            {props.renderTokenPrice(row.type)}
            <span className='text-muted-foreground/40 font-sans text-[10px]'>
              / {props.unit}
            </span>
          </span>
        </div>
      ))}
    </div>
  )
}

function DynamicCacheWritePriceGroup(props: {
  entries: DynamicPriceEntry[]
  originalEntries?: DynamicPriceEntry[]
  unit?: string
}) {
  const { t } = useTranslation()
  if (props.entries.length === 0) return null

  const originalByField = new Map(
    (props.originalEntries || []).map((entry) => [entry.field, entry])
  )

  return (
    <div className='inline-flex min-w-52 flex-col gap-1 text-right'>
      {props.entries.map((entry) => {
        const originalEntry = originalByField.get(entry.field)
        return (
          <div
            key={entry.field}
            className='flex items-baseline justify-between gap-3'
          >
            <span className='text-muted-foreground min-w-0 truncate text-xs font-medium'>
              {t(entry.shortLabel)}
            </span>
            <span className='text-foreground flex shrink-0 items-baseline gap-1.5 font-mono text-sm font-semibold tabular-nums'>
              {originalEntry && (
                <span className='text-muted-foreground/45 text-xs font-medium line-through'>
                  {originalEntry.formatted}
                </span>
              )}
              {entry.formatted}
              {props.unit ? (
                <span className='text-muted-foreground/40 font-sans text-[10px]'>
                  / {props.unit}
                </span>
              ) : null}
            </span>
          </div>
        )
      })}
    </div>
  )
}

function DynamicProviderPriceItem(props: {
  entry: DynamicPriceEntry
  originalEntry?: DynamicPriceEntry
  unit: string
  featured?: boolean
}) {
  const { t } = useTranslation()
  const unitLabel = getDynamicPriceUnitLabelKey(props.entry, props.unit)

  return (
    <ProviderPriceItem
      label={t(props.entry.shortLabel)}
      value={props.entry.formatted}
      originalValue={props.originalEntry?.formatted}
      unit={unitLabel === props.unit ? unitLabel : t(unitLabel)}
      featured={props.featured}
    />
  )
}

function DynamicTierPriceRail(props: {
  row: DynamicTierPriceDisplayRow
  unit: string
  showTierLabel: boolean
}) {
  const { t } = useTranslation()
  const originalRegularByField = new Map(
    props.row.originalRegularEntries.map((entry) => [entry.field, entry])
  )
  const featuredFields = new Set(['inputPrice', 'outputPrice'])

  return (
    <div className='flex min-w-max items-stretch gap-2'>
      {props.showTierLabel && (
        <div className='flex min-w-36 shrink-0 items-center px-1 py-1'>
          <span className='text-muted-foreground bg-muted/40 rounded-md px-2 py-1 text-xs font-medium'>
            {props.row.label || t('Default')}
          </span>
        </div>
      )}
      {props.row.regularEntries.map((entry) => (
        <DynamicProviderPriceItem
          key={entry.field}
          entry={entry}
          originalEntry={originalRegularByField.get(entry.field)}
          unit={props.unit}
          featured={featuredFields.has(entry.field)}
        />
      ))}
      {props.row.cacheWriteEntries.length > 0 && (
        <DynamicCacheWritePriceGroup
          entries={props.row.cacheWriteEntries}
          originalEntries={props.row.originalCacheWriteEntries}
          unit={props.unit}
        />
      )}
    </div>
  )
}

function DynamicProviderPriceCard(props: {
  group: string
  groupDesc?: string
  discountLabel?: string
  rows: DynamicTierPriceDisplayRow[]
  tokenUnitLabel: string
  performance?: PerformanceGroup
  performanceLoading?: boolean
  isPage?: boolean
}) {
  const { t } = useTranslation()
  const showTierLabel = props.rows.length > 1

  return (
    <Card
      size='sm'
      className={cn(
        'border-border/80 bg-background/80 hover:border-foreground/20 hover:bg-background gap-0 py-0 shadow-none transition-colors',
        props.isPage && 'rounded-2xl'
      )}
    >
      <CardHeader className='flex flex-row items-center justify-between gap-3 border-b px-3 py-2.5 sm:px-4'>
        <div className='flex min-w-0 flex-1 items-center gap-2.5 overflow-hidden'>
          <CardTitle className='min-w-0 shrink-0'>
            <ProviderGroupTitle group={props.group} desc={props.groupDesc} />
          </CardTitle>
          {props.discountLabel && (
            <span
              className={cn(
                'inline-flex shrink-0 rounded-md border px-2 py-0.5 text-xs font-semibold shadow-sm',
                getDiscountBadgeClass()
              )}
            >
              {props.discountLabel}
            </span>
          )}
          {props.groupDesc && (
            <CardDescription className='min-w-0 flex-1 truncate text-xs'>
              {t(props.groupDesc)}
            </CardDescription>
          )}
        </div>
        <ProviderPerformanceInline
          performance={props.performance}
          isLoading={props.performanceLoading}
        />
      </CardHeader>

      <CardContent className='overflow-x-auto px-3 py-3 sm:px-4'>
        <div className='flex min-w-max flex-col gap-2'>
          {props.rows.map((row) => (
            <DynamicTierPriceRail
              key={`${props.group}-${row.label}`}
              row={row}
              unit={props.tokenUnitLabel}
              showTierLabel={showTierLabel}
            />
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

function ProviderPriceCard(props: {
  model: PricingModel
  group: string
  groupDesc?: string
  discountLabel?: string
  ratio: number
  groupRatio: Record<string, number>
  tokenUnit: TokenUnit
  tokenUnitLabel: string
  showRechargePrice: boolean
  priceRate: number
  usdExchangeRate: number
  extraPriceTypes: { label: string; type: PriceType }[]
  showCacheWritePrices?: boolean
  performance?: PerformanceGroup
  performanceLoading?: boolean
  isPage?: boolean
}) {
  const { t } = useTranslation()
  const isTokenBased = isTokenBasedModel(props.model)
  const priceUnit = isTokenBased ? props.tokenUnitLabel : t('request')

  const renderTokenPrice = (type: PriceType) =>
    formatGroupPrice(
      props.model,
      props.group,
      type,
      props.tokenUnit,
      props.showRechargePrice,
      props.priceRate,
      props.usdExchangeRate,
      props.groupRatio
    )
  const renderOriginalTokenPrice = (type: PriceType) =>
    formatGroupPrice(
      props.model,
      props.group,
      type,
      props.tokenUnit,
      props.showRechargePrice,
      props.priceRate,
      props.usdExchangeRate,
      { ...props.groupRatio, [props.group]: 1 }
    )
  const renderFixedPrice = () =>
    formatFixedPrice(
      props.model,
      props.group,
      props.showRechargePrice,
      props.priceRate,
      props.usdExchangeRate,
      props.groupRatio
    )
  const renderOriginalFixedPrice = () =>
    formatFixedPrice(
      props.model,
      props.group,
      props.showRechargePrice,
      props.priceRate,
      props.usdExchangeRate,
      { ...props.groupRatio, [props.group]: 1 }
    )
  const isFree = isModelPriceFreeForRatio(props.model, props.ratio)
  const hasDiscount = props.ratio > 0 && props.ratio < 1

  return (
    <Card
      size='sm'
      className={cn(
        'border-border/80 bg-background/80 hover:border-foreground/20 hover:bg-background gap-0 py-0 shadow-none transition-colors',
        props.isPage && 'rounded-2xl'
      )}
    >
      <CardHeader className='flex flex-row items-center justify-between gap-3 border-b px-3 py-2.5 sm:px-4'>
        <div className='flex min-w-0 flex-1 items-center gap-2.5 overflow-hidden'>
          <CardTitle className='min-w-0 shrink-0'>
            <ProviderGroupTitle group={props.group} desc={props.groupDesc} />
          </CardTitle>
          {props.discountLabel && (
            <span
              className={cn(
                'inline-flex shrink-0 rounded-md border px-2 py-0.5 text-xs font-semibold shadow-sm',
                getDiscountBadgeClass()
              )}
            >
              {props.discountLabel}
            </span>
          )}
          {props.groupDesc && (
            <CardDescription className='min-w-0 flex-1 truncate text-xs'>
              {t(props.groupDesc)}
            </CardDescription>
          )}
        </div>
        <ProviderPerformanceInline
          performance={props.performance}
          isLoading={props.performanceLoading}
        />
      </CardHeader>

      <CardContent className='overflow-x-auto px-3 py-3 sm:px-4'>
        {isFree ? (
          <ProviderFreePriceItem />
        ) : isTokenBased ? (
          <div className='flex min-w-max items-stretch gap-2'>
            <ProviderPriceItem
              label={t('Input')}
              value={renderTokenPrice('input')}
              originalValue={
                hasDiscount ? renderOriginalTokenPrice('input') : undefined
              }
              unit={priceUnit}
              featured
            />
            <ProviderPriceItem
              label={t('Output')}
              value={renderTokenPrice('output')}
              originalValue={
                hasDiscount ? renderOriginalTokenPrice('output') : undefined
              }
              unit={priceUnit}
              featured
            />
            {props.extraPriceTypes.map((priceType) => (
              <ProviderPriceItem
                key={priceType.type}
                label={priceType.label}
                value={renderTokenPrice(priceType.type)}
                originalValue={
                  hasDiscount
                    ? renderOriginalTokenPrice(priceType.type)
                    : undefined
                }
                unit={priceUnit}
              />
            ))}
            {props.showCacheWritePrices && (
              <ProviderCacheWritePriceGroup
                renderTokenPrice={renderTokenPrice}
                renderOriginalTokenPrice={renderOriginalTokenPrice}
                hasDiscount={hasDiscount}
                unit={priceUnit}
              />
            )}
          </div>
        ) : (
          <div className='flex min-w-max'>
            <ProviderPriceItem
              label={t('Price')}
              value={renderFixedPrice()}
              originalValue={
                hasDiscount ? renderOriginalFixedPrice() : undefined
              }
              unit={priceUnit}
              featured
            />
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function GroupPricingSection(props: {
  model: PricingModel
  groupRatio: Record<string, number>
  usableGroup: PricingUsableGroupMap
  groupDisplay?: PricingGroupDisplayConfig
  priceRate: number
  usdExchangeRate: number
  tokenUnit: TokenUnit
  showRechargePrice?: boolean
  variant?: ModelDetailsVariant
}) {
  const { t } = useTranslation()
  const discountLabels = useGroupDiscountLabels()
  const showRechargePrice = props.showRechargePrice ?? false
  const isPage = props.variant === 'page'

  const availableGroups = useMemo(
    () =>
      getAvailableGroups(
        props.model,
        props.usableGroup || {},
        props.groupDisplay
      ),
    [props.model, props.usableGroup, props.groupDisplay]
  )
  const metricsQuery = useQuery({
    queryKey: ['perf-metrics', props.model.model_name],
    queryFn: () => getPerfMetrics(props.model.model_name, 24),
    staleTime: 60 * 1000,
  })
  const performanceByGroup = useMemo<Record<string, PerformanceGroup>>(() => {
    const map: Record<string, PerformanceGroup> = {}
    for (const group of metricsQuery.data?.data.groups ?? []) {
      map[group.group] = group
    }
    return map
  }, [metricsQuery.data])

  const tokenUnitLabel = props.tokenUnit === 'K' ? '1K' : '1M'

  const extraPriceTypes = useMemo(() => {
    const types: { label: string; type: PriceType }[] = []
    if (props.model.cache_ratio != null)
      types.push({ label: t('Cache Read'), type: 'cache' })
    if (props.model.image_ratio != null)
      types.push({ label: t('Image'), type: 'image' })
    if (props.model.audio_ratio != null)
      types.push({ label: t('Audio In'), type: 'audio_input' })
    if (
      props.model.audio_ratio != null &&
      props.model.audio_completion_ratio != null
    )
      types.push({ label: t('Audio Out'), type: 'audio_output' })
    return types
  }, [props.model, t])

  if (availableGroups.length === 0) {
    return (
      <section>
        <SectionTitle className={cn(isPage && 'mb-4')}>
          {t(USER_FACING_GROUP_TERMS.pricingBy)}
        </SectionTitle>
        <p className='text-muted-foreground text-sm'>
          {t(
            'This model is not available in any group, or no group pricing information is configured.'
          )}
        </p>
      </section>
    )
  }

  if (isDynamicPricingModel(props.model)) {
    const dynamicTiers = getDynamicPricingTiers(props.model)

    if (dynamicTiers.length === 0) {
      return (
        <section>
          <SectionTitle className={cn(isPage && 'mb-4')}>
            {t(USER_FACING_GROUP_TERMS.pricingBy)}
          </SectionTitle>
          <div className='rounded-lg border border-amber-200/70 bg-amber-50/70 p-3 dark:border-amber-500/20 dark:bg-amber-500/10'>
            <div className='text-sm font-medium text-amber-800 dark:text-amber-200'>
              {t('Special billing expression')}
            </div>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Group prices cannot be expanded because this expression is not a standard tiered pricing expression.'
              )}
            </p>
            <div className='mt-3'>
              <div className='text-muted-foreground mb-1 text-[10px] font-medium tracking-wider uppercase'>
                {t('Raw expression')}
              </div>
              <code className='text-muted-foreground bg-background/80 block max-h-28 overflow-auto rounded-md border px-2 py-1.5 font-mono text-xs break-all'>
                {props.model.billing_expr}
              </code>
            </div>
          </div>
        </section>
      )
    }

    return (
      <section>
        <SectionTitle className={cn(isPage && 'mb-4')}>
          {t(USER_FACING_GROUP_TERMS.pricingBy)}
        </SectionTitle>
        <div className={cn('grid gap-3', isPage && 'gap-4')}>
          {availableGroups.map((group) => {
            const ratio = props.groupRatio[group] || 1
            const discountLabel = getDiscountLabel(ratio, discountLabels)
            const groupDesc = getUsableGroupDescription(
              props.usableGroup,
              group
            )
            const priceRows = buildDynamicTierPriceDisplayRows(
              dynamicTiers,
              {
                tokenUnit: props.tokenUnit,
                showRechargePrice,
                priceRate: props.priceRate,
                usdExchangeRate: props.usdExchangeRate,
                groupRatioMultiplier: ratio,
              },
              ratio > 0 && ratio < 1
                ? {
                    tokenUnit: props.tokenUnit,
                    showRechargePrice,
                    priceRate: props.priceRate,
                    usdExchangeRate: props.usdExchangeRate,
                    groupRatioMultiplier: 1,
                  }
                : undefined
            )
            return (
              <DynamicProviderPriceCard
                key={group}
                group={group}
                groupDesc={groupDesc}
                discountLabel={discountLabel}
                rows={priceRows}
                tokenUnitLabel={tokenUnitLabel}
                performance={performanceByGroup[group]}
                performanceLoading={metricsQuery.isLoading}
                isPage={isPage}
              />
            )
          })}
        </div>
      </section>
    )
  }

  return (
    <section>
      <SectionTitle className={cn(isPage && 'mb-4')}>
        {t(USER_FACING_GROUP_TERMS.pricingBy)}
      </SectionTitle>
      <div className={cn('grid gap-3', isPage && 'gap-4')}>
        {availableGroups.map((group) => {
          const ratio = props.groupRatio[group] || 1
          const groupDesc = getUsableGroupDescription(props.usableGroup, group)
          return (
            <ProviderPriceCard
              key={group}
              model={props.model}
              group={group}
              groupDesc={groupDesc}
              discountLabel={getDiscountLabel(ratio, discountLabels)}
              ratio={ratio}
              groupRatio={props.groupRatio}
              tokenUnit={props.tokenUnit}
              tokenUnitLabel={tokenUnitLabel}
              showRechargePrice={showRechargePrice}
              priceRate={props.priceRate}
              usdExchangeRate={props.usdExchangeRate}
              extraPriceTypes={extraPriceTypes}
              showCacheWritePrices={props.model.create_cache_ratio != null}
              performance={performanceByGroup[group]}
              performanceLoading={metricsQuery.isLoading}
              isPage={isPage}
            />
          )
        })}
      </div>
    </section>
  )
}

const TAB_VALUES = ['overview', 'api'] as const
type TabValue = (typeof TAB_VALUES)[number]

const TAB_META: Record<
  TabValue,
  { icon: React.ComponentType<{ className?: string }>; labelKey: string }
> = {
  overview: { icon: Info, labelKey: 'Overview' },
  api: { icon: Code2, labelKey: 'API' },
}

export interface ModelDetailsContentProps {
  model: PricingModel
  groupRatio: Record<string, number>
  usableGroup: PricingUsableGroupMap
  groupDisplay?: PricingGroupDisplayConfig
  endpointMap: Record<string, { path?: string; method?: string }>
  autoGroups: string[]
  priceRate: number
  usdExchangeRate: number
  tokenUnit: TokenUnit
  showRechargePrice?: boolean
  variant?: ModelDetailsVariant
  onBack?: () => void
}

export function ModelDetailsContent(props: ModelDetailsContentProps) {
  const showRechargePrice = props.showRechargePrice ?? false
  const variant = props.variant ?? 'compact'
  const isPage = variant === 'page'
  const [activeTab, setActiveTab] = useState<TabValue>('overview')
  const [showStickyNav, setShowStickyNav] = useState(false)
  const [stickyTop, setStickyTop] = useState(64)
  const tabsListRef = useRef<HTMLDivElement>(null)
  const metadata = useMemo(() => inferModelMetadata(props.model), [props.model])

  useEffect(() => {
    if (!isPage || !props.onBack) {
      return
    }

    const getOffset = () => {
      const anchor = tabsListRef.current
      const styles = anchor ? getComputedStyle(anchor) : null
      const chromeHeight =
        getFloatingPublicTopChromeHeight(styles) ||
        getFloatingPublicTopChromeHeight(
          getComputedStyle(document.documentElement)
        )

      return chromeHeight
    }

    const updateStickyState = () => {
      const anchor = tabsListRef.current
      if (!anchor) return

      const offset = getOffset()
      setStickyTop(offset)
      setShowStickyNav(anchor.getBoundingClientRect().top <= offset)
    }

    updateStickyState()
    window.addEventListener('scroll', updateStickyState, { passive: true })
    window.addEventListener('resize', updateStickyState)

    return () => {
      window.removeEventListener('scroll', updateStickyState)
      window.removeEventListener('resize', updateStickyState)
    }
  }, [isPage, props.onBack])

  return (
    <>
      {isPage && props.onBack && showStickyNav && (
        <ModelDetailsStickyNav
          model={props.model}
          activeTab={activeTab}
          onTabChange={setActiveTab}
          onBack={props.onBack}
          top={stickyTop}
        />
      )}

      <div
        className={cn('@container/details space-y-4', isPage && 'space-y-8')}
      >
        <ModelHeader model={props.model} variant={variant} />

        <Tabs
          value={activeTab}
          onValueChange={(value) => setActiveTab(value as TabValue)}
          className={cn('gap-4', isPage && 'gap-6')}
        >
          <div ref={tabsListRef}>
            <ModelDetailsTabsList page={isPage} />
          </div>

          <TabsContent
            value='overview'
            className={cn('space-y-6 outline-none', isPage && 'space-y-8')}
          >
            <section
              className={cn(
                'bg-card/60 space-y-5 rounded-xl border p-4 shadow-sm',
                isPage && 'space-y-7 rounded-3xl p-5 md:p-6'
              )}
            >
              <GroupPricingSection
                model={props.model}
                groupRatio={props.groupRatio}
                usableGroup={props.usableGroup}
                groupDisplay={props.groupDisplay}
                priceRate={props.priceRate}
                usdExchangeRate={props.usdExchangeRate}
                tokenUnit={props.tokenUnit}
                showRechargePrice={showRechargePrice}
                variant={variant}
              />
            </section>

            <ModelDetailsPerformance
              model={props.model}
              usableGroup={props.usableGroup}
            />

            <ModelDetailsQuickStats metadata={metadata} />

            <ModelSignalsSection
              capabilities={metadata.capabilities}
              input={metadata.input_modalities}
              output={metadata.output_modalities}
              variant={variant}
            />

            <ModelDetailsProviderInfo model={props.model} />
          </TabsContent>

          <TabsContent value='api' className='outline-none'>
            <ModelDetailsApi
              model={props.model}
              endpointMap={props.endpointMap}
              usableGroup={props.usableGroup}
            />
          </TabsContent>
        </Tabs>
      </div>
    </>
  )
}

function ModelDetailsTabsList(props: { compact?: boolean; page?: boolean }) {
  const { t } = useTranslation()

  return (
    <TabsList
      className={cn(
        'bg-muted/60 grid w-full grid-cols-2 gap-1 rounded-lg p-1 group-data-horizontal/tabs:h-auto',
        props.page && 'rounded-2xl p-1.5',
        props.compact && 'h-9 w-auto min-w-44 rounded-full p-1'
      )}
    >
      {TAB_VALUES.map((value) => {
        const Icon = TAB_META[value].icon
        return (
          <TabsTrigger
            key={value}
            value={value}
            className={cn(
              'h-8 min-w-0 gap-1.5 rounded-md px-3 text-xs sm:text-sm',
              props.page && 'h-10 rounded-xl text-sm',
              props.compact && 'h-7 rounded-full px-2.5 text-xs'
            )}
          >
            <Icon className='size-3.5' />
            <span className='truncate'>{t(TAB_META[value].labelKey)}</span>
          </TabsTrigger>
        )
      })}
    </TabsList>
  )
}

function ModelDetailsStickyNav(props: {
  model: PricingModel
  activeTab: TabValue
  onTabChange: (value: TabValue) => void
  onBack: () => void
  top: number
}) {
  const { t } = useTranslation()
  const vendorIcon = props.model.vendor_icon
    ? getLobeIcon(props.model.vendor_icon, 18)
    : null

  return createPortal(
    <div
      className='pointer-events-none fixed inset-x-0 z-[60] px-6'
      style={{ top: props.top }}
    >
      <div className={MODEL_DETAILS_STICKY_BAR_CLASS}>
        <div className='flex min-w-0 items-center gap-1.5'>
          <Button
            variant='ghost'
            size='icon'
            onClick={props.onBack}
            className='size-8 shrink-0 rounded-full'
            aria-label={t('Back')}
          >
            <ArrowLeft />
          </Button>
          <div className={MODEL_DETAILS_STICKY_INFO_CLASS}>
            <span className='bg-background flex size-6 shrink-0 items-center justify-center rounded-full border shadow-sm'>
              {vendorIcon ?? (
                <Info className='text-muted-foreground size-3.5' />
              )}
            </span>
            <div className='min-w-0'>
              <p className='truncate font-mono text-sm leading-tight font-semibold'>
                {props.model.model_name}
              </p>
              <p className='text-muted-foreground mt-0.5 flex min-w-0 items-center gap-1 text-[11px] leading-none'>
                {props.model.vendor_name && (
                  <span className='truncate'>{props.model.vendor_name}</span>
                )}
                {props.model.vendor_name && (
                  <span className='text-muted-foreground/40'>/</span>
                )}
                <span className='shrink-0'>
                  {props.model.quota_type === QUOTA_TYPE_VALUES.TOKEN
                    ? t('Token-based')
                    : t('Per Request')}
                </span>
              </p>
            </div>
          </div>
        </div>

        <Tabs
          value={props.activeTab}
          onValueChange={(value) => props.onTabChange(value as TabValue)}
          className='shrink-0'
        >
          <ModelDetailsTabsList compact />
        </Tabs>
      </div>
    </div>,
    document.body
  )
}

// ----------------------------------------------------------------------------
// Drawer & page wrappers
// ----------------------------------------------------------------------------

export interface ModelDetailsDrawerProps extends ModelDetailsContentProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ModelDetailsDrawer(props: ModelDetailsDrawerProps) {
  const { t } = useTranslation()
  const { open, onOpenChange, ...contentProps } = props

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side='right'
        className='flex h-dvh w-full overflow-hidden p-0 sm:max-w-2xl lg:max-w-3xl xl:max-w-4xl 2xl:max-w-5xl'
      >
        <SheetHeader className='sr-only'>
          <SheetTitle>{props.model.model_name}</SheetTitle>
          <SheetDescription>{t('Model details')}</SheetDescription>
        </SheetHeader>
        <div className='flex-1 overflow-y-auto px-4 pt-11 pb-5 sm:px-6 sm:pt-12 sm:pb-6'>
          <ModelDetailsContent {...contentProps} />
        </div>
      </SheetContent>
    </Sheet>
  )
}

export function ModelDetails() {
  const { t } = useTranslation()
  const { modelId } = useParams({ from: '/pricing/$modelId/' })
  const search = useSearch({ from: '/pricing/$modelId/' })
  const navigate = useNavigate()

  const {
    models,
    groupRatio,
    usableGroup,
    groupDisplay,
    endpointMap,
    autoGroups,
    isLoading,
    priceRate,
    usdExchangeRate,
  } = usePricingData()

  const tokenUnit: TokenUnit =
    search.tokenUnit === 'K' ? 'K' : DEFAULT_TOKEN_UNIT

  const model = useMemo(() => {
    if (!models || !modelId) return null
    return (
      models.find(
        (m) =>
          m.model_name === modelId ||
          m.alias_models?.some((alias) => alias === modelId)
      ) || null
    )
  }, [models, modelId])

  const handleBack = () => {
    navigate({ to: '/pricing', search })
  }

  if (isLoading) {
    return (
      <PublicLayout showMainContainer={false}>
        <div
          className={`mx-auto w-full max-w-6xl px-6 pb-12 md:pb-16 ${PRICING_DETAILS_TOP_PADDING_CLASS}`}
        >
          <Skeleton className='mb-6 h-8 w-24 rounded-full' />
          <div className='space-y-3'>
            <Skeleton className='h-14 w-full max-w-3xl' />
            <Skeleton className='h-5 w-64' />
            <Skeleton className='h-5 w-full max-w-2xl' />
          </div>
          <div className='mt-8 grid gap-4 sm:grid-cols-3'>
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className='h-24 w-full rounded-2xl' />
            ))}
          </div>
          <div className='mt-8 space-y-4'>
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className='h-28 w-full rounded-2xl' />
            ))}
          </div>
        </div>
      </PublicLayout>
    )
  }

  if (!model) {
    return (
      <PublicLayout showMainContainer={false}>
        <div className='mx-auto flex min-h-[60vh] max-w-2xl flex-col items-center justify-center px-6 text-center'>
          <h2 className='mb-2 text-2xl font-bold tracking-tight'>
            {t('Model not found')}
          </h2>
          <p className='text-muted-foreground mb-6 text-base'>
            {t("The model you're looking for doesn't exist.")}
          </p>
          <Button onClick={handleBack} variant='outline' size='sm'>
            {t('Back to Models')}
          </Button>
        </div>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <div className='relative overflow-hidden'>
        <div
          aria-hidden='true'
          className='pointer-events-none absolute inset-0 -z-10 bg-[radial-gradient(circle_at_16%_18%,rgba(14,165,233,0.13),transparent_28%),radial-gradient(circle_at_84%_8%,rgba(16,185,129,0.11),transparent_24%),linear-gradient(to_bottom,var(--background),var(--muted)_180%,var(--background))]'
        />
        <div
          aria-hidden='true'
          className='absolute inset-0 -z-10 bg-[linear-gradient(to_right,var(--border)_1px,transparent_1px),linear-gradient(to_bottom,var(--border)_1px,transparent_1px)] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_20%,black_20%,transparent_100%)] bg-[size:4rem_4rem] opacity-[0.08]'
        />
        <div
          className={`mx-auto w-full max-w-6xl px-6 pb-14 md:pb-20 ${PRICING_DETAILS_TOP_PADDING_CLASS}`}
        >
          <Button
            variant='ghost'
            size='sm'
            onClick={handleBack}
            className='text-muted-foreground hover:text-foreground mb-6 h-auto gap-1 px-0 py-1 text-sm'
          >
            <ArrowLeft className='size-4' />
            {t('Back')}
          </Button>

          <ModelDetailsContent
            model={model}
            groupRatio={groupRatio || {}}
            usableGroup={usableGroup || {}}
            groupDisplay={groupDisplay}
            autoGroups={autoGroups || []}
            priceRate={priceRate ?? 1}
            usdExchangeRate={usdExchangeRate ?? 1}
            tokenUnit={tokenUnit}
            showRechargePrice={search.rechargePrice ?? false}
            variant='page'
            onBack={handleBack}
            endpointMap={
              (endpointMap as Record<
                string,
                { path?: string; method?: string }
              >) || {}
            }
          />
        </div>
      </div>
    </PublicLayout>
  )
}
