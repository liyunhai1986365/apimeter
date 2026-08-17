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
import {
  formatGroupDiscount,
  type GroupDiscountLabels,
} from '@/lib/group-discount'
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

const PAGE_WIDTH = 1684
const PAGE_HEIGHT = 1190
const PAGE_MARGIN = 80
const FIRST_TABLE_START = 374
const CONTINUATION_TABLE_START = 96
const TABLE_HEADER_HEIGHT = 48
const CONTENT_BOTTOM = 1072
const BASE_ROW_HEIGHT = 82
const SUPPLIER_LINE_HEIGHT = 60

const COLORS = {
  ink: '#171717',
  muted: '#6b7280',
  subtle: '#9ca3af',
  line: '#e5e7eb',
  paper: '#ffffff',
  soft: '#f7f7f5',
  accent: '#6d5dfc',
  accentSoft: '#f0edff',
  accentInk: '#4c3ed6',
} as const

const SANS_FONT =
  '"Public Sans", "PingFang SC", "Microsoft YaHei", "Hiragino Sans", "Yu Gothic", "Noto Sans", sans-serif'
const MONO_FONT = 'ui-monospace, "SFMono-Regular", Menlo, Consolas, monospace'

const CATEGORY_ORDER: ModelCategory[] = [
  MODEL_CATEGORIES.TEXT,
  MODEL_CATEGORIES.VECTOR,
  MODEL_CATEGORIES.IMAGE,
  MODEL_CATEGORIES.AUDIO,
  MODEL_CATEGORIES.VIDEO,
  MODEL_CATEGORIES.OTHER,
]

const CATEGORY_LABEL_KEYS: Record<ModelCategory, string> = {
  text: 'Text',
  vector: 'Vector',
  image: 'Image',
  audio: 'Audio',
  video: 'Video',
  other: 'Other',
}

type Translate = (
  key: string,
  values?: Record<string, string | number>
) => string

type UsableGroupMap = Record<string, string | { desc?: string; ratio?: number }>

export type SupplierDiscount = {
  group: string
  description: string
  ratio: number
  label: string
}

type SupplierPricingMode = 'full' | 'same-model' | 'same-segment'

export type QuotationRow = {
  key: string
  category: ModelCategory
  modelName: string
  vendorName: string
  vendorSortOrder: number
  modelSortOrder: number
  billingLabelKey: string
  scenario: string
  primaryLabelKey: string
  primaryPrice: string
  outputPrice: string
  cachePrice: string
  primaryUnitLabel: string
  outputUnitLabel: string
  cacheUnitLabel: string
  supplierDiscounts: SupplierDiscount[]
  supplierPricingMode: SupplierPricingMode
  inputModalities: Modality[]
  outputModalities: Modality[]
  isFirstModelRow: boolean
  requiresOnlineDetails: boolean
}

type QuotationBuildOptions = ModelCardPriceDisplayOptions & {
  usableGroup?: UsableGroupMap
  groupDisplay?: PricingGroupDisplayConfig
  discountLabels?: GroupDiscountLabels
}

