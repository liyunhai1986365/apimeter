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
  getAICreationSSOUrlFromStatus,
  parseHeaderNavModules,
  serializeHeaderNavModules,
  withAICreationBaseUrl,
} from '../src/lib/nav-modules'

describe('header navigation modules', () => {
  it('keeps AI Creation disabled until an OpenMosaic origin is configured', () => {
    const modules = parseHeaderNavModules('')

    expect(modules.aiCreation).toEqual({ enabled: false, baseUrl: '' })
    expect(
      getOrderedHeaderNavItems(modules).some((item) => item.id === 'aiCreation')
    ).toBe(false)
  })

  it('builds the embedded AI Creation route for the configured OpenMosaic origin', () => {
    const modules = parseHeaderNavModules({
      aiCreation: {
        enabled: true,
        baseUrl: 'https://openmosaic.example.com/',
      },
    })

    expect(modules.aiCreation).toEqual({
      enabled: true,
      baseUrl: 'https://openmosaic.example.com',
    })
    expect(getOrderedHeaderNavItems(modules)).toContainEqual(
      expect.objectContaining({
        id: 'aiCreation',
        titleKey: 'AI Creation',
        href: '/ai-creation',
        external: false,
        newWindow: false,
        requireAuth: true,
      })
    )
  })

  it('hides AI Creation when the configured OpenMosaic origin is invalid', () => {
    const modules = parseHeaderNavModules({
      aiCreation: {
        enabled: true,
        baseUrl: 'javascript:alert(1)',
      },
    })

    expect(modules.aiCreation).toEqual({ enabled: true, baseUrl: '' })
    expect(
      getOrderedHeaderNavItems(modules).some((item) => item.id === 'aiCreation')
    ).toBe(false)
  })

  it('preserves the normalized AI Creation configuration when serialized', () => {
    const modules = parseHeaderNavModules({
      aiCreation: {
        enabled: true,
        baseUrl: 'http://localhost:9520/',
      },
    })

    const serialized = JSON.parse(serializeHeaderNavModules(modules))

    expect(serialized.aiCreation).toEqual({
      enabled: true,
      baseUrl: 'http://localhost:9520',
    })
    expect(serialized.order).toContain('aiCreation')
  })

  it('updates the OpenMosaic origin without changing the AI Creation switch', () => {
    const modules = parseHeaderNavModules({
      aiCreation: { enabled: true, baseUrl: '' },
    })

    const updated = withAICreationBaseUrl(
      modules,
      'https://create.example.com/'
    )

    expect(updated.aiCreation).toEqual({
      enabled: true,
      baseUrl: 'https://create.example.com/',
    })
  })

  it('resolves the configured AI Creation URL directly from public status', () => {
    expect(
      getAICreationSSOUrlFromStatus({
        HeaderNavModules: JSON.stringify({
          aiCreation: {
            enabled: true,
            baseUrl: 'https://create.example.com',
          },
        }),
      })
    ).toBe('/ai-creation')
  })

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
      href: '/docs/apps',
      external: true,
      newWindow: false,
    })
  })

  it('can build Agent Access from the configured server address', () => {
    const modules = parseHeaderNavModules('')
    const items = getOrderedHeaderNavItems(
      modules,
      undefined,
      'https://api.example.com/docs/apps'
    )

    expect(items.find((item) => item.id === 'agentAccess')).toMatchObject({
      href: 'https://api.example.com/docs/apps',
      external: true,
    })
  })

  it('treats same-origin docs links as document routes outside the SPA', () => {
    const modules = parseHeaderNavModules('')
    const items = getOrderedHeaderNavItems(modules, '/zh/docs')

    expect(items.find((item) => item.id === 'agentAccess')).toMatchObject({
      href: '/docs/apps',
      external: true,
      newWindow: false,
    })
    expect(items.find((item) => item.id === 'docs')).toMatchObject({
      href: '/zh/docs',
      external: true,
      newWindow: true,
    })
  })

  it('forces configured same-origin docs custom links to leave the SPA', () => {
    const modules = parseHeaderNavModules({
      order: ['custom:localized-docs'],
      customLinks: [
        {
          id: 'localized-docs',
          title: 'Localized Docs',
          href: '/zh/docs',
          enabled: true,
          external: false,
          newWindow: false,
        },
      ],
      home: false,
      agentAccess: false,
      console: false,
      pricing: false,
      subscription: false,
      rankings: false,
      docs: false,
      about: false,
    })

    expect(getOrderedHeaderNavItems(modules)).toContainEqual(
      expect.objectContaining({
        id: 'custom:localized-docs',
        href: '/zh/docs',
        external: true,
      })
    )
  })

  it('keeps legacy boolean settings while adding missing default modules', () => {
    const modules = parseHeaderNavModules('{"docs":false,"about":false}')
    const items = getOrderedHeaderNavItems(modules)

    expect(items.map((item) => item.id)).toEqual([
      'home',
      'agentAccess',
      'console',
      'subscription',
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
      subscription: false,
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
