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
  test('labels personal navigation as account for all user types', () => {
    const regularData = buildSidebarData((key) => key, null)
    const workspaceData = buildSidebarData((key) => key, {
      workspace_subaccount: true,
    })

    assert.equal(
      regularData.navGroups.find((group) => group.id === 'personal')?.title,
      'Account'
    )
    assert.equal(
      workspaceData.navGroups.find((group) => group.id === 'personal')?.title,
      'Account'
    )
  })

  test('adds billing as an independent personal menu item', () => {
    const data = buildSidebarData((key) => key, null)
    const urls = flattenUrls(data)

    assert.ok(urls.includes('/billing/monthly'))
    assert.ok(!urls.includes('/dashboard/billing'))
  })

  test('adds invite rewards as an independent personal menu item', () => {
    const data = buildSidebarData((key) => key, null)
    const personal = data.navGroups.find((group) => group.id === 'personal')
    const personalUrls =
      personal?.items
        .filter((item) => 'url' in item && item.url)
        .map((item) => item.url as string) ?? []

    assert.ok(personalUrls.includes('/invite-rewards'))
    assert.equal(
      personalUrls.indexOf('/invite-rewards'),
      personalUrls.indexOf('/wallet') + 1
    )
  })

  test('adds model profit as an independent admin menu item', () => {
    const data = buildSidebarData((key) => key, null)
    const urls = flattenUrls(data)

    assert.ok(urls.includes('/model-profit'))
  })

  test('keeps team settings out of the sidebar', () => {
    const data = buildSidebarData((key) => key, null)
    const urls = flattenUrls(data)
    const general = data.navGroups.find((group) => group.id === 'general')

    assert.equal(
      data.navGroups.some((group) => group.id === 'organization'),
      false
    )
    assert.ok(!urls.includes('/workspaces'))
    assert.ok(!urls.includes('/workspace-accounts'))
    assert.ok(
      general?.items.some(
        (item) =>
          item.title === 'API Keys' && 'url' in item && item.url === '/keys'
      )
    )
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

  test('limits workspace accounts to their operational pages', () => {
    const data = buildSidebarData((key) => key, {
      workspace_subaccount: true,
    })
    const general = data.navGroups.find((group) => group.id === 'general')

    assert.deepEqual(flattenUrls(data), [
      '/dashboard/overview',
      '/keys',
      '/usage-logs/common',
      '/profile',
    ])
    assert.ok(
      general?.items.some(
        (item) =>
          item.title === 'API Keys' && 'url' in item && item.url === '/keys'
      )
    )
  })
})
