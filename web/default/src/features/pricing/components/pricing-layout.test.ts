import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  getPublicTopChromeHeight,
  PRICING_DETAILS_TOP_PADDING_CLASS,
  PRICING_PAGE_TOP_PADDING_CLASS,
  PRICING_SIDEBAR_STICKY_CLASS,
  PRICING_STICKY_TOP_CLASS,
} from './pricing-layout'

describe('pricing layout offsets', () => {
  test('sticky classes account for system notice banner height', () => {
    assert.match(
      PRICING_PAGE_TOP_PADDING_CLASS,
      /--system-notice-banner-height/
    )
    assert.match(
      PRICING_DETAILS_TOP_PADDING_CLASS,
      /--system-notice-banner-height/
    )
    assert.match(PRICING_SIDEBAR_STICKY_CLASS, /--system-notice-banner-height/)
    assert.match(PRICING_STICKY_TOP_CLASS, /--system-notice-banner-height/)
  })

  test('top chrome height includes public header, invite banner, and system notice', () => {
    const styles = {
      getPropertyValue(name: string) {
        const values: Record<string, string> = {
          '--app-header-height': '64px',
          '--invite-promo-banner-height': '40px',
          '--system-notice-banner-height': '40px',
        }
        return values[name] ?? ''
      },
    } as CSSStyleDeclaration

    assert.equal(getPublicTopChromeHeight(styles), 144)
  })
})
