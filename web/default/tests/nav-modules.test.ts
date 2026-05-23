/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, it } from 'vitest'
import {
  getOrderedHeaderNavItems,
  parseHeaderNavModules,
} from '../src/lib/nav-modules'

describe('header navigation modules', () => {
  it('includes Agent Access after Home by default', () => {
    const modules = parseHeaderNavModules('')
    const items = getOrderedHeaderNavItems(modules)

    expect(items.map((item) => item.id).slice(0, 3)).toEqual([
      'home',
      'agentAccess',
      'console',
    ])
    expect(items.find((item) => item.id === 'agentAccess')).toMatchObject({
      titleKey: 'Agent Access',
      href: 'https://docs.modelsell.com',
      external: true,
      newWindow: false,
    })
  })

  it('keeps legacy boolean settings while adding missing default modules', () => {
    const modules = parseHeaderNavModules('{"docs":false,"about":false}')
    const items = getOrderedHeaderNavItems(modules)

    expect(items.map((item) => item.id)).toEqual([
      'home',
      'agentAccess',
      'console',
      'pricing',
      'rankings',
    ])
  })

  it('respects configured order and custom links', () => {
    const modules = parseHeaderNavModules({
      newWindow: {
        agentAccess: true,
      },
      order: ['home', 'custom:status', 'agentAccess', 'docs'],
      customLinks: [
        {
          id: 'status',
          title: 'Status',
          href: 'https://status.example.com',
          enabled: true,
          external: true,
          newWindow: false,
        },
      ],
      console: false,
      pricing: false,
      rankings: false,
      about: false,
    })
    const items = getOrderedHeaderNavItems(modules)

    expect(items.map((item) => item.id)).toEqual([
      'home',
      'custom:status',
      'agentAccess',
      'docs',
    ])
    expect(items.find((item) => item.id === 'custom:status')).toMatchObject({
      newWindow: false,
    })
    expect(items.find((item) => item.id === 'agentAccess')).toMatchObject({
      newWindow: true,
    })
  })

  it('keeps legacy external links opening in a new window', () => {
    const modules = parseHeaderNavModules({
      order: ['custom:legacy'],
      customLinks: [
        {
          id: 'legacy',
          title: 'Legacy',
          href: 'https://legacy.example.com',
          enabled: true,
          external: true,
        },
      ],
      agentAccess: false,
      console: false,
      pricing: false,
      rankings: false,
      docs: false,
      about: false,
    })

    expect(getOrderedHeaderNavItems(modules)).toContainEqual(
      expect.objectContaining({
        id: 'custom:legacy',
        newWindow: true,
      })
    )
  })
})
