import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildSitePathURL,
  getAgentToolsURL,
  getCliDisplayName,
  getCliDocsURL,
  getCliInstallCommands,
  getCliScreenshotURL,
  getSitePlanName,
  getSiteServerAddress,
  shouldShowAPIMeterCliSection,
} from './site-branding'

describe('site branding helpers', () => {
  test('builds APIMeter-facing names from the configured system name', () => {
    assert.equal(getCliDisplayName('Acme AI'), 'Acme AI CLI')
    assert.equal(getSitePlanName('Acme AI'), 'Acme AI Tokens Plan')
  })

  test('builds public URLs from the configured server address', () => {
    assert.equal(
      buildSitePathURL('https://api.example.com/', '/docs/apps'),
      'https://api.example.com/docs/apps'
    )
  })

  test('uses the APIMeter static and documentation origins for CLI assets', () => {
    const staticOrigin =
      process.env.VITE_APIMETER_STATIC_URL || 'https://static.apimeter.ai'
    const docsOrigin =
      process.env.VITE_APIMETER_DOCS_URL || 'https://docs.apimeter.ai'
    assert.deepEqual(getCliInstallCommands('https://api.example.com'), {
      unix: `curl -fsSL ${staticOrigin.replace(/\/+$/, '')}/apimeter-cli/install.sh | sh`,
      windows: `irm ${staticOrigin.replace(/\/+$/, '')}/apimeter-cli/install.ps1 | iex`,
    })
    assert.equal(
      getCliDocsURL('https://api.example.com'),
      `${docsOrigin.replace(/\/+$/, '')}/zh/docs/apps/apimeter-cli`
    )
    assert.equal(
      getAgentToolsURL('https://api.example.com'),
      `${docsOrigin.replace(/\/+$/, '')}/zh/docs/apps`
    )
    assert.equal(
      getCliScreenshotURL('https://api.example.com'),
      `${docsOrigin.replace(/\/+$/, '')}/assets/docs/apps/apimeter/apimeter-cli.png`
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

  test('shows the APIMeter CLI section only on the APIMeter domain', () => {
    assert.equal(shouldShowAPIMeterCliSection('apimeter.ai'), true)
    assert.equal(shouldShowAPIMeterCliSection('www.apimeter.ai'), true)
    assert.equal(shouldShowAPIMeterCliSection('agent.example.com'), false)
    assert.equal(shouldShowAPIMeterCliSection('apimeter.ai.evil.com'), false)
  })
})
