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

export const CACHE_MODE_TIMED = 'timed'
export const CACHE_MODE_GENERIC = 'generic'
export type CacheMode = typeof CACHE_MODE_TIMED | typeof CACHE_MODE_GENERIC

export type TokenTierCondition = {
  source: 'token'
  var: 'p' | 'c' | 'len'
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

export function getTierCacheMode(
  tier: Partial<VisualTier> | null | undefined
): CacheMode {
  if (tier?.cache_mode === CACHE_MODE_TIMED) return CACHE_MODE_TIMED
  if (tier?.cache_mode === CACHE_MODE_GENERIC) return CACHE_MODE_GENERIC
  return Number(tier?.cache_create_1h_unit_cost) > 0
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
        tokenCondition.var === 'p' || tokenCondition.var === 'c'
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
      condition.includes(' || ') ? `(${condition})` : condition
    )
    .join(' && ')
}

function buildTierBodyExpr(tier: VisualTier): string {
  const parts: string[] = []
  const perRequestPrice = Number(tier.per_request_unit_cost) || 0
  const perSecondPrice = Number(tier.per_second_unit_cost) || 0
  const ic = Number(tier.input_unit_cost) || 0
  const oc = Number(tier.output_unit_cost) || 0
  if (perRequestPrice !== 0) {
    parts.push(String(perRequestPrice * 1000000))
  }
  parts.push(`p * ${ic}`)
  parts.push(`c * ${oc}`)
  if (perSecondPrice !== 0) {
    parts.push(`param("parameters.duration") * ${perSecondPrice} * 1000000`)
  }
  for (const variable of BILLING_EXTRA_VARS) {
    if (
      variable.key === 'duration' ||
      variable.key === 'request' ||
      !variable.tierField
    ) {
      continue
    }
    const value =
      Number((tier as Record<string, unknown>)[variable.tierField]) || 0
    if (value !== 0) parts.push(`${variable.key} * ${value}`)
  }
  return parts.join(' + ')
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

    if (i < tiers.length - 1 && cond) {
      parts.push(`${cond} ? ${body}`)
    } else {
      parts.push(body)
    }
  }
  return parts.join(' : ')
}

function hasFullOuterParens(value: string): boolean {
  const text = value.trim()
  if (!text.startsWith('(') || !text.endsWith(')')) return false
  let depth = 0
  let inString = false
  let escaped = false
  for (let index = 0; index < text.length; index += 1) {
    const char = text[index]
    if (escaped) {
      escaped = false
      continue
    }
    if (char === '\\') {
      escaped = true
      continue
    }
    if (char === '"') {
      inString = !inString
      continue
    }
    if (inString) continue
    if (char === '(') depth += 1
    if (char === ')') depth -= 1
    if (depth === 0 && index < text.length - 1) return false
  }
  return depth === 0
}

function unwrapOuterParens(value: string): string {
  let text = value.trim()
  while (hasFullOuterParens(text)) text = text.slice(1, -1).trim()
  return text
}

function splitTopLevel(value: string, operator: string): string[] {
  const parts: string[] = []
  let start = 0
  let depth = 0
  let inString = false
  let escaped = false
  for (let index = 0; index < value.length; index += 1) {
    const char = value[index]
    if (escaped) {
      escaped = false
      continue
    }
    if (char === '\\') {
      escaped = true
      continue
    }
    if (char === '"') {
      inString = !inString
      continue
    }
    if (inString) continue
    if (char === '(') depth += 1
    if (char === ')') depth -= 1
    const isExponentSign =
      operator === '+' && (value[index - 1] === 'e' || value[index - 1] === 'E')
    if (
      depth === 0 &&
      !isExponentSign &&
      value.slice(index, index + operator.length) === operator
    ) {
      parts.push(value.slice(start, index).trim())
      start = index + operator.length
      index += operator.length - 1
    }
  }
  parts.push(value.slice(start).trim())
  return parts.filter(Boolean)
}

