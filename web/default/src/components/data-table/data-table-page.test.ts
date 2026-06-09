import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { getDataTableCellClassName } from './table-cell-class'

describe('data table page', () => {
  test('keeps action column fixed on the right side', () => {
    const className = getDataTableCellClassName('actions')

    assert.ok(className)
    assert.match(className, /\bsticky\b/)
    assert.match(className, /\bright-0\b/)
  })

  test('does not fix regular columns', () => {
    assert.equal(getDataTableCellClassName('name'), undefined)
  })
})
