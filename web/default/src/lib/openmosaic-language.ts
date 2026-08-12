import { normalizeInterfaceLanguage } from '../i18n/languages'

type EmbeddedAuthorization = {
  code: string
  site_origin: string
}

export function buildOpenMosaicStartUrl(openMosaicOrigin: string): string {
  const url = new URL('/api/auth/apimeter/start', openMosaicOrigin)
  url.searchParams.set('redirect', '/home')
  return url.toString()
}

export function buildOpenMosaicCallbackUrl(openMosaicOrigin: string): string {
  return new URL('/auth/apimeter/callback', openMosaicOrigin).toString()
}

export function buildOpenMosaicEmbedUrl(
  openMosaicOrigin: string,
  authorization: EmbeddedAuthorization,
  interfaceLanguage?: string | null
): string {
  const url = new URL('/auth/apimeter/embed', openMosaicOrigin)
  url.searchParams.set('code', authorization.code)
  url.searchParams.set('site_origin', authorization.site_origin)
  url.searchParams.set('redirect', '/image')
  url.searchParams.set('lang', normalizeInterfaceLanguage(interfaceLanguage))
  return url.toString()
}
