export const USER_FACING_GROUP_TERMS = {
  single: 'Supplier',
  plural: 'Suppliers',
  all: 'All Suppliers',
  modelGroup: 'Model Supplier',
  choose: 'Choose Supplier',
  search: 'Search suppliers...',
  selectOne: 'Select a supplier',
  selectAtLeastOne: 'Select at least one supplier',
  noneFound: 'No supplier found.',
  add: 'Add supplier',
  callOrder: 'Supplier call order',
  crossRetry: 'Cross-supplier retry',
  crossRetryShort: 'Cross-supplier',
  autoLabel: 'auto automatic supplier',
  autoDescription:
    'Automatically selects the best available supplier with circuit breaker mechanism',
  requestOrderDescription:
    'Requests use the suppliers in this order. auto keeps the current automatic supplier selection.',
  crossRetryDescription:
    'When enabled, if channels in the current supplier fail, it will try channels in the next supplier in order.',
  perPerformance: 'Per-supplier performance',
  pricingBy: 'Pricing by Supplier',
} as const
