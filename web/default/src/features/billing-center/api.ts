import { api } from '@/lib/api'
import type {
  BillingApiResponse,
  BillingBreakdownParams,
  BillingBreakdownRow,
  BillingStatement,
  BillingStatementDispute,
  BillingStatementParams,
  BillingStatementSummary,
  BillingStatementWorkflowDetail,
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

export async function getBillingMonthlyStatementSummaries(
  statementNo: string
): Promise<BillingApiResponse<BillingStatementSummary[]>> {
  const res = await api.get<BillingApiResponse<BillingStatementSummary[]>>(
    `/api/billing/monthly-statements/${encodeURIComponent(statementNo)}/summaries`
  )
  return res.data
}

export async function getBillingStatementWorkflow(
  statementNo: string
): Promise<BillingApiResponse<BillingStatementWorkflowDetail>> {
  const res = await api.get<BillingApiResponse<BillingStatementWorkflowDetail>>(
    `/api/billing/monthly-statements/${encodeURIComponent(statementNo)}/workflow`
  )
  return res.data
}

export async function confirmBillingStatement(
  statementNo: string,
  revision: number
): Promise<BillingApiResponse<BillingStatement>> {
  const res = await api.post<BillingApiResponse<BillingStatement>>(
    `/api/billing/monthly-statements/${encodeURIComponent(statementNo)}/confirm`,
    { revision }
  )
  return res.data
}

export async function createBillingStatementDispute(
  statementNo: string,
  payload: {
    revision: number
    reason_type: string
    description: string
    expected_amount: number
    has_expected_amount: boolean
  }
): Promise<BillingApiResponse<BillingStatementDispute>> {
  const res = await api.post<BillingApiResponse<BillingStatementDispute>>(
    `/api/billing/monthly-statements/${encodeURIComponent(statementNo)}/disputes`,
    payload
  )
  return res.data
}
