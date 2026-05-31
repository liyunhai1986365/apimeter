import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildSitePathURL,
  getCliDisplayName,
  getSitePlanName,
  getSiteServerAddress,
} from './site-branding'

describe('site branding helpers', () => {
  test('builds Modelsell-facing names from the configured system name', () => {
    assert.equal(getCliDisplayName('Acme AI'), 'Acme AI CLI')
    assert.equal(getSitePlanName('Acme AI'), 'Acme AI Tokens Plan')
  })

  test('builds public URLs from the configured server address', () => {
    assert.equal(
      buildSitePathURL('https://api.example.com/', '/docs/apps'),
      'https://api.example.com/docs/apps'
    )
  })

  test('falls back to current origin only when server address is empty', () => {
    const originalWindow = globalThis.window
    globalThis.window = {
      location: { origin: 'https://current.example.com' },
    } as unknown as Window & typeof globalThis

    try {
      assert.equal(getSiteServerAddress(''), 'https://current.example.com')
    } finally {
      globalThis.window = originalWindow
    }
  })
})
