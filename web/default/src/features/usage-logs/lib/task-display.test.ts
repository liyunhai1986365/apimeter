import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { buildTaskLogSubtitle } from './task-display'
import type { TaskLog } from '../types'

function taskLog(overrides: Partial<TaskLog>): TaskLog {
  return {
    id: 1,
    user_id: 1,
    platform: '1',
    task_id: 'task_test',
    action: 'generate',
    channel_id: 1,
    submit_time: 1779955200,
    status: 'SUCCESS',
    ...overrides,
  }
}

describe('buildTaskLogSubtitle', () => {
  test('shows async image model name instead of generic image-to-video action', () => {
    const log = taskLog({
      properties: {
        origin_model_name: 'gemini-3-pro-image-preview',
        upstream_model_name: 'gemini-3.1-flash-image-preview',
      },
    })

    assert.equal(
      buildTaskLogSubtitle(log, (value) => value),
      'gemini-3-pro-image-preview'
    )
  })

  test('falls back to upstream image model name when origin model is blank', () => {
    const log = taskLog({
      properties: {
        origin_model_name: ' ',
        upstream_model_name: 'gpt-image-2',
      },
    })

    assert.equal(buildTaskLogSubtitle(log, (value) => value), 'gpt-image-2')
  })

  test('keeps the existing platform and action subtitle for video tasks', () => {
    const log = taskLog({ platform: 'kling', action: 'generate' })

    assert.equal(
      buildTaskLogSubtitle(log, (value) => value),
      'kling · Image to Video'
    )
  })
})
