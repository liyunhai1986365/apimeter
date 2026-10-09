import axios from 'axios'
import { QueryClient, QueryObserver } from '@tanstack/react-query'
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { toast } from 'sonner'
import { getTaskLogDetail } from '@/features/usage-logs/api'
import { api } from './api'

test('task details load after the query is unmounted and immediately remounted', async (t) => {
  const originalAdapter = api.defaults.adapter
  const client = new QueryClient()
  const detail = { task_id: 'task_remount', status: 'SUCCESS' }
  api.defaults.adapter = async (config) => ({
    config,
    data: { success: true, data: detail },
    headers: {},
    status: 200,
    statusText: 'OK',
  })
  t.after(() => {
    client.clear()
    api.defaults.adapter = originalAdapter
  })
  const errors = t.mock.method(toast, 'error', () => '')
  const observer = new QueryObserver(client, {
    queryKey: ['task-log-detail', true, detail.task_id],
    queryFn: ({ signal }) => getTaskLogDetail(detail.task_id, true, signal),
    gcTime: 0,
    retry: false,
  })

  const unsubscribe = observer.subscribe(() => {})
  unsubscribe()
  const result = await new Promise<
    ReturnType<typeof observer.getCurrentResult>
  >((resolve) => {
    t.after(
      observer.subscribe((value) => {
        if (!value.isFetching) resolve(value)
      })
    )
  })

  assert.equal(result.status, 'success')
  assert.deepEqual(result.data, detail)
  assert.equal(errors.mock.callCount(), 0)
})

test('ordinary GET requests still share an in-flight response and then refresh', async (t) => {
  const originalAdapter = api.defaults.adapter
  let calls = 0
  api.defaults.adapter = async (config) => ({
    config,
    data: ++calls,
    headers: {},
    status: 200,
    statusText: 'OK',
  })
  t.after(() => {
    api.defaults.adapter = originalAdapter
  })

  const [first, second] = await Promise.all([
    api.get('/api/dedup-regression'),
    api.get('/api/dedup-regression'),
  ])
  assert.equal(first.data, 1)
  assert.equal(second.data, 1)
  const next = await api.get('/api/dedup-regression')
  assert.equal(next.data, 2)
})

test('canceling a GET does not cancel another caller without a signal', async (t) => {
  const originalAdapter = api.defaults.adapter
  api.defaults.adapter = async (config) => ({
    config,
    data: 'loaded',
    headers: {},
    status: 200,
    statusText: 'OK',
  })
  t.after(() => {
    api.defaults.adapter = originalAdapter
  })
  const errors = t.mock.method(toast, 'error', () => '')
  const controller = new AbortController()
  const canceled = api.get('/api/cancel-regression', {
    signal: controller.signal,
  })
  controller.abort()
  const active = api.get('/api/cancel-regression')
  const [canceledResult, activeResult] = await Promise.allSettled([
    canceled,
    active,
  ])

  assert.equal(canceledResult.status, 'rejected')
  assert.ok(axios.isCancel(canceledResult.reason))
  assert.equal(activeResult.status, 'fulfilled')
  assert.equal(activeResult.value.data, 'loaded')
  assert.equal(errors.mock.callCount(), 0)
})
