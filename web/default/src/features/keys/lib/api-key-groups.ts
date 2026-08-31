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
import type { ApiKeyGroupOption } from '../components/api-key-group-combobox'

export const AUTO_GROUP_VALUE = 'auto'

type UserGroupInfo = {
  desc: string
  ratio: number | string
  hide_discount?: boolean
}

export type ApiKeyGroupPricingScope = {
  models?: readonly PricingModel[]
  modelLimits?: readonly string[]
}

function numericRatio(value: number | string | undefined): number | undefined {
  const ratio = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(ratio) && ratio >= 0 ? ratio : undefined
}

function modelMatchesLimits(
  model: PricingModel,
  modelLimits: ReadonlySet<string>
): boolean {
  if (modelLimits.size === 0) return true
  if (modelLimits.has(model.model_name)) return true
  return (model.alias_models ?? []).some((alias) => modelLimits.has(alias))
}

export function getLowestApiKeyGroupRatio(
  group: string,
  fallbackRatio: number | string | undefined,
  scope: ApiKeyGroupPricingScope = {}
): number | undefined {
  const fallback = numericRatio(fallbackRatio)
  const models = scope.models ?? []
  if (models.length === 0) return fallback

  const modelLimits = new Set(
    (scope.modelLimits ?? []).map((model) => model.trim()).filter(Boolean)
  )
  const ratios: number[] = []

  for (const model of models) {
    if (!modelMatchesLimits(model, modelLimits)) continue
    const enabledGroups = Array.isArray(model.enable_groups)
      ? model.enable_groups
      : []
    if (!enabledGroups.includes('all') && !enabledGroups.includes(group)) {
      continue
    }

    const ratio = numericRatio(model.group_ratio?.[group]) ?? fallback
    if (ratio !== undefined) ratios.push(ratio)
  }

  return ratios.length > 0 ? Math.min(...ratios) : fallback
}

export function buildApiKeyGroupOptions(
  groupsRaw: Record<string, UserGroupInfo>,
  _includeAutoGroup: boolean,
  _selectedGroup?: string,
  pricingScope: ApiKeyGroupPricingScope = {}
): ApiKeyGroupOption[] {
  return Object.entries(groupsRaw)
    .filter(([key]) => key !== AUTO_GROUP_VALUE)
    .map(([key, info]) => ({
      value: key,
      label: key,
      desc: info.desc || key,
      ratio:
        getLowestApiKeyGroupRatio(key, info.ratio, pricingScope) ?? info.ratio,
      hideDiscount: info.hide_discount === true,
    }))
}

export function shouldFallbackApiKeyGroup(
  group: string | undefined,
  options: ApiKeyGroupOption[]
): boolean {
  if (!group) return false
  return !options.some((option) => option.value === group)
}
