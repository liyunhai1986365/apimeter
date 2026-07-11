import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { getPublicHeaderTopClass } from './public-header-layout'

describe('public header layout', () => {
  test('reserves invite banner space before floating', () => {
    assert.match(getPublicHeaderTopClass(false), /--invite-promo-banner-height/)
  })

  test('stops reserving invite banner space while floating', () => {
    const className = getPublicHeaderTopClass(true)

    assert.doesNotMatch(className, /--invite-promo-banner-height/)
    assert.match(className, /--system-notice-banner-height/)
  })
})
