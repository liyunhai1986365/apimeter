import type { BillingCenterSectionId } from './section-registry'

export type { BillingCenterSectionId }

export type BillingApiResponse<T> = {
  success: boolean
  message?: string
  data: T
}

export type BillingStatementStatus =
  | 'open'
  | 'pending'
  | 'confirmed'
  | 'disputed'
  | 'exception'
  | string

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
  base_settlement_amount: number
  difference_amount: number
  status: BillingStatementStatus
  reconciliation_status: 'matched' | 'exception' | string
  confirmation_status: 'pending' | 'confirmed' | 'disputed' | string
  revision: number
  confirmed_revision: number
  confirmed_at: number
  generated_at: number
  finalized_at: number
  exception_count: number
}

export type BillingStatementDispute = {
  id: number
  dispute_no: string
  statement_no: string
  user_id: number
  statement_revision: number
  reason_type: string
  description: string
  expected_amount: number
  has_expected_amount: boolean
  status: 'pending' | 'accepted' | 'rejected' | 'withdrawn' | string
  resolution: string
  operator_user_id: number
  operator_username: string
  created_at: number
  updated_at: number
  resolved_at: number
}

export type BillingStatementAdjustment = {
  id: number
  adjustment_no: string
  idempotency_key: string
  statement_no: string
  user_id: number
  statement_revision: number
  amount: number
  amount_before: number
  amount_after: number
  reason: string
  dispute_id: number
  operator_user_id: number
  operator_username: string
  balance_sync_status: 'pending' | 'synced' | 'failed' | string
  balance_sync_error: string
  created_at: number
  synced_at: number
}

export type BillingStatementEvent = {
  id: number
  statement_no: string
  user_id: number
  revision: number
  event_type: string
  actor_type: string
  actor_user_id: number
  actor_username: string
  detail: string
  created_at: number
}

export type BillingStatementWorkflowDetail = {
  statement: BillingStatement
  summaries: BillingStatementSummary[]
  disputes: BillingStatementDispute[]
  adjustments: BillingStatementAdjustment[]
  events: BillingStatementEvent[]
}

export type BillingAdminStatementItem = BillingStatement & {
  username: string
  display_name: string
  email: string
}

export type BillingAdminStatementPage = {
  items: BillingAdminStatementItem[]
  total: number
  limit: number
  offset: number
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
  group_ratio: number
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
  group_ratio: number
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
