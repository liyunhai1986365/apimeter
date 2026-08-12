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
    const descriptor = resolveSEODescriptor('/pricing/', 'APIMeter', t)

    assert.equal(
      descriptor.title,
      'AI Model API Pricing & Comparison | APIMeter'
    )
    assert.equal(
      descriptor.description,
      'Compare AI model API pricing, supported endpoints, capabilities and access options on APIMeter.'
    )
    assert.equal(descriptor.canonicalPath, '/pricing')
    assert.equal(descriptor.robots, 'index, follow')
    assert.deepEqual(descriptor.structuredData?.isPartOf, {
      '@id': '/#website',
    })
  })

  test('uses partner program metadata for the partner page', () => {
    const descriptor = resolveSEODescriptor('/partner/', 'APIMeter', t)

    assert.equal(descriptor.title, 'Partner Program | APIMeter')
    assert.equal(descriptor.canonicalPath, '/partner')
    assert.equal(descriptor.robots, 'index, follow')
  })

  test('uses model names for pricing detail metadata', () => {
    const descriptor = resolveSEODescriptor(
      '/pricing/gpt-4.1%20mini/',
      'APIMeter',
      t
    )

    assert.equal(
      descriptor.title,
      'gpt-4.1 mini API Pricing & Access | APIMeter'
    )
    assert.equal(
      descriptor.description,
      'Compare gpt-4.1 mini API pricing, supported endpoints, capabilities and access options on APIMeter.'
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

  test('builds canonical collection metadata for model categories', () => {
    const descriptor = resolveSEODescriptor(
      '/pricing/categories/image/',
      'APIMeter',
      t
    )

    assert.equal(descriptor.title, 'Image Generation model APIs | APIMeter')
    assert.equal(descriptor.canonicalPath, '/pricing/categories/image')
    assert.equal(descriptor.robots, 'index, follow')
    const graph = descriptor.structuredData?.['@graph'] as Array<
      Record<string, unknown>
    >
    assert.equal(graph[0]?.['@type'], 'CollectionPage')
    assert.equal(graph[0]?.['@id'], '/pricing/categories/image#webpage')
    assert.equal(graph[1]?.['@type'], 'BreadcrumbList')
  })

  test('indexes provider directory and provider detail pages', () => {
    const directory = resolveSEODescriptor('/providers/', 'APIMeter', t)
    assert.equal(directory.title, 'AI Model Providers | APIMeter')
    assert.equal(directory.canonicalPath, '/providers')
    assert.equal(directory.robots, 'index, follow')

    const provider = resolveSEODescriptor('/providers/openai/', 'APIMeter', t)
    assert.equal(provider.title, 'OpenAI AI Models & APIs | APIMeter')
    assert.equal(provider.canonicalPath, '/providers/openai')
    assert.equal(provider.robots, 'index, follow')
    const graph = provider.structuredData?.['@graph'] as Array<
      Record<string, unknown>
    >
    assert.equal(graph[0]?.['@type'], 'CollectionPage')
    assert.equal(graph[1]?.['@type'], 'BreadcrumbList')
  })

  test('keeps authentication and dashboard pages out of search results', () => {
    assert.equal(
      resolveSEODescriptor('/sign-in', 'APIMeter', t).robots,
      'noindex, nofollow'
    )
    assert.equal(
      resolveSEODescriptor('/dashboard/overview', 'APIMeter', t).robots,
      'noindex, nofollow'
    )
    const inviteRewards = resolveSEODescriptor(
      '/invite-rewards',
      'APIMeter',
      t
    )
    assert.equal(inviteRewards.title, 'APIMeter')
    assert.equal(inviteRewards.robots, 'noindex, nofollow')
  })

  test('uses the AI creation tools title for the private creation page', () => {
    const descriptor = resolveSEODescriptor('/ai-creation', 'APIMeter', t)

    assert.equal(descriptor.title, 'AI Creation Tools | APIMeter')
    assert.equal(descriptor.robots, 'noindex, nofollow')
  })

  test('marks unknown pages noindex and maps interface languages to HTML tags', () => {
    assert.equal(
      resolveSEODescriptor('/unknown', 'APIMeter', t).canonicalPath,
      undefined
    )
    assert.equal(getHTMLLanguage('zhCN'), 'zh-CN')
    assert.equal(getHTMLLanguage('zhTW'), 'zh-TW')
  })
})
