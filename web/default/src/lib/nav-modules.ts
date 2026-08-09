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
export type ModuleAccess = { enabled: boolean; requireAuth: boolean }

export type AICreationConfig = { enabled: boolean; baseUrl: string }

export type HeaderNavModule = 'rankings' | 'pricing' | 'subscription'

export type HeaderNavBuiltInModule =
  | 'home'
  | 'partner'
  | 'agentAccess'
  | 'aiCreation'
  | 'console'
  | 'pricing'
  | 'subscription'
  | 'rankings'
  | 'docs'
  | 'about'

export type HeaderNavCustomLink = {
  id: string
  title: string
  href: string
  enabled: boolean
  external: boolean
  newWindow: boolean
  requireAuth?: boolean
}

export type HeaderNavItem = {
  id: HeaderNavBuiltInModule | `custom:${string}`
  titleKey: string
  href: string
  enabled: boolean
  external: boolean
  newWindow: boolean
  requireAuth: boolean
  custom?: boolean
}

export type HeaderNavNewWindow = Partial<
  Record<HeaderNavBuiltInModule, boolean>
>

export type HeaderNavModules = {
  home: boolean
  partner: boolean
  agentAccess: boolean
  aiCreation: AICreationConfig
  console: boolean
  pricing: ModuleAccess
  subscription: ModuleAccess
  rankings: ModuleAccess
  docs: boolean
  about: boolean
  newWindow: HeaderNavNewWindow
  order: string[]
  customLinks: HeaderNavCustomLink[]
  [key: string]:
    | boolean
    | ModuleAccess
    | AICreationConfig
    | HeaderNavNewWindow
    | string[]
    | HeaderNavCustomLink[]
}

const DEFAULT_HEADER_NAV_MODULES: HeaderNavModules = {
  home: true,
  partner: true,
  agentAccess: true,
  aiCreation: { enabled: false, baseUrl: '' },
  console: true,
  pricing: { enabled: true, requireAuth: false },
  subscription: { enabled: true, requireAuth: false },
  rankings: { enabled: true, requireAuth: false },
  docs: true,
  about: true,
  newWindow: {
    agentAccess: false,
    docs: true,
  },
  order: [
    'home',
    'partner',
    'agentAccess',
    'aiCreation',
    'console',
    'subscription',
    'pricing',
    'rankings',
    'docs',
    'about',
  ],
  customLinks: [],
}

const DEFAULTS: Record<HeaderNavModule, ModuleAccess> = {
  pricing: DEFAULT_HEADER_NAV_MODULES.pricing,
  subscription: DEFAULT_HEADER_NAV_MODULES.subscription,
  rankings: DEFAULT_HEADER_NAV_MODULES.rankings,
}

const BUILT_IN_HEADER_NAV_ITEMS: Record<
  HeaderNavBuiltInModule,
  Omit<HeaderNavItem, 'enabled' | 'requireAuth'>
> = {
  home: {
    id: 'home',
    titleKey: 'Home',
    href: '/',
    external: false,
    newWindow: false,
  },
  partner: {
    id: 'partner',
    titleKey: 'Partner Program',
    href: '/partner',
    external: false,
    newWindow: false,
  },
  agentAccess: {
    id: 'agentAccess',
    titleKey: 'Agent Access',
    href: '/docs/apps',
    external: false,
    newWindow: false,
  },
  aiCreation: {
    id: 'aiCreation',
    titleKey: 'AI Creation',
    href: '/ai-creation',
    external: false,
    newWindow: false,
  },
  console: {
    id: 'console',
    titleKey: 'Console',
    href: '/dashboard',
    external: false,
    newWindow: false,
  },
  pricing: {
    id: 'pricing',
    titleKey: 'Model Price',
    href: '/pricing',
    external: false,
    newWindow: false,
  },
  subscription: {
    id: 'subscription',
    titleKey: 'Subscriptions',
    href: '/subscription',
    external: false,
    newWindow: false,
  },
  rankings: {
    id: 'rankings',
    titleKey: 'Rankings',
    href: '/rankings',
    external: false,
    newWindow: false,
  },
  docs: {
    id: 'docs',
    titleKey: 'Docs',
    href: '/docs',
    external: false,
    newWindow: true,
  },
  about: {
    id: 'about',
    titleKey: 'About',
    href: '/about',
    external: false,
    newWindow: false,
  },
}

