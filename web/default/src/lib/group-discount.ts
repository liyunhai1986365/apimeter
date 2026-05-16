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

function trimTrailingZeros(value: string): string {
  return value.replace(/\.?0+$/, '')
}

function formatPercentNumber(value: number): string {
  const rounded = Math.round(value * 1000) / 1000
  return trimTrailingZeros(rounded.toFixed(3))
}

export type GroupDiscountLabels = {
  originalPrice: string
  fold: string
  percentDiscount: string
  percentPrice: string
  startingFrom: string
}

export const defaultGroupDiscountLabels: GroupDiscountLabels = {
  originalPrice: 'Original price',
  fold: '{{percent}}% off',
  percentDiscount: '{{value}}% off',
  percentPrice: '{{value}}% price',
  startingFrom: 'From {{value}}',
}

function formatTemplate(
  template: string,
  value: string,
  percent = value
): string {
  return template.replace('{{value}}', value).replace('{{percent}}', percent)
}

export function formatGroupDiscount(
  ratio: number | string | null | undefined,
  labels: GroupDiscountLabels = defaultGroupDiscountLabels
): string | undefined {
  if (ratio === undefined || ratio === null || ratio === '') return undefined

  const value = typeof ratio === 'number' ? ratio : Number(ratio)
  if (!Number.isFinite(value)) return undefined

  if (value === 1) return labels.originalPrice

  const percent = value * 100
  if (value > 0 && value < 1 && Math.abs(percent % 10) < 1e-9) {
    return formatTemplate(
      labels.fold,
      formatPercentNumber(percent / 10),
      formatPercentNumber(percent)
    )
  }

  if (value < 1) {
    return formatTemplate(labels.percentDiscount, formatPercentNumber(percent))
  }

  return formatTemplate(labels.percentPrice, formatPercentNumber(percent))
}

function parseRatio(
  ratio: number | string | null | undefined
): number | undefined {
  if (ratio === undefined || ratio === null || ratio === '') return undefined

  const value = typeof ratio === 'number' ? ratio : Number(ratio)
  return Number.isFinite(value) ? value : undefined
}

export function getLowestGroupDiscountSummary(
  enableGroups: string[] | null | undefined,
  groupRatio:
    | Record<string, number | string | null | undefined>
    | null
    | undefined,
  labels: GroupDiscountLabels = defaultGroupDiscountLabels
): string | undefined {
  const groups = Array.isArray(enableGroups) ? enableGroups : []
  const ratios = groupRatio || {}
  const candidateGroups = groups.includes('all') ? Object.keys(ratios) : groups
  const candidates = candidateGroups
    .map((group) => parseRatio(ratios[group]))
    .filter((ratio): ratio is number => ratio !== undefined)

  if (candidates.length === 0) return undefined

  const lowestRatio = Math.min(...candidates)
  const summary = formatGroupDiscount(lowestRatio, labels)
  if (!summary) return undefined

  return candidates.length > 1
    ? formatTemplate(labels.startingFrom, summary)
    : summary
}
