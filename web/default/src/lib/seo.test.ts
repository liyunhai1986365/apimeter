import type { TFunction } from 'i18next'
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { getHTMLLanguage, normalizeSEOPath, resolveSEODescriptor } from './seo'

const t = ((key: string, options?: Record<string, string>) =>
  Object.entries(options ?? {}).reduce(
    (value, [name, replacement]) =>
      value.replaceAll(`{{${name}}}`, replacement),
    key
  )) as TFunction

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

    assert.equal(
      descriptor.title,
      'AI Model API Pricing & Comparison | ModelSell'
    )
    assert.equal(
      descriptor.description,
      'Compare AI model API pricing, supported endpoints, capabilities and access options on ModelSell.'
    )
    assert.equal(descriptor.canonicalPath, '/pricing')
    assert.equal(descriptor.robots, 'index, follow')
    assert.deepEqual(descriptor.structuredData?.isPartOf, {
      '@id': '/#website',
    })
  })

  test('uses model names for pricing detail metadata', () => {
    const descriptor = resolveSEODescriptor(
      '/pricing/gpt-4.1%20mini/',
      'ModelSell',
      t
    )

    assert.equal(
      descriptor.title,
      'gpt-4.1 mini API Pricing & Access | ModelSell'
    )
    assert.equal(
      descriptor.description,
      'Compare gpt-4.1 mini API pricing, supported endpoints, capabilities and access options on ModelSell.'
    )
    assert.equal(descriptor.canonicalPath, '/pricing/gpt-4.1%20mini')
    assert.equal(descriptor.robots, 'index, follow')
    const graph = descriptor.structuredData?.['@graph'] as Array<
      Record<string, unknown>
    >
    assert.equal(graph[0]?.['@id'], '/pricing/gpt-4.1%20mini#webpage')
    assert.equal(graph[1]?.['@type'], 'Service')
    assert.deepEqual(graph[1]?.provider, { '@id': '/#organization' })
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
    const inviteRewards = resolveSEODescriptor(
      '/invite-rewards',
      'ModelSell',
      t
    )
    assert.equal(inviteRewards.title, 'ModelSell')
    assert.equal(inviteRewards.robots, 'noindex, nofollow')
  })

  test('uses the AI creation tools title for the private creation page', () => {
    const descriptor = resolveSEODescriptor('/ai-creation', 'ModelSell', t)

    assert.equal(descriptor.title, 'AI Creation Tools | ModelSell')
    assert.equal(descriptor.robots, 'noindex, nofollow')
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
