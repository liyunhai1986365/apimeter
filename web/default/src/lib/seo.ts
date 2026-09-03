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
import { toIntlLocale } from '@/i18n/languages'
import type { TFunction } from 'i18next'
import { getSiteName } from '@/lib/site-branding'
import { getSEOCategory } from '@/features/pricing/seo-categories'

export type SEODescriptor = {
  title: string
  description: string
  robots: 'index, follow' | 'noindex, nofollow'
  canonicalPath?: string
  structuredData?: Record<string, unknown>
}

const PRIVATE_PREFIXES = [
  '/agent-management',
  '/agents',
  '/ai-creation',
  '/billing',
  '/channels',
  '/chat',
  '/console',
  '/dashboard',
  '/invite-rewards',
  '/keys',
  '/model-billing',
  '/model-monitor',
  '/model-profit',
  '/models',
  '/oauth',
  '/playground',
  '/profile',
  '/provider',
  '/redemption-codes',
  '/setup',
  '/subscriptions',
  '/suppliers',
  '/system-settings',
  '/system-tasks',
  '/usage-logs',
  '/user-subscription',
  '/users',
  '/wallet',
]

const PRIVATE_PATHS = new Set([
  '/forgot-password',
  '/invite',
  '/login',
  '/otp',
  '/register',
  '/reset',
  '/sign-in',
  '/sign-up',
  '/user/reset',
])

export function normalizeSEOPath(pathname: string): string {
  if (!pathname || pathname === '/index.html') return '/'
  return pathname !== '/' ? pathname.replace(/\/+$/, '') : pathname
}

export function getHTMLLanguage(language?: string | null): string {
  return toIntlLocale(language) ?? 'en'
}

