import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { sanitizeStatusForCache } from './status-cache'

describe('status cache', () => {
  test('does not persist personalized announcements', () => {
    const status = sanitizeStatusForCache({
      system_name: 'New API',
      announcements: [{ title: 'VIP only', target_groups: ['vip'] }],
    })

    assert.deepEqual(status, { system_name: 'New API' })
  })
})
