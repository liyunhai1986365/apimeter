import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { PricingVendor } from '../types'
import { sortVendorsByConfiguredOrder } from './vendor-order'

describe('pricing vendor order', () => {
  test('uses configured vendor sort order with id as the tie-breaker', () => {
    const vendors: PricingVendor[] = [
      { id: 30, name: 'Alpha', sort_order: 20 },
      { id: 20, name: 'Zulu', sort_order: 10 },
      { id: 10, name: 'Beta', sort_order: 10 },
    ]

    const result = sortVendorsByConfiguredOrder(vendors)

    assert.deepEqual(
      result.map((vendor) => vendor.name),
      ['Beta', 'Zulu', 'Alpha']
    )
  })
})
