import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  applyCustomerServiceScript,
  dismissCustomerServiceScriptForCurrentPage,
  extractScriptSrc,
  openCustomerServiceChat,
  resetCustomerServiceScriptDismissal,
} from './customer-service-script'

class FakeElement {
  id = ''
  src = ''
  async = false
  fetchPriority = ''
  tagName: string
  removed = false
  attributes = new Map<string, string>()
  children: FakeElement[] = []
  onclick: (() => void) | null = null
  listeners = new Map<string, EventListener>()

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

  addEventListener(type: string, listener: EventListener) {
    this.listeners.set(type, listener)
  }

  dispatchEvent(event: Event) {
    this.listeners.get(event.type)?.(event)
    return true
  }

  remove() {
    this.removed = true
    fakeElements.delete(this.id)
  }
}

const fakeElements = new Map<string, FakeElement>()
let tidioReadyListener: EventListener | null = null

function installFakeDom() {
  fakeElements.clear()
  tidioReadyListener = null
  const head = new FakeElement('head')
  const createElement = (tagName: string) => new FakeElement(tagName)

  globalThis.document = {
    head,
    documentElement: { lang: 'en' },
    createElement,
    getElementById: (id: string) => {
      const existing = fakeElements.get(id)
      return existing && !existing.removed ? existing : null
    },
    addEventListener: (
      type: string,
      listener: EventListenerOrEventListenerObject
    ) => {
      if (type === 'tidioChat-ready' && typeof listener === 'function') {
        tidioReadyListener = listener
      }
    },
    removeEventListener: (
      type: string,
      listener: EventListenerOrEventListenerObject
    ) => {
      if (type === 'tidioChat-ready' && tidioReadyListener === listener) {
        tidioReadyListener = null
      }
    },
    querySelectorAll: () => [],
  } as unknown as Document
  globalThis.HTMLScriptElement =
    FakeElement as unknown as typeof HTMLScriptElement

  const originalAppendChild = head.appendChild.bind(head)
  head.appendChild = (child: FakeElement) => {
    originalAppendChild(child)
    if (child.id) fakeElements.set(child.id, child)
    return child
  }
}

function installFakeWindow(
  tidioChatApi?: {
    show: () => void
    open: () => void
  },
  saleWiselySdk?: {
    destroy?: () => void
    setLoginInfo?: (user: unknown) => void
    showChat?: () => void | Promise<unknown>
  },
  storage: Record<string, string> = {}
) {
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    writable: true,
    value: {
      ...(tidioChatApi ? { tidioChatApi } : {}),
      ...(saleWiselySdk ? { SaleWiselySDK: saleWiselySdk } : {}),
      localStorage: {
        getItem: (key: string) => storage[key] ?? null,
      },
      navigator: { language: 'en-US' },
    },
  })
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

  test('extracts the SaleWisely SDK URL', () => {
    assert.equal(
      extractScriptSrc(
        '<script async fetchpriority="low" src="https://file.salewisely.com/sdk/release/salewisely-bundled.js?appId=735d606344a8422b87002bd98cb5812d"></script>'
      ),
      'https://file.salewisely.com/sdk/release/salewisely-bundled.js?appId=735d606344a8422b87002bd98cb5812d'
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

    const scriptCode =
      '<script src="//code.tidio.co/example.js" async></script>'

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

  test('prepares SaleWisely user data and loads its SDK with low priority', () => {
    installFakeDom()
    installFakeWindow(undefined, undefined, {
      i18nextLng: 'zhCN',
      user: JSON.stringify({
        id: 42,
        username: 'modelsell-user',
        display_name: 'ModelSell User',
        email: 'user@example.com',
      }),
    })

    applyCustomerServiceScript(
      '<script async fetchpriority="low" src="https://file.salewisely.com/sdk/release/salewisely-bundled.js?appId=test-app"></script>'
    )

    const script = getFakeElement('customer-service-script')
    assert.ok(script)
    assert.equal(script.fetchPriority, 'low')
    assert.deepEqual(
      (window as Window & { _salewiselyUser?: unknown })._salewiselyUser,
      {
        userId: '42',
        userName: 'ModelSell User',
        email: 'user@example.com',
        language: 'zh',
      }
    )
    assert.deepEqual(
      (window as Window & { _salewiselyConfig?: unknown })._salewiselyConfig,
      { language: 'zh' }
    )
  })

  test('removes the previous provider before switching scripts', () => {
    installFakeDom()
    installFakeWindow()

    applyCustomerServiceScript('https://code.tidio.co/example.js')
    assert.ok(getFakeElement('customer-service-script-dismiss'))

    applyCustomerServiceScript(
      'https://file.salewisely.com/sdk/release/salewisely-bundled.js?appId=test-app'
    )

    assert.equal(getFakeElement('customer-service-script-dismiss'), null)
    assert.equal(
      getFakeElement('customer-service-script')?.src,
      'https://file.salewisely.com/sdk/release/salewisely-bundled.js?appId=test-app'
    )
  })
})

describe('openCustomerServiceChat', () => {
  test('shows and opens a ready Tidio widget', () => {
    installFakeDom()
    let shown = 0
    let opened = 0
    installFakeWindow({
      show: () => shown++,
      open: () => opened++,
    })

    assert.equal(
      openCustomerServiceChat('https://code.tidio.co/example.js'),
      true
    )
    assert.equal(shown, 1)
    assert.equal(opened, 1)
    assert.equal(tidioReadyListener, null)
  })

  test('waits for Tidio when the widget is still loading', () => {
    installFakeDom()
    installFakeWindow()

    assert.equal(
      openCustomerServiceChat('https://code.tidio.co/example.js'),
      true
    )
    assert.ok(tidioReadyListener)

    let shown = 0
    let opened = 0
    installFakeWindow({
      show: () => shown++,
      open: () => opened++,
    })
    tidioReadyListener?.(new Event('tidioChat-ready'))

    assert.equal(shown, 1)
    assert.equal(opened, 1)
  })

  test('opens a ready SaleWisely widget', () => {
    installFakeDom()
    let opened = 0
    installFakeWindow(undefined, {
      showChat: () => {
        opened++
      },
    })

    assert.equal(
      openCustomerServiceChat(
        'https://file.salewisely.com/sdk/release/salewisely-bundled.js?appId=test-app'
      ),
      true
    )
    assert.equal(opened, 1)
  })

  test('waits for SaleWisely when the widget is still loading', () => {
    installFakeDom()
    installFakeWindow()

    assert.equal(
      openCustomerServiceChat(
        'https://file.salewisely.com/sdk/release/salewisely-bundled.js?appId=test-app'
      ),
      true
    )

    const script = getFakeElement('customer-service-script')
    assert.ok(script)

    let opened = 0
    ;(
      window as Window & {
        SaleWiselySDK?: { showChat: () => void }
      }
    ).SaleWiselySDK = {
      showChat: () => {
        opened++
      },
    }
    script.dispatchEvent(new Event('load'))

    assert.equal(opened, 1)
  })

  test('rejects unsupported customer service scripts', () => {
    installFakeDom()
    installFakeWindow()

    assert.equal(
      openCustomerServiceChat('https://chat.example.com/widget.js'),
      false
    )
  })
})
