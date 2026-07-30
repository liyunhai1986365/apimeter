export const BILLING_CENTER_SECTIONS = ['monthly'] as const

export type BillingCenterSectionId = (typeof BILLING_CENTER_SECTIONS)[number]

export const BILLING_CENTER_DEFAULT_SECTION: BillingCenterSectionId = 'monthly'

export function isBillingCenterSectionId(
  section: string | undefined
): section is BillingCenterSectionId {
  return Boolean(
    section && (BILLING_CENTER_SECTIONS as readonly string[]).includes(section)
  )
}
