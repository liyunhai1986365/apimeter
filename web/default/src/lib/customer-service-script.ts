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
const CUSTOMER_SERVICE_SCRIPT_ID = 'customer-service-script'
const CUSTOMER_SERVICE_DISMISS_ID = 'customer-service-script-dismiss'
const CUSTOMER_SERVICE_STYLE_ID = 'customer-service-script-style'
const TIDIO_SCRIPT_RE = /(^|\/\/|https?:\/\/)code\.tidio\.co\//i
const SALEWISELY_SCRIPT_RE =
  /(^|\/\/|https?:\/\/)file\.salewisely\.com\/sdk\/release\/salewisely-bundled\.js(?:\?|$)/i

type TidioChatApi = {
  on?: (event: 'ready', callback: () => void) => void
  open: () => void
  show: () => void
}

type SaleWiselyUser = {
  id?: number | string
  username?: string
  display_name?: string
  email?: string
  phone?: string
  description?: string
  setting?: Record<string, unknown> | string
}

type SaleWiselyLoginInfo = {
  userId?: string
  userName?: string
  email?: string
  language: string
  phone?: string
  description?: string
}

type SaleWiselyConfig = {
  appId?: string
  hiddenIcon?: boolean
  language?: string
  fullscreen?: boolean
}

type SaleWiselySdk = {
  destroy?: () => void
  hideChat?: () => void | Promise<unknown>
  setLoginInfo?: (user?: SaleWiselyLoginInfo) => void
  showChat?: () => void | Promise<unknown>
}

type CustomerServiceWindow = Window & {
  tidioChatApi?: TidioChatApi
  SaleWiselySDK?: SaleWiselySdk
  _salewiselyConfig?: SaleWiselyConfig
  _salewiselyUser?: SaleWiselyLoginInfo
}

let dismissedForCurrentPage = false
let pendingTidioReadyListener: EventListener | null = null

