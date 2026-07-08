import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  PRICING_SUPPLIER_FILTER_DEFAULT_OPEN,
  PRICING_VENDOR_FILTER_DEFAULT_OPEN,
  shouldFilterSectionDefaultOpen,
} from './pricing-sidebar-state'

describe('pricing sidebar filter sections', () => {
  test('keeps ordinary filter sections expanded by default', () => {
    assert.equal(shouldFilterSectionDefaultOpen(), true)
  })

  test('keeps vendor filters expanded and supplier filters collapsed by default', () => {
    assert.equal(PRICING_VENDOR_FILTER_DEFAULT_OPEN, true)
    assert.equal(PRICING_SUPPLIER_FILTER_DEFAULT_OPEN, false)
    assert.equal(
      shouldFilterSectionDefaultOpen(PRICING_VENDOR_FILTER_DEFAULT_OPEN),
      true
    )
    assert.equal(
      shouldFilterSectionDefaultOpen(PRICING_SUPPLIER_FILTER_DEFAULT_OPEN),
      false
    )
  })
})
