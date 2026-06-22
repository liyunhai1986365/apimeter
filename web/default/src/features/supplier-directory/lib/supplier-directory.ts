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
  PricingData,
  PricingGroupDisplayCategory,
  PricingGroupPerformance,
  PricingModel,
  PricingVendor,
} from '@/features/pricing/types'

export const ALL_SUPPLIER_CATEGORY_VALUE = '__all_supplier_categories__'
export const UNCATEGORIZED_SUPPLIER_CATEGORY_VALUE =
  '__uncategorized_supplier_category__'
export const ALL_SUPPLIER_VENDOR_VALUE = '__all_supplier_vendors__'

export type SupplierDirectoryCategory = {
  value: string
  label: string
  count: number
}

export type SupplierDirectoryVendor = {
  value: string
  label: string
  count: number
}

export type SupplierDirectoryItem = {
  group: string
  description?: string
  ratio: number
  performance?: PricingGroupPerformance
  categoryId: string
  categoryName: string
  order: number
  vendors: PricingVendor[]
  models: PricingModel[]
}

export type SupplierProviderItem = {
  vendor: PricingVendor
  groups: SupplierDirectoryItem[]
  models: PricingModel[]
  categories: SupplierDirectoryCategory[]
  discountedGroups: SupplierDirectoryItem[]
}

export type SupplierDirectoryData = {
  items: SupplierDirectoryItem[]
  providers: SupplierProviderItem[]
  categories: SupplierDirectoryCategory[]
  vendors: SupplierDirectoryVendor[]
}

export type SupplierDirectoryFilters = {
  category: string
  vendor: string
  search: string
}

type GroupInfo = string | { desc?: string; ratio?: number }

function getGroupDescription(info: GroupInfo | undefined): string | undefined {
  if (typeof info === 'string') return info
  return info?.desc
}

function getGroupRatio(
  group: string,
  info: GroupInfo | undefined,
  groupRatio: Record<string, number>
): number {
  if (typeof info === 'object' && Number.isFinite(info.ratio)) {
    return Number(info.ratio)
  }
  const ratio = groupRatio[group]
  return Number.isFinite(ratio) ? Number(ratio) : 1
}

function getSortedCategories(categories: PricingGroupDisplayCategory[]) {
  return [...categories].sort((a, b) => {
    if (a.order !== b.order) return a.order - b.order
    if (a.name !== b.name) return a.name.localeCompare(b.name)
    return a.id.localeCompare(b.id)
  })
}

function getModelSupportedGroups(model: PricingModel, allGroups: Set<string>) {
  const enabledGroups = Array.isArray(model.enable_groups)
    ? model.enable_groups
    : []
  if (enabledGroups.includes('all')) return [...allGroups]
  return enabledGroups.filter((group) => allGroups.has(group))
}

