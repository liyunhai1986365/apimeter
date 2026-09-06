import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { TaskLog } from '../types'
import {
  buildTaskLogSubtitle,
  formatDrawingSubmitTime,
  getTaskLogImagePreviewUrl,
  getTaskLogVideoPreviewUrl,
  getTaskImageStatus,
} from './task-display'

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

describe('task image expiry', () => {
  test('expires at the deadline even before physical cleanup', () => {
    const metadata = {
      image_status: 'available' as const,
      image_expires_at: 100,
    }
    assert.equal(getTaskImageStatus(metadata, 99), 'available')
    assert.equal(getTaskImageStatus(metadata, 100), 'expired')
    assert.equal(
      getTaskImageStatus({ ...metadata, image_has_url: true }, 100),
      'partially_expired'
    )
  })
  test('does not display a cached base64 image after expiry or as video', () => {
    const log = taskLog({
      image_status: 'expired',
      data: { data: { images: [{ b64_json: 'SECRET' }] } },
    })
    assert.equal(getTaskLogImagePreviewUrl(log), '')
    assert.equal(getTaskLogVideoPreviewUrl(log), '')
  })
  test('keeps the available URL when the first image expired', () => {
    const log = taskLog({
      image_status: 'partially_expired',
      data: {
        data: {
          images: [
            { image_expired: true },
            { url: 'https://example.com/keep.png' },
          ],
        },
      },
    })
    assert.equal(getTaskLogImagePreviewUrl(log), 'https://example.com/keep.png')
  })
})

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

    assert.equal(
      buildTaskLogSubtitle(log, (value) => value),
      'gpt-image-2'
    )
  })

  test('uses the recorded model for video tasks too', () => {
    const log = taskLog({
      platform: '999',
      action: 'generate',
      properties: {
        origin_model_name: 'doubao-seedance-2-0-mini-260615',
        upstream_model_name: 'doubao-seedance-2-0-mini-260615-max',
      },
    })

    assert.equal(
      buildTaskLogSubtitle(log, (value) => value),
      'doubao-seedance-2-0-mini-260615'
    )
  })

  test('uses nano-banana model names instead of numeric platform and video action', () => {
    const log = taskLog({
      platform: '24',
      properties: { origin_model_name: 'nano-banana-2' },
    })
    assert.equal(
      buildTaskLogSubtitle(log, (value) => value),
      'nano-banana-2'
    )
    assert.equal(
      getTaskLogVideoPreviewUrl({
        ...log,
        result_url: 'https://example.com/image.png',
      }),
      ''
    )
    assert.equal(
      getTaskLogImagePreviewUrl({
        ...log,
        result_url: 'https://example.com/image.png',
      }),
      'https://example.com/image.png'
    )
  })

  test('uses upstream metadata when the original model is missing', () => {
    const log = taskLog({
      properties: JSON.stringify({
        origin_model_name: ' ',
        upstream_model_name: 'custom-model',
      }),
    })
    assert.equal(
      buildTaskLogSubtitle(log, (value) => value),
      'custom-model'
    )
  })

  test('does not invent a model from a numeric platform or generic action', () => {
    const log = taskLog({ platform: '24', properties: undefined })
    assert.equal(
      buildTaskLogSubtitle(log, (value) => value),
      'Unknown model'
    )
  })
})

describe('formatDrawingSubmitTime', () => {
  test('formats Midjourney submit_time as milliseconds', () => {
    assert.equal(formatDrawingSubmitTime(1779955200000), '2026-05-28 16:00:00')
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

    assert.equal(
      getTaskLogVideoPreviewUrl(log),
      '/v1/videos/task_legacy/content'
    )
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

describe('saved task detail payloads', () => {
  test('previews normalized image objects containing only base64', () => {
    const log = taskLog({
      properties: { origin_model_name: 'gpt-image-2-count' },
      data: { data: { images: [{ url: '', b64_json: 'aW1hZ2U=' }] } },
    })
    assert.equal(
      getTaskLogImagePreviewUrl(log),
      'data:image/png;base64,aW1hZ2U='
    )
    assert.equal(getTaskLogVideoPreviewUrl(log), '')
  })

  test('supports normalized images for custom model names', () => {
    const log = taskLog({
      properties: { origin_model_name: 'custom-image-model' },
      data: { data: { images: [{ url: 'https://example.com/image.png' }] } },
    })
    assert.equal(
      getTaskLogImagePreviewUrl(log),
      'https://example.com/image.png'
    )
  })

  test('reads video URLs from object results returned by the detail endpoint', () => {
    const log = taskLog({
      data: { output: { video_url: 'https://example.com/video.mp4' } },
    })
    assert.equal(
      getTaskLogVideoPreviewUrl(log),
      'https://example.com/video.mp4'
    )
  })

  test('summary rows do not contain eager media URLs', () => {
    const log = taskLog({
      data: null,
      data_omitted: true,
      properties: { origin_model_name: 'gpt-image-2-count' },
    })
    assert.equal(getTaskLogImagePreviewUrl(log), '')
    assert.equal(getTaskLogVideoPreviewUrl(log), '')
  })
})
