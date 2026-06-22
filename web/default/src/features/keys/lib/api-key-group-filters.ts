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
import type { PerfGroupSummary } from '@/features/performance-metrics/types'
import type {
  PricingGroupDisplayConfig,
  PricingModel,
  PricingVendor,
} from '@/features/pricing/types'
import type { ApiKeyGroupOption } from '../components/api-key-group-combobox'
import { AUTO_GROUP_VALUE } from './api-key-groups'

export const ALL_CATEGORY_VALUE = '__all_categories__'
export const UNCATEGORIZED_CATEGORY_VALUE = '__uncategorized__'
export const USER_OWNED_CATEGORY_VALUE = 'user_owned'
export const ALL_VENDOR_VALUE = '__all_vendors__'

export type ApiKeyGroupSort =
  | 'name_asc'
  | 'name_desc'
  | 'discount_desc'
  | 'discount_asc'
  | 'latency_asc'
  | 'latency_desc'

export type ApiKeyGroupCategoryFilter = {
  value: string
  label: string
  count: number
}

export type ApiKeyGroupVendorFilter = {
  value: string
  label: string
  count: number
}

export type ApiKeyGroupFilterMetadata = {
  categories: ApiKeyGroupCategoryFilter[]
  vendors: ApiKeyGroupVendorFilter[]
  groupCategory: Map<string, string>
  groupCategoryLabels: Map<string, string>
  groupVendors: Map<string, Set<string>>
}

type BuildApiKeyGroupFilterMetadataParams = {
  options: ApiKeyGroupOption[]
  groupDisplay?: PricingGroupDisplayConfig
  vendors?: PricingVendor[]
  models?: PricingModel[]
}

type FilterApiKeyGroupOptionsParams = {
  category: string
  vendor: string
  search: string
}

type SortApiKeyGroupOptionsParams = {
  sort: ApiKeyGroupSort
  groupPerformance?: Record<string, PerfGroupSummary>
}

function optionValues(options: ApiKeyGroupOption[]) {
  return new Set(
    options
      .map((option) => option.value)
      .filter((value) => value && value !== AUTO_GROUP_VALUE)
  )
}

function isUserOwnedGroup(group: string) {
  return group.startsWith('user_owned:')
}

export function buildApiKeyGroupFilterMetadata({
  options,
  groupDisplay,
  vendors = [],
  models = [],
}: BuildApiKeyGroupFilterMetadataParams): ApiKeyGroupFilterMetadata {
  const groups = optionValues(options)
  const categoryDefinitions = [...(groupDisplay?.categories ?? [])].sort(
    (a, b) => {
      if (a.order !== b.order) return a.order - b.order
      if (a.name !== b.name) return a.name.localeCompare(b.name)
      return a.id.localeCompare(b.id)
    }
  )
  const categoryLabels = new Map(
    categoryDefinitions.map((category) => [category.id, category.name])
  )
  const groupCategory = new Map<string, string>()
  for (const item of groupDisplay?.groups ?? []) {
    if (!groups.has(item.group)) continue
    if (isUserOwnedGroup(item.group)) {
      groupCategory.set(item.group, USER_OWNED_CATEGORY_VALUE)
      continue
    }
    const categoryId = categoryLabels.has(item.category_id)
      ? item.category_id
      : UNCATEGORIZED_CATEGORY_VALUE
    groupCategory.set(item.group, categoryId)
  }
  for (const group of groups) {
    if (!groupCategory.has(group)) {
      groupCategory.set(
        group,
        isUserOwnedGroup(group)
          ? USER_OWNED_CATEGORY_VALUE
          : UNCATEGORIZED_CATEGORY_VALUE
      )
    }
  }

  const groupVendors = new Map<string, Set<string>>()
  const vendorIds = new Set(vendors.map((vendor) => String(vendor.id)))
  for (const group of groups) {
    groupVendors.set(group, new Set())
  }
  for (const model of models) {
    const vendorId = model.vendor_id == null ? '' : String(model.vendor_id)
    if (!vendorId || !vendorIds.has(vendorId)) continue
    const enabledGroups = Array.isArray(model.enable_groups)
      ? model.enable_groups
      : []
    const supportedGroups = enabledGroups.includes('all')
      ? [...groups]
      : enabledGroups.filter((group) => groups.has(group))
    for (const group of supportedGroups) {
      groupVendors.get(group)?.add(vendorId)
    }
  }

  const categories: ApiKeyGroupCategoryFilter[] = [
    {
      value: ALL_CATEGORY_VALUE,
      label: 'All categories',
      count: groups.size,
    },
  ]
  const userOwnedCount = [...groupCategory.values()].filter(
    (value) => value === USER_OWNED_CATEGORY_VALUE
  ).length
  if (userOwnedCount > 0) {
    categories.push({
      value: USER_OWNED_CATEGORY_VALUE,
      label: 'User-owned suppliers',
      count: userOwnedCount,
    })
  }
  for (const category of categoryDefinitions) {
    const count = [...groupCategory.values()].filter(
      (value) => value === category.id
    ).length
    if (count === 0) continue
    categories.push({ value: category.id, label: category.name, count })
  }
  const uncategorizedCount = [...groupCategory.values()].filter(
    (value) => value === UNCATEGORIZED_CATEGORY_VALUE
  ).length
  if (uncategorizedCount > 0) {
    categories.push({
      value: UNCATEGORIZED_CATEGORY_VALUE,
      label: 'Uncategorized',
      count: uncategorizedCount,
    })
  }
  const categoryLabelByValue = new Map(
    categories.map((category) => [category.value, category.label])
  )
  const groupCategoryLabels = new Map(
    [...groupCategory.entries()].map(([group, category]) => [
      group,
      categoryLabelByValue.get(category) || 'Uncategorized',
    ])
  )

  const vendorsWithCounts = vendors
    .map((vendor) => {
      const vendorId = String(vendor.id)
      const count = [...groupVendors.values()].filter((ids) =>
        ids.has(vendorId)
      ).length
      return {
        value: vendorId,
        label: vendor.name,
        count,
        sortOrder: vendor.sort_order ?? Number.MAX_SAFE_INTEGER,
        id: vendor.id,
      }
    })
    .filter((vendor) => vendor.count > 0)
    .sort((a, b) => {
      if (a.sortOrder !== b.sortOrder) return a.sortOrder - b.sortOrder
      return a.id - b.id
    })

  return {
    categories,
    vendors: [
      {
        value: ALL_VENDOR_VALUE,
        label: 'All',
        count: groups.size,
      },
      ...vendorsWithCounts.map(({ value, label, count }) => ({
        value,
        label,
        count,
      })),
    ],
    groupCategory,
    groupCategoryLabels,
    groupVendors,
  }
}