export const BUILT_IN_HEADER_NAV_ORDER =
  DEFAULT_HEADER_NAV_MODULES.order as HeaderNavBuiltInModule[]

function cloneHeaderNavDefaults(): HeaderNavModules {
  return {
    ...DEFAULT_HEADER_NAV_MODULES,
    aiCreation: { ...DEFAULT_HEADER_NAV_MODULES.aiCreation },
    pricing: { ...DEFAULT_HEADER_NAV_MODULES.pricing },
    subscription: { ...DEFAULT_HEADER_NAV_MODULES.subscription },
    rankings: { ...DEFAULT_HEADER_NAV_MODULES.rankings },
    newWindow: { ...DEFAULT_HEADER_NAV_MODULES.newWindow },
    order: [...DEFAULT_HEADER_NAV_MODULES.order],
    customLinks: [...DEFAULT_HEADER_NAV_MODULES.customLinks],
  }
}

export function parseHeaderNavBoolean(
  raw: unknown,
  fallback: boolean
): boolean {
  if (typeof raw === 'boolean') return raw
  if (typeof raw === 'number') {
    if (raw === 1) return true
    if (raw === 0) return false
    return fallback
  }
  if (typeof raw === 'string') {
    const normalized = raw.trim().toLowerCase()
    if (normalized === 'true' || normalized === '1') return true
    if (normalized === 'false' || normalized === '0') return false
  }
  return fallback
}

function parseAccess(raw: unknown, fallback: ModuleAccess): ModuleAccess {
  if (
    typeof raw === 'boolean' ||
    typeof raw === 'number' ||
    typeof raw === 'string'
  ) {
    return {
      enabled: parseHeaderNavBoolean(raw, fallback.enabled),
      requireAuth: fallback.requireAuth,
    }
  }
  if (raw && typeof raw === 'object') {
    const r = raw as Record<string, unknown>
    return {
      enabled: parseHeaderNavBoolean(r.enabled, fallback.enabled),
      requireAuth: parseHeaderNavBoolean(r.requireAuth, fallback.requireAuth),
    }
  }
  return { ...fallback }
}

export function normalizeOpenMosaicBaseUrl(raw: unknown): string {
  if (typeof raw !== 'string' || !raw.trim()) return ''

  try {
    const url = new URL(raw.trim())
    if (
      (url.protocol !== 'http:' && url.protocol !== 'https:') ||
      url.username ||
      url.password ||
      url.pathname !== '/' ||
      url.search ||
      url.hash
    ) {
      return ''
    }
    return url.origin
  } catch {
    return ''
  }
}

function parseAICreation(
  raw: unknown,
  fallback: AICreationConfig
): AICreationConfig {
  if (!raw || typeof raw !== 'object') return { ...fallback }

  const record = raw as Record<string, unknown>
  return {
    enabled: parseHeaderNavBoolean(record.enabled, fallback.enabled),
    baseUrl: normalizeOpenMosaicBaseUrl(record.baseUrl),
  }
}

function parseHeaderNavRecord(raw: unknown): Record<string, unknown> | null {
  if (!raw || String(raw).trim() === '') return null
  if (raw && typeof raw === 'object') return raw as Record<string, unknown>

  try {
    return JSON.parse(String(raw)) as Record<string, unknown>
  } catch {
    return null
  }
}

function parseStringList(raw: unknown, fallback: string[]): string[] {
  if (!Array.isArray(raw)) return [...fallback]

  const seen = new Set<string>()
  return raw.reduce<string[]>((acc, item) => {
    if (typeof item !== 'string') return acc
    const trimmed = item.trim()
    if (!trimmed || seen.has(trimmed)) return acc
    seen.add(trimmed)
    acc.push(trimmed)
    return acc
  }, [])
}

