/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { GroupDiscountLabels } from '@/lib/group-discount'
import { MODEL_CATEGORIES } from '../constants'
import type {
  Modality,
  ModelCategory,
  PricingGroupDisplayConfig,
  PricingModel,
  TokenUnit,
} from '../types'
import { getHiddenDiscountGroups, isGroupDiscountHidden } from './group-display'
import {
  buildModelCardPriceDisplay,
  type ModelCardPriceDisplayOptions,
  type ModelCardPriceEntry,
} from './model-card-price'
import { inferModelMetadata } from './model-metadata'

const CATEGORY_ORDER: ModelCategory[] = [
  MODEL_CATEGORIES.TEXT,
  MODEL_CATEGORIES.VECTOR,
  MODEL_CATEGORIES.IMAGE,
  MODEL_CATEGORIES.AUDIO,
  MODEL_CATEGORIES.VIDEO,
  MODEL_CATEGORIES.OTHER,
]

type Translate = (
  key: string,
  values?: Record<string, string | number>
) => string

type UsableGroupMap = Record<string, string | { desc?: string; ratio?: number }>

export type SupplierDiscount = {
  group: string
  ratio: number
}

export type QuotationRow = {
  key: string
  category: ModelCategory
  modelName: string
  vendorName: string
  billingLabelKey: string
  scenario: string
  primaryPrice: string
  outputPrice: string
  cacheWritePrice: string
  cacheWrite1hPrice: string
  cachePrice: string
  primaryUnitLabel: string
  outputUnitLabel: string
  cacheWriteUnitLabel: string
  cacheWrite1hUnitLabel: string
  cacheUnitLabel: string
  supplierDiscounts: SupplierDiscount[]
  inputModalities: Modality[]
  outputModalities: Modality[]
  requiresOnlineDetails: boolean
}

type QuotationBuildOptions = ModelCardPriceDisplayOptions & {
  usableGroup?: UsableGroupMap
  groupDisplay?: PricingGroupDisplayConfig
  discountLabels?: GroupDiscountLabels
}

export type QuotationOptions = {
  models: PricingModel[]
  siteName: string
  tokenUnit: TokenUnit
  priceRate?: number
  usdExchangeRate?: number
  locale: string
  hasActiveFilters: boolean
  sourceUrl: string
  usableGroup: UsableGroupMap
  groupDisplay?: PricingGroupDisplayConfig
  discountLabels: GroupDiscountLabels
  userGroup?: string
  translate: Translate
  generatedAt?: Date
}

function findEntry(
  entries: ModelCardPriceEntry[],
  keys: string[]
): ModelCardPriceEntry | undefined {
  return entries.find((entry) => keys.includes(entry.key))
}

function getScenarioEntries(entries: ModelCardPriceEntry[]) {
  const scenarios = new Map<string, ModelCardPriceEntry[]>()
  for (const entry of entries) {
    const scenario = entry.specLabel || ''
    const current = scenarios.get(scenario) || []
    current.push(entry)
    scenarios.set(scenario, current)
  }
  return scenarios
}

function getSupplementaryLabel(entry: ModelCardPriceEntry): string {
  if (entry.labelKey) return entry.labelKey
  if (entry.key === 'request') return 'Request'
  if (entry.key === 'duration') return 'Per second'
  return entry.key
}

function resolveCategory(model: PricingModel): ModelCategory {
  return model.category && CATEGORY_ORDER.includes(model.category)
    ? model.category
    : MODEL_CATEGORIES.TEXT
}

function groupOrderMap(groupDisplay?: PricingGroupDisplayConfig) {
  const categories = [...(groupDisplay?.categories || [])].sort((a, b) => {
    if (a.order !== b.order) return a.order - b.order
    return a.name.localeCompare(b.name)
  })
  const categoryOrder = new Map(
    categories.map((category, index) => [category.id, index])
  )
  return new Map(
    (groupDisplay?.groups || []).map((item) => [
      item.group,
      [
        categoryOrder.get(item.category_id) ?? Number.MAX_SAFE_INTEGER,
        item.order,
      ] as const,
    ])
  )
}

