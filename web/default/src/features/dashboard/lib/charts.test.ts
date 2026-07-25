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
import { processTokenChartData } from './charts'
import { calculateDashboardStats } from './stats'

describe('calculateDashboardStats', () => {
  test('keeps total and cache token metrics separate', () => {
    assert.deepEqual(
      calculateDashboardStats([
        {
          created_at: 1_700_000_000,
          token_used: 160,
          cache_token_used: 40,
          count: 1,
          quota: 100,
        },
        {
          created_at: 1_700_003_600,
          token_used: 120,
          cache_token_used: 30,
          count: 2,
          quota: 200,
        },
      ]),
      {
        totalQuota: 300,
        totalCount: 3,
        totalTokens: 280,
        totalCacheTokens: 70,
      }
    )
  })
})

describe('processTokenChartData', () => {
  test('builds current-user token ranking and trend specs from token aggregates', () => {
    const charts = processTokenChartData(
      [
        {
          created_at: 1_700_000_000,
          token_id: 11,
          token_name: 'primary',
          quota: 100,
          count: 2,
          token_used: 40,
        },
        {
          created_at: 1_700_000_000,
          token_id: 11,
          token_name: 'primary',
          quota: 70,
          count: 3,
          token_used: 30,
        },
        {
          created_at: 1_700_003_600,
          token_id: 22,
          token_name: 'worker',
          quota: 25,
          count: 1,
          token_used: 10,
        },
      ],
      'hour',
      (key, options) =>
        key === 'Deleted token #{{id}}'
          ? `Deleted token #${String(options?.id ?? '')}`
          : key,
      10
    )

    assert.deepEqual(charts.spec_token_rank.data[0].values, [
      { Token: 'primary', rawQuota: 170, Usage: 170 },
      { Token: 'worker', rawQuota: 25, Usage: 25 },
    ])
    const trendValues = charts.spec_token_trend.data[0].values
    const primaryPoint = trendValues.find(
      (item: { Token: string; rawQuota: number }) =>
        item.Token === 'primary' && item.rawQuota === 170
    )
    const workerPoint = trendValues.find(
      (item: { Token: string; rawQuota: number }) =>
        item.Token === 'worker' && item.rawQuota === 25
    )
    assert.ok(primaryPoint)
    assert.ok(workerPoint)
    assert.ok(
      trendValues.some(
        (item: {
          Time: string
          Token: string
          rawQuota: number
          Usage: number
        }) =>
          item.Time === primaryPoint.Time &&
          item.Token === 'worker' &&
          item.rawQuota === 0 &&
          item.Usage === 0
      )
    )
    assert.ok(
      trendValues.some(
        (item: {
          Time: string
          Token: string
          rawQuota: number
          Usage: number
        }) =>
          item.Time === workerPoint.Time &&
          item.Token === 'primary' &&
          item.rawQuota === 0 &&
          item.Usage === 0
      )
    )
  })
})
