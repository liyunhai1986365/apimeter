import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { DASHBOARD_SECTION_IDS } from './section-registry'

describe('dashboard section registry', () => {
  test('keeps billing out of dashboard sections', () => {
    assert.deepEqual(
      [...DASHBOARD_SECTION_IDS],
      ['overview', 'models', 'users']
    )
  })
})
