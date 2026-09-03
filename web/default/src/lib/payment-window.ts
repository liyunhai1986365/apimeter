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

type WindowOpener = (
  url?: string | URL,
  target?: string,
  features?: string
) => Window | null

const defaultWindowOpener: WindowOpener = (url, target, features) =>
  window.open(url, target, features)

export function isSafePaymentUrl(value: string): boolean {
  const trimmed = value.trim()
  if (!trimmed) return false

  try {
    const url = new URL(trimmed)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

export function openPendingPaymentWindow(
  openWindow: WindowOpener = defaultWindowOpener
): Window | null {
  const paymentWindow = openWindow('about:blank', '_blank')
  if (paymentWindow) {
    paymentWindow.opener = null
  }
  return paymentWindow
}

export function navigatePendingPaymentWindow(
  paymentWindow: Window | null,
  checkoutUrl: string,
  openWindow: WindowOpener = defaultWindowOpener
): void {
  if (paymentWindow && !paymentWindow.closed) {
    paymentWindow.location.replace(checkoutUrl)
    return
  }

  openWindow(checkoutUrl, '_blank', 'noopener,noreferrer')
}

export function closePendingPaymentWindow(paymentWindow: Window | null): void {
  if (paymentWindow && !paymentWindow.closed) {
    paymentWindow.close()
  }
}