export function buildSupplierDirectoryData(
  pricing: PricingData | undefined
): SupplierDirectoryData {
  if (!pricing) {
    return { items: [], providers: [], categories: [], vendors: [] }
  }

  const groups = Object.keys(pricing.usable_group || {}).filter(
    (group) => group !== 'auto'
  )
  const groupSet = new Set(groups)
  const categoryDefinitions = getSortedCategories(
    pricing.group_display?.categories ?? []
  )
  const categoryNames = new Map(
    categoryDefinitions.map((category) => [category.id, category.name])
  )
  const groupCategory = new Map<string, { id: string; order: number }>()

  for (const item of pricing.group_display?.groups ?? []) {
    if (!groupSet.has(item.group)) continue
    const id = categoryNames.has(item.category_id)
      ? item.category_id
      : UNCATEGORIZED_SUPPLIER_CATEGORY_VALUE
    groupCategory.set(item.group, { id, order: item.order })
  }

  const vendorMap = new Map(
    (pricing.vendors || []).map((vendor) => [String(vendor.id), vendor])
  )
  const groupModels = new Map<string, PricingModel[]>()
  const groupVendorIds = new Map<string, Set<string>>()
  for (const group of groups) {
    groupModels.set(group, [])
    groupVendorIds.set(group, new Set())
  }

  for (const model of pricing.data || []) {
    const vendorId = model.vendor_id == null ? '' : String(model.vendor_id)
    const supportedGroups = getModelSupportedGroups(model, groupSet)
    for (const group of supportedGroups) {
      groupModels.get(group)?.push(model)
      if (vendorId && vendorMap.has(vendorId)) {
        groupVendorIds.get(group)?.add(vendorId)
      }
    }
  }

  const items = groups
    .map<SupplierDirectoryItem>((group) => {
      const info = pricing.usable_group[group]
      const category = groupCategory.get(group) ?? {
        id: UNCATEGORIZED_SUPPLIER_CATEGORY_VALUE,
        order: Number.MAX_SAFE_INTEGER,
      }
      const categoryName =
        categoryNames.get(category.id) ||
        (category.id === UNCATEGORIZED_SUPPLIER_CATEGORY_VALUE
          ? 'Uncategorized'
          : category.id)
      const vendors = [...(groupVendorIds.get(group) ?? new Set())]
        .map((id) => vendorMap.get(id))
        .filter((vendor): vendor is PricingVendor => Boolean(vendor))
        .sort((a, b) => {
          const aOrder = a.sort_order ?? Number.MAX_SAFE_INTEGER
          const bOrder = b.sort_order ?? Number.MAX_SAFE_INTEGER
          if (aOrder !== bOrder) return aOrder - bOrder
          if (a.id !== b.id) return a.id - b.id
          return a.name.localeCompare(b.name)
        })
      const models = [...(groupModels.get(group) ?? [])].sort((a, b) => {
        const aOrder = a.sort_order ?? Number.MAX_SAFE_INTEGER
        const bOrder = b.sort_order ?? Number.MAX_SAFE_INTEGER
        if (aOrder !== bOrder) return aOrder - bOrder
        return a.model_name.localeCompare(b.model_name)
      })

      return {
        group,
        description: getGroupDescription(info),
        ratio: getGroupRatio(group, info, pricing.group_ratio || {}),
        performance: pricing.group_perf?.[group],
        categoryId: category.id,
        categoryName,
        order: category.order,
        vendors,
        models,
      }
    })
    .sort((a, b) => {
      const aCategoryOrder = categoryDefinitions.findIndex(
        (category) => category.id === a.categoryId
      )
      const bCategoryOrder = categoryDefinitions.findIndex(
        (category) => category.id === b.categoryId
      )
      const normalizedA =
        aCategoryOrder === -1 ? Number.MAX_SAFE_INTEGER : aCategoryOrder
      const normalizedB =
        bCategoryOrder === -1 ? Number.MAX_SAFE_INTEGER : bCategoryOrder
      if (normalizedA !== normalizedB) return normalizedA - normalizedB
      if (a.order !== b.order) return a.order - b.order
      return a.group.localeCompare(b.group)
    })

  const categories: SupplierDirectoryCategory[] = [
    {
      value: ALL_SUPPLIER_CATEGORY_VALUE,
      label: 'All categories',
      count: items.length,
    },
  ]
  for (const category of categoryDefinitions) {
    const count = items.filter((item) => item.categoryId === category.id).length
    if (count > 0) {
      categories.push({ value: category.id, label: category.name, count })
    }
  }
  const uncategorizedCount = items.filter(
    (item) => item.categoryId === UNCATEGORIZED_SUPPLIER_CATEGORY_VALUE
  ).length
  if (uncategorizedCount > 0) {
    categories.push({
      value: UNCATEGORIZED_SUPPLIER_CATEGORY_VALUE,
      label: 'Uncategorized',
      count: uncategorizedCount,
    })
  }

  const vendors = [
    {
      value: ALL_SUPPLIER_VENDOR_VALUE,
      label: 'All vendors',
      count: items.length,
    },
    ...(pricing.vendors || [])
      .map((vendor) => {
        const value = String(vendor.id)
        const count = items.filter((item) =>
          item.vendors.some((itemVendor) => String(itemVendor.id) === value)
        ).length
        return { value, label: vendor.name, count, vendor }
      })
      .filter((item) => item.count > 0)
      .sort((a, b) => {
        const aOrder = a.vendor.sort_order ?? Number.MAX_SAFE_INTEGER
        const bOrder = b.vendor.sort_order ?? Number.MAX_SAFE_INTEGER
        if (aOrder !== bOrder) return aOrder - bOrder
        return a.vendor.id - b.vendor.id
      })
      .map(({ value, label, count }) => ({ value, label, count })),
  ]

  const providers = (pricing.vendors || [])
    .map<SupplierProviderItem | null>((vendor) => {
      const vendorId = String(vendor.id)
      const groupsForVendor = items.filter((item) =>
        item.vendors.some((itemVendor) => String(itemVendor.id) === vendorId)
      )
      if (groupsForVendor.length === 0) return null

      const modelMap = new Map<string, PricingModel>()
      for (const group of groupsForVendor) {
        for (const model of group.models) {
          if (model.vendor_id === vendor.id) {
            modelMap.set(model.model_name, model)
          }
        }
      }

      const categoryMap = new Map<string, SupplierDirectoryCategory>()
      for (const group of groupsForVendor) {
        categoryMap.set(group.categoryId, {
          value: group.categoryId,
          label: group.categoryName,
          count: groupsForVendor.filter(
            (candidate) => candidate.categoryId === group.categoryId
          ).length,
        })
      }

      return {
        vendor,
        groups: groupsForVendor,
        models: [...modelMap.values()].sort((a, b) => {
          const aOrder = a.sort_order ?? Number.MAX_SAFE_INTEGER
          const bOrder = b.sort_order ?? Number.MAX_SAFE_INTEGER
          if (aOrder !== bOrder) return aOrder - bOrder
          return a.model_name.localeCompare(b.model_name)
        }),
        categories: [...categoryMap.values()].sort((a, b) =>
          a.label.localeCompare(b.label)
        ),
        discountedGroups: groupsForVendor.filter(
          (group) => group.ratio > 0 && group.ratio < 1
        ),
      }
    })
    .filter((item): item is SupplierProviderItem => Boolean(item))
    .sort((a, b) => {
      const aOrder = a.vendor.sort_order ?? Number.MAX_SAFE_INTEGER
      const bOrder = b.vendor.sort_order ?? Number.MAX_SAFE_INTEGER
      if (aOrder !== bOrder) return aOrder - bOrder
      if (a.vendor.id !== b.vendor.id) return a.vendor.id - b.vendor.id
      return a.vendor.name.localeCompare(b.vendor.name)
    })

  return { items, providers, categories, vendors }
}

