import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Add01Icon,
  Delete02Icon,
  Edit01Icon,
  Key01Icon,
  MoreHorizontalIcon,
  PowerIcon,
  PowerOffIcon,
  UserMultipleIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatTimestamp } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { SectionPageLayout } from '@/components/layout'
import { getWorkspaces } from '@/features/keys/api'
import { TeamSettingsTabs } from '@/features/team-settings/components/team-settings-tabs'
import { getWorkspaceSubaccounts, updateWorkspaceSubaccountStatus } from './api'
import {
  WorkspaceSubaccountActionDialogs,
  type WorkspaceSubaccountAction,
} from './components/workspace-subaccount-action-dialogs'
import { WorkspaceSubaccountFormDialog } from './components/workspace-subaccount-form-dialog'
import type { WorkspaceSubaccountSummary } from './types'

function formatOptionalTimestamp(timestamp: number, fallback: string): string {
  return timestamp > 0 ? formatTimestamp(timestamp) : fallback
}

export function WorkspaceSubaccounts() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [formOpen, setFormOpen] = useState(false)
  const [editingAccount, setEditingAccount] =
    useState<WorkspaceSubaccountSummary | null>(null)
  const [action, setAction] = useState<WorkspaceSubaccountAction>(null)
  const [actionAccount, setActionAccount] =
    useState<WorkspaceSubaccountSummary | null>(null)
  const [statusAccountId, setStatusAccountId] = useState<number | null>(null)

  const accountsQuery = useQuery({
    queryKey: ['workspace-subaccounts'],
    queryFn: getWorkspaceSubaccounts,
  })
  const workspacesQuery = useQuery({
    queryKey: ['workspaces'],
    queryFn: getWorkspaces,
  })

  const accounts = accountsQuery.data?.data || []
  const workspaces = workspacesQuery.data?.data || []

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['workspace-subaccounts'] })
    void queryClient.invalidateQueries({ queryKey: ['workspaces'] })
  }

  const openCreate = () => {
    setEditingAccount(null)
    setFormOpen(true)
  }

  const openEdit = (account: WorkspaceSubaccountSummary) => {
    setEditingAccount(account)
    setFormOpen(true)
  }

  const openAction = (
    nextAction: Exclude<WorkspaceSubaccountAction, null>,
    account: WorkspaceSubaccountSummary
  ) => {
    setActionAccount(account)
    setAction(nextAction)
  }

  const handleStatusChange = async (account: WorkspaceSubaccountSummary) => {
    const nextStatus = account.status === 1 ? 2 : 1
    setStatusAccountId(account.id)
    try {
      const result = await updateWorkspaceSubaccountStatus(
        account.id,
        nextStatus
      )
      if (!result.success) {
        toast.error(result.message || t('Failed to update account status'))
        return
      }
      toast.success(
        t(
          nextStatus === 1
            ? 'Workspace account enabled'
            : 'Workspace account disabled'
        )
      )
      refresh()
    } catch {
      toast.error(t('Failed to update account status'))
    } finally {
      setStatusAccountId(null)
    }
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Team settings')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t(
            'Create limited accounts that access assigned workspaces and manage their API keys.'
          )}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <Button onClick={openCreate}>
            <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
            {t('Create account')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex flex-col gap-4'>
            <TeamSettingsTabs value='subaccounts' />
            <div className='overflow-hidden rounded-lg border'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Account')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead>{t('Workspaces')}</TableHead>
                    <TableHead>{t('API Keys')}</TableHead>
                    <TableHead>{t('Last used')}</TableHead>
                    <TableHead className='w-12'>
                      <span className='sr-only'>{t('Actions')}</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {accountsQuery.isLoading ? (
                    Array.from({ length: 4 }).map((_, index) => (
                      <TableRow key={index}>
                        <TableCell colSpan={6}>
                          <Skeleton className='h-10 w-full' />
                        </TableCell>
                      </TableRow>
                    ))
                  ) : accounts.length > 0 ? (
                    accounts.map((account) => (
                      <TableRow key={account.id}>
                        <TableCell>
                          <div className='flex min-w-44 flex-col gap-0.5'>
                            <span className='font-medium'>
                              {account.display_name || account.username}
                            </span>
                            <span className='text-muted-foreground text-xs'>
                              {account.username}
                              {account.email ? ` · ${account.email}` : ''}
                            </span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className='flex flex-wrap gap-1'>
                            <Badge
                              variant={
                                account.status === 1 ? 'secondary' : 'outline'
                              }
                            >
                              {t(account.status === 1 ? 'Enabled' : 'Disabled')}
                            </Badge>
                            {account.must_change_password && (
                              <Badge variant='outline'>
                                {t('Password change required')}
                              </Badge>
                            )}
                          </div>
                        </TableCell>
                        <TableCell className='tabular-nums'>
                          {account.workspace_count}
                        </TableCell>
                        <TableCell className='tabular-nums'>
                          {account.token_count}
                        </TableCell>
                        <TableCell className='text-muted-foreground'>
                          {formatOptionalTimestamp(
                            account.last_used_at,
                            t('Never')
                          )}
                        </TableCell>
                        <TableCell>
                          <DropdownMenu>
                            <DropdownMenuTrigger
                              render={
                                <Button
                                  variant='ghost'
                                  size='icon-sm'
                                  aria-label={t('Open account actions')}
                                />
                              }
                            >
                              <HugeiconsIcon icon={MoreHorizontalIcon} />
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align='end' className='w-48'>
                              <DropdownMenuGroup>
                                <DropdownMenuItem
                                  onClick={() => openEdit(account)}
                                >
                                  <HugeiconsIcon icon={Edit01Icon} />
                                  {t('Edit account')}
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  onClick={() =>
                                    void handleStatusChange(account)
                                  }
                                  disabled={statusAccountId === account.id}
                                >
                                  {account.status === 1 ? (
                                    <HugeiconsIcon icon={PowerOffIcon} />
                                  ) : (
                                    <HugeiconsIcon icon={PowerIcon} />
                                  )}
                                  {t(
                                    account.status === 1
                                      ? 'Disable account'
                                      : 'Enable account'
                                  )}
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  onClick={() =>
                                    openAction('reset-password', account)
                                  }
                                >
                                  <HugeiconsIcon icon={Key01Icon} />
                                  {t('Reset password')}
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  variant='destructive'
                                  onClick={() => openAction('delete', account)}
                                >
                                  <HugeiconsIcon icon={Delete02Icon} />
                                  {t('Delete account')}
                                </DropdownMenuItem>
                              </DropdownMenuGroup>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </TableCell>
                      </TableRow>
                    ))
                  ) : (
                    <TableRow>
                      <TableCell colSpan={6} className='p-0'>
                        <Empty className='border-0'>
                          <EmptyHeader>
                            <EmptyMedia variant='icon'>
                              <HugeiconsIcon icon={UserMultipleIcon} />
                            </EmptyMedia>
                            <EmptyTitle>
                              {t('No workspace accounts')}
                            </EmptyTitle>
                            <EmptyDescription>
                              {t(
                                'Create an account when someone needs access to a workspace and its API keys.'
                              )}
                            </EmptyDescription>
                          </EmptyHeader>
                        </Empty>
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <WorkspaceSubaccountFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        account={editingAccount}
        workspaces={workspaces}
        onSaved={refresh}
      />
      <WorkspaceSubaccountActionDialogs
        action={action}
        account={actionAccount}
        onClose={() => {
          setAction(null)
          setActionAccount(null)
        }}
        onSaved={refresh}
      />
    </>
  )
}
