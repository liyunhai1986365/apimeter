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
/**
 * Billing expression parsing utilities.
 *
 * Only complete, recognized prices may become a structured price table.
 * Other valid backend expressions are displayed verbatim by callers.
 */
import {
  splitExprVersion,
  splitTopLevel,
  splitTopLevelProduct,
  splitTopLevelTernary,
  unwrapOuterParens,
} from './expr-syntax'

// ---------------------------------------------------------------------------
// Variable registry
// ---------------------------------------------------------------------------

export type BillingVar = {
  key: string
  field: string | null
  tierField: string | null
  label: string
  shortLabel: string
  side: 'input' | 'output' | 'condition'
  isBase?: boolean
  isConditionOnly?: boolean
  group?: string
  unit?: 'tokens' | 'second' | 'request'
}

export const BILLING_VARS: BillingVar[] = [
  {
    key: 'p',
    field: 'inputPrice',
    tierField: 'input_unit_cost',
    label: 'Input price',
    shortLabel: 'Input',
    side: 'input',
    isBase: true,
  },
  {
    key: 'c',
    field: 'outputPrice',
    tierField: 'output_unit_cost',
    label: 'Completion price',
    shortLabel: 'Output',
    side: 'output',
    isBase: true,
  },
  {
    key: 'duration',
    field: 'perSecondPrice',
    tierField: 'per_second_unit_cost',
    label: 'Per second price',
    shortLabel: 'Per second',
    side: 'output',
    group: 'time',
    unit: 'second',
  },
  {
    key: 'request',
    field: 'perRequestPrice',
    tierField: 'per_request_unit_cost',
    label: 'Per request price',
    shortLabel: '',
    side: 'output',
    group: 'request',
    unit: 'request',
  },
  {
    key: 'len',
    field: null,
    tierField: null,
    label: 'Input length',
    shortLabel: 'Length',
    side: 'condition',
    isConditionOnly: true,
  },
  {
    key: 'cr',
    field: 'cacheReadPrice',
    tierField: 'cache_read_unit_cost',
    label: 'Cache read price',
    shortLabel: 'Cache Read',
    side: 'input',
    group: 'cache',
  },
  {
    key: 'cc',
    field: 'cacheCreatePrice',
    tierField: 'cache_create_unit_cost',
    label: 'Cache create price',
    shortLabel: 'Cache Write (5m)',
    side: 'input',
    group: 'cache',
  },
  {
    key: 'cc1h',
    field: 'cacheCreate1hPrice',
    tierField: 'cache_create_1h_unit_cost',
    label: 'Cache create (1h) price',
    shortLabel: 'Cache Write (1h)',
    side: 'input',
    group: 'cache',
  },
  {
    key: 'cc12h',
    field: 'cacheCreate12hPrice',
    tierField: 'cache_create_12h_unit_cost',
    label: 'Cache create (12h) price',
    shortLabel: 'Cache Write (12h)',
    side: 'input',
    group: 'cache',
  },
  {
    key: 'img',
    field: 'imagePrice',
    tierField: 'image_unit_cost',
    label: 'Image input price',
    shortLabel: 'Image In',
    side: 'input',
    group: 'media',
  },
  {
    key: 'img_o',
    field: 'imageOutputPrice',
    tierField: 'image_output_unit_cost',
    label: 'Image output price',
    shortLabel: 'Image Out',
    side: 'output',
    group: 'media',
  },
  {
    key: 'ai',
    field: 'audioInputPrice',
    tierField: 'audio_input_unit_cost',
    label: 'Audio input price',
    shortLabel: 'Audio In',
    side: 'input',
    group: 'media',
  },
  {
    key: 'ao',
    field: 'audioOutputPrice',
    tierField: 'audio_output_unit_cost',
    label: 'Audio output price',
    shortLabel: 'Audio Out',
    side: 'output',
    group: 'media',
  },
]

/** Vars that have real price fields (excludes condition-only vars like `len`) */
export const BILLING_PRICING_VARS: BillingVar[] = BILLING_VARS.filter(
  (v) => !v.isConditionOnly
)

/** Vars valid in tier conditions (`p`, `c`, `len`) */
export const BILLING_CONDITION_VARS: string[] = BILLING_VARS.filter(
  (v) => v.isBase || v.isConditionOnly
).map((v) => v.key)

