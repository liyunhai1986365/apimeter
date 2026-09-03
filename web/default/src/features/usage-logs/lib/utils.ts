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
 * Utility functions for usage logs feature
 */
import {
  getAllLogs,
  getUserLogs,
  getAllMidjourneyLogs,
  getUserMidjourneyLogs,
  getAllTaskLogs,
  getUserTaskLogs,
  getChannelOperationRecords,
  getRetryRouteEvents,
} from '../api'
import {
  LOG_TYPES,
  DISPLAYABLE_LOG_TYPES,
  TIMING_LOG_TYPES,
} from '../constants'
import type {
  GetLogsParams,
  GetLogsResponse,
  FetchLogsConfig,
  GetMidjourneyLogsParams,
  GetRetryRouteEventsParams,
  GetTaskLogsParams,
} from '../types'

// ============================================================================
// Type Checkers & Utilities
// ============================================================================

/**
 * Check if log type is displayable (has detailed info)
 */
export function isDisplayableLogType(type: number): boolean {
  return (DISPLAYABLE_LOG_TYPES as readonly number[]).includes(type)
}

/**
 * Check if log type shows timing info
 */
export function isTimingLogType(type: number): boolean {
  return (TIMING_LOG_TYPES as readonly number[]).includes(type)
}

/**
 * Get log type configuration by type number
 */
export function getLogTypeConfig(type: number) {
  return LOG_TYPES.find((t) => t.value === type) || LOG_TYPES[0]
}

/**
 * Check if log uses per-call billing
 */
export function isPerCallBilling(modelPrice?: number): boolean {
  return (modelPrice ?? 0) > 0
}

/**
 * Get default time range (today 00:00:00 to now + 1 hour)
 */
export function getDefaultTimeRange(): { start: Date; end: Date } {
  const now = new Date()
  const start = new Date(now)
  start.setHours(0, 0, 0, 0)
  const end = new Date(now.getTime() + 3600 * 1000) // +1 hour

  return { start, end }
}

/**
 * Convert milliseconds timestamp to seconds for API
 */
function timestampToSeconds(ms: number): number {
  return Math.floor(ms / 1000)
}

/**
 * Build query parameters from filters
 */
export function buildQueryParams(
  params: Record<string, unknown>
): URLSearchParams {
  const queryParams = new URLSearchParams()

  Object.entries(params).forEach(([key, value]) => {
    // Keep 0 as a valid value, only filter out undefined, null, and empty string
    if (value !== undefined && value !== null && value !== '') {
      queryParams.append(key, String(value))
    }
  })

  return queryParams
}

