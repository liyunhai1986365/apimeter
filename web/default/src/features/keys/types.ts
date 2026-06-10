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
import { z } from 'zod'

// ============================================================================
// API Key Schema & Types
// ============================================================================

export const apiKeySchema = z.object({
  id: z.number(),
  workspace_id: z.number().optional().default(0),
  workspace_name: z.string().nullish().default(''),
  name: z.string(),
  key: z.string(),
  status: z.number(), // 1: enabled, 2: disabled, 3: expired, 4: exhausted
  remain_quota: z.number(),
  used_quota: z.number(),
  today_used_quota: z.number().optional().default(0),
  unlimited_quota: z.boolean(),
  expired_time: z.number(), // -1 for never expires
  created_time: z.number(),
  accessed_time: z.number(),
  group: z.string().nullish().default(''),
  group_policy: z.string().nullish().default(''),
  cross_group_retry: z
    .preprocess((v) => {
      if (v === 1) return true
      if (v === 0) return false
      return v
    }, z.boolean())
    .optional()
    .default(false),
  model_limits_enabled: z.boolean(),
  model_limits: z.string().nullish().default(''),
  allow_ips: z.string().nullish().default(''),
  image_settings: z
    .object({
      format: z
        .enum(['follow_request', 'url', 'b64_json'])
        .optional()
        .default('follow_request'),
      store: z
        .enum(['default', 'only_store_base64', 'force_store_url_and_base64'])
        .optional()
        .default('default'),
    })
    .optional()
    .default({ format: 'follow_request', store: 'default' }),
})

export type ApiKey = z.infer<typeof apiKeySchema>

export const workspaceSchema = z.object({
  id: z.number(),
  user_id: z.number().optional().default(0),
  name: z.string(),
  description: z.string().nullish().default(''),
  is_default: z.boolean(),
  status: z.number(),
  created_time: z.number().optional().default(0),
  updated_time: z.number().optional().default(0),
  token_count: z.number().optional().default(0),
})

export type Workspace = z.infer<typeof workspaceSchema>

export const workspaceUsageStatsSchema = z.object({
  workspace_id: z.number(),
  today_quota: z.number(),
  week_quota: z.number(),
  month_quota: z.number(),
  total_used_quota: z.number(),
})

export type WorkspaceUsageStats = z.infer<typeof workspaceUsageStatsSchema>

export const workspaceQuotaResetConfigSchema = z.object({
  workspace_id: z.number(),
  enabled: z.boolean(),
  period: z.enum(['daily', 'weekly', 'monthly']).default('daily'),
  amount: z.number(),
  last_applied_at: z.number(),
  next_at: z.number(),
})

export type WorkspaceQuotaResetPeriod = 'daily' | 'weekly' | 'monthly'

export type WorkspaceQuotaResetConfig = z.infer<
  typeof workspaceQuotaResetConfigSchema
>

export interface WorkspaceQuotaResetConfigPayload {
  enabled: boolean
  period: WorkspaceQuotaResetPeriod
  amount: number
}

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface GetApiKeysParams {
  p?: number
  size?: number
  workspace_id?: number
}

export interface GetApiKeysResponse {
  success: boolean
  message?: string
  data?: {
    items: ApiKey[]
    total: number
    page: number
    page_size: number
  }
}

export interface GetApiKeySearchResponse {
  success: boolean
  message?: string
  data?:
    | ApiKey[]
    | {
        items: ApiKey[]
        total: number
        page?: number
        page_size?: number
      }
}

export interface SearchApiKeysParams {
  keyword?: string
  token?: string
  p?: number
  size?: number
  workspace_id?: number
}

export interface ApiKeyFormData {
  workspace_id?: number
  name: string
  remain_quota: number
  expired_time: number
  unlimited_quota: boolean
  model_limits_enabled: boolean
  model_limits: string
  allow_ips: string
  group: string
  group_policy?: string
  cross_group_retry: boolean
  image_settings: {
    format: 'follow_request' | 'url' | 'b64_json'
    store: 'default' | 'only_store_base64' | 'force_store_url_and_base64'
  }
}

// ============================================================================
// Dialog Types
// ============================================================================

export type ApiKeysDialogType =
  | 'create'
  | 'update'
  | 'delete'
  | 'batch-delete'
  | 'cc-switch'
  | 'workspace-create'
