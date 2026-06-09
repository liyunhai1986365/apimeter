export function getDataTableCellClassName(columnId: string) {
  if (columnId !== 'actions') return undefined
  return 'bg-card border-l border-border sticky right-0 z-10 shadow-sm'
}
