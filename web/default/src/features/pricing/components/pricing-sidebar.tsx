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
import type { ReactNode } from 'react'
import { ChevronDown, ListFilter, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatGroupDiscount } from '@/lib/group-discount'
import { getLobeIcon } from '@/lib/lobe-icon'
import { USER_FACING_GROUP_TERMS } from '@/lib/user-facing-group-terms'
import { cn } from '@/lib/utils'
import { useGroupDiscountLabels } from '@/hooks/use-group-discount-labels'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  ENDPOINT_TYPES,
  FILTER_ALL,
  MODALITY_TYPES,
  QUOTA_TYPES,
  getEndpointTypeLabels,
  getModalityTypeLabels,
  getQuotaTypeLabels,
} from '../constants'
import { inferModelMetadata } from '../lib/model-metadata'
import { sortVendorsByConfiguredOrder } from '../lib/vendor-order'
import type { PricingModel, PricingVendor } from '../types'
import {
  PRICING_SUPPLIER_FILTER_DEFAULT_OPEN,
  PRICING_VENDOR_FILTER_DEFAULT_OPEN,
  shouldFilterSectionDefaultOpen,
} from './pricing-sidebar-state'

type FilterOption = {
  value: string
  label: string
  count?: number
  suffix?: string
  icon?: ReactNode
}

type FilterSectionProps = {
  title: string
  value: string
  options: FilterOption[]
  onChange: (value: string) => void
  defaultOpen?: boolean
}

export interface PricingSidebarProps {
  quotaTypeFilter: string
  endpointTypeFilter: string
  vendorFilter: string
  groupFilter: string
  inputModalityFilter: string
  outputModalityFilter: string
  onQuotaTypeChange: (value: string) => void
  onEndpointTypeChange: (value: string) => void
  onVendorChange: (value: string) => void
  onGroupChange: (value: string) => void
  onInputModalityChange: (value: string) => void
  onOutputModalityChange: (value: string) => void
  vendors: PricingVendor[]
  groups: string[]
  groupRatios?: Record<string, number>
  models: PricingModel[]
  hasActiveFilters: boolean
  activeFilterCount: number
  onClearFilters: () => void
  className?: string
}

function countBy(
  models: PricingModel[],
  predicate: (model: PricingModel) => boolean
): number {
  return models.reduce((count, model) => count + (predicate(model) ? 1 : 0), 0)
}

function FilterChip(props: {
  option: FilterOption
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      type='button'
      onClick={props.onClick}
      aria-pressed={props.active}
      className={cn(
        'grid h-7 w-full cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-center gap-2 rounded-md px-2 text-left text-xs transition-colors',
        'focus-visible:ring-ring/50 focus-visible:ring-2 focus-visible:outline-none',
        props.active
          ? 'bg-muted text-foreground ring-border/80 ring-1'
          : 'text-muted-foreground hover:bg-muted/70 hover:text-foreground'
      )}
      title={props.option.label}
    >
      <span className='flex min-w-0 items-center gap-1.5'>
        {props.option.icon && (
          <span className='flex shrink-0 items-center'>
            {props.option.icon}
          </span>
        )}
        <span className='truncate'>{props.option.label}</span>
      </span>
      {props.option.count != null && (
        <span
          className={cn(
            'rounded px-1.5 py-0.5 text-[10px] tabular-nums',
            props.active
              ? 'bg-background text-muted-foreground'
              : 'bg-muted text-muted-foreground'
          )}
        >
          {props.option.count}
        </span>
      )}
      {props.option.count == null && props.option.suffix && (
        <span className='bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[10px]'>
          {props.option.suffix}
        </span>
      )}
    </button>
  )
}

