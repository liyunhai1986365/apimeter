import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { UsageLog } from '../data/schema'
import {
  buildCacheBillingRows,
  formatSensitiveQuota,
  formatSignedLogQuota,
  isTaskPreConsumeLog,
} from './format'

function usageLog(overrides: Partial<UsageLog>): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1779955200,
    type: 2,
    content: '',
    username: '',
    token_name: '',
    model_name: '',
    quota: 100,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    is_stream: false,
    channel: 0,
    channel_name: '',
    token_id: 0,
    group: '',
    ip: '',
    other: '',
    request_id: '',
    upstream_request_id: '',
    ...overrides,
  }
}

describe('isTaskPreConsumeLog', () => {
  test('marks Seedance 2 task consume logs as pre-consumed charges', () => {
    assert.equal(
      isTaskPreConsumeLog(
        usageLog({
          model_name: 'doubao-seedance-2.0',
          other: JSON.stringify({
            is_task: true,
            request_path: '/api/v3/contents/generations/tasks',
          }),
        })
      ),
      true
    )

    assert.equal(
      isTaskPreConsumeLog(
        usageLog({
          model_name: 'dreamina-seedance-2-0-fast-260128',
          other: JSON.stringify({
            is_task: true,
            request_path: '/api/v3/contents/generations/tasks',
          }),
        })
      ),
      true
    )
  })

  test('does not mark non-Seedance task consume logs as pre-consumed charges', () => {
    assert.equal(
      isTaskPreConsumeLog(
        usageLog({
          model_name: 'other-video-model',
          other: JSON.stringify({
            is_task: true,
            request_path: '/api/v3/contents/generations/tasks',
          }),
        })
      ),
      false
    )

    assert.equal(
      isTaskPreConsumeLog(
        usageLog({
          model_name: 'doubao-seedance-1-5-pro-251215',
          other: JSON.stringify({
            is_task: true,
            request_path: '/api/v3/contents/generations/tasks',
          }),
        })
      ),
      false
    )
  })

  test('does not mark task adjustment or refund logs as pre-consumed charges', () => {
    assert.equal(
      isTaskPreConsumeLog(
        usageLog({
          type: 6,
          other: JSON.stringify({
            is_task: true,
            pre_consumed_quota: 100,
            actual_quota: 40,
          }),
        })
      ),
      false
    )

    assert.equal(
      isTaskPreConsumeLog(
        usageLog({
          other: JSON.stringify({
            task_id: 'task_123',
            pre_consumed_quota: 100,
            actual_quota: 130,
          }),
        })
      ),
      false
    )
  })

  test('does not mark Suno submit logs as pre-consumed charges', () => {
    assert.equal(
      isTaskPreConsumeLog(
        usageLog({
          model_name: 'suno_music',
          other: JSON.stringify({
            is_task: true,
            request_path: '/suno/submit/MUSIC',
          }),
        })
      ),
      false
    )

    assert.equal(
      isTaskPreConsumeLog(
        usageLog({
          model_name: 'suno_lyrics',
          other: JSON.stringify({
            is_task: true,
            request_path: '/suno/submit/LYRICS',
          }),
        })
      ),
      false
    )
  })
})

describe('formatSignedLogQuota', () => {
  test('formats refund log quota as a negative amount', () => {
    assert.equal(
      formatSignedLogQuota(usageLog({ type: 6, quota: 100 })),
      '-$0.0002'
    )
  })

  test('keeps consume log quota positive', () => {
    assert.equal(
      formatSignedLogQuota(usageLog({ type: 2, quota: 100 })),
      '$0.0002'
    )
  })
})

describe('formatSensitiveQuota', () => {
  test('masks quota when sensitive values are hidden', () => {
    assert.equal(formatSensitiveQuota(123, false), '••••')
  })

  test('formats quota when sensitive values are visible', () => {
    assert.equal(formatSensitiveQuota(123, true), '$0.000246')
  })
})

describe('buildCacheBillingRows', () => {
  test('splits Claude 5m and 1h cache write prices and costs', () => {
    const rows = buildCacheBillingRows({
      claude: true,
      model_ratio: 1,
      group_ratio: 0.5,
      cache_tokens: 1000,
      cache_ratio: 0.1,
      cache_creation_tokens_5m: 2000,
      cache_creation_ratio_5m: 1.25,
      cache_creation_tokens_1h: 3000,
      cache_creation_ratio_1h: 2,
    })

    assert.deepEqual(rows, [
      {
        labelKey: 'Cache Read Price',
        value: '$0.2/M',
      },
      {
        labelKey: 'Cache Read Cost',
        value: '$0.0001',
      },
      {
        labelKey: 'Cache Write (5m) Price',
        value: '$2.5/M',
      },
      {
        labelKey: 'Cache Write (5m) Cost',
        value: '$0.0025',
      },
      {
        labelKey: 'Cache Write (1h) Price',
        value: '$4/M',
      },
      {
        labelKey: 'Cache Write (1h) Cost',
        value: '$0.006',
      },
    ])
  })
})
