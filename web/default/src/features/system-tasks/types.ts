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
 * System task status
 */
export type SystemTaskStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled'

/**
 * System task type
 */
export type SystemTaskType =
  | 'channel_test'
  | 'model_update'
  | 'midjourney_poll'
  | 'async_task_poll'
  | 'log_cleanup'

/**
 * System task from API
 */
export interface SystemTask {
  id: number
  task_id: string
  type: SystemTaskType
  status: SystemTaskStatus
  payload: Record<string, any> | null
  state: Record<string, any> | null
  result: Record<string, any> | null
  error_message: string
  created_at: number
  updated_at: number
}

/**
 * System instance from API
 */
export interface SystemInstance {
  id: number
  runner_id: string
  hostname: string
  ip: string
  started_at: number
  last_heartbeat: number
  is_active: boolean
  inactive_seconds: number
}
