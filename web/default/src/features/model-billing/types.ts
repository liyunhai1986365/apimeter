export type ModelBillingPeriod = 'day' | 'month'

export type ModelBillingSummaryRow = {
  period: string
  model_name: string
  request_count: number
  input_tokens: number
  output_tokens: number
  cache_write_tokens: number
  cache_read_tokens: number
  original_quota: number
  discount_quota: number
  payable_quota: number
}

export type ModelBillingSummaryParams = {
  period: ModelBillingPeriod
  start_timestamp?: number
  end_timestamp?: number
  model_name?: string
  group?: string
  billing_source?: string
}

export type ModelBillingBackfillResult = {
  scanned: number
  created: number
  skipped: number
  failed: number
}
