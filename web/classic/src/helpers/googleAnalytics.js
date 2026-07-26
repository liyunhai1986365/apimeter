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

const GOOGLE_ANALYTICS_SCRIPT_ID = 'google-analytics-script';
const GOOGLE_ANALYTICS_ID_PATTERN = /^G-[A-Z0-9]{4,32}$/;

export function normalizeGoogleAnalyticsId(measurementId) {
  const normalized = String(measurementId || '')
    .trim()
    .toUpperCase();
  return GOOGLE_ANALYTICS_ID_PATTERN.test(normalized) ? normalized : '';
}

export function applyGoogleAnalytics(measurementId) {
  if (typeof window === 'undefined' || typeof document === 'undefined') return;

  const normalized = normalizeGoogleAnalyticsId(measurementId);
  const existing = document.getElementById(GOOGLE_ANALYTICS_SCRIPT_ID);
  const existingMeasurementId =
    existing instanceof HTMLScriptElement
      ? existing.dataset.measurementId
      : undefined;

  if (existingMeasurementId && existingMeasurementId !== normalized) {
    window[`ga-disable-${existingMeasurementId}`] = true;
  }

  if (!normalized) {
    existing?.remove();
    return;
  }

  if (
    existing instanceof HTMLScriptElement &&
    existing.dataset.measurementId === normalized
  ) {
    return;
  }

  existing?.remove();

  const script = document.createElement('script');
  script.id = GOOGLE_ANALYTICS_SCRIPT_ID;
  script.async = true;
  script.src = `https://www.googletagmanager.com/gtag/js?id=${encodeURIComponent(normalized)}`;
  script.dataset.measurementId = normalized;
  document.head.appendChild(script);

  window.dataLayer = window.dataLayer || [];
  window[`ga-disable-${normalized}`] = false;
  window.gtag =
    window.gtag ||
    function () {
      window.dataLayer.push(arguments);
    };
  window.gtag('js', new Date());
  window.gtag('config', normalized);
}
