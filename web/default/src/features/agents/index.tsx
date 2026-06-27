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
import type { ReactNode } from 'react'
import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  BadgeDollarSign,
  CheckCircle2,
  Copy,
  Globe2,
  RefreshCcw,
  Save,
  Store,
  Users,
  Wallet,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  formatQuota as formatDisplayQuota,
  formatTimestampToDate,
} from '@/lib/format'
import {
  parseHeaderNavModules,
  serializeHeaderNavModules,
} from '@/lib/nav-modules'
import { normalizePagedData } from '@/lib/paged-response'
import {
  parseThemeCustomization,
  serializeThemeCustomization,
  type ThemeCustomization,
} from '@/lib/theme-customization'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
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
import { StatusBadge } from '@/components/status-badge'
import { ThemeCustomizationEditor } from '@/components/theme-customization-editor'
import { HeaderNavigationSection } from '@/features/system-settings/maintenance/header-navigation-section'
import { USER_STATUS, USER_STATUSES } from '@/features/users/constants'
import {
  createAgentDomain,
  buildAgentUserGroupOptions,
  getAgentSelf,
  listAgentDomains,
  listAgentGroupRatios,
  listAgentLedger,
  listAgentUsers,
  listAgentUserGroups,
  listAgentWithdrawals,
  parseAgentBranding,
  stringifyAgentBranding,
  submitAgentWithdrawal,
  updateAgentBranding,
  updateAgentUserGroup,
  updateAgentUserStatus,
  upsertAgentGroupRatio,
  upsertAgentUserGroup,
  verifyAgentDomain,
} from './api'
import { AgentGroupManager } from './components/agent-group-manager'
import { AgentUserGroupManager } from './components/agent-user-group-manager'
import {
  AgentUsersPaginationControls,
  AgentUsersSearchControls,
} from './components/agent-users-list-controls'
import { AGENT_HOME_PAGE_CONTENT_TEXTAREA_CLASS } from './constants'
import type {
  AgentDomain,
  AgentGroupRatio,
  AgentLedger,
  AgentUser,
  AgentUserGroupConfig,
  AgentWithdrawal,
} from './types'

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

function getQuotaProgressColor(percentage: number): string {
  if (percentage <= 10) return '[&_[data-slot=progress-indicator]]:bg-rose-500'
  if (percentage <= 30) return '[&_[data-slot=progress-indicator]]:bg-amber-500'
  return '[&_[data-slot=progress-indicator]]:bg-emerald-500'
}

function AgentUserStatusBadge({ status }: { status: number }) {
  const { t } = useTranslation()
  const config = USER_STATUSES[status as keyof typeof USER_STATUSES]
  if (!config) {
    return <Badge variant='outline'>{status}</Badge>
  }
  return (
    <StatusBadge
      label={t(config.labelKey)}
      variant={config.variant}
      showDot={config.showDot}
      copyable={false}
    />
  )
}

function UserQuotaCell(props: { quota: number; usedQuota: number }) {
  const { t } = useTranslation()
  const total = props.quota + props.usedQuota
  const percentage = total > 0 ? (props.quota / total) * 100 : 0

  if (total === 0) {
    return (
      <StatusBadge label={t('No Quota')} variant='neutral' copyable={false} />
    )
  }

  return (
    <div className='w-[150px] space-y-1'>
      <div className='flex justify-between text-xs'>
        <span className='font-medium tabular-nums'>
          {formatDisplayQuota(props.quota)}
        </span>
        <span className='text-muted-foreground tabular-nums'>
          {formatDisplayQuota(total)}
        </span>
      </div>
      <Progress
        value={percentage}
        className={cn('h-1.5', getQuotaProgressColor(percentage))}
      />
      <div className='text-muted-foreground flex justify-between text-[11px]'>
        <span>
          {t('Used:')} {formatDisplayQuota(props.usedQuota)}
        </span>
        <span>{percentage.toFixed(1)}%</span>
      </div>
    </div>
  )
}

function statusLabel(status: number) {
  if (status === 1) return 'Active'
  if (status === 2) return 'Disabled'
  return 'Pending'
}

function withdrawalVariant(
  status: string
): 'default' | 'outline' | 'secondary' | 'destructive' {
  if (status === 'paid') return 'default'
  if (status === 'approved') return 'secondary'
  if (status === 'rejected') return 'destructive'
  return 'outline'
}

