import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { USER_FACING_GROUP_TERMS } from './user-facing-group-terms'

describe('user-facing group terms', () => {
  test('uses supplier wording for non-admin group controls', () => {
    assert.equal(USER_FACING_GROUP_TERMS.single, 'Supplier')
    assert.equal(USER_FACING_GROUP_TERMS.plural, 'Suppliers')
    assert.equal(USER_FACING_GROUP_TERMS.selectOne, 'Select a supplier')
    assert.equal(USER_FACING_GROUP_TERMS.noneFound, 'No supplier found.')
    assert.equal(USER_FACING_GROUP_TERMS.callOrder, 'Supplier call order')
  })
})
