export interface AffiliateInviteRecord {
  invitee_id: number
  username: string
  display_name: string
  created_at: number
  completed_reward_quota: number
  pending_reward_quota: number
  reward_count: number
}

export interface AffiliateInviteStats {
  invite_count: number
  available_reward_quota: number
  registration_reward_quota: number
  completed_topup_reward_quota: number
  pending_topup_reward_quota: number
  total_reward_quota: number
}

export interface AffiliateInvitePage {
  items: AffiliateInviteRecord[]
  total: number
  page: number
  page_size: number
  stats: AffiliateInviteStats
}

export interface ApiResponse<T> {
  success: boolean
  message?: string
  data?: T
}