export type QuotationPdfOptions = {
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

export type QuotationPdfResult = {
  filename: string
  pageCount: number
  rowCount: number
}

type LayoutItem = { row: QuotationRow; height: number }

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
      const configuredDescription =
        typeof usableInfo === 'string' ? usableInfo : usableInfo?.desc || ''
      const ratio = Number(model.group_ratio?.[group] ?? configuredRatio ?? 1)
      const safeRatio = Number.isFinite(ratio) ? ratio : 1
      return {
        group,
        description:
          configuredDescription ||
          (safeRatio === 1
            ? 'Standard-price supplier channel'
            : 'Discount supplier channel'),
        ratio: safeRatio,
        label:
          formatGroupDiscount(safeRatio, options.discountLabels) ||
          `${safeRatio * 100}%`,
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

function supplierPricingSignature(discounts: SupplierDiscount[]): string {
  return discounts
    .map(
      (discount) =>
        `${discount.group}\u0000${discount.description}\u0000${discount.ratio}`
    )
    .join('\u0001')
}

function mergeRepeatedSupplierPricing(rows: QuotationRow[]): QuotationRow[] {
  let previousRow:
    | {
        category: ModelCategory
        vendorName: string
        signature: string
      }
    | undefined

  for (const row of rows) {
    const signature = supplierPricingSignature(row.supplierDiscounts)
    const isRepeated = Boolean(
      signature &&
      previousRow &&
      previousRow.category === row.category &&
      previousRow.vendorName === row.vendorName &&
      previousRow.signature === signature
    )
    row.supplierPricingMode = isRepeated
      ? row.isFirstModelRow
        ? 'same-segment'
        : 'same-model'
      : 'full'
    previousRow = {
      category: row.category,
      vendorName: row.vendorName,
      signature,
    }
  }

  return rows
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
      hiddenDiscountGroups: getHiddenDiscountGroups(options.groupDisplay),
    })
    const scenarioEntries = getScenarioEntries(display.entries)
    const supplierDiscounts = getSupplierDiscounts(model, options)
    const shared = {
      category: resolveCategory(model),
      modelName: model.model_name,
      vendorName: model.vendor_name || 'Other Providers',
      vendorSortOrder: model.vendor_sort_order ?? Number.MAX_SAFE_INTEGER,
      modelSortOrder: model.sort_order ?? 0,
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
          primaryLabelKey: 'Official list price',
          primaryPrice: '-',
          outputPrice: '-',
          cachePrice: '-',
          primaryUnitLabel: display.unitLabel,
          outputUnitLabel: display.unitLabel,
          cacheUnitLabel: display.unitLabel,
          supplierPricingMode: 'full' as const,
          isFirstModelRow: true,
          requiresOnlineDetails: Boolean(display.rawExpression),
        },
      ]
    }

    const rows = Array.from(scenarioEntries.entries()).flatMap(
      ([scenario, entries], index) => {
        const primary =
          findEntry(entries, ['request', 'input', 'p']) || entries[0]
        const output = findEntry(entries, ['output', 'c'])
        const cache = findEntry(entries, ['cache', 'cr', 'cacheReadPrice'])
        const representedKeys = new Set(
          [primary?.key, output?.key, cache?.key].filter(Boolean)
        )
        const baseRow: QuotationRow = {
          ...shared,
          key: `${model.id}:${model.model_name}:${scenario || index}`,
          scenario,
          primaryLabelKey: primary?.labelKey || 'Official list price',
          primaryPrice: primary?.original || '-',
          outputPrice: output?.original || '-',
          cachePrice: cache?.original || '-',
          primaryUnitLabel: primary?.unitLabel || display.unitLabel,
          outputUnitLabel: output?.unitLabel || display.unitLabel,
          cacheUnitLabel: cache?.unitLabel || display.unitLabel,
          supplierDiscounts,
          supplierPricingMode: 'same-model',
          isFirstModelRow: false,
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
              primaryLabelKey: entry.labelKey || getSupplementaryLabel(entry),
              primaryPrice: entry.original,
              outputPrice: '-',
              cachePrice: '-',
              primaryUnitLabel: entry.unitLabel,
              outputUnitLabel: display.unitLabel,
              cacheUnitLabel: display.unitLabel,
              supplierDiscounts,
              supplierPricingMode: 'same-model',
              isFirstModelRow: false,
              requiresOnlineDetails: false,
            })
          )

        return [baseRow, ...supplementaryRows]
      }
    )

    if (rows[0]) rows[0].isFirstModelRow = true
    return rows
  })

  return mergeRepeatedSupplierPricing(rows)
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
  return `${safeSiteName || 'model-api'}-pricing-quotation${groupPart}-${datePart}.pdf`
}

export function normalizeQuotationLocale(locale: string): string {
  const normalized = locale
    .trim()
    .replace('_', '-')
    .replace(/^([a-zA-Z]{2,3})([A-Z]{2})$/, '$1-$2')
  try {
    return new Intl.Locale(normalized || 'en').toString()
  } catch {
    return 'en'
  }
}

function buildDocumentNumber(date: Date): string {
  const datePart = [
    date.getFullYear(),
    String(date.getMonth() + 1).padStart(2, '0'),
    String(date.getDate()).padStart(2, '0'),
  ].join('')
  const timePart = [
    String(date.getHours()).padStart(2, '0'),
    String(date.getMinutes()).padStart(2, '0'),
  ].join('')
  return `Q-${datePart}-${timePart}`
}

function rowHeight(row: QuotationRow): number {
  if (
    row.supplierPricingMode !== 'full' ||
    row.supplierDiscounts.length === 0
  ) {
    return BASE_ROW_HEIGHT
  }
  return Math.max(
    BASE_ROW_HEIGHT,
    18 + row.supplierDiscounts.length * SUPPLIER_LINE_HEIGHT
  )
}

