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
export type Agent = {
  id: number
  owner_user_id: number
  name: string
  slug: string
  status: number
  price_mode: string
  default_markup: number
  settlement_currency: string
  branding: string
  created_at: number
  updated_at: number
  balance?: AgentBalance
}

export type AgentBranding = {
  site_name?: string
  logo?: string
  home_page_content?: string
  header_nav_modules?: string
  site_style?: string
}

export type AgentBalance = {
  profit_quota: number
  pending_withdrawal_quota: number
  approved_withdrawal_quota: number
  available_quota: number
  currency: string
  profit_amount: number
  pending_withdrawal_amount: number
  approved_withdrawal_amount: number
  available_amount: number
}

export type AgentSelfPayload = {
  agent: Agent
  balance: AgentBalance
  context?: {
    AgentID: number
    Domain: string
    OwnerUserID: number
    DefaultMarkup: number
    Branding: string
    GroupRatios?: Record<string, number>
  }
  view_context: AgentViewContext
}

export type AgentViewContext = {
  mode: 'own' | 'admin' | 'none'
  agent_id: number
  own_agent_id: number
  can_switch: boolean
}

export type AgentAnalyticsSummary = {
  total_users: number
  active_users: number
  new_users: number
  usage_users: number
  total_requests: number
  successful_requests: number
  failed_requests: number
  total_tokens: number
  total_quota: number
  profit_quota: number
  profit_amount: number
  currency: string
  success_rate: number
}

export type AgentAnalyticsTrendPoint = {
  timestamp: number
  requests: number
  errors: number
  tokens: number
  quota: number
}

export type AgentAnalyticsModelItem = {
  model_name: string
  requests: number
  tokens: number
  quota: number
}

export type AgentAnalyticsUserItem = {
  user_id: number
  username: string
  requests: number
  errors: number
  tokens: number
  quota: number
}

export type AgentAnalyticsLogItem = {
  id: number
  user_id: number
  username: string
  created_at: number
  type: number
  token_name: string
  model_name: string
  quota: number
  prompt_tokens: number
  completion_tokens: number
  use_time: number
  is_stream: boolean
  group: string
  request_id?: string
}

export type AgentAnalytics = {
  start_timestamp: number
  end_timestamp: number
  bucket_seconds: number
  summary: AgentAnalyticsSummary
  trend: AgentAnalyticsTrendPoint[]
  top_models: AgentAnalyticsModelItem[]
  top_users: AgentAnalyticsUserItem[]
  recent_logs: AgentAnalyticsLogItem[]
}

export type AgentPage<T> = {
  page: number
  page_size: number
  total: number
  items: T[]
}

export type AgentDomain = {
  id: number
  agent_id: number
  domain: string
  status: number
  verify_token: string
  cname_target: string
  verified_at: number
  created_at: number
  updated_at: number
}

export type AdminAgentDomain = AgentDomain & {
  agent_name: string
  agent_slug: string
  owner_user_id: number
}

export type AgentGroupRatio = {
  group_name: string
  system_group_name: string
  description?: string
  agent_ratio?: number
  system_ratio: number
  configured_ratio: number
  effective_ratio: number
  configured: boolean
  visible: boolean
  available: boolean
}

export type AgentUserGroupConfig = {
  group_name: string
  visible_groups?: string[]
  group_ratios?: Record<string, number>
}

export type AgentUser = {
  id: number
  agent_id: number
  user_id: number
  source: string
  agent_user_status: number
  agent_user_created_at: number
  username: string
  display_name: string
  email: string
  role: number
  status: number
  group: string
  quota: number
  used_quota: number
  request_count: number
  aff_count?: number
  aff_quota?: number
  aff_history_quota?: number
  inviter_id?: number
  remark?: string
  created_at: number
  last_login_at: number
}

export type AgentLedger = {
  id: number
  agent_id: number
  user_id: number
  log_id: number
  operator_user_id: number
  type: string
  base_quota: number
  charged_quota: number
  profit_quota: number
  balance_after: number
  currency: string
  base_amount: number
  charged_amount: number
  profit_amount: number
  balance_after_amount: number
  remark: string
  created_at: number
}

export type AgentWithdrawal = {
  id: number
  agent_id: number
  amount_quota: number
  amount_money: number
  settlement_amount: number
  currency: string
  fee: number
  status: string
  account_info: string
  admin_remark: string
  created_at: number
  processed_at: number
}
