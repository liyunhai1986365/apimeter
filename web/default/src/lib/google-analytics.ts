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

const GOOGLE_ANALYTICS_SCRIPT_ID = 'google-analytics-script'
const GOOGLE_ANALYTICS_ID_PATTERN = /^G-[A-Z0-9]{4,32}$/
const MARKETING_ATTRIBUTION_STORAGE_KEY = 'marketing-attribution-v1'
const MARKETING_ATTRIBUTION_TTL_MS = 30 * 24 * 60 * 60 * 1000
const MARKETING_ATTRIBUTION_VALUE_MAX_LENGTH = 100

type GoogleAnalyticsWindow = Window & {
  dataLayer?: unknown[]
  gtag?: (...args: unknown[]) => void
}

export type GoogleAnalyticsPurchase = {
  transactionId: string
  value: number
  currency: string
}

type MarketingAttribution = {
  source?: string
  medium?: string
  campaign?: string
  content?: string
  term?: string
  campaignId?: string
  adGroupId?: string
  creativeId?: string
  matchType?: string
  network?: string
  device?: string
  clickIdType?: 'gclid' | 'gbraid' | 'wbraid' | 'gad'
  landingPath?: string
  capturedAt: number
}

const pendingEvents: Array<[string, Record<string, unknown>]> = []
const trackedPurchaseIds = new Set<string>()

function getStoredAttribution(): MarketingAttribution | undefined {
  if (typeof window === 'undefined') return undefined

  try {
    const stored = window.localStorage.getItem(
      MARKETING_ATTRIBUTION_STORAGE_KEY
    )
    if (!stored) return undefined

    const attribution = JSON.parse(stored) as MarketingAttribution
    if (
      !Number.isFinite(attribution.capturedAt) ||
      Date.now() - attribution.capturedAt > MARKETING_ATTRIBUTION_TTL_MS
    ) {
      window.localStorage.removeItem(MARKETING_ATTRIBUTION_STORAGE_KEY)
      return undefined
    }
    return attribution
  } catch {
    return undefined
  }
}

function getSearchValue(
  searchParams: URLSearchParams,
  name: string
): string | undefined {
  const value = searchParams.get(name)?.trim()
  if (!value) return undefined
  return value.slice(0, MARKETING_ATTRIBUTION_VALUE_MAX_LENGTH)
}

function getClickIdType(
  searchParams: URLSearchParams
): MarketingAttribution['clickIdType'] | undefined {
  if (getSearchValue(searchParams, 'gclid')) return 'gclid'
  if (getSearchValue(searchParams, 'gbraid')) return 'gbraid'
  if (getSearchValue(searchParams, 'wbraid')) return 'wbraid'
  if (
    getSearchValue(searchParams, 'gad_source') ||
    getSearchValue(searchParams, 'gad_campaignid')
  ) {
    return 'gad'
  }
  return undefined
}

/**
 * Persist only campaign metadata, never the raw Google click identifier.
 * This keeps attribution available across auth and payment redirects without
 * introducing a second high-cardinality identifier into Analytics events.
 */
function captureMarketingAttribution(): MarketingAttribution | undefined {
  if (typeof window === 'undefined') return undefined

  let searchParams: URLSearchParams
  try {
    searchParams = new URLSearchParams(window.location?.search ?? '')
  } catch {
    return getStoredAttribution()
  }

  const clickIdType = getClickIdType(searchParams)
  const source = getSearchValue(searchParams, 'utm_source')
  const medium = getSearchValue(searchParams, 'utm_medium')
  const hasCampaignData = Boolean(
    clickIdType ||
    source ||
    medium ||
    getSearchValue(searchParams, 'utm_campaign') ||
    getSearchValue(searchParams, 'campaign_id') ||
    getSearchValue(searchParams, 'gad_campaignid')
  )

  if (!hasCampaignData) return getStoredAttribution()

  const attribution: MarketingAttribution = {
    source: source || (clickIdType ? 'google' : undefined),
    medium: medium || (clickIdType ? 'cpc' : undefined),
    campaign: getSearchValue(searchParams, 'utm_campaign'),
    content: getSearchValue(searchParams, 'utm_content'),
    term: getSearchValue(searchParams, 'utm_term'),
    campaignId:
      getSearchValue(searchParams, 'campaign_id') ||
      getSearchValue(searchParams, 'gad_campaignid'),
    adGroupId: getSearchValue(searchParams, 'adgroup_id'),
    creativeId: getSearchValue(searchParams, 'creative_id'),
    matchType: getSearchValue(searchParams, 'match_type'),
    network: getSearchValue(searchParams, 'network'),
    device: getSearchValue(searchParams, 'device'),
    clickIdType,
    landingPath: window.location?.pathname,
    capturedAt: Date.now(),
  }

  try {
    window.localStorage.setItem(
      MARKETING_ATTRIBUTION_STORAGE_KEY,
      JSON.stringify(attribution)
    )
  } catch {
    // Analytics must remain best-effort when browser storage is unavailable.
  }

  return attribution
}

