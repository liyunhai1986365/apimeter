import type { BillingCenterSectionId } from './section-registry'

export type { BillingCenterSectionId }

export type BillingApiResponse<T> = {
  success: boolean
  message?: string
  data: T
}

export type BillingSource = 'wallet' | 'subscription' | string
export type BillingStatementStatus = 'open' | 'confirmed' | 'exception' | string
export type AccountLedgerEntryType =
  | 'topup'
  | 'consume'
  | 'refund'
  | 'adjustment'
  | string

export type BillingCurrentPeriodSummary = {
  request_count: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  original_amount: number
  discount_amount: number
  settlement_amount: number
}

export type BillingCurrentPeriod = {
  month: string
  status: string
  summary: BillingCurrentPeriodSummary
}

export type AccountLedgerEntry = {
  id: number
  user_id: number
  account_type: BillingSource
  entry_type: AccountLedgerEntryType
  amount: number
  balance_before: number
  balance_after: number
  source_type: string
  source_id: string
  log_id: number
  request_id: string
  model_name: string
  token_id: number
  token_name: string
  occurred_at: number
  ledger_date: string
  ledger_month: string
  content: string
}

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
  dimension: 'model' | 'group' | 'key' | 'source' | string
  dimension_value: string
  request_count: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  original_amount: number
  discount_amount: number
  settlement_amount: number
}

export type DailyBillingReconciliation = {
  date: string
  usage_settlement_amount: number
  account_consume_amount: number
  difference_amount: number
  request_count: number
  status: BillingStatementStatus
}

export type AccountLedgerParams = {
  month?: string
  start_time?: number
  end_time?: number
  entry_type?: string
  limit?: number
  offset?: number
}

export type BillingStatementParams = {
  start_month?: string
  end_month?: string
  period?: string
  limit?: number
  offset?: number
}

export type BillingBackfillResult = {
  scanned: number
  usage_created: number
  ledger_created: number
  skipped: number
  failed: number
}