export function normalizeNumericFilterValue(
  value: unknown
): string | undefined {
  if (value === undefined || value === null || value === '') return undefined
  const raw = String(value).trim()
  if (!raw) return undefined
  const unquoted = raw.replace(/^["'](.+)["']$/, '$1').trim()
  if (!unquoted) return undefined
  const numeric = Number(unquoted)
  return Number.isFinite(numeric) ? String(numeric) : undefined
}

/**
 * Build time range parameters with default values
 * Shared logic for all log types
 */
function buildTimeRangeParams(
  searchParams: Record<string, unknown>,
  useMilliseconds: boolean
): { start_timestamp?: number; end_timestamp?: number } {
  const hasTimeParams = searchParams.startTime ?? searchParams.endTime
  const defaultTimeRange = !hasTimeParams ? getDefaultTimeRange() : null

  const convertTimestamp = (timestamp: number) =>
    useMilliseconds ? timestamp : timestampToSeconds(timestamp)

  const getTimestamp = (paramTime?: unknown, defaultTime?: Date) => {
    const time = (paramTime as number) || defaultTime?.getTime()
    return time ? convertTimestamp(time) : undefined
  }

  return {
    start_timestamp: getTimestamp(
      searchParams.startTime,
      defaultTimeRange?.start
    ),
    end_timestamp: getTimestamp(searchParams.endTime, defaultTimeRange?.end),
  }
}

/**
 * Build base parameters with time range (for drawing and task logs)
 * @param useMilliseconds - Whether to use millisecond timestamps (true for drawing logs, false for task logs)
 */
export function buildBaseParams(config: {
  page: number
  pageSize: number
  searchParams: Record<string, unknown>
  useMilliseconds?: boolean
}): {
  p: number
  page_size: number
  channel_id?: string
  start_timestamp?: number
  end_timestamp?: number
} {
  const { page, pageSize, searchParams, useMilliseconds = false } = config
  const channelId = normalizeNumericFilterValue(searchParams.channel)

  return {
    p: page,
    page_size: pageSize,
    ...(channelId
      ? {
          channel_id: channelId,
        }
      : {}),
    ...buildTimeRangeParams(searchParams, useMilliseconds),
  }
}

/**
 * Build API params from search params and column filters (for common logs)
 */
export function buildApiParams(config: {
  page: number
  pageSize: number
  cursor?: number
  cursorMode?: boolean
  searchParams: Record<string, unknown>
  columnFilters?: Array<{ id: string; value: unknown }>
  isAdmin: boolean
}): GetLogsParams {
  const {
    page,
    pageSize,
    cursor,
    cursorMode = false,
    searchParams,
    columnFilters = [],
    isAdmin,
  } = config
  const channelId = normalizeNumericFilterValue(searchParams.channel)

  // Helper to process type parameter (single value from array)
  const processType = (value: unknown) => {
    if (Array.isArray(value) && value.length === 1) {
      return Number(value[0])
    }
    return undefined
  }

  // Build base params from search params
  const params: GetLogsParams = {
    p: page,
    page_size: pageSize,
    ...(cursorMode ? { cursor_mode: 1 as const } : {}),
    ...(cursorMode && cursor ? { cursor } : {}),
    ...(searchParams.type ? { type: processType(searchParams.type) } : {}),
    ...(searchParams.model ? { model_name: String(searchParams.model) } : {}),
    ...(searchParams.token ? { token_name: String(searchParams.token) } : {}),
    ...(searchParams.workspace
      ? { workspace_name: String(searchParams.workspace) }
      : {}),
    ...(searchParams.group ? { group: String(searchParams.group) } : {}),
    ...(isAdmin && channelId ? { channel: Number(channelId) } : {}),
    ...(isAdmin && searchParams.username
      ? { username: String(searchParams.username) }
      : {}),
    ...(searchParams.requestId
      ? { request_id: String(searchParams.requestId) }
      : {}),
    ...(searchParams.upstreamRequestId
      ? { upstream_request_id: String(searchParams.upstreamRequestId) }
      : {}),
    ...buildTimeRangeParams(searchParams, false),
  }

  // Override with column filters if present
  if (columnFilters.length > 0) {
    columnFilters.forEach(({ id, value }) => {
      if (value === undefined || value === null || value === '') return

      switch (id) {
        case 'type':
          params.type = processType(value)
          break
        case 'model_name':
          params.model_name = String(value)
          break
        case 'token_name':
          params.token_name = String(value)
          break
        case 'workspace_name':
          params.workspace_name = String(value)
          break
        case 'group':
          params.group = String(value)
          break
        case 'channel':
          if (isAdmin) {
            const channelFilter = normalizeNumericFilterValue(value)
            if (channelFilter) params.channel = Number(channelFilter)
          }
          break
        case 'username':
          if (isAdmin) params.username = String(value)
          break
      }
    })
  }

  return params
}

// ============================================================================
// Data Fetching
// ============================================================================

/**
 * Fetch logs based on category type
 */
export async function fetchLogsByCategory(
  config: FetchLogsConfig
): Promise<GetLogsResponse> {
  const {
    logCategory,
    isAdmin,
    page,
    pageSize,
    cursor,
    searchParams,
    columnFilters,
  } = config

  if (logCategory === 'common') {
    const params = buildApiParams({
      page,
      pageSize,
      cursor,
      cursorMode: true,
      searchParams,
      columnFilters,
      isAdmin,
    })
    return isAdmin ? await getAllLogs(params) : await getUserLogs(params)
  }

  if (logCategory === 'channel-operations') {
    const baseParams = buildBaseParams({
      page,
      pageSize,
      searchParams,
    })
    return await getChannelOperationRecords({
      p: baseParams.p,
      page_size: baseParams.page_size,
      channel: baseParams.channel_id
        ? Number(baseParams.channel_id) || undefined
        : undefined,
      start_timestamp: baseParams.start_timestamp,
      end_timestamp: baseParams.end_timestamp,
      model_name: searchParams.model ? String(searchParams.model) : undefined,
    })
  }

  if (logCategory === 'retry-route-events') {
    const baseParams = buildBaseParams({
      page,
      pageSize,
      searchParams,
    })
    const sourceChannelId = normalizeNumericFilterValue(searchParams.channel)
    const tokenId = normalizeNumericFilterValue(searchParams.token)
    const params: GetRetryRouteEventsParams = {
      p: baseParams.p,
      page_size: baseParams.page_size,
      start_timestamp: baseParams.start_timestamp,
      end_timestamp: baseParams.end_timestamp,
      request_id: searchParams.requestId
        ? String(searchParams.requestId)
        : undefined,
      username:
        isAdmin && searchParams.username
          ? String(searchParams.username)
          : undefined,
      token_name:
        searchParams.token && !tokenId ? String(searchParams.token) : undefined,
      token_id: tokenId ? Number(tokenId) : undefined,
      model_name: searchParams.model ? String(searchParams.model) : undefined,
      action: searchParams.filter ? String(searchParams.filter) : undefined,
      source_channel_id: sourceChannelId ? Number(sourceChannelId) : undefined,
      target_group: searchParams.group ? String(searchParams.group) : undefined,
      final_status: searchParams.status
        ? String(searchParams.status)
        : undefined,
    }
    return await getRetryRouteEvents(params)
  }

  // For drawing and task logs
  const baseParams = buildBaseParams({
    page,
    pageSize,
    searchParams,
    useMilliseconds: logCategory === 'drawing',
  })

  const paramsWithFilter = {
    ...baseParams,
    ...(logCategory === 'drawing'
      ? { mj_id: searchParams.filter as string | undefined }
      : {}),
    ...(logCategory === 'task'
      ? {
          task_id: searchParams.filter as string | undefined,
          token_name: searchParams.token as string | undefined,
          workspace_name: searchParams.workspace as string | undefined,
        }
      : {}),
  }

  if (logCategory === 'drawing') {
    return isAdmin
      ? await getAllMidjourneyLogs(paramsWithFilter as GetMidjourneyLogsParams)
      : await getUserMidjourneyLogs(paramsWithFilter as GetMidjourneyLogsParams)
  }

  // task logs
  return isAdmin
    ? await getAllTaskLogs(paramsWithFilter as GetTaskLogsParams)
    : await getUserTaskLogs(paramsWithFilter as GetTaskLogsParams)
}