export function filterApiKeyGroupOptions(
  options: ApiKeyGroupOption[],
  metadata: ApiKeyGroupFilterMetadata,
  filters: FilterApiKeyGroupOptionsParams
): ApiKeyGroupOption[] {
  const search = filters.search.trim().toLowerCase()

  return options.filter((option) => {
    if (option.value === AUTO_GROUP_VALUE) {
      return filters.category === ALL_CATEGORY_VALUE
    }
    if (
      filters.category !== ALL_CATEGORY_VALUE &&
      metadata.groupCategory.get(option.value) !== filters.category
    ) {
      return false
    }
    if (
      filters.vendor !== ALL_VENDOR_VALUE &&
      !metadata.groupVendors.get(option.value)?.has(filters.vendor)
    ) {
      return false
    }
    if (!search) return true
    const ratioText = String(option.ratio ?? '').toLowerCase()
    return (
      option.value.toLowerCase().includes(search) ||
      option.label.toLowerCase().includes(search) ||
      option.desc?.toLowerCase().includes(search) ||
      ratioText.includes(search)
    )
  })
}

function optionName(option: ApiKeyGroupOption) {
  return (option.label || option.value).toLowerCase()
}

function numericRatio(option: ApiKeyGroupOption) {
  return typeof option.ratio === 'number' && Number.isFinite(option.ratio)
    ? option.ratio
    : undefined
}

function compareName(a: ApiKeyGroupOption, b: ApiKeyGroupOption) {
  const byName = optionName(a).localeCompare(optionName(b))
  if (byName !== 0) return byName
  return a.value.localeCompare(b.value)
}

function compareOptionalNumber(
  a: number | undefined,
  b: number | undefined,
  direction: 'asc' | 'desc'
) {
  if (a == null && b == null) return 0
  if (a == null) return 1
  if (b == null) return -1
  return direction === 'asc' ? a - b : b - a
}

export function sortApiKeyGroupOptions(
  options: ApiKeyGroupOption[],
  params: SortApiKeyGroupOptionsParams
): ApiKeyGroupOption[] {
  const sorted = [...options]
  sorted.sort((a, b) => {
    if (a.value === AUTO_GROUP_VALUE || b.value === AUTO_GROUP_VALUE) {
      if (a.value === b.value) return 0
      return a.value === AUTO_GROUP_VALUE ? -1 : 1
    }

    if (params.sort === 'discount_desc') {
      const byDiscount = compareOptionalNumber(
        numericRatio(a),
        numericRatio(b),
        'asc'
      )
      return byDiscount || compareName(a, b)
    }

    if (params.sort === 'discount_asc') {
      const byDiscount = compareOptionalNumber(
        numericRatio(a),
        numericRatio(b),
        'desc'
      )
      return byDiscount || compareName(a, b)
    }

    if (params.sort === 'latency_asc') {
      const byLatency = compareOptionalNumber(
        params.groupPerformance?.[a.value]?.avg_latency_ms,
        params.groupPerformance?.[b.value]?.avg_latency_ms,
        'asc'
      )
      return byLatency || compareName(a, b)
    }

    if (params.sort === 'latency_desc') {
      const byLatency = compareOptionalNumber(
        params.groupPerformance?.[a.value]?.avg_latency_ms,
        params.groupPerformance?.[b.value]?.avg_latency_ms,
        'desc'
      )
      return byLatency || compareName(a, b)
    }

    if (params.sort === 'name_desc') {
      return -compareName(a, b)
    }

    return compareName(a, b)
  })
  return sorted
}
