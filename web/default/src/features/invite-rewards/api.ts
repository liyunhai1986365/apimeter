import { api } from '@/lib/api'
import type { AffiliateInvitePage, ApiResponse } from './types'

export async function getAffiliateInvites(
  page: number,
  pageSize: number
): Promise<AffiliateInvitePage> {
  const response = await api.get<ApiResponse<AffiliateInvitePage>>(
    '/api/user/aff/invites',
    { params: { p: page, page_size: pageSize } }
  )
  if (!response.data.success || !response.data.data) {
    throw new Error(response.data.message || 'Failed to load invite records')
  }
  return {
    ...response.data.data,
    items: Array.isArray(response.data.data.items)
      ? response.data.data.items
      : [],
  }
}
