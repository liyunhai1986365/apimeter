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
import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  BadgeDollarSign,
  Check,
  CircleDollarSign,
  Globe2,
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
import { GroupBadge } from '@/components/group-badge'
import { LongText } from '@/components/long-text'
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
import { Switch } from '@/components/ui/switch'
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
import { SectionPageLayout } from '@/components/layout'
import {
  bindAdminAgentUser,
  completeAdminAgentWithdrawal,
  createAdminAgent,
  createAdminAgentDomain,
  listAdminAgentDomainsByAgent,
  listAdminAgentPricingRules,
  listAdminAgents,
  listAdminAgentUsers,
  listAdminAgentWithdrawals,
  parseAgentBranding,
  stringifyAgentBranding,
  updateAdminAgent,
  updateAdminAgentDomainStatus,
  upsertAdminAgentPricingRule,
} from './api'
import type { Agent, AgentDomain, AgentUser, AgentWithdrawal } from './types'

const AGENT_DOMAIN_STATUS_ACTIVE = 1
const AGENT_DOMAIN_STATUS_DISABLED = 2
const AGENT_STATUS_ENABLED = 1

const formatQuota = (quota?: number) => (quota ?? 0).toLocaleString()

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
  const queryClient = useQueryClient()
  const [selectedAgentId, setSelectedAgentId] = useState<number | null>(null)
  const [detailAgentId, setDetailAgentId] = useState<number | null>(null)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [createdAgent, setCreatedAgent] = useState<Agent | null>(null)
  const [newAgentOwnerId, setNewAgentOwnerId] = useState('')
  const [newAgentName, setNewAgentName] = useState('')
  const [newAgentSlug, setNewAgentSlug] = useState('')
  const [newAgentMarkup, setNewAgentMarkup] = useState('1')
  const [newAgentSiteName, setNewAgentSiteName] = useState('')
  const [newAgentLogo, setNewAgentLogo] = useState('')
  const [brandSiteName, setBrandSiteName] = useState('')
  const [brandLogo, setBrandLogo] = useState('')
  const [newDomain, setNewDomain] = useState('')
  const [bindUserId, setBindUserId] = useState('')
  const [ruleModelPattern, setRuleModelPattern] = useState('*')
  const [ruleMarkup, setRuleMarkup] = useState('1.2')
  const [ruleEnabled, setRuleEnabled] = useState(true)
  const [withdrawalRemark, setWithdrawalRemark] = useState('')

  const agentsQuery = useQuery({
    queryKey: ['admin', 'agents'],
    queryFn: () => listAdminAgents(1, 50),
  })

  const selectedUsersQuery = useQuery({
    queryKey: ['admin', 'agents', selectedAgentId, 'users'],
    queryFn: () => listAdminAgentUsers(selectedAgentId ?? 0, 1, 50),
    enabled: selectedAgentId != null,
  })

  const selectedDomainsQuery = useQuery({
    queryKey: ['admin', 'agents', selectedAgentId, 'domains'],
    queryFn: () => listAdminAgentDomainsByAgent(selectedAgentId ?? 0, 1, 50),
    enabled: selectedAgentId != null,
  })

  const selectedPricingQuery = useQuery({
    queryKey: ['admin', 'agents', selectedAgentId, 'pricing-rules'],
    queryFn: () => listAdminAgentPricingRules(selectedAgentId ?? 0, 1, 50),
    enabled: selectedAgentId != null,
  })

  const withdrawalsQuery = useQuery({
    queryKey: ['admin', 'agents', 'withdrawals'],
    queryFn: () => listAdminAgentWithdrawals(undefined, 1, 50),
  })

  const agents = agentsQuery.data?.data.items ?? []
  const selectedAgent = useMemo(
    () => agents.find((agent) => agent.id === selectedAgentId),
    [agents, selectedAgentId]
  )
  const detailAgent = useMemo(
    () => agents.find((agent) => agent.id === detailAgentId) ?? createdAgent,
    [agents, detailAgentId, createdAgent]
  )
  const selectedUsers = selectedUsersQuery.data?.data.items ?? []
  const selectedDomains = selectedDomainsQuery.data?.data.items ?? []
  const selectedPricingRules = selectedPricingQuery.data?.data.items ?? []
  const withdrawals = withdrawalsQuery.data?.data.items ?? []
  const pendingWithdrawalCount = withdrawals.filter(
    (withdrawal) => withdrawal.status === 'pending'
  ).length
  const totalAgentUsers = selectedUsersQuery.data?.data.total
  const openAgentDetail = (agent: Agent) => {
    const branding = parseAgentBranding(agent.branding)
    setBrandSiteName(branding.site_name ?? '')
    setBrandLogo(branding.logo ?? '')
    setSelectedAgentId(agent.id)
    setDetailAgentId(agent.id)
  }

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
      setNewAgentMarkup('1')
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
    mutationFn: upsertAdminAgentPricingRule,
    onSuccess: () => {
      toast.success(t('Pricing rule saved'))
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

  const canCreateAgent =
    Number(newAgentOwnerId) > 0 &&
    newAgentName.trim() !== '' &&
    newAgentSlug.trim() !== ''
  const canCreateDomain = selectedAgentId != null && newDomain.trim() !== ''
  const canBindUser = selectedAgentId != null && Number(bindUserId) > 0
  const canSavePricing =
    selectedAgentId != null &&
    ruleModelPattern.trim() !== '' &&
    Number(ruleMarkup) > 0

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
                value={formatQuota(agentsQuery.data?.data.total)}
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
                        <TableHead>{t('Markup')}</TableHead>
                        <TableHead>{t('Created At')}</TableHead>
                        <TableHead className='text-right'>
                          {t('Actions')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {agentsQuery.isLoading ? (
                        <LoadingRow colSpan={7} />
                      ) : agents.length === 0 ? (
                        <TableEmpty
                          colSpan={7}
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
                            <TableCell>{agent.default_markup}</TableCell>
                            <TableCell>
                              {formatTimestampToDate(agent.created_at)}
                            </TableCell>
                            <TableCell className='text-right'>
                              <div className='inline-flex gap-2'>
                                <Button
                                  size='sm'
                                  variant='outline'
                                  onClick={() => setSelectedAgentId(agent.id)}
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
                                  {t(domainStatusLabel(user.agent_user_status))}
                                </Badge>
                              </TableCell>
                              <TableCell>
                                <GroupBadge group={user.group} />
                              </TableCell>
                              <TableCell>{user.quota.toLocaleString()}</TableCell>
                              <TableCell>
                                {formatTimestampToDate(user.created_at)}
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
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
                        <TableHead>{t('Quota Amount')}</TableHead>
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
                        <LoadingRow colSpan={6} />
                      ) : withdrawals.length === 0 ? (
                        <TableEmpty
                          colSpan={6}
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
                              {formatQuota(withdrawal.amount_quota)}
                            </TableCell>
                            <TableCell>{withdrawal.amount_money}</TableCell>
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
        markup={newAgentMarkup}
        siteName={newAgentSiteName}
        logo={newAgentLogo}
        isPending={createAgentMutation.isPending}
        canSubmit={canCreateAgent}
        onOwnerUserIdChange={setNewAgentOwnerId}
        onNameChange={setNewAgentName}
        onSlugChange={setNewAgentSlug}
        onMarkupChange={setNewAgentMarkup}
        onSiteNameChange={setNewAgentSiteName}
        onLogoChange={setNewAgentLogo}
        onSubmit={() =>
          createAgentMutation.mutate({
            owner_user_id: Number(newAgentOwnerId),
            name: newAgentName.trim(),
            slug: newAgentSlug.trim(),
            status: AGENT_STATUS_ENABLED,
            default_markup: Number(newAgentMarkup) || 1,
            branding: stringifyAgentBranding({
              site_name: newAgentSiteName,
              logo: newAgentLogo,
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
        pricingRules={selectedPricingRules}
        users={selectedUsers}
        brandSiteName={brandSiteName}
        brandLogo={brandLogo}
        newDomain={newDomain}
        bindUserId={bindUserId}
        ruleModelPattern={ruleModelPattern}
        ruleMarkup={ruleMarkup}
        ruleEnabled={ruleEnabled}
        isDomainPending={createDomainMutation.isPending}
        isBindPending={bindUserMutation.isPending}
        isPricingPending={savePricingMutation.isPending}
        isDomainStatusPending={updateDomainStatusMutation.isPending}
        canCreateDomain={canCreateDomain}
        canBindUser={canBindUser}
        canSavePricing={canSavePricing}
        onNewDomainChange={setNewDomain}
        onBindUserIdChange={setBindUserId}
        onRuleModelPatternChange={setRuleModelPattern}
        onRuleMarkupChange={setRuleMarkup}
        onRuleEnabledChange={setRuleEnabled}
        onBrandSiteNameChange={setBrandSiteName}
        onBrandLogoChange={setBrandLogo}
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
            }),
          })
        }
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
            model_pattern: ruleModelPattern.trim(),
            markup: Number(ruleMarkup),
            enabled: ruleEnabled,
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
  markup: string
  siteName: string
  logo: string
  isPending: boolean
  canSubmit: boolean
  onOpenChange: (open: boolean) => void
  onOwnerUserIdChange: (value: string) => void
  onNameChange: (value: string) => void
  onSlugChange: (value: string) => void
  onMarkupChange: (value: string) => void
  onSiteNameChange: (value: string) => void
  onLogoChange: (value: string) => void
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
            value={props.markup}
            onChange={(event) => props.onMarkupChange(event.target.value)}
            type='number'
            min='0.01'
            step='0.01'
            placeholder={t('Markup')}
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
  pricingRules: Array<{
    id: number
    model_pattern: string
    markup: number
    enabled: boolean
  }>
  users: AgentUser[]
  brandSiteName: string
  brandLogo: string
  newDomain: string
  bindUserId: string
  ruleModelPattern: string
  ruleMarkup: string
  ruleEnabled: boolean
  isDomainPending: boolean
  isBindPending: boolean
  isPricingPending: boolean
  isDomainStatusPending: boolean
  isBrandingPending: boolean
  canCreateDomain: boolean
  canBindUser: boolean
  canSavePricing: boolean
  onOpenChange: (open: boolean) => void
  onNewDomainChange: (value: string) => void
  onBindUserIdChange: (value: string) => void
  onRuleModelPatternChange: (value: string) => void
  onRuleMarkupChange: (value: string) => void
  onRuleEnabledChange: (value: boolean) => void
  onBrandSiteNameChange: (value: string) => void
  onBrandLogoChange: (value: string) => void
  onSaveBranding: () => void
  onCreateDomain: () => void
  onBindUser: () => void
  onSavePricing: () => void
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
            <div className='grid gap-3 md:grid-cols-4'>
              <InfoItem label={t('Agent ID')} value={String(props.agent.id)} />
              <InfoItem
                label={t('Owner User ID')}
                value={String(props.agent.owner_user_id)}
              />
              <InfoItem label={t('Slug')} value={props.agent.slug} />
              <InfoItem
                label={t('Markup')}
                value={String(props.agent.default_markup)}
              />
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
              </section>
            </div>

            <section className='rounded-lg border p-3'>
              <div className='mb-3 grid gap-2 lg:grid-cols-[minmax(0,1fr)_minmax(360px,520px)]'>
                <div>
                  <h3 className='text-sm font-semibold'>
                    {t('Pricing Rules')}
                  </h3>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t('Configure agent markup by model pattern.')}
                  </p>
                </div>
                <div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_96px_auto_auto]'>
                  <Input
                    value={props.ruleModelPattern}
                    onChange={(event) =>
                      props.onRuleModelPatternChange(event.target.value)
                    }
                    placeholder={t('Model')}
                  />
                  <Input
                    value={props.ruleMarkup}
                    onChange={(event) =>
                      props.onRuleMarkupChange(event.target.value)
                    }
                    type='number'
                    min='0.01'
                    step='0.01'
                    placeholder={t('Markup')}
                  />
                  <div className='flex h-8 items-center gap-2 px-1'>
                    <Switch
                      checked={props.ruleEnabled}
                      onCheckedChange={props.onRuleEnabledChange}
                    />
                    <span className='text-muted-foreground text-xs'>
                      {t('Enabled')}
                    </span>
                  </div>
                  <Button
                    disabled={!props.canSavePricing || props.isPricingPending}
                    onClick={props.onSavePricing}
                  >
                    <Save />
                    {t('Save')}
                  </Button>
                </div>
              </div>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Model')}</TableHead>
                    <TableHead>{t('Markup')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {props.pricingRules.length === 0 ? (
                    <TableEmpty
                      colSpan={3}
                      title={t('No Pricing Rules')}
                      description={t(
                        'Default markup is used when no rule matches.'
                      )}
                      icon={<BadgeDollarSign className='size-6' />}
                    />
                  ) : (
                    props.pricingRules.map((rule) => (
                      <TableRow key={rule.id}>
                        <TableCell className='font-mono text-xs'>
                          {rule.model_pattern}
                        </TableCell>
                        <TableCell>{rule.markup}</TableCell>
                        <TableCell>
                          <Badge variant={rule.enabled ? 'default' : 'outline'}>
                            {rule.enabled ? t('Enabled') : t('Disabled')}
                          </Badge>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </section>
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