const BILLING_VAR_KEY_TO_FIELD = Object.fromEntries(
  BILLING_PRICING_VARS.map((v) => [v.key, v.field as string])
) as Record<string, string>

export const BILLING_EXTRA_VARS: BillingVar[] = BILLING_VARS.filter(
  (v) => !v.isBase && !v.isConditionOnly
)

export const BILLING_CACHE_VAR_MAP = BILLING_EXTRA_VARS.map((v) => ({
  field: v.tierField as string,
  exprVar: v.key,
}))

const PRICE_NUMBER_PATTERN = '-?(?:\\d+\\.?\\d*|\\.\\d+)(?:[eE][+-]?\\d+)?'

const BILLING_VAR_REGEX = new RegExp(
  `^(${BILLING_PRICING_VARS.filter((v) => !v.unit || v.unit === 'tokens')
    .map((v) => v.key)
    .join('|')})\\s*\\*\\s*(${PRICE_NUMBER_PATTERN})$`
)

const DURATION_PARAM_PATTERN = 'param\\(\\s*"(?:parameters\\.)?duration"\\s*\\)'
const DURATION_WITH_DEFAULT_PATTERN =
  `\\(\\s*${DURATION_PARAM_PATTERN}\\s*\\?\\?\\s*${PRICE_NUMBER_PATTERN}\\s*\\)`
const PARAM_DURATION_PRICE_REGEX = new RegExp(
  `^(${DURATION_PARAM_PATTERN}|${DURATION_WITH_DEFAULT_PATTERN})\\s*\\*\\s*(${PRICE_NUMBER_PATTERN})\\s*\\*\\s*1000000$`
)

const CONSTANT_REQUEST_PRICE_REGEX = new RegExp(
  `^(${PRICE_NUMBER_PATTERN})\\s*\\*\\s*1000000$`
)

const QUOTA_SCALED_REQUEST_PRICE_REGEX = new RegExp(
  `^(${PRICE_NUMBER_PATTERN})$`
)

// ---------------------------------------------------------------------------
// Request rule constants
// ---------------------------------------------------------------------------

export const SOURCE_PARAM = 'param'
export const SOURCE_HEADER = 'header'
export const SOURCE_TIME = 'time'

export const MATCH_EQ = 'eq'
export const MATCH_CONTAINS = 'contains'
export const MATCH_GT = 'gt'
export const MATCH_GTE = 'gte'
export const MATCH_LT = 'lt'
export const MATCH_LTE = 'lte'
export const MATCH_EXISTS = 'exists'
export const MATCH_RANGE = 'range'

export const TIME_FUNCS = ['hour', 'minute', 'weekday', 'month', 'day'] as const
export type TimeFunc = (typeof TIME_FUNCS)[number]

export const COMMON_TIMEZONES: { value: string; label: string }[] = [
  { value: 'Asia/Shanghai', label: 'UTC+8 Shanghai (Asia/Shanghai)' },
  { value: 'UTC', label: 'UTC' },
  { value: 'America/New_York', label: 'UTC-5 New York (America/New_York)' },
  {
    value: 'America/Los_Angeles',
    label: 'UTC-8 Los Angeles (America/Los_Angeles)',
  },
  { value: 'America/Chicago', label: 'UTC-6 Chicago (America/Chicago)' },
  { value: 'Europe/London', label: 'UTC+0 London (Europe/London)' },
  { value: 'Europe/Berlin', label: 'UTC+1 Berlin (Europe/Berlin)' },
  { value: 'Asia/Tokyo', label: 'UTC+9 Tokyo (Asia/Tokyo)' },
  { value: 'Asia/Singapore', label: 'UTC+8 Singapore (Asia/Singapore)' },
  { value: 'Asia/Seoul', label: 'UTC+9 Seoul (Asia/Seoul)' },
  { value: 'Australia/Sydney', label: 'UTC+10 Sydney (Australia/Sydney)' },
]

const NUMERIC_LITERAL_REGEX = /^-?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?$/

export type ParamHeaderCondition = {
  source: 'param' | 'header'
  path: string
  mode: string
  value: string
  /** Preserve explicitly quoted numeric/boolean strings from saved formulas. */
  valueType?: 'string'
}

