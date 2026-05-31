import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  applyCustomerServiceScript,
  dismissCustomerServiceScriptForCurrentPage,
  extractScriptSrc,
  resetCustomerServiceScriptDismissal,
} from './customer-service-script'

class FakeElement {
  id = ''
  src = ''
  async = false
  tagName: string
  removed = false
  attributes = new Map<string, string>()
  children: FakeElement[] = []
  onclick: (() => void) | null = null

  constructor(tagName: string) {
    this.tagName = tagName.toUpperCase()
  }

  getAttribute(name: string) {
    if (name === 'src') return this.src || null
    return this.attributes.get(name) ?? null
  }

  setAttribute(name: string, value: string) {
    this.attributes.set(name, value)
  }

  appendChild(child: FakeElement) {
    this.children.push(child)
    child.removed = false
    return child
  }

  remove() {
    this.removed = true
    fakeElements.delete(this.id)
  }
}

const fakeElements = new Map<string, FakeElement>()

function installFakeDom() {
  fakeElements.clear()
  const head = new FakeElement('head')
  const createElement = (tagName: string) => new FakeElement(tagName)

  globalThis.document = {
    head,
    createElement,
    getElementById: (id: string) => {
      const existing = fakeElements.get(id)
      return existing && !existing.removed ? existing : null
    },
    querySelectorAll: () => [],
  } as unknown as Document
  globalThis.HTMLScriptElement = FakeElement as unknown as typeof HTMLScriptElement

  const originalAppendChild = head.appendChild.bind(head)
  head.appendChild = (child: FakeElement) => {
    originalAppendChild(child)
    if (child.id) fakeElements.set(child.id, child)
    return child
  }
}

function getFakeElement(id: string) {
  return fakeElements.get(id) ?? null
}

describe('extractScriptSrc', () => {
  test('extracts src from pasted script tags', () => {
    assert.equal(
      extractScriptSrc(
        '<script src="//code.tidio.co/lgdxmqumd5zeatipsguynjy3uijop1t3.js" async></script>'
      ),
      '//code.tidio.co/lgdxmqumd5zeatipsguynjy3uijop1t3.js'
    )
  })

  test('accepts bare script URLs', () => {
    assert.equal(
      extractScriptSrc('https://code.tidio.co/example.js'),
      'https://code.tidio.co/example.js'
    )
  })

  test('ignores inline script content', () => {
    assert.equal(extractScriptSrc('<script>alert(1)</script>'), '')
  })
})

describe('applyCustomerServiceScript', () => {
  test('keeps Tidio hidden after close until the current page is reloaded', () => {
    installFakeDom()
    resetCustomerServiceScriptDismissal()

    const scriptCode = '<script src="//code.tidio.co/example.js" async></script>'

    applyCustomerServiceScript(scriptCode)
    assert.ok(getFakeElement('customer-service-script'))
    assert.ok(getFakeElement('customer-service-script-dismiss'))

    dismissCustomerServiceScriptForCurrentPage()
    assert.equal(getFakeElement('customer-service-script'), null)
    assert.equal(getFakeElement('customer-service-script-dismiss'), null)

    applyCustomerServiceScript(scriptCode)
    assert.equal(getFakeElement('customer-service-script'), null)

    resetCustomerServiceScriptDismissal()
    applyCustomerServiceScript(scriptCode)
    assert.ok(getFakeElement('customer-service-script'))
  })
})
