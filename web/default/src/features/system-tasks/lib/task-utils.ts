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
import type { SystemTaskType, SystemTaskStatus } from '../types'

/**
 * Get task type display name
 */
export function getTaskTypeLabel(type: SystemTaskType): string {
  const labels: Record<SystemTaskType, string> = {
    channel_test: 'Channel Test',
    model_update: 'Model Update',
    midjourney_poll: 'Midjourney Poll',
    async_task_poll: 'Async Task Poll',
    log_cleanup: 'Log Cleanup',
  }
  return labels[type] || type
}

/**
 * Get task status display info
 */
export function getTaskStatusInfo(status: SystemTaskStatus): {
  label: string
  variant: 'neutral' | 'blue' | 'green' | 'red' | 'yellow'
} {
  const statusMap: Record<
    SystemTaskStatus,
    { label: string; variant: 'neutral' | 'blue' | 'green' | 'red' | 'yellow' }
  > = {
    pending: { label: 'Pending', variant: 'neutral' },
    running: { label: 'Running', variant: 'blue' },
    succeeded: { label: 'Succeeded', variant: 'green' },
    failed: { label: 'Failed', variant: 'red' },
    cancelled: { label: 'Cancelled', variant: 'yellow' },
  }
  return statusMap[status] || { label: status, variant: 'neutral' }
}

/**
 * Format timestamp to relative time
 */
export function formatRelativeTime(timestamp: number): string {
  const now = Math.floor(Date.now() / 1000)
  const diff = now - timestamp

  if (diff < 60) {
    return `${diff}s ago`
  } else if (diff < 3600) {
    return `${Math.floor(diff / 60)}m ago`
  } else if (diff < 86400) {
    return `${Math.floor(diff / 3600)}h ago`
  } else {
    return `${Math.floor(diff / 86400)}d ago`
  }
}

/**
 * Format timestamp to readable date
 */
export function formatTimestamp(timestamp: number): string {
  return new Date(timestamp * 1000).toLocaleString()
}

/**
 * Format duration in seconds to human readable
 */
export function formatDuration(seconds: number): string {
  if (seconds < 60) {
    return `${seconds}s`
  } else if (seconds < 3600) {
    const mins = Math.floor(seconds / 60)
    const secs = seconds % 60
    return `${mins}m ${secs}s`
  } else {
    const hours = Math.floor(seconds / 3600)
    const mins = Math.floor((seconds % 3600) / 60)
    return `${hours}h ${mins}m`
  }
}
