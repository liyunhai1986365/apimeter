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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { formatTokenCount } from './format'

describe('formatTokenCount', () => {
  test('keeps values below one million unabridged', () => {
    assert.equal(formatTokenCount(999_999), '999,999')
  })

  test('uses fixed three-decimal M, B, and T units', () => {
    assert.equal(formatTokenCount(1_234_567), '1.235 M')
    assert.equal(formatTokenCount(2_500_000_000), '2.500 B')
    assert.equal(formatTokenCount(3_000_000_000_000), '3.000 T')
  })

  test('handles negative and invalid values', () => {
    assert.equal(formatTokenCount(-1_250_000), '-1.250 M')
    assert.equal(formatTokenCount(Number.NaN), '-')
  })
})