function paginateRows(rows: QuotationRow[]): LayoutItem[][] {
  const pages: LayoutItem[][] = [[]]
  let pageIndex = 0
  let usedHeight = 0
  let visibleSupplierPricing = new Set<string>()

  const capacity = () =>
    CONTENT_BOTTOM -
    (pageIndex === 0 ? FIRST_TABLE_START : CONTINUATION_TABLE_START) -
    TABLE_HEADER_HEIGHT
  const newPage = () => {
    pages.push([])
    pageIndex += 1
    usedHeight = 0
    visibleSupplierPricing = new Set()
  }

  for (const row of rows) {
    const signature = supplierPricingSignature(row.supplierDiscounts)
    let pageRow = row
    if (
      row.supplierPricingMode !== 'full' &&
      signature &&
      !visibleSupplierPricing.has(signature)
    ) {
      pageRow = { ...row, supplierPricingMode: 'full' }
    }
    let requiredHeight = rowHeight(pageRow)

    if (usedHeight > 0 && usedHeight + requiredHeight > capacity()) {
      newPage()
      pageRow =
        row.supplierPricingMode !== 'full' && signature
          ? { ...row, supplierPricingMode: 'full' }
          : row
      requiredHeight = rowHeight(pageRow)
    }

    pages[pageIndex].push({ row: pageRow, height: requiredHeight })
    usedHeight += requiredHeight
    if (pageRow.supplierPricingMode === 'full' && signature) {
      visibleSupplierPricing.add(signature)
    }
  }

  return pages
}

function setFont(
  context: CanvasRenderingContext2D,
  size: number,
  weight: number,
  family = SANS_FONT
) {
  context.font = `${weight} ${size}px ${family}`
}

function roundRect(
  context: CanvasRenderingContext2D,
  x: number,
  y: number,
  width: number,
  height: number,
  radius: number,
  fill: string
) {
  context.beginPath()
  context.roundRect(x, y, width, height, radius)
  context.fillStyle = fill
  context.fill()
}

function fitText(
  context: CanvasRenderingContext2D,
  value: string,
  maxWidth: number
): string {
  if (context.measureText(value).width <= maxWidth) return value
  const characters = Array.from(value)
  while (characters.length > 0) {
    characters.pop()
    const candidate = `${characters.join('')}...`
    if (context.measureText(candidate).width <= maxWidth) return candidate
  }
  return '...'
}

function wrapText(
  context: CanvasRenderingContext2D,
  value: string,
  maxWidth: number,
  maxLines: number
): string[] {
  const characters = Array.from(value)
  const lines: string[] = []
  let current = ''
  for (const character of characters) {
    const candidate = current + character
    if (context.measureText(candidate).width <= maxWidth || !current) {
      current = candidate
      continue
    }
    lines.push(current)
    current = character
    if (lines.length === maxLines - 1) break
  }
  const consumedLength = lines.join('').length + current.length
  if (current) {
    lines.push(
      consumedLength < value.length
        ? fitText(context, `${current}${value.slice(consumedLength)}`, maxWidth)
        : current
    )
  }
  return lines.slice(0, maxLines)
}

function drawBrandMark(
  context: CanvasRenderingContext2D,
  siteName: string,
  x: number,
  y: number,
  size: number,
  dark = true
) {
  roundRect(
    context,
    x,
    y,
    size,
    size,
    size * 0.25,
    dark ? '#ffffff' : COLORS.ink
  )
  context.fillStyle = dark ? COLORS.ink : '#ffffff'
  context.textAlign = 'center'
  setFont(context, size * 0.44, 760)
  context.fillText(
    Array.from(siteName.trim())[0]?.toLocaleUpperCase() || 'A',
    x + size / 2,
    y + size * 0.7
  )
}

function drawFirstPageHeader(
  context: CanvasRenderingContext2D,
  options: QuotationPdfOptions,
  documentNumber: string,
  formattedDate: string
) {
  const { translate: t } = options
  const gradient = context.createLinearGradient(0, 0, PAGE_WIDTH, 220)
  gradient.addColorStop(0, '#151515')
  gradient.addColorStop(0.72, '#202020')
  gradient.addColorStop(1, '#312b5a')
  context.fillStyle = gradient
  context.fillRect(0, 0, PAGE_WIDTH, 220)
  context.fillStyle = COLORS.accent
  context.fillRect(0, 0, PAGE_WIDTH, 8)

  drawBrandMark(context, options.siteName, PAGE_MARGIN, 42, 58)
  context.textAlign = 'left'
  context.fillStyle = '#ffffff'
  setFont(context, 27, 720)
  context.fillText(options.siteName, PAGE_MARGIN + 80, 68)
  context.fillStyle = '#aaa5c8'
  setFont(context, 18, 580)
  context.fillText(
    t('Official prices and supplier discounts'),
    PAGE_MARGIN + 80,
    96
  )

  context.fillStyle = '#ffffff'
  setFont(context, 48, 760)
  context.fillText(t('Model API Pricing Quotation'), PAGE_MARGIN, 170)

  roundRect(context, 1130, 38, 474, 144, 18, 'rgba(255,255,255,0.08)')
  context.textAlign = 'left'
  context.fillStyle = '#aaa5c8'
  setFont(context, 17, 600)
  context.fillText(t('Quote no.'), 1160, 70)
  context.fillText(t('Generated on'), 1160, 129)
  context.fillStyle = '#ffffff'
  setFont(context, 24, 700, MONO_FONT)
  context.fillText(documentNumber, 1160, 100)
  setFont(context, 21, 620)
  context.fillText(fitText(context, formattedDate, 414), 1160, 159)
}

