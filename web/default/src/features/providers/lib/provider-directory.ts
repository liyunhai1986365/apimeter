/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { MODEL_CATEGORIES } from '@/features/pricing/constants'
import { sortVendorsByConfiguredOrder } from '@/features/pricing/lib/vendor-order'
import { isSEOCategory } from '@/features/pricing/seo-categories'
import type {
  ModelCategory,
  PricingModel,
  PricingVendor,
} from '@/features/pricing/types'

export type ProviderDirectoryItem = {
  vendor: PricingVendor
  slug: string
  models: PricingModel[]
  categories: ModelCategory[]
  endpointTypes: string[]
}

export function getProviderSlugBase(name: string, id: number): string {
  const slug = name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')

  return slug || `provider-${id}`
}

export function buildProviderDirectory(
  models: PricingModel[],
  vendors: PricingVendor[]
): ProviderDirectoryItem[] {
  const modelsByVendor = new Map<number, PricingModel[]>()
  for (const model of models) {
    if (!model.vendor_id) continue
    const items = modelsByVendor.get(model.vendor_id) ?? []
    items.push(model)
    modelsByVendor.set(model.vendor_id, items)
  }

  const usedSlugs = new Set<string>()
  return sortVendorsByConfiguredOrder(vendors)
    .map((vendor) => {
      const vendorModels = [...(modelsByVendor.get(vendor.id) ?? [])].sort(
        (a, b) => {
          const aOrder = a.sort_order ?? Number.MAX_SAFE_INTEGER
          const bOrder = b.sort_order ?? Number.MAX_SAFE_INTEGER
          if (aOrder !== bOrder) return aOrder - bOrder
          return a.model_name.localeCompare(b.model_name)
        }
      )
      if (vendorModels.length === 0) return null

      const slugBase = getProviderSlugBase(vendor.name, vendor.id)
      let slug = usedSlugs.has(slugBase) ? `${slugBase}-${vendor.id}` : slugBase
      let suffix = 2
      while (usedSlugs.has(slug)) {
        slug = `${slugBase}-${vendor.id}-${suffix}`
        suffix += 1
      }
      usedSlugs.add(slug)

      const categories = Array.from(
        new Set(vendorModels.map((model) => getProviderModelCategory(model)))
      )
      const endpointTypes = Array.from(
        new Set(
          vendorModels.flatMap((model) => model.supported_endpoint_types ?? [])
        )
      ).sort()

      return {
        vendor,
        slug,
        models: vendorModels,
        categories,
        endpointTypes,
      }
    })
    .filter((item): item is ProviderDirectoryItem => Boolean(item))
}

export function getProviderModelCategory(model: PricingModel): ModelCategory {
  const category = model.category || MODEL_CATEGORIES.TEXT
  return isSEOCategory(category) ? category : MODEL_CATEGORIES.OTHER
}

export function filterProviderDirectory(
  providers: ProviderDirectoryItem[],
  search: string
): ProviderDirectoryItem[] {
  const query = search.trim().toLowerCase()
  if (!query) return providers

  return providers.filter(
    (provider) =>
      provider.vendor.name.toLowerCase().includes(query) ||
      provider.vendor.description?.toLowerCase().includes(query) ||
      provider.models.some((model) =>
        model.model_name.toLowerCase().includes(query)
      )
  )
}
