export function normalizeSortOrderInput(value: string): number | null {
  const trimmed = value.trim()
  if (!trimmed) return null

  const parsed = Number(trimmed)
  if (!Number.isFinite(parsed) || parsed < 0) return null

  return Math.floor(parsed)
}
