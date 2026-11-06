import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  getEmbeddedHeaderNavLink,
  getOrderedHeaderNavItems,
  normalizeHeaderNavEmbedUrl,
  parseHeaderNavModules,
  parseHeaderNavModulesFromStatus,
  serializeHeaderNavModules,
} from './nav-modules'

const href = 'http://www.apimeter.cn/home/index/article_ff.html'

function modulesFor(overrides: Record<string, unknown> = {}) {
  return parseHeaderNavModules({
    customLinks: [
      {
        id: 'news',
        title: '付费资讯',
        href,
        enabled: true,
        embed: true,
        ...overrides,
      },
    ],
  })
}

describe('embedded third-party navigation', () => {
  test('keeps legacy links opening externally unless embedding is enabled', () => {
    const modules = modulesFor({ embed: undefined })
    const menu = getOrderedHeaderNavItems(modules).find(
      (item) => item.id === 'custom:news'
    )
    assert.equal(menu?.href, href)
    assert.equal(menu?.external, true)
    assert.equal(menu?.newWindow, true)
    assert.equal(getEmbeddedHeaderNavLink(modules, 'news'), undefined)
  })

  test('retains the full external URL and access settings through backend serialization', () => {
    const modules = modulesFor({ requireAuth: true })
    const serialized = serializeHeaderNavModules(modules)
    const restored = parseHeaderNavModulesFromStatus({
      HeaderNavModules: serialized,
    })
    const link = getEmbeddedHeaderNavLink(restored, 'news')
    assert.equal(link?.title, '付费资讯')
    assert.equal(link?.href, href)
    assert.equal(link?.requireAuth, true)
    assert.equal(link?.embed, true)
    const menu = getOrderedHeaderNavItems(restored).find(
      (item) => item.id === 'custom:news'
    )
    assert.equal(menu?.href, '/external/news')
    assert.equal(menu?.external, false)
    assert.equal(menu?.newWindow, false)
    assert.equal(menu?.requireAuth, true)
  })

  test('encodes configured IDs for internal navigation', () => {
    const id = 'news/中文 ?#'
    const menu = getOrderedHeaderNavItems(modulesFor({ id })).find(
      (item) => item.id === `custom:${id}`
    )
    assert.equal(menu?.href, `/external/${encodeURIComponent(id)}`)
  })

  test('rejects direct access after a link is removed, disabled, or changed to redirect mode', () => {
    assert.equal(getEmbeddedHeaderNavLink(modulesFor(), 'missing'), undefined)
    for (const overrides of [{ enabled: false }, { embed: false }]) {
      assert.equal(
        getEmbeddedHeaderNavLink(modulesFor(overrides), 'news'),
        undefined
      )
    }
    assert.equal(
      getOrderedHeaderNavItems(modulesFor({ enabled: false })).some(
        (item) => item.id === 'custom:news'
      ),
      false
    )
  })

  test('preserves HTTP and HTTPS while rejecting unsafe or incomplete URLs', () => {
    assert.equal(normalizeHeaderNavEmbedUrl(` ${href} `), href)
    assert.equal(
      normalizeHeaderNavEmbedUrl('https://example.com/path?q=1#section'),
      'https://example.com/path?q=1#section'
    )
    for (const invalid of [
      null,
      1,
      '',
      '/relative',
      'javascript:alert(1)',
      'data:text/html,test',
      'ftp://example.com',
      'https://user:password@example.com',
    ]) {
      assert.equal(normalizeHeaderNavEmbedUrl(invalid), '')
      const modules = modulesFor({ href: invalid })
      assert.equal(getEmbeddedHeaderNavLink(modules, 'news'), undefined)
      assert.equal(
        getOrderedHeaderNavItems(modules).some(
          (item) => item.id === 'custom:news'
        ),
        false
      )
    }
  })

  test('prevents recursive embeds into another embedded route on this host', () => {
    const origin = 'https://gateway.example'
    for (const target of [
      `${origin}/external/news`,
      `${origin}/external/other?query=1`,
      'http://gateway.example/external/news',
    ]) {
      assert.equal(
        getEmbeddedHeaderNavLink(modulesFor({ href: target }), 'news', origin),
        undefined
      )
    }
    assert.equal(
      getEmbeddedHeaderNavLink(modulesFor(), 'news', origin)?.href,
      href
    )
  })
})
