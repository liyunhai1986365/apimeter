export const DEFAULT_PRICES = {
  web_search: 10.0,
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
};

export function parseInitialPrices(raw) {
  try {
    const saved = typeof raw === 'string' ? JSON.parse(raw) : raw;
    if (
      saved &&
      typeof saved === 'object' &&
      !Array.isArray(saved) &&
      Object.keys(saved).length
    ) {
      // A saved google_search rule without a grounded-prompt rule is billed
      // per request. Merging in defaults would change that existing contract.
      return saved;
    }
  } catch {
    // Invalid or empty settings use the same defaults as the reset button.
  }
  return { ...DEFAULT_PRICES };
}
