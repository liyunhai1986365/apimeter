export const ARK_ASSET_DEFAULTS = {
  asset_endpoint_path: '/api',
  asset_api_version: '2024-01-01',
  asset_fixed_query: '{}',
  asset_pagination: 'page' as const,
}

const reservedKeys = new Set([
  'action',
  'version',
  'projectname',
  'project_name',
  'authorization',
  'key',
  'api_key',
  'apikey',
  'token',
  'access_token',
])

export function parseAssetFixedQuery(raw: string): Record<string, string> {
  const value: unknown = JSON.parse(raw.trim() || '{}')
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error('Invalid fixed query')
  }
  const entries = Object.entries(value)
  if (
    entries.length > 32 ||
    entries.some(
      ([key, item]) =>
        key.length > 64 ||
        !/^[A-Za-z][A-Za-z0-9_-]*$/.test(key) ||
        reservedKeys.has(key.toLowerCase()) ||
        typeof item !== 'string' ||
        item.length > 2048 ||
        ['\0', '\r', '\n'].some((character) => item.includes(character))
    )
  ) {
    throw new Error('Invalid fixed query')
  }
  return Object.fromEntries(entries) as Record<string, string>
}

export function validAssetEndpointPath(path: string): boolean {
  return (
    path.length <= 512 &&
    /^\/[A-Za-z0-9/_-]*$/.test(path) &&
    !path.includes('//')
  )
}
