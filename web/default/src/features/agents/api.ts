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
  AdminAgentDomain,
  Agent,
  AgentBalance,
  AgentBranding,
  AgentDomain,
  AgentLedger,
  AgentPage,
  AgentPricingRule,
  AgentSelfPayload,
  AgentUser,
  AgentWithdrawal,
} from './types'

export async function getAgentSelf() {
  const res = await api.get<{ success: boolean; data: AgentSelfPayload }>(
    '/api/agent/self'
  )
  return res.data
}

export async function listAgentDomains(page = 1, pageSize = 20) {
  const res = await api.get<{ success: boolean; data: AgentPage<AgentDomain> }>(
    '/api/agent/domains',
    { params: { p: page, page_size: pageSize } }
  )
  return res.data
}

export async function createAgentDomain(input: { domain: string }) {
  const res = await api.post<{ success: boolean; data: AgentDomain }>(
    '/api/agent/domains',
    input
  )
  return res.data
}

export async function updateAgentBranding(input: { branding: string }) {
  const res = await api.put<{ success: boolean; data: Agent }>(
    '/api/agent/self/branding',
    input
  )
  return res.data
}

export async function createAdminAgent(input: {
  owner_user_id: number
  name: string
  slug: string
  status: number
  default_markup: number
  branding?: string
}) {
  const res = await api.post<{ success: boolean; data: Agent }>(
    '/api/agents/',
    {
      price_mode: 'multiplier',
      settlement_currency: 'USD',
      ...input,
    }
  )
  return res.data
}

export async function updateAdminAgent(input: {
  id: number
  owner_user_id: number
  name: string
  slug: string
  status: number
  default_markup: number
  branding?: string
}) {
  const res = await api.put<{ success: boolean; data: Agent }>(
    `/api/agents/${input.id}`,
    {
      price_mode: 'multiplier',
      settlement_currency: 'USD',
      owner_user_id: input.owner_user_id,
      name: input.name,
      slug: input.slug,
      status: input.status,
      default_markup: input.default_markup,
      branding: input.branding ?? '',
    }
  )
  return res.data
}

export function parseAgentBranding(branding?: string): AgentBranding {
  if (!branding?.trim()) return {}
  try {
    const parsed = JSON.parse(branding)
    if (!parsed || typeof parsed !== 'object') return {}
    return {
      site_name:
        typeof parsed.site_name === 'string' ? parsed.site_name : undefined,
      logo: typeof parsed.logo === 'string' ? parsed.logo : undefined,
    }
  } catch {
    return {}
  }
}

export function stringifyAgentBranding(input: AgentBranding) {
  const siteName = input.site_name?.trim() ?? ''
  const logo = input.logo?.trim() ?? ''
  if (!siteName && !logo) return ''
  return JSON.stringify({
    site_name: siteName,
    logo,
  })
}

export async function createAdminAgentDomain(input: {
  agentId: number
  domain: string
}) {
  const res = await api.post<{ success: boolean; data: AgentDomain }>(
    `/api/agents/${input.agentId}/domains`,
    { domain: input.domain }
  )
  return res.data
}

export async function bindAdminAgentUser(input: {
  agentId: number
  userId: number
}) {
  const res = await api.post<{ success: boolean; data: { user_id: number } }>(
    `/api/agents/${input.agentId}/users`,
    { user_id: input.userId, source: 'admin_bind' }
  )
  return res.data
}

export async function listAgentPricingRules(page = 1, pageSize = 20) {
  const res = await api.get<{
    success: boolean
    data: AgentPage<AgentPricingRule>
  }>('/api/agent/pricing_rules', { params: { p: page, page_size: pageSize } })
  return res.data
}

export async function upsertAgentPricingRule(input: {
  model_pattern: string
  markup: number
  enabled: boolean
}) {
  const res = await api.post<{ success: boolean; data: AgentPricingRule }>(
    '/api/agent/pricing_rules',
    input
  )
  return res.data
}

export async function listAgentUsers(page = 1, pageSize = 20) {
  const res = await api.get<{ success: boolean; data: AgentPage<AgentUser> }>(
    '/api/agent/users',
    { params: { p: page, page_size: pageSize } }
  )
  return res.data
}