function drawContinuationHeader(
  context: CanvasRenderingContext2D,
  options: QuotationPdfOptions,
  documentNumber: string,
  pageNumber: number,
  pageCount: number
) {
  const { translate: t } = options
  context.fillStyle = COLORS.paper
  context.fillRect(0, 0, PAGE_WIDTH, 72)
  context.fillStyle = COLORS.accent
  context.fillRect(0, 0, PAGE_WIDTH, 6)
  context.strokeStyle = COLORS.line
  context.beginPath()
  context.moveTo(PAGE_MARGIN, 71)
  context.lineTo(PAGE_WIDTH - PAGE_MARGIN, 71)
  context.stroke()
  drawBrandMark(context, options.siteName, PAGE_MARGIN, 20, 34, false)
  context.textAlign = 'left'
  context.fillStyle = COLORS.ink
  setFont(context, 20, 720)
  context.fillText(options.siteName, PAGE_MARGIN + 48, 43)
  context.fillStyle = COLORS.muted
  setFont(context, 16, 560)
  context.fillText(t('Model API Pricing Quotation'), PAGE_MARGIN + 250, 43)
  context.textAlign = 'right'
  setFont(context, 15, 560, MONO_FONT)
  context.fillText(
    `${documentNumber}  |  ${t('Page {{current}} of {{total}}', { current: pageNumber, total: pageCount })}`,
    PAGE_WIDTH - PAGE_MARGIN,
    43
  )
}

function drawSummaryCards(
  context: CanvasRenderingContext2D,
  options: QuotationPdfOptions,
  manufacturerCount: number,
  categoryCount: number,
  supplierCount: number
) {
  const { translate: t } = options
  const y = 242
  const gap = 16
  const cardWidth = (PAGE_WIDTH - PAGE_MARGIN * 2 - gap * 3) / 4
  const cards = [
    [t('Models'), options.models.length.toLocaleString()],
    [t('Manufacturers'), manufacturerCount.toLocaleString()],
    [t('Categories'), categoryCount.toLocaleString()],
    [t('Supplier channels'), supplierCount.toLocaleString()],
  ]
  cards.forEach(([label, value], index) => {
    const x = PAGE_MARGIN + index * (cardWidth + gap)
    roundRect(context, x, y, cardWidth, 84, 16, COLORS.soft)
    context.textAlign = 'left'
    context.fillStyle = COLORS.muted
    setFont(context, 17, 620)
    context.fillText(label, x + 20, y + 28)
    context.fillStyle = COLORS.ink
    setFont(context, 31, 750, MONO_FONT)
    context.fillText(value, x + 20, y + 65)
  })
  context.fillStyle = COLORS.muted
  setFont(context, 16, 560)
  context.textAlign = 'left'
  const scope = options.hasActiveFilters
    ? t('Current filtered results')
    : t('All available models')
  const userGroup = options.userGroup
    ? `  |  ${t('User group')}: ${options.userGroup}`
    : ''
  context.fillText(
    fitText(
      context,
      `${t('Quote scope')}: ${scope}${userGroup}  |  ${t('Price basis')}: ${t('Official list price')} + ${t('Supplier discount information')} (${t('Discounted prices are not shown')})`,
      PAGE_WIDTH - PAGE_MARGIN * 2
    ),
    PAGE_MARGIN,
    355
  )
}

const TABLE_COLUMNS = [88, 134, 198, 200, 174, 350, 380] as const

function drawTableHeader(
  context: CanvasRenderingContext2D,
  options: QuotationPdfOptions,
  startY: number
) {
  const headers = [
    options.translate('Category'),
    options.translate('Manufacturer'),
    options.translate('Model'),
    options.translate('Modality'),
    options.translate('Billing / Scenario'),
    options.translate('Official standard price'),
    options.translate('Supplier information'),
  ]
  roundRect(
    context,
    PAGE_MARGIN,
    startY,
    PAGE_WIDTH - PAGE_MARGIN * 2,
    TABLE_HEADER_HEIGHT,
    12,
    COLORS.ink
  )
  let x = PAGE_MARGIN
  TABLE_COLUMNS.forEach((width, index) => {
    context.textAlign = 'left'
    context.fillStyle = '#ffffff'
    setFont(context, 17, 660)
    context.fillText(headers[index], x + 18, startY + 31)
    x += width
  })
}

function priceLines(row: QuotationRow) {
  const lines = [
    {
      labelKey: row.primaryLabelKey,
      value: row.primaryPrice,
      unit: row.primaryUnitLabel,
    },
  ]
  if (row.outputPrice !== '-') {
    lines.push({
      labelKey: 'Output',
      value: row.outputPrice,
      unit: row.outputUnitLabel,
    })
  }
  if (row.cachePrice !== '-') {
    lines.push({
      labelKey: 'Cache read',
      value: row.cachePrice,
      unit: row.cacheUnitLabel,
    })
  }
  return lines
}

