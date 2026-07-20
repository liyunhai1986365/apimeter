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
import { type ReactNode, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import {
  BadgeDollarSign,
  Check,
  CircleDollarSign,
  Globe2,
  Eye,
  Plus,
  RefreshCcw,
  Save,
  Settings2,
  Store,
  UserPlus,
  Users,
  WalletCards,
  X,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatTimestampToDate } from '@/lib/format'
import {
  parseHeaderNavModules,
  serializeHeaderNavModules,
  type HeaderNavModules,
} from '@/lib/nav-modules'
import { normalizePagedData } from '@/lib/paged-response'
import {
  parseThemeCustomization,
  serializeThemeCustomization,
  type ThemeCustomization,
} from '@/lib/theme-customization'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { TableEmpty } from '@/components/data-table'
import { GroupBadge } from '@/components/group-badge'
import { SectionPageLayout } from '@/components/layout'
import { LongText } from '@/components/long-text'
import { ThemeCustomizationEditor } from '@/components/theme-customization-editor'
import { HeaderNavigationSection } from '@/features/system-settings/maintenance/header-navigation-section'
import {
  bindAdminAgentUser,
  completeAdminAgentWithdrawal,
  createAdminAgent,
  createAdminAgentDomain,
  getAgentGroupRatioFormDraft,
  getAgentGroupRatioInputFloor,
  getAgentSystemGroupDefaultRatio,
  getAgentUserGroupFormDraft,
  listAdminAgentDomainsByAgent,
  listAdminAgentGroupRatios,
  listAdminAgentUserGroups,
  listAdminAgents,
  listAdminAgentUsers,
  listAdminAgentWithdrawals,
  parseAgentBranding,
  stringifyAgentBranding,
  switchAgentViewContext,
  updateAdminAgent,
  updateAdminAgentDomainStatus,
  upsertAdminAgentGroupRatio,
  upsertAdminAgentUserGroup,
} from './api'
import { AgentGroupManager } from './components/agent-group-manager'
import { AgentUserGroupManager } from './components/agent-user-group-manager'
import {
  AgentUsersPaginationControls,
  AgentUsersSearchControls,
} from './components/agent-users-list-controls'
import { AGENT_HOME_PAGE_CONTENT_TEXTAREA_CLASS } from './constants'
import type {
  Agent,
  AgentDomain,
  AgentGroupRatio,
  AgentUser,
  AgentUserGroupConfig,
  AgentWithdrawal,
} from './types'

const AGENT_DOMAIN_STATUS_ACTIVE = 1
const AGENT_DOMAIN_STATUS_DISABLED = 2
const AGENT_STATUS_ENABLED = 1

const formatQuota = (quota?: number) => (quota ?? 0).toLocaleString()

function normalizeSettlementCurrency(currency?: string) {
  const code = (currency || 'USD').toUpperCase()
  return code === 'RMB' || code === 'CNY' ? 'RMB' : 'USD'
}

function formatSettlementAmount(amount?: number, currency?: string) {
  const value = amount ?? 0
  const code = normalizeSettlementCurrency(currency)
  const symbol = code === 'RMB' ? '¥' : '$'
  const sign = value < 0 ? '-' : ''
  return `${sign}${symbol}${Math.abs(value).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 6,
  })} ${code}`
}

function agentStatusLabel(status: number) {
  return status === AGENT_STATUS_ENABLED ? 'Enabled' : 'Disabled'
}

function domainStatusLabel(status: number) {
  if (status === AGENT_DOMAIN_STATUS_ACTIVE) return 'Active'
  if (status === AGENT_DOMAIN_STATUS_DISABLED) return 'Disabled'
  return 'Pending'
}

function domainStatusVariant(
  status: number
): 'default' | 'outline' | 'secondary' {
  if (status === AGENT_DOMAIN_STATUS_ACTIVE) return 'default'
  if (status === AGENT_DOMAIN_STATUS_DISABLED) return 'secondary'
  return 'outline'
}

function withdrawalVariant(
  status: string
): 'default' | 'outline' | 'secondary' | 'destructive' {
  if (status === 'paid') return 'default'
  if (status === 'approved') return 'secondary'
  if (status === 'rejected') return 'destructive'
  return 'outline'
}

export function AgentManagement() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [selectedAgentId, setSelectedAgentId] = useState<number | null>(null)
  const [detailAgentId, setDetailAgentId] = useState<number | null>(null)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [createdAgent, setCreatedAgent] = useState<Agent | null>(null)
  const [newAgentOwnerId, setNewAgentOwnerId] = useState('')
  const [newAgentName, setNewAgentName] = useState('')
  const [newAgentSlug, setNewAgentSlug] = useState('')
  const [newAgentSiteName, setNewAgentSiteName] = useState('')
  const [newAgentLogo, setNewAgentLogo] = useState('')
  const [newAgentHomePageContent, setNewAgentHomePageContent] = useState('')
  const [brandSiteName, setBrandSiteName] = useState('')
  const [brandLogo, setBrandLogo] = useState('')
  const [brandHomePageContent, setBrandHomePageContent] = useState('')
  const [brandHeaderNavModules, setBrandHeaderNavModules] = useState('')
  const [brandSiteStyle, setBrandSiteStyle] = useState('')
  const [newDomain, setNewDomain] = useState('')
  const [bindUserId, setBindUserId] = useState('')
  const [systemGroupName, setSystemGroupName] = useState('default')
  const [groupDescription, setGroupDescription] = useState('')
  const [groupRatio, setGroupRatio] = useState('1')
  const [groupVisible, setGroupVisible] = useState(true)
  const [userGroupName, setUserGroupName] = useState('')
  const [userGroupVisibleGroups, setUserGroupVisibleGroups] = useState<
    string[]
  >([])
  const [userGroupRatios, setUserGroupRatios] = useState<
    Record<string, number>
  >({})
  const [withdrawalRemark, setWithdrawalRemark] = useState('')
  const [selectedUsersKeywordInput, setSelectedUsersKeywordInput] = useState('')
  const [selectedUsersKeyword, setSelectedUsersKeyword] = useState('')
  const [selectedUsersPageNumber, setSelectedUsersPageNumber] = useState(1)
  const [selectedUsersPageSize, setSelectedUsersPageSize] = useState(20)

  const agentsQuery = useQuery({
    queryKey: ['admin', 'agents'],
    queryFn: () => listAdminAgents(1, 50),
  })

  const selectedUsersQuery = useQuery({
    queryKey: [
      'admin',
      'agents',
      selectedAgentId,
      'users',
      selectedUsersPageNumber,
      selectedUsersPageSize,
      selectedUsersKeyword,
    ],
    queryFn: () =>
      listAdminAgentUsers(
        selectedAgentId ?? 0,
        selectedUsersPageNumber,
        selectedUsersPageSize,
        selectedUsersKeyword
      ),
    enabled: selectedAgentId != null,
  })

  const selectedDomainsQuery = useQuery({
    queryKey: ['admin', 'agents', selectedAgentId, 'domains'],
    queryFn: () => listAdminAgentDomainsByAgent(selectedAgentId ?? 0, 1, 50),
    enabled: selectedAgentId != null,
  })

  const selectedGroupRatiosQuery = useQuery({
    queryKey: ['admin', 'agents', selectedAgentId, 'group-ratios'],
    queryFn: () => listAdminAgentGroupRatios(selectedAgentId ?? 0),
    enabled: selectedAgentId != null,
  })

  const selectedUserGroupsQuery = useQuery({
    queryKey: ['admin', 'agents', selectedAgentId, 'user-groups'],
    queryFn: () => listAdminAgentUserGroups(selectedAgentId ?? 0),
    enabled: selectedAgentId != null,
  })

  const withdrawalsQuery = useQuery({
    queryKey: ['admin', 'agents', 'withdrawals'],
    queryFn: () => listAdminAgentWithdrawals(undefined, 1, 50),
  })

  const agentsPage = useMemo(
    () => normalizePagedData(agentsQuery.data),
    [agentsQuery.data]
  )
  const selectedUsersPage = useMemo(
    () => normalizePagedData(selectedUsersQuery.data),
    [selectedUsersQuery.data]
  )
  const selectedDomainsPage = useMemo(
    () => normalizePagedData(selectedDomainsQuery.data),
    [selectedDomainsQuery.data]
  )
  const withdrawalsPage = useMemo(
    () => normalizePagedData(withdrawalsQuery.data),
    [withdrawalsQuery.data]
  )
  const agents = agentsPage.items
  const selectedAgent = useMemo(
    () => agents.find((agent) => agent.id === selectedAgentId),
    [agents, selectedAgentId]
  )
  const detailAgent = useMemo(
    () => agents.find((agent) => agent.id === detailAgentId) ?? createdAgent,
    [agents, detailAgentId, createdAgent]
  )
  const selectedUsers = selectedUsersPage.items
  const selectedDomains = selectedDomainsPage.items
  const selectedGroupRatios = selectedGroupRatiosQuery.data?.data ?? []
  const selectedUserGroups = selectedUserGroupsQuery.data?.data ?? []
  const withdrawals = withdrawalsPage.items
  const pendingWithdrawalCount = withdrawals.filter(
    (withdrawal) => withdrawal.status === 'pending'
  ).length
  const totalAgentUsers = selectedUsersPage.total
  const selectAgent = (agentId: number) => {
    if (selectedAgentId !== agentId) {
      setSelectedUsersKeywordInput('')
      setSelectedUsersKeyword('')
      setSelectedUsersPageNumber(1)
    }
    setSelectedAgentId(agentId)
  }
  const openAgentDetail = (agent: Agent) => {
    const branding = parseAgentBranding(agent.branding)
    setBrandSiteName(branding.site_name ?? '')
    setBrandLogo(branding.logo ?? '')
    setBrandHomePageContent(branding.home_page_content ?? '')
    setBrandHeaderNavModules(branding.header_nav_modules ?? '')
    setBrandSiteStyle(branding.site_style ?? '')
    setUserGroupName('')
    setUserGroupVisibleGroups([])
    setUserGroupRatios({})
    selectAgent(agent.id)
    setDetailAgentId(agent.id)
  }
  const searchSelectedAgentUsers = () => {
    setSelectedUsersPageNumber(1)
    setSelectedUsersKeyword(selectedUsersKeywordInput.trim())
  }
  const clearSelectedAgentUserSearch = () => {
    setSelectedUsersKeywordInput('')
    setSelectedUsersKeyword('')
    setSelectedUsersPageNumber(1)
  }
  const brandHeaderNavConfig = useMemo(
    () => parseHeaderNavModules(brandHeaderNavModules),
    [brandHeaderNavModules]
  )
  const brandHeaderNavSerialized = useMemo(
    () => serializeHeaderNavModules(brandHeaderNavConfig),
    [brandHeaderNavConfig]
  )
  const brandSiteStyleConfig = useMemo(
    () => parseThemeCustomization(brandSiteStyle),
    [brandSiteStyle]
  )

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['admin', 'agents'] })
  }

  const refreshSelectedAgent = () => {
    refresh()
    if (selectedAgentId != null) {
      queryClient.invalidateQueries({
        queryKey: ['admin', 'agents', selectedAgentId],
      })
    }
  }

  const createAgentMutation = useMutation({
    mutationFn: createAdminAgent,
    onSuccess: (res) => {
      toast.success(t('Agent created'))
      setCreatedAgent(res.data)
      openAgentDetail(res.data)
      setNewAgentOwnerId('')
      setNewAgentName('')
      setNewAgentSlug('')
      setNewAgentSiteName('')
      setNewAgentLogo('')
      setCreateDialogOpen(false)
      refresh()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const saveBrandingMutation = useMutation({
    mutationFn: updateAdminAgent,
    onSuccess: (res) => {
      toast.success(t('Branding saved'))
      setCreatedAgent(res.data)
      refresh()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const createDomainMutation = useMutation({
    mutationFn: createAdminAgentDomain,
    onSuccess: () => {
      toast.success(t('Domain added'))
      setNewDomain('')
      refreshSelectedAgent()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const bindUserMutation = useMutation({
    mutationFn: bindAdminAgentUser,
    onSuccess: () => {
      toast.success(t('Agent user bound'))
      setBindUserId('')
      refreshSelectedAgent()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const savePricingMutation = useMutation({
    mutationFn: upsertAdminAgentGroupRatio,
    onSuccess: () => {
      toast.success(t('Group ratio saved'))
      refreshSelectedAgent()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const saveUserGroupMutation = useMutation({
    mutationFn: upsertAdminAgentUserGroup,
    onSuccess: () => {
      toast.success(t('User group saved'))
      refreshSelectedAgent()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const updateDomainStatusMutation = useMutation({
    mutationFn: updateAdminAgentDomainStatus,
    onSuccess: () => {
      toast.success(t('Updated successfully'))
      refreshSelectedAgent()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const completeWithdrawalMutation = useMutation({
    mutationFn: completeAdminAgentWithdrawal,
    onSuccess: () => {
      toast.success(t('Updated successfully'))
      setWithdrawalRemark('')
      refresh()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const viewAgentMutation = useMutation({
    mutationFn: switchAgentViewContext,
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: ['agent'] })
      navigate({ to: '/agents' })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const canCreateAgent =
    Number(newAgentOwnerId) > 0 &&
    newAgentName.trim() !== '' &&
    newAgentSlug.trim() !== ''
  const canCreateDomain = selectedAgentId != null && newDomain.trim() !== ''
  const canBindUser = selectedAgentId != null && Number(bindUserId) > 0
  const selectedGroupBaseRatio = getAgentGroupRatioInputFloor(
    selectedGroupRatios,
    systemGroupName
  )
  const canSavePricing =
    selectedAgentId != null &&
    systemGroupName.trim() !== '' &&
    Number(groupRatio) >= selectedGroupBaseRatio
  const canSaveUserGroup =
    selectedAgentId != null && userGroupName.trim() !== ''

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Agent Management')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button variant='outline' onClick={refresh}>
            <RefreshCcw />
            {t('Refresh')}
          </Button>
          <Button type='button' onClick={() => setCreateDialogOpen(true)}>
            <Plus />
            {t('Create Agent')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='space-y-4'>
            <div className='grid gap-3 md:grid-cols-4'>
              <MetricCard
                label={t('Total Agents')}
                value={formatQuota(agentsPage.total)}
                icon={<Store className='size-4' />}
              />
              <MetricCard
                label={t('Selected Agent')}
                value={selectedAgent?.name ?? '-'}
                icon={<Settings2 className='size-4' />}
              />
              <MetricCard
                label={t('Agent Users')}
                value={
                  selectedAgentId == null ? '-' : formatQuota(totalAgentUsers)
                }
                icon={<Users className='size-4' />}
              />
              <MetricCard
                label={t('Pending Withdrawal')}
                value={formatQuota(pendingWithdrawalCount)}
                icon={<WalletCards className='size-4' />}
              />
            </div>

            <Tabs defaultValue='agents'>
              <TabsList>
                <TabsTrigger value='agents'>
                  <Store className='size-4' />
                  {t('Agents')}
                </TabsTrigger>
                <TabsTrigger value='users'>
                  <Users className='size-4' />
                  {t('Agent Users')}
                </TabsTrigger>
                <TabsTrigger value='withdrawals'>
                  <BadgeDollarSign className='size-4' />
                  {t('Withdrawal Processing')}
                </TabsTrigger>
              </TabsList>

              <TabsContent value='agents'>
                <section className='rounded-lg border p-3'>
                  <div className='mb-3 flex items-center justify-between gap-2'>
                    <div>
                      <h3 className='text-sm font-semibold'>{t('Agents')}</h3>
                      <p className='text-muted-foreground mt-1 text-xs'>
                        {t(
                          'Admins create agents, bind users, and manage configuration here.'
                        )}
                      </p>
                    </div>
                  </div>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('Agent Name')}</TableHead>
                        <TableHead>{t('Owner User ID')}</TableHead>
                        <TableHead>{t('Slug')}</TableHead>
                        <TableHead>{t('Status')}</TableHead>
                        <TableHead>{t('Created At')}</TableHead>
                        <TableHead className='text-right'>
                          {t('Actions')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {agentsQuery.isLoading ? (
                        <LoadingRow colSpan={6} />
                      ) : agents.length === 0 ? (
                        <TableEmpty
                          colSpan={6}
                          title={t('No Agents')}
                          description={t('Agent records will appear here.')}
                          icon={<Store className='size-6' />}
                        />
                      ) : (
                        agents.map((agent) => (
                          <TableRow
                            key={agent.id}
                            data-state={
                              selectedAgentId === agent.id
                                ? 'selected'
                                : undefined
                            }
                          >
                            <TableCell className='font-medium'>
                              {agent.name}
                            </TableCell>
                            <TableCell>{agent.owner_user_id}</TableCell>
                            <TableCell className='font-mono text-xs'>
                              {agent.slug}
                            </TableCell>
                            <TableCell>
                              <Badge
                                variant={
                                  agent.status === AGENT_STATUS_ENABLED
                                    ? 'default'
                                    : 'outline'
                                }
                              >
                                {t(agentStatusLabel(agent.status))}
                              </Badge>
                            </TableCell>
                            <TableCell>
                              {formatTimestampToDate(agent.created_at)}
                            </TableCell>
                            <TableCell className='text-right'>
                              <div className='inline-flex gap-2'>
                                <Button
                                  size='sm'
                                  variant='outline'
                                  disabled={viewAgentMutation.isPending}
                                  onClick={() =>
                                    viewAgentMutation.mutate(agent.id)
                                  }
                                >
                                  <Eye />
                                  {t('View Agent Console')}
                                </Button>
                                <Button
                                  size='sm'
                                  variant='outline'
                                  onClick={() => selectAgent(agent.id)}
                                >
                                  {t('Select')}
                                </Button>
                                <Button
                                  size='sm'
                                  onClick={() => {
                                    openAgentDetail(agent)
                                  }}
                                >
                                  <Settings2 />
                                  {t('Manage')}
                                </Button>
                              </div>
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </section>
              </TabsContent>

              <TabsContent value='users'>
                <section className='rounded-lg border p-3'>
                  <div className='mb-3 grid gap-3 lg:grid-cols-[minmax(0,1fr)_360px]'>
                    <div>
                      <h3 className='text-sm font-semibold'>
                        {t('Agent Users')}
                      </h3>
                      <p className='text-muted-foreground mt-1 text-xs'>
                        {selectedAgent
                          ? t('Only bound users can access the agent console.')
                          : t('Select an agent before binding users.')}
                      </p>
                    </div>
                    <div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]'>
                      <Input
                        value={bindUserId}
                        onChange={(event) => setBindUserId(event.target.value)}
                        disabled={selectedAgentId == null}
                        type='number'
                        min='1'
                        placeholder={t('User ID')}
                      />
                      <Button
                        disabled={!canBindUser || bindUserMutation.isPending}
                        onClick={() =>
                          selectedAgentId != null &&
                          bindUserMutation.mutate({
                            agentId: selectedAgentId,
                            userId: Number(bindUserId),
                          })
                        }
                      >
                        <UserPlus />
                        {t('Bind User')}
                      </Button>
                    </div>
                  </div>
                  {selectedAgentId == null ? (
                    <EmptySelection
                      message={t('Select an agent to manage users.')}
                    />
                  ) : (
                    <>
                      <AgentUsersSearchControls
                        keyword={selectedUsersKeywordInput}
                        isLoading={
                          selectedUsersQuery.isLoading ||
                          selectedUsersQuery.isFetching
                        }
                        onKeywordChange={setSelectedUsersKeywordInput}
                        onSearch={searchSelectedAgentUsers}
                        onClear={clearSelectedAgentUserSearch}
                      />
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>{t('Username')}</TableHead>
                            <TableHead>{t('Source')}</TableHead>
                            <TableHead>{t('Status')}</TableHead>
                            <TableHead>{t('Group')}</TableHead>
                            <TableHead>{t('Quota')}</TableHead>
                            <TableHead>{t('Created At')}</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {selectedUsersQuery.isLoading ? (
                            <LoadingRow colSpan={6} />
                          ) : selectedUsers.length === 0 ? (
                            <TableEmpty
                              colSpan={6}
                              title={t('No Agent Users')}
                              description={t(
                                'Bind users to open the agent console for them.'
                              )}
                              icon={<Users className='size-6' />}
                            />
                          ) : (
                            selectedUsers.map((user) => (
                              <TableRow key={user.id}>
                                <TableCell>
                                  <div className='flex min-w-[160px] flex-col gap-1'>
                                    <LongText className='max-w-[150px] font-medium'>
                                      {user.username || `#${user.user_id}`}
                                    </LongText>
                                    <span className='text-muted-foreground text-xs'>
                                      #{user.user_id}
                                    </span>
                                  </div>
                                </TableCell>
                                <TableCell>{user.source}</TableCell>
                                <TableCell>
                                  <Badge variant='outline'>
                                    {t(
                                      domainStatusLabel(user.agent_user_status)
                                    )}
                                  </Badge>
                                </TableCell>
                                <TableCell>
                                  <GroupBadge group={user.group} />
                                </TableCell>
                                <TableCell>
                                  {user.quota.toLocaleString()}
                                </TableCell>
                                <TableCell>
                                  {formatTimestampToDate(user.created_at)}
                                </TableCell>
                              </TableRow>
                            ))
                          )}
                        </TableBody>
                      </Table>
                      <AgentUsersPaginationControls
                        page={selectedUsersPage.page ?? selectedUsersPageNumber}
                        pageSize={
                          selectedUsersPage.page_size ?? selectedUsersPageSize
                        }
                        total={selectedUsersPage.total}
                        itemCount={selectedUsers.length}
                        isLoading={
                          selectedUsersQuery.isLoading ||
                          selectedUsersQuery.isFetching
                        }
                        onPageChange={setSelectedUsersPageNumber}
                        onPageSizeChange={(nextPageSize) => {
                          setSelectedUsersPageSize(nextPageSize)
                          setSelectedUsersPageNumber(1)
                        }}
                      />
                    </>
                  )}
                </section>
              </TabsContent>

              <TabsContent value='withdrawals'>
                <section className='rounded-lg border p-3'>
                  <div className='mb-3 grid gap-3 lg:grid-cols-[minmax(0,1fr)_360px]'>
                    <div>
                      <h3 className='text-sm font-semibold'>
                        {t('Withdrawal Processing')}
                      </h3>
                      <p className='text-muted-foreground mt-1 text-xs'>
                        {t(
                          'Approve, reject, or mark agent withdrawals as paid.'
                        )}
                      </p>
                    </div>
                    <Textarea
                      value={withdrawalRemark}
                      onChange={(event) =>
                        setWithdrawalRemark(event.target.value)
                      }
                      placeholder={t('Admin Remark')}
                      className='min-h-20'
                    />
                  </div>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('Agent ID')}</TableHead>
                        <TableHead>{t('Money Amount')}</TableHead>
                        <TableHead>{t('Status')}</TableHead>
                        <TableHead>{t('Created At')}</TableHead>
                        <TableHead className='text-right'>
                          {t('Actions')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {withdrawalsQuery.isLoading ? (
                        <LoadingRow colSpan={5} />
                      ) : withdrawals.length === 0 ? (
                        <TableEmpty
                          colSpan={5}
                          title={t('No Withdrawals')}
                          description={t(
                            'Agent withdrawal requests will appear here.'
                          )}
                          icon={<CircleDollarSign className='size-6' />}
                        />
                      ) : (
                        withdrawals.map((withdrawal) => (
                          <TableRow key={withdrawal.id}>
                            <TableCell>{withdrawal.agent_id}</TableCell>
                            <TableCell>
                              {formatSettlementAmount(
                                withdrawal.settlement_amount ??
                                  withdrawal.amount_money,
                                withdrawal.currency
                              )}
                            </TableCell>
                            <TableCell>
                              <Badge
                                variant={withdrawalVariant(withdrawal.status)}
                              >
                                {t(withdrawal.status)}
                              </Badge>
                            </TableCell>
                            <TableCell>
                              {formatTimestampToDate(withdrawal.created_at)}
                            </TableCell>
                            <TableCell className='text-right'>
                              <WithdrawalActions
                                withdrawal={withdrawal}
                                disabled={completeWithdrawalMutation.isPending}
                                onUpdate={(status) =>
                                  completeWithdrawalMutation.mutate({
                                    withdrawalId: withdrawal.id,
                                    status,
                                    admin_remark: withdrawalRemark,
                                  })
                                }
                              />
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </section>
              </TabsContent>
            </Tabs>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <CreateAgentDialog
        ownerUserId={newAgentOwnerId}
        name={newAgentName}
        slug={newAgentSlug}
        siteName={newAgentSiteName}
        logo={newAgentLogo}
        homePageContent={newAgentHomePageContent}
        isPending={createAgentMutation.isPending}
        canSubmit={canCreateAgent}
        onOwnerUserIdChange={setNewAgentOwnerId}
        onNameChange={setNewAgentName}
        onSlugChange={setNewAgentSlug}
        onSiteNameChange={setNewAgentSiteName}
        onLogoChange={setNewAgentLogo}
        onHomePageContentChange={setNewAgentHomePageContent}
        onSubmit={() =>
          createAgentMutation.mutate({
            owner_user_id: Number(newAgentOwnerId),
            name: newAgentName.trim(),
            slug: newAgentSlug.trim(),
            status: AGENT_STATUS_ENABLED,
            default_markup: 1,
            branding: stringifyAgentBranding({
              site_name: newAgentSiteName,
              logo: newAgentLogo,
              home_page_content: newAgentHomePageContent,
            }),
          })
        }
        open={createDialogOpen}
        onOpenChange={setCreateDialogOpen}
      />

      <AgentDetailDialog
        agent={detailAgent}
        open={detailAgentId != null}
        onOpenChange={(open) => {
          if (!open) setDetailAgentId(null)
        }}
        domains={selectedDomains}
        groupRatios={selectedGroupRatios}
        userGroups={selectedUserGroups}
        users={selectedUsers}
        usersPage={selectedUsersPage.page ?? selectedUsersPageNumber}
        usersPageSize={selectedUsersPage.page_size ?? selectedUsersPageSize}
        usersTotal={selectedUsersPage.total}
        usersLoading={
          selectedUsersQuery.isLoading || selectedUsersQuery.isFetching
        }
        usersKeyword={selectedUsersKeywordInput}
        brandSiteName={brandSiteName}
        brandLogo={brandLogo}
        brandHomePageContent={brandHomePageContent}
        brandHeaderNavConfig={brandHeaderNavConfig}
        brandHeaderNavSerialized={brandHeaderNavSerialized}
        brandSiteStyleConfig={brandSiteStyleConfig}
        newDomain={newDomain}
        bindUserId={bindUserId}
        systemGroupName={systemGroupName}
        groupDescription={groupDescription}
        groupRatio={groupRatio}
        groupRatioFloor={selectedGroupBaseRatio}
        groupVisible={groupVisible}
        userGroupName={userGroupName}
        userGroupVisibleGroups={userGroupVisibleGroups}
        userGroupRatios={userGroupRatios}
        isDomainPending={createDomainMutation.isPending}
        isBindPending={bindUserMutation.isPending}
        isPricingPending={savePricingMutation.isPending}
        isUserGroupPending={saveUserGroupMutation.isPending}
        isDomainStatusPending={updateDomainStatusMutation.isPending}
        canCreateDomain={canCreateDomain}
        canBindUser={canBindUser}
        canSavePricing={canSavePricing}
        canSaveUserGroup={canSaveUserGroup}
        onNewDomainChange={setNewDomain}
        onBindUserIdChange={setBindUserId}
        onUsersKeywordChange={setSelectedUsersKeywordInput}
        onUsersSearch={searchSelectedAgentUsers}
        onUsersClear={clearSelectedAgentUserSearch}
        onUsersPageChange={setSelectedUsersPageNumber}
        onUsersPageSizeChange={(nextPageSize) => {
          setSelectedUsersPageSize(nextPageSize)
          setSelectedUsersPageNumber(1)
        }}
        onSystemGroupNameChange={(nextGroup) => {
          setSystemGroupName(nextGroup)
          const rule = selectedGroupRatios.find(
            (item) => item.system_group_name === nextGroup
          )
          setGroupDescription(rule?.description ?? '')
          setGroupRatio(
            rule
              ? getAgentGroupRatioFormDraft(rule).ratio
              : getAgentSystemGroupDefaultRatio(selectedGroupRatios, nextGroup)
          )
          setGroupVisible(rule?.visible ?? true)
        }}
        onGroupDescriptionChange={setGroupDescription}
        onGroupRatioChange={setGroupRatio}
        onGroupVisibleChange={setGroupVisible}
        onUserGroupNameChange={setUserGroupName}
        onUserGroupVisibleGroupsChange={setUserGroupVisibleGroups}
        onUserGroupRatiosChange={setUserGroupRatios}
        onBrandSiteNameChange={setBrandSiteName}
        onBrandLogoChange={setBrandLogo}
        onBrandHomePageContentChange={setBrandHomePageContent}
        onBrandSiteStyleChange={(next) =>
          setBrandSiteStyle(serializeThemeCustomization(next))
        }
        isBrandingPending={saveBrandingMutation.isPending}
        onSaveBranding={() =>
          detailAgent &&
          saveBrandingMutation.mutate({
            id: detailAgent.id,
            owner_user_id: detailAgent.owner_user_id,
            name: detailAgent.name,
            slug: detailAgent.slug,
            status: detailAgent.status,
            default_markup: detailAgent.default_markup,
            branding: stringifyAgentBranding({
              site_name: brandSiteName,
              logo: brandLogo,
              home_page_content: brandHomePageContent,
              header_nav_modules: brandHeaderNavModules,
              site_style: brandSiteStyle,
            }),
          })
        }
        onSaveSiteStyle={() =>
          detailAgent &&
          saveBrandingMutation.mutate({
            id: detailAgent.id,
            owner_user_id: detailAgent.owner_user_id,
            name: detailAgent.name,
            slug: detailAgent.slug,
            status: detailAgent.status,
            default_markup: detailAgent.default_markup,
            branding: stringifyAgentBranding({
              site_name: brandSiteName,
              logo: brandLogo,
              home_page_content: brandHomePageContent,
              header_nav_modules: brandHeaderNavModules,
              site_style: serializeThemeCustomization(brandSiteStyleConfig),
            }),
          })
        }
        onSaveHeaderNavigation={async (serialized) => {
          if (!detailAgent) return
          await saveBrandingMutation.mutateAsync({
            id: detailAgent.id,
            owner_user_id: detailAgent.owner_user_id,
            name: detailAgent.name,
            slug: detailAgent.slug,
            status: detailAgent.status,
            default_markup: detailAgent.default_markup,
            branding: stringifyAgentBranding({
              site_name: brandSiteName,
              logo: brandLogo,
              home_page_content: brandHomePageContent,
              header_nav_modules: serialized,
              site_style: brandSiteStyle,
            }),
          })
          setBrandHeaderNavModules(serialized)
        }}
        onCreateDomain={() =>
          selectedAgentId != null &&
          createDomainMutation.mutate({
            agentId: selectedAgentId,
            domain: newDomain.trim(),
          })
        }
        onBindUser={() =>
          selectedAgentId != null &&
          bindUserMutation.mutate({
            agentId: selectedAgentId,
            userId: Number(bindUserId),
          })
        }
        onSavePricing={() =>
          selectedAgentId != null &&
          savePricingMutation.mutate({
            agentId: selectedAgentId,
            group_name: systemGroupName,
            system_group_name: systemGroupName,
            description: groupDescription,
            ratio: Number(groupRatio),
            visible: groupVisible,
          })
        }
        onResetPricing={() => {
          const rule = selectedGroupRatios.find(
            (item) => item.system_group_name === systemGroupName
          )
          if (rule) {
            const draft = getAgentGroupRatioFormDraft(rule)
            setSystemGroupName(draft.systemGroupName)
            setGroupDescription(draft.description)
            setGroupRatio(draft.ratio)
            setGroupVisible(draft.visible)
            return
          }
          setGroupDescription('')
          setGroupRatio(
            getAgentSystemGroupDefaultRatio(
              selectedGroupRatios,
              systemGroupName
            )
          )
          setGroupVisible(true)
        }}
        onSaveUserGroup={() =>
          selectedAgentId != null &&
          saveUserGroupMutation.mutate({
            agentId: selectedAgentId,
            group_name: userGroupName,
            visible_groups: userGroupVisibleGroups,
            group_ratios: userGroupRatios,
          })
        }
        onDomainStatusChange={(domain, status) =>
          updateDomainStatusMutation.mutate({
            agentId: domain.agent_id,
            domainId: domain.id,
            status,
          })
        }
      />
    </>
  )
}

function MetricCard({
  label,
  value,
  icon,
}: {
  label: string
  value: string
  icon: ReactNode
}) {
  return (
    <div className='rounded-lg border p-3'>
      <div className='text-muted-foreground flex items-center gap-2 text-xs'>
        {icon}
        {label}
      </div>
      <div className='mt-2 truncate text-sm font-medium'>{value}</div>
    </div>
  )
}

function CreateAgentDialog(props: {
  open: boolean
  ownerUserId: string
  name: string
  slug: string
  siteName: string
  logo: string
  homePageContent: string
  isPending: boolean
  canSubmit: boolean
  onOpenChange: (open: boolean) => void
  onOwnerUserIdChange: (value: string) => void
  onNameChange: (value: string) => void
  onSlugChange: (value: string) => void
  onSiteNameChange: (value: string) => void
  onLogoChange: (value: string) => void
  onHomePageContentChange: (value: string) => void
  onSubmit: () => void
}) {
  const { t } = useTranslation()

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Create Agent')}</DialogTitle>
          <DialogDescription>
            {t('Create an agent and bind its console owner.')}
          </DialogDescription>
        </DialogHeader>
        <div className='grid gap-3'>
          <Input
            value={props.ownerUserId}
            onChange={(event) => props.onOwnerUserIdChange(event.target.value)}
            type='number'
            min='1'
            placeholder={t('Owner User ID')}
          />
          <Input
            value={props.name}
            onChange={(event) => props.onNameChange(event.target.value)}
            placeholder={t('Agent Name')}
          />
          <Input
            value={props.slug}
            onChange={(event) => props.onSlugChange(event.target.value)}
            placeholder={t('Slug')}
          />
          <Input
            value={props.siteName}
            onChange={(event) => props.onSiteNameChange(event.target.value)}
            placeholder={t('Agent site name')}
          />
          <Input
            value={props.logo}
            onChange={(event) => props.onLogoChange(event.target.value)}
            placeholder={t('Logo URL')}
          />
          <Textarea
            value={props.homePageContent}
            onChange={(event) =>
              props.onHomePageContentChange(event.target.value)
            }
            placeholder={t('Agent home page content (Markdown, HTML, or URL)')}
            className={AGENT_HOME_PAGE_CONTENT_TEXTAREA_CLASS}
          />
        </div>
        <DialogFooter>
          <Button
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={props.isPending}
          >
            {t('Cancel')}
          </Button>
          <Button
            disabled={!props.canSubmit || props.isPending}
            onClick={props.onSubmit}
          >
            <Plus />
            {t('Create Agent')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function AgentDetailDialog(props: {
  agent?: Agent | null
  open: boolean
  domains: AgentDomain[]
  groupRatios: AgentGroupRatio[]
  userGroups: AgentUserGroupConfig[]
  users: AgentUser[]
  usersPage: number
  usersPageSize: number
  usersTotal: number
  usersLoading: boolean
  usersKeyword: string
  brandSiteName: string
  brandLogo: string
  brandHomePageContent: string
  brandHeaderNavConfig: HeaderNavModules
  brandHeaderNavSerialized: string
  brandSiteStyleConfig: ThemeCustomization
  newDomain: string
  bindUserId: string
  systemGroupName: string
  groupDescription: string
  groupRatio: string
  groupRatioFloor: number
  groupVisible: boolean
  userGroupName: string
  userGroupVisibleGroups: string[]
  userGroupRatios: Record<string, number>
  isDomainPending: boolean
  isBindPending: boolean
  isPricingPending: boolean
  isUserGroupPending: boolean
  isDomainStatusPending: boolean
  isBrandingPending: boolean
  canCreateDomain: boolean
  canBindUser: boolean
  canSavePricing: boolean
  canSaveUserGroup: boolean
  onOpenChange: (open: boolean) => void
  onNewDomainChange: (value: string) => void
  onBindUserIdChange: (value: string) => void
  onUsersKeywordChange: (value: string) => void
  onUsersSearch: () => void
  onUsersClear: () => void
  onUsersPageChange: (page: number) => void
  onUsersPageSizeChange: (pageSize: number) => void
  onSystemGroupNameChange: (value: string) => void
  onGroupDescriptionChange: (value: string) => void
  onGroupRatioChange: (value: string) => void
  onGroupVisibleChange: (value: boolean) => void
  onUserGroupNameChange: (value: string) => void
  onUserGroupVisibleGroupsChange: (value: string[]) => void
  onUserGroupRatiosChange: (value: Record<string, number>) => void
  onBrandSiteNameChange: (value: string) => void
  onBrandLogoChange: (value: string) => void
  onBrandHomePageContentChange: (value: string) => void
  onBrandSiteStyleChange: (value: ThemeCustomization) => void
  onSaveBranding: () => void
  onSaveSiteStyle: () => void
  onSaveHeaderNavigation: (serialized: string) => Promise<unknown> | unknown
  onCreateDomain: () => void
  onBindUser: () => void
  onSavePricing: () => void
  onResetPricing: () => void
  onSaveUserGroup: () => void
  onDomainStatusChange: (domain: AgentDomain, status: number) => void
}) {
  const { t } = useTranslation()
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-4xl'>
        <DialogHeader>
          <DialogTitle>{props.agent?.name ?? t('Agent Detail')}</DialogTitle>
          <DialogDescription>
            {props.agent
              ? t(
                  'Agent created. Review its identity, users, domains, and pricing.'
                )
              : t('Loading...')}
          </DialogDescription>
        </DialogHeader>

        {props.agent && (
          <div className='space-y-4'>
            <div className='grid gap-3 md:grid-cols-3'>
              <InfoItem label={t('Agent ID')} value={String(props.agent.id)} />
              <InfoItem
                label={t('Owner User ID')}
                value={String(props.agent.owner_user_id)}
              />
              <InfoItem label={t('Slug')} value={props.agent.slug} />
            </div>

            <section className='rounded-lg border p-3'>
              <div className='mb-3 flex items-center justify-between gap-2'>
                <div>
                  <h3 className='text-sm font-semibold'>
                    {t('Agent Branding')}
                  </h3>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t(
                      'Branding is applied when users visit an active agent domain.'
                    )}
                  </p>
                </div>
                <Button
                  disabled={props.isBrandingPending}
                  onClick={props.onSaveBranding}
                >
                  <Save />
                  {t('Save Branding')}
                </Button>
              </div>
              <div className='grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]'>
                <Input
                  value={props.brandSiteName}
                  onChange={(event) =>
                    props.onBrandSiteNameChange(event.target.value)
                  }
                  placeholder={t('Agent site name')}
                />
                <Input
                  value={props.brandLogo}
                  onChange={(event) =>
                    props.onBrandLogoChange(event.target.value)
                  }
                  placeholder={t('Logo URL')}
                />
              </div>
              <Textarea
                value={props.brandHomePageContent}
                onChange={(event) =>
                  props.onBrandHomePageContentChange(event.target.value)
                }
                placeholder={t(
                  'Agent home page content (Markdown, HTML, or URL)'
                )}
                className={`mt-3 ${AGENT_HOME_PAGE_CONTENT_TEXTAREA_CLASS}`}
              />
            </section>

            <section className='rounded-lg border p-3'>
              <div className='mb-3 flex items-center justify-between gap-2'>
                <div>
                  <h3 className='text-sm font-semibold'>
                    {t('Agent site style')}
                  </h3>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t(
                      'Customize the color, radius, density, and content width for this agent domain.'
                    )}
                  </p>
                </div>
                <Button
                  disabled={props.isBrandingPending}
                  onClick={props.onSaveSiteStyle}
                >
                  <Save />
                  {t('Save Style')}
                </Button>
              </div>
              <ThemeCustomizationEditor
                value={props.brandSiteStyleConfig}
                disabled={props.isBrandingPending}
                onChange={props.onBrandSiteStyleChange}
              />
            </section>

            <section className='rounded-lg border p-3'>
              <HeaderNavigationSection
                title={t('Agent header navigation')}
                description={t(
                  'Control the top navigation shown on this agent domain.'
                )}
                config={props.brandHeaderNavConfig}
                initialSerialized={props.brandHeaderNavSerialized}
                saveLabel={t('Save navigation')}
                saving={props.isBrandingPending}
                onSave={props.onSaveHeaderNavigation}
              />
            </section>

            <div className='grid gap-4 xl:grid-cols-2'>
              <section className='rounded-lg border p-3'>
                <div className='mb-3 grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]'>
                  <div>
                    <h3 className='text-sm font-semibold'>
                      {t('Agent Domains')}
                    </h3>
                    <p className='text-muted-foreground mt-1 text-xs'>
                      {t('Domains are configured by admins only.')}
                    </p>
                  </div>
                  <div className='flex gap-2'>
                    <Input
                      value={props.newDomain}
                      onChange={(event) =>
                        props.onNewDomainChange(event.target.value)
                      }
                      placeholder={t('agent.example.com')}
                    />
                    <Button
                      disabled={!props.canCreateDomain || props.isDomainPending}
                      onClick={props.onCreateDomain}
                    >
                      <Plus />
                      {t('Add Domain')}
                    </Button>
                  </div>
                </div>
                <AgentUsersSearchControls
                  keyword={props.usersKeyword}
                  isLoading={props.usersLoading}
                  onKeywordChange={props.onUsersKeywordChange}
                  onSearch={props.onUsersSearch}
                  onClear={props.onUsersClear}
                />
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Domain')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead className='text-right'>
                        {t('Actions')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {props.domains.length === 0 ? (
                      <TableEmpty
                        colSpan={3}
                        title={t('No Domains')}
                        description={t(
                          'Add a domain before using this agent site.'
                        )}
                        icon={<Globe2 className='size-6' />}
                      />
                    ) : (
                      props.domains.map((domain) => (
                        <TableRow key={domain.id}>
                          <TableCell className='max-w-[220px] truncate font-medium'>
                            {domain.domain}
                          </TableCell>
                          <TableCell>
                            <Badge variant={domainStatusVariant(domain.status)}>
                              {t(domainStatusLabel(domain.status))}
                            </Badge>
                          </TableCell>
                          <TableCell className='text-right'>
                            <DomainActions
                              compact
                              disabled={props.isDomainStatusPending}
                              status={domain.status}
                              onApprove={() =>
                                props.onDomainStatusChange(
                                  domain,
                                  AGENT_DOMAIN_STATUS_ACTIVE
                                )
                              }
                              onDisable={() =>
                                props.onDomainStatusChange(
                                  domain,
                                  AGENT_DOMAIN_STATUS_DISABLED
                                )
                              }
                            />
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </section>

              <section className='rounded-lg border p-3'>
                <div className='mb-3 grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]'>
                  <div>
                    <h3 className='text-sm font-semibold'>
                      {t('Agent Users')}
                    </h3>
                    <p className='text-muted-foreground mt-1 text-xs'>
                      {t('Only bound users can access the agent console.')}
                    </p>
                  </div>
                  <div className='flex gap-2'>
                    <Input
                      value={props.bindUserId}
                      onChange={(event) =>
                        props.onBindUserIdChange(event.target.value)
                      }
                      type='number'
                      min='1'
                      placeholder={t('User ID')}
                    />
                    <Button
                      disabled={!props.canBindUser || props.isBindPending}
                      onClick={props.onBindUser}
                    >
                      <UserPlus />
                      {t('Bind User')}
                    </Button>
                  </div>
                </div>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Username')}</TableHead>
                      <TableHead>{t('Source')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Group')}</TableHead>
                      <TableHead>{t('Quota')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {props.users.length === 0 ? (
                      <TableEmpty
                        colSpan={5}
                        title={t('No Agent Users')}
                        description={t(
                          'Bind users to open the agent console for them.'
                        )}
                        icon={<Users className='size-6' />}
                      />
                    ) : (
                      props.users.map((user) => (
                        <TableRow key={user.id}>
                          <TableCell>
                            <div className='flex min-w-[160px] flex-col gap-1'>
                              <LongText className='max-w-[150px] font-medium'>
                                {user.username || `#${user.user_id}`}
                              </LongText>
                              <span className='text-muted-foreground text-xs'>
                                #{user.user_id}
                              </span>
                            </div>
                          </TableCell>
                          <TableCell>{user.source}</TableCell>
                          <TableCell>
                            <Badge variant='outline'>
                              {t(domainStatusLabel(user.agent_user_status))}
                            </Badge>
                          </TableCell>
                          <TableCell>
                            <GroupBadge group={user.group} />
                          </TableCell>
                          <TableCell>{user.quota.toLocaleString()}</TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
                <AgentUsersPaginationControls
                  page={props.usersPage}
                  pageSize={props.usersPageSize}
                  total={props.usersTotal}
                  itemCount={props.users.length}
                  isLoading={props.usersLoading}
                  onPageChange={props.onUsersPageChange}
                  onPageSizeChange={props.onUsersPageSizeChange}
                />
              </section>
            </div>

            <div className='flex flex-col gap-3'>
              <div>
                <h3 className='text-sm font-semibold'>{t('Agent Pricing')}</h3>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t(
                    'Configure system group rules and user group overrides in one place.'
                  )}
                </p>
              </div>
              <div className='flex flex-col gap-4'>
                <AgentUserGroupManager
                  userGroups={props.userGroups}
                  groupRatios={props.groupRatios}
                  groupName={props.userGroupName}
                  visibleGroups={props.userGroupVisibleGroups}
                  groupRatioOverrides={props.userGroupRatios}
                  canSave={props.canSaveUserGroup}
                  isPending={props.isUserGroupPending}
                  onGroupNameChange={props.onUserGroupNameChange}
                  onVisibleGroupsChange={props.onUserGroupVisibleGroupsChange}
                  onGroupRatioOverridesChange={props.onUserGroupRatiosChange}
                  onSave={props.onSaveUserGroup}
                  onEdit={(rule) => {
                    const draft = getAgentUserGroupFormDraft(rule)
                    props.onUserGroupNameChange(draft.groupName)
                    props.onUserGroupVisibleGroupsChange(draft.visibleGroups)
                    props.onUserGroupRatiosChange(draft.groupRatios)
                  }}
                  onResetForm={() => {
                    const draft = getAgentUserGroupFormDraft()
                    props.onUserGroupNameChange(draft.groupName)
                    props.onUserGroupVisibleGroupsChange(draft.visibleGroups)
                    props.onUserGroupRatiosChange(draft.groupRatios)
                  }}
                />

                <AgentGroupManager
                  groupRatios={props.groupRatios}
                  systemGroupName={props.systemGroupName}
                  groupDescription={props.groupDescription}
                  groupRatio={props.groupRatio}
                  groupRatioFloor={props.groupRatioFloor}
                  groupVisible={props.groupVisible}
                  canSave={props.canSavePricing}
                  isPending={props.isPricingPending}
                  onSystemGroupNameChange={props.onSystemGroupNameChange}
                  onGroupDescriptionChange={props.onGroupDescriptionChange}
                  onGroupRatioChange={props.onGroupRatioChange}
                  onGroupVisibleChange={props.onGroupVisibleChange}
                  onSave={props.onSavePricing}
                  onEdit={(rule) => {
                    const draft = getAgentGroupRatioFormDraft(rule)
                    props.onSystemGroupNameChange(draft.systemGroupName)
                    props.onGroupDescriptionChange(draft.description)
                    props.onGroupRatioChange(draft.ratio)
                    props.onGroupVisibleChange(draft.visible)
                  }}
                  onResetForm={props.onResetPricing}
                />
              </div>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

function InfoItem({ label, value }: { label: string; value: string }) {
  return (
    <div className='rounded-lg border p-3'>
      <div className='text-muted-foreground text-xs'>{label}</div>
      <div className='mt-1 truncate text-sm font-medium'>{value}</div>
    </div>
  )
}

function EmptySelection({ message }: { message: string }) {
  return (
    <div className='text-muted-foreground flex min-h-40 items-center justify-center rounded-lg border border-dashed p-6 text-center text-sm'>
      {message}
    </div>
  )
}

function LoadingRow({ colSpan }: { colSpan: number }) {
  const { t } = useTranslation()
  return (
    <TableRow>
      <TableCell
        colSpan={colSpan}
        className='text-muted-foreground h-24 text-center text-sm'
      >
        {t('Loading...')}
      </TableCell>
    </TableRow>
  )
}

function DomainActions(props: {
  status: number
  disabled?: boolean
  compact?: boolean
  onApprove: () => void
  onDisable: () => void
}) {
  const { t } = useTranslation()
  const approveDisabled =
    props.disabled || props.status === AGENT_DOMAIN_STATUS_ACTIVE
  const disableDisabled =
    props.disabled || props.status === AGENT_DOMAIN_STATUS_DISABLED

  return (
    <div className='flex justify-end gap-2'>
      <Button
        size={props.compact ? 'icon-sm' : 'sm'}
        variant='outline'
        disabled={approveDisabled}
        title={t('Approve')}
        onClick={props.onApprove}
      >
        <Check />
        {!props.compact && t('Approve')}
      </Button>
      <Button
        size={props.compact ? 'icon-sm' : 'sm'}
        variant='outline'
        disabled={disableDisabled}
        title={t('Disable')}
        onClick={props.onDisable}
      >
        <X />
        {!props.compact && t('Disable')}
      </Button>
    </div>
  )
}

function WithdrawalActions(props: {
  withdrawal: AgentWithdrawal
  disabled?: boolean
  onUpdate: (status: string) => void
}) {
  const { t } = useTranslation()
  const isPending = props.withdrawal.status === 'pending'
  const isApproved = props.withdrawal.status === 'approved'

  return (
    <div className='inline-flex justify-end gap-2'>
      <Button
        size='sm'
        variant='outline'
        disabled={props.disabled || !isPending}
        onClick={() => props.onUpdate('approved')}
      >
        <Check />
        {t('Approve')}
      </Button>
      <Button
        size='sm'
        disabled={props.disabled || !isApproved}
        onClick={() => props.onUpdate('paid')}
      >
        {t('Mark Paid')}
      </Button>
      <Button
        size='sm'
        variant='outline'
        disabled={props.disabled || (!isPending && !isApproved)}
        onClick={() => props.onUpdate('rejected')}
      >
        <X />
        {t('Reject')}
      </Button>
    </div>
  )
}
