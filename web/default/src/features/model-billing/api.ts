import { api } from '@/lib/api'
import type {
  ModelBillingBackfillResult,
  ModelBillingSummaryParams,
  ModelBillingSummaryRow,
} from './types'

type ModelBillingSummaryResponse = {
  success: boolean
  message?: string
  data: ModelBillingSummaryRow[]
}

export async function getModelBillingSummary(
  params: ModelBillingSummaryParams
): Promise<ModelBillingSummaryResponse> {
  const res = await api.get<ModelBillingSummaryResponse>(
    '/api/billing/models/summary',
    { params }
  )
  return res.data
}

type ModelBillingBackfillResponse = {
  success: boolean
  message?: string
  data: ModelBillingBackfillResult
}

export async function backfillModelBilling(
  params: Pick<
    ModelBillingSummaryParams,
    'start_timestamp' | 'end_timestamp'
  > = {}
): Promise<ModelBillingBackfillResponse> {
  const res = await api.post<ModelBillingBackfillResponse>(
    '/api/billing/admin/models/backfill',
    null,
    { params }
  )
  return res.data
}
