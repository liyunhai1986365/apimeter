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
import type { UsageLog } from '../data/schema'
import type { LogOtherData, ToolSurchargeLogItem } from '../types'
import { getTieredBillingSummary } from './format'

export type BillingDetailMode = 'per-token' | 'per-call' | 'dynamic'

export interface BillingDetailFactor {
  labelKey: string
  value: number
}

export interface BillingDetailLine {
  key: string
  labelKey: string
  quantity?: number
  divisor?: 1 | 1000 | 1000000
  quantityUnitKey?: string
  unitPriceUSD?: number
  factors: BillingDetailFactor[]
  originalAmountUSD: number
  finalAmountUSD: number
  amountOnly?: boolean
}

export interface BillingDetail {
  mode: BillingDetailMode
  discount: number
  discountLabelKey: string
  originalAmountUSD: number
  finalAmountUSD: number
  matchedTier?: string
  lines: BillingDetailLine[]
}

interface AddUsageLineOptions {
  key: string
  labelKey: string
  quantity: number
  divisor?: 1 | 1000 | 1000000
  quantityUnitKey?: string
  unitPriceUSD: number
  factors?: BillingDetailFactor[]
}

const MILLION = 1_000_000 as const
const THOUSAND = 1_000 as const

function finitePositive(value: unknown): number {
  const number = Number(value)
  return Number.isFinite(number) && number > 0 ? number : 0
}

function resolveDiscount(other: LogOtherData): {
  value: number
  labelKey: string
} {
  const userRatio = Number(other.user_group_ratio)
  if (Number.isFinite(userRatio) && userRatio >= 0 && userRatio !== -1) {
    return { value: userRatio, labelKey: 'User Exclusive Discount' }
  }

  const groupRatio = Number(other.group_ratio)
  return {
    value: Number.isFinite(groupRatio) && groupRatio >= 0 ? groupRatio : 1,
    labelKey: 'Billing discount',
  }
}

function requestRuleFactors(other: LogOtherData): BillingDetailFactor[] {
  const multiplier = (other.request_rules || [])
    .filter(
      (rule) =>
        rule?.matched === true &&
        Number.isFinite(rule.multiplier) &&
        rule.multiplier >= 0
    )
    .reduce((product, rule) => product * rule.multiplier, 1)

  return multiplier === 1
    ? []
    : [{ labelKey: 'Request rule pricing', value: multiplier }]
}

function createLine(
  options: AddUsageLineOptions,
  discount: number
): BillingDetailLine | null {
  const quantity = finitePositive(options.quantity)
  const unitPriceUSD = finitePositive(options.unitPriceUSD)
  const divisor = options.divisor ?? MILLION
  if (quantity === 0 || unitPriceUSD === 0) return null

  const factors = options.factors || []
  const factor = factors.reduce((product, item) => product * item.value, 1)
  const originalAmountUSD = (quantity / divisor) * unitPriceUSD * factor

  return {
    key: options.key,
    labelKey: options.labelKey,
    quantity,
    divisor,
    quantityUnitKey: options.quantityUnitKey,
    unitPriceUSD,
    factors,
    originalAmountUSD,
    finalAmountUSD: originalAmountUSD * discount,
  }
}

function pushLine(
  lines: BillingDetailLine[],
  options: AddUsageLineOptions,
  discount: number
) {
  const line = createLine(options, discount)
  if (line) lines.push(line)
}

function cacheCounts(log: UsageLog, other: LogOtherData) {
  const read =
    finitePositive(log.cache_read_tokens) || finitePositive(other.cache_tokens)
  const write5m = finitePositive(other.cache_creation_tokens_5m)
  const write1h = finitePositive(other.cache_creation_tokens_1h)
  const persistedWrite =
    finitePositive(log.cache_write_tokens) ||
    finitePositive(other.cache_write_tokens) ||
    finitePositive(other.cache_creation_tokens)
  const splitWrite = write5m + write1h
  const writeTotal = Math.max(persistedWrite, splitWrite)

  return {
    read,
    write5m,
    write1h,
    writeGeneric: Math.max(0, writeTotal - splitWrite),
    writeTotal,
  }
}

