import type { PricingVendor } from '../types'

export function compareVendorOrder(
  a: Pick<PricingVendor, 'id' | 'name' | 'sort_order'>,
  b: Pick<PricingVendor, 'id' | 'name' | 'sort_order'>
): number {
  const aOrder = a.sort_order ?? Number.MAX_SAFE_INTEGER
  const bOrder = b.sort_order ?? Number.MAX_SAFE_INTEGER
  if (aOrder !== bOrder) return aOrder - bOrder
  if (a.id !== b.id) return a.id - b.id
  return a.name.localeCompare(b.name)
}

export function sortVendorsByConfiguredOrder<T extends PricingVendor>(
  vendors: T[]
): T[] {
  return [...vendors].sort(compareVendorOrder)
}
