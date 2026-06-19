import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { PerformanceGroup } from '@/features/performance-metrics/types'
import {
  toGroupedLatencySeries,
  toGroupedUptimeSeries,
} from './performance-series'

const groups: PerformanceGroup[] = [
  {
    group: 'default',
    avg_ttft_ms: 100,
    avg_latency_ms: 200,
    success_rate: 99,
    avg_tps: 10,
    series: [
      {
        ts: 100,
        avg_ttft_ms: 100,
        avg_latency_ms: 200,
        success_rate: 99,
        avg_tps: 10,
      },
    ],
  },
  {
    group: 'vip',
    avg_ttft_ms: 50,
    avg_latency_ms: 100,
    success_rate: 100,
    avg_tps: 20,
    series: [
      {
        ts: 100,
        avg_ttft_ms: 50,
        avg_latency_ms: 100,
        success_rate: 100,
        avg_tps: 20,
      },
    ],
  },
]

describe('performance trend series', () => {
  test('keeps latency and availability trends separated by supplier name', () => {
    const usableGroup = {
      default: '官方供应商',
      vip: { desc: '高速供应商' },
    }

    assert.deepEqual(
      toGroupedLatencySeries(groups, usableGroup).map((point) => ({
        group: point.group,
        ttft_ms: point.ttft_ms,
      })),
      [
        { group: '官方供应商', ttft_ms: 100 },
        { group: '高速供应商', ttft_ms: 50 },
      ]
    )

    assert.deepEqual(
      toGroupedUptimeSeries(groups, usableGroup).map((point) => ({
        group: point.group,
        uptime_pct: point.uptime_pct,
        incidents: point.incidents,
      })),
      [
        { group: '官方供应商', uptime_pct: 99, incidents: 1 },
        { group: '高速供应商', uptime_pct: 100, incidents: 0 },
      ]
    )
  })
})
