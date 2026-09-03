import type { PricingGroupDisplayConfig } from '../types'

export function isGroupDiscountHidden(
  group: string,
  config?: PricingGroupDisplayConfig
): boolean {
  return (
    config?.groups.some(
      (item) => item.group === group && item.hide_discount === true
    ) ?? false
  )
}

export function getHiddenDiscountGroups(
  config?: PricingGroupDisplayConfig
): Set<string> {
  return new Set(
    (config?.groups ?? [])
      .filter((item) => item.hide_discount === true)
      .map((item) => item.group)
  )
}
