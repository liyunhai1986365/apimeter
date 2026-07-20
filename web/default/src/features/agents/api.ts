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
  AgentAnalytics,
  AgentAnalyticsLogItem,
  AgentBalance,
  AgentBranding,
  AgentDomain,
  AgentGroupRatio,
  AgentUserGroupConfig,
  AgentLedger,
  AgentPage,
  AgentSelfPayload,
  AgentViewContext,
  AgentUser,
  AgentWithdrawal,
} from './types'

export function buildAgentUserGroupOptions(userGroups: AgentUserGroupConfig[]) {
  return userGroups
    .map((item) => item.group_name.trim())
    .filter(
      (groupName, index, groups) =>
        groupName && groups.indexOf(groupName) === index
    )
    .sort((a, b) => a.localeCompare(b))
}

export async function getAgentSelf() {
  const res = await api.get<{ success: boolean; data: AgentSelfPayload }>(
    '/api/agent/self'
  )
  return res.data
}

export async function switchAgentViewContext(agentId: number) {
  const res = await api.post<{ success: boolean; data: AgentViewContext }>(
    '/api/agent/view-context',
    { agent_id: agentId }
  )
  return res.data
}

export async function clearAgentViewContext() {
  const res = await api.delete<{ success: boolean; data: AgentViewContext }>(
    '/api/agent/view-context'
  )
  return res.data
}

export async function getAgentAnalytics(params: {
  start_timestamp: number
  end_timestamp: number
}) {
  const res = await api.get<{ success: boolean; data: AgentAnalytics }>(
    '/api/agent/analytics',
    { params }
  )
  return res.data
}

export async function listAgentAnalyticsLogs(params: {
  p?: number
  page_size?: number
  start_timestamp: number
  end_timestamp: number
  type?: number
  username?: string
  model_name?: string
  request_id?: string
}) {
  const res = await api.get<{
    success: boolean
    data: AgentPage<AgentAnalyticsLogItem>
  }>('/api/agent/analytics/logs', { params })
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

export async function verifyAgentDomain(input: { id: number }) {
  const res = await api.post<{ success: boolean; data: AgentDomain }>(
    `/api/agent/domains/${input.id}/verify`
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
      home_page_content:
        typeof parsed.home_page_content === 'string'
          ? parsed.home_page_content
          : undefined,
      header_nav_modules:
        typeof parsed.header_nav_modules === 'string'
          ? parsed.header_nav_modules
          : undefined,
      site_style:
        typeof parsed.site_style === 'string' ? parsed.site_style : undefined,
    }
  } catch {
    return {}
  }
}

