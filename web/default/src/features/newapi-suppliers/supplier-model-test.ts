export type SupplierModelTestResult = {
  model: string
  success: boolean
}

export function applySupplierModelTestResults(
  selectedModels: Record<string, boolean>,
  results: SupplierModelTestResult[]
): Record<string, boolean> {
  const next = { ...selectedModels }
  for (const result of results) {
    next[result.model] = result.success
  }
  return next
}
