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
type StatusRecord = Record<string, unknown> | null | undefined

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object'
    ? (value as Record<string, unknown>)
    : undefined
}

function asString(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function withoutTrailingSlash(value: string): string {
  return value.replace(/\/+$/, '')
}

function currentOrigin(): string | undefined {
  if (typeof window === 'undefined') return undefined
  return withoutTrailingSlash(window.location.origin)
}

function currentHost(): string | undefined {
  if (typeof window === 'undefined') return undefined
  return window.location.host
}

function normalizeHost(host?: string): string {
  return (host || '').trim().toLowerCase().replace(/\.$/, '')
}

function hostnameFromURL(value?: string): string {
  if (!value) return ''
  try {
    return normalizeHost(new URL(value).host)
  } catch {
    return ''
  }
}

function getStatusData(status: StatusRecord): Record<string, unknown> | undefined {
  return asRecord(status?.data)
}

export function getStatusAgentDomain(status: StatusRecord): string | undefined {
  const directAgent = asRecord(status?.agent)
  const dataAgent = asRecord(getStatusData(status)?.agent)
  return (
    asString(directAgent?.domain) ??
    asString(directAgent?.Domain) ??
    asString(dataAgent?.domain) ??
    asString(dataAgent?.Domain)
  )
}

export function isAgentSiteStatus(status: StatusRecord): boolean {
  return Boolean(getStatusAgentDomain(status))
}

export function getStatusServerAddress(
  status: StatusRecord
): string | undefined {
  const data = getStatusData(status)
  const value =
    asString(status?.server_address) ??
    asString(status?.serverAddress) ??
    asString(data?.server_address) ??
    asString(data?.serverAddress)
  return value ? withoutTrailingSlash(value) : undefined
}

function buildOriginFromDomain(domain: string, schemeSource?: string): string {
  let scheme = 'https:'
  if (schemeSource) {
    try {
      scheme = new URL(schemeSource).protocol || scheme
    } catch {
      /* empty */
    }
  } else if (typeof window !== 'undefined') {
    scheme = window.location.protocol || scheme
  }
  return `${scheme}//${domain}`
}

export function getPublicServerAddress(
  status?: StatusRecord,
  fallback?: string
): string {
  const agentDomain = getStatusAgentDomain(status)
  const statusServerAddress = getStatusServerAddress(status)
  const browserOrigin = currentOrigin()

  if (agentDomain) {
    const normalizedAgentDomain = normalizeHost(agentDomain)
    if (normalizeHost(currentHost()) === normalizedAgentDomain && browserOrigin) {
      return browserOrigin
    }
    if (
      statusServerAddress &&
      hostnameFromURL(statusServerAddress) === normalizedAgentDomain
    ) {
      return statusServerAddress
    }
    return buildOriginFromDomain(
      agentDomain,
      statusServerAddress ?? browserOrigin ?? fallback
    )
  }

  return (
    statusServerAddress ??
    (fallback ? withoutTrailingSlash(fallback) : undefined) ??
    browserOrigin ??
    ''
  )
}

export function getStoredPublicServerAddress(fallback?: string): string {
  if (typeof window === 'undefined') return fallback ?? ''
  try {
    const raw = window.localStorage.getItem('status')
    if (raw) {
      return getPublicServerAddress(JSON.parse(raw), fallback)
    }
  } catch {
    /* empty */
  }
  return getPublicServerAddress(undefined, fallback)
}
