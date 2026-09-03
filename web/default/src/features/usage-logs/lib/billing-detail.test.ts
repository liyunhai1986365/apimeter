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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'
import { buildBillingDetail } from './billing-detail'

function makeLog(overrides: Partial<UsageLog> = {}): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 0,
    type: 2,
    content: '',
    username: '',
    token_name: '',
    model_name: 'test-model',
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    input_tokens: 0,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    use_time: 0,
    is_stream: false,
    channel: 0,
    channel_name: '',
    token_id: 0,
    group: '',
    ip: '',
    other: '',
    request_id: '',
    upstream_request_id: '',
    ...overrides,
  }
}

describe('buildBillingDetail', () => {
  test('expands the reference token example in the requested order', () => {
    const log = makeLog({
      prompt_tokens: 7048,
      completion_tokens: 2440,
      cache_read_tokens: 6656,
    })
    const other: LogOtherData = {
      model_ratio: 0.1,
      completion_ratio: 6,
      cache_ratio: 0.1,
      group_ratio: 0.25,
      user_group_ratio: 0.25,
    }
    const actualUSD =
      (392 / 1_000_000) * 0.2 * 0.25 +
      (2440 / 1_000_000) * 1.2 * 0.25 +
      (6656 / 1_000_000) * 0.02 * 0.25

    const result = buildBillingDetail(log, other, actualUSD)

    assert.equal(result.mode, 'per-token')
    assert.equal(result.discountLabelKey, 'User Exclusive Discount')
    assert.deepEqual(
      result.lines.map((line) => line.labelKey),
      ['Input', 'Output', 'Cache Read']
    )
    assert.deepEqual(
      result.lines.map((line) => line.quantity),
      [392, 2440, 6656]
    )
    assert.ok(Math.abs((result.lines[0].unitPriceUSD || 0) - 0.2) < 1e-12)
    assert.ok(Math.abs((result.lines[1].unitPriceUSD || 0) - 1.2) < 1e-12)
    assert.ok(Math.abs((result.lines[2].unitPriceUSD || 0) - 0.02) < 1e-12)
    assert.ok(Math.abs(result.originalAmountUSD - actualUSD / 0.25) < 1e-12)
    assert.ok(Math.abs(result.finalAmountUSD - actualUSD) < 1e-12)
  })

  test('shows cache write splits before cache reads', () => {
    const log = makeLog({
      prompt_tokens: 1170,
      completion_tokens: 100,
      cache_read_tokens: 100,
      cache_write_tokens: 70,
    })
    const other: LogOtherData = {
      model_ratio: 0.5,
      completion_ratio: 2,
      cache_ratio: 0.1,
      cache_creation_ratio: 1.25,
      cache_creation_ratio_5m: 1.25,
      cache_creation_ratio_1h: 2,
      cache_creation_tokens: 70,
      cache_creation_tokens_5m: 50,
      cache_creation_tokens_1h: 20,
      group_ratio: 0.5,
    }
    const actualUSD =
      ((1000 + 100 * 2 + 50 * 1.25 + 20 * 2 + 100 * 0.1) / 1_000_000) * 0.5

    const result = buildBillingDetail(log, other, actualUSD)

    assert.deepEqual(
      result.lines.map((line) => line.labelKey),
      ['Input', 'Output', 'Cache Write (5m)', 'Cache Write (1h)', 'Cache Read']
    )
    assert.notEqual(result.lines.at(-1)?.key, 'other')
  })

  test('supports fixed per-request pricing', () => {
    const log = makeLog()
    const other: LogOtherData = {
      model_price: 0.04,
      group_ratio: 0.25,
    }

    const result = buildBillingDetail(log, other, 0.01)

    assert.equal(result.mode, 'per-call')
    assert.equal(result.discountLabelKey, 'Billing discount')
    assert.ok(Math.abs(result.originalAmountUSD - 0.04) < 1e-12)
    assert.equal(result.lines.length, 1)
    assert.equal(result.lines[0].labelKey, 'Per request')
    assert.equal(result.lines[0].quantity, 1)
    assert.equal(result.lines[0].divisor, 1)
    assert.equal(result.lines[0].unitPriceUSD, 0.04)
    assert.equal(result.lines[0].finalAmountUSD, 0.01)
  })

  test('expands dynamic pricing and applies matched request rules', () => {
    const expression = 'tier("base", p * 0.2 + c * 1.2 + cr * 0.02 + cc * 0.25)'
    const log = makeLog({
      prompt_tokens: 1000,
      completion_tokens: 200,
      cache_read_tokens: 100,
      cache_write_tokens: 50,
    })
    const other: LogOtherData = {
      billing_mode: 'tiered_expr',
      expr_b64: Buffer.from(expression).toString('base64'),
      matched_tier: 'base',
      cache_tokens: 100,
      cache_creation_tokens: 50,
      group_ratio: 0.5,
      request_rules: [
        {
          cond: 'param("service_tier") == "fast"',
          multiplier: 2,
          matched: true,
        },
      ],
    }
    const baseUSD = (850 * 0.2 + 200 * 1.2 + 50 * 0.25 + 100 * 0.02) / 1_000_000
    const actualUSD = baseUSD * 2 * 0.5

    const result = buildBillingDetail(log, other, actualUSD)

    assert.equal(result.mode, 'dynamic')
    assert.equal(result.matchedTier, 'base')
    assert.deepEqual(
      result.lines.map((line) => line.labelKey),
      ['Input', 'Output', 'Cache Write', 'Cache Read']
    )
    assert.ok(result.lines.every((line) => line.factors[0]?.value === 2))
    assert.notEqual(result.lines.at(-1)?.key, 'other')
  })

  test('shows tool surcharges and reconciles unknown fees to the actual charge', () => {
    const log = makeLog()
    const other: LogOtherData = {
      group_ratio: 0.25,
      tool_surcharges: [{ name: 'web_search', count: 2, price: 10 }],
    }

    const result = buildBillingDetail(log, other, 0.006)

    assert.equal(result.lines[0].labelKey, 'Web Search')
    assert.equal(result.lines[0].quantity, 2)
    assert.equal(result.lines[0].divisor, 1000)
    assert.equal(result.lines[0].unitPriceUSD, 10)
    assert.equal(result.lines[0].finalAmountUSD, 0.005)
    assert.equal(result.lines[1].key, 'other')
    assert.equal(result.lines[1].labelKey, 'Other')
    assert.equal(result.lines[1].originalAmountUSD, 0.004)
    assert.ok(Math.abs(result.lines[1].finalAmountUSD - 0.001) < 1e-12)
    assert.equal(result.lines[1].amountOnly, true)
  })
})
