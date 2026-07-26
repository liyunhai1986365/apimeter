import assert from 'node:assert/strict'
import { beforeEach, describe, test } from 'node:test'
import {
  applyGoogleAnalytics,
  normalizeGoogleAnalyticsId,
} from './google-analytics'

class FakeScriptElement {
  id = ''
  src = ''
  async = false
  dataset: Record<string, string> = {}

  remove() {
    fakeElements.delete(this.id)
  }
}

const fakeElements = new Map<string, FakeScriptElement>()

function installFakeDom() {
  fakeElements.clear()
  const head = {
    appendChild(element: FakeScriptElement) {
      fakeElements.set(element.id, element)
      return element
    },
  }

  globalThis.window = { dataLayer: [] } as unknown as Window & typeof globalThis
  globalThis.document = {
    head,
    createElement: () => new FakeScriptElement(),
    getElementById: (id: string) => fakeElements.get(id) ?? null,
  } as unknown as Document
  globalThis.HTMLScriptElement =
    FakeScriptElement as unknown as typeof HTMLScriptElement
}

describe('normalizeGoogleAnalyticsId', () => {
  test('normalizes valid GA4 measurement IDs', () => {
    assert.equal(normalizeGoogleAnalyticsId(' g-6b94bx72ew '), 'G-6B94BX72EW')
  })

  test('rejects script code and legacy IDs', () => {
    assert.equal(normalizeGoogleAnalyticsId('<script>alert(1)</script>'), '')
    assert.equal(normalizeGoogleAnalyticsId('UA-12345-1'), '')
  })
})

describe('applyGoogleAnalytics', () => {
  beforeEach(installFakeDom)

  test('loads and configures gtag once for the current measurement ID', () => {
    applyGoogleAnalytics('G-6B94BX72EW')
    applyGoogleAnalytics('G-6B94BX72EW')

    const script = fakeElements.get('google-analytics-script')
    assert.equal(
      script?.src,
      'https://www.googletagmanager.com/gtag/js?id=G-6B94BX72EW'
    )
    assert.equal(script?.async, true)
    const dataLayer = (window as unknown as Window & { dataLayer: unknown[] })
      .dataLayer
    assert.equal(dataLayer.length, 2)
    assert.equal(
      Object.prototype.toString.call(dataLayer[0]),
      '[object Arguments]'
    )
    assert.equal(
      Object.prototype.toString.call(dataLayer[1]),
      '[object Arguments]'
    )
    assert.equal(Array.isArray(dataLayer[0]), false)
    assert.equal(Array.isArray(dataLayer[1]), false)
    assert.equal(Array.from(dataLayer[0] as IArguments)[0], 'js')
    assert.deepEqual(Array.from(dataLayer[1] as IArguments), [
      'config',
      'G-6B94BX72EW',
    ])
  })

  test('removes the loader when analytics is disabled', () => {
    applyGoogleAnalytics('G-6B94BX72EW')
    applyGoogleAnalytics('')

    assert.equal(fakeElements.get('google-analytics-script'), undefined)
    assert.equal(
      (window as unknown as Record<string, unknown>)['ga-disable-G-6B94BX72EW'],
      true
    )
  })
})
