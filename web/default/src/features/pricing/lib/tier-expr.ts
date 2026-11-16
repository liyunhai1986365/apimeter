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
  BILLING_EXTRA_VARS,
  buildRequestConditionExpr,
  normalizeCondition,
  tryParseRequestCondition,
  type RequestCondition,
} from './billing-expr'
import {
  splitExprVersion,
  splitTopLevel,
  splitTopLevelTernary,
  unwrapOuterParens,
} from './expr-syntax'

export const CACHE_MODE_TIMED = 'timed'
export const CACHE_MODE_GENERIC = 'generic'
export type CacheMode = typeof CACHE_MODE_TIMED | typeof CACHE_MODE_GENERIC

export type TokenTierCondition = {
  source: 'token'
  var: 'p' | 'c' | 'len' | 'cr' | 'cc' | 'cc1h' | 'img' | 'img_o' | 'ai' | 'ao'
  op: '<' | '<=' | '>' | '>='
  value: number | string
}

export type TierConditionInput = TokenTierCondition | RequestCondition

export type VisualTier = {
  label: string
  conditions: TierConditionInput[]
  input_unit_cost: number
  output_unit_cost: number
  cache_mode: CacheMode
  priceTerms?: { field: string; value: number; expression: string }[]
  priceEnabled?: Record<string, boolean>
  cache_read_unit_cost?: number
  cache_create_unit_cost?: number
  cache_create_1h_unit_cost?: number
  image_unit_cost?: number
  image_output_unit_cost?: number
  audio_input_unit_cost?: number
  audio_output_unit_cost?: number
  per_second_unit_cost?: number
  per_request_unit_cost?: number
  [field: string]: unknown
}

export type VisualConfig = {
  tiers: VisualTier[]
}

/** Presence is independent of price: an explicit zero opts into free pricing. */
export function isVisualPriceEnabled(
  tier: Partial<VisualTier>,
  field: string
): boolean {
  const enabled = tier.priceEnabled?.[field]
  if (enabled !== undefined) return enabled
  if (tier.priceTerms?.some((term) => term.field === field)) return true
  if (Number(tier[field])) return true
  return (
    !tier.priceTerms &&
    (field === 'input_unit_cost' || field === 'output_unit_cost')
  )
}

export function setVisualTierPriceEnabled(
  tier: VisualTier,
  field: string,
  enabled: boolean
): VisualTier {
  return {
    ...tier,
    priceEnabled: { ...tier.priceEnabled, [field]: enabled },
  }
}

export function setVisualTierPrice(
  tier: VisualTier,
  field: string,
  value: number
): VisualTier {
  return { ...setVisualTierPriceEnabled(tier, field, true), [field]: value }
}

export function setVisualTierCacheMode(
  tier: VisualTier,
  mode: CacheMode
): VisualTier {
  const next = { ...tier, cache_mode: mode }
  return mode === CACHE_MODE_GENERIC
    ? setVisualTierPriceEnabled(next, 'cache_create_1h_unit_cost', false)
    : next
}

export function getTierCacheMode(
  tier: Partial<VisualTier> | null | undefined
): CacheMode {
  if (tier?.cache_mode === CACHE_MODE_TIMED) return CACHE_MODE_TIMED
  if (tier?.cache_mode === CACHE_MODE_GENERIC) return CACHE_MODE_GENERIC
  return tier && isVisualPriceEnabled(tier, 'cache_create_1h_unit_cost')
    ? CACHE_MODE_TIMED
    : CACHE_MODE_GENERIC
}

export function normalizeVisualTier(
  tier: Partial<VisualTier> = {}
): VisualTier {
  return {
    ...tier,
    label: tier.label ?? '',
    input_unit_cost: Number(tier.input_unit_cost) || 0,
    output_unit_cost: Number(tier.output_unit_cost) || 0,
    cache_mode: getTierCacheMode(tier),
    conditions: Array.isArray(tier.conditions)
      ? tier.conditions.map(normalizeTierCondition)
      : [],
    cache_read_unit_cost: Number(tier.cache_read_unit_cost) || 0,
    cache_create_unit_cost: Number(tier.cache_create_unit_cost) || 0,
    cache_create_1h_unit_cost: Number(tier.cache_create_1h_unit_cost) || 0,
    image_unit_cost: Number(tier.image_unit_cost) || 0,
    image_output_unit_cost: Number(tier.image_output_unit_cost) || 0,
    audio_input_unit_cost: Number(tier.audio_input_unit_cost) || 0,
    audio_output_unit_cost: Number(tier.audio_output_unit_cost) || 0,
    per_second_unit_cost: Number(tier.per_second_unit_cost) || 0,
    per_request_unit_cost: Number(tier.per_request_unit_cost) || 0,
  }
}

