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
  ArrowLeft,
  ChevronDown,
  Code2,
  HeartPulse,
  Info,
  Timer,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatGroupDiscount } from '@/lib/group-discount'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'
import { useGroupDiscountLabels } from '@/hooks/use-group-discount-labels'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CopyButton } from '@/components/copy-button'
import { GroupBadge } from '@/components/group-badge'
import { PublicLayout } from '@/components/layout'
import { getPerfMetrics } from '@/features/performance-metrics/api'
import {
  formatLatency,
  formatThroughput,
  formatUptimePct,
} from '@/features/performance-metrics/lib/format'
import { DEFAULT_TOKEN_UNIT, QUOTA_TYPE_VALUES } from '../constants'
import { usePricingData } from '../hooks/use-pricing-data'
import {
  getDynamicPriceEntries,
  getDynamicPricingSummary,
  getDynamicPricingTiers,
  isDynamicPricingModel,
} from '../lib/dynamic-price'
import { parseTags } from '../lib/filters'
import { getAvailableGroups, isTokenBasedModel } from '../lib/model-helpers'
import { inferModelMetadata } from '../lib/model-metadata'
import { formatFixedPrice, formatGroupPrice } from '../lib/price'
import type {
  Modality,
  ModelCapability,
  PriceType,
  PricingModel,
  TokenUnit,
} from '../types'
import { DynamicPricingBreakdown } from './dynamic-pricing-breakdown'
import { ModelDetailsApi, ModelDetailsProviderInfo } from './model-details-api'
import { ModalityIcons } from './model-details-modalities'
import { ModelDetailsPerformance } from './model-details-performance'
import { ModelDetailsQuickStats } from './model-details-quick-stats'

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

function OverviewMetric(props: {
  icon: React.ComponentType<{ className?: string }>
  label: string
  value: React.ReactNode
  intent?: 'default' | 'warning' | 'success'
  variant?: ModelDetailsVariant
}) {
  const Icon = props.icon
  const intent = props.intent ?? 'default'
  const isPage = props.variant === 'page'

  return (
    <div
      className={cn(
        'flex min-w-0 items-center gap-2 px-3 py-2',
        isPage && 'gap-3 px-5 py-4'
      )}
    >
      <Icon
        className={cn(
          'text-muted-foreground/70 size-3.5 shrink-0',
          isPage && 'size-4'
        )}
      />
      <div className='min-w-0 flex-1'>
        <div
          className={cn(
            'text-muted-foreground truncate text-[10px] font-medium tracking-wider uppercase',
            isPage && 'text-[11px]'
          )}
        >
          {props.label}
        </div>
        <div
          className={cn(
            'text-foreground truncate font-mono text-sm font-semibold tabular-nums',
            isPage && 'mt-1 text-base md:text-lg',
            intent === 'warning' && 'text-amber-600 dark:text-amber-400',
            intent === 'success' && 'text-emerald-600 dark:text-emerald-400'
          )}
        >
          {props.value}
        </div>
      </div>
    </div>
  )
}