export type TimeCondition = {
  source: 'time'
  timeFunc: TimeFunc
  timezone: string
  mode: string
  value: string
  rangeStart: string
  rangeEnd: string
}

export type ExpressionCondition = {
  source: 'expression'
  expression: string
}

export type RequestCondition =
  | TimeCondition
  | ParamHeaderCondition
  | ExpressionCondition

export type RequestRuleGroup = {
  conditions: RequestCondition[]
  multiplier: string
}

export type TierCondition = {
  var: string
  op: '<' | '<=' | '>' | '>='
  value: number
}

export type ParsedTier = {
  label: string
  conditions: (TierCondition | ExpressionCondition)[]
  [field: string]: unknown
}

// ---------------------------------------------------------------------------
// Tier parser
// ---------------------------------------------------------------------------

export function parseDurationPriceTerm(term: string) {
  const match = unwrapOuterParens(term).match(PARAM_DURATION_PRICE_REGEX)
  return match ? { durationExpr: match[1], price: Number(match[2]) } : null
}

function parsePriceTerm(value: string): { key: string; price: number } | null {
  const term = unwrapOuterParens(value)
  const token = term.match(BILLING_VAR_REGEX)
  if (token) return { key: token[1], price: Number(token[2]) }
  const duration = parseDurationPriceTerm(term)
  if (duration) return { key: 'duration', price: duration.price }
  const request = term.match(CONSTANT_REQUEST_PRICE_REGEX)
  if (request) return { key: 'request', price: Number(request[1]) }
  const constant = term.match(QUOTA_SCALED_REQUEST_PRICE_REGEX)
  if (constant) return { key: 'request', price: Number(constant[1]) / 1000000 }
  return null
}

function parseTierBody(bodyStr: string): Record<string, number> | null {
  const tier: Record<string, number> = {}
  for (const part of splitTopLevel(unwrapOuterParens(bodyStr), '+')) {
    const term = parsePriceTerm(part)
    if (!term || !Number.isFinite(term.price) || term.price < 0) return null
    const field = BILLING_VAR_KEY_TO_FIELD[term.key]
    // A duplicate or unrecognized term must never be silently discarded.
    if (Object.hasOwn(tier, field)) return null
    // Missing prices stay absent; explicit zero is a configured free price.
    tier[field] = term.price
  }
  return tier
}

type TierCall = {
  label: string
  body: string
  start: number
  end: number
}

function findTierCalls(expr: string): TierCall[] {
  const calls: TierCall[] = []
  let searchIndex = 0

  while (searchIndex < expr.length) {
    const start = expr.indexOf('tier("', searchIndex)
    if (start === -1) break

    let index = start + 'tier('.length
    let labelEnd: number
    let escaped = false
    for (labelEnd = index + 1; labelEnd < expr.length; labelEnd += 1) {
      const char = expr[labelEnd]
      if (escaped) {
        escaped = false
        continue
      }
      if (char === '\\') {
        escaped = true
        continue
      }
      if (char === '"') break
    }
    if (labelEnd >= expr.length) {
      searchIndex = start + 1
      continue
    }

    const rawLabel = expr.slice(index, labelEnd + 1)
    let label: string
    try {
      label = JSON.parse(rawLabel) as string
    } catch {
      searchIndex = start + 1
      continue
    }

    index = labelEnd + 1
    while (/\s/.test(expr[index] || '')) index += 1
    if (expr[index] !== ',') {
      searchIndex = start + 1
      continue
    }
    index += 1
    while (/\s/.test(expr[index] || '')) index += 1

    const bodyStart = index
    let depth = 0
    let inString = false
    escaped = false
    for (; index < expr.length; index += 1) {
      const char = expr[index]
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
      if (char === '(') {
        depth += 1
        continue
      }
      if (char === ')') {
        if (depth === 0) break
        depth -= 1
      }
    }

    if (index >= expr.length) {
      searchIndex = start + 1
      continue
    }

    calls.push({
      label,
      body: expr.slice(bodyStart, index).trim(),
      start,
      end: index + 1,
    })
    searchIndex = index + 1
  }

  return calls
}