function mediaCounts(other: LogOtherData) {
  return {
    imageInput:
      finitePositive(other.image_input_tokens) ||
      (other.image ? finitePositive(other.image_output) : 0),
    imageOutput: finitePositive(other.image_output_tokens),
    audioInput:
      finitePositive(other.audio_input_token_count) ||
      finitePositive(other.audio_input),
    audioOutput: finitePositive(other.audio_output),
  }
}

function buildPerTokenLines(
  log: UsageLog,
  other: LogOtherData,
  discount: number
): BillingDetailLine[] {
  const lines: BillingDetailLine[] = []
  const baseInputPrice = finitePositive(other.model_ratio) * 2
  const completionRatio = finitePositive(other.completion_ratio)
  const cache = cacheCounts(log, other)
  const media = mediaCounts(other)
  const isAudioLog = other.audio === true || other.ws === true
  const isClaude = other.claude === true || other.usage_semantic === 'anthropic'

  let inputTokens = isAudioLog
    ? finitePositive(other.text_input)
    : finitePositive(log.prompt_tokens)
  const outputTokens = isAudioLog
    ? finitePositive(other.text_output)
    : finitePositive(log.completion_tokens)

  if (!isAudioLog) {
    if (!isClaude) inputTokens -= cache.read + cache.writeTotal
    inputTokens -= media.imageInput
    if (other.audio_input_seperate_price) inputTokens -= media.audioInput
    inputTokens = Math.max(0, inputTokens)
  }

  pushLine(
    lines,
    {
      key: 'input',
      labelKey: isAudioLog ? 'Text Input' : 'Input',
      quantity: inputTokens,
      unitPriceUSD: baseInputPrice,
    },
    discount
  )
  pushLine(
    lines,
    {
      key: 'output',
      labelKey: isAudioLog ? 'Text Output' : 'Output',
      quantity: outputTokens,
      unitPriceUSD: baseInputPrice * completionRatio,
    },
    discount
  )

  const hasSplitCacheWrite = cache.write5m > 0 || cache.write1h > 0
  if (hasSplitCacheWrite) {
    pushLine(
      lines,
      {
        key: 'cache-write-generic',
        labelKey: 'Cache Write',
        quantity: cache.writeGeneric,
        unitPriceUSD:
          baseInputPrice * finitePositive(other.cache_creation_ratio),
      },
      discount
    )
    pushLine(
      lines,
      {
        key: 'cache-write-5m',
        labelKey: 'Cache Write (5m)',
        quantity: cache.write5m,
        unitPriceUSD:
          baseInputPrice *
          finitePositive(
            other.cache_creation_ratio_5m ?? other.cache_creation_ratio
          ),
      },
      discount
    )
    pushLine(
      lines,
      {
        key: 'cache-write-1h',
        labelKey: 'Cache Write (1h)',
        quantity: cache.write1h,
        unitPriceUSD:
          baseInputPrice *
          finitePositive(
            other.cache_creation_ratio_1h ?? other.cache_creation_ratio
          ),
      },
      discount
    )
  } else {
    pushLine(
      lines,
      {
        key: 'cache-write',
        labelKey: 'Cache Write',
        quantity: cache.writeTotal,
        unitPriceUSD:
          baseInputPrice * finitePositive(other.cache_creation_ratio),
      },
      discount
    )
  }

  pushLine(
    lines,
    {
      key: 'cache-read',
      labelKey: 'Cache Read',
      quantity: cache.read,
      unitPriceUSD: baseInputPrice * finitePositive(other.cache_ratio),
    },
    discount
  )

  if (isAudioLog) {
    pushLine(
      lines,
      {
        key: 'audio-input',
        labelKey: 'Audio input',
        quantity: media.audioInput,
        unitPriceUSD: baseInputPrice * finitePositive(other.audio_ratio),
      },
      discount
    )
    pushLine(
      lines,
      {
        key: 'audio-output',
        labelKey: 'Audio output',
        quantity: media.audioOutput,
        unitPriceUSD:
          baseInputPrice *
          finitePositive(other.audio_ratio) *
          finitePositive(other.audio_completion_ratio),
      },
      discount
    )
  } else {
    pushLine(
      lines,
      {
        key: 'image-input',
        labelKey: 'Image input',
        quantity: media.imageInput,
        unitPriceUSD: baseInputPrice * finitePositive(other.image_ratio),
      },
      discount
    )
  }

  return lines
}

