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
import type { PerformanceGroup } from '@/features/performance-metrics/types'
import type { UptimeDayPoint } from './mock-stats'

type UsableGroupMap = Record<string, string | { desc?: string; ratio?: number }>

export type GroupedLatencyPoint = {
  timestamp: string
  group: string
  ttft_ms: number
}

export type GroupedUptimePoint = UptimeDayPoint & {
  group: string
}

export function getPerformanceGroupDisplayName(
  group: string,
  _usableGroup: UsableGroupMap
) {
  return group
}

export function toGroupedLatencySeries(
  groups: PerformanceGroup[],
  usableGroup: UsableGroupMap
): GroupedLatencyPoint[] {
  return groups
    .flatMap((group) => {
      const displayName = getPerformanceGroupDisplayName(
        group.group,
        usableGroup
      )
      return group.series
        .filter((point) => point.avg_ttft_ms > 0)
        .map((point) => ({
          timestamp: new Date(point.ts * 1000).toISOString(),
          group: displayName,
          ttft_ms: Math.round(point.avg_ttft_ms),
        }))
    })
    .sort(
      (a, b) =>
        new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
    )
}

export function toGroupedUptimeSeries(
  groups: PerformanceGroup[],
  usableGroup: UsableGroupMap
): GroupedUptimePoint[] {
  return groups
    .flatMap((group) => {
      const displayName = getPerformanceGroupDisplayName(
        group.group,
        usableGroup
      )
      return group.series.map((point) => ({
        date: new Date(point.ts * 1000).toISOString(),
        group: displayName,
        uptime_pct: Math.round(point.success_rate * 100) / 100,
        incidents: point.success_rate < 100 ? 1 : 0,
        outage_minutes: 0,
      }))
    })
    .sort((a, b) => new Date(a.date).getTime() - new Date(b.date).getTime())
}

export function toGroupUptimeSeries(group: PerformanceGroup): UptimeDayPoint[] {
  return group.series.map((point) => ({
    date: new Date(point.ts * 1000).toISOString(),
    uptime_pct: Math.round(point.success_rate * 100) / 100,
    incidents: point.success_rate < 100 ? 1 : 0,
    outage_minutes: 0,
  }))
}