function parseBuiltInNewWindow(
  raw: unknown,
  fallback: HeaderNavNewWindow
): HeaderNavNewWindow {
  if (!raw || typeof raw !== 'object') return { ...fallback }

  const record = raw as Record<string, unknown>
  return BUILT_IN_HEADER_NAV_ORDER.reduce<HeaderNavNewWindow>((acc, id) => {
    const item = BUILT_IN_HEADER_NAV_ITEMS[id]
    acc[id] = parseHeaderNavBoolean(record[id], fallback[id] ?? item.newWindow)
    return acc
  }, {})
}

function normalizeCustomLinkId(raw: unknown, index: number): string {
  if (typeof raw === 'string') {
    const normalized = raw.trim().replace(/^custom:/, '')
    if (normalized) return normalized
  }
  return `link-${index + 1}`
}

function isDocsDocumentRoute(href: string): boolean {
  return /^\/(?:[a-z]{2}\/)?docs(?:\/|$)/i.test(href.trim())
}

export function isExternalHeaderNavHref(href: string): boolean {
  return /^https?:\/\//i.test(href) || isDocsDocumentRoute(href)
}

function parseCustomLinks(raw: unknown): HeaderNavCustomLink[] {
  if (!Array.isArray(raw)) return []

  return raw.reduce<HeaderNavCustomLink[]>((acc, item, index) => {
    if (!item || typeof item !== 'object') return acc

    const record = item as Record<string, unknown>
    const title = typeof record.title === 'string' ? record.title.trim() : ''
    const href = typeof record.href === 'string' ? record.href.trim() : ''
    if (!title || !href) return acc

    const id = normalizeCustomLinkId(record.id, index)
    if (acc.some((link) => link.id === id)) return acc

    const external = isDocsDocumentRoute(href)
      ? true
      : parseHeaderNavBoolean(record.external, isExternalHeaderNavHref(href))

    acc.push({
      id,
      title,
      href,
      enabled: parseHeaderNavBoolean(record.enabled, true),
      external,
      newWindow: parseHeaderNavBoolean(record.newWindow, external),
      requireAuth: parseHeaderNavBoolean(record.requireAuth, false),
    })
    return acc
  }, [])
}

export function parseHeaderNavModules(raw: unknown): HeaderNavModules {
  const result = cloneHeaderNavDefaults()
  const parsed = parseHeaderNavRecord(raw)
  if (!parsed) {
    result.order = mergeHeaderNavOrder(result.order, result.customLinks)
    return result
  }

  Object.entries(parsed).forEach(([key, value]) => {
    if (key === 'aiCreation') {
      result.aiCreation = parseAICreation(value, result.aiCreation)
      return
    }
    if (key === 'pricing' || key === 'subscription' || key === 'rankings') {
      result[key] = parseAccess(value, result[key] as ModuleAccess)
      return
    }
    if (key === 'order') {
      result.order = parseStringList(value, result.order)
      return
    }
    if (key === 'customLinks') {
      result.customLinks = parseCustomLinks(value)
      return
    }
    if (key === 'newWindow') {
      result.newWindow = parseBuiltInNewWindow(value, result.newWindow)
      return
    }

    const fallback = result[key]
    if (
      typeof fallback === 'boolean' &&
      (typeof value === 'boolean' ||
        typeof value === 'number' ||
        typeof value === 'string')
    ) {
      result[key] = parseHeaderNavBoolean(value, fallback)
    }
  })

  result.order = mergeHeaderNavOrder(result.order, result.customLinks)
  return result
}

export function isHeaderNavBuiltInModule(
  id: string
): id is HeaderNavBuiltInModule {
  return id in BUILT_IN_HEADER_NAV_ITEMS
}

export function getBuiltInHeaderNavItem(
  id: HeaderNavBuiltInModule
): Omit<HeaderNavItem, 'enabled' | 'requireAuth'> {
  return BUILT_IN_HEADER_NAV_ITEMS[id]
}

export function getHeaderNavModuleEnabled(
  modules: HeaderNavModules,
  id: HeaderNavBuiltInModule
): boolean {
  if (id === 'aiCreation') return modules.aiCreation.enabled
  if (id === 'pricing' || id === 'subscription' || id === 'rankings') {
    return modules[id].enabled
  }
  return modules[id]
}

