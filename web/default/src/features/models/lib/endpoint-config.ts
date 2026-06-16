export type EndpointConfigRow = {
  type: string
  path: string
  method: string
  label: string
  docs_url: string
}

const DEFAULT_METHOD = 'POST'

function normalizeMethod(method: unknown): string {
  const value = typeof method === 'string' ? method.trim().toUpperCase() : ''
  return value || DEFAULT_METHOD
}

function normalizeUrl(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function normalizeRow(type: string, value: unknown): EndpointConfigRow | null {
  const endpointType = type.trim()
  if (!endpointType) return null

  if (typeof value === 'string') {
    return {
      type: endpointType,
      path: value.trim(),
      method: DEFAULT_METHOD,
      label: '',
      docs_url: '',
    }
  }

  if (typeof value === 'boolean') {
    if (!value) return null
    return {
      type: endpointType,
      path: '',
      method: DEFAULT_METHOD,
      label: '',
      docs_url: '',
    }
  }

  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return null
  }

  const record = value as Record<string, unknown>
  return {
    type: endpointType,
    path: normalizeUrl(record.path),
    method: normalizeMethod(record.method),
    label: normalizeUrl(record.label),
    docs_url: normalizeUrl(record.docs_url ?? record.docsUrl),
  }
}

export function normalizeEndpointConfig(raw: string): EndpointConfigRow[] {
  const trimmed = raw.trim()
  if (!trimmed) return []

  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch {
    return []
  }

  if (Array.isArray(parsed)) {
    return parsed
      .map((item) => {
        if (typeof item === 'string') {
          return normalizeRow(item, true)
        }
        if (item && typeof item === 'object') {
          const record = item as Record<string, unknown>
          const type = typeof record.type === 'string' ? record.type : ''
          return normalizeRow(type, record)
        }
        return null
      })
      .filter((row): row is EndpointConfigRow => row !== null)
  }

  if (!parsed || typeof parsed !== 'object') return []

  return Object.entries(parsed as Record<string, unknown>)
    .map(([type, value]) => normalizeRow(type, value))
    .filter((row): row is EndpointConfigRow => row !== null)
}

export function serializeEndpointConfig(rows: EndpointConfigRow[]): string {
  const payload: Record<string, unknown> = {}

  for (const row of rows) {
    const type = row.type.trim()
    if (!type) continue

    payload[type] = {
      path: row.path.trim(),
      method: normalizeMethod(row.method),
      ...(row.label.trim() ? { label: row.label.trim() } : {}),
      ...(row.docs_url.trim() ? { docs_url: row.docs_url.trim() } : {}),
    }
  }

  if (Object.keys(payload).length === 0) return ''
  return JSON.stringify(payload, null, 2)
}