function FilterSection(props: FilterSectionProps) {
  return (
    <Collapsible
      defaultOpen={shouldFilterSectionDefaultOpen(props.defaultOpen)}
      className='border-border/70 border-b pb-2.5 last:border-b-0 last:pb-0'
    >
      <CollapsibleTrigger className='group flex w-full items-center justify-between py-2 text-left'>
        <span className='text-muted-foreground text-[11px] font-semibold tracking-wide uppercase'>
          {props.title}
        </span>
        <ChevronDown className='text-muted-foreground size-4 transition-transform group-data-[panel-open]:rotate-180' />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className='flex flex-col gap-1'>
          {props.options.map((option) => (
            <FilterChip
              key={option.value}
              option={option}
              active={props.value === option.value}
              onClick={() => props.onChange(option.value)}
            />
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

function buildModalityOptions(
  models: PricingModel[],
  labels: Record<string, string>,
  direction: 'input' | 'output'
): FilterOption[] {
  return [
    {
      value: MODALITY_TYPES.ALL,
      label: labels[MODALITY_TYPES.ALL],
      count: models.length,
    },
    ...Object.values(MODALITY_TYPES)
      .filter((value) => value !== MODALITY_TYPES.ALL)
      .map((value) => ({
        value,
        label: labels[value],
        count: countBy(models, (model) => {
          const metadata = inferModelMetadata(model)
          const modalities =
            direction === 'input'
              ? metadata.input_modalities
              : metadata.output_modalities
          return modalities.includes(value)
        }),
      })),
  ]
}

export function PricingSidebar(props: PricingSidebarProps) {
  const { t } = useTranslation()
  const discountLabels = useGroupDiscountLabels()
  const quotaTypeLabels = getQuotaTypeLabels(t)
  const endpointTypeLabels = getEndpointTypeLabels(t)
  const modalityLabels = getModalityTypeLabels(t)

  const vendorOptions: FilterOption[] = [
    {
      value: FILTER_ALL,
      label: t('All Model Square Vendors'),
      count: props.models.length,
    },
    ...sortVendorsByConfiguredOrder(props.vendors)
      .map((vendor) => ({
        value: vendor.name,
        label: vendor.name,
        count: countBy(
          props.models,
          (model) => model.vendor_name === vendor.name
        ),
        icon: vendor.icon ? getLobeIcon(vendor.icon, 14) : undefined,
      }))
      .filter((vendor) => vendor.count > 0),
  ]

  const quotaOptions: FilterOption[] = [
    {
      value: QUOTA_TYPES.ALL,
      label: quotaTypeLabels[QUOTA_TYPES.ALL],
      count: props.models.length,
    },
    {
      value: QUOTA_TYPES.TOKEN,
      label: quotaTypeLabels[QUOTA_TYPES.TOKEN],
      count: countBy(props.models, (model) => model.quota_type === 0),
    },
    {
      value: QUOTA_TYPES.REQUEST,
      label: quotaTypeLabels[QUOTA_TYPES.REQUEST],
      count: countBy(props.models, (model) => model.quota_type === 1),
    },
  ]

  const groupOptions: FilterOption[] = [
    {
      value: FILTER_ALL,
      label: t(USER_FACING_GROUP_TERMS.all),
      count: props.models.length,
    },
    ...props.groups.map((group) => ({
      value: group,
      label: group,
      count: countBy(props.models, (model) =>
        model.enable_groups?.includes(group)
      ),
      suffix: formatGroupDiscount(props.groupRatios?.[group], discountLabels),
    })),
  ].filter((option) => option.value === FILTER_ALL || (option.count ?? 0) > 0)

  const inputModalityOptions = buildModalityOptions(
    props.models,
    modalityLabels,
    'input'
  )
  const outputModalityOptions = buildModalityOptions(
    props.models,
    modalityLabels,
    'output'
  )

  const endpointOptions: FilterOption[] = [
    {
      value: ENDPOINT_TYPES.ALL,
      label: endpointTypeLabels[ENDPOINT_TYPES.ALL],
      count: props.models.length,
    },
    ...Object.entries(endpointTypeLabels)
      .filter(([value]) => value !== ENDPOINT_TYPES.ALL)
      .map(([value, label]) => ({
        value,
        label,
        count: countBy(
          props.models,
          (model) => model.supported_endpoint_types?.includes(value) ?? false
        ),
      })),
  ]

  const content = (
    <>
      <div className='bg-background/95 border-border/70 sticky top-0 z-20 -mx-2 mb-2 hidden items-center justify-between gap-2 rounded-t-lg border-b px-2 py-2 backdrop-blur lg:flex'>
        <div className='flex min-w-0 items-center gap-2'>
          <span className='text-foreground text-xs font-semibold'>
            {t('Filter')}
          </span>
          {props.activeFilterCount > 0 && (
            <Badge variant='secondary' className='h-5 px-1.5 text-[10px]'>
              {props.activeFilterCount}
            </Badge>
          )}
        </div>
        <Button
          type='button'
          variant='ghost'
          size='sm'
          onClick={props.onClearFilters}
          disabled={!props.hasActiveFilters}
          className='h-6 gap-1 px-1.5 text-[11px]'
        >
          <RotateCcw className='size-3.5' />
          {t('Reset')}
        </Button>
      </div>

      <div className='flex flex-col gap-2'>
        <FilterSection
          title={t('Model Square Vendor')}
          value={props.vendorFilter}
          options={vendorOptions}
          onChange={props.onVendorChange}
          defaultOpen={PRICING_VENDOR_FILTER_DEFAULT_OPEN}
        />
        <FilterSection
          title={t('Pricing Type')}
          value={props.quotaTypeFilter}
          options={quotaOptions}
          onChange={props.onQuotaTypeChange}
        />
        <FilterSection
          title={t(USER_FACING_GROUP_TERMS.plural)}
          value={props.groupFilter}
          options={groupOptions}
          onChange={props.onGroupChange}
          defaultOpen={PRICING_SUPPLIER_FILTER_DEFAULT_OPEN}
        />
        <FilterSection
          title={t('Input Modality')}
          value={props.inputModalityFilter}
          options={inputModalityOptions}
          onChange={props.onInputModalityChange}
        />
        <FilterSection
          title={t('Output Modality')}
          value={props.outputModalityFilter}
          options={outputModalityOptions}
          onChange={props.onOutputModalityChange}
        />
        <FilterSection
          title={t('Endpoint Type')}
          value={props.endpointTypeFilter}
          options={endpointOptions}
          onChange={props.onEndpointTypeChange}
        />
      </div>
    </>
  )

  return (
    <aside
      className={cn(
        'bg-background/80 rounded-lg border px-2 pb-2 shadow-sm backdrop-blur',
        props.className
      )}
    >
      <Collapsible defaultOpen={false} className='lg:hidden'>
        <div className='flex items-center justify-between gap-2 py-2'>
          <CollapsibleTrigger
            render={
              <Button
                type='button'
                variant='ghost'
                size='sm'
                className='group h-7 gap-1.5 px-1.5 text-xs'
              />
            }
          >
            <ListFilter className='size-3.5' />
            {t('Filter')}
            {props.activeFilterCount > 0 && (
              <Badge variant='secondary' className='h-5 px-1.5 text-[10px]'>
                {props.activeFilterCount}
              </Badge>
            )}
            <ChevronDown className='size-3.5 transition-transform group-data-[panel-open]:rotate-180' />
          </CollapsibleTrigger>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={props.onClearFilters}
            disabled={!props.hasActiveFilters}
            className='h-7 gap-1 px-1.5 text-[11px]'
          >
            <RotateCcw className='size-3.5' />
            {t('Reset')}
          </Button>
        </div>
        <CollapsibleContent className='border-border/70 border-t pt-2'>
          {content}
        </CollapsibleContent>
      </Collapsible>

      <div className='hidden lg:block'>{content}</div>
    </aside>
  )
}
