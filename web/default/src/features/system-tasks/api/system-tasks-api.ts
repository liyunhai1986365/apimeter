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
import { api } from '@/lib/api'
import {
  normalizePagedData,
  type NormalizedPagedData,
} from '@/lib/paged-response'
import type { SystemTask, SystemInstance } from '../types'

export type ListSystemTasksParams = {
  p?: number
  page_size?: number
}

export function buildSystemTasksListParams(
  page: number,
  pageSize: number
): Required<ListSystemTasksParams> {
  return {
    p: Math.max(1, page),
    page_size: Math.max(1, pageSize),
  }
}

/**
 * List system tasks
 */
export async function listSystemTasks(
  params: ListSystemTasksParams = {}
): Promise<NormalizedPagedData<SystemTask>> {
  const response = await api.get('/api/system/tasks', {
    params,
  })
  return normalizePagedData<SystemTask>(response.data)
}

/**
 * Get a system task by ID
 */
export async function getSystemTask(taskId: string): Promise<SystemTask | null> {
  const response = await api.get(`/api/system/tasks/${taskId}`)
  return response.data?.data || null
}

/**
 * Get current system task by type
 */
export async function getCurrentSystemTask(type: string): Promise<SystemTask | null> {
  const response = await api.get('/api/system/tasks/current', {
    params: { type },
  })
  return response.data?.data || null
}

/**
 * Create log cleanup system task
 */
export async function createLogCleanupTask(targetTimestamp: number): Promise<SystemTask> {
  const response = await api.post('/api/system/tasks', null, {
    params: { target_timestamp: targetTimestamp },
  })
  return response.data?.data
}

/**
 * List system instances
 */
export async function listSystemInstances(): Promise<SystemInstance[]> {
  const response = await api.get('/api/system/info/instances')
  return response.data?.data || []
}
