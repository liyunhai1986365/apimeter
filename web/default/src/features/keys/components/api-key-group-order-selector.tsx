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
import { useMemo, useState } from 'react'
import {
  ArrowDown,
  ArrowUp,
  Check,
  ChevronsUpDown,
  Gauge,
  Percent,
  Plus,
  Search,
  SlidersHorizontal,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { USER_FACING_GROUP_TERMS } from '@/lib/user-facing-group-terms'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { ScrollArea } from '@/components/ui/scroll-area'
import { GroupPerformanceInline } from '@/features/performance-metrics/components/group-performance-inline'
import type { PerfGroupSummary } from '@/features/performance-metrics/types'
import type {
  PricingGroupDisplayConfig,
  PricingModel,
  PricingVendor,
} from '@/features/pricing/types'
import { addGroupsToChain, removeGroupFromChain } from '../lib/api-key-form'
import {
  ALL_CATEGORY_VALUE,
  ALL_VENDOR_VALUE,
  type ApiKeyGroupSort,
  buildApiKeyGroupFilterMetadata,
  buildPricingGroupUrl,
  filterApiKeyGroupOptions,
  sortApiKeyGroupOptions,
} from '../lib/api-key-group-filters'
import { AUTO_GROUP_VALUE } from '../lib/api-key-groups'
import {
  GroupDescription,
  GroupRatioBadge,
  type ApiKeyGroupOption,
} from './api-key-group-combobox'

type ApiKeyGroupOrderSelectorProps = {
  options: ApiKeyGroupOption[]
  value: string[]
  onChange: (value: string[]) => void
  disabled?: boolean
  groupDisplay?: PricingGroupDisplayConfig
  vendors?: PricingVendor[]
  models?: PricingModel[]
  groupPerformance?: Record<string, PerfGroupSummary>
}

type ApiKeyGroupPickerPopoverProps = {
  options: ApiKeyGroupOption[]
  selectedValues: string[]
  onApply: (values: string[]) => void
  disabled?: boolean
  groupDisplay?: PricingGroupDisplayConfig
  vendors?: PricingVendor[]
  models?: PricingModel[]
  groupPerformance?: Record<string, PerfGroupSummary>
  triggerLabel: string
  selectedCountLabel?: string
  applyLabel?: string
}

function normalizeChain(value: string[]) {
  const seen = new Set<string>()
  const result: string[] = []
  for (const item of value) {
    const group = item.trim()
    if (!group || seen.has(group)) continue
    seen.add(group)
    result.push(group)
  }
  return result
}

type SortButtonConfig = {
  key: 'discount' | 'latency' | 'name'
  label: string
  icon: typeof Percent
  asc: ApiKeyGroupSort
  desc: ApiKeyGroupSort
  defaultMode: ApiKeyGroupSort
}

const SORT_BUTTONS: SortButtonConfig[] = [
  {
    key: 'discount',
    label: 'Discount',
    icon: Percent,
    asc: 'discount_asc',
    desc: 'discount_desc',
    defaultMode: 'discount_desc',
  },
  {
    key: 'latency',
    label: 'Latency',
    icon: Gauge,
    asc: 'latency_asc',
    desc: 'latency_desc',
    defaultMode: 'latency_asc',
  },
  {
    key: 'name',
    label: 'Name',
    icon: SlidersHorizontal,
    asc: 'name_asc',
    desc: 'name_desc',
    defaultMode: 'name_asc',
  },
]

export function ApiKeyGroupOrderSelector({
  options,
  value,
  onChange,
  disabled,
  groupDisplay,
  vendors,
  models,
  groupPerformance,
}: ApiKeyGroupOrderSelectorProps) {
  const { t } = useTranslation()
  const groupTerms = USER_FACING_GROUP_TERMS
  const selected = normalizeChain(value)
  const optionMap = useMemo(
    () => new Map(options.map((option) => [option.value, option])),
    [options]
  )

  const emit = (next: string[]) => onChange(normalizeChain(next))

  const move = (index: number, direction: -1 | 1) => {
    const targetIndex = index + direction
    if (targetIndex < 0 || targetIndex >= selected.length) return
    const next = [...selected]
    const [item] = next.splice(index, 1)
    next.splice(targetIndex, 0, item)
    emit(next)
  }

  const remove = (index: number) => {
    emit(removeGroupFromChain(selected, index))
  }

  const supplierPicker = (
    <ApiKeyGroupPickerPopover
      options={options}
      selectedValues={selected}
      onApply={(groups) => emit(addGroupsToChain(selected, groups))}
      disabled={disabled}
      groupDisplay={groupDisplay}
      vendors={vendors}
      models={models}
      groupPerformance={groupPerformance}
      triggerLabel={t(groupTerms.add)}
      applyLabel={t('Add selected suppliers')}
    />
  )

  return (
    <div className='flex flex-col gap-2'>
      {selected.length === 0 ? (
        <Empty className='bg-muted/20 border py-8'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <SlidersHorizontal />
            </EmptyMedia>
            <EmptyTitle>{t('No suppliers selected')}</EmptyTitle>
            <EmptyDescription>
              {t('Add suppliers to set the request order for this API key.')}
            </EmptyDescription>
          </EmptyHeader>
          <EmptyContent>{supplierPicker}</EmptyContent>
        </Empty>
      ) : (
        <div className='flex flex-col gap-2'>
          {selected.map((group, index) => {
            const option = optionMap.get(group) ?? {
              value: group,
              label: group,
              desc: group,
            }
            return (
              <div
                key={`${group}-${index}`}
                className='bg-muted/35 flex min-h-14 items-center gap-2 rounded-lg border px-2.5 py-2 sm:min-h-16 sm:px-3'
              >
                <Badge
                  variant='secondary'
                  className='bg-background text-muted-foreground h-6 min-w-7 justify-center rounded-md px-1.5 tabular-nums'
                >
                  {index + 1}
                </Badge>
                <div className='min-w-0 flex-1'>
                  <div className='flex min-w-0 items-center gap-2'>
                    <span className='truncate text-sm font-medium'>
                      {option.label}
                    </span>
                    <span className='hidden sm:block'>
                      <GroupRatioBadge ratio={option.ratio} />
                    </span>
                  </div>
                  <GroupDescription desc={option.desc} />
                </div>
                <div className='flex shrink-0 items-center gap-1'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    className='size-8'
                    disabled={disabled || index === 0}
                    onClick={() => move(index, -1)}
                    title={t('Move up')}
                  >
                    <ArrowUp className='size-4' />
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    className='size-8'
                    disabled={disabled || index === selected.length - 1}
                    onClick={() => move(index, 1)}
                    title={t('Move down')}
                  >
                    <ArrowDown className='size-4' />
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    className={cn(
                      'size-8',
                      selected.length === 1 &&
                        group === AUTO_GROUP_VALUE &&
                        'text-muted-foreground'
                    )}
                    disabled={
                      disabled ||
                      (selected.length === 1 && group === AUTO_GROUP_VALUE)
                    }
                    onClick={() => remove(index)}
                    title={t('Remove')}
                  >
                    <Trash2 className='size-4' />
                  </Button>
                </div>
              </div>
            )
          })}
        </div>
      )}

      {selected.length > 0 && supplierPicker}
    </div>
  )
}

export function ApiKeyGroupPickerPopover({
  options,
  selectedValues,
  onApply,
  disabled,
  groupDisplay,
  vendors,
  models,
  groupPerformance,
  triggerLabel,
  selectedCountLabel,
  applyLabel,
}: ApiKeyGroupPickerPopoverProps) {
  const { t } = useTranslation()
  const groupTerms = USER_FACING_GROUP_TERMS
  const [open, setOpen] = useState(false)
  const [searchValue, setSearchValue] = useState('')
  const [vendorFilter, setVendorFilter] = useState(ALL_VENDOR_VALUE)
  const [sortMode, setSortMode] = useState<ApiKeyGroupSort>('name_asc')
  const [pendingGroups, setPendingGroups] = useState<string[]>([])
  const selectedSet = useMemo(() => new Set(selectedValues), [selectedValues])
  const availableOptions = useMemo(
    () => options.filter((option) => !selectedSet.has(option.value)),
    [options, selectedSet]
  )
  const filterMetadata = useMemo(
    () =>
      buildApiKeyGroupFilterMetadata({
        options: availableOptions,
        groupDisplay,
        vendors,
        models,
      }),
    [availableOptions, groupDisplay, models, vendors]
  )
  const effectiveVendorFilter = filterMetadata.vendors.some(
    (vendor) => vendor.value === vendorFilter
  )
    ? vendorFilter
    : ALL_VENDOR_VALUE
  const filteredOptions = useMemo(() => {
    return filterApiKeyGroupOptions(availableOptions, filterMetadata, {
      category: ALL_CATEGORY_VALUE,
      vendor: effectiveVendorFilter,
      search: searchValue,
    })
  }, [availableOptions, effectiveVendorFilter, filterMetadata, searchValue])
  const sortedFilteredOptions = useMemo(
    () =>
      sortApiKeyGroupOptions(filteredOptions, {
        sort: sortMode,
        groupPerformance,
      }),
    [filteredOptions, groupPerformance, sortMode]
  )
  const pendingSet = useMemo(() => new Set(pendingGroups), [pendingGroups])
  const filteredValues = useMemo(
    () => sortedFilteredOptions.map((option) => option.value),
    [sortedFilteredOptions]
  )
  const showVendorFilters = filterMetadata.vendors.length > 1

  const togglePendingGroup = (group: string, checked: boolean) => {
    setPendingGroups((current) => {
      if (checked) {
        return current.includes(group) ? current : [...current, group]
      }
      return current.filter((item) => item !== group)
    })
  }

  const selectVisible = (groups: string[]) => {
    setPendingGroups((current) => {
      const next = [...current]
      for (const group of groups) {
        if (!next.includes(group)) next.push(group)
      }
      return next
    })
  }

  const clearPendingGroups = () => {
    setPendingGroups([])
  }

  const toggleSortMode = (config: SortButtonConfig) => {
    setSortMode((current) => {
      if (current === config.asc) return config.desc
      if (current === config.desc) return config.asc
      return config.defaultMode
    })
  }

  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen)
    if (!nextOpen) {
      setSearchValue('')
      setPendingGroups([])
    }
  }

  const applyPendingGroups = () => {
    onApply(pendingGroups)
    handleOpenChange(false)
  }

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger
        render={
          <Button
            type='button'
            variant='outline'
            className='h-9 w-full justify-between rounded-lg border-dashed'
            disabled={disabled || availableOptions.length === 0}
          />
        }
      >
        <span className='flex items-center gap-2'>
          <Plus className='size-4' />
          {triggerLabel}
        </span>
        <ChevronsUpDown className='size-4 opacity-50' />
      </PopoverTrigger>
      <PopoverContent
        className='w-[min(92vw,760px)] overflow-hidden rounded-xl p-0 shadow-lg'
        onWheel={(event) => event.stopPropagation()}
        onTouchMove={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
      >
        <div className='bg-background'>
          <div className='border-b p-3'>
            <div className='flex flex-col gap-2'>
              <div className='flex flex-col gap-2 sm:flex-row'>
                <div className='relative min-w-0 flex-1'>
                  <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2' />
                  <Input
                    value={searchValue}
                    onChange={(event) => setSearchValue(event.target.value)}
                    placeholder={t(groupTerms.search)}
                    className='h-9 pl-9'
                  />
                </div>
                <div
                  className='flex shrink-0 gap-1 overflow-x-auto pb-0.5 sm:pb-0'
                  aria-label={t('Sort suppliers')}
                >
                  {SORT_BUTTONS.map((config) => {
                    const active =
                      sortMode === config.asc || sortMode === config.desc
                    const ascending = sortMode === config.asc
                    const Icon = config.icon
                    const DirectionIcon = ascending ? ArrowUp : ArrowDown

                    return (
                      <Button
                        key={config.key}
                        type='button'
                        variant={active ? 'secondary' : 'outline'}
                        size='sm'
                        className='h-9 shrink-0 gap-1.5 px-2.5 text-xs'
                        onClick={() => toggleSortMode(config)}
                        aria-pressed={active}
                        title={t('Click again to reverse sort')}
                      >
                        <Icon className='size-3.5' />
                        <span>{t(config.label)}</span>
                        {active && <DirectionIcon className='size-3.5' />}
                      </Button>
                    )
                  })}
                </div>
              </div>
              {showVendorFilters && (
                <div
                  className='flex gap-1.5 overflow-x-auto pb-1'
                  role='tablist'
                  aria-label={t('Filter by vendor')}
                >
                  {filterMetadata.vendors.map((vendor) => (
                    <button
                      key={vendor.value}
                      type='button'
                      role='tab'
                      aria-selected={effectiveVendorFilter === vendor.value}
                      className={cn(
                        'border-border bg-background text-muted-foreground hover:bg-muted/60 hover:text-foreground inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition-colors',
                        effectiveVendorFilter === vendor.value &&
                          'border-primary/40 bg-primary/10 text-primary shadow-xs'
                      )}
                      onClick={() => setVendorFilter(vendor.value)}
                    >
                      <span>{t(vendor.label)}</span>
                      <span
                        className={cn(
                          'bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[10px] tabular-nums',
                          effectiveVendorFilter === vendor.value &&
                            'bg-primary/15 text-primary'
                        )}
                      >
                        {vendor.count}
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>

          <ScrollArea className='h-[380px]'>
            <div className='p-3'>
              {sortedFilteredOptions.length === 0 ? (
                <div className='text-muted-foreground flex h-40 items-center justify-center rounded-lg border border-dashed text-sm'>
                  {t(groupTerms.noneFound)}
                </div>
              ) : (
                <div className='grid gap-1.5'>
                  {sortedFilteredOptions.map((option) => {
                    const categoryLabel =
                      filterMetadata.groupCategoryLabels.get(option.value)
                    const selected = pendingSet.has(option.value)
                    const canOpenSupportedModels =
                      option.value !== AUTO_GROUP_VALUE

                    return (
                      <div
                        key={option.value}
                        className={cn(
                          'hover:bg-muted/60 flex items-start gap-3 rounded-lg border px-3 py-2.5 transition-colors',
                          selected && 'border-primary/30 bg-primary/5'
                        )}
                      >
                        <Checkbox
                          checked={selected}
                          onCheckedChange={(checked) =>
                            togglePendingGroup(option.value, checked === true)
                          }
                          className='mt-0.5'
                        />
                        <button
                          type='button'
                          className='min-w-0 flex-1 cursor-pointer text-left'
                          onClick={() =>
                            togglePendingGroup(option.value, !selected)
                          }
                          aria-pressed={selected}
                        >
                          <span className='flex min-w-0 items-center gap-2'>
                            <span className='truncate text-sm font-medium'>
                              {option.label}
                            </span>
                            {categoryLabel && (
                              <Badge
                                variant='outline'
                                className='bg-background text-muted-foreground h-6 max-w-32 shrink-0 rounded-md px-2 text-xs'
                              >
                                <span className='truncate'>
                                  {t(categoryLabel)}
                                </span>
                              </Badge>
                            )}
                            {selected && (
                              <Check className='text-primary size-4 shrink-0' />
                            )}
                          </span>
                          <GroupDescription desc={option.desc} />
                        </button>
                        <span className='flex shrink-0 flex-col items-end gap-1'>
                          <span className='flex items-center gap-1.5'>
                            <GroupPerformanceInline
                              perf={groupPerformance?.[option.value]}
                              compact
                              className='hidden sm:flex'
                            />
                            <GroupRatioBadge ratio={option.ratio} />
                          </span>
                          {canOpenSupportedModels && (
                            <a
                              href={buildPricingGroupUrl(option.value)}
                              target='_blank'
                              rel='noopener noreferrer'
                              className='text-muted-foreground hover:text-foreground inline-flex items-center gap-1 text-xs underline-offset-2 hover:underline'
                              onClick={(event) => {
                                event.stopPropagation()
                              }}
                              title={t('View supported models')}
                            >
                              {t('View supported models')}
                            </a>
                          )}
                        </span>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          </ScrollArea>

          <div className='flex items-center justify-between gap-3 border-t p-3'>
            <span className='text-muted-foreground text-xs'>
              {selectedCountLabel ??
                t('{{count}} supplier(s) selected', {
                  count: pendingGroups.length,
                })}
            </span>
            <div className='flex items-center gap-2'>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={() => selectVisible(filteredValues)}
                disabled={filteredValues.length === 0}
              >
                {t('Select all')}
              </Button>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={clearPendingGroups}
                disabled={pendingGroups.length === 0}
              >
                {t('Clear all')}
              </Button>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => handleOpenChange(false)}
              >
                {t('Cancel')}
              </Button>
              <Button
                type='button'
                size='sm'
                disabled={pendingGroups.length === 0}
                onClick={applyPendingGroups}
              >
                {applyLabel ?? t('Add selected suppliers')}
              </Button>
            </div>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}
