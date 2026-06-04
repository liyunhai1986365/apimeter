import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildTaskLogSubtitle,
  getTaskLogImagePreviewUrl,
  getTaskLogVideoPreviewUrl,
} from './task-display'
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

describe('getTaskLogVideoPreviewUrl', () => {
  test('uses result_url from successful video tasks', () => {
    const log = taskLog({
      result_url: 'https://cdn.example.com/result.mp4',
    })

    assert.equal(
      getTaskLogVideoPreviewUrl(log),
      'https://cdn.example.com/result.mp4'
    )
  })

  test('extracts video url from task data when result_url is absent', () => {
    const log = taskLog({
      data: JSON.stringify({
        output: {
          videos: [{ url: 'https://cdn.example.com/output.mp4' }],
        },
      }),
    })

    assert.equal(
      getTaskLogVideoPreviewUrl(log),
      'https://cdn.example.com/output.mp4'
    )
  })

  test('falls back to proxied video content for successful legacy video tasks', () => {
    const log = taskLog({
      task_id: 'task_legacy',
      fail_reason: 'https://cdn.example.com/legacy.mp4',
    })

    assert.equal(getTaskLogVideoPreviewUrl(log), '/v1/videos/task_legacy/content')
  })

  test('does not expose preview url for failed video tasks', () => {
    const log = taskLog({
      status: 'FAILURE',
      result_url: 'https://cdn.example.com/result.mp4',
    })

    assert.equal(getTaskLogVideoPreviewUrl(log), '')
  })

  test('does not expose image task result url as video preview', () => {
    const log = taskLog({
      result_url: 'https://cdn.example.com/result.png',
      properties: {
        origin_model_name: 'gpt-image-2',
      },
    })

    assert.equal(getTaskLogVideoPreviewUrl(log), '')
  })
})

describe('getTaskLogImagePreviewUrl', () => {
  test('uses result_url from successful image tasks', () => {
    const log = taskLog({
      result_url: 'https://cdn.example.com/result.png',
      properties: {
        origin_model_name: 'gpt-image-2',
      },
    })

    assert.equal(
      getTaskLogImagePreviewUrl(log),
      'https://cdn.example.com/result.png'
    )
  })

  test('extracts image url from normalized task data', () => {
    const log = taskLog({
      properties: {
        upstream_model_name: 'gemini-3-pro-image-preview',
      },
      data: JSON.stringify({
        data: {
          images: [{ url: 'https://cdn.example.com/output.jpeg' }],
        },
      }),
    })

    assert.equal(
      getTaskLogImagePreviewUrl(log),
      'https://cdn.example.com/output.jpeg'
    )
  })
})
