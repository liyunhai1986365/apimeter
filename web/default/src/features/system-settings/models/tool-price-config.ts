export const DEFAULT_PRICES: Record<string, number> = {
  web_search: 10.0,
  web_fetch: 0,
  code_execution: 0,
  tool_search: 0,
  web_search_preview: 10.0,
  'web_search_preview:gpt-4o*': 25.0,
  'web_search_preview:gpt-4.1*': 25.0,
  'web_search_preview:gpt-4o-mini*': 25.0,
  'web_search_preview:gpt-4.1-mini*': 25.0,
  file_search: 2.5,
  google_search: 14.0,
  google_search_grounded_prompt: 0,
  'google_search:gemini-2.5*': 0,
  'google_search:gemini-2.0*': 0,
  'google_search:gemini-1.5*': 0,
  'google_search_grounded_prompt:gemini-2.5*': 35,
  'google_search_grounded_prompt:gemini-2.0*': 35,
  'google_search_grounded_prompt:gemini-1.5*': 35,
}

// Do not materialize Google's new defaults over saved rules: adding a
// grounded-prompt rule opts a model into the new two-counter pricing scheme.
export function parseInitialPrices(
  rawValue: string | undefined
): Record<string, number> {
  if (!rawValue) return { ...DEFAULT_PRICES }
  try {
    const parsed = JSON.parse(rawValue) as unknown
    if (
      parsed &&
      typeof parsed === 'object' &&
      !Array.isArray(parsed) &&
      Object.keys(parsed).length > 0
    ) {
      const saved = parsed as Record<string, number>
      const hasGoogleRules = Object.keys(saved).some(isGooglePriceKey)
      const defaults = Object.fromEntries(
        Object.entries(DEFAULT_PRICES).filter(
          ([key]) => !hasGoogleRules || !isGooglePriceKey(key)
        )
      )
      return { ...defaults, ...saved }
    }
  } catch {
    // Invalid saved JSON keeps the existing default editor behavior.
  }
  return { ...DEFAULT_PRICES }
}

function isGooglePriceKey(key: string): boolean {
  return (
    key === 'google_search' ||
    key.startsWith('google_search:') ||
    key === 'google_search_grounded_prompt' ||
    key.startsWith('google_search_grounded_prompt:')
  )
}
