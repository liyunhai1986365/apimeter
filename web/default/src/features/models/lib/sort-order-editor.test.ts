import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { normalizeSortOrderInput } from './sort-order-editor'

describe('model sort order editor', () => {
  test('normalizes decimal input to a non-negative integer', () => {
    assert.equal(normalizeSortOrderInput('12.8'), 12)
  })

  test('rejects empty and negative input', () => {
    assert.equal(normalizeSortOrderInput(''), null)
    assert.equal(normalizeSortOrderInput('-1'), null)
  })
})