function dynamicTokenCount(
  field: string,
  log: UsageLog,
  other: LogOtherData,
  usedFields: Set<string>
): { quantity: number; labelKey?: string } {
  const cache = cacheCounts(log, other)
  const media = mediaCounts(other)
  const isClaude = other.claude === true || other.usage_semantic === 'anthropic'

  if (field === 'inputPrice') {
    let input = finitePositive(log.prompt_tokens)
    if (!isClaude) {
      if (usedFields.has('cacheReadPrice')) input -= cache.read
      if (usedFields.has('cacheCreatePrice')) {
        input -= cache.write5m || cache.writeGeneric || cache.writeTotal
      }
      if (usedFields.has('cacheCreate1hPrice')) input -= cache.write1h
      if (usedFields.has('imagePrice')) input -= media.imageInput
      if (usedFields.has('audioInputPrice')) input -= media.audioInput
    }
    return { quantity: Math.max(0, input), labelKey: 'Input' }
  }
  if (field === 'outputPrice') {
    let output = finitePositive(log.completion_tokens)
    if (!isClaude) {
      if (usedFields.has('imageOutputPrice')) output -= media.imageOutput
      if (usedFields.has('audioOutputPrice')) output -= media.audioOutput
    }
    return { quantity: Math.max(0, output), labelKey: 'Output' }
  }
  if (field === 'cacheReadPrice') {
    return { quantity: cache.read, labelKey: 'Cache Read' }
  }
  if (field === 'cacheCreatePrice') {
    const split = cache.write5m > 0 || cache.write1h > 0
    return {
      quantity: split ? cache.write5m || cache.writeGeneric : cache.writeTotal,
      labelKey: split ? 'Cache Write (5m)' : 'Cache Write',
    }
  }
  if (field === 'cacheCreate1hPrice') {
    return { quantity: cache.write1h, labelKey: 'Cache Write (1h)' }
  }
  if (field === 'imagePrice') {
    return { quantity: media.imageInput, labelKey: 'Image input' }
  }
  if (field === 'imageOutputPrice') {
    return { quantity: media.imageOutput, labelKey: 'Image Tokens' }
  }
  if (field === 'audioInputPrice') {
    return { quantity: media.audioInput, labelKey: 'Audio input' }
  }
  if (field === 'audioOutputPrice') {
    return { quantity: media.audioOutput, labelKey: 'Audio output' }
  }
  return { quantity: 0 }
}

function buildDynamicLines(
  log: UsageLog,
  other: LogOtherData,
  discount: number
): { lines: BillingDetailLine[]; matchedTier?: string } {
  const summary = getTieredBillingSummary(other)
  if (!summary) return { lines: [], matchedTier: other.matched_tier }

  const lines: BillingDetailLine[] = []
  const factors = requestRuleFactors(other)
  const usedFields = new Set(summary.priceEntries.map((entry) => entry.field))
  const fieldOrder: Record<string, number> = {
    inputPrice: 0,
    outputPrice: 1,
    cacheCreatePrice: 2,
    cacheCreate1hPrice: 3,
    cacheReadPrice: 4,
    imagePrice: 5,
    imageOutputPrice: 6,
    audioInputPrice: 7,
    audioOutputPrice: 8,
    perSecondPrice: 9,
    perRequestPrice: 10,
  }
  const priceEntries = [...summary.priceEntries].sort(
    (left, right) =>
      (fieldOrder[left.field] ?? 99) - (fieldOrder[right.field] ?? 99)
  )

  for (const entry of priceEntries) {
    if (entry.field === 'perRequestPrice') {
      pushLine(
        lines,
        {
          key: 'dynamic-request',
          labelKey: 'Per request',
          quantity: 1,
          divisor: 1,
          quantityUnitKey: 'request',
          unitPriceUSD: entry.price,
          factors,
        },
        discount
      )
      continue
    }

    const token = dynamicTokenCount(entry.field, log, other, usedFields)
    pushLine(
      lines,
      {
        key: `dynamic-${entry.field}`,
        labelKey: token.labelKey || entry.shortLabel,
        quantity: token.quantity,
        unitPriceUSD: entry.price,
        factors,
      },
      discount
    )
  }

  return { lines, matchedTier: summary.tier.label || other.matched_tier }
}

