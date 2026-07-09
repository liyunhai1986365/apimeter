export const PUBLIC_TOP_CHROME_HEIGHT =
  'calc(var(--app-header-height,3rem)+var(--invite-promo-banner-height,0px)+var(--system-notice-banner-height,0px))'

export const PRICING_STICKY_TOP_CLASS =
  'top-[calc(var(--app-header-height,3rem)+var(--invite-promo-banner-height,0px)+var(--system-notice-banner-height,0px)+1.5rem)]'

export const PRICING_SIDEBAR_STICKY_CLASS =
  'lg:top-[calc(var(--app-header-height,3rem)+var(--invite-promo-banner-height,0px)+var(--system-notice-banner-height,0px)+1.5rem)] lg:max-h-[calc(100vh-var(--app-header-height,3rem)-var(--invite-promo-banner-height,0px)-var(--system-notice-banner-height,0px)-2rem)]'

export const PRICING_PAGE_TOP_PADDING_CLASS =
  'pt-[calc(var(--app-header-height,3rem)+var(--invite-promo-banner-height,0px)+var(--system-notice-banner-height,0px)+1.5rem)] sm:pt-[calc(var(--app-header-height,3rem)+var(--invite-promo-banner-height,0px)+var(--system-notice-banner-height,0px)+2rem)]'

export const PRICING_DETAILS_TOP_PADDING_CLASS =
  'pt-[calc(var(--app-header-height,3rem)+var(--invite-promo-banner-height,0px)+var(--system-notice-banner-height,0px)+3rem)] md:pt-[calc(var(--app-header-height,3rem)+var(--invite-promo-banner-height,0px)+var(--system-notice-banner-height,0px)+5rem)]'

function readCssPx(styles: CSSStyleDeclaration | null, name: string): number {
  if (!styles) return 0

  return parseFloat(styles.getPropertyValue(name)) || 0
}

export function getPublicTopChromeHeight(styles: CSSStyleDeclaration | null) {
  return (
    readCssPx(styles, '--app-header-height') +
    readCssPx(styles, '--invite-promo-banner-height') +
    readCssPx(styles, '--system-notice-banner-height')
  )
}