function splitTopLevelTernary(
  value: string
): { condition: string; whenTrue: string; whenFalse: string } | null {
  const text = unwrapOuterParens(value)
  let depth = 0
  let inString = false
  let escaped = false
  let questionIndex = -1
  let nestedQuestions = 0

  for (let index = 0; index < text.length; index += 1) {
    const char = text[index]
    if (escaped) {
      escaped = false
      continue
    }
    if (char === '\\') {
      escaped = true
      continue
    }
    if (char === '"') {
      inString = !inString
      continue
    }
    if (inString) continue
    if (char === '(') depth += 1
    if (char === ')') depth -= 1
    if (depth !== 0) continue
    if (char === '?') {
      if (questionIndex === -1) questionIndex = index
      else nestedQuestions += 1
      continue
    }
    if (char === ':' && questionIndex !== -1) {
      if (nestedQuestions > 0) {
        nestedQuestions -= 1
        continue
      }
      return {
        condition: text.slice(0, questionIndex).trim(),
        whenTrue: text.slice(questionIndex + 1, index).trim(),
        whenFalse: text.slice(index + 1).trim(),
      }
    }
  }
  return null
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
  for (const part of splitTopLevel(unwrapOuterParens(body), '+')) {
    let match = part.match(
      new RegExp(
        `^(p|c|cr|cc|cc1h|img|img_o|ai|ao)\\s*\\*\\s*(${NUMBER_PATTERN})$`
      )
    )
    if (match) {
      tier[TOKEN_FIELD_BY_VAR[match[1]]] = Number(match[2])
      continue
    }

    match = part.match(
      new RegExp(
        `^param\\("parameters\\.duration"\\)\\s*\\*\\s*(${NUMBER_PATTERN})\\s*\\*\\s*1000000$`
      )
    )
    if (match) {
      tier.per_second_unit_cost = Number(match[1])
      continue
    }

    match = part.match(new RegExp(`^(${NUMBER_PATTERN})\\s*\\*\\s*1000000$`))
    if (match) {
      tier.per_request_unit_cost = Number(match[1])
      continue
    }

    match = part.match(new RegExp(`^(${NUMBER_PATTERN})$`))
    if (match) {
      tier.per_request_unit_cost = Number(match[1]) / 1000000
      continue
    }
    return null
  }
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
  const parts = splitTopLevel(unwrapOuterParens(value), '&&')
  const conditions: TierConditionInput[] = []
  for (let index = 0; index < parts.length; index += 1) {
    const tokenMatch = unwrapOuterParens(parts[index]).match(
      new RegExp(`^(p|c|len)\\s*(<=|>=|<|>)\\s*(${NUMBER_PATTERN})$`)
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
    const requestCondition =
      (combined && tryParseRequestCondition(combined)) ||
      tryParseRequestCondition(unwrapOuterParens(parts[index]))
    if (!requestCondition) return null
    if (combined && tryParseRequestCondition(combined)) index += 1
    conditions.push(requestCondition)
  }
  return conditions
}

function isZeroCostExpr(value: string): boolean {
  const parsed = parseTierBody(value)
  if (!parsed) return false
  return Object.entries(parsed).every(([key, fieldValue]) => {
    if (key === 'label' || key === 'conditions' || key === 'cache_mode') {
      return true
    }
    return Number(fieldValue) === 0
  })
}

function collectVisualTiers(
  value: string,
  inheritedConditions: TierConditionInput[],
  tiers: VisualTier[]
): boolean {
  const tier = parseTierCall(value)
  if (tier) {
    tiers.push(
      normalizeVisualTier({ ...tier, conditions: inheritedConditions })
    )
    return true
  }

  const conditional = splitTopLevelTernary(value)
  if (!conditional) return isZeroCostExpr(unwrapOuterParens(value))
  const conditions = parseTierConditions(conditional.condition)
  if (!conditions || conditions.length === 0) return false
  return (
    collectVisualTiers(
      conditional.whenTrue,
      [...inheritedConditions, ...conditions],
      tiers
    ) && collectVisualTiers(conditional.whenFalse, inheritedConditions, tiers)
  )
}

export function tryParseVisualConfig(
  exprStr: string | null | undefined
): VisualConfig | null {
  if (!exprStr) return null
  try {
    let body = exprStr
    const versionMatch = body.match(/^v\d+:([\s\S]*)$/)
    if (versionMatch) body = versionMatch[1]
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
      `"use strict"; return (${exprStr});`
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