function getSupplierDiscounts(
  model: PricingModel,
  options: QuotationBuildOptions
): SupplierDiscount[] {
  const usableGroups = Object.keys(options.usableGroup || {}).filter(
    (group) => group && group !== 'auto'
  )
  const enabledGroups = Array.isArray(model.enable_groups)
    ? model.enable_groups
    : []
  const groups = enabledGroups.includes('all')
    ? usableGroups
    : enabledGroups.filter((group) => group && group !== 'auto')
  const configuredOrder = groupOrderMap(options.groupDisplay)

  return [...new Set(groups)]
    .filter((group) => !isGroupDiscountHidden(group, options.groupDisplay))
    .filter(
      (group) =>
        usableGroups.length === 0 ||
        usableGroups.includes(group) ||
        Object.hasOwn(model.group_ratio || {}, group)
    )
    .map((group) => {
      const usableInfo = options.usableGroup?.[group]
      const configuredRatio =
        typeof usableInfo === 'object' ? usableInfo.ratio : undefined
      const ratio = Number(model.group_ratio?.[group] ?? configuredRatio ?? 1)
      return {
        group,
        ratio: Number.isFinite(ratio) ? ratio : 1,
      }
    })
    .sort((a, b) => {
      const aOrder = configuredOrder.get(a.group)
      const bOrder = configuredOrder.get(b.group)
      if (aOrder && bOrder) {
        if (aOrder[0] !== bOrder[0]) return aOrder[0] - bOrder[0]
        if (aOrder[1] !== bOrder[1]) return aOrder[1] - bOrder[1]
      } else if (aOrder || bOrder) {
        return aOrder ? -1 : 1
      }
      return a.group.localeCompare(b.group)
    })
}

function compareQuotationModels(a: PricingModel, b: PricingModel): number {
  const categoryDelta =
    CATEGORY_ORDER.indexOf(resolveCategory(a)) -
    CATEGORY_ORDER.indexOf(resolveCategory(b))
  if (categoryDelta !== 0) return categoryDelta

  const vendorOrderDelta =
    (a.vendor_sort_order ?? Number.MAX_SAFE_INTEGER) -
    (b.vendor_sort_order ?? Number.MAX_SAFE_INTEGER)
  if (vendorOrderDelta !== 0) return vendorOrderDelta

  const vendorDelta = (a.vendor_name || '').localeCompare(b.vendor_name || '')
  if (vendorDelta !== 0) return vendorDelta

  const modelOrderDelta = (a.sort_order ?? 0) - (b.sort_order ?? 0)
  if (modelOrderDelta !== 0) return modelOrderDelta
  return a.model_name.localeCompare(b.model_name)
}

