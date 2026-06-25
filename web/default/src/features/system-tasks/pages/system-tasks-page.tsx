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
import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Card } from '@/components/ui/card'
import { StatusBadge } from '@/components/status-badge'
import { listSystemTasks, listSystemInstances } from '../api/system-tasks-api'
import {
  getTaskTypeLabel,
  getTaskStatusInfo,
  formatRelativeTime,
  formatTimestamp,
  formatDuration,
} from '../lib/task-utils'
import type { SystemTask, SystemInstance } from '../types'

export function SystemTasksPage() {
  const { t } = useTranslation()
  const [tasks, setTasks] = useState<SystemTask[]>([])
  const [instances, setInstances] = useState<SystemInstance[]>([])
  const [loading, setLoading] = useState(true)
  const [selectedTask, setSelectedTask] = useState<SystemTask | null>(null)

  const loadData = async () => {
    try {
      setLoading(true)
      const [tasksData, instancesData] = await Promise.all([
        listSystemTasks(50),
        listSystemInstances(),
      ])
      setTasks(tasksData)
      setInstances(instancesData)
    } catch (error) {
      console.error('Failed to load system tasks:', error)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
    // Auto refresh every 10 seconds
    const interval = setInterval(loadData, 10000)
    return () => clearInterval(interval)
  }, [])

  if (loading && tasks.length === 0) {
    return (
      <div className="container mx-auto py-6">
        <div className="mb-6">
          <h1 className="text-2xl font-bold">{t('System Tasks')}</h1>
          <p className="text-neutral-600 dark:text-neutral-400 mt-1">
            {t('Manage and monitor scheduled system tasks')}
          </p>
        </div>
        <div className="mt-6 text-center text-neutral-500">{t('Loading...')}</div>
      </div>
    )
  }

  return (
    <div className="container mx-auto py-6">
      <div className="mb-6">
        <h1 className="text-2xl font-bold">{t('System Tasks')}</h1>
        <p className="text-neutral-600 dark:text-neutral-400 mt-1">
          {t('Manage and monitor scheduled system tasks')}
        </p>
      </div>

      {/* System Instances */}
      <Card className="mt-6 p-6">
        <h2 className="text-lg font-semibold mb-4">{t('Active Instances')}</h2>
        <div className="space-y-2">
          {instances.length === 0 && (
            <div className="text-neutral-500 text-sm">{t('No active instances')}</div>
          )}
          {instances.map((instance) => (
            <div
              key={instance.id}
              className="flex items-center justify-between p-3 bg-neutral-50 dark:bg-neutral-800 rounded"
            >
              <div className="flex-1">
                <div className="font-medium">{instance.runner_id}</div>
                <div className="text-sm text-neutral-500">
                  {instance.hostname} ({instance.ip})
                </div>
              </div>
              <div className="text-sm text-neutral-500">
                <div>{t('Started')}: {formatRelativeTime(instance.started_at)}</div>
                <div>{t('Last heartbeat')}: {formatRelativeTime(instance.last_heartbeat)}</div>
              </div>
              <StatusBadge variant={instance.is_active ? 'green' : 'neutral'}>
                {instance.is_active ? t('Active') : t('Inactive')}
              </StatusBadge>
            </div>
          ))}
        </div>
      </Card>

      {/* Task List */}
      <Card className="mt-6 p-6">
        <h2 className="text-lg font-semibold mb-4">{t('Recent Tasks')}</h2>
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead className="border-b border-neutral-200 dark:border-neutral-700">
              <tr>
                <th className="text-left py-3 px-4 font-medium text-sm">{t('Type')}</th>
                <th className="text-left py-3 px-4 font-medium text-sm">{t('Status')}</th>
                <th className="text-left py-3 px-4 font-medium text-sm">{t('Created')}</th>
                <th className="text-left py-3 px-4 font-medium text-sm">{t('Updated')}</th>
                <th className="text-left py-3 px-4 font-medium text-sm">{t('Duration')}</th>
                <th className="text-right py-3 px-4 font-medium text-sm">{t('Actions')}</th>
              </tr>
            </thead>
            <tbody>
              {tasks.length === 0 && (
                <tr>
                  <td colSpan={6} className="text-center py-8 text-neutral-500">
                    {t('No tasks found')}
                  </td>
                </tr>
              )}
              {tasks.map((task) => {
                const statusInfo = getTaskStatusInfo(task.status)
                const duration = task.updated_at - task.created_at
                return (
                  <tr
                    key={task.id}
                    className="border-b border-neutral-100 dark:border-neutral-800 hover:bg-neutral-50 dark:hover:bg-neutral-800/50"
                  >
                    <td className="py-3 px-4">
                      <div className="font-medium">{t(getTaskTypeLabel(task.type))}</div>
                      <div className="text-xs text-neutral-500">{task.task_id}</div>
                    </td>
                    <td className="py-3 px-4">
                      <StatusBadge variant={statusInfo.variant}>
                        {t(statusInfo.label)}
                      </StatusBadge>
                    </td>
                    <td className="py-3 px-4 text-sm text-neutral-600 dark:text-neutral-400">
                      {formatRelativeTime(task.created_at)}
                    </td>
                    <td className="py-3 px-4 text-sm text-neutral-600 dark:text-neutral-400">
                      {formatRelativeTime(task.updated_at)}
                    </td>
                    <td className="py-3 px-4 text-sm text-neutral-600 dark:text-neutral-400">
                      {formatDuration(duration)}
                    </td>
                    <td className="py-3 px-4 text-right">
                      <button
                        onClick={() => setSelectedTask(task)}
                        className="text-sm text-blue-600 dark:text-blue-400 hover:underline"
                      >
                        {t('Details')}
                      </button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Task Detail Modal */}
      {selectedTask && (
        <div
          className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
          onClick={() => setSelectedTask(null)}
        >
          <Card
            className="w-full max-w-3xl max-h-[80vh] overflow-y-auto m-4"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="p-6">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-xl font-semibold">{t('Task Details')}</h2>
                <button
                  onClick={() => setSelectedTask(null)}
                  className="text-neutral-500 hover:text-neutral-700"
                >
                  ✕
                </button>
              </div>

              <div className="space-y-4">
                <div>
                  <div className="text-sm font-medium text-neutral-500">{t('Task ID')}</div>
                  <div className="font-mono text-sm">{selectedTask.task_id}</div>
                </div>

                <div>
                  <div className="text-sm font-medium text-neutral-500">{t('Type')}</div>
                  <div>{t(getTaskTypeLabel(selectedTask.type))}</div>
                </div>

                <div>
                  <div className="text-sm font-medium text-neutral-500">{t('Status')}</div>
                  <StatusBadge variant={getTaskStatusInfo(selectedTask.status).variant}>
                    {t(getTaskStatusInfo(selectedTask.status).label)}
                  </StatusBadge>
                </div>

                <div>
                  <div className="text-sm font-medium text-neutral-500">{t('Created At')}</div>
                  <div>{formatTimestamp(selectedTask.created_at)}</div>
                </div>

                <div>
                  <div className="text-sm font-medium text-neutral-500">{t('Updated At')}</div>
                  <div>{formatTimestamp(selectedTask.updated_at)}</div>
                </div>

                {selectedTask.error_message && (
                  <div>
                    <div className="text-sm font-medium text-neutral-500">{t('Error')}</div>
                    <div className="text-red-600 dark:text-red-400 font-mono text-sm">
                      {selectedTask.error_message}
                    </div>
                  </div>
                )}

                {selectedTask.state && Object.keys(selectedTask.state).length > 0 && (
                  <div>
                    <div className="text-sm font-medium text-neutral-500 mb-2">{t('State')}</div>
                    <pre className="bg-neutral-100 dark:bg-neutral-800 p-3 rounded text-xs overflow-x-auto">
                      {JSON.stringify(selectedTask.state, null, 2)}
                    </pre>
                  </div>
                )}

                {selectedTask.result && Object.keys(selectedTask.result).length > 0 && (
                  <div>
                    <div className="text-sm font-medium text-neutral-500 mb-2">{t('Result')}</div>
                    <pre className="bg-neutral-100 dark:bg-neutral-800 p-3 rounded text-xs overflow-x-auto">
                      {JSON.stringify(selectedTask.result, null, 2)}
                    </pre>
                  </div>
                )}

                {selectedTask.payload && Object.keys(selectedTask.payload).length > 0 && (
                  <div>
                    <div className="text-sm font-medium text-neutral-500 mb-2">{t('Payload')}</div>
                    <pre className="bg-neutral-100 dark:bg-neutral-800 p-3 rounded text-xs overflow-x-auto">
                      {JSON.stringify(selectedTask.payload, null, 2)}
                    </pre>
                  </div>
                )}
              </div>
            </div>
          </Card>
        </div>
      )}
    </div>
  )
}