export function getHeaderNavModuleRequireAuth(
  modules: HeaderNavModules,
  id: HeaderNavBuiltInModule
): boolean {
  if (id === 'aiCreation') return true
  if (id === 'pricing' || id === 'subscription' || id === 'rankings') {
    return modules[id].requireAuth
  }
  return false
}

export function getHeaderNavModuleNewWindow(
  modules: HeaderNavModules,
  id: HeaderNavBuiltInModule
): boolean {
  return parseHeaderNavBoolean(
    modules.newWindow[id],
    BUILT_IN_HEADER_NAV_ITEMS[id].newWindow
  )
}

export function withHeaderNavModuleEnabled(
  modules: HeaderNavModules,
  id: HeaderNavBuiltInModule,
  enabled: boolean
): HeaderNavModules {
  if (id === 'aiCreation') {
    return {
      ...modules,
      aiCreation: {
        ...modules.aiCreation,
        enabled,
      },
    }
  }
  if (id === 'pricing' || id === 'subscription' || id === 'rankings') {
    return {
      ...modules,
      [id]: {
        ...modules[id],
        enabled,
      },
    }
  }

  return { ...modules, [id]: enabled }
}

export function withAICreationBaseUrl(
  modules: HeaderNavModules,
  baseUrl: string
): HeaderNavModules {
  return {
    ...modules,
    aiCreation: {
      ...modules.aiCreation,
      baseUrl,
    },
  }
}

export function withHeaderNavModuleRequireAuth(
  modules: HeaderNavModules,
  id: HeaderNavBuiltInModule,
  requireAuth: boolean
): HeaderNavModules {
  if (id !== 'pricing' && id !== 'subscription' && id !== 'rankings') {
    return modules
  }

  return {
    ...modules,
    [id]: {
      ...modules[id],
      requireAuth,
    },
  }
}

export function withHeaderNavModuleNewWindow(
  modules: HeaderNavModules,
  id: HeaderNavBuiltInModule,
  newWindow: boolean
): HeaderNavModules {
  return {
    ...modules,
    newWindow: {
      ...modules.newWindow,
      [id]: newWindow,
    },
  }
}

export function serializeHeaderNavModules(modules: HeaderNavModules): string {
  const normalized: HeaderNavModules = {
    ...modules,
    aiCreation: {
      ...modules.aiCreation,
      baseUrl: normalizeOpenMosaicBaseUrl(modules.aiCreation.baseUrl),
    },
    pricing: { ...modules.pricing },
    subscription: { ...modules.subscription },
    rankings: { ...modules.rankings },
    newWindow: { ...modules.newWindow },
    order: mergeHeaderNavOrder(modules.order, modules.customLinks),
    customLinks: modules.customLinks.map((link) => ({ ...link })),
  }

  return JSON.stringify(normalized)
}

export function parseHeaderNavModulesFromStatus(
  status: Record<string, unknown> | null
): HeaderNavModules {
  return parseHeaderNavModules(status?.HeaderNavModules)
}

export function mergeHeaderNavOrder(
  order: string[],
  customLinks: HeaderNavCustomLink[] = []
): string[] {
  const validIds = new Set<string>([
    ...BUILT_IN_HEADER_NAV_ORDER,
    ...customLinks.map((link) => `custom:${link.id}`),
  ])
  const seen = new Set<string>()
  const merged = order.reduce<string[]>((acc, item) => {
    if (!validIds.has(item) || seen.has(item)) return acc
    seen.add(item)
    acc.push(item)
    return acc
  }, [])

  validIds.forEach((id) => {
    if (!seen.has(id)) merged.push(id)
  })

  return merged
}

