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
import { DEFAULT_SYSTEM_NAME } from '@/lib/constants'

const MODELSELL_NAME = 'Modelsell'
const DEFAULT_SERVER_ADDRESS = ''

function trimSlash(value: string): string {
  return value.trim().replace(/\/+$/, '')
}

export function getSiteName(systemName?: string): string {
  return systemName?.trim() || DEFAULT_SYSTEM_NAME
}

export function getCliDisplayName(systemName?: string): string {
  return `${getSiteName(systemName)} CLI`
}

export function getSitePlanName(systemName?: string): string {
  return `${getSiteName(systemName)} Tokens Plan`
}

export function formatSystemTemplate(
  template: string,
  systemName?: string
): string {
  const siteName = getSiteName(systemName)
  return template
    .replaceAll(MODELSELL_NAME, siteName)
    .replaceAll('ModelSell', siteName)
}

export function getSiteServerAddress(serverAddress?: string): string {
  const configured = trimSlash(serverAddress || '')
  if (configured) return configured
  if (typeof window !== 'undefined') return trimSlash(window.location.origin)
  return DEFAULT_SERVER_ADDRESS
}

export function buildSitePathURL(
  serverAddress: string | undefined,
  path: string
): string {
  const base = getSiteServerAddress(serverAddress)
  if (!base) return path
  return `${base}/${path.replace(/^\/+/, '')}`
}

export function getCliInstallCommands(
  serverAddress?: string
): Record<'unix' | 'windows', string> {
  const staticBase = buildSitePathURL(serverAddress, '/modelsell-cli')
  return {
    unix: `curl -fsSL ${staticBase}/install.sh | sh`,
    windows: `irm ${staticBase}/install.ps1 | iex`,
  }
}

export function getCliDocsURL(serverAddress?: string): string {
  return buildSitePathURL(serverAddress, '/docs/apps/modelsell-cli')
}

export function getAgentToolsURL(serverAddress?: string): string {
  return buildSitePathURL(serverAddress, '/docs/apps')
}

export function getCliScreenshotURL(serverAddress?: string): string {
  return buildSitePathURL(
    serverAddress,
    '/assets/docs/apps/modelsell/modelsell-cli.png'
  )
}
