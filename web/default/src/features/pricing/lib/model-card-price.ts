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
import {
  getLowestGroupDiscountSummary,
  type GroupDiscountLabels,
} from '@/lib/group-discount'
import { DEFAULT_TOKEN_UNIT } from '../constants'
import type { PricingModel, TokenUnit } from '../types'
import {
  getDynamicPriceEntries,
  getDynamicPricingTiers,
  hasDynamicRequestRules,
  isDynamicPricingModel,
} from './dynamic-price'
import { isTokenBasedModel } from './model-helpers'
import {
  formatFixedPrice,
  formatGroupPrice,
  formatPrice,
  formatRequestPrice,
} from './price'

export type ModelCardPriceKind = 'token' | 'request' | 'dynamic' | 'special'

export type ModelCardPriceEntry = {
  key: string
  labelKey: string
  original: string
  current: string
  unitLabel: string
  specLabel?: string
  featured?: boolean
}

export type ModelCardPriceDisplay = {
  kind: ModelCardPriceKind
  billingLabelKey: string
  unitLabel: string
  isFree: boolean
  discountLabel?: string
  hasDiscount: boolean
  rawExpression?: string
  entries: ModelCardPriceEntry[]
}

export type ModelCardPriceDisplayOptions = {
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  priceRate?: number
  usdExchangeRate?: number
  discountLabels?: GroupDiscountLabels
  includeAllDynamicTiers?: boolean
}

const ORIGINAL_GROUP_KEY = '__original__'
const MODEL_CARD_HIDDEN_DYNAMIC_FIELDS = new Set([
  'cacheCreatePrice',
  'cacheCreate1hPrice',
])

function getEnabledGroups(model: PricingModel): string[] {
  return Array.isArray(model.enable_groups) ? model.enable_groups : []
}

function getGroupRatios(model: PricingModel): Record<string, number> {
  return model.group_ratio || {}
}

function getLowestEnabledGroupRatio(model: PricingModel): number {
  const groups = getEnabledGroups(model)
  const ratios = getGroupRatios(model)
  if (groups.length === 0) return 1

  let minRatio = Number.POSITIVE_INFINITY
  for (const group of groups) {
    const ratio = ratios[group]
    if (Number.isFinite(ratio) && ratio < minRatio) {
      minRatio = ratio
    }
  }

  return minRatio === Number.POSITIVE_INFINITY ? 1 : minRatio
}

function getDiscountLabel(
  model: PricingModel,
  labels: GroupDiscountLabels | undefined
): string | undefined {
  if (getLowestEnabledGroupRatio(model) >= 1) return undefined

  return getLowestGroupDiscountSummary(
    getEnabledGroups(model),
    getGroupRatios(model),
    labels
  )
}

function tokenUnitLabel(tokenUnit: TokenUnit): string {
  return tokenUnit === 'K' ? '1K' : '1M'
}

function commonOptions(options: ModelCardPriceDisplayOptions) {
  return {
    tokenUnit: options.tokenUnit ?? DEFAULT_TOKEN_UNIT,
    showRechargePrice: options.showRechargePrice ?? false,
    priceRate: options.priceRate ?? 1,
    usdExchangeRate: options.usdExchangeRate ?? 1,
  }
}

function isTokenDisplayFree(model: PricingModel, ratio = 1): boolean {
  const modelRatio = Number(model.model_ratio)
  const groupRatio = Number(ratio)
  return (
    Number.isFinite(modelRatio) &&
    Number.isFinite(groupRatio) &&
    modelRatio * groupRatio <= 0
  )
}

function isRequestDisplayFree(model: PricingModel, ratio = 1): boolean {
  const modelPrice = Number(model.model_price ?? 0)
  const groupRatio = Number(ratio)
  return (
    Number.isFinite(modelPrice) &&
    Number.isFinite(groupRatio) &&
    modelPrice * groupRatio <= 0
  )
}

export function isModelPriceFreeForRatio(
  model: PricingModel,
  ratio = 1
): boolean {
  return isTokenBasedModel(model)
    ? isTokenDisplayFree(model, ratio)
    : isRequestDisplayFree(model, ratio)
}

