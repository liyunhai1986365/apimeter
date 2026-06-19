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
import type {
  PricingGroupDisplayConfig,
  PricingModel,
  PricingVendor,
} from '@/features/pricing/types'
import type { ApiKeyGroupOption } from '../components/api-key-group-combobox'
import { AUTO_GROUP_VALUE } from './api-key-groups'

export const ALL_CATEGORY_VALUE = '__all_categories__'
export const UNCATEGORIZED_CATEGORY_VALUE = '__uncategorized__'
export const ALL_VENDOR_VALUE = '__all_vendors__'

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

function optionValues(options: ApiKeyGroupOption[]) {
  return new Set(
    options
      .map((option) => option.value)
      .filter((value) => value && value !== AUTO_GROUP_VALUE)
  )
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
    const categoryId = categoryLabels.has(item.category_id)
      ? item.category_id
      : UNCATEGORIZED_CATEGORY_VALUE
    groupCategory.set(item.group, categoryId)
  }
  for (const group of groups) {
    if (!groupCategory.has(group)) {
      groupCategory.set(group, UNCATEGORIZED_CATEGORY_VALUE)
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
        label: 'All vendors',
        count: groups.size,
      },
      ...vendorsWithCounts.map(({ value, label, count }) => ({
        value,
        label,
        count,
      })),
    ],
    groupCategory,
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
