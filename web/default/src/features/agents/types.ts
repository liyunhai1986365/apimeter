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
}

export type AgentBranding = {
  site_name?: string
  logo?: string
}

export type AgentBalance = {
  profit_quota: number
  pending_withdrawal_quota: number
  approved_withdrawal_quota: number
  available_quota: number
}

export type AgentSelfPayload = {
  agent: Agent
  balance: AgentBalance
  context: {
    AgentID: number
    Domain: string
    OwnerUserID: number
    DefaultMarkup: number
    Branding: string
  }
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

export type AgentPricingRule = {
  id: number
  agent_id: number
  model_pattern: string
  markup: number
  enabled: boolean
}

export type AgentUser = {
  id: number
  agent_id: number
  user_id: number
  source: string
  status: number
  created_at: number
}

export type AgentLedger = {
  id: number
  agent_id: number
  user_id: number
  log_id: number
  type: string
  base_quota: number
  charged_quota: number
  profit_quota: number
  balance_after: number
  created_at: number
}

export type AgentWithdrawal = {
  id: number
  agent_id: number
  amount_quota: number
  amount_money: number
  fee: number
  status: string
  account_info: string
  admin_remark: string
  created_at: number
  processed_at: number
}
