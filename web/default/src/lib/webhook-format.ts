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
export type WebhookFormat = 'generic' | 'wecom'

// Only older settings without an explicit format use URL detection.
export function resolveWebhookFormat(
  format?: string,
  webhookURL?: string
): WebhookFormat {
  if (format === 'generic' || format === 'wecom') return format
  try {
    const url = new URL(webhookURL?.trim() || '')
    if (
      url.hostname.toLowerCase() === 'qyapi.weixin.qq.com' &&
      url.pathname === '/cgi-bin/webhook/send' &&
      url.searchParams.get('key')?.trim()
    ) {
      return 'wecom'
    }
  } catch {
    // Empty URLs are expected while creating a configuration.
  }
  return 'generic'
}