export function getOrderedHeaderNavItems(
  modules: HeaderNavModules,
  docsLink?: string,
  agentAccessLink?: string
): HeaderNavItem[] {
  const customLinkMap = new Map(
    modules.customLinks.map((link) => [`custom:${link.id}`, link] as const)
  )

  return mergeHeaderNavOrder(modules.order, modules.customLinks).reduce<
    HeaderNavItem[]
  >((acc, id) => {
    if (id.startsWith('custom:')) {
      const customId = id as `custom:${string}`
      const customLink = customLinkMap.get(customId)
      if (customLink?.enabled) {
        acc.push({
          id: `custom:${customLink.id}`,
          titleKey: customLink.title,
          href: customLink.href,
          enabled: customLink.enabled,
          external: customLink.external,
          newWindow: customLink.newWindow,
          requireAuth: customLink.requireAuth ?? false,
          custom: true,
        })
      }
      return acc
    }

    const builtInId = id as HeaderNavBuiltInModule
    const item = BUILT_IN_HEADER_NAV_ITEMS[builtInId]
    if (!item) return acc

    if (builtInId === 'aiCreation') {
      if (modules.aiCreation.enabled && modules.aiCreation.baseUrl) {
        acc.push({
          ...item,
          enabled: true,
          requireAuth: true,
        })
      }
      return acc
    }

    if (
      builtInId === 'pricing' ||
      builtInId === 'subscription' ||
      builtInId === 'rankings'
    ) {
      const access = modules[builtInId]
      if (access.enabled) {
        acc.push({
          ...item,
          newWindow: getHeaderNavModuleNewWindow(modules, builtInId),
          enabled: access.enabled,
          requireAuth: access.requireAuth,
        })
      }
      return acc
    }

    const enabled = modules[builtInId]
    if (enabled === true) {
      const href =
        builtInId === 'agentAccess' && agentAccessLink
          ? agentAccessLink
          : builtInId === 'docs' && docsLink
            ? docsLink
            : item.href

      acc.push({
        ...item,
        href,
        external: isExternalHeaderNavHref(href),
        newWindow: getHeaderNavModuleNewWindow(modules, builtInId),
        enabled,
        requireAuth: false,
      })
    }

    return acc
  }, [])
}

export function getAICreationSSOUrl(
  modules: HeaderNavModules
): string | undefined {
  const baseUrl = normalizeOpenMosaicBaseUrl(modules.aiCreation.baseUrl)
  if (!modules.aiCreation.enabled || !baseUrl) return undefined
  return '/ai-creation'
}

export function getAICreationSSOUrlFromStatus(
  status: Record<string, unknown> | null
): string | undefined {
  return getAICreationSSOUrl(parseHeaderNavModulesFromStatus(status))
}

function getCachedStatus(): Record<string, unknown> | null {
  try {
    if (typeof window === 'undefined') return null
    const raw = window.localStorage.getItem('status')
    return raw ? (JSON.parse(raw) as Record<string, unknown>) : null
  } catch {
    return null
  }
}

function cacheStatus(status: Record<string, unknown> | null): void {
  try {
    if (typeof window !== 'undefined' && status) {
      window.localStorage.setItem('status', JSON.stringify(status))
    }
  } catch {
    /* empty */
  }
}

export function getModuleAccessFromStatus(
  status: Record<string, unknown> | null,
  module: HeaderNavModule
): ModuleAccess {
  return parseHeaderNavModulesFromStatus(status)[module] ?? DEFAULTS[module]
}

export function getModuleAccess(module: HeaderNavModule): ModuleAccess {
  return getModuleAccessFromStatus(getCachedStatus(), module)
}

export async function getFreshModuleAccess(
  module: HeaderNavModule
): Promise<ModuleAccess> {
  try {
    const { getStatus } = await import('./api')
    const status = (await getStatus()) as Record<string, unknown> | null
    cacheStatus(status)
    return getModuleAccessFromStatus(status, module)
  } catch {
    return getModuleAccess(module)
  }
}

export function isSidebarModuleEnabled(
  section: string,
  module: string
): boolean {
  const status = getCachedStatus()
  if (!status) return true

  const raw = status.SidebarModulesAdmin
  if (!raw || String(raw).trim() === '') return true

  try {
    const parsed = JSON.parse(String(raw)) as Record<
      string,
      Record<string, boolean>
    >
    const sectionConfig = parsed[section]
    if (!sectionConfig) return true
    if (sectionConfig.enabled === false) return false
    if (sectionConfig[module] === false) return false
    return true
  } catch {
    return true
  }
}