export function extractScriptSrc(scriptCode?: string | null): string {
  const trimmed = scriptCode?.trim() ?? ''
  if (!trimmed) return ''

  const srcMatch = trimmed.match(/\bsrc\s*=\s*["']([^"']+)["']/i)
  if (srcMatch?.[1]) return srcMatch[1].trim()

  if (/^(https?:)?\/\/[^\s"'<>]+$/i.test(trimmed)) {
    return trimmed
  }

  return ''
}

function isTidioScript(src: string): boolean {
  return TIDIO_SCRIPT_RE.test(src)
}

function isSaleWiselyScript(src: string): boolean {
  return SALEWISELY_SCRIPT_RE.test(src)
}

function getTidioChatApi(): TidioChatApi | undefined {
  if (typeof window === 'undefined') return undefined
  return (window as CustomerServiceWindow).tidioChatApi
}

function showAndOpenTidioChat(): boolean {
  const api = getTidioChatApi()
  if (!api) return false

  api.show()
  api.open()
  return true
}

function getSaleWiselySdk(): SaleWiselySdk | undefined {
  if (typeof window === 'undefined') return undefined
  return (window as CustomerServiceWindow).SaleWiselySDK
}

function normalizeSaleWiselyLanguage(language?: unknown): string {
  const normalized = String(language ?? '')
    .trim()
    .replaceAll('_', '-')
    .toLowerCase()

  if (
    normalized === 'zhtw' ||
    normalized === 'zh-tw' ||
    normalized === 'zh-hk' ||
    normalized === 'zh-mo' ||
    normalized.startsWith('zh-hant')
  ) {
    return 'zh-hant'
  }

  if (
    normalized === 'zh' ||
    normalized === 'zhcn' ||
    normalized === 'zh-cn' ||
    normalized === 'zh-sg' ||
    normalized.startsWith('zh-hans')
  ) {
    return 'zh'
  }

  const baseLanguage = normalized.split('-')[0]
  if (
    [
      'en',
      'ja',
      'ko',
      'es',
      'fr',
      'de',
      'pt',
      'ru',
      'ar',
      'vi',
      'th',
      'id',
      'ms',
      'hi',
    ].includes(baseLanguage)
  ) {
    return baseLanguage
  }

  return 'en'
}

function readStoredSaleWiselyUser(): SaleWiselyUser | null {
  if (typeof window === 'undefined') return null

  try {
    const storedUser = window.localStorage?.getItem('user')
    return storedUser ? (JSON.parse(storedUser) as SaleWiselyUser) : null
  } catch {
    return null
  }
}

function getUserLanguage(user?: SaleWiselyUser | null): string | undefined {
  const setting = user?.setting
  if (setting && typeof setting === 'object') {
    return typeof setting.language === 'string' ? setting.language : undefined
  }

  if (typeof setting === 'string') {
    try {
      const parsed = JSON.parse(setting) as Record<string, unknown>
      return typeof parsed.language === 'string' ? parsed.language : undefined
    } catch {
      return undefined
    }
  }

  return undefined
}

function getSaleWiselyLanguage(user?: SaleWiselyUser | null): string {
  let storedLanguage: string | null = null
  try {
    storedLanguage = window.localStorage?.getItem('i18nextLng') ?? null
  } catch {
    // Ignore unavailable browser storage.
  }

  return normalizeSaleWiselyLanguage(
    getUserLanguage(user) ||
      storedLanguage ||
      document.documentElement?.lang ||
      window.navigator?.language
  )
}

function normalizeSaleWiselyUser(
  user?: SaleWiselyUser | null
): SaleWiselyLoginInfo {
  const normalized: SaleWiselyLoginInfo = {
    language: getSaleWiselyLanguage(user),
  }
  const userId = String(user?.id ?? '').trim()
  const userName = String(user?.display_name || user?.username || '').trim()
  const email = String(user?.email ?? '').trim()
  const phone = String(user?.phone ?? '').trim()
  const description = String(user?.description ?? '').trim()

  if (userId) normalized.userId = userId
  if (userName) normalized.userName = userName
  if (email) normalized.email = email
  if (phone) normalized.phone = phone
  if (description) normalized.description = description

  return normalized
}

export function syncCustomerServiceUser(user?: SaleWiselyUser | null): void {
  if (typeof window === 'undefined' || typeof document === 'undefined') return

  const customerServiceWindow = window as CustomerServiceWindow
  const loginInfo = normalizeSaleWiselyUser(
    user === undefined ? readStoredSaleWiselyUser() : user
  )
  customerServiceWindow._salewiselyUser = loginInfo
  customerServiceWindow.SaleWiselySDK?.setLoginInfo?.(loginInfo)
}

function prepareSaleWiselyGlobals(): void {
  const customerServiceWindow = window as CustomerServiceWindow
  syncCustomerServiceUser()
  customerServiceWindow._salewiselyConfig = {
    ...customerServiceWindow._salewiselyConfig,
    language:
      customerServiceWindow._salewiselyConfig?.language ||
      customerServiceWindow._salewiselyUser?.language ||
      'en',
  }
}

function showSaleWiselyChat(): boolean {
  const showChat = getSaleWiselySdk()?.showChat
  if (!showChat) return false

  try {
    const result = showChat()
    if (result instanceof Promise) {
      void result.catch(() => undefined)
    }
    return true
  } catch {
    return false
  }
}

function clearPendingTidioReadyListener(): void {
  if (!pendingTidioReadyListener) return
  document.removeEventListener('tidioChat-ready', pendingTidioReadyListener)
  pendingTidioReadyListener = null
}

function removeTidioInjectedElements(): void {
  clearPendingTidioReadyListener()
  document.getElementById('tidio-chat')?.remove()
  document.querySelectorAll('iframe[src*="tidio"]').forEach((element) => {
    element.remove()
  })

  if (typeof window !== 'undefined') {
    delete (window as CustomerServiceWindow).tidioChatApi
  }
}

function removeSaleWiselyInjectedElements(): void {
  if (typeof window !== 'undefined') {
    const customerServiceWindow = window as CustomerServiceWindow
    try {
      customerServiceWindow.SaleWiselySDK?.destroy?.()
    } catch {
      // Fall through to DOM cleanup if the provider teardown fails.
    }
    delete customerServiceWindow.SaleWiselySDK
  }

  document.getElementById('salewisely-container')?.remove()
  document.getElementById('salewisely-style')?.remove()
  document
    .querySelectorAll('iframe[id*="salewisely"], iframe[src*="salewisely.com"]')
    .forEach((element) => {
      element.remove()
    })
}

function removeCustomerServiceElements(): void {
  document.getElementById(CUSTOMER_SERVICE_SCRIPT_ID)?.remove()
  document.getElementById(CUSTOMER_SERVICE_DISMISS_ID)?.remove()
  document.getElementById(CUSTOMER_SERVICE_STYLE_ID)?.remove()
  removeTidioInjectedElements()
  removeSaleWiselyInjectedElements()
}

function ensureDismissButton(): void {
  if (document.getElementById(CUSTOMER_SERVICE_DISMISS_ID)) return

  if (!document.getElementById(CUSTOMER_SERVICE_STYLE_ID)) {
    const style = document.createElement('style')
    style.id = CUSTOMER_SERVICE_STYLE_ID
    style.textContent = `
      #${CUSTOMER_SERVICE_DISMISS_ID} {
        position: fixed;
        right: 14px;
        bottom: 96px;
        z-index: 2147483647;
        width: 28px;
        height: 28px;
        border: 1px solid rgba(15, 23, 42, 0.12);
        border-radius: 9999px;
        background: rgba(255, 255, 255, 0.96);
        color: #475569;
        box-shadow: 0 8px 20px rgba(15, 23, 42, 0.16);
        cursor: pointer;
        font-family: Arial, sans-serif;
        font-size: 18px;
        line-height: 1;
      }
      #${CUSTOMER_SERVICE_DISMISS_ID}:hover {
        background: #f8fafc;
        color: #0f172a;
      }
    `
    document.head.appendChild(style)
  }

  const button = document.createElement('button')
  button.id = CUSTOMER_SERVICE_DISMISS_ID
  button.type = 'button'
  button.setAttribute('aria-label', 'Close customer service')
  button.textContent = '×'
  button.onclick = dismissCustomerServiceScriptForCurrentPage
  ;(document.body ?? document.head).appendChild(button)
}

export function dismissCustomerServiceScriptForCurrentPage(): void {
  if (typeof document === 'undefined') return

  dismissedForCurrentPage = true
  removeCustomerServiceElements()
}

export function resetCustomerServiceScriptDismissal(): void {
  dismissedForCurrentPage = false
}

export function openCustomerServiceChat(scriptCode?: string | null): boolean {
  if (typeof document === 'undefined') return false

  const src = extractScriptSrc(scriptCode)
  if (isSaleWiselyScript(src)) {
    resetCustomerServiceScriptDismissal()
    applyCustomerServiceScript(scriptCode)

    if (showSaleWiselyChat()) return true

    const script = document.getElementById(CUSTOMER_SERVICE_SCRIPT_ID)
    if (!(script instanceof HTMLScriptElement)) return false

    script.addEventListener('load', showSaleWiselyChat, { once: true })
    return true
  }

  if (!isTidioScript(src)) return false

  const onReady = () => {
    pendingTidioReadyListener = null
    showAndOpenTidioChat()
  }

  resetCustomerServiceScriptDismissal()
  applyCustomerServiceScript(scriptCode)
  clearPendingTidioReadyListener()
  pendingTidioReadyListener = onReady
  document.addEventListener('tidioChat-ready', onReady, { once: true })

  if (showAndOpenTidioChat()) {
    document.removeEventListener('tidioChat-ready', onReady)
    pendingTidioReadyListener = null
  }

  return true
}

export function applyCustomerServiceScript(scriptCode?: string | null): void {
  if (typeof document === 'undefined') return

  const existing = document.getElementById(CUSTOMER_SERVICE_SCRIPT_ID)
  const src = extractScriptSrc(scriptCode)

  if (!src || (dismissedForCurrentPage && isTidioScript(src))) {
    removeCustomerServiceElements()
    return
  }

  if (
    existing instanceof HTMLScriptElement &&
    existing.getAttribute('src') === src
  ) {
    if (isTidioScript(src)) ensureDismissButton()
    if (isSaleWiselyScript(src)) prepareSaleWiselyGlobals()
    return
  }

  if (existing) {
    removeCustomerServiceElements()
  }

  if (isSaleWiselyScript(src)) {
    prepareSaleWiselyGlobals()
  }

  const script = document.createElement('script')
  script.id = CUSTOMER_SERVICE_SCRIPT_ID
  script.src = src
  script.async = true
  if (isSaleWiselyScript(src)) {
    script.fetchPriority = 'low'
  }
  document.head.appendChild(script)

  if (isTidioScript(src)) {
    ensureDismissButton()
  } else {
    document.getElementById(CUSTOMER_SERVICE_DISMISS_ID)?.remove()
    document.getElementById(CUSTOMER_SERVICE_STYLE_ID)?.remove()
  }
}
