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
import { useEffect, useRef } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { getHTMLLanguage, resolveSEODescriptor } from '@/lib/seo'
import { getSiteName } from '@/lib/site-branding'

function upsertMeta(
  selector: string,
  attributes: Record<string, string>
): void {
  let element = document.head.querySelector<HTMLMetaElement>(selector)
  if (!element) {
    element = document.createElement('meta')
    document.head.append(element)
  }
  Object.entries(attributes).forEach(([name, value]) => {
    element?.setAttribute(name, value)
  })
}

function upsertOptionalMeta(
  selector: string,
  attributes: Record<string, string> | undefined
): void {
  if (!attributes) {
    document.head.querySelector(selector)?.remove()
    return
  }
  upsertMeta(selector, attributes)
}

function upsertCanonical(href?: string): void {
  const element = document.head.querySelector<HTMLLinkElement>(
    'link[rel="canonical"]'
  )
  if (!href) {
    element?.remove()
    return
  }
  const canonical = element ?? document.createElement('link')
  canonical.setAttribute('rel', 'canonical')
  canonical.setAttribute('href', href)
  if (!element) document.head.append(canonical)
}

function upsertStructuredData(
  data: Record<string, unknown> | undefined,
  origin: string
): void {
  const existing = document.head.querySelector<HTMLScriptElement>(
    'script[data-seo-jsonld="true"]'
  )
  if (!data) {
    existing?.remove()
    return
  }
  const script = existing ?? document.createElement('script')
  script.type = 'application/ld+json'
  script.dataset.seoJsonld = 'true'
  script.textContent = JSON.stringify(absolutizeStructuredData(data, origin))
  if (!existing) document.head.append(script)
}

function absolutizeStructuredData(value: unknown, origin: string): unknown {
  if (Array.isArray(value)) {
    return value.map((item) => absolutizeStructuredData(item, origin))
  }
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [
        key,
        absolutizeStructuredData(item, origin),
      ])
    )
  }
  if (typeof value === 'string' && value.startsWith('/')) {
    return new URL(value, origin).toString()
  }
  return value
}

export function SEOManager() {
  const { i18n, t } = useTranslation()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const routeStatusCode = useRouterState({
    select: (state) => state.statusCode,
  })
  const systemName = useSystemConfigStore((state) => state.config.systemName)
  const serverAddress = useSystemConfigStore(
    (state) => state.config.serverAddress
  )
  const initialPathname = useRef(pathname)
  const initialCanonical = useRef(
    document.head.querySelector<HTMLLinkElement>('link[rel="canonical"]')?.href
  )
  const canonicalOrigin = useRef(
    getCanonicalOrigin(
      document.head.querySelector<HTMLLinkElement>('link[rel="canonical"]')
        ?.href,
      getCanonicalOrigin(serverAddress, window.location.origin)
    )
  )
  const initialTitle = useRef(document.title)
  const initialDescription = useRef(
    document.head.querySelector<HTMLMetaElement>('meta[name="description"]')
      ?.content
  )
  const initialStructuredData = useRef(
    document.head.querySelector<HTMLScriptElement>(
      'script[data-seo-jsonld="true"]'
    )?.textContent
  )

  useEffect(() => {
    const descriptor = resolveSEODescriptor(pathname, systemName, t)
    if (routeStatusCode === 404) {
      descriptor.title = `${t('Oops! Page Not Found!')} | ${getSiteName(systemName)}`
      descriptor.robots = 'noindex, nofollow'
      descriptor.canonicalPath = undefined
    }
    let canonical = descriptor.canonicalPath
      ? new URL(descriptor.canonicalPath, canonicalOrigin.current).toString()
      : undefined
    const preserveServerCatalogMetadata =
      pathname === initialPathname.current &&
      (pathname.startsWith('/pricing/') ||
        pathname.startsWith('/providers/')) &&
      Boolean(initialCanonical.current)
    if (preserveServerCatalogMetadata) {
      canonical = initialCanonical.current
      descriptor.title = initialTitle.current
      descriptor.description =
        initialDescription.current ?? descriptor.description
      if (initialStructuredData.current) {
        try {
          descriptor.structuredData = JSON.parse(
            initialStructuredData.current
          ) as Record<string, unknown>
        } catch {
          /* keep the client-generated fallback */
        }
      }
    }

    document.documentElement.lang = getHTMLLanguage(i18n.resolvedLanguage)
    document.title = descriptor.title
    upsertMeta('meta[name="title"]', {
      name: 'title',
      content: descriptor.title,
    })
    upsertMeta('meta[name="description"]', {
      name: 'description',
      content: descriptor.description,
    })
    upsertMeta('meta[name="robots"]', {
      name: 'robots',
      content: descriptor.robots,
    })
    upsertMeta('meta[property="og:title"]', {
      property: 'og:title',
      content: descriptor.title,
    })
    upsertMeta('meta[property="og:site_name"]', {
      property: 'og:site_name',
      content: getSiteName(systemName),
    })
    upsertMeta('meta[property="og:description"]', {
      property: 'og:description',
      content: descriptor.description,
    })
    upsertOptionalMeta(
      'meta[property="og:url"]',
      canonical
        ? {
            property: 'og:url',
            content: canonical,
          }
        : undefined
    )
    upsertMeta('meta[name="twitter:title"]', {
      name: 'twitter:title',
      content: descriptor.title,
    })
    upsertMeta('meta[name="twitter:card"]', {
      name: 'twitter:card',
      content: 'summary',
    })
    upsertMeta('meta[name="twitter:description"]', {
      name: 'twitter:description',
      content: descriptor.description,
    })
    upsertCanonical(canonical)
    upsertStructuredData(descriptor.structuredData, canonicalOrigin.current)
  }, [i18n.resolvedLanguage, pathname, routeStatusCode, systemName, t])

  return null
}

function getCanonicalOrigin(
  canonical: string | undefined,
  fallback: string
): string {
  if (!canonical) return fallback
  try {
    return new URL(canonical).origin
  } catch {
    return fallback
  }
}