export function stringifyAgentBranding(input: AgentBranding) {
  const siteName = input.site_name?.trim() ?? ''
  const logo = input.logo?.trim() ?? ''
  const homePageContent = input.home_page_content?.trim() ?? ''
  const headerNavModules = input.header_nav_modules?.trim() ?? ''
  const siteStyle = input.site_style?.trim() ?? ''
  if (!siteName && !logo && !homePageContent && !headerNavModules && !siteStyle)
    return ''
  return JSON.stringify({
    site_name: siteName,
    logo,
    home_page_content: homePageContent,
    header_nav_modules: headerNavModules,
    site_style: siteStyle,
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

export async function listAgentGroupRatios() {
  const res = await api.get<{
    success: boolean
    data: AgentGroupRatio[]
  }>('/api/agent/group_ratios')
  return res.data
}

type AgentGroupRatioPayloadInput = {
  group_name: string
  system_group_name: string
  description?: string
  ratio: number
  visible: boolean
}

export function buildAgentGroupRatioPayload(
  input: AgentGroupRatioPayloadInput
) {
  const systemGroupName = input.system_group_name.trim()
  return {
    group_name: systemGroupName,
    system_group_name: systemGroupName,
    description: input.description?.trim() ?? '',
    ratio: input.ratio,
    visible: input.visible,
  }
}

export function getAgentGroupRatioEditValue(rule: AgentGroupRatio) {
  if (rule.configured) {
    return String(
      rule.configured_ratio ?? rule.agent_ratio ?? rule.system_ratio ?? 1
    )
  }
  return String(rule.agent_ratio ?? rule.system_ratio ?? 1)
}

export function getAgentGroupRatioFormDraft(rule: AgentGroupRatio) {
  return {
    systemGroupName: rule.system_group_name || rule.group_name,
    description: rule.description ?? '',
    ratio: getAgentGroupRatioEditValue(rule),
    visible: rule.visible,
  }
}

export function getAgentSystemGroupDefaultRatio(
  groupRatios: AgentGroupRatio[],
  systemGroupName: string
) {
  return String(
    getAgentSystemGroupRatioFloor(groupRatios, systemGroupName) || 1
  )
}

export function getAgentGroupRatioInputFloor(
  groupRatios: AgentGroupRatio[],
  systemGroupName: string
) {
  const trimmedSystemGroupName = systemGroupName.trim()
  const configuredRule = groupRatios.find(
    (item) =>
      item.configured &&
      item.system_group_name === trimmedSystemGroupName &&
      getAgentRatioFloor(item) != null
  )
  const configuredFloor = getAgentRatioFloor(configuredRule)
  if (configuredFloor != null) {
    return configuredFloor
  }
  return getAgentSystemGroupRatioFloor(groupRatios, trimmedSystemGroupName)
}

export function getAgentSystemGroupRatioFloor(
  groupRatios: AgentGroupRatio[],
  systemGroupName: string
) {
  const trimmedSystemGroupName = systemGroupName.trim()
  const configuredSystemGroup = groupRatios.find(
    (item) =>
      item.configured &&
      item.system_group_name === trimmedSystemGroupName &&
      getAgentRatioFloor(item) != null
  )
  if (configuredSystemGroup) {
    return getAgentRatioFloor(configuredSystemGroup) ?? 0
  }
  const systemGroup = groupRatios.find(
    (item) => item.system_group_name === trimmedSystemGroupName
  )
  return getAgentRatioFloor(systemGroup) ?? systemGroup?.system_ratio ?? 0
}

export function getAgentGroupRatioTableValues(rule: AgentGroupRatio) {
  return {
    agentDiscount: String(getAgentRatioFloor(rule) ?? '-'),
    effectiveDiscount: String(rule.effective_ratio),
  }
}

export function buildAgentGroupRuleRows(groupRatios: AgentGroupRatio[]) {
  return [...groupRatios]
    .sort((a, b) => {
      if (a.configured !== b.configured) return a.configured ? -1 : 1
      return (a.system_group_name || a.group_name).localeCompare(
        b.system_group_name || b.group_name
      )
    })
    .map((rule) => ({
      systemGroupName: rule.system_group_name || rule.group_name,
      status: rule.configured ? 'configured' : 'system_default',
      agentDiscount: String(getAgentRatioFloor(rule) ?? '-'),
      effectiveDiscount: String(rule.effective_ratio),
      baseDiscount: String(rule.system_ratio),
      visible: rule.visible,
      available: rule.available,
    }))
}

export function getAgentUserGroupRatioFloor(rule: AgentGroupRatio) {
  const agentRatio = getAgentRatioFloor(rule)
  if (agentRatio != null) {
    return agentRatio
  }
  return rule.system_ratio ?? 0
}

function getAgentRatioFloor(rule?: AgentGroupRatio) {
  if (!rule) return undefined
  if (rule.agent_ratio != null && rule.agent_ratio >= 0) {
    return rule.agent_ratio
  }
  if (rule.configured && rule.configured_ratio >= 0) {
    return rule.configured_ratio
  }
  return undefined
}

export async function upsertAgentGroupRatio(
  input: AgentGroupRatioPayloadInput
) {
  const res = await api.post<{ success: boolean; data: AgentGroupRatio }>(
    '/api/agent/group_ratios',
    buildAgentGroupRatioPayload(input)
  )
  return res.data
}

export async function listAgentUserGroups() {
  const res = await api.get<{
    success: boolean
    data: AgentUserGroupConfig[]
  }>('/api/agent/user_groups')
  return res.data
}

type AgentUserGroupPayloadInput = {
  group_name: string
  visible_groups?: string[]
  group_ratios?: Record<string, number>
}

export function buildAgentUserGroupPayload(input: AgentUserGroupPayloadInput) {
  return {
    group_name: input.group_name.trim(),
    visible_groups: input.visible_groups ?? [],
    group_ratios: input.group_ratios ?? {},
  }
}

export function getAgentUserGroupFormDraft(rule?: AgentUserGroupConfig) {
  return {
    groupName: rule?.group_name ?? '',
    visibleGroups: [...(rule?.visible_groups ?? [])],
    groupRatios: { ...(rule?.group_ratios ?? {}) },
  }
}

export async function upsertAgentUserGroup(input: AgentUserGroupPayloadInput) {
  const res = await api.post<{
    success: boolean
    data: AgentUserGroupConfig
  }>('/api/agent/user_groups', buildAgentUserGroupPayload(input))
  return res.data
}

export function buildAgentUserListParams(
  page = 1,
  pageSize = 20,
  keyword = ''
) {
  const trimmedKeyword = keyword.trim()
  return {
    p: page,
    page_size: pageSize,
    ...(trimmedKeyword ? { keyword: trimmedKeyword } : {}),
  }
}

export async function listAgentUsers(page = 1, pageSize = 20, keyword = '') {
  const res = await api.get<{ success: boolean; data: AgentPage<AgentUser> }>(
    '/api/agent/users',
    { params: buildAgentUserListParams(page, pageSize, keyword) }
  )
  return res.data
}

export async function updateAgentUserStatus(input: {
  userId: number
  status: number
}) {
  const res = await api.put<{
    success: boolean
    data: { user_id: number; status: number }
  }>(`/api/agent/users/${input.userId}/status`, {
    status: input.status,
  })
  return res.data
}

export async function updateAgentUserGroup(input: {
  userId: number
  group_name: string
}) {
  const res = await api.put<{
    success: boolean
    data: { user_id: number; group: string }
  }>(`/api/agent/users/${input.userId}/group`, {
    group_name: input.group_name,
  })
  return res.data
}

export async function fundAgentUserBalance(input: {
  userId: number
  amount_money: number
}) {
  const res = await api.post<{
    success: boolean
    data: { user_id: number; balance: AgentBalance }
  }>(`/api/agent/users/${input.userId}/balance`, {
    amount_money: input.amount_money,
  })
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
  const res = await api.put<{
    success: boolean
    data: { id: number; status: number }
  }>(`/api/agents/${input.agentId}/domains/${input.domainId}/status`, {
    status: input.status,
  })
  return res.data
}

export async function listAdminAgentUsers(
  agentId: number,
  page = 1,
  pageSize = 20,
  keyword = ''
) {
  const res = await api.get<{ success: boolean; data: AgentPage<AgentUser> }>(
    `/api/agents/${agentId}/users`,
    { params: buildAgentUserListParams(page, pageSize, keyword) }
  )
  return res.data
}

export async function listAdminAgentGroupRatios(agentId: number) {
  const res = await api.get<{
    success: boolean
    data: AgentGroupRatio[]
  }>(`/api/agents/${agentId}/group_ratios`)
  return res.data
}

export async function upsertAdminAgentGroupRatio(input: {
  agentId: number
  group_name: string
  system_group_name: string
  description?: string
  ratio: number
  visible: boolean
}) {
  const res = await api.post<{ success: boolean; data: AgentGroupRatio }>(
    `/api/agents/${input.agentId}/group_ratios`,
    buildAgentGroupRatioPayload(input)
  )
  return res.data
}

export async function listAdminAgentUserGroups(agentId: number) {
  const res = await api.get<{
    success: boolean
    data: AgentUserGroupConfig[]
  }>(`/api/agents/${agentId}/user_groups`)
  return res.data
}

export async function upsertAdminAgentUserGroup(input: {
  agentId: number
  group_name: string
  visible_groups?: string[]
  group_ratios?: Record<string, number>
}) {
  const payload = buildAgentUserGroupPayload(input)
  const res = await api.post<{
    success: boolean
    data: AgentUserGroupConfig
  }>(`/api/agents/${input.agentId}/user_groups`, payload)
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

export async function getAdminAgentBalance(agentId: number) {
  const res = await api.get<{ success: boolean; data: AgentBalance }>(
    `/api/agents/${agentId}/balance`
  )
  return res.data
}

export async function addAdminAgentBalance(input: {
  agentId: number
  amount_money: number
  remark?: string
}) {
  const res = await api.post<{ success: boolean; data: AgentBalance }>(
    `/api/agents/${input.agentId}/balance`,
    {
      amount_money: input.amount_money,
      remark: input.remark?.trim() ?? '',
    }
  )
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
