import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { TFunction } from 'i18next'
import {
  getHTMLLanguage,
  normalizeSEOPath,
  resolveSEODescriptor,
} from './seo'

const t = ((key: string) => key) as TFunction

describe('SEO helpers', () => {
  test('normalizes canonical paths without trailing slashes', () => {
    assert.equal(normalizeSEOPath('/pricing/'), '/pricing')
    assert.equal(
      normalizeSEOPath('/pricing/vendor%2Fmodel/'),
      '/pricing/vendor%2Fmodel'
    )
    assert.equal(normalizeSEOPath('/'), '/')
  })

  test('indexes public pages and strips filter state from canonical URLs', () => {
    const descriptor = resolveSEODescriptor('/pricing/', 'ModelSell', t)

    assert.equal(descriptor.title, 'Model Price | ModelSell')
    assert.equal(descriptor.canonicalPath, '/pricing')
    assert.equal(descriptor.robots, 'index, follow')
  })

  test('uses model names for pricing detail metadata', () => {
    const descriptor = resolveSEODescriptor(
      '/pricing/gpt-4.1%20mini/',
      'ModelSell',
      t
    )

    assert.equal(descriptor.title, 'gpt-4.1 mini Model Price | ModelSell')
    assert.equal(descriptor.canonicalPath, '/pricing/gpt-4.1%20mini')
    assert.equal(descriptor.robots, 'index, follow')
  })

  test('keeps authentication and dashboard pages out of search results', () => {
    assert.equal(
      resolveSEODescriptor('/sign-in', 'ModelSell', t).robots,
      'noindex, nofollow'
    )
    assert.equal(
      resolveSEODescriptor('/dashboard/overview', 'ModelSell', t).robots,
      'noindex, nofollow'
    )
  })

  test('marks unknown pages noindex and maps interface languages to HTML tags', () => {
    assert.equal(
      resolveSEODescriptor('/unknown', 'ModelSell', t).canonicalPath,
      undefined
    )
    assert.equal(getHTMLLanguage('zhCN'), 'zh-CN')
    assert.equal(getHTMLLanguage('zhTW'), 'zh-TW')
  })
})
