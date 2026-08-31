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
import { api } from '@/lib/api'
import type { PricingData, PricingModel } from './types'

// ----------------------------------------------------------------------------
// Pricing APIs
// ----------------------------------------------------------------------------

// Get model pricing data
export async function getPricing(): Promise<PricingData> {
  const res = await api.get('/api/pricing')
  return res.data
}

export async function getQuotationUserGroups(): Promise<string[]> {
  const res = await api.get('/api/group/')
  return res.data.data ?? []
}

export async function getPricingForQuotation(
  userGroup: string
): Promise<PricingData> {
  const res = await api.get('/api/pricing/quotation', {
    params: { user_group: userGroup },
  })
  return res.data
}

export function hydratePricingModels(data: PricingData): PricingModel[] {
  const vendorMap = new Map(data.vendors.map((vendor) => [vendor.id, vendor]))

  return data.data.map((model) => {
    const vendor = model.vendor_id ? vendorMap.get(model.vendor_id) : undefined
    const modelGroupRatio = { ...data.group_ratio }
    for (const [group, modelRatios] of Object.entries(
      data.group_model_ratio ?? {}
    )) {
      const ratio = modelRatios[model.model_name]
      if (ratio !== undefined) modelGroupRatio[group] = ratio
    }
    return {
      ...model,
      key: model.model_name,
      vendor_name: vendor?.name,
      vendor_icon: vendor?.icon,
      vendor_description: vendor?.description,
      vendor_sort_order: vendor?.sort_order ?? Number.MAX_SAFE_INTEGER,
      sort_order: model.sort_order ?? 0,
      group_ratio: modelGroupRatio,
    }
  })
}
