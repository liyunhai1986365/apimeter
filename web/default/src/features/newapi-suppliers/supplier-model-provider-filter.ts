export type SupplierModelProviderFilter = {
  provider: string
  models: string[]
}

export function buildSupplierModelProviderFilters(
  models: string[],
  providersByModel?: Record<string, string[]>
): SupplierModelProviderFilter[] {
  const modelSet = new Set(models)
  const providerModels = new Map<string, Set<string>>()

  for (const model of models) {
    const providers = providersByModel?.[model] ?? []
    for (const provider of providers) {
      const trimmedProvider = provider.trim()
      if (!trimmedProvider) continue
      if (!providerModels.has(trimmedProvider)) {
        providerModels.set(trimmedProvider, new Set())
      }
      providerModels.get(trimmedProvider)?.add(model)
    }
  }

  return [...providerModels.entries()]
    .map(([provider, providerModelSet]) => ({
      provider,
      models: [...providerModelSet].filter((model) => modelSet.has(model)).sort(),
    }))
    .filter((item) => item.models.length > 0)
    .sort((a, b) => a.provider.localeCompare(b.provider))
}

export function selectSupplierProviderModels(
  allModels: string[],
  providerModels: string[]
): Record<string, boolean> {
  const selected = new Set(providerModels)
  return Object.fromEntries(
    allModels.map((model) => [model, selected.has(model)])
  )
}