function drawOfficialPrices(
  context: CanvasRenderingContext2D,
  options: QuotationPdfOptions,
  row: QuotationRow,
  x: number,
  y: number,
  width: number
) {
  if (row.requiresOnlineDetails) {
    context.fillStyle = COLORS.accentInk
    context.textAlign = 'left'
    setFont(context, 17, 650)
    context.fillText(options.translate('See online details'), x + 18, y + 42)
    return
  }
  const lines = priceLines(row)
  const lineHeight = 21
  const startY =
    y + Math.max(25, (rowHeight(row) - lines.length * lineHeight) / 2 + 16)
  lines.forEach((line, index) => {
    const lineY = startY + index * lineHeight
    context.textAlign = 'left'
    context.fillStyle = COLORS.muted
    setFont(context, 14, 580)
    context.fillText(
      fitText(context, options.translate(line.labelKey), 116),
      x + 18,
      lineY
    )
    context.textAlign = 'right'
    context.fillStyle = COLORS.ink
    setFont(context, 16, 700, MONO_FONT)
    const value = `${line.value}${line.unit ? ` /${line.unit}` : ''}`
    context.fillText(
      fitText(context, value, width - 160),
      x + width - 18,
      lineY
    )
  })
}

function drawSupplierInformation(
  context: CanvasRenderingContext2D,
  options: QuotationPdfOptions,
  row: QuotationRow,
  x: number,
  y: number,
  width: number
) {
  context.fillStyle = '#faf9ff'
  context.fillRect(x + 1, y, width - 1, rowHeight(row))

  if (row.supplierPricingMode !== 'full') {
    return
  }

  context.strokeStyle = COLORS.line
  context.beginPath()
  context.moveTo(x + 1, y)
  context.lineTo(x + width, y)
  context.stroke()

  if (row.supplierDiscounts.length === 0) {
    context.textAlign = 'left'
    context.fillStyle = COLORS.subtle
    setFont(context, 15, 560)
    context.fillText('-', x + 18, y + 42)
    return
  }

  const startY =
    y +
    Math.max(
      9,
      (rowHeight(row) - row.supplierDiscounts.length * SUPPLIER_LINE_HEIGHT) / 2
    )
  const innerLeft = x + 16
  const innerRight = x + width - 16
  const nameX = innerLeft + 13
  const discountValueWidth = 72
  const nameWidth = innerRight - discountValueWidth - nameX - 14

  row.supplierDiscounts.forEach((discount, index) => {
    const lineTop = startY + index * SUPPLIER_LINE_HEIGHT
    const titleY = lineTop + 18
    const descriptionY = lineTop + 36

    if (index > 0) {
      context.strokeStyle = '#ebe8fb'
      context.beginPath()
      context.moveTo(innerLeft, lineTop)
      context.lineTo(innerRight, lineTop)
      context.stroke()
    }

    context.fillStyle = discount.ratio < 1 ? COLORS.accent : COLORS.subtle
    context.beginPath()
    context.arc(innerLeft + 3, titleY - 5, 3, 0, Math.PI * 2)
    context.fill()

    context.textAlign = 'left'
    context.fillStyle = COLORS.ink
    setFont(context, 13, 700)
    context.fillText(fitText(context, discount.group, nameWidth), nameX, titleY)
    context.textAlign = 'right'
    context.fillStyle = discount.ratio < 1 ? COLORS.accentInk : COLORS.ink
    setFont(context, 12, 720, MONO_FONT)
    context.fillText(
      fitText(context, discount.label, discountValueWidth),
      innerRight,
      titleY
    )

    context.textAlign = 'left'
    context.fillStyle = COLORS.muted
    setFont(context, 11, 520)
    const descriptionLines = wrapText(
      context,
      discount.description ? options.translate(discount.description) : '-',
      innerRight - nameX,
      2
    )
    descriptionLines.forEach((line, lineIndex) => {
      context.fillText(line, nameX, descriptionY + lineIndex * 14)
    })
  })
}