export function Agents() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [siteName, setSiteName] = useState('')
  const [logo, setLogo] = useState('')
  const [homePageContent, setHomePageContent] = useState('')
  const [headerNavModules, setHeaderNavModules] = useState('')
  const [siteStyle, setSiteStyle] = useState('')
  const [newDomain, setNewDomain] = useState('')
  const [groupName, setGroupName] = useState('default')
  const [groupRatio, setGroupRatio] = useState('1')
  const [withdrawMoney, setWithdrawMoney] = useState('')
  const [accountInfo, setAccountInfo] = useState('')
  const [systemGroupName, setSystemGroupName] = useState('default')
  const [groupDescription, setGroupDescription] = useState('')
  const [groupVisible, setGroupVisible] = useState(true)
  const [userGroupName, setUserGroupName] = useState('')
  const [userGroupVisibleGroups, setUserGroupVisibleGroups] = useState<
    string[]
  >([])
  const [userKeywordInput, setUserKeywordInput] = useState('')
  const [userKeyword, setUserKeyword] = useState('')
  const [userPage, setUserPage] = useState(1)
  const [userPageSize, setUserPageSize] = useState(20)

  const selfQuery = useQuery({
    queryKey: ['agent', 'self'],
    queryFn: getAgentSelf,
  })
  const domainsQuery = useQuery({
    queryKey: ['agent', 'domains'],
    queryFn: () => listAgentDomains(),
  })
  const groupRatiosQuery = useQuery({
    queryKey: ['agent', 'group-ratios'],
    queryFn: () => listAgentGroupRatios(),
  })
  const userGroupsQuery = useQuery({
    queryKey: ['agent', 'user-groups'],
    queryFn: () => listAgentUserGroups(),
  })
  const usersQuery = useQuery({
    queryKey: ['agent', 'users', userPage, userPageSize, userKeyword],
    queryFn: () => listAgentUsers(userPage, userPageSize, userKeyword),
  })
  const ledgerQuery = useQuery({
    queryKey: ['agent', 'ledger'],
    queryFn: () => listAgentLedger(),
  })
  const withdrawalsQuery = useQuery({
    queryKey: ['agent', 'withdrawals'],
    queryFn: () => listAgentWithdrawals(),
  })

  const refreshAgent = () => {
    queryClient.invalidateQueries({ queryKey: ['agent'] })
  }

  const saveBrandingMutation = useMutation({
    mutationFn: updateAgentBranding,
    onSuccess: () => {
      toast.success(t('Branding saved'))
      refreshAgent()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const createDomainMutation = useMutation({
    mutationFn: createAgentDomain,
    onSuccess: () => {
      toast.success(t('Domain added'))
      setNewDomain('')
      refreshAgent()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const verifyDomainMutation = useMutation({
    mutationFn: verifyAgentDomain,
    onSuccess: () => {
      toast.success(t('Domain verified'))
      refreshAgent()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const saveGroupRatioMutation = useMutation({
    mutationFn: upsertAgentGroupRatio,
    onSuccess: () => {
      toast.success(t('Group ratio saved'))
      refreshAgent()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const saveUserGroupMutation = useMutation({
    mutationFn: upsertAgentUserGroup,
    onSuccess: () => {
      toast.success(t('User group saved'))
      queryClient.invalidateQueries({ queryKey: ['agent', 'user-groups'] })
      queryClient.invalidateQueries({ queryKey: ['agent', 'users'] })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const withdrawMutation = useMutation({
    mutationFn: submitAgentWithdrawal,
    onSuccess: () => {
      toast.success(t('Withdrawal submitted'))
      setWithdrawMoney('')
      setAccountInfo('')
      refreshAgent()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const updateUserStatusMutation = useMutation({
    mutationFn: updateAgentUserStatus,
    onSuccess: () => {
      toast.success(t('User status updated'))
      queryClient.invalidateQueries({ queryKey: ['agent', 'users'] })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const updateUserGroupMutation = useMutation({
    mutationFn: updateAgentUserGroup,
    onSuccess: () => {
      toast.success(t('User group updated'))
      queryClient.invalidateQueries({ queryKey: ['agent', 'users'] })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const self = selfQuery.data?.data
  const balance = self?.balance
  const agentDomain = self?.context?.Domain || self?.agent?.slug || '-'
  const usersPage = useMemo(
    () => normalizePagedData<AgentUser>(usersQuery.data),
    [usersQuery.data]
  )
  const domainsPage = useMemo(
    () => normalizePagedData<AgentDomain>(domainsQuery.data),
    [domainsQuery.data]
  )
  const ledgerPage = useMemo(
    () => normalizePagedData<AgentLedger>(ledgerQuery.data),
    [ledgerQuery.data]
  )
  const withdrawalsPage = useMemo(
    () => normalizePagedData<AgentWithdrawal>(withdrawalsQuery.data),
    [withdrawalsQuery.data]
  )
  const users = usersPage.items
  const domains = domainsPage.items
  const groupRatios = groupRatiosQuery.data?.data ?? []
  const userGroups = userGroupsQuery.data?.data ?? []
  const userGroupOptions = buildAgentUserGroupOptions(userGroups)
  const ledger = ledgerPage.items
  const withdrawals = withdrawalsPage.items
  const canCreateDomain = newDomain.trim() !== ''
  const selectedGroupBaseRatio =
    groupRatios.find((item) => item.system_group_name === systemGroupName)
      ?.system_ratio ?? 0
  const canSaveGroupRatio =
    groupName.trim() !== '' &&
    systemGroupName.trim() !== '' &&
    Number(groupRatio) >= selectedGroupBaseRatio
  const canSaveUserGroup = userGroupName.trim() !== ''
  const canSubmitWithdrawal =
    Number(withdrawMoney) > 0 && accountInfo.trim() !== ''
  const editAgentGroup = (rule: AgentGroupRatio) => {
    setGroupName(rule.group_name)
    setSystemGroupName(rule.system_group_name)
    setGroupDescription(rule.description ?? '')
    setGroupRatio(String(rule.effective_ratio || rule.system_ratio || 1))
    setGroupVisible(rule.visible)
  }
  const editAgentUserGroup = (rule: AgentUserGroupConfig) => {
    setUserGroupName(rule.group_name)
    setUserGroupVisibleGroups(rule.visible_groups ?? [])
  }
  const searchAgentUsers = () => {
    setUserPage(1)
    setUserKeyword(userKeywordInput.trim())
  }
  const clearAgentUserSearch = () => {
    setUserKeywordInput('')
    setUserPage(1)
    setUserKeyword('')
  }
  const headerNavConfig = useMemo(
    () => parseHeaderNavModules(headerNavModules),
    [headerNavModules]
  )
  const headerNavSerialized = useMemo(
    () => serializeHeaderNavModules(headerNavConfig),
    [headerNavConfig]
  )
  const siteStyleConfig = useMemo(
    () => parseThemeCustomization(siteStyle),
    [siteStyle]
  )

  useEffect(() => {
    if (!self?.agent) return
    const branding = parseAgentBranding(self.agent.branding)
    setSiteName(branding.site_name ?? '')
    setLogo(branding.logo ?? '')
    setHomePageContent(branding.home_page_content ?? '')
    setHeaderNavModules(branding.header_nav_modules ?? '')
    setSiteStyle(branding.site_style ?? '')
  }, [self?.agent?.branding])

  const copyText = async (text?: string) => {
    if (!text) {
      toast.error(t('No CNAME target configured'))
      return
    }
    try {
      await navigator.clipboard.writeText(text)
      toast.success(t('Copied'))
    } catch {
      toast.error(t('Operation failed'))
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Agent Console')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button variant='outline' onClick={refreshAgent}>
          <RefreshCcw />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <div className='grid gap-3 md:grid-cols-4'>
            <MetricCard
              label={t('Agent Name')}
              value={self?.agent.name || '-'}
              icon={<Store className='size-4' />}
            />
            <MetricCard
              label={t('Agent Domain')}
              value={agentDomain}
              icon={<Globe2 className='size-4' />}
            />
            <MetricCard
              label={t('Available Balance')}
              value={formatSettlementAmount(
                balance?.available_amount,
                balance?.currency
              )}
              icon={<BadgeDollarSign className='size-4' />}
            />
            <MetricCard
              label={t('Pending Withdrawal')}
              value={formatSettlementAmount(
                balance?.pending_withdrawal_amount,
                balance?.currency
              )}
              icon={<Wallet className='size-4' />}
            />
          </div>

          <Tabs defaultValue='overview'>
            <TabsList>
              <TabsTrigger value='overview'>
                <Store className='size-4' />
                {t('Overview')}
              </TabsTrigger>
              <TabsTrigger value='users'>
                <Users className='size-4' />
                {t('Users')}
              </TabsTrigger>
              <TabsTrigger value='pricing'>
                <BadgeDollarSign className='size-4' />
                {t('Pricing')}
              </TabsTrigger>
              <TabsTrigger value='settlement'>
                <Wallet className='size-4' />
                {t('Settlement')}
              </TabsTrigger>
            </TabsList>

            <TabsContent value='overview'>
              <div className='grid gap-4 xl:grid-cols-[minmax(0,0.85fr)_minmax(0,1.15fr)]'>
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
                      disabled={saveBrandingMutation.isPending}
                      onClick={() =>
                        saveBrandingMutation.mutate({
                          branding: stringifyAgentBranding({
                            site_name: siteName,
                            logo,
                            home_page_content: homePageContent,
                            header_nav_modules: headerNavModules,
                            site_style: siteStyle,
                          }),
                        })
                      }
                    >
                      <Save />
                      {t('Save Branding')}
                    </Button>
                  </div>
                  <div className='mb-3 grid gap-2'>
                    <Input
                      value={siteName}
                      onChange={(event) => setSiteName(event.target.value)}
                      placeholder={t('Agent site name')}
                    />
                    <Input
                      value={logo}
                      onChange={(event) => setLogo(event.target.value)}
                      placeholder={t('Logo URL')}
                    />
                    <Textarea
                      value={homePageContent}
                      onChange={(event) =>
                        setHomePageContent(event.target.value)
                      }
                      placeholder={t(
                        'Agent home page content (Markdown, HTML, or URL)'
                      )}
                      className={AGENT_HOME_PAGE_CONTENT_TEXTAREA_CLASS}
                    />
                  </div>
                  <div className='grid gap-3 sm:grid-cols-2'>
                    <InfoItem
                      label={t('Owner User ID')}
                      value={String(self?.agent.owner_user_id ?? '-')}
                    />
                    <InfoItem
                      label={t('Slug')}
                      value={self?.agent.slug ?? '-'}
                    />
                    <InfoItem
                      label={t('Settlement Currency')}
                      value={self?.agent.settlement_currency ?? '-'}
                    />
                  </div>
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
                      disabled={saveBrandingMutation.isPending}
                      onClick={() =>
                        saveBrandingMutation.mutate({
                          branding: stringifyAgentBranding({
                            site_name: siteName,
                            logo,
                            home_page_content: homePageContent,
                            header_nav_modules: headerNavModules,
                            site_style:
                              serializeThemeCustomization(siteStyleConfig),
                          }),
                        })
                      }
                    >
                      <Save />
                      {t('Save Style')}
                    </Button>
                  </div>
                  <ThemeCustomizationEditor
                    value={siteStyleConfig}
                    disabled={saveBrandingMutation.isPending}
                    onChange={(next: ThemeCustomization) =>
                      setSiteStyle(serializeThemeCustomization(next))
                    }
                  />
                </section>

                <section className='rounded-lg border p-3'>
                  <HeaderNavigationSection
                    title={t('Agent header navigation')}
                    description={t(
                      'Control the top navigation shown on this agent domain.'
                    )}
                    config={headerNavConfig}
                    initialSerialized={headerNavSerialized}
                    saveLabel={t('Save navigation')}
                    saving={saveBrandingMutation.isPending}
                    onSave={async (serialized) => {
                      await saveBrandingMutation.mutateAsync({
                        branding: stringifyAgentBranding({
                          site_name: siteName,
                          logo,
                          home_page_content: homePageContent,
                          header_nav_modules: serialized,
                          site_style: siteStyle,
                        }),
                      })
                      setHeaderNavModules(serialized)
                    }}
                  />
                </section>

                <section className='rounded-lg border p-3'>
                  <div className='mb-3 grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(260px,360px)]'>
                    <div>
                      <h3 className='text-sm font-semibold'>
                        {t('Agent Domains')}
                      </h3>
                      <p className='text-muted-foreground mt-1 text-xs'>
                        {t(
                          'Add a custom domain, point its CNAME to the target, then verify it.'
                        )}
                      </p>
                    </div>
                    <div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]'>
                      <Input
                        value={newDomain}
                        onChange={(event) => setNewDomain(event.target.value)}
                        placeholder={t('agent.example.com')}
                      />
                      <Button
                        disabled={
                          !canCreateDomain || createDomainMutation.isPending
                        }
                        onClick={() =>
                          createDomainMutation.mutate({
                            domain: newDomain.trim(),
                          })
                        }
                      >
                        <Globe2 />
                        {t('Add Domain')}
                      </Button>
                    </div>
                  </div>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('Domain')}</TableHead>
                        <TableHead>{t('Status')}</TableHead>
                        <TableHead>{t('CNAME Target')}</TableHead>
                        <TableHead>{t('Actions')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {domains.length === 0 ? (
                        <TableEmpty
                          colSpan={4}
                          title={t('No Domains')}
                          description={t(
                            'Add a domain before using this agent site.'
                          )}
                          icon={<Globe2 className='size-6' />}
                        />
                      ) : (
                        domains.map((item) => (
                          <TableRow key={item.id}>
                            <TableCell>
                              <div className='flex items-center gap-2'>
                                <Globe2 className='size-4' />
                                {item.domain}
                              </div>
                            </TableCell>
                            <TableCell>
                              <Badge variant='outline'>
                                {t(statusLabel(item.status))}
                              </Badge>
                            </TableCell>
                            <TableCell className='max-w-[300px]'>
                              <div className='flex items-center gap-2'>
                                <span className='truncate font-mono text-xs'>
                                  {item.cname_target || '-'}
                                </span>
                                {item.cname_target ? (
                                  <Button
                                    variant='ghost'
                                    size='icon'
                                    onClick={() => copyText(item.cname_target)}
                                  >
                                    <Copy className='size-4' />
                                  </Button>
                                ) : null}
                              </div>
                            </TableCell>
                            <TableCell>
                              <Button
                                variant='outline'
                                size='sm'
                                disabled={
                                  item.status === 1 ||
                                  verifyDomainMutation.isPending
                                }
                                onClick={() =>
                                  verifyDomainMutation.mutate({ id: item.id })
                                }
                              >
                                <CheckCircle2 className='size-4' />
                                {t('Verify Domain')}
                              </Button>
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </section>
              </div>
            </TabsContent>

            <TabsContent value='users'>
              <section className='rounded-lg border p-3'>
                <div className='mb-3'>
                  <h3 className='text-sm font-semibold'>{t('Agent Users')}</h3>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t(
                      'Users bound to this agent can access the agent console.'
                    )}
                  </p>
                </div>
                <AgentUsersSearchControls
                  keyword={userKeywordInput}
                  isLoading={usersQuery.isLoading || usersQuery.isFetching}
                  onKeywordChange={setUserKeywordInput}
                  onSearch={searchAgentUsers}
                  onClear={clearAgentUserSearch}
                />
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Username')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Quota')}</TableHead>
                      <TableHead>{t('Group')}</TableHead>
                      <TableHead>{t('Email')}</TableHead>
                      <TableHead>{t('Requests:')}</TableHead>
                      <TableHead>{t('Last Login')}</TableHead>
                      <TableHead>{t('Actions')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {users.length === 0 ? (
                      <TableEmpty
                        colSpan={8}
                        title={t('No Agent Users')}
                        description={t(
                          'Admin-bound agent users will appear here.'
                        )}
                        icon={<Users className='size-6' />}
                      />
                    ) : (
                      users.map((user) => (
                        <TableRow key={user.id}>
                          <TableCell>
                            <div className='flex min-w-[160px] flex-col gap-1'>
                              <div className='flex items-center gap-2'>
                                <LongText className='max-w-[150px] font-medium'>
                                  {user.username || `#${user.user_id}`}
                                </LongText>
                                <Badge variant='outline'>#{user.user_id}</Badge>
                              </div>
                              {user.display_name &&
                              user.display_name !== user.username ? (
                                <LongText className='text-muted-foreground max-w-[180px] text-xs'>
                                  {user.display_name}
                                </LongText>
                              ) : null}
                              <span className='text-muted-foreground text-xs'>
                                {user.source}
                              </span>
                            </div>
                          </TableCell>
                          <TableCell>
                            <AgentUserStatusBadge status={user.status} />
                          </TableCell>
                          <TableCell>
                            <UserQuotaCell
                              quota={user.quota}
                              usedQuota={user.used_quota}
                            />
                          </TableCell>
                          <TableCell>
                            <div className='flex min-w-[150px] items-center gap-2'>
                              <GroupBadge group={user.group} />
                              <select
                                className='border-input bg-background h-8 max-w-[140px] rounded-md border px-2 text-xs'
                                value={user.group}
                                disabled={
                                  updateUserGroupMutation.isPending ||
                                  userGroupOptions.length === 0
                                }
                                onChange={(event) =>
                                  updateUserGroupMutation.mutate({
                                    userId: user.user_id,
                                    group_name: event.target.value,
                                  })
                                }
                              >
                                {user.group &&
                                !userGroupOptions.includes(user.group) ? (
                                  <option value={user.group}>
                                    {user.group}
                                  </option>
                                ) : null}
                                {userGroupOptions.map((group) => (
                                  <option
                                    key={group}
                                    value={group}
                                  >
                                    {group}
                                  </option>
                                ))}
                              </select>
                            </div>
                          </TableCell>
                          <TableCell>
                            <LongText className='text-muted-foreground max-w-[180px] text-sm'>
                              {user.email || '-'}
                            </LongText>
                          </TableCell>
                          <TableCell className='tabular-nums'>
                            {user.request_count?.toLocaleString() ?? 0}
                          </TableCell>
                          <TableCell>
                            {formatTimestampToDate(user.last_login_at)}
                          </TableCell>
                          <TableCell>
                            <Button
                              variant='outline'
                              size='sm'
                              disabled={updateUserStatusMutation.isPending}
                              onClick={() =>
                                updateUserStatusMutation.mutate({
                                  userId: user.user_id,
                                  status:
                                    user.status === USER_STATUS.ENABLED
                                      ? USER_STATUS.DISABLED
                                      : USER_STATUS.ENABLED,
                                })
                              }
                            >
                              {user.status === USER_STATUS.ENABLED
                                ? t('Disable')
                                : t('Enable')}
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
                <AgentUsersPaginationControls
                  page={usersPage.page ?? userPage}
                  pageSize={usersPage.page_size ?? userPageSize}
                  total={usersPage.total}
                  itemCount={users.length}
                  isLoading={usersQuery.isLoading || usersQuery.isFetching}
                  onPageChange={setUserPage}
                  onPageSizeChange={(nextPageSize) => {
                    setUserPageSize(nextPageSize)
                    setUserPage(1)
                  }}
                />
              </section>
            </TabsContent>

            <TabsContent value='pricing'>
              <div className='flex flex-col gap-4'>
                <AgentUserGroupManager
                  userGroups={userGroups}
                  groupRatios={groupRatios}
                  groupName={userGroupName}
                  visibleGroups={userGroupVisibleGroups}
                  canSave={canSaveUserGroup}
                  isPending={saveUserGroupMutation.isPending}
                  onGroupNameChange={setUserGroupName}
                  onVisibleGroupsChange={setUserGroupVisibleGroups}
                  onSave={() =>
                    saveUserGroupMutation.mutate({
                      group_name: userGroupName,
                      visible_groups: userGroupVisibleGroups,
                    })
                  }
                  onEdit={editAgentUserGroup}
                />
                <AgentGroupManager
                  groupRatios={groupRatios}
                  groupName={groupName}
                  systemGroupName={systemGroupName}
                  groupDescription={groupDescription}
                  groupRatio={groupRatio}
                  groupVisible={groupVisible}
                  canSave={canSaveGroupRatio}
                  isPending={saveGroupRatioMutation.isPending}
                  onGroupNameChange={setGroupName}
                  onSystemGroupNameChange={(nextSystemGroup) => {
                    setSystemGroupName(nextSystemGroup)
                    const nextRatio = groupRatios.find(
                      (item) => item.system_group_name === nextSystemGroup
                    )
                    setGroupRatio(
                      String(
                        nextRatio?.effective_ratio ?? nextRatio?.system_ratio ?? 1
                      )
                    )
                  }}
                  onGroupDescriptionChange={setGroupDescription}
                  onGroupRatioChange={setGroupRatio}
                  onGroupVisibleChange={setGroupVisible}
                  onSave={() =>
                    saveGroupRatioMutation.mutate({
                      group_name: groupName,
                      system_group_name: systemGroupName,
                      description: groupDescription,
                      ratio: Number(groupRatio),
                      visible: groupVisible,
                    })
                  }
                  onEdit={editAgentGroup}
                />
              </div>
            </TabsContent>

            <TabsContent value='settlement'>
              <div className='grid gap-4 xl:grid-cols-[minmax(320px,0.65fr)_minmax(0,1fr)]'>
                <section className='rounded-lg border p-3'>
                  <h3 className='mb-3 flex items-center gap-2 text-sm font-semibold'>
                    <Wallet className='size-4' />
                    {t('Withdraw')}
                  </h3>
                  <div className='space-y-2'>
                    <Input
                      value={withdrawMoney}
                      onChange={(event) => setWithdrawMoney(event.target.value)}
                      type='number'
                      min='0.01'
                      step='0.01'
                      placeholder={`${t('Money Amount')} (${normalizeSettlementCurrency(
                        balance?.currency ?? self?.agent.settlement_currency
                      )})`}
                    />
                    <Textarea
                      value={accountInfo}
                      onChange={(event) => setAccountInfo(event.target.value)}
                      placeholder={t('Withdrawal Account')}
                      className='min-h-20'
                    />
                    <Button
                      className='w-full'
                      disabled={
                        !canSubmitWithdrawal || withdrawMutation.isPending
                      }
                      onClick={() =>
                        withdrawMutation.mutate({
                          amount_money: Number(withdrawMoney),
                          account_info: accountInfo.trim(),
                        })
                      }
                    >
                      {t('Submit Withdrawal')}
                    </Button>
                  </div>
                </section>

                <section className='rounded-lg border p-3'>
                  <h3 className='mb-3 text-sm font-semibold'>
                    {t('Withdrawals')}
                  </h3>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('Money Amount')}</TableHead>
                        <TableHead>{t('Status')}</TableHead>
                        <TableHead>{t('Created At')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {withdrawals.length === 0 ? (
                        <TableEmpty
                          colSpan={3}
                          title={t('No Withdrawals')}
                          description={t(
                            'Submitted withdrawals will appear here.'
                          )}
                          icon={<Wallet className='size-6' />}
                        />
                      ) : (
                        withdrawals.map((item) => (
                          <TableRow key={item.id}>
                            <TableCell>
                              {formatSettlementAmount(
                                item.settlement_amount ?? item.amount_money,
                                item.currency
                              )}
                            </TableCell>
                            <TableCell>
                              <Badge variant={withdrawalVariant(item.status)}>
                                {t(item.status)}
                              </Badge>
                            </TableCell>
                            <TableCell>
                              {formatTimestampToDate(item.created_at)}
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </section>
              </div>

              <section className='mt-4 rounded-lg border p-3'>
                <h3 className='mb-3 text-sm font-semibold'>{t('Ledger')}</h3>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Type')}</TableHead>
                      <TableHead>{t('Profit')}</TableHead>
                      <TableHead>{t('Balance')}</TableHead>
                      <TableHead>{t('Created At')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {ledger.length === 0 ? (
                      <TableEmpty
                        colSpan={4}
                        title={t('No Ledger Records')}
                        description={t(
                          'Agent income and withdrawal records will appear here.'
                        )}
                        icon={<BadgeDollarSign className='size-6' />}
                      />
                    ) : (
                      ledger.map((item) => (
                        <TableRow key={item.id}>
                          <TableCell>{item.type}</TableCell>
                          <TableCell>
                            {formatSettlementAmount(
                              item.profit_amount,
                              item.currency
                            )}
                          </TableCell>
                          <TableCell>
                            {formatSettlementAmount(
                              item.balance_after_amount,
                              item.currency
                            )}
                          </TableCell>
                          <TableCell>
                            {formatTimestampToDate(item.created_at)}
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

function InfoItem({ label, value }: { label: string; value: string }) {
  return (
    <div className='rounded-lg border p-3'>
      <div className='text-muted-foreground text-xs'>{label}</div>
      <div className='mt-1 truncate text-sm font-medium'>{value}</div>
    </div>
  )
}
