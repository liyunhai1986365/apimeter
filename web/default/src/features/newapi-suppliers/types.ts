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

export type NewAPISupplierStatus = 0 | 1 | 2

export interface NewAPISupplierGroupSnapshot {
  group: string
  models: string[]
  source: string
  ratio?: string
  desc?: string
  model_providers?: Record<string, string[]>
}

export interface NewAPISupplier {
  id: number
  name: string
  base_url: string
  username: string
  password?: string
  access_token?: string
  api_key?: string
  upstream_user_id: number
  upstream_username: string
  status: NewAPISupplierStatus
  quota: number
  used_quota: number
  groups_json: string
  group_models_json: string
  group_keys_json?: string
  model_source: string
  default_local_group: string
  selected_models_json: string
  tag: string
  channel_type: number
  balance_threshold: number
  last_check_time: number
  last_configure_time: number
  last_error: string
  remark: string
  bound_channel_count: number
}

export interface NewAPISupplierCheckResult {
  supplier_id: number
  quota: number
  used_quota: number
  groups: string[]
  group_ratios: Record<string, string>
  group_models: NewAPISupplierGroupSnapshot[]
  group_keys: Record<string, string>
  model_source: string
  upstream_user_id: number
  username: string
}

export interface NewAPISupplierBalanceResult {
  supplier_id: number
  quota: number
  used_quota: number
  upstream_user_id: number
  username: string
}

export interface NewAPISupplierChannelProfile {
  id: number
  supplier_id: number
  supplier_name_snapshot: string
  base_url_snapshot: string
  upstream_group: string
  upstream_group_desc: string
  upstream_group_ratio: string
  model_source: string
  channel_type: number
  endpoint_type: string
  local_group: string
  channel_name_template: string
  tag: string
  weight?: number
  priority?: number
  auto_ban: number
  balance_threshold: number
  channel_ratio?: number
  channel_id?: number
  sync_mode: string
  sync_status: string
  channel_status: string
  config_hash: string
  model_set_hash: string
  last_checked_at: number
  last_synced_at: number
  last_error: string
  model_names?: string[]
}

export interface NewAPISupplierChannelProfileModel {
  id: number
  profile_id: number
  supplier_id: number
  upstream_group: string
  model_name: string
  model_provider: string
  available_status: string
  last_test_success: boolean
  last_test_time: number
  last_response_time: number
  last_error: string
}

export interface NewAPISupplierChannelProfileUpdateRequest {
  local_group: string
  channel_type: number
  channel_name_template: string
  tag: string
  weight?: number
  priority?: number
  auto_ban: number
  balance_threshold: number
  channel_ratio?: number
  sync_mode: string
  sync_status: string
  channel_status: string
}

export interface SupplierProfileSyncResult {
  profiles: NewAPISupplierChannelProfile[]
  created: number
  updated: number
}

export interface ConfigureItem {
  upstream_group: string
  local_group: string
  models: string[]
  channel_type?: number
  channel_name?: string
}

export interface ConfiguredChannel {
  channel_id: number
  name: string
  upstream_group: string
  channel_type?: number
  local_group: string
  models: string
  created: boolean
}

export interface TestModelRequest {
  upstream_group: string
  model: string
  channel_type?: number
  endpoint_type?: string
  stream?: boolean
}

export interface TestModelResult {
  success: boolean
  message?: string
  upstream_group: string
  model: string
  time: number
  error_code?: string
  profile_model?: NewAPISupplierChannelProfileModel
}

export interface SupplierChannelProfileBatchTestResult {
  profile_id: number
  supplier_id: number
  upstream_group: string
  channel_status: string
  total: number
  passed: number
  failed: number
  results: TestModelResult[]
}

export interface SupplierChannelProfileAllTestResult {
  total: number
  passed: number
  failed: number
  profiles: Array<{
    profile_id: number
    supplier_id: number
    upstream_group: string
    channel_status?: string
    total?: number
    passed?: number
    failed?: number
    success: boolean
    message?: string
    model_results?: TestModelResult[]
  }>
}

export interface SupplierChannelProfileListParams {
  p?: number
  page_size?: number
  supplier_id?: number
  supplier?: string
  upstream_group?: string
  local_group?: string
  managed_channel?: 'linked' | 'unlinked'
  sync_status?: string
  channel_status?: string
  model?: string
}

export interface SupplierChannelProfileBatchTestRequest {
  stream?: boolean
  models?: string[]
}

export interface SupplierChannelProfileAllTestRequest {
  stream?: boolean
  profile_ids?: number[]
}

export interface PagedResponse<T> {
  success: boolean
  message?: string
  data?: {
    items: T[]
    total: number
    page: number
    page_size: number
  }
}

export interface ApiResponse<T> {
  success: boolean
  message?: string
  data?: T
}
