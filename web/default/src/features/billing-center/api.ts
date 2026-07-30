import { api } from '@/lib/api'
import type {
  BillingApiResponse,
  BillingBreakdownParams,
  BillingBreakdownRow,
  BillingStatement,
  BillingStatementParams,
  BillingStatementSummary,
} from './types'

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

export async function generateBillingMonthlyStatement(month: string): Promise<
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
