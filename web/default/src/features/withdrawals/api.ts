import { api } from '@/lib/api'
import type {
  WithdrawalManagementPage,
  WithdrawalSource,
  WithdrawalStatus,
} from './types'

type ApiResponse<T> = {
  success: boolean
  message?: string
  data: T
}

export async function listAdminWithdrawals(input: {
  page: number
  pageSize: number
  source?: WithdrawalSource
  status?: WithdrawalStatus
}): Promise<WithdrawalManagementPage> {
  const response = await api.get<ApiResponse<WithdrawalManagementPage>>(
    '/api/withdrawals',
    {
      params: {
        p: input.page,
        page_size: input.pageSize,
        ...(input.source ? { source: input.source } : {}),
        ...(input.status ? { status: input.status } : {}),
      },
    }
  )
  return {
    ...response.data.data,
    items: Array.isArray(response.data.data.items)
      ? response.data.data.items
      : [],
  }
}

export async function completeAdminWithdrawal(input: {
  source: WithdrawalSource
  withdrawalId: number
  status: WithdrawalStatus
  adminRemark: string
}): Promise<void> {
  await api.put(
    `/api/withdrawals/${input.source}/${input.withdrawalId}/status`,
    {
      status: input.status,
      admin_remark: input.adminRemark.trim(),
    }
  )
}