export function filterSupplierDirectoryItems(
  items: SupplierDirectoryItem[],
  filters: SupplierDirectoryFilters
): SupplierDirectoryItem[] {
  const search = filters.search.trim().toLowerCase()

  return items.filter((item) => {
    if (
      filters.category !== ALL_SUPPLIER_CATEGORY_VALUE &&
      item.categoryId !== filters.category
    ) {
      return false
    }
    if (
      filters.vendor !== ALL_SUPPLIER_VENDOR_VALUE &&
      !item.vendors.some((vendor) => String(vendor.id) === filters.vendor)
    ) {
      return false
    }
    if (!search) return true

    return (
      item.group.toLowerCase().includes(search) ||
      item.categoryName.toLowerCase().includes(search) ||
      item.description?.toLowerCase().includes(search) ||
      item.vendors.some((vendor) =>
        vendor.name.toLowerCase().includes(search)
      ) ||
      item.models.some((model) =>
        model.model_name.toLowerCase().includes(search)
      )
    )
  })
}

export function filterSupplierProviders(
  providers: SupplierProviderItem[],
  filters: Pick<SupplierDirectoryFilters, 'category' | 'search'>
): SupplierProviderItem[] {
  const search = filters.search.trim().toLowerCase()

  return providers
    .map((provider) => {
      const groups = provider.groups.filter((group) => {
        if (
          filters.category !== ALL_SUPPLIER_CATEGORY_VALUE &&
          group.categoryId !== filters.category
        ) {
          return false
        }
        if (!search) return true

        return (
          provider.vendor.name.toLowerCase().includes(search) ||
          provider.vendor.description?.toLowerCase().includes(search) ||
          group.group.toLowerCase().includes(search) ||
          group.categoryName.toLowerCase().includes(search) ||
          group.description?.toLowerCase().includes(search) ||
          provider.models.some((model) =>
            model.model_name.toLowerCase().includes(search)
          )
        )
      })

      if (groups.length === 0) return null

      const modelMap = new Map<string, PricingModel>()
      for (const group of groups) {
        for (const model of group.models) {
          if (model.vendor_id === provider.vendor.id) {
            modelMap.set(model.model_name, model)
          }
        }
      }

      const categoryMap = new Map<string, SupplierDirectoryCategory>()
      for (const group of groups) {
        categoryMap.set(group.categoryId, {
          value: group.categoryId,
          label: group.categoryName,
          count: groups.filter(
            (candidate) => candidate.categoryId === group.categoryId
          ).length,
        })
      }

      return {
        ...provider,
        groups,
        models: [...modelMap.values()],
        categories: [...categoryMap.values()],
        discountedGroups: groups.filter(
          (group) => group.ratio > 0 && group.ratio < 1
        ),
      }
    })
    .filter((item): item is SupplierProviderItem => Boolean(item))
}