function drawModalityIcon(
  context: CanvasRenderingContext2D,
  modality: Modality,
  centerX: number,
  centerY: number,
  size: number
) {
  const half = size / 2
  const left = centerX - half
  const top = centerY - half
  context.save()
  context.strokeStyle = '#4b5563'
  context.fillStyle = '#4b5563'
  context.lineWidth = Math.max(1.3, size * 0.1)
  context.lineCap = 'round'
  context.lineJoin = 'round'

  if (modality === 'text') {
    context.textAlign = 'center'
    context.textBaseline = 'middle'
    setFont(context, size * 0.88, 700, SANS_FONT)
    context.fillText('T', centerX, centerY + size * 0.03)
  } else if (modality === 'image') {
    context.strokeRect(left + 1, top + 2, size - 2, size - 4)
    context.beginPath()
    context.arc(
      left + size * 0.7,
      top + size * 0.34,
      size * 0.1,
      0,
      Math.PI * 2
    )
    context.stroke()
    context.beginPath()
    context.moveTo(left + size * 0.14, top + size * 0.78)
    context.lineTo(left + size * 0.42, top + size * 0.5)
    context.lineTo(left + size * 0.58, top + size * 0.65)
    context.lineTo(left + size * 0.75, top + size * 0.48)
    context.lineTo(left + size * 0.9, top + size * 0.72)
    context.stroke()
  } else if (modality === 'audio') {
    context.beginPath()
    context.roundRect(
      centerX - size * 0.2,
      top + size * 0.08,
      size * 0.4,
      size * 0.56,
      size * 0.2
    )
    context.stroke()
    context.beginPath()
    context.moveTo(left + size * 0.18, centerY)
    context.quadraticCurveTo(
      centerX,
      top + size * 0.92,
      left + size * 0.82,
      centerY
    )
    context.stroke()
    context.beginPath()
    context.moveTo(centerX, top + size * 0.8)
    context.lineTo(centerX, top + size)
    context.moveTo(left + size * 0.32, top + size)
    context.lineTo(left + size * 0.68, top + size)
    context.stroke()
  } else if (modality === 'video') {
    context.strokeRect(left + 1, top + size * 0.18, size * 0.64, size * 0.64)
    context.beginPath()
    context.moveTo(left + size * 0.65, top + size * 0.38)
    context.lineTo(left + size * 0.95, top + size * 0.24)
    context.lineTo(left + size * 0.95, top + size * 0.76)
    context.lineTo(left + size * 0.65, top + size * 0.62)
    context.closePath()
    context.stroke()
  } else {
    context.beginPath()
    context.moveTo(left + size * 0.18, top + 1)
    context.lineTo(left + size * 0.62, top + 1)
    context.lineTo(left + size * 0.86, top + size * 0.25)
    context.lineTo(left + size * 0.86, top + size - 1)
    context.lineTo(left + size * 0.18, top + size - 1)
    context.closePath()
    context.stroke()
    context.beginPath()
    context.moveTo(left + size * 0.62, top + 1)
    context.lineTo(left + size * 0.62, top + size * 0.25)
    context.lineTo(left + size * 0.86, top + size * 0.25)
    context.stroke()
  }
  context.restore()
}

function drawModalityFlow(
  context: CanvasRenderingContext2D,
  row: QuotationRow,
  x: number,
  y: number,
  width: number,
  height: number
) {
  if (!row.isFirstModelRow) {
    context.textAlign = 'left'
    context.fillStyle = COLORS.subtle
    setFont(context, 14, 560)
    context.fillText('-', x + 14, y + 44)
    return
  }

  const inputs = row.inputModalities
  const outputs = row.outputModalities
  const totalIcons = inputs.length + outputs.length
  if (totalIcons === 0) return

  const innerWidth = width - 24
  const arrowWidth = 18
  const slotWidth = Math.min(
    22,
    Math.floor((innerWidth - arrowWidth) / Math.max(totalIcons, 1))
  )
  const iconSize = Math.max(12, slotWidth - 4)
  const flowWidth = totalIcons * slotWidth + arrowWidth
  let cursorX = x + (width - flowWidth) / 2 + slotWidth / 2
  const centerY = y + height / 2

  inputs.forEach((modality) => {
    drawModalityIcon(context, modality, cursorX, centerY, iconSize)
    cursorX += slotWidth
  })

  const arrowStart = cursorX - slotWidth / 2 + 3
  const arrowEnd = arrowStart + arrowWidth - 6
  context.strokeStyle = COLORS.subtle
  context.lineWidth = 1.6
  context.lineCap = 'round'
  context.beginPath()
  context.moveTo(arrowStart, centerY)
  context.lineTo(arrowEnd, centerY)
  context.lineTo(arrowEnd - 4, centerY - 4)
  context.moveTo(arrowEnd, centerY)
  context.lineTo(arrowEnd - 4, centerY + 4)
  context.stroke()
  cursorX += arrowWidth

  outputs.forEach((modality) => {
    drawModalityIcon(context, modality, cursorX, centerY, iconSize)
    cursorX += slotWidth
  })
}

