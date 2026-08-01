export type WithdrawalSource = 'user' | 'agent'
export type WithdrawalStatus =
  | 'pending'
  | 'approved'
  | 'paid'
  | 'rejected'
  | 'cancelled'

export type WithdrawalManagementItem = {
  id: number
  source: WithdrawalSource
  applicant_id: number
  applicant_name: string
  owner_user_id: number
  amount_quota: number
  amount_money: number
  currency: string
  fee: number
  status: WithdrawalStatus
  account_info: string
  admin_remark: string
  created_at: number
  processed_at: number
}

export type WithdrawalManagementPage = {
  page: number
  page_size: number
  total: number
  items: WithdrawalManagementItem[]
}