export async function listAgentLedger(page = 1, pageSize = 20) {
  const res = await api.get<{ success: boolean; data: AgentPage<AgentLedger> }>(
    '/api/agent/ledger',
    { params: { p: page, page_size: pageSize } }
  )
  return res.data
}

export async function listAgentWithdrawals(page = 1, pageSize = 20) {
  const res = await api.get<{
    success: boolean
    data: AgentPage<AgentWithdrawal>
  }>('/api/agent/withdrawals', { params: { p: page, page_size: pageSize } })
  return res.data
}

export async function submitAgentWithdrawal(input: {
  amount_quota: number
  amount_money: number
  account_info: string
}) {
  const res = await api.post<{ success: boolean; data: AgentWithdrawal }>(
    '/api/agent/withdrawals',
    input
  )
  return res.data
}

export async function listAdminAgents(page = 1, pageSize = 20) {
  const res = await api.get<{ success: boolean; data: AgentPage<Agent> }>(
    '/api/agents/',
    { params: { p: page, page_size: pageSize } }
  )
  return res.data
}

export async function listAdminAgentDomains(
  status?: number,
  page = 1,
  pageSize = 20
) {
  const res = await api.get<{
    success: boolean
    data: AgentPage<AdminAgentDomain>
  }>('/api/agents/domains', {
    params: {
      p: page,
      page_size: pageSize,
      ...(status === undefined ? {} : { status }),
    },
  })
  return res.data
}

export async function listAdminAgentDomainsByAgent(
  agentId: number,
  page = 1,
  pageSize = 20
) {
  const res = await api.get<{ success: boolean; data: AgentPage<AgentDomain> }>(
    `/api/agents/${agentId}/domains`,
    { params: { p: page, page_size: pageSize } }
  )
  return res.data
}

export async function updateAdminAgentDomainStatus(input: {
  agentId: number
  domainId: number
  status: number
}) {
  const res = await api.put<{ success: boolean; data: { id: number; status: number } }>(
    `/api/agents/${input.agentId}/domains/${input.domainId}/status`,
    { status: input.status }
  )
  return res.data
}

export async function listAdminAgentUsers(
  agentId: number,
  page = 1,
  pageSize = 20
) {
  const res = await api.get<{ success: boolean; data: AgentPage<AgentUser> }>(
    `/api/agents/${agentId}/users`,
    { params: { p: page, page_size: pageSize } }
  )
  return res.data
}

export async function listAdminAgentPricingRules(
  agentId: number,
  page = 1,
  pageSize = 20
) {
  const res = await api.get<{
    success: boolean
    data: AgentPage<AgentPricingRule>
  }>(`/api/agents/${agentId}/pricing_rules`, {
    params: { p: page, page_size: pageSize },
  })
  return res.data
}

export async function upsertAdminAgentPricingRule(input: {
  agentId: number
  model_pattern: string
  markup: number
  enabled: boolean
}) {
  const res = await api.post<{ success: boolean; data: AgentPricingRule }>(
    `/api/agents/${input.agentId}/pricing_rules`,
    {
      model_pattern: input.model_pattern,
      markup: input.markup,
      enabled: input.enabled,
    }
  )
  return res.data
}

export async function listAdminAgentWithdrawals(
  agentId?: number,
  page = 1,
  pageSize = 20
) {
  const res = await api.get<{
    success: boolean
    data: AgentPage<AgentWithdrawal>
  }>('/api/agents/withdrawals', {
    params: {
      p: page,
      page_size: pageSize,
      ...(agentId == null ? {} : { agent_id: agentId }),
    },
  })
  return res.data
}

export async function completeAdminAgentWithdrawal(input: {
  withdrawalId: number
  status: string
  admin_remark?: string
}) {
  const res = await api.put<{
    success: boolean
    data: { id: number; status: string }
  }>(`/api/agents/withdrawals/${input.withdrawalId}/status`, {
    status: input.status,
    admin_remark: input.admin_remark ?? '',
  })
  return res.data
}

export type { Agent, AgentBalance }