function drawQuotationRow(
  context: CanvasRenderingContext2D,
  options: QuotationPdfOptions,
  row: QuotationRow,
  y: number,
  height: number,
  rowIndex: number
) {
  context.fillStyle = rowIndex % 2 === 0 ? COLORS.paper : COLORS.soft
  context.fillRect(PAGE_MARGIN, y, PAGE_WIDTH - PAGE_MARGIN * 2, height)
  context.strokeStyle = COLORS.line
  context.beginPath()
  context.moveTo(PAGE_MARGIN, y + height)
  context.lineTo(PAGE_WIDTH - PAGE_MARGIN, y + height)
  context.stroke()

  let dividerX = PAGE_MARGIN
  TABLE_COLUMNS.slice(0, -1).forEach((width) => {
    dividerX += width
    context.strokeStyle = '#eeeeec'
    context.beginPath()
    context.moveTo(dividerX, y + 10)
    context.lineTo(dividerX, y + height - 10)
    context.stroke()
  })

  let x = PAGE_MARGIN
  context.textAlign = 'left'

  if (row.isFirstModelRow) {
    const categoryLabel = options.translate(CATEGORY_LABEL_KEYS[row.category])
    setFont(context, 13, 700)
    const categoryWidth = Math.min(
      TABLE_COLUMNS[0] - 22,
      context.measureText(categoryLabel).width + 22
    )
    roundRect(
      context,
      x + 11,
      y + height / 2 - 16,
      categoryWidth,
      32,
      9,
      COLORS.accentSoft
    )
    context.fillStyle = COLORS.accentInk
    context.fillText(categoryLabel, x + 22, y + height / 2 + 5)
  } else {
    context.fillStyle = COLORS.subtle
    setFont(context, 14, 560)
    context.fillText('-', x + 18, y + 44)
  }

  x += TABLE_COLUMNS[0]
  if (row.isFirstModelRow) {
    context.fillStyle = COLORS.ink
    setFont(context, 14, 650)
    const vendorLines = wrapText(
      context,
      row.vendorName,
      TABLE_COLUMNS[1] - 28,
      2
    )
    const vendorStart =
      y + Math.max(28, (height - vendorLines.length * 19) / 2 + 15)
    vendorLines.forEach((line, index) => {
      context.fillText(line, x + 14, vendorStart + index * 19)
    })
  } else {
    context.fillStyle = COLORS.subtle
    setFont(context, 14, 560)
    context.fillText('-', x + 14, y + 44)
  }

  x += TABLE_COLUMNS[1]
  if (row.isFirstModelRow) {
    context.fillStyle = COLORS.ink
    setFont(context, 15, 700, MONO_FONT)
    const modelLines = wrapText(
      context,
      row.modelName,
      TABLE_COLUMNS[2] - 28,
      2
    )
    const modelStart =
      y + Math.max(28, (height - modelLines.length * 20) / 2 + 15)
    modelLines.forEach((line, index) => {
      context.fillText(line, x + 14, modelStart + index * 20)
    })
  } else {
    context.fillStyle = COLORS.subtle
    setFont(context, 12, 580)
    context.fillText(
      options.translate('Additional pricing tier'),
      x + 14,
      y + 44
    )
  }

  x += TABLE_COLUMNS[2]
  drawModalityFlow(context, row, x, y, TABLE_COLUMNS[3], height)

  x += TABLE_COLUMNS[3]
  context.fillStyle = COLORS.ink
  setFont(context, 14, 650)
  context.fillText(
    fitText(
      context,
      options.translate(row.billingLabelKey),
      TABLE_COLUMNS[4] - 28
    ),
    x + 14,
    y + (row.scenario ? 34 : 45)
  )
  if (row.scenario) {
    context.fillStyle = COLORS.muted
    setFont(context, 12, 520)
    context.fillText(
      fitText(context, row.scenario, TABLE_COLUMNS[4] - 28),
      x + 14,
      y + 56
    )
  }

  x += TABLE_COLUMNS[4]
  drawOfficialPrices(context, options, row, x, y, TABLE_COLUMNS[5])
  x += TABLE_COLUMNS[5]
  drawSupplierInformation(context, options, row, x, y, TABLE_COLUMNS[6])
}

function drawLayoutItems(
  context: CanvasRenderingContext2D,
  options: QuotationPdfOptions,
  items: LayoutItem[],
  startY: number
) {
  let y = startY + TABLE_HEADER_HEIGHT
  let rowIndex = 0
  for (const item of items) {
    drawQuotationRow(context, options, item.row, y, item.height, rowIndex)
    rowIndex += 1
    y += item.height
  }
}