function OverviewSummaryGrid(props: {
  model: PricingModel
  variant?: ModelDetailsVariant
}) {
  const { t } = useTranslation()
  const isPage = props.variant === 'page'
  const metricsQuery = useQuery({
    queryKey: ['perf-metrics', props.model.model_name],
    queryFn: () => getPerfMetrics(props.model.model_name, 24),
    staleTime: 60 * 1000,
  })

  const groups = metricsQuery.data?.data.groups ?? []
  const successRates = groups
    .map((group) => group.success_rate)
    .filter((rate) => Number.isFinite(rate))
  const successRate =
    successRates.length > 0
      ? successRates.reduce((sum, rate) => sum + rate, 0) / successRates.length
      : Number.NaN
  let successIntent: 'default' | 'warning' | 'success' = 'warning'
  if (successRate >= 99.9) {
    successIntent = 'success'
  } else if (successRate >= 99) {
    successIntent = 'default'
  }
  const tpsValues = groups
    .map((group) => group.avg_tps)
    .filter((value) => value > 0)
  const avgTps =
    tpsValues.length > 0
      ? tpsValues.reduce((sum, value) => sum + value, 0) / tpsValues.length
      : 0
  const latencyValues = groups
    .map((group) => group.avg_latency_ms)
    .filter((value) => value > 0)
  const avgLatency =
    latencyValues.length > 0
      ? Math.round(
          latencyValues.reduce((sum, value) => sum + value, 0) /
            latencyValues.length
        )
      : 0

  return (
    <div
      className={cn(
        'bg-muted/20 grid overflow-hidden rounded-lg border sm:grid-cols-3 sm:divide-x',
        isPage && 'bg-background/70 rounded-2xl shadow-sm'
      )}
    >
      <OverviewMetric
        icon={Timer}
        label='TPS'
        value={formatThroughput(avgTps)}
        variant={props.variant}
      />
      <OverviewMetric
        icon={Timer}
        label={t('Average latency')}
        value={formatLatency(avgLatency)}
        variant={props.variant}
      />
      <OverviewMetric
        icon={HeartPulse}
        label={t('Success rate')}
        value={formatUptimePct(successRate)}
        intent={successIntent}
        variant={props.variant}
      />
    </div>
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
  const [descriptionExpanded, setDescriptionExpanded] = useState(false)
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

  useEffect(() => {
    setDescriptionExpanded(false)
  }, [description, model.model_name])

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
          <div
            className={cn(
              'flex items-center gap-2.5',
              isPage && 'items-baseline gap-3'
            )}
          >
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
              className={cn('size-6 self-baseline', isPage && 'size-8')}
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
              onClick={() => setDescriptionExpanded((current) => !current)}
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
// Base price card (used in the Overview tab)
// ----------------------------------------------------------------------------

function PriceSection(props: {
  model: PricingModel
  priceRate: number
  usdExchangeRate: number
  tokenUnit: TokenUnit
  showRechargePrice: boolean
  variant?: ModelDetailsVariant
}) {
  const { t } = useTranslation()
  const isPage = props.variant === 'page'
  const isTokenBased = isTokenBasedModel(props.model)
  const tokenUnitLabel = props.tokenUnit === 'K' ? '1K' : '1M'
  const baseGroupKey = '_base'
  const baseGroupRatioMap = { [baseGroupKey]: 1 }
  const dynamicSummary = getDynamicPricingSummary(props.model, {
    tokenUnit: props.tokenUnit,
    showRechargePrice: props.showRechargePrice,
    priceRate: props.priceRate,
    usdExchangeRate: props.usdExchangeRate,
    groupRatioMultiplier: 1,
  })

  const primaryPriceTypes: { label: string; type: PriceType }[] = [
    { label: t('Input'), type: 'input' },
    { label: t('Output'), type: 'output' },
  ]
  const secondaryPriceTypes: {
    label: string
    type: PriceType
    available: boolean
  }[] = [
    {
      label: t('Cached input'),
      type: 'cache',
      available: props.model.cache_ratio != null,
    },
    {
      label: t('Cache write'),
      type: 'create_cache',
      available: props.model.create_cache_ratio != null,
    },
    {
      label: t('Image input'),
      type: 'image',
      available: props.model.image_ratio != null,
    },
    {
      label: t('Audio input'),
      type: 'audio_input',
      available: props.model.audio_ratio != null,
    },
    {
      label: t('Audio output'),
      type: 'audio_output',
      available:
        props.model.audio_ratio != null &&
        props.model.audio_completion_ratio != null,
    },
  ]

  if (dynamicSummary) {
    if (dynamicSummary.isSpecialExpression) {
      return (
        <section>
          <SectionTitle className={cn(isPage && 'mb-4')}>
            {t('Base Price')}
          </SectionTitle>
          <div className='rounded-lg border border-amber-200/70 bg-amber-50/70 p-3 dark:border-amber-500/20 dark:bg-amber-500/10'>
            <div className='text-sm font-medium text-amber-800 dark:text-amber-200'>
              {t('Special billing expression')}
            </div>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t('Unable to parse structured pricing')}
            </p>
            <div className='mt-3'>
              <div className='text-muted-foreground mb-1 text-[10px] font-medium tracking-wider uppercase'>
                {t('Raw expression')}
              </div>
              <code className='text-muted-foreground bg-background/80 block max-h-28 overflow-auto rounded-md border px-2 py-1.5 font-mono text-xs break-all'>
                {dynamicSummary.rawExpression}
              </code>
            </div>
          </div>
        </section>
      )
    }

    return (
      <section>
        <SectionTitle className={cn(isPage && 'mb-4')}>
          {t('Base Price')}
        </SectionTitle>
        {dynamicSummary.primaryEntries.length > 0 ? (
          <div className={cn('grid grid-cols-2 gap-2', isPage && 'gap-4')}>
            {dynamicSummary.primaryEntries.map((entry) => (
              <div
                key={entry.key}
                className={cn(
                  'bg-muted/20 rounded-lg border p-3',
                  isPage && 'rounded-2xl p-5'
                )}
              >
                <div
                  className={cn(
                    'text-muted-foreground text-xs',
                    isPage && 'text-sm'
                  )}
                >
                  {t(entry.shortLabel)}
                </div>
                <div
                  className={cn(
                    'text-foreground mt-1 font-mono text-base font-semibold tabular-nums',
                    isPage && 'mt-2 text-xl md:text-2xl'
                  )}
                >
                  {entry.formatted}
                  <span className='text-muted-foreground/40 ml-1 text-xs font-normal'>
                    / {tokenUnitLabel}
                  </span>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <p className='text-muted-foreground text-sm'>
            {t('Dynamic Pricing')}
          </p>
        )}
        {dynamicSummary.secondaryEntries.length > 0 && (
          <div
            className={cn(
              'bg-muted/20 mt-3 rounded-lg border px-3 py-2.5',
              isPage && 'mt-4 rounded-2xl px-5 py-4'
            )}
          >
            <div className='space-y-1.5'>
              {dynamicSummary.secondaryEntries.map((entry) => (
                <div
                  key={entry.key}
                  className='flex items-baseline justify-between gap-4'
                >
                  <span className='text-muted-foreground/70 text-sm'>
                    {t(entry.shortLabel)}
                  </span>
                  <span className='text-muted-foreground font-mono text-sm tabular-nums'>
                    {entry.formatted}
                    <span className='text-muted-foreground/40 ml-1 text-xs font-normal'>
                      / {tokenUnitLabel}
                    </span>
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </section>
    )
  }

  if (!isTokenBased) {
    return (
      <section>
        <SectionTitle className={cn(isPage && 'mb-4')}>
          {t('Base Price')}
        </SectionTitle>
        <div
          className={cn(
            'flex items-baseline justify-between',
            isPage && 'bg-muted/20 rounded-2xl border p-5'
          )}
        >
          <span
            className={cn(
              'text-muted-foreground text-sm',
              isPage && 'text-base'
            )}
          >
            {t('Per request')}
          </span>
          <span
            className={cn(
              'text-foreground font-mono text-sm font-semibold tabular-nums',
              isPage && 'text-xl md:text-2xl'
            )}
          >
            {formatFixedPrice(
              props.model,
              baseGroupKey,
              props.showRechargePrice,
              props.priceRate,
              props.usdExchangeRate,
              baseGroupRatioMap
            )}
          </span>
        </div>
      </section>
    )
  }

  const secondaryItems = secondaryPriceTypes.filter((p) => p.available)
  const renderPrice = (type: PriceType) => (
    <>
      {formatGroupPrice(
        props.model,
        baseGroupKey,
        type,
        props.tokenUnit,
        props.showRechargePrice,
        props.priceRate,
        props.usdExchangeRate,
        baseGroupRatioMap
      )}
      <span className='text-muted-foreground/40 ml-1 text-xs font-normal'>
        / {tokenUnitLabel}
      </span>
    </>
  )

  return (
    <section>
      <SectionTitle className={cn(isPage && 'mb-4')}>
        {t('Base Price')}
      </SectionTitle>
      <div className={cn('grid grid-cols-2 gap-2', isPage && 'gap-4')}>
        {primaryPriceTypes.map((item) => (
          <div
            key={item.type}
            className={cn(
              'bg-muted/20 rounded-lg border p-3',
              isPage && 'rounded-2xl p-5'
            )}
          >
            <div
              className={cn(
                'text-muted-foreground text-xs',
                isPage && 'text-sm'
              )}
            >
              {item.label}
            </div>
            <div
              className={cn(
                'text-foreground mt-1 font-mono text-base font-semibold tabular-nums',
                isPage && 'mt-2 text-xl md:text-2xl'
              )}
            >
              {renderPrice(item.type)}
            </div>
          </div>
        ))}
      </div>
      {secondaryItems.length > 0 && (
        <div
          className={cn(
            'bg-muted/20 mt-3 rounded-lg border px-3 py-2.5',
            isPage && 'mt-4 rounded-2xl px-5 py-4'
          )}
        >
          <div className={cn('space-y-1.5', isPage && 'space-y-2')}>
            {secondaryItems.map((item) => (
              <div
                key={item.type}
                className='flex items-baseline justify-between gap-4'
              >
                <span className='text-muted-foreground/70 text-sm'>
                  {item.label}
                </span>
                <span
                  className={cn(
                    'text-muted-foreground font-mono text-sm tabular-nums',
                    isPage && 'text-base'
                  )}
                >
                  {renderPrice(item.type)}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </section>
  )
}

// ----------------------------------------------------------------------------
// Auto group chain (used inside group pricing section)
// ----------------------------------------------------------------------------

function AutoGroupChain(props: { model: PricingModel; autoGroups: string[] }) {
  const { t } = useTranslation()
  const modelEnableGroups = Array.isArray(props.model.enable_groups)
    ? props.model.enable_groups
    : []
  const autoChain = props.autoGroups.filter((g) =>
    modelEnableGroups.includes(g)
  )

  if (autoChain.length === 0) return null

  return (
    <div className='text-muted-foreground mb-3 flex flex-wrap items-center gap-1 text-xs'>
      <span className='font-medium'>{t('Auto Group Chain')}</span>
      <span className='text-muted-foreground/40'>→</span>
      {autoChain.map((g, idx) => (
        <span key={g} className='flex items-center gap-1'>
          <GroupBadge group={g} size='sm' />
          {idx < autoChain.length - 1 && (
            <span className='text-muted-foreground/40'>→</span>
          )}
        </span>
      ))}
    </div>
  )
}

// ----------------------------------------------------------------------------
// Group pricing table
// ----------------------------------------------------------------------------

function GroupPricingSection(props: {
  model: PricingModel
  groupRatio: Record<string, number>
  usableGroup: Record<string, { desc: string; ratio: number }>
  autoGroups: string[]
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
    () => getAvailableGroups(props.model, props.usableGroup || {}),
    [props.model, props.usableGroup]
  )

  const isTokenBased = isTokenBasedModel(props.model)
  const tokenUnitLabel = props.tokenUnit === 'K' ? '1K' : '1M'

  const extraPriceTypes = useMemo(() => {
    const types: { label: string; type: PriceType }[] = []
    if (props.model.cache_ratio != null)
      types.push({ label: t('Cache'), type: 'cache' })
    if (props.model.create_cache_ratio != null)
      types.push({ label: t('Cache Write'), type: 'create_cache' })
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
          {t('Pricing by Group')}
        </SectionTitle>
        <AutoGroupChain model={props.model} autoGroups={props.autoGroups} />
        <p className='text-muted-foreground text-sm'>
          {t(
            'This model is not available in any group, or no group pricing information is configured.'
          )}
        </p>
      </section>
    )
  }

  const thClass =
    'text-muted-foreground py-2 text-[10px] font-medium tracking-wider uppercase'

  if (isDynamicPricingModel(props.model)) {
    const dynamicTiers = getDynamicPricingTiers(props.model)

    if (dynamicTiers.length === 0) {
      return (
        <section>
          <SectionTitle className={cn(isPage && 'mb-4')}>
            {t('Pricing by Group')}
          </SectionTitle>
          <AutoGroupChain model={props.model} autoGroups={props.autoGroups} />
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

    const priceFields = Array.from(
      new Map(
        dynamicTiers
          .flatMap((tier) =>
            getDynamicPriceEntries(tier, {
              tokenUnit: props.tokenUnit,
              showRechargePrice,
              priceRate: props.priceRate,
              usdExchangeRate: props.usdExchangeRate,
              groupRatioMultiplier: 1,
            })
          )
          .map((entry) => [entry.field, entry])
      ).values()
    )

    return (
      <section>
        <SectionTitle className={cn(isPage && 'mb-4')}>
          {t('Pricing by Group')}
        </SectionTitle>
        <AutoGroupChain model={props.model} autoGroups={props.autoGroups} />
        <div className={cn('space-y-3', isPage && 'space-y-4')}>
          {availableGroups.map((group) => {
            const ratio = props.groupRatio[group] || 1
            return (
              <div
                key={group}
                className={cn(
                  'overflow-hidden rounded-lg border',
                  isPage && 'rounded-2xl'
                )}
              >
                <div
                  className={cn(
                    'bg-muted/20 flex items-center justify-between gap-3 border-b px-3 py-2',
                    isPage && 'px-5 py-3'
                  )}
                >
                  <GroupBadge group={group} size='sm' />
                  <span className='text-muted-foreground text-xs'>
                    {formatGroupDiscount(ratio, discountLabels)}
                  </span>
                </div>
                <div className='overflow-x-auto'>
                  <Table className='text-sm'>
                    <TableHeader>
                      <TableRow className='hover:bg-transparent'>
                        <TableHead className={thClass}>{t('Tier')}</TableHead>
                        {priceFields.map((entry) => (
                          <TableHead
                            key={entry.field}
                            className={`${thClass} text-right`}
                          >
                            {t(entry.shortLabel)}
                          </TableHead>
                        ))}
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {dynamicTiers.map((tier, tierIndex) => {
                        const entries = getDynamicPriceEntries(tier, {
                          tokenUnit: props.tokenUnit,
                          showRechargePrice,
                          priceRate: props.priceRate,
                          usdExchangeRate: props.usdExchangeRate,
                          groupRatioMultiplier: ratio,
                        })
                        const entryMap = new Map(
                          entries.map((entry) => [entry.field, entry])
                        )

                        return (
                          <TableRow key={`${group}-${tier.label || tierIndex}`}>
                            <TableCell className='text-muted-foreground py-2.5 text-xs'>
                              {tier.label || t('Default')}
                            </TableCell>
                            {priceFields.map((fieldEntry) => {
                              const entry = entryMap.get(fieldEntry.field)
                              return (
                                <TableCell
                                  key={fieldEntry.field}
                                  className='py-2.5 text-right font-mono'
                                >
                                  {entry?.formatted ?? '-'}
                                </TableCell>
                              )
                            })}
                          </TableRow>
                        )
                      })}
                    </TableBody>
                  </Table>
                </div>
              </div>
            )
          })}
          <p className='text-muted-foreground/40 mt-1.5 text-[10px]'>
            {t('Prices shown per')} {tokenUnitLabel} tokens
          </p>
        </div>
      </section>
    )
  }

  return (
    <section>
      <SectionTitle className={cn(isPage && 'mb-4')}>
        {t('Pricing by Group')}
      </SectionTitle>
      <AutoGroupChain model={props.model} autoGroups={props.autoGroups} />
      <div
        className={cn(
          '-mx-4 overflow-x-auto sm:mx-0',
          isPage && 'bg-muted/10 rounded-2xl border sm:mx-0'
        )}
      >
        <Table className='text-sm'>
          <TableHeader>
            <TableRow className='hover:bg-transparent'>
              <TableHead className={thClass}>{t('Group')}</TableHead>
              <TableHead className={thClass}>{t('Discount')}</TableHead>
              {isTokenBased ? (
                <>
                  <TableHead className={`${thClass} text-right`}>
                    {t('Input')}
                  </TableHead>
                  <TableHead className={`${thClass} text-right`}>
                    {t('Output')}
                  </TableHead>
                  {extraPriceTypes.map((ep) => (
                    <TableHead
                      key={ep.type}
                      className={`${thClass} text-right`}
                    >
                      {ep.label}
                    </TableHead>
                  ))}
                </>
              ) : (
                <TableHead className={`${thClass} text-right`}>
                  {t('Price')}
                </TableHead>
              )}
            </TableRow>
          </TableHeader>
          <TableBody>
            {availableGroups.map((group) => {
              const ratio = props.groupRatio[group] || 1
              return (
                <TableRow key={group}>
                  <TableCell className='py-2.5'>
                    <GroupBadge group={group} size='sm' />
                  </TableCell>
                  <TableCell className='text-muted-foreground py-2.5 text-xs'>
                    {formatGroupDiscount(ratio, discountLabels)}
                  </TableCell>
                  {isTokenBased ? (
                    <>
                      <TableCell className='py-2.5 text-right font-mono'>
                        {formatGroupPrice(
                          props.model,
                          group,
                          'input',
                          props.tokenUnit,
                          showRechargePrice,
                          props.priceRate,
                          props.usdExchangeRate,
                          props.groupRatio
                        )}
                      </TableCell>
                      <TableCell className='py-2.5 text-right font-mono'>
                        {formatGroupPrice(
                          props.model,
                          group,
                          'output',
                          props.tokenUnit,
                          showRechargePrice,
                          props.priceRate,
                          props.usdExchangeRate,
                          props.groupRatio
                        )}
                      </TableCell>
                      {extraPriceTypes.map((ep) => (
                        <TableCell
                          key={ep.type}
                          className='py-2.5 text-right font-mono'
                        >
                          {formatGroupPrice(
                            props.model,
                            group,
                            ep.type,
                            props.tokenUnit,
                            showRechargePrice,
                            props.priceRate,
                            props.usdExchangeRate,
                            props.groupRatio
                          )}
                        </TableCell>
                      ))}
                    </>
                  ) : (
                    <TableCell className='py-2.5 text-right font-mono'>
                      {formatFixedPrice(
                        props.model,
                        group,
                        showRechargePrice,
                        props.priceRate,
                        props.usdExchangeRate,
                        props.groupRatio
                      )}
                    </TableCell>
                  )}
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
        {isTokenBased && (
          <p className='text-muted-foreground/40 mt-1.5 px-4 text-[10px] sm:px-0'>
            {t('Prices shown per')} {tokenUnitLabel} tokens
          </p>
        )}
      </div>
    </section>
  )
}

const TAB_VALUES = ['overview', 'performance', 'api'] as const
type TabValue = (typeof TAB_VALUES)[number]

const TAB_META: Record<
  TabValue,
  { icon: React.ComponentType<{ className?: string }>; labelKey: string }
> = {
  overview: { icon: Info, labelKey: 'Overview' },
  performance: { icon: HeartPulse, labelKey: 'Performance' },
  api: { icon: Code2, labelKey: 'API' },
}

export interface ModelDetailsContentProps {
  model: PricingModel
  groupRatio: Record<string, number>
  usableGroup: Record<string, { desc: string; ratio: number }>
  endpointMap: Record<string, { path?: string; method?: string }>
  autoGroups: string[]
  priceRate: number
  usdExchangeRate: number
  tokenUnit: TokenUnit
  showRechargePrice?: boolean
  variant?: ModelDetailsVariant
}

export function ModelDetailsContent(props: ModelDetailsContentProps) {
  const { t } = useTranslation()
  const showRechargePrice = props.showRechargePrice ?? false
  const variant = props.variant ?? 'compact'
  const isPage = variant === 'page'
  const metadata = useMemo(() => inferModelMetadata(props.model), [props.model])

  const isDynamic =
    props.model.billing_mode === 'tiered_expr' &&
    Boolean(props.model.billing_expr)

  return (
    <div className={cn('@container/details space-y-4', isPage && 'space-y-8')}>
      <ModelHeader model={props.model} variant={variant} />

      <Tabs defaultValue='overview' className={cn('gap-4', isPage && 'gap-6')}>
        <TabsList
          className={cn(
            'bg-muted/60 grid w-full grid-cols-3 gap-1 rounded-lg p-1 group-data-horizontal/tabs:h-auto',
            isPage && 'rounded-2xl p-1.5'
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
                  isPage && 'h-10 rounded-xl text-sm'
                )}
              >
                <Icon className='size-3.5' />
                <span className='truncate'>{t(TAB_META[value].labelKey)}</span>
              </TabsTrigger>
            )
          })}
        </TabsList>

        <TabsContent
          value='overview'
          className={cn('space-y-6 outline-none', isPage && 'space-y-8')}
        >
          <OverviewSummaryGrid model={props.model} variant={variant} />

          <section
            className={cn(
              'bg-card/60 space-y-5 rounded-xl border p-4 shadow-sm',
              isPage && 'space-y-7 rounded-3xl p-5 md:p-6'
            )}
          >
            <SectionTitle className={cn(isPage && 'mb-4')}>
              {t('Pricing')}
            </SectionTitle>
            <PriceSection
              model={props.model}
              priceRate={props.priceRate}
              usdExchangeRate={props.usdExchangeRate}
              tokenUnit={props.tokenUnit}
              showRechargePrice={showRechargePrice}
              variant={variant}
            />
            {isDynamic && (
              <DynamicPricingBreakdown billingExpr={props.model.billing_expr} />
            )}
            <GroupPricingSection
              model={props.model}
              groupRatio={props.groupRatio}
              usableGroup={props.usableGroup}
              autoGroups={props.autoGroups}
              priceRate={props.priceRate}
              usdExchangeRate={props.usdExchangeRate}
              tokenUnit={props.tokenUnit}
              showRechargePrice={showRechargePrice}
              variant={variant}
            />
          </section>

          <ModelDetailsQuickStats metadata={metadata} />

          <ModelSignalsSection
            capabilities={metadata.capabilities}
            input={metadata.input_modalities}
            output={metadata.output_modalities}
            variant={variant}
          />

          <ModelDetailsProviderInfo model={props.model} />
        </TabsContent>

        <TabsContent value='performance' className='outline-none'>
          <ModelDetailsPerformance model={props.model} />
        </TabsContent>

        <TabsContent value='api' className='outline-none'>
          <ModelDetailsApi
            model={props.model}
            endpointMap={props.endpointMap}
          />
        </TabsContent>
      </Tabs>
    </div>
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
        <div className='mx-auto w-full max-w-6xl px-6 pt-24 pb-12 md:pt-32 md:pb-16'>
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
        <div className='mx-auto w-full max-w-6xl px-6 pt-24 pb-14 md:pt-32 md:pb-20'>
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
            autoGroups={autoGroups || []}
            priceRate={priceRate ?? 1}
            usdExchangeRate={usdExchangeRate ?? 1}
            tokenUnit={tokenUnit}
            showRechargePrice={search.rechargePrice ?? false}
            variant='page'
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