export function resolveSEODescriptor(
  pathname: string,
  systemName: string | undefined,
  t: TFunction,
  mainlandChinaPresentationEnabled = false
): SEODescriptor {
  const siteName = getSiteName(systemName)
  const path = normalizeSEOPath(pathname)
  const homeDescription = t(
    mainlandChinaPresentationEnabled
      ? 'Connect DeepSeek, Qwen, Doubao, Kimi and other domestic models with shared keys, billing, fallback routing, and live request visibility.'
      : 'Unify OpenAI, Claude, Gemini, DeepSeek and private channels with shared keys, billing, fallback routing, and live request visibility.'
  )

  if (path === '/') {
    return {
      title: `${t('AI Model APIs, Pricing & Access')} | ${siteName}`,
      description: t(
        'Compare and access AI model providers, API pricing, supported endpoints and capabilities on {{site}}.',
        { site: siteName }
      ),
      robots: 'index, follow',
      canonicalPath: '/',
      structuredData: {
        '@context': 'https://schema.org',
        '@graph': [
          {
            '@type': 'WebSite',
            '@id': '/#website',
            name: siteName,
            url: '/',
            publisher: { '@id': '/#organization' },
          },
          {
            '@type': 'Organization',
            '@id': '/#organization',
            name: siteName,
            url: '/',
          },
        ],
      },
    }
  }

  const publicPage = getPublicPageTitle(path, t)
  if (publicPage) {
    const descriptor: SEODescriptor = {
      title: `${publicPage} | ${siteName}`,
      description: `${publicPage} - ${siteName}. ${t('Powerful API Management Platform')}`,
      robots: 'index, follow',
      canonicalPath: path,
    }
    if (path === '/pricing') {
      descriptor.title = `${t('AI Model API Pricing & Comparison')} | ${siteName}`
      descriptor.description = t(
        'Compare AI model API pricing, supported endpoints, capabilities and access options on {{site}}.',
        { site: siteName }
      )
    }
    if (path === '/providers') {
      descriptor.title = `${t('AI Model Providers')} | ${siteName}`
      descriptor.description = t(
        'Compare AI providers, available models, supported API types and pricing on {{site}}.',
        { site: siteName }
      )
    }
    descriptor.structuredData =
      path === '/pricing' || path === '/providers'
        ? {
            '@context': 'https://schema.org',
            '@type': 'CollectionPage',
            '@id': `${path}#webpage`,
            name:
              path === '/pricing'
                ? t('AI Model API Pricing & Comparison')
                : t('AI Model Providers'),
            url: path,
            isPartOf: { '@id': '/#website' },
          }
        : {
            '@context': 'https://schema.org',
            '@type': 'WebPage',
            '@id': `${path}#webpage`,
            name: publicPage,
            url: path,
            isPartOf: { '@id': '/#website' },
          }
    return descriptor
  }

  if (path.startsWith('/pricing/')) {
    const categoryPrefix = '/pricing/categories/'
    if (path.startsWith(categoryPrefix)) {
      const category = getSEOCategory(path.slice(categoryPrefix.length))
      if (category) {
        const canonicalPath = `${categoryPrefix}${category.slug}`
        return {
          title: `${t(category.titleKey)} | ${siteName}`,
          description: `${t(category.descriptionKey)} ${t(
            'Compare current pricing, supported endpoints and API access on {{site}}.',
            { site: siteName }
          )}`,
          robots: 'index, follow',
          canonicalPath,
          structuredData: {
            '@context': 'https://schema.org',
            '@graph': [
              {
                '@type': 'CollectionPage',
                '@id': `${canonicalPath}#webpage`,
                name: t(category.titleKey),
                url: canonicalPath,
                isPartOf: { '@id': '/#website' },
              },
              {
                '@type': 'BreadcrumbList',
                '@id': `${canonicalPath}#breadcrumb`,
                itemListElement: [
                  {
                    '@type': 'ListItem',
                    position: 1,
                    name: t('AI Model API Pricing & Comparison'),
                    item: '/pricing',
                  },
                  {
                    '@type': 'ListItem',
                    position: 2,
                    name: t(category.titleKey),
                    item: canonicalPath,
                  },
                ],
              },
            ],
          },
        }
      }
    }
    const modelName = decodePathSegment(path.slice('/pricing/'.length))
    if (modelName) {
      return {
        title: `${t('{{model}} API Pricing & Access', { model: modelName })} | ${siteName}`,
        description: t(
          'Compare {{model}} API pricing, supported endpoints, capabilities and access options on {{site}}.',
          { model: modelName, site: siteName }
        ),
        robots: 'index, follow',
        canonicalPath: `/pricing/${encodeURIComponent(modelName)}`,
        structuredData: {
          '@context': 'https://schema.org',
          '@graph': [
            {
              '@type': 'WebPage',
              '@id': `/pricing/${encodeURIComponent(modelName)}#webpage`,
              name: t('{{model}} API Pricing & Access', { model: modelName }),
              url: `/pricing/${encodeURIComponent(modelName)}`,
              isPartOf: { '@id': '/#website' },
              mainEntity: {
                '@id': `/pricing/${encodeURIComponent(modelName)}#service`,
              },
            },
            {
              '@type': 'Service',
              '@id': `/pricing/${encodeURIComponent(modelName)}#service`,
              name: `${modelName} API`,
              provider: { '@id': '/#organization' },
            },
            {
              '@type': 'BreadcrumbList',
              '@id': `/pricing/${encodeURIComponent(modelName)}#breadcrumb`,
              itemListElement: [
                {
                  '@type': 'ListItem',
                  position: 1,
                  name: t('AI Model API Pricing & Comparison'),
                  item: '/pricing',
                },
                {
                  '@type': 'ListItem',
                  position: 2,
                  name: modelName,
                  item: `/pricing/${encodeURIComponent(modelName)}`,
                },
              ],
            },
          ],
        },
      }
    }
  }

  if (path.startsWith('/providers/')) {
    const providerSlug = decodePathSegment(path.slice('/providers/'.length))
    if (providerSlug) {
      const providerName = providerNameFromSlug(providerSlug)
      const canonicalPath = `/providers/${encodeURIComponent(providerSlug)}`
      return {
        title: `${t('{{provider}} AI Models & APIs', { provider: providerName })} | ${siteName}`,
        description: t(
          'Compare {{provider}} models, supported API types and current pricing on {{site}}.',
          { provider: providerName, site: siteName }
        ),
        robots: 'index, follow',
        canonicalPath,
        structuredData: {
          '@context': 'https://schema.org',
          '@graph': [
            {
              '@type': 'CollectionPage',
              '@id': `${canonicalPath}#webpage`,
              name: t('{{provider}} AI Models & APIs', {
                provider: providerName,
              }),
              url: canonicalPath,
              isPartOf: { '@id': '/#website' },
            },
            {
              '@type': 'BreadcrumbList',
              '@id': `${canonicalPath}#breadcrumb`,
              itemListElement: [
                {
                  '@type': 'ListItem',
                  position: 1,
                  name: t('AI Model Providers'),
                  item: '/providers',
                },
                {
                  '@type': 'ListItem',
                  position: 2,
                  name: providerName,
                  item: canonicalPath,
                },
              ],
            },
          ],
        },
      }
    }
  }

  if (isPrivatePath(path)) {
    return {
      title:
        path === '/ai-creation'
          ? `${t('AI Creation Tools')} | ${siteName}`
          : siteName,
      description: homeDescription,
      robots: 'noindex, nofollow',
    }
  }

  return {
    title: `${t('Oops! Page Not Found!')} | ${siteName}`,
    description: homeDescription,
    robots: 'noindex, nofollow',
  }
}

function getPublicPageTitle(path: string, t: TFunction): string | undefined {
  const titles: Record<string, string> = {
    '/about': t('About'),
    '/partner': t('Partner Program'),
    '/pricing': t('Model Price'),
    '/providers': t('AI Model Providers'),
    '/privacy-policy': t('Privacy Policy'),
    '/rankings': t('Rankings'),
    '/subscription': t('Subscriptions'),
    '/user-agreement': t('User Agreement'),
  }
  return titles[path]
}

function providerNameFromSlug(slug: string): string {
  const acronyms: Record<string, string> = {
    ai: 'AI',
    api: 'API',
    aws: 'AWS',
    google: 'Google',
    openai: 'OpenAI',
  }
  return slug
    .split('-')
    .filter(Boolean)
    .map(
      (part) => acronyms[part] || part.charAt(0).toUpperCase() + part.slice(1)
    )
    .join(' ')
}

function decodePathSegment(value: string): string | undefined {
  if (!value || value.includes('/')) return undefined
  try {
    return decodeURIComponent(value)
  } catch {
    return undefined
  }
}

function isPrivatePath(path: string): boolean {
  if (PRIVATE_PATHS.has(path)) return true
  if (/^\/(401|403|404|500|503)$/.test(path)) return true
  return PRIVATE_PREFIXES.some(
    (prefix) => path === prefix || path.startsWith(`${prefix}/`)
  )
}
