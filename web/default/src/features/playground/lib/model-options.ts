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
import type { PricingModel } from '@/features/pricing/types'
import type { ModelOption } from '../types'

export function isPricingModelEnabledForGroup(
  model: PricingModel,
  group: string
): boolean {
  if (!group) return true
  const groups = Array.isArray(model.enable_groups) ? model.enable_groups : []
  return groups.includes(group) || groups.includes('all')
}

export function buildPlaygroundModelOptions(
  models: PricingModel[],
  group: string
): ModelOption[] {
  return models
    .filter((model) => isPricingModelEnabledForGroup(model, group))
    .map((model) => {
      const option: ModelOption = {
        label: model.model_name,
        value: model.model_name,
      }
      if (model.category) option.category = model.category
      if (model.description) option.description = model.description
      return option
    })
}
