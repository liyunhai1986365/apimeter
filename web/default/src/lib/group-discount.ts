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

export function formatGroupDiscount(
  ratio: number | string | null | undefined
): string | undefined {
  if (ratio === undefined || ratio === null || ratio === '') return undefined

  const value = typeof ratio === 'number' ? ratio : Number(ratio)
  if (!Number.isFinite(value)) return undefined

  if (value === 1) return '原价'

  const percent = value * 100
  if (value > 0 && value < 1 && Math.abs(percent % 10) < 1e-9) {
    return `${formatPercentNumber(percent / 10)}折`
  }

  if (value < 1) {
    return `${formatPercentNumber(percent)}%折扣`
  }

  return `${formatPercentNumber(percent)}%价格`
}
