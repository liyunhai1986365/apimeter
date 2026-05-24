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
import { RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  ENDPOINT_TYPES,
  FILTER_ALL,
  MODEL_CATEGORIES,
  QUOTA_TYPES,
  getEndpointTypeLabels,
  getModelCategoryLabels,
  getQuotaTypeLabels,
} from '../constants'
import type { PricingModel, PricingVendor } from '../types'

type FilterOption = {
  value: string
  label: string
  count?: number
  icon?: ReactNode
}

type FilterGroupProps = {
  title: string
  value: string
  options: FilterOption[]
  onChange: (value: string) => void
}

export interface PricingFilterBarProps {
  quotaTypeFilter: string
  endpointTypeFilter: string
  categoryFilter: string
  vendorFilter: string
  onQuotaTypeChange: (value: string) => void
  onEndpointTypeChange: (value: string) => void
  onCategoryChange: (value: string) => void
  onVendorChange: (value: string) => void
  vendors: PricingVendor[]
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
        'inline-flex h-8 max-w-full cursor-pointer items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition-colors',
        'focus-visible:ring-ring/50 focus-visible:ring-2 focus-visible:outline-none',
        props.active
          ? 'border-primary/40 bg-primary text-primary-foreground shadow-sm'
          : 'border-border/70 bg-background text-muted-foreground hover:border-border hover:bg-muted/60 hover:text-foreground'
      )}
      title={props.option.label}
    >
      {props.option.icon && (
        <span className='flex shrink-0 items-center'>{props.option.icon}</span>
      )}
      <span className='truncate'>{props.option.label}</span>
      {props.option.count != null && (
        <span
          className={cn(
            'rounded-md px-1.5 py-0.5 text-[10px] tabular-nums',
            props.active
              ? 'bg-primary-foreground/20 text-primary-foreground'
              : 'bg-muted text-muted-foreground'
          )}
        >
          {props.option.count}
        </span>
      )}
    </button>
  )
}

function FilterGroup(props: FilterGroupProps) {
  return (
    <section className='border-border/70 grid min-w-0 gap-2 border-b pb-3 last:border-b-0 last:pb-0 sm:grid-cols-[7rem_minmax(0,1fr)] sm:items-start'>
      <div className='text-muted-foreground pt-1.5 text-xs font-semibold'>
        {props.title}
      </div>
      <div className='flex flex-wrap gap-1.5'>
        {props.options.map((option) => (
          <FilterChip
            key={option.value}
            option={option}
            active={props.value === option.value}
            onClick={() => props.onChange(option.value)}
          />
        ))}
      </div>
    </section>
  )
}

export function PricingFilterBar(props: PricingFilterBarProps) {
  const { t } = useTranslation()
  const quotaTypeLabels = getQuotaTypeLabels(t)
  const endpointTypeLabels = getEndpointTypeLabels(t)
  const categoryLabels = getModelCategoryLabels(t)

  const categoryOptions: FilterOption[] = [
    {
      value: MODEL_CATEGORIES.ALL,
      label: categoryLabels[MODEL_CATEGORIES.ALL],
      count: props.models.length,
    },
    ...Object.entries(categoryLabels)
      .filter(([value]) => value !== MODEL_CATEGORIES.ALL)
      .map(([value, label]) => ({
        value,
        label,
        count: countBy(
          props.models,
          (model) => (model.category || MODEL_CATEGORIES.TEXT) === value
        ),
      })),
  ]

  const vendorOptions: FilterOption[] = [
    {
      value: FILTER_ALL,
      label: t('All Vendors'),
      count: props.models.length,
    },
    ...props.vendors
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

  return (
    <aside
      className={cn(
        'border-primary/15 bg-primary/5 rounded-xl border p-3 shadow-sm',
        props.className
      )}
    >
      <div className='mb-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
        <div className='flex min-w-0 items-center gap-2'>
          <span className='text-foreground text-sm font-semibold'>
            {t('Filter')}
          </span>
          {props.activeFilterCount > 0 && (
            <Badge variant='secondary'>{props.activeFilterCount}</Badge>
          )}
        </div>
        <Button
          type='button'
          variant='ghost'
          size='sm'
          onClick={props.onClearFilters}
          disabled={!props.hasActiveFilters}
          className='h-7 w-fit gap-1.5 px-2 text-xs'
        >
          <RotateCcw className='size-3.5' />
          {t('Reset')}
        </Button>
      </div>

      <div className='space-y-3'>
        <FilterGroup
          title={t('Model Category')}
          value={props.categoryFilter}
          options={categoryOptions}
          onChange={props.onCategoryChange}
        />
        <FilterGroup
          title={t('Provider')}
          value={props.vendorFilter}
          options={vendorOptions}
          onChange={props.onVendorChange}
        />
        <FilterGroup
          title={t('Pricing Type')}
          value={props.quotaTypeFilter}
          options={quotaOptions}
          onChange={props.onQuotaTypeChange}
        />
        <FilterGroup
          title={t('Endpoint Type')}
          value={props.endpointTypeFilter}
          options={endpointOptions}
          onChange={props.onEndpointTypeChange}
        />
      </div>
    </aside>
  )
}
