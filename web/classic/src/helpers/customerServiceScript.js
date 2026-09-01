/*
Copyright (C) 2025 QuantumNous

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

const CUSTOMER_SERVICE_SCRIPT_ID = 'customer-service-script';
const CUSTOMER_SERVICE_DISMISS_ID = 'customer-service-script-dismiss';
const CUSTOMER_SERVICE_STYLE_ID = 'customer-service-script-style';
const TIDIO_SCRIPT_RE = /(^|\/\/|https?:\/\/)code\.tidio\.co\//i;
const SALEWISELY_SCRIPT_RE =
  /(^|\/\/|https?:\/\/)file\.salewisely\.com\/sdk\/release\/salewisely-bundled\.js(?:\?|$)/i;

let dismissedForCurrentPage = false;

export function extractCustomerServiceScriptSrc(scriptCode) {
  const trimmed = String(scriptCode || '').trim();
  if (!trimmed) return '';

  const srcMatch = trimmed.match(/\bsrc\s*=\s*["']([^"']+)["']/i);
  if (srcMatch?.[1]) return srcMatch[1].trim();

  if (/^(https?:)?\/\/[^\s"'<>]+$/i.test(trimmed)) {
    return trimmed;
  }

  return '';
}

function isTidioScript(src) {
  return TIDIO_SCRIPT_RE.test(src);
}

function isSaleWiselyScript(src) {
  return SALEWISELY_SCRIPT_RE.test(src);
}

function normalizeSaleWiselyLanguage(language) {
  const normalized = String(language || '')
    .trim()
    .replaceAll('_', '-')
    .toLowerCase();

  if (
    normalized === 'zhtw' ||
    normalized === 'zh-tw' ||
    normalized === 'zh-hk' ||
    normalized === 'zh-mo' ||
    normalized.startsWith('zh-hant')
  ) {
    return 'zh-hant';
  }

  if (
    normalized === 'zh' ||
    normalized === 'zhcn' ||
    normalized === 'zh-cn' ||
    normalized === 'zh-sg' ||
    normalized.startsWith('zh-hans')
  ) {
    return 'zh';
  }

  const baseLanguage = normalized.split('-')[0];
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
    return baseLanguage;
  }

  return 'en';
}

function readStoredUser() {
  try {
    const storedUser = window.localStorage?.getItem('user');
    return storedUser ? JSON.parse(storedUser) : null;
  } catch {
    return null;
  }
}

function getUserLanguage(user) {
  const setting = user?.setting;
  if (setting && typeof setting === 'object') {
    return typeof setting.language === 'string' ? setting.language : undefined;
  }

  if (typeof setting === 'string') {
    try {
      const parsed = JSON.parse(setting);
      return typeof parsed.language === 'string' ? parsed.language : undefined;
    } catch {
      return undefined;
    }
  }

  return undefined;
}

function getSaleWiselyLanguage(user) {
  let storedLanguage = null;
  try {
    storedLanguage = window.localStorage?.getItem('i18nextLng') || null;
  } catch {
    // Ignore unavailable browser storage.
  }

  return normalizeSaleWiselyLanguage(
    getUserLanguage(user) ||
      storedLanguage ||
      document.documentElement?.lang ||
      window.navigator?.language,
  );
}

function normalizeSaleWiselyUser(user) {
  const normalized = { language: getSaleWiselyLanguage(user) };
  const userId = String(user?.id || '').trim();
  const userName = String(user?.display_name || user?.username || '').trim();
  const email = String(user?.email || '').trim();
  const phone = String(user?.phone || '').trim();
  const description = String(user?.description || '').trim();

  if (userId) normalized.userId = userId;
  if (userName) normalized.userName = userName;
  if (email) normalized.email = email;
  if (phone) normalized.phone = phone;
  if (description) normalized.description = description;

  return normalized;
}

export function syncCustomerServiceUser(user) {
  if (typeof window === 'undefined' || typeof document === 'undefined') return;

  const loginInfo = normalizeSaleWiselyUser(
    user === undefined ? readStoredUser() : user,
  );
  window._salewiselyUser = loginInfo;
  window.SaleWiselySDK?.setLoginInfo?.(loginInfo);
}

function prepareSaleWiselyGlobals() {
  syncCustomerServiceUser();
  window._salewiselyConfig = {
    ...window._salewiselyConfig,
    language:
      window._salewiselyConfig?.language ||
      window._salewiselyUser?.language ||
      'en',
  };
}

function removeTidioInjectedElements() {
  document.getElementById('tidio-chat')?.remove();
  document.querySelectorAll('iframe[src*="tidio"]').forEach((element) => {
    element.remove();
  });
  delete window.tidioChatApi;
}

function removeSaleWiselyInjectedElements() {
  try {
    window.SaleWiselySDK?.destroy?.();
  } catch {
    // Fall through to DOM cleanup if the provider teardown fails.
  }
  delete window.SaleWiselySDK;

  document.getElementById('salewisely-container')?.remove();
  document.getElementById('salewisely-style')?.remove();
  document
    .querySelectorAll('iframe[id*="salewisely"], iframe[src*="salewisely.com"]')
    .forEach((element) => {
      element.remove();
    });
}

function removeCustomerServiceElements() {
  document.getElementById(CUSTOMER_SERVICE_SCRIPT_ID)?.remove();
  document.getElementById(CUSTOMER_SERVICE_DISMISS_ID)?.remove();
  document.getElementById(CUSTOMER_SERVICE_STYLE_ID)?.remove();
  removeTidioInjectedElements();
  removeSaleWiselyInjectedElements();
}

function ensureDismissButton() {
  if (document.getElementById(CUSTOMER_SERVICE_DISMISS_ID)) return;

  if (!document.getElementById(CUSTOMER_SERVICE_STYLE_ID)) {
    const style = document.createElement('style');
    style.id = CUSTOMER_SERVICE_STYLE_ID;
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
    `;
    document.head.appendChild(style);
  }

  const button = document.createElement('button');
  button.id = CUSTOMER_SERVICE_DISMISS_ID;
  button.type = 'button';
  button.setAttribute('aria-label', 'Close customer service');
  button.textContent = '×';
  button.onclick = dismissCustomerServiceScriptForCurrentPage;
  (document.body ?? document.head).appendChild(button);
}

export function dismissCustomerServiceScriptForCurrentPage() {
  if (typeof document === 'undefined') return;

  dismissedForCurrentPage = true;
  removeCustomerServiceElements();
}

export function applyCustomerServiceScript(scriptCode) {
  if (typeof document === 'undefined') return;

  const existing = document.getElementById(CUSTOMER_SERVICE_SCRIPT_ID);
  const src = extractCustomerServiceScriptSrc(scriptCode);

  if (!src || (dismissedForCurrentPage && isTidioScript(src))) {
    removeCustomerServiceElements();
    return;
  }

  if (
    existing instanceof HTMLScriptElement &&
    existing.getAttribute('src') === src
  ) {
    if (isTidioScript(src)) ensureDismissButton();
    if (isSaleWiselyScript(src)) prepareSaleWiselyGlobals();
    return;
  }

  if (existing) {
    removeCustomerServiceElements();
  }

  if (isSaleWiselyScript(src)) {
    prepareSaleWiselyGlobals();
  }

  const script = document.createElement('script');
  script.id = CUSTOMER_SERVICE_SCRIPT_ID;
  script.src = src;
  script.async = true;
  if (isSaleWiselyScript(src)) {
    script.fetchPriority = 'low';
  }
  document.head.appendChild(script);

  if (isTidioScript(src)) {
    ensureDismissButton();
  } else {
    document.getElementById(CUSTOMER_SERVICE_DISMISS_ID)?.remove();
    document.getElementById(CUSTOMER_SERVICE_STYLE_ID)?.remove();
  }
}
