export const PUBLIC_HEADER_RESTING_TOP_CLASS =
  'top-[calc(var(--invite-promo-banner-height,0px)+var(--system-notice-banner-height,0px))]'

export const PUBLIC_HEADER_FLOATING_TOP_CLASS =
  'top-[var(--system-notice-banner-height,0px)]'

export function getPublicHeaderTopClass(hasScrolled: boolean) {
  return hasScrolled
    ? PUBLIC_HEADER_FLOATING_TOP_CLASS
    : PUBLIC_HEADER_RESTING_TOP_CLASS
}