function getMarketingAttributionEventParams(): Record<string, unknown> {
  const attribution = captureMarketingAttribution()
  if (!attribution) return {}

  return Object.fromEntries(
    Object.entries({
      sem_source: attribution.source,
      sem_medium: attribution.medium,
      sem_campaign: attribution.campaign,
      sem_content: attribution.content,
      sem_term: attribution.term,
      campaign_id: attribution.campaignId,
      ad_group_id: attribution.adGroupId,
      creative_id: attribution.creativeId,
      match_type: attribution.matchType,
      network: attribution.network,
      device: attribution.device,
      click_id_type: attribution.clickIdType,
      attribution_landing_path: attribution.landingPath,
    }).filter(([, value]) => value !== undefined)
  )
}

export function normalizeGoogleAnalyticsId(
  measurementId?: string | null
): string {
  const normalized = measurementId?.trim().toUpperCase() ?? ''
  return GOOGLE_ANALYTICS_ID_PATTERN.test(normalized) ? normalized : ''
}

export function applyGoogleAnalytics(measurementId?: string | null): void {
  if (typeof window === 'undefined' || typeof document === 'undefined') return

  captureMarketingAttribution()

  const normalized = normalizeGoogleAnalyticsId(measurementId)
  const existing = document.getElementById(GOOGLE_ANALYTICS_SCRIPT_ID)
  const existingMeasurementId =
    existing instanceof HTMLScriptElement
      ? existing.dataset.measurementId
      : undefined

  if (existingMeasurementId && existingMeasurementId !== normalized) {
    ;(window as unknown as Record<string, unknown>)[
      `ga-disable-${existingMeasurementId}`
    ] = true
  }

  if (!normalized) {
    existing?.remove()
    pendingEvents.length = 0
    return
  }

  if (
    existing instanceof HTMLScriptElement &&
    existing.dataset.measurementId === normalized
  ) {
    return
  }

  existing?.remove()

  const script = document.createElement('script')
  script.id = GOOGLE_ANALYTICS_SCRIPT_ID
  script.async = true
  script.src = `https://www.googletagmanager.com/gtag/js?id=${encodeURIComponent(normalized)}`
  script.dataset.measurementId = normalized
  document.head.appendChild(script)

  const analyticsWindow = window as GoogleAnalyticsWindow
  ;(window as unknown as Record<string, unknown>)[`ga-disable-${normalized}`] =
    false
  analyticsWindow.dataLayer = analyticsWindow.dataLayer || []
  analyticsWindow.gtag =
    analyticsWindow.gtag ||
    function () {
      // Google gtag.js expects an Arguments object, not a rest-parameter array.
      // eslint-disable-next-line prefer-rest-params
      analyticsWindow.dataLayer?.push(arguments)
    }
  analyticsWindow.gtag('js', new Date())
  analyticsWindow.gtag('config', normalized)

  while (pendingEvents.length > 0) {
    const [eventName, params] = pendingEvents.shift()!
    analyticsWindow.gtag('event', eventName, params)
  }
}

export function trackGoogleAnalyticsEvent(
  eventName: string,
  params: Record<string, unknown> = {}
): void {
  if (typeof window === 'undefined') return

  captureMarketingAttribution()

  const analyticsWindow = window as GoogleAnalyticsWindow
  if (analyticsWindow.gtag) {
    analyticsWindow.gtag('event', eventName, params)
    return
  }
  pendingEvents.push([eventName, params])
}

export function trackSignUp(method: string): void {
  trackGoogleAnalyticsEvent('sign_up', {
    method,
    ...getMarketingAttributionEventParams(),
  })
}

export function trackPurchase({
  transactionId,
  value,
  currency,
}: GoogleAnalyticsPurchase): void {
  if (
    !transactionId ||
    trackedPurchaseIds.has(transactionId) ||
    !Number.isFinite(value) ||
    value < 0 ||
    !currency
  ) {
    return
  }

  trackedPurchaseIds.add(transactionId)

  trackGoogleAnalyticsEvent('purchase', {
    transaction_id: transactionId,
    value,
    currency: currency.toUpperCase(),
    payment_type: 'stripe',
    ...getMarketingAttributionEventParams(),
    items: [
      {
        item_id: 'stripe_payment',
        item_name: 'Stripe payment',
        price: value,
        quantity: 1,
      },
    ],
  })
}
