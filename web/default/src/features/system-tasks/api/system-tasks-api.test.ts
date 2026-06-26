import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { buildSystemTasksListParams } from './system-tasks-api'

describe('system task list params', () => {
  test('uses page and page_size for paginated task list requests', () => {
    assert.deepEqual(buildSystemTasksListParams(3, 50), {
      p: 3,
      page_size: 50,
    })
  })
})
