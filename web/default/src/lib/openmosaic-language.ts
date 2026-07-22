import { normalizeInterfaceLanguage } from '../i18n/languages'

type EmbeddedAuthorization = {
  code: string
  site_origin: string
}

export function buildOpenMosaicEmbedUrl(
  openMosaicOrigin: string,
  authorization: EmbeddedAuthorization,
  interfaceLanguage?: string | null
): string {
  const url = new URL('/auth/modelsell/embed', openMosaicOrigin)
  url.searchParams.set('code', authorization.code)
  url.searchParams.set('site_origin', authorization.site_origin)
  url.searchParams.set('redirect', '/image')
  url.searchParams.set('lang', normalizeInterfaceLanguage(interfaceLanguage))
  return url.toString()
}
