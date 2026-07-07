import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { stopPricingTableRowClick } from './pricing-table-events'

describe('pricing table events', () => {
  test('stops copy action clicks from opening the model details row', () => {
    let stopped = false

    stopPricingTableRowClick({
      stopPropagation() {
        stopped = true
      },
    })

    assert.equal(stopped, true)
  })
})
