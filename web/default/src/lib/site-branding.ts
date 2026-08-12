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

const APIMETER_NAME = 'APIMeter'
const APIMETER_CLI_SECTION_HOSTS = new Set([
  'apimeter.ai',
  'www.apimeter.ai',
])
const DEFAULT_SERVER_ADDRESS = ''
const buildEnv = (
  import.meta as ImportMeta & {
    env?: Record<string, string | boolean | undefined>
  }
).env
const APIMETER_STATIC_URL = trimSlash(
  String(
    buildEnv?.VITE_APIMETER_STATIC_URL || 'https://static.apimeter.ai'
  )
)
const APIMETER_DOCS_URL = trimSlash(
  String(buildEnv?.VITE_APIMETER_DOCS_URL || 'https://docs.apimeter.ai')
)

function trimSlash(value: string): string {
  return value.trim().replace(/\/+$/, '')
}

export function getSiteName(systemName?: string): string {
  return systemName?.trim() || DEFAULT_SYSTEM_NAME
}

export function getCliDisplayName(systemName?: string): string {
  return `${getSiteName(systemName)} CLI`
}

export function shouldShowAPIMeterCliSection(hostname?: string): boolean {
  const host =
    hostname ??
    (typeof window !== 'undefined' ? window.location.hostname : undefined)
  return APIMETER_CLI_SECTION_HOSTS.has((host || '').trim().toLowerCase())
}

export function getSitePlanName(systemName?: string): string {
  return `${getSiteName(systemName)} Tokens Plan`
}

export function formatSystemTemplate(
  template: string,
  systemName?: string
): string {
  const siteName = getSiteName(systemName)
  return template.replaceAll(APIMETER_NAME, siteName)
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
  _serverAddress?: string
): Record<'unix' | 'windows', string> {
  const staticBase = buildSitePathURL(APIMETER_STATIC_URL, '/apimeter-cli')
  return {
    unix: `curl -fsSL ${staticBase}/install.sh | sh`,
    windows: `irm ${staticBase}/install.ps1 | iex`,
  }
}

export function getCliDocsURL(_serverAddress?: string): string {
  return buildSitePathURL(APIMETER_DOCS_URL, '/zh/docs/apps/apimeter-cli')
}

export function getAgentToolsURL(_serverAddress?: string): string {
  return buildSitePathURL(APIMETER_DOCS_URL, '/zh/docs/apps')
}

export function getCliScreenshotURL(_serverAddress?: string): string {
  return buildSitePathURL(
    APIMETER_DOCS_URL,
    '/assets/docs/apps/apimeter/apimeter-cli.png'
  )
}
