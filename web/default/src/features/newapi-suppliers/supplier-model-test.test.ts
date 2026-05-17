import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { applySupplierModelTestResults } from './supplier-model-test'

describe('applySupplierModelTestResults', () => {
  test('unchecks failed models and keeps successful models selected', () => {
    const selected = {
      'gpt-4o-mini': true,
      'gpt-4o': true,
      'claude-sonnet': true,
      'unchecked-model': false,
    }

    const next = applySupplierModelTestResults(selected, [
      { model: 'gpt-4o-mini', success: true },
      { model: 'gpt-4o', success: false },
      { model: 'claude-sonnet', success: true },
    ])

    assert.deepEqual(next, {
      'gpt-4o-mini': true,
      'gpt-4o': false,
      'claude-sonnet': true,
      'unchecked-model': false,
    })
  })

  test('selects a previously unchecked model when its test passes', () => {
    const selected = {
      'gpt-4o-mini': false,
      'gpt-4o': true,
    }

    const next = applySupplierModelTestResults(selected, [
      { model: 'gpt-4o-mini', success: true },
      { model: 'gpt-4o', success: false },
    ])

    assert.deepEqual(next, {
      'gpt-4o-mini': true,
      'gpt-4o': false,
    })
  })
})
