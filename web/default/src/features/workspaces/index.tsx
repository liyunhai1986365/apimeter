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
import { Add01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardHeader } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { SectionPageLayout } from '@/components/layout'
import {
  ApiKeysProvider,
  useApiKeys,
} from '@/features/keys/components/api-keys-provider'
import type { Workspace } from '@/features/keys/types'
import { TeamSettingsTabs } from '@/features/team-settings/components/team-settings-tabs'
import { WorkspaceCard } from './components/workspace-card'
import { WorkspaceDialogs } from './components/workspace-dialogs'

function WorkspacesContent() {
  const { t } = useTranslation()
  const { workspaces, isLoadingWorkspaces, setOpen, setSelectedWorkspaceId } =
    useApiKeys()

  const handleOpenSettings = (workspace: Workspace) => {
    setSelectedWorkspaceId(workspace.id)
    setOpen('workspace-settings')
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Team settings')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Manage workspace access, API keys, quotas, and subaccounts.')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <Button onClick={() => setOpen('workspace-create')}>
            <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
            {t('New Workspace')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex flex-col gap-4'>
            <TeamSettingsTabs value='workspaces' />
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3'>
              {isLoadingWorkspaces && workspaces.length === 0
                ? Array.from({ length: 3 }).map((_, index) => (
                    <Card key={index} className='min-h-72'>
                      <CardHeader className='border-b'>
                        <Skeleton className='size-10 rounded-lg' />
                        <Skeleton className='ml-13 h-10 w-3/4' />
                        <CardAction>
                          <Skeleton className='size-7 rounded-md' />
                        </CardAction>
                      </CardHeader>
                      <CardContent className='flex flex-1 flex-col gap-4'>
                        <div className='grid grid-cols-[1fr_auto_1fr] items-center gap-4'>
                          <Skeleton className='h-10 w-full' />
                          <Separator orientation='vertical' className='h-10' />
                          <Skeleton className='h-10 w-full' />
                        </div>
                        <Separator />
                        <Skeleton className='h-10 w-full' />
                      </CardContent>
                    </Card>
                  ))
                : workspaces.map((workspace) => (
                    <WorkspaceCard
                      key={workspace.id}
                      workspace={workspace}
                      onOpenSettings={handleOpenSettings}
                    />
                  ))}
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <WorkspaceDialogs />
    </>
  )
}

export function Workspaces() {
  return (
    <ApiKeysProvider>
      <WorkspacesContent />
    </ApiKeysProvider>
  )
}
