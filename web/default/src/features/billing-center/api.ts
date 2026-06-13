import { api } from '@/lib/api'
import type {
  AccountLedgerEntry,
  AccountLedgerParams,
  BillingApiResponse,
  BillingBackfillResult,
  BillingBreakdownParams,
  BillingBreakdownRow,
  BillingStatement,
  BillingStatementParams,
  BillingStatementSummary,
  DailyBillingReconciliation,
} from './types'

export async function getAccountLedgerEntries(
  params: AccountLedgerParams = {}
): Promise<BillingApiResponse<AccountLedgerEntry[]>> {
  const res = await api.get<BillingApiResponse<AccountLedgerEntry[]>>(
    '/api/billing/account-ledger',
    { params }
  )
  return res.data
}

export async function getBillingMonthlyStatements(
  params: BillingStatementParams = {}
): Promise<BillingApiResponse<BillingStatement[]>> {
  const res = await api.get<BillingApiResponse<BillingStatement[]>>(
    '/api/billing/monthly-statements',
    { params: { period: 'month', ...params } }
  )
  return res.data
}

export async function getBillingBreakdowns(
  params: BillingBreakdownParams = {}
): Promise<BillingApiResponse<BillingBreakdownRow[]>> {
  const res = await api.get<BillingApiResponse<BillingBreakdownRow[]>>(
    '/api/billing/breakdowns',
    { params }
  )
  return res.data
}

export async function exportBillingBreakdowns(
  params: BillingBreakdownParams = {}
): Promise<Blob> {
  const res = await api.get<Blob>('/api/billing/breakdowns/export', {
    params,
    responseType: 'blob',
    disableDuplicate: true,
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

export async function exportBillingMonthlyStatement(
  statementNo: string
): Promise<Blob> {
  const res = await api.get<Blob>(
    `/api/billing/monthly-statements/${encodeURIComponent(statementNo)}/export`,
    {
      responseType: 'blob',
      disableDuplicate: true,
      skipBusinessError: true,
    } as Record<string, unknown>
  )
  return res.data
}

export async function generateBillingMonthlyStatement(
  month: string
): Promise<
  BillingApiResponse<{
    statement: BillingStatement
    summaries: BillingStatementSummary[]
  }>
> {
  const res = await api.post<
    BillingApiResponse<{
      statement: BillingStatement
      summaries: BillingStatementSummary[]
    }>
  >('/api/billing/monthly-statements/generate', null, { params: { month } })
  return res.data
}

export async function getBillingMonthlyStatementSummaries(
  statementNo: string
): Promise<BillingApiResponse<BillingStatementSummary[]>> {
  const res = await api.get<BillingApiResponse<BillingStatementSummary[]>>(
    `/api/billing/monthly-statements/${encodeURIComponent(statementNo)}/summaries`
  )
  return res.data
}

export async function getDailyBillingReconciliations(
  params: BillingStatementParams = {}
): Promise<BillingApiResponse<DailyBillingReconciliation[]>> {
  const res = await api.get<BillingApiResponse<DailyBillingReconciliation[]>>(
    '/api/billing/daily-reconciliations',
    { params }
  )
  return res.data
}

export async function backfillBillingV2(
  params: {
    start_time?: number
    end_time?: number
    batch_size?: number
  } = {}
): Promise<BillingApiResponse<BillingBackfillResult>> {
  const res = await api.post<BillingApiResponse<BillingBackfillResult>>(
    '/api/billing/admin/backfill-v2',
    null,
    { params }
  )
  return res.data
}
