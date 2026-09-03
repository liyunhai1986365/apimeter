/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { api } from '@/lib/api'
import type {
  BillingAdminStatementPage,
  BillingApiResponse,
  BillingStatement,
  BillingStatementAdjustment,
  BillingStatementDispute,
  BillingStatementSummary,
  BillingStatementWorkflowDetail,
} from '@/features/billing-center/types'

export type BillingAdminQuery = {
  user_id?: number
  username?: string
  month?: string
  confirmation_status?: string
  limit?: number
  offset?: number
}

export async function getAdminBillingStatements(params: BillingAdminQuery) {
  const response = await api.get<BillingApiResponse<BillingAdminStatementPage>>(
    '/api/billing/admin/monthly-statements',
    { params }
  )
  return response.data
}

export async function getAdminBillingStatement(statementNo: string) {
  const response = await api.get<
    BillingApiResponse<BillingStatementWorkflowDetail>
  >(`/api/billing/admin/monthly-statements/${encodeURIComponent(statementNo)}`)
  return response.data
}

export async function generateAdminBillingStatement(
  userId: number,
  month: string
) {
  const response = await api.post<
    BillingApiResponse<{
      statement: BillingStatement
      summaries: BillingStatementSummary[]
    }>
  >('/api/billing/admin/monthly-statements/generate', undefined, {
    params: { user_id: userId, month },
  })
  return response.data
}

export async function adjustAdminBillingStatement(
  statementNo: string,
  payload: {
    amount: number
    reason: string
    dispute_id: number
    idempotency_key: string
  }
) {
  const response = await api.post<
    BillingApiResponse<{
      adjustment: BillingStatementAdjustment
      statement: BillingStatement
    }>
  >(
    `/api/billing/admin/monthly-statements/${encodeURIComponent(statementNo)}/adjustments`,
    payload
  )
  return response.data
}

export async function retryAdminBillingAdjustment(adjustmentNo: string) {
  const response = await api.post<
    BillingApiResponse<{ adjustment_no: string }>
  >(`/api/billing/admin/adjustments/${encodeURIComponent(adjustmentNo)}/retry`)
  return response.data
}

export async function resolveAdminBillingDispute(
  disputeId: number,
  action: 'accept' | 'reject',
  resolution: string
) {
  const response = await api.post<BillingApiResponse<BillingStatementDispute>>(
    `/api/billing/admin/disputes/${disputeId}/resolve`,
    {
      action,
      resolution,
    }
  )
  return response.data
}