function surchargeLabel(item: ToolSurchargeLogItem): string {
  if (item.name.includes('web_search') || item.name === 'google_search') {
    return 'Web Search'
  }
  if (item.name === 'file_search') return 'File Search'
  if (item.name === 'image_generation') return 'Image Generation'
  return item.name
}

function appendAdditionalLines(
  lines: BillingDetailLine[],
  other: LogOtherData,
  discount: number,
  mode: BillingDetailMode
) {
  const toolSurcharges =
    mode !== 'per-call' && Array.isArray(other.tool_surcharges)
      ? other.tool_surcharges
      : []

  for (const [index, item] of toolSurcharges.entries()) {
    pushLine(
      lines,
      {
        key: `tool-${item.name}-${index}`,
        labelKey: surchargeLabel(item),
        quantity: item.count,
        divisor: THOUSAND,
        unitPriceUSD: item.price,
      },
      discount
    )
  }

  if (mode !== 'per-call' && toolSurcharges.length === 0) {
    pushLine(
      lines,
      {
        key: 'legacy-web-search',
        labelKey: 'Web Search',
        quantity: finitePositive(other.web_search_call_count),
        divisor: THOUSAND,
        unitPriceUSD: finitePositive(other.web_search_price),
      },
      discount
    )
    pushLine(
      lines,
      {
        key: 'legacy-file-search',
        labelKey: 'File Search',
        quantity: finitePositive(other.file_search_call_count),
        divisor: THOUSAND,
        unitPriceUSD: finitePositive(other.file_search_price),
      },
      discount
    )
    pushLine(
      lines,
      {
        key: 'legacy-image-generation',
        labelKey: 'Image Generation',
        quantity: other.image_generation_call ? 1 : 0,
        divisor: 1,
        quantityUnitKey: 'request',
        unitPriceUSD: finitePositive(other.image_generation_call_price),
      },
      discount
    )
  }

  if (mode !== 'dynamic' && other.audio_input_seperate_price) {
    pushLine(
      lines,
      {
        key: 'audio-input-separate',
        labelKey: 'Audio input',
        quantity: mediaCounts(other).audioInput,
        unitPriceUSD: finitePositive(other.audio_input_price),
      },
      discount
    )
  }
}

export function buildBillingDetail(
  log: UsageLog,
  other: LogOtherData,
  actualAmountUSD: number,
  roundingUnitUSD = 0
): BillingDetail {
  const { value: discount, labelKey: discountLabelKey } = resolveDiscount(other)
  const mode: BillingDetailMode =
    other.billing_mode === 'tiered_expr'
      ? 'dynamic'
      : finitePositive(other.model_price) > 0
        ? 'per-call'
        : 'per-token'

  let lines: BillingDetailLine[] = []
  let matchedTier = other.matched_tier

  if (mode === 'dynamic') {
    const dynamic = buildDynamicLines(log, other, discount)
    lines = dynamic.lines
    matchedTier = dynamic.matchedTier
  } else if (mode === 'per-call') {
    pushLine(
      lines,
      {
        key: 'request',
        labelKey: 'Per request',
        quantity: 1,
        divisor: 1,
        quantityUnitKey: 'request',
        unitPriceUSD: finitePositive(other.model_price),
      },
      discount
    )
  } else {
    lines = buildPerTokenLines(log, other, discount)
  }

  appendAdditionalLines(lines, other, discount, mode)

  const knownFinalAmountUSD = lines.reduce(
    (total, line) => total + line.finalAmountUSD,
    0
  )
  const residual = actualAmountUSD - knownFinalAmountUSD
  const tolerance = Math.max(roundingUnitUSD * 1.01, 1e-12)
  if (Math.abs(residual) > tolerance) {
    const originalAmountUSD = discount > 0 ? residual / discount : residual
    lines.push({
      key: 'other',
      labelKey: 'Other',
      factors: [],
      originalAmountUSD,
      finalAmountUSD: residual,
      amountOnly: true,
    })
  }

  const knownOriginalAmountUSD = lines.reduce(
    (total, line) => total + line.originalAmountUSD,
    0
  )

  return {
    mode,
    discount,
    discountLabelKey,
    originalAmountUSD:
      discount > 0 ? actualAmountUSD / discount : knownOriginalAmountUSD,
    finalAmountUSD: actualAmountUSD,
    matchedTier,
    lines,
  }
}