function buildDynamicDisplay(
  model: PricingModel,
  options: ModelCardPriceDisplayOptions
): ModelCardPriceDisplay {
  const shared = commonOptions(options)
  const unitLabel = tokenUnitLabel(shared.tokenUnit)
  const tiers = getDynamicPricingTiers(model)
  const tier = tiers[0] || null
  const displayTiers = options.includeAllDynamicTiers
    ? tiers
    : [tier].filter(Boolean)
  const discountRatio = getLowestEnabledGroupRatio(model)
  const originalEntries = displayTiers.flatMap((item) =>
    getDynamicPriceEntries(item, {
      ...shared,
      groupRatioMultiplier: 1,
    })
      .filter((entry) => !MODEL_CARD_HIDDEN_DYNAMIC_FIELDS.has(entry.field))
      .map((entry) => ({ ...entry, specLabel: item.label }))
  )
  const currentEntries = displayTiers.flatMap((item) =>
    getDynamicPriceEntries(item, {
      ...shared,
      groupRatioMultiplier: discountRatio,
    })
      .filter((entry) => !MODEL_CARD_HIDDEN_DYNAMIC_FIELDS.has(entry.field))
      .map((entry) => ({ ...entry, specLabel: item.label }))
  )
  const currentByKey = new Map(
    currentEntries.map((entry) => [
      `${entry.key}:${entry.specLabel || ''}`,
      entry,
    ])
  )
  const fallbackOriginalEntries = getDynamicPriceEntries(tier, {
    ...shared,
    groupRatioMultiplier: 1,
  })

  if (
    fallbackOriginalEntries.length === 0 &&
    (model.billing_expr || '').trim()
  ) {
    const hasDiscount = discountRatio < 1

    return {
      kind: 'special',
      billingLabelKey: hasDynamicRequestRules(model)
        ? 'Dynamic Pricing'
        : 'Special billing expression',
      unitLabel,
      isFree: false,
      discountLabel: getDiscountLabel(model, options.discountLabels),
      hasDiscount,
      rawExpression: model.billing_expr,
      entries: [],
    }
  }

  const hasDiscount = discountRatio < 1

  return {
    kind: 'dynamic',
    billingLabelKey: 'Dynamic Pricing',
    unitLabel,
    isFree: false,
    discountLabel: getDiscountLabel(model, options.discountLabels),
    hasDiscount,
    entries: originalEntries.map((entry, index) => {
      const current = currentByKey.get(`${entry.key}:${entry.specLabel || ''}`)
      return {
        key: entry.key,
        labelKey: entry.shortLabel,
        original: entry.formatted,
        current: current?.formatted ?? entry.formatted,
        unitLabel: entry.variable.unit === 'second' ? 'second' : unitLabel,
        specLabel: entry.specLabel,
        featured: index < 2,
      }
    }),
  }
}

function buildTokenDisplay(
  model: PricingModel,
  options: ModelCardPriceDisplayOptions
): ModelCardPriceDisplay {
  const shared = commonOptions(options)
  const unitLabel = tokenUnitLabel(shared.tokenUnit)
  const originalRatios = { [ORIGINAL_GROUP_KEY]: 1 }
  const isFree = isModelPriceFreeForRatio(
    model,
    getLowestEnabledGroupRatio(model)
  )
  const hasDiscount = !isFree && getLowestEnabledGroupRatio(model) < 1
  const args = [
    shared.tokenUnit,
    shared.showRechargePrice,
    shared.priceRate,
    shared.usdExchangeRate,
  ] as const

  const entries: ModelCardPriceEntry[] = [
    {
      key: 'input',
      labelKey: 'Input',
      original: formatGroupPrice(
        model,
        ORIGINAL_GROUP_KEY,
        'input',
        ...args,
        originalRatios
      ),
      current: formatPrice(model, 'input', ...args),
      unitLabel,
      featured: true,
    },
    {
      key: 'output',
      labelKey: 'Output',
      original: formatGroupPrice(
        model,
        ORIGINAL_GROUP_KEY,
        'output',
        ...args,
        originalRatios
      ),
      current: formatPrice(model, 'output', ...args),
      unitLabel,
      featured: true,
    },
  ]

  if (model.cache_ratio != null) {
    entries.push({
      key: 'cache',
      labelKey: 'Cached',
      original: formatGroupPrice(
        model,
        ORIGINAL_GROUP_KEY,
        'cache',
        ...args,
        originalRatios
      ),
      current: formatPrice(model, 'cache', ...args),
      unitLabel,
    })
  }
  return {
    kind: 'token',
    billingLabelKey: 'Token-based',
    unitLabel,
    isFree,
    discountLabel: isFree
      ? undefined
      : getDiscountLabel(model, options.discountLabels),
    hasDiscount,
    entries,
  }
}

function buildRequestDisplay(
  model: PricingModel,
  options: ModelCardPriceDisplayOptions
): ModelCardPriceDisplay {
  const shared = commonOptions(options)
  const originalRatios = { [ORIGINAL_GROUP_KEY]: 1 }
  const isFree = isModelPriceFreeForRatio(
    model,
    getLowestEnabledGroupRatio(model)
  )
  const hasDiscount = !isFree && getLowestEnabledGroupRatio(model) < 1
  const args = [
    shared.showRechargePrice,
    shared.priceRate,
    shared.usdExchangeRate,
  ] as const

  return {
    kind: 'request',
    billingLabelKey: 'Per Request',
    unitLabel: 'request',
    isFree,
    discountLabel: isFree
      ? undefined
      : getDiscountLabel(model, options.discountLabels),
    hasDiscount,
    entries: [
      {
        key: 'request',
        labelKey: 'Request',
        original: formatFixedPrice(
          model,
          ORIGINAL_GROUP_KEY,
          ...args,
          originalRatios
        ),
        current: formatRequestPrice(model, ...args),
        unitLabel: 'request',
        featured: true,
      },
    ],
  }
}

export function buildModelCardPriceDisplay(
  model: PricingModel,
  options: ModelCardPriceDisplayOptions = {}
): ModelCardPriceDisplay {
  if (isDynamicPricingModel(model)) {
    return buildDynamicDisplay(model, options)
  }

  if (isTokenBasedModel(model)) {
    return buildTokenDisplay(model, options)
  }

  return buildRequestDisplay(model, options)
}