export function buildQuotationRows(
  models: PricingModel[],
  options: QuotationBuildOptions = {}
): QuotationRow[] {
  const rows = [...models].sort(compareQuotationModels).flatMap((model) => {
    const metadata = inferModelMetadata(model)
    const display = buildModelCardPriceDisplay(model, {
      ...options,
      includeAllDynamicTiers: true,
      includeCacheWritePrices: true,
      hiddenDiscountGroups: getHiddenDiscountGroups(options.groupDisplay),
    })
    const scenarioEntries = getScenarioEntries(display.entries)
    const supplierDiscounts = getSupplierDiscounts(model, options)
    const shared = {
      category: resolveCategory(model),
      modelName: model.model_name,
      vendorName: model.vendor_name || 'Other Providers',
      billingLabelKey: display.billingLabelKey,
      supplierDiscounts,
      inputModalities: metadata.input_modalities,
      outputModalities: metadata.output_modalities,
    }

    if (scenarioEntries.size === 0) {
      return [
        {
          ...shared,
          key: `${model.id}:${model.model_name}`,
          scenario: '',
          primaryPrice: '-',
          outputPrice: '-',
          cacheWritePrice: '-',
          cacheWrite1hPrice: '-',
          cachePrice: '-',
          primaryUnitLabel: display.unitLabel,
          outputUnitLabel: display.unitLabel,
          cacheWriteUnitLabel: display.unitLabel,
          cacheWrite1hUnitLabel: display.unitLabel,
          cacheUnitLabel: display.unitLabel,
          requiresOnlineDetails: Boolean(display.rawExpression),
        },
      ]
    }

    const rows = Array.from(scenarioEntries.entries()).flatMap(
      ([scenario, entries], index) => {
        const primary =
          findEntry(entries, ['request', 'input', 'p']) || entries[0]
        const output = findEntry(entries, ['output', 'c'])
        const cacheWrite = findEntry(entries, ['create_cache', 'cc'])
        const cacheWrite1h = findEntry(entries, ['create_cache_1h', 'cc1h'])
        const cache = findEntry(entries, ['cache', 'cr', 'cacheReadPrice'])
        const representedKeys = new Set(
          [
            primary?.key,
            output?.key,
            cacheWrite?.key,
            cacheWrite1h?.key,
            cache?.key,
          ].filter(Boolean)
        )
        const baseRow: QuotationRow = {
          ...shared,
          key: `${model.id}:${model.model_name}:${scenario || index}`,
          scenario,
          primaryPrice: primary?.original || '-',
          outputPrice: output?.original || '-',
          cacheWritePrice: cacheWrite?.original || '-',
          cacheWrite1hPrice: cacheWrite1h?.original || '-',
          cachePrice: cache?.original || '-',
          primaryUnitLabel: primary?.unitLabel || display.unitLabel,
          outputUnitLabel: output?.unitLabel || display.unitLabel,
          cacheWriteUnitLabel: cacheWrite?.unitLabel || display.unitLabel,
          cacheWrite1hUnitLabel: cacheWrite1h?.unitLabel || display.unitLabel,
          cacheUnitLabel: cache?.unitLabel || display.unitLabel,
          supplierDiscounts,
          requiresOnlineDetails: false,
        }
        const supplementaryRows = entries
          .filter((entry) => !representedKeys.has(entry.key))
          .map(
            (entry): QuotationRow => ({
              ...shared,
              key: `${baseRow.key}:${entry.key}`,
              scenario: [scenario, getSupplementaryLabel(entry)]
                .filter(Boolean)
                .join(' - '),
              primaryPrice: entry.original,
              outputPrice: '-',
              cacheWritePrice: '-',
              cacheWrite1hPrice: '-',
              cachePrice: '-',
              primaryUnitLabel: entry.unitLabel,
              outputUnitLabel: display.unitLabel,
              cacheWriteUnitLabel: display.unitLabel,
              cacheWrite1hUnitLabel: display.unitLabel,
              cacheUnitLabel: display.unitLabel,
              supplierDiscounts,
              requiresOnlineDetails: false,
            })
          )

        return [baseRow, ...supplementaryRows]
      }
    )

    return rows
  })

  return rows
}

function safeFilenameSegment(value: string): string {
  return value
    .trim()
    .replace(/[^\p{L}\p{N}]+/gu, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48)
}

export function buildQuotationFilename(
  siteName: string,
  date: Date,
  userGroup?: string
): string {
  const safeSiteName = safeFilenameSegment(siteName)
  const safeUserGroup = safeFilenameSegment(userGroup || '')
  const datePart = [
    date.getFullYear(),
    String(date.getMonth() + 1).padStart(2, '0'),
    String(date.getDate()).padStart(2, '0'),
  ].join('-')
  const groupPart = safeUserGroup ? `-${safeUserGroup}` : ''
  return `${safeSiteName || 'model-api'}-pricing-quotation${groupPart}-${datePart}.xlsx`
}
