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
export function splitExprVersion(value: string): {
  prefix: string
  body: string
} {
  const match = value.match(/^(v\d+:)([\s\S]*)$/)
  return match
    ? { prefix: match[1], body: match[2] }
    : { prefix: '', body: value }
}

function hasFullOuterParens(value: string): boolean {
  const text = value.trim()
  if (!text.startsWith('(') || !text.endsWith(')')) return false
  let depth = 0
  let quote = ''
  let escaped = false
  for (let index = 0; index < text.length; index += 1) {
    const char = text[index]
    if (escaped) {
      escaped = false
      continue
    }
    if (quote && char === '\\') {
      escaped = true
      continue
    }
    if (quote) {
      if (char === quote) quote = ''
      continue
    }
    if (char === '"' || char === "'" || char === '`') {
      quote = char
      continue
    }
    if (char === '(') depth += 1
    if (char === ')') depth -= 1
    if (depth === 0 && index < text.length - 1) return false
  }
  return depth === 0 && !quote
}

export function unwrapOuterParens(value: string): string {
  let text = value.trim()
  while (hasFullOuterParens(text)) text = text.slice(1, -1).trim()
  return text
}

export function splitTopLevel(value: string, operator: string): string[] {
  const parts: string[] = []
  let start = 0
  let depth = 0
  let quote = ''
  let escaped = false
  for (let index = 0; index < value.length; index += 1) {
    const char = value[index]
    if (escaped) {
      escaped = false
      continue
    }
    if (quote && char === '\\') {
      escaped = true
      continue
    }
    if (quote) {
      if (char === quote) quote = ''
      continue
    }
    if (char === '"' || char === "'" || char === '`') {
      quote = char
      continue
    }
    if (char === '(' || char === '[' || char === '{') depth += 1
    if (char === ')' || char === ']' || char === '}') depth -= 1
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
  return parts
}

/** Only extract factors from a product, never from a sum, quotient or branch. */
export function splitTopLevelProduct(value: string): string[] {
  const parts = splitTopLevel(value, '*')
  const isFactor = (part: string) => {
    if (/^-?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?$/.test(part)) return true
    if (/^[A-Za-z_]\w*$/.test(part)) return true
    if (hasFullOuterParens(part)) return true
    const call = part.match(/^[A-Za-z_]\w*\s*(\()/)
    return !!call && hasFullOuterParens(part.slice(call[0].length - 1))
  }
  return parts.every(isFactor) ? parts : [value]
}

export function splitTopLevelTernary(
  value: string
): { condition: string; whenTrue: string; whenFalse: string } | null {
  const text = unwrapOuterParens(value)
  let depth = 0
  let quote = ''
  let escaped = false
  let questionIndex = -1
  let nestedQuestions = 0

  for (let index = 0; index < text.length; index += 1) {
    const char = text[index]
    if (escaped) {
      escaped = false
      continue
    }
    if (quote && char === '\\') {
      escaped = true
      continue
    }
    if (quote) {
      if (char === quote) quote = ''
      continue
    }
    if (char === '"' || char === "'" || char === '`') {
      quote = char
      continue
    }
    if (char === '(' || char === '[' || char === '{') depth += 1
    if (char === ')' || char === ']' || char === '}') depth -= 1
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
