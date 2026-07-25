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
import { BriefcaseBusiness, Check, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useApiKeys } from './api-keys-provider'

type ApiKeyWorkspacePanelProps = {
  showTokenCount?: boolean
}

export function ApiKeyWorkspacePanel({
  showTokenCount = true,
}: ApiKeyWorkspacePanelProps) {
  const { t } = useTranslation()
  const isWorkspaceSubaccount = useAuthStore(
    (state) => state.auth.user?.workspace_subaccount === true
  )
  const {
    workspaces,
    selectedWorkspaceId,
    setSelectedWorkspaceId,
    isLoadingWorkspaces,
    setOpen,
  } = useApiKeys()

  return (
    <section className='bg-muted/20 rounded-lg border p-2'>
      <div className='flex gap-2 overflow-x-auto pb-1'>
        {isLoadingWorkspaces && workspaces.length === 0
          ? Array.from({ length: 4 }).map((_, index) => (
              <Skeleton key={index} className='h-14 min-w-48 rounded-md' />
            ))
          : workspaces.map((workspace) => {
              const selected = workspace.id === selectedWorkspaceId

              return (
                <button
                  key={workspace.id}
                  type='button'
                  onClick={() => setSelectedWorkspaceId(workspace.id)}
                  className={cn(
                    'bg-background min-w-48 rounded-md border px-3 py-2 text-left transition-colors',
                    'hover:border-primary/40 hover:bg-accent/40 focus-visible:ring-ring focus-visible:ring-2 focus-visible:outline-none',
                    selected && 'border-primary bg-primary/5'
                  )}
                >
                  <div className='flex min-w-0 items-center justify-between gap-2'>
                    <div className='flex min-w-0 items-center gap-2'>
                      <span
                        className={cn(
                          'text-muted-foreground flex size-6 shrink-0 items-center justify-center rounded-md border',
                          selected && 'border-primary/30 text-primary'
                        )}
                      >
                        {selected ? (
                          <Check className='size-3.5' />
                        ) : (
                          <BriefcaseBusiness className='size-3.5' />
                        )}
                      </span>
                      <span className='truncate text-sm font-medium'>
                        {workspace.name}
                      </span>
                    </div>
                    {workspace.is_default && (
                      <Badge variant='secondary' className='h-5 px-1.5'>
                        {t('Default')}
                      </Badge>
                    )}
                  </div>
                  <div className='text-muted-foreground mt-1 text-xs tabular-nums'>
                    {showTokenCount
                      ? t('{{count}} keys', {
                          count: workspace.token_count || 0,
                        })
                      : workspace.description ||
                        (workspace.access_users.length > 0
                          ? t('Accessible by {{count}} subaccounts', {
                              count: workspace.access_users.length,
                            })
                          : t('Main account only'))}
                    {showTokenCount &&
                    !isWorkspaceSubaccount &&
                    workspace.access_users.length > 0
                      ? ` · ${t('{{count}} subaccounts', {
                          count: workspace.access_users.length,
                        })}`
                      : ''}
                  </div>
                </button>
              )
            })}

        {!isWorkspaceSubaccount && (
          <Button
            type='button'
            variant='outline'
            className='bg-background h-auto min-h-14 min-w-44 shrink-0 flex-col gap-1 border-dashed px-3 py-2'
            onClick={() => setOpen('workspace-create')}
          >
            <Plus className='size-4' />
            <span className='text-sm'>{t('New Workspace')}</span>
          </Button>
        )}
      </div>
    </section>
  )
}