function normalizeTierCondition(
  condition: Partial<TierConditionInput> & {
    var?: TokenTierCondition['var']
  }
): TierConditionInput {
  if (!condition.source || condition.source === 'token') {
    const tokenCondition = condition as Partial<TokenTierCondition>
    return {
      source: 'token',
      var:
        tokenCondition.var &&
        [
          'p',
          'c',
          'len',
          'cr',
          'cc',
          'cc1h',
          'img',
          'img_o',
          'ai',
          'ao',
        ].includes(tokenCondition.var)
          ? tokenCondition.var
          : 'len',
      op:
        tokenCondition.op === '<=' ||
        tokenCondition.op === '>' ||
        tokenCondition.op === '>='
          ? tokenCondition.op
          : '<',
      value: tokenCondition.value == null ? '' : tokenCondition.value,
    }
  }
  return normalizeCondition(condition as Partial<RequestCondition>)
}

export function createEmptyTokenCondition(): TokenTierCondition {
  return { source: 'token', var: 'len', op: '<', value: 200000 }
}

export function createDefaultVisualConfig(): VisualConfig {
  return {
    tiers: [
      normalizeVisualTier({
        conditions: [],
        input_unit_cost: 0,
        output_unit_cost: 0,
        label: 'base',
        cache_mode: CACHE_MODE_GENERIC,
      }),
    ],
  }
}

export function normalizeVisualConfig(
  config: VisualConfig | null | undefined
): VisualConfig {
  if (!config || !Array.isArray(config.tiers) || config.tiers.length === 0) {
    return createDefaultVisualConfig()
  }
  return {
    ...config,
    tiers: config.tiers.map((tier) => normalizeVisualTier(tier)),
  }
}

function buildConditionStr(conditions: TierConditionInput[]): string {
  if (!conditions || conditions.length === 0) return ''
  return conditions
    .map((condition) => {
      if (condition.source === 'token') {
        if (
          !condition.var ||
          !condition.op ||
          condition.value == null ||
          condition.value === ''
        ) {
          return ''
        }
        return `${condition.var} ${condition.op} ${condition.value}`
      }
      return buildRequestConditionExpr(condition)
    })
    .filter(Boolean)
    .map((condition) =>
      splitTopLevel(condition, '||').length > 1 ? `(${condition})` : condition
    )
    .join(' && ')
}

function buildTierBodyExpr(tier: VisualTier): string {
  const fields = [
    { key: 'request', field: 'per_request_unit_cost' },
    { key: 'p', field: 'input_unit_cost' },
    { key: 'c', field: 'output_unit_cost' },
    { key: 'duration', field: 'per_second_unit_cost' },
    ...BILLING_EXTRA_VARS.filter(
      (v) => v.key !== 'request' && v.key !== 'duration'
    ).map((v) => ({ key: v.key, field: v.tierField! })),
  ]
  const parts: string[] = []
  for (const { key, field } of fields) {
    if (!isVisualPriceEnabled(tier, field)) continue
    const value = Number(tier[field]) || 0
    const original = tier.priceTerms?.find((term) => term.field === field)
    if (original && value === original.value) {
      parts.push(original.expression)
      continue
    }
    if (key === 'request') parts.push(`${value} * 1000000`)
    else if (key === 'duration')
      parts.push(`param("parameters.duration") * ${value} * 1000000`)
    else parts.push(`${key} * ${value}`)
  }
  return parts.join(' + ') || '0'
}

