import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { buildSidebarData } from './use-sidebar-data'

function flattenUrls(data: ReturnType<typeof buildSidebarData>): string[] {
  return data.navGroups.flatMap((group): string[] =>
    group.items.flatMap((item) => {
      if ('url' in item && item.url) return [item.url as string]
      if ('items' in item && item.items) {
        return item.items.map((subItem) => subItem.url as string)
      }
      return []
    })
  )
}

describe('buildSidebarData', () => {
  test('adds billing as an independent personal menu item', () => {
    const data = buildSidebarData((key) => key, null)
    const urls = flattenUrls(data)

    assert.ok(urls.includes('/billing/monthly'))
    assert.ok(!urls.includes('/dashboard/billing'))
  })

  test('adds model profit as an independent admin menu item', () => {
    const data = buildSidebarData((key) => key, null)
    const urls = flattenUrls(data)

    assert.ok(urls.includes('/model-profit'))
  })

  test('adds provider directory between subscription and profile', () => {
    const data = buildSidebarData((key) => key, null)
    const urls = flattenUrls(data)
    const personal = data.navGroups.find((group) => group.id === 'personal')
    const personalUrls =
      personal?.items
        .filter((item) => 'url' in item && item.url)
        .map((item) => item.url as string) ?? []

    assert.ok(urls.includes('/provider'))
    assert.ok(urls.includes('/suppliers'))
    assert.equal(
      personalUrls.indexOf('/provider'),
      personalUrls.indexOf('/user-subscription') + 1
    )
    assert.equal(
      personalUrls.indexOf('/profile'),
      personalUrls.indexOf('/provider') + 1
    )
  })
})
