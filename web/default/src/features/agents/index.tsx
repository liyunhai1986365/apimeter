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
import { useEffect, useState } from 'react'
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
import { TableEmpty } from '@/components/data-table'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import { formatTimestampToDate } from '@/lib/format'
import {
  createAgentDomain,
  getAgentSelf,
  listAgentDomains,
  listAgentLedger,
  listAgentPricingRules,
  listAgentUsers,
  listAgentWithdrawals,
  parseAgentBranding,
  stringifyAgentBranding,
  submitAgentWithdrawal,
  updateAgentBranding,
  upsertAgentPricingRule,
  verifyAgentDomain,
} from './api'

const formatQuota = (quota?: number) => (quota ?? 0).toLocaleString()

function statusLabel(status: number) {
  if (status === 1) return 'Active'
  if (status === 2) return 'Disabled'
  return 'Pending'
}

function withdrawalVariant(status: string): 'default' | 'outline' | 'secondary' | 'destructive' {
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
  const [newDomain, setNewDomain] = useState('')
  const [modelPattern, setModelPattern] = useState('*')
  const [markup, setMarkup] = useState('1.2')
  const [withdrawQuota, setWithdrawQuota] = useState('')
  const [withdrawMoney, setWithdrawMoney] = useState('')
  const [accountInfo, setAccountInfo] = useState('')

  const selfQuery = useQuery({
    queryKey: ['agent', 'self'],
    queryFn: getAgentSelf,
  })
  const domainsQuery = useQuery({
    queryKey: ['agent', 'domains'],
    queryFn: () => listAgentDomains(),
  })
  const rulesQuery = useQuery({
    queryKey: ['agent', 'pricing-rules'],
    queryFn: () => listAgentPricingRules(),
  })
  const usersQuery = useQuery({
    queryKey: ['agent', 'users'],
    queryFn: () => listAgentUsers(),
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
      toast.error(error instanceof Error ? error.message : t('Operation failed'))
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
      toast.error(error instanceof Error ? error.message : t('Operation failed'))
    },
  })

  const verifyDomainMutation = useMutation({
    mutationFn: verifyAgentDomain,
    onSuccess: () => {
      toast.success(t('Domain verified'))
      refreshAgent()
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Operation failed'))
    },
  })

  const saveRuleMutation = useMutation({
    mutationFn: upsertAgentPricingRule,
    onSuccess: () => {
      toast.success(t('Pricing rule saved'))
      refreshAgent()
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Operation failed'))
    },
  })

  const withdrawMutation = useMutation({
    mutationFn: submitAgentWithdrawal,
    onSuccess: () => {
      toast.success(t('Withdrawal submitted'))
      setWithdrawQuota('')
      setWithdrawMoney('')
      setAccountInfo('')
      refreshAgent()
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Operation failed'))
    },
  })

  const self = selfQuery.data?.data
  const balance = self?.balance
  const agentDomain = self?.context?.Domain || self?.agent?.slug || '-'
  const users = usersQuery.data?.data.items ?? []
  const domains = domainsQuery.data?.data.items ?? []
  const rules = rulesQuery.data?.data.items ?? []
  const ledger = ledgerQuery.data?.data.items ?? []
  const withdrawals = withdrawalsQuery.data?.data.items ?? []
  const canCreateDomain = newDomain.trim() !== ''
  const canSaveRule = modelPattern.trim() !== '' && Number(markup) > 0
  const canSubmitWithdrawal =
    Number(withdrawQuota) > 0 &&
    Number(withdrawMoney) >= 0 &&
    accountInfo.trim() !== ''

  useEffect(() => {
    if (!self?.agent) return
    const branding = parseAgentBranding(self.agent.branding)
    setSiteName(branding.site_name ?? '')
    setLogo(branding.logo ?? '')
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
              value={formatQuota(balance?.available_quota)}
              icon={<BadgeDollarSign className='size-4' />}
            />
            <MetricCard
              label={t('Pending Withdrawal')}
              value={formatQuota(balance?.pending_withdrawal_quota)}
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
                      label={t('Markup')}
                      value={String(self?.agent.default_markup ?? '-')}
                    />
                    <InfoItem
                      label={t('Settlement Currency')}
                      value={self?.agent.settlement_currency ?? '-'}
                    />
                  </div>
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
                    {t('Users bound to this agent can access the agent console.')}
                  </p>
                </div>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('User ID')}</TableHead>
                      <TableHead>{t('Source')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Created At')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {users.length === 0 ? (
                      <TableEmpty
                        colSpan={4}
                        title={t('No Agent Users')}
                        description={t('Admin-bound agent users will appear here.')}
                        icon={<Users className='size-6' />}
                      />
                    ) : (
                      users.map((user) => (
                        <TableRow key={user.id}>
                          <TableCell>{user.user_id}</TableCell>
                          <TableCell>{user.source}</TableCell>
                          <TableCell>
                            <Badge variant='outline'>
                              {t(statusLabel(user.status))}
                            </Badge>
                          </TableCell>
                          <TableCell>
                            {formatTimestampToDate(user.created_at)}
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </section>
            </TabsContent>

            <TabsContent value='pricing'>
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
                  <div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_96px_auto]'>
                    <Input
                      value={modelPattern}
                      onChange={(event) => setModelPattern(event.target.value)}
                      placeholder={t('Model')}
                    />
                    <Input
                      value={markup}
                      onChange={(event) => setMarkup(event.target.value)}
                      type='number'
                      step='0.01'
                      min='0.01'
                    />
                    <Button
                      disabled={!canSaveRule || saveRuleMutation.isPending}
                      onClick={() =>
                        saveRuleMutation.mutate({
                          model_pattern: modelPattern.trim(),
                          markup: Number(markup),
                          enabled: true,
                        })
                      }
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
                    {rules.length === 0 ? (
                      <TableEmpty
                        colSpan={3}
                        title={t('No Pricing Rules')}
                        description={t('Default markup is used when no rule matches.')}
                        icon={<BadgeDollarSign className='size-6' />}
                      />
                    ) : (
                      rules.map((rule) => (
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
                      value={withdrawQuota}
                      onChange={(event) => setWithdrawQuota(event.target.value)}
                      type='number'
                      min='1'
                      placeholder={t('Quota Amount')}
                    />
                    <Input
                      value={withdrawMoney}
                      onChange={(event) => setWithdrawMoney(event.target.value)}
                      type='number'
                      min='0'
                      step='0.01'
                      placeholder={t('Money Amount')}
                    />
                    <Textarea
                      value={accountInfo}
                      onChange={(event) => setAccountInfo(event.target.value)}
                      placeholder={t('Withdrawal Account')}
                      className='min-h-20'
                    />
                    <Button
                      className='w-full'
                      disabled={!canSubmitWithdrawal || withdrawMutation.isPending}
                      onClick={() =>
                        withdrawMutation.mutate({
                          amount_quota: Number(withdrawQuota),
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
                        <TableHead>{t('Quota Amount')}</TableHead>
                        <TableHead>{t('Money Amount')}</TableHead>
                        <TableHead>{t('Status')}</TableHead>
                        <TableHead>{t('Created At')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {withdrawals.length === 0 ? (
                        <TableEmpty
                          colSpan={4}
                          title={t('No Withdrawals')}
                          description={t('Submitted withdrawals will appear here.')}
                          icon={<Wallet className='size-6' />}
                        />
                      ) : (
                        withdrawals.map((item) => (
                          <TableRow key={item.id}>
                            <TableCell>{formatQuota(item.amount_quota)}</TableCell>
                            <TableCell>{item.amount_money}</TableCell>
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
                        description={t('Agent income and withdrawal records will appear here.')}
                        icon={<BadgeDollarSign className='size-6' />}
                      />
                    ) : (
                      ledger.map((item) => (
                        <TableRow key={item.id}>
                          <TableCell>{item.type}</TableCell>
                          <TableCell>{formatQuota(item.profit_quota)}</TableCell>
                          <TableCell>{formatQuota(item.balance_after)}</TableCell>
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
