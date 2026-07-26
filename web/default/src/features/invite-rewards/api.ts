import { api } from '@/lib/api'
import type {
  AffiliateInvitePage,
  AffiliateWithdrawal,
  AffiliateWithdrawalPage,
  ApiResponse,
} from './types'

function requireApiData<T>(response: ApiResponse<T>, fallback: string): T {
  if (!response.success || response.data === undefined) {
    throw new Error(response.message || fallback)
  }
  return response.data
}

export async function getAffiliateInvites(
  page: number,
  pageSize: number
): Promise<AffiliateInvitePage> {
  const response = await api.get<ApiResponse<AffiliateInvitePage>>(
    '/api/user/aff/invites',
    { params: { p: page, page_size: pageSize } }
  )
  const data = requireApiData(response.data, 'Failed to load invite records')
  return {
    ...data,
    items: Array.isArray(data.items) ? data.items : [],
  }
}

export async function transferAffiliateRewards(quota: number): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    '/api/user/aff_transfer',
    { quota },
    { skipBusinessError: true } as Record<string, unknown>
  )
  requireApiData(response.data, 'Transfer failed')
}

export async function getAffiliateWithdrawals(
  page: number,
  pageSize: number
): Promise<AffiliateWithdrawalPage> {
  const response = await api.get<ApiResponse<AffiliateWithdrawalPage>>(
    '/api/user/aff/withdrawals',
    { params: { p: page, page_size: pageSize } }
  )
  const data = requireApiData(
    response.data,
    'Failed to load withdrawal records'
  )
  return {
    ...data,
    items: Array.isArray(data.items) ? data.items : [],
  }
}

export async function submitAffiliateWithdrawal(input: {
  amount_quota: number
  account_info: string
}): Promise<AffiliateWithdrawal> {
  const response = await api.post<ApiResponse<AffiliateWithdrawal>>(
    '/api/user/aff/withdrawals',
    input,
    { skipBusinessError: true } as Record<string, unknown>
  )
  return requireApiData(response.data, 'Failed to submit withdrawal')
}

export async function cancelAffiliateWithdrawal(id: number): Promise<void> {
  const response = await api.delete<ApiResponse<unknown>>(
    `/api/user/aff/withdrawals/${id}`,
    { skipBusinessError: true } as Record<string, unknown>
  )
  requireApiData(response.data, 'Failed to cancel withdrawal')
}
