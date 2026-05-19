import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { ROLE } from '@/lib/roles'
import type { NavGroup } from '@/components/layout/types'
import { applyRoleSidebarRestrictions } from './use-sidebar-config'

const adminNavGroups: NavGroup[] = [
  {
    id: 'admin',
    title: 'Admin',
    items: [
      {
        title: 'Channels',
        url: '/channels',
      },
      {
        title: 'Supplier Management',
        url: '/suppliers',
      },
      {
        title: 'Models',
        url: '/models/metadata',
      },
      {
        title: 'System Settings',
        url: '/system-settings/site',
      },
    ],
  },
]

function flattenUrls(navGroups: NavGroup[]): string[] {
  return navGroups.flatMap((group): string[] =>
    group.items.flatMap((item) => {
      if ('url' in item && item.url) return [item.url as string]
      if ('items' in item && item.items) {
        return item.items.map((subItem) => subItem.url as string)
      }
      return []
    })
  )
}

describe('applyRoleSidebarRestrictions', () => {
  test('keeps all admin menus visible for root users', () => {
    assert.deepEqual(
      flattenUrls(applyRoleSidebarRestrictions(adminNavGroups, ROLE.SUPER_ADMIN)),
      ['/channels', '/suppliers', '/models/metadata', '/system-settings/site']
    )
  })

  test('hides channel, supplier, and system settings menus for regular administrators', () => {
    assert.deepEqual(
      flattenUrls(applyRoleSidebarRestrictions(adminNavGroups, ROLE.ADMIN)),
      ['/models/metadata']
    )
  })
})
