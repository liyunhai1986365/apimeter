type EndpointTemplateValue = {
  path: string
  method: string
  label?: string
  docs_url?: string
  config?: Record<string, unknown>
}

export function appendEndpointTemplate(
  currentEndpoints: string,
  templateKey: string,
  template: EndpointTemplateValue
): string {
  const trimmed = currentEndpoints.trim()
  if (!trimmed) {
    return JSON.stringify({ [templateKey]: template }, null, 2)
  }

  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch {
    return currentEndpoints
  }

  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return currentEndpoints
  }

  return JSON.stringify(
    {
      ...(parsed as Record<string, unknown>),
      [templateKey]: template,
    },
    null,
    2
  )
}