export function generateExprFromVisualConfig(
  config: VisualConfig | null | undefined
): string {
  if (!config || !config.tiers || config.tiers.length === 0) {
    return 'p * 0 + c * 0'
  }
  const tiers = config.tiers

  if (tiers.length === 1) {
    const tier = tiers[0]
    const label = tier.label || 'default'
    const body = `tier(${JSON.stringify(label)}, ${buildTierBodyExpr(tier)})`
    const cond = buildConditionStr(tier.conditions)
    if (cond) {
      return `${cond} ? ${body} : p * 0 + c * 0`
    }
    return body
  }

  const parts: string[] = []
  for (let i = 0; i < tiers.length; i++) {
    const tier = tiers[i]
    const label = tier.label || `tier_${i + 1}`
    const body = `tier(${JSON.stringify(label)}, ${buildTierBodyExpr(tier)})`
    const cond = buildConditionStr(tier.conditions)

    if (cond) {
      parts.push(`${cond} ? ${body}`)
    } else {
      parts.push(body)
    }
  }
  if (buildConditionStr(tiers[tiers.length - 1].conditions))
    parts.push('p * 0 + c * 0')
  return parts.join(' : ')
}

const TOKEN_FIELD_BY_VAR: Record<string, keyof VisualTier> = {
  p: 'input_unit_cost',
  c: 'output_unit_cost',
  cr: 'cache_read_unit_cost',
  cc: 'cache_create_unit_cost',
  cc1h: 'cache_create_1h_unit_cost',
  img: 'image_unit_cost',
  img_o: 'image_output_unit_cost',
  ai: 'audio_input_unit_cost',
  ao: 'audio_output_unit_cost',
}

const NUMBER_PATTERN = '-?(?:\\d+\\.?\\d*|\\.\\d+)(?:[eE][+-]?\\d+)?'

function parseTierBody(body: string): Partial<VisualTier> | null {
  const tier = normalizeVisualTier()
  tier.priceTerms = []
  const addTerm = (field: string, value: number, expression: string) => {
    if (
      !Number.isFinite(value) ||
      tier.priceTerms!.some((term) => term.field === field)
    )
      return false
    tier[field] = value
    tier.priceTerms!.push({ field, value, expression })
    return true
  }
  for (const part of splitTopLevel(unwrapOuterParens(body), '+')) {
    let match = part.match(
      new RegExp(
        `^(p|c|cr|cc|cc1h|img|img_o|ai|ao)\\s*\\*\\s*(${NUMBER_PATTERN})$`
      )
    )
    if (match) {
      if (
        !addTerm(String(TOKEN_FIELD_BY_VAR[match[1]]), Number(match[2]), part)
      )
        return null
      continue
    }

    match = part.match(
      new RegExp(
        `^param\\("parameters\\.duration"\\)\\s*\\*\\s*(${NUMBER_PATTERN})\\s*\\*\\s*1000000$`
      )
    )
    if (match) {
      if (!addTerm('per_second_unit_cost', Number(match[1]), part)) return null
      continue
    }

    match = part.match(new RegExp(`^(${NUMBER_PATTERN})\\s*\\*\\s*1000000$`))
    if (match) {
      if (!addTerm('per_request_unit_cost', Number(match[1]), part)) return null
      continue
    }

    match = part.match(new RegExp(`^(${NUMBER_PATTERN})$`))
    if (match) {
      if (!addTerm('per_request_unit_cost', Number(match[1]) / 1000000, part))
        return null
      continue
    }
    return null
  }
  tier.priceTerms.sort((a, b) => a.field.localeCompare(b.field))
  tier.cache_mode = isVisualPriceEnabled(tier, 'cache_create_1h_unit_cost')
    ? CACHE_MODE_TIMED
    : CACHE_MODE_GENERIC
  return tier
}

