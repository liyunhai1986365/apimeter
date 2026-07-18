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
import { useEffect } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { getHTMLLanguage, resolveSEODescriptor } from '@/lib/seo'
import { getSiteName } from '@/lib/site-branding'

function upsertMeta(selector: string, attributes: Record<string, string>): void {
  let element = document.head.querySelector<HTMLMetaElement>(selector)
  if (!element) {
    element = document.createElement('meta')
    document.head.append(element)
  }
  Object.entries(attributes).forEach(([name, value]) => {
    element?.setAttribute(name, value)
  })
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

export function SEOManager() {
  const { i18n, t } = useTranslation()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const routeStatusCode = useRouterState({
    select: (state) => state.statusCode,
  })
  const systemName = useSystemConfigStore((state) => state.config.systemName)

  useEffect(() => {
    const descriptor = resolveSEODescriptor(pathname, systemName, t)
    if (routeStatusCode === 404) {
      descriptor.title = `${t('Oops! Page Not Found!')} | ${getSiteName(systemName)}`
      descriptor.robots = 'noindex, nofollow'
      descriptor.canonicalPath = undefined
    }
    const canonical = descriptor.canonicalPath
      ? new URL(descriptor.canonicalPath, window.location.origin).toString()
      : undefined

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
    upsertMeta('meta[property="og:description"]', {
      property: 'og:description',
      content: descriptor.description,
    })
    upsertMeta('meta[property="og:url"]', {
      property: 'og:url',
      content: canonical ?? window.location.origin,
    })
    upsertMeta('meta[name="twitter:title"]', {
      name: 'twitter:title',
      content: descriptor.title,
    })
    upsertMeta('meta[name="twitter:description"]', {
      name: 'twitter:description',
      content: descriptor.description,
    })
    upsertCanonical(canonical)
  }, [i18n.resolvedLanguage, pathname, routeStatusCode, systemName, t])

  return null
}
