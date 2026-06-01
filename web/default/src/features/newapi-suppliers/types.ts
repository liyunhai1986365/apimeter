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