function drawFooter(
  context: CanvasRenderingContext2D,
  options: QuotationPdfOptions,
  pageNumber: number,
  pageCount: number
) {
  const { translate: t } = options
  const footerY = 1098
  context.strokeStyle = COLORS.line
  context.beginPath()
  context.moveTo(PAGE_MARGIN, footerY)
  context.lineTo(PAGE_WIDTH - PAGE_MARGIN, footerY)
  context.stroke()
  context.textAlign = 'left'
  context.fillStyle = COLORS.muted
  setFont(context, 14, 520)
  const disclaimer = wrapText(
    context,
    t(
      'Reference quotation only. Official prices and supplier discounts may change. Discounted prices are not shown; the online price at the time of use prevails.'
    ),
    1120,
    2
  )
  disclaimer.forEach((line, index) => {
    context.fillText(line, PAGE_MARGIN, footerY + 25 + index * 20)
  })
  context.fillStyle = COLORS.subtle
  setFont(context, 13, 500)
  context.fillText(
    `${t('Source')}: ${fitText(context, options.sourceUrl, 900)}`,
    PAGE_MARGIN,
    footerY + 70
  )
  context.textAlign = 'right'
  context.fillStyle = COLORS.ink
  setFont(context, 15, 650)
  context.fillText(
    t('Page {{current}} of {{total}}', {
      current: pageNumber,
      total: pageCount,
    }),
    PAGE_WIDTH - PAGE_MARGIN,
    footerY + 42
  )
}

function drawPage(
  options: QuotationPdfOptions,
  items: LayoutItem[],
  documentNumber: string,
  formattedDate: string,
  manufacturerCount: number,
  categoryCount: number,
  supplierCount: number,
  pageNumber: number,
  pageCount: number
): HTMLCanvasElement {
  const canvas = document.createElement('canvas')
  canvas.width = PAGE_WIDTH
  canvas.height = PAGE_HEIGHT
  const context = canvas.getContext('2d')
  if (!context) throw new Error('Unable to initialize quotation canvas')
  context.fillStyle = COLORS.paper
  context.fillRect(0, 0, PAGE_WIDTH, PAGE_HEIGHT)
  context.textBaseline = 'alphabetic'

  const isFirstPage = pageNumber === 1
  if (isFirstPage) {
    drawFirstPageHeader(context, options, documentNumber, formattedDate)
    drawSummaryCards(
      context,
      options,
      manufacturerCount,
      categoryCount,
      supplierCount
    )
  } else {
    drawContinuationHeader(
      context,
      options,
      documentNumber,
      pageNumber,
      pageCount
    )
  }
  const tableStart = isFirstPage ? FIRST_TABLE_START : CONTINUATION_TABLE_START
  drawTableHeader(context, options, tableStart)
  drawLayoutItems(context, options, items, tableStart)
  drawFooter(context, options, pageNumber, pageCount)
  return canvas
}

function savePdfBlob(blob: Blob, filename: string) {
  const objectUrl = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = objectUrl
  link.download = filename
  link.hidden = true
  document.body.appendChild(link)
  link.click()
  link.remove()
  window.setTimeout(() => URL.revokeObjectURL(objectUrl), 1000)
}

export async function downloadQuotationPdf(
  options: QuotationPdfOptions
): Promise<QuotationPdfResult> {
  const generatedAt = options.generatedAt || new Date()
  const rows = buildQuotationRows(options.models, {
    tokenUnit: options.tokenUnit,
    showRechargePrice: false,
    priceRate: options.priceRate,
    usdExchangeRate: options.usdExchangeRate,
    usableGroup: options.usableGroup,
    groupDisplay: options.groupDisplay,
    discountLabels: options.discountLabels,
  })
  const pages = paginateRows(rows)
  const manufacturerCount = new Set(
    options.models.map((model) => model.vendor_name).filter(Boolean)
  ).size
  const categoryCount = new Set(options.models.map(resolveCategory)).size
  const supplierCount = new Set(
    rows.flatMap((row) => row.supplierDiscounts.map((item) => item.group))
  ).size
  const formattedDate = new Intl.DateTimeFormat(
    normalizeQuotationLocale(options.locale),
    {
      year: 'numeric',
      month: 'short',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    }
  ).format(generatedAt)
  const documentNumber = buildDocumentNumber(generatedAt)
  const filename = buildQuotationFilename(
    options.siteName,
    generatedAt,
    options.userGroup
  )
  const { jsPDF } = await import('jspdf')
  const pdf = new jsPDF({
    orientation: 'landscape',
    unit: 'mm',
    format: 'a4',
    compress: true,
  })

  pages.forEach((pageItems, index) => {
    if (index > 0) pdf.addPage('a4', 'landscape')
    const canvas = drawPage(
      options,
      pageItems,
      documentNumber,
      formattedDate,
      manufacturerCount,
      categoryCount,
      supplierCount,
      index + 1,
      pages.length
    )
    pdf.addImage(
      canvas.toDataURL('image/jpeg', 0.94),
      'JPEG',
      0,
      0,
      297,
      210,
      undefined,
      'FAST'
    )
  })

  savePdfBlob(pdf.output('blob'), filename)
  return { filename, pageCount: pages.length, rowCount: rows.length }
}