function parseTierCall(value: string): Partial<VisualTier> | null {
  const text = unwrapOuterParens(value)
  const match = text.match(/^tier\(\s*("(?:[^"\\]|\\.)*")\s*,\s*([\s\S]+)\)$/)
  if (!match) return null
  const parsedBody = parseTierBody(match[2])
  if (!parsedBody) return null
  return {
    ...parsedBody,
    label: JSON.parse(match[1]) as string,
  }
}

function parseTierConditions(value: string): TierConditionInput[] | null {
  const expr = unwrapOuterParens(value)
  if (splitTopLevel(expr, '||').length > 1) {
    const condition = tryParseRequestCondition(expr)
    return condition ? [condition] : null
  }
  const parts = splitTopLevel(expr, '&&')
  const conditions: TierConditionInput[] = []
  for (let index = 0; index < parts.length; index += 1) {
    const tokenMatch = unwrapOuterParens(parts[index]).match(
      new RegExp(
        `^(p|c|len|cr|cc|cc1h|img|img_o|ai|ao)\\s*(<=|>=|<|>)\\s*(${NUMBER_PATTERN})$`
      )
    )
    if (tokenMatch) {
      conditions.push({
        source: 'token',
        var: tokenMatch[1] as TokenTierCondition['var'],
        op: tokenMatch[2] as TokenTierCondition['op'],
        value: Number(tokenMatch[3]),
      })
      continue
    }

    const combined =
      index + 1 < parts.length
        ? `${unwrapOuterParens(parts[index])} && ${unwrapOuterParens(parts[index + 1])}`
        : ''
    const paired = combined ? tryParseRequestCondition(combined) : null
    const guarded = paired && paired.source !== 'expression' ? paired : null
    const requestCondition =
      guarded || tryParseRequestCondition(unwrapOuterParens(parts[index]))
    if (!requestCondition) return null
    if (guarded) index += 1
    conditions.push(requestCondition)
  }
  return conditions
}

function isZeroCostExpr(value: string): boolean {
  const parsed = parseTierBody(value)
  if (!parsed) return false
  // Dropping an opt-in variable even from a free fallback changes normalization
  // in every branch. Only plain zero and base p/c zero terms may be omitted.
  if (
    parsed.priceTerms?.some(
      (term) =>
        ![
          'input_unit_cost',
          'output_unit_cost',
          'per_request_unit_cost',
        ].includes(term.field)
    )
  )
    return false
  return Object.entries(parsed).every(([key, fieldValue]) => {
    if (
      key === 'label' ||
      key === 'conditions' ||
      key === 'cache_mode' ||
      key === 'priceTerms'
    ) {
      return true
    }
    return Number(fieldValue) === 0
  })
}

function collectVisualTiers(
  value: string,
  inheritedConditions: TierConditionInput[],
  tiers: VisualTier[],
  allowZeroFallback = false
): boolean {
  const tier = parseTierCall(value)
  if (tier) {
    tiers.push(
      normalizeVisualTier({ ...tier, conditions: inheritedConditions })
    )
    return true
  }

  const conditional = splitTopLevelTernary(value)
  if (!conditional)
    return (
      allowZeroFallback &&
      inheritedConditions.length === 0 &&
      isZeroCostExpr(unwrapOuterParens(value))
    )
  const conditions = parseTierConditions(conditional.condition)
  if (!conditions || conditions.length === 0) return false
  return (
    collectVisualTiers(
      conditional.whenTrue,
      [...inheritedConditions, ...conditions],
      tiers
    ) &&
    collectVisualTiers(conditional.whenFalse, inheritedConditions, tiers, true)
  )
}

export function tryParseVisualConfig(
  exprStr: string | null | undefined
): VisualConfig | null {
  if (!exprStr) return null
  try {
    const { prefix, body } = splitExprVersion(exprStr.trim())
    if (prefix && prefix !== 'v1:') return null
    const tiers: VisualTier[] = []
    if (!collectVisualTiers(body, [], tiers)) return null
    if (tiers.length === 0) return null
    return normalizeVisualConfig({ tiers })
  } catch {
    return null
  }
}

// ---------------------------------------------------------------------------
// Local cost evaluator (for the estimator preview)
// ---------------------------------------------------------------------------

const ESTIMATOR_VARS = [
  { var: 'cr', stateKey: 'cacheReadTokens' },
  { var: 'cc', stateKey: 'cacheCreateTokens' },
  { var: 'cc1h', stateKey: 'cacheCreate1hTokens' },
  { var: 'img', stateKey: 'imageTokens' },
  { var: 'img_o', stateKey: 'imageOutputTokens' },
  { var: 'ai', stateKey: 'audioInputTokens' },
  { var: 'ao', stateKey: 'audioOutputTokens' },
] as const

export type ExtraTokenValues = Record<
  (typeof ESTIMATOR_VARS)[number]['stateKey'],
  number
>

export type EvalResult = {
  cost: number
  matchedTier: string
  error: string | null
}

function timeInZone(tz: string): Date {
  const trimmed = tz.trim()
  if (!trimmed) return new Date()
  try {
    return new Date(new Date().toLocaleString('en-US', { timeZone: trimmed }))
  } catch {
    return new Date()
  }
}

const SIZE_PIXELS_REGEX = /(\d+(?:\.\d+)?)\s*(?:x|×|\*)\s*(\d+(?:\.\d+)?)/i

function pixelsFromSizeValue(value: unknown): number {
  if (value == null) return 0
  if (typeof value === 'number')
    return Number.isFinite(value) && value > 0 ? value : 0
  const text = String(value).trim()
  if (!text) return 0
  const normalized = text.toLowerCase()
  if (normalized === '1k') return 1024 * 1024
  if (normalized === '2k') return 2048 * 2048
  const numeric = Number(text)
  if (Number.isFinite(numeric) && numeric > 0) return numeric
  const match = text.match(SIZE_PIXELS_REGEX)
  if (!match) return 0
  const width = Number(match[1])
  const height = Number(match[2])
  if (
    !Number.isFinite(width) ||
    !Number.isFinite(height) ||
    width <= 0 ||
    height <= 0
  ) {
    return 0
  }
  return width * height
}

export function evalExprLocally(
  exprStr: string,
  promptTokens: number,
  completionTokens: number,
  extraTokenValues: ExtraTokenValues
): EvalResult {
  try {
    if (!exprStr || !exprStr.trim()) {
      return { cost: 0, matchedTier: '', error: null }
    }
    const { prefix, body } = splitExprVersion(exprStr.trim())
    if (prefix && prefix !== 'v1:')
      throw new Error(`Unsupported expression version: ${prefix}`)
    let matchedTier = ''
    const tierFn = (name: string, value: number) => {
      matchedTier = name
      return value
    }
    const cacheReadTokens = extraTokenValues.cacheReadTokens || 0
    const cacheCreateTokens = extraTokenValues.cacheCreateTokens || 0
    const cacheCreate1hTokens = extraTokenValues.cacheCreate1hTokens || 0
    const len =
      promptTokens + cacheReadTokens + cacheCreateTokens + cacheCreate1hTokens
    const env: Record<string, unknown> = {
      p: promptTokens,
      c: completionTokens,
      len,
      nil: null,
      tier: tierFn,
      max: Math.max,
      min: Math.min,
      abs: Math.abs,
      ceil: Math.ceil,
      floor: Math.floor,
      header: () => '',
      param: () => null,
      response: () => null,
      pixels: pixelsFromSizeValue,
      has: (source: unknown, substr: string) => {
        if (source == null || !substr) return false
        return String(source).includes(substr)
      },
      hour: (tz: string) => timeInZone(tz).getHours(),
      minute: (tz: string) => timeInZone(tz).getMinutes(),
      weekday: (tz: string) => timeInZone(tz).getDay(),
      month: (tz: string) => timeInZone(tz).getMonth() + 1,
      day: (tz: string) => timeInZone(tz).getDate(),
    }
    for (const field of ESTIMATOR_VARS) {
      env[field.var] = extraTokenValues[field.stateKey] || 0
    }
    const fn = new Function(
      ...Object.keys(env),
      `"use strict"; return (${body});`
    )
    const cost = Number(fn(...Object.values(env))) || 0
    return { cost, matchedTier, error: null }
  } catch (e) {
    const message = e instanceof Error ? e.message : String(e)
    return { cost: 0, matchedTier: '', error: message }
  }
}

export function exprUsesExtraVars(exprStr: string): boolean {
  if (!exprStr) return false
  const varNames = ESTIMATOR_VARS.map((f) => f.var).join('|')
  return new RegExp(`\\b(${varNames})\\b`).test(exprStr)
}

export const ESTIMATOR_EXTRA_FIELDS = ESTIMATOR_VARS
