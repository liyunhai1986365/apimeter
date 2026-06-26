import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { UsageLog } from '../data/schema'
import { formatSignedLogQuota, isTaskPreConsumeLog } from './format'

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
  test('marks task consume logs as pre-consumed charges', () => {
    assert.equal(
      isTaskPreConsumeLog(
        usageLog({ other: JSON.stringify({ is_task: true }) })
      ),
      true
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
