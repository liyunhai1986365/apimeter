export const BILLING_CENTER_SECTIONS = [
  'current',
  'monthly',
  'reconciliation',
  'ledger',
] as const

export type BillingCenterSectionId = (typeof BILLING_CENTER_SECTIONS)[number]

export const BILLING_CENTER_DEFAULT_SECTION: BillingCenterSectionId = 'current'

export function isBillingCenterSectionId(
  section: string | undefined
): section is BillingCenterSectionId {
  return Boolean(
    section && (BILLING_CENTER_SECTIONS as readonly string[]).includes(section)
  )
}