export function parseTiersFromExpr(exprStr: string): ParsedTier[] {
  if (!exprStr) return []
  try {
    const { prefix, body } = splitExprVersion(exprStr.trim())
    if (prefix && prefix !== 'v1:') return []
    const tiers: ParsedTier[] = []
    const collect = (
      value: string,
      conditions: ParsedTier['conditions']
    ): boolean => {
      const text = unwrapOuterParens(value)
      const conditional = splitTopLevelTernary(text)
      if (conditional) {
        const condition: ExpressionCondition = {
          source: 'expression',
          expression: conditional.condition,
        }
        return (
          collect(conditional.whenTrue, [...conditions, condition]) &&
          collect(conditional.whenFalse, [
            ...conditions,
            { source: 'expression', expression: `!(${conditional.condition})` },
          ])
        )
      }
      // Only expose tiers when the complete outer expression is understood.
      if (!/^tier\(/.test(text)) return false
      const calls = findTierCalls(text)
      if (
        calls.length !== 1 ||
        calls[0].start !== 0 ||
        calls[0].end !== text.length
      )
        return false
      const call = calls[0]
      const prices = parseTierBody(call.body)
      if (!prices) return false
      tiers.push({ ...prices, label: call.label, conditions })
      return true
    }
    const split = splitBillingExprAndRequestRules(body)
    return collect(split.billingExpr, []) ? tiers : []
  } catch {
    return []
  }
}

export function normalizeTierLabel(label: string | undefined): string {
  if (!label) return ''
  return label
    .replace(/<[=＝]?|≤|＜[=＝]?/g, '<')
    .replace(/>[=＝]?|≥|＞[=＝]?/g, '>')
    .replace(/\s+/g, '')
    .toLowerCase()
}

// ---------------------------------------------------------------------------
// Request rule parser
// ---------------------------------------------------------------------------

function parseExprLiteral(raw: string): string | null {
  const text = raw.trim()
  if (text === 'true' || text === 'false') return text
  if (NUMERIC_LITERAL_REGEX.test(text)) return text
  try {
    const value: unknown = JSON.parse(text)
    return typeof value === 'string' ? value : null
  } catch {
    return null
  }
}

function tryParseTimeCondition(expr: string): RequestCondition | null {
  let m = expr.match(
    /^(hour|minute|weekday|month|day)\(("(?:[^"\\]|\\.)+")\) >= ([\d.eE+-]+) \|\| \1\(\2\) < ([\d.eE+-]+)$/
  )
  if (m) {
    return {
      source: 'time',
      timeFunc: m[1] as TimeFunc,
      timezone: JSON.parse(m[2]) as string,
      mode: MATCH_RANGE,
      value: '',
      rangeStart: m[3],
      rangeEnd: m[4],
    }
  }
  m = expr.match(
    /^(hour|minute|weekday|month|day)\(("(?:[^"\\]|\\.)+")\) (==|>=|<) ([\d.eE+-]+)$/
  )
  if (m) {
    const opMap: Record<string, string> = {
      '==': MATCH_EQ,
      '>=': MATCH_GTE,
      '<': MATCH_LT,
    }
    return {
      source: 'time',
      timeFunc: m[1] as TimeFunc,
      timezone: JSON.parse(m[2]) as string,
      mode: opMap[m[3]] || MATCH_EQ,
      value: m[4],
      rangeStart: '',
      rangeEnd: '',
    }
  }
  return null
}

function parseSimpleRequestCondition(expr: string): RequestCondition | null {
  const tc = tryParseTimeCondition(expr)
  if (tc) return tc

  let m = expr.match(/^header\(("(?:[^"\\]|\\.)*")\) != ""$/)
  if (m)
    return {
      source: 'header',
      path: JSON.parse(m[1]) as string,
      mode: MATCH_EXISTS,
      value: '',
    }

  m = expr.match(/^param\(("(?:[^"\\]|\\.)*")\) != nil$/)
  if (m)
    return {
      source: 'param',
      path: JSON.parse(m[1]) as string,
      mode: MATCH_EXISTS,
      value: '',
    }

  m = expr.match(
    /^has\(header\(("(?:[^"\\]|\\.)*")\), ((?:"(?:[^"\\]|\\.)*"))\)$/
  )
  if (m)
    return {
      source: 'header',
      path: JSON.parse(m[1]) as string,
      mode: MATCH_CONTAINS,
      value: JSON.parse(m[2]) as string,
    }

  m = expr.match(
    /^param\(("(?:[^"\\]|\\.)*")\) != nil && has\(param\(("(?:[^"\\]|\\.)*")\), ((?:"(?:[^"\\]|\\.)*"))\)$/
  )
  if (m && m[1] === m[2])
    return {
      source: 'param',
      path: JSON.parse(m[1]) as string,
      mode: MATCH_CONTAINS,
      value: JSON.parse(m[3]) as string,
    }

  m = expr.match(
    /^param\(("(?:[^"\\]|\\.)*")\) != nil && param\(("(?:[^"\\]|\\.)*")\) (>|>=|<|<=) ([\d.eE+-]+)$/
  )
  if (m && m[1] === m[2]) {
    const opMap: Record<string, string> = {
      '>': MATCH_GT,
      '>=': MATCH_GTE,
      '<': MATCH_LT,
      '<=': MATCH_LTE,
    }
    return {
      source: 'param',
      path: JSON.parse(m[1]) as string,
      mode: opMap[m[3]],
      value: m[4],
    }
  }

  m = expr.match(/^(param|header)\(("(?:[^"\\]|\\.)*")\) == (.+)$/)
  if (m) {
    const parsedValue = parseExprLiteral(m[3])
    if (parsedValue === null) return null
    return {
      source: m[1] as 'param' | 'header',
      path: JSON.parse(m[2]) as string,
      mode: MATCH_EQ,
      value: parsedValue,
      ...(m[3].trim().startsWith('"') &&
      buildExprLiteral(MATCH_EQ, parsedValue) !== JSON.stringify(parsedValue)
        ? { valueType: 'string' as const }
        : {}),
    }
  }

  return null
}

/** Complex boolean conditions stay intact so OR never becomes stacked discounts. */
export function tryParseRequestCondition(
  value: string
): RequestCondition | null {
  const expr = unwrapOuterParens(value)
  try {
    const simple = parseSimpleRequestCondition(expr)
    if (simple) return simple
  } catch (error) {
    // Expr permits escapes such as \x2d that JSON cannot decode losslessly.
    if (error instanceof SyntaxError) return null
    throw error
  }
  for (const operator of ['||', '&&']) {
    const parts = splitTopLevel(expr, operator)
    if (
      parts.length > 1 &&
      parts.every((part) => tryParseRequestCondition(part))
    ) {
      return { source: 'expression', expression: expr }
    }
  }
  return null
}

function tryParseRuleGroupFactor(part: string): RequestRuleGroup | null {
  const ternary = splitTopLevelTernary(part)
  if (
    !ternary ||
    ternary.whenFalse !== '1' ||
    !NUMERIC_LITERAL_REGEX.test(ternary.whenTrue)
  )
    return null
  const condition = tryParseRequestCondition(ternary.condition)
  if (!condition) return null
  return { conditions: [condition], multiplier: ternary.whenTrue }
}

export function tryParseRequestRuleExpr(
  expr: string
): RequestRuleGroup[] | null {
  const trimmed = (expr || '').trim()
  if (!trimmed) return []

  const parts = splitTopLevel(trimmed, '*')
  const groups: RequestRuleGroup[] = []
  for (const part of parts) {
    const group = tryParseRuleGroupFactor(part)
    if (!group) return null
    groups.push(group)
  }
  return groups
}

// ---------------------------------------------------------------------------
// Combine / split billing expr and request rules
// ---------------------------------------------------------------------------

export function splitBillingExprAndRequestRules(expr: string): {
  billingExpr: string
  requestRuleExpr: string
} {
  const trimmed = (expr || '').trim()
  if (!trimmed) return { billingExpr: '', requestRuleExpr: '' }
  const { prefix, body } = splitExprVersion(trimmed)
  if (prefix && prefix !== 'v1:')
    return { billingExpr: trimmed, requestRuleExpr: '' }

  const parts = splitTopLevelProduct(body)
  if (parts.length <= 1) return { billingExpr: trimmed, requestRuleExpr: '' }

  const ruleParts: string[] = []
  const baseParts: string[] = []

  parts.forEach((part) => {
    const parsed = tryParseRequestRuleExpr(part)
    if (parsed && parsed.length > 0) {
      ruleParts.push(part)
    } else {
      baseParts.push(part)
    }
  })

  if (ruleParts.length === 0 || baseParts.length !== 1) {
    return { billingExpr: trimmed, requestRuleExpr: '' }
  }

  return {
    billingExpr: prefix + unwrapOuterParens(baseParts[0]),
    requestRuleExpr: ruleParts.join(' * '),
  }
}

export function combineBillingExpr(
  baseExpr: string,
  requestRuleExpr: string
): string {
  const base = (baseExpr || '').trim()
  const rules = (requestRuleExpr || '').trim()
  if (!base) return ''
  if (!rules) return base
  const { prefix, body } = splitExprVersion(base)
  return `${prefix}(${body}) * ${rules}`
}

// ---------------------------------------------------------------------------
// Editor: empty constructors
// ---------------------------------------------------------------------------

export function createEmptyCondition(): ParamHeaderCondition {
  return { source: 'param', path: '', mode: MATCH_EQ, value: '' }
}

export function createEmptyTimeCondition(): TimeCondition {
  return {
    source: 'time',
    timeFunc: 'hour',
    timezone: 'Asia/Shanghai',
    mode: MATCH_GTE,
    value: '',
    rangeStart: '',
    rangeEnd: '',
  }
}

export function createEmptyRuleGroup(): RequestRuleGroup {
  return { conditions: [createEmptyCondition()], multiplier: '' }
}

export function createEmptyTimeRuleGroup(): RequestRuleGroup {
  return { conditions: [createEmptyTimeCondition()], multiplier: '' }
}

// ---------------------------------------------------------------------------
// Editor: match option helpers
// ---------------------------------------------------------------------------

export type MatchOption = { value: string; labelKey: string }

export function getRequestRuleMatchOptions(source: string): MatchOption[] {
  if (source === SOURCE_TIME) {
    return [
      { value: MATCH_EQ, labelKey: 'Equals' },
      { value: MATCH_GTE, labelKey: 'Greater than or equal' },
      { value: MATCH_LT, labelKey: 'Less than' },
      { value: MATCH_RANGE, labelKey: 'Overnight range' },
    ]
  }
  const base: MatchOption[] = [
    { value: MATCH_EQ, labelKey: 'Equals' },
    { value: MATCH_CONTAINS, labelKey: 'Contains' },
    { value: MATCH_EXISTS, labelKey: 'Exists' },
  ]
  if (source === SOURCE_HEADER) return base
  return [
    ...base,
    { value: MATCH_GT, labelKey: 'Greater than' },
    { value: MATCH_GTE, labelKey: 'Greater than or equal' },
    { value: MATCH_LT, labelKey: 'Less than' },
    { value: MATCH_LTE, labelKey: 'Less than or equal' },
  ]
}

// ---------------------------------------------------------------------------
// Editor: normalize a single condition
// ---------------------------------------------------------------------------

function isTimeFunc(value: unknown): value is TimeFunc {
  return typeof value === 'string' && TIME_FUNCS.includes(value as TimeFunc)
}

export function normalizeCondition(
  cond: Partial<RequestCondition> | null | undefined
): RequestCondition {
  if (cond?.source === 'expression') {
    return { source: 'expression', expression: cond.expression || '' }
  }
  const source =
    cond?.source === 'time'
      ? 'time'
      : cond?.source === 'header'
        ? 'header'
        : 'param'

  if (source === 'time') {
    const timeCond = cond as Partial<TimeCondition> | null | undefined
    const timeFunc: TimeFunc = isTimeFunc(timeCond?.timeFunc)
      ? timeCond.timeFunc
      : 'hour'
    const options = getRequestRuleMatchOptions(SOURCE_TIME)
    const mode = options.some((item) => item.value === timeCond?.mode)
      ? (timeCond?.mode as string)
      : MATCH_GTE
    return {
      source: 'time',
      timeFunc,
      timezone: timeCond?.timezone || 'Asia/Shanghai',
      mode,
      value: timeCond?.value == null ? '' : String(timeCond.value),
      rangeStart:
        timeCond?.rangeStart == null ? '' : String(timeCond.rangeStart),
      rangeEnd: timeCond?.rangeEnd == null ? '' : String(timeCond.rangeEnd),
    }
  }

  const phCond = cond as Partial<ParamHeaderCondition> | null | undefined
  const options = getRequestRuleMatchOptions(source)
  const mode = options.some((item) => item.value === phCond?.mode)
    ? (phCond?.mode as string)
    : MATCH_EQ
  return {
    source,
    path: phCond?.path || '',
    ...(phCond?.valueType ? { valueType: phCond.valueType } : {}),
    mode,
    value: phCond?.value == null ? '' : String(phCond.value),
  }
}

// ---------------------------------------------------------------------------
// Editor: build expression strings
// ---------------------------------------------------------------------------

function buildExprLiteral(mode: string, value: string): string {
  if (mode === MATCH_CONTAINS) return JSON.stringify(value)
  const text = String(value || '').trim()
  if (text === 'true' || text === 'false') return text
  if (NUMERIC_LITERAL_REGEX.test(text)) return text
  return JSON.stringify(text)
}

function buildTimeConditionExpr(cond: TimeCondition): string {
  const normalized = normalizeCondition(cond) as TimeCondition
  const { timeFunc, timezone, mode } = normalized
  const tz = JSON.stringify(timezone)
  const fn = `${timeFunc}(${tz})`

  if (mode === MATCH_RANGE) {
    const s = normalized.rangeStart.trim()
    const e = normalized.rangeEnd.trim()
    if (!NUMERIC_LITERAL_REGEX.test(s) || !NUMERIC_LITERAL_REGEX.test(e)) {
      return ''
    }
    return `${fn} >= ${s} || ${fn} < ${e}`
  }
  const v = normalized.value.trim()
  if (!NUMERIC_LITERAL_REGEX.test(v)) return ''
  const opMap: Record<string, string> = {
    [MATCH_EQ]: '==',
    [MATCH_GTE]: '>=',
    [MATCH_LT]: '<',
  }
  return `${fn} ${opMap[mode] || '=='} ${v}`
}

export function buildRequestConditionExpr(cond: RequestCondition): string {
  if (cond.source === 'expression') return cond.expression.trim()
  if (cond.source === 'time') return buildTimeConditionExpr(cond)
  const normalized = normalizeCondition(cond) as ParamHeaderCondition
  const path = normalized.path.trim()
  if (!path) return ''

  const sourceExpr =
    normalized.source === 'header'
      ? `header(${JSON.stringify(path)})`
      : `param(${JSON.stringify(path)})`

  switch (normalized.mode) {
    case MATCH_EXISTS:
      return normalized.source === 'header'
        ? `${sourceExpr} != ""`
        : `${sourceExpr} != nil`
    case MATCH_CONTAINS:
      return normalized.source === 'header'
        ? `has(${sourceExpr}, ${buildExprLiteral(normalized.mode, normalized.value)})`
        : `${sourceExpr} != nil && has(${sourceExpr}, ${buildExprLiteral(normalized.mode, normalized.value)})`
    case MATCH_GT:
    case MATCH_GTE:
    case MATCH_LT:
    case MATCH_LTE: {
      const opMap: Record<string, string> = {
        [MATCH_GT]: '>',
        [MATCH_GTE]: '>=',
        [MATCH_LT]: '<',
        [MATCH_LTE]: '<=',
      }
      const numText = String(normalized.value).trim()
      if (!NUMERIC_LITERAL_REGEX.test(numText)) return ''
      return `${sourceExpr} != nil && ${sourceExpr} ${opMap[normalized.mode]} ${numText}`
    }
    case MATCH_EQ:
    default:
      return `${sourceExpr} == ${normalized.valueType === 'string' ? JSON.stringify(normalized.value) : buildExprLiteral(normalized.mode, normalized.value)}`
  }
}

function buildRuleGroupFactor(group: RequestRuleGroup): string {
  const multiplier = (group.multiplier || '').trim()
  if (!NUMERIC_LITERAL_REGEX.test(multiplier)) return ''
  const condExprs = (group.conditions || [])
    .map(buildRequestConditionExpr)
    .filter(Boolean)
  if (condExprs.length === 0) return ''

  const combined =
    condExprs.length === 1
      ? condExprs[0]
      : condExprs.map((e) => `(${e})`).join(' && ')
  return `(${combined} ? ${multiplier} : 1)`
}

export function buildRequestRuleExpr(groups: RequestRuleGroup[]): string {
  return (groups || []).map(buildRuleGroupFactor).filter(Boolean).join(' * ')
}
