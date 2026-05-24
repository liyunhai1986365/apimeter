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

export function applyCustomerServiceScript(scriptCode) {
  if (typeof document === 'undefined') return;

  const existing = document.getElementById(CUSTOMER_SERVICE_SCRIPT_ID);
  const src = extractCustomerServiceScriptSrc(scriptCode);

  if (!src) {
    existing?.remove();
    return;
  }

  if (
    existing instanceof HTMLScriptElement &&
    existing.getAttribute('src') === src
  ) {
    return;
  }

  existing?.remove();

  const script = document.createElement('script');
  script.id = CUSTOMER_SERVICE_SCRIPT_ID;
  script.src = src;
  script.async = true;
  document.head.appendChild(script);
}
