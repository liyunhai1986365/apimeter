import type { BillingCenterSectionId } from './section-registry'

export type { BillingCenterSectionId }

export type BillingApiResponse<T> = {
  success: boolean
  message?: string
  data: T
}

export type BillingStatementStatus = 'open' | 'confirmed' | 'exception' | string

export type BillingStatement = {
  id: number
  statement_no: string
  user_id: number
  period: 'day' | 'month' | string
  period_value: string
  period_start: number
  period_end: number
  opening_balance: number
  closing_balance: number
  topup_amount: number
  consume_amount: number
  refund_amount: number
  adjustment_amount: number
  request_count: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  original_amount: number
  discount_amount: number
  settlement_amount: number
  difference_amount: number
  status: BillingStatementStatus
  generated_at: number
  finalized_at: number
  exception_count: number
}

export type BillingStatementSummary = {
  id: number
  statement_no: string
  user_id: number
  period: string
  period_value: string
  dimension: 'month_model_group' | string
  dimension_value: string
  model_name: string
  group: string
  billing_source: string
  billing_mode: string
  request_count: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  original_amount: number
  discount_amount: number
  settlement_amount: number
}

export type BillingBreakdownRow = {
  period: 'day' | 'month' | string
  period_value: string
  model_name: string
  group: string
  billing_source: string
  billing_mode: string
  request_count: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  original_amount: number
  discount_amount: number
  settlement_amount: number
}

export type BillingStatementParams = {
  start_month?: string
  end_month?: string
  period?: string
  limit?: number
  offset?: number
}

export type BillingBreakdownParams = {
  period?: 'day' | 'month' | string
  start_date?: string
  end_date?: string
  month?: string
  model_name?: string
  group?: string
  billing_source?: string
  billing_mode?: string
  limit?: number
  offset?: number
}
