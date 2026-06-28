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
import type { PricingModel } from '../types'
import { compareVendorOrder } from './vendor-order'

export type ModelCardVendorGroup = {
  key: string
  name: string
  description?: string
  icon?: string
  id: number
  sortOrder: number
  models: PricingModel[]
  totalCount: number
}

function compareModelOrder(a: PricingModel, b: PricingModel): number {
  const orderDelta = (a.sort_order || 0) - (b.sort_order || 0)
  if (orderDelta !== 0) return orderDelta
  const updatedDelta = (b.updated_time || 0) - (a.updated_time || 0)
  if (updatedDelta !== 0) return updatedDelta
  return (a.model_name || '').localeCompare(b.model_name || '')
}

export function buildModelCardGroups(
  models: PricingModel[]
): ModelCardVendorGroup[] {
  const groups = new Map<string, ModelCardVendorGroup>()

  for (const model of models) {
    const vendorId = model.vendor_id ?? Number.MAX_SAFE_INTEGER
    const vendorName = model.vendor_name || 'Other Providers'
    const key = model.vendor_id ? String(model.vendor_id) : `unknown-${vendorName}`
    const existing = groups.get(key)

    if (existing) {
      existing.models.push(model)
      existing.totalCount += 1
      continue
    }

    groups.set(key, {
      key,
      name: vendorName,
      description: model.vendor_description,
      icon: model.vendor_icon,
      id: vendorId,
      sortOrder: model.vendor_sort_order ?? Number.MAX_SAFE_INTEGER,
      models: [model],
      totalCount: 1,
    })
  }

  return Array.from(groups.values())
    .map((group) => ({
      ...group,
      models: [...group.models].sort(compareModelOrder),
    }))
    .sort((a, b) =>
      compareVendorOrder(
        { id: a.id, name: a.name, sort_order: a.sortOrder },
        { id: b.id, name: b.name, sort_order: b.sortOrder }
      )
    )
}
