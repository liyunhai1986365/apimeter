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
import {
  Key01Icon,
  Settings02Icon,
  UserMultiple02Icon,
  WorkIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import {
  Avatar,
  AvatarFallback,
  AvatarGroup,
  AvatarGroupCount,
} from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { Workspace } from '@/features/keys/types'

type WorkspaceCardProps = {
  workspace: Workspace
  onOpenSettings: (workspace: Workspace) => void
}

export function WorkspaceCard({
  workspace,
  onOpenSettings,
}: WorkspaceCardProps) {
  const { t } = useTranslation()
  const members = workspace.access_users || []
  const visibleMembers = members.slice(0, 3)
  const hiddenMemberCount = Math.max(0, members.length - visibleMembers.length)
  const visibleMemberNames = visibleMembers
    .map((member) => member.display_name || member.username)
    .join(', ')

  const getInitials = (name: string) =>
    name
      .trim()
      .split(/\s+/)
      .slice(0, 2)
      .map((part) => part.charAt(0).toUpperCase())
      .join('') || '?'

  return (
    <Card className='min-h-72'>
      <CardHeader className='border-b'>
        <CardTitle className='flex min-w-0 items-center gap-3 pr-8'>
          <span className='bg-muted text-muted-foreground flex size-10 shrink-0 items-center justify-center rounded-lg border'>
            <HugeiconsIcon icon={WorkIcon} />
          </span>
          <span className='flex min-w-0 items-center gap-2'>
            <span className='truncate'>{workspace.name}</span>
            {workspace.is_default && (
              <Badge variant='secondary'>{t('Default')}</Badge>
            )}
          </span>
        </CardTitle>
        <CardDescription className='line-clamp-2 min-h-10 pl-13'>
          {workspace.description || t('No description for this workspace')}
        </CardDescription>
        <CardAction>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    aria-label={t('Workspace settings')}
                    onClick={() => onOpenSettings(workspace)}
                  />
                }
              >
                <HugeiconsIcon icon={Settings02Icon} />
              </TooltipTrigger>
              <TooltipContent>{t('Workspace settings')}</TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </CardAction>
      </CardHeader>

      <CardContent className='flex flex-1 flex-col gap-4'>
        <div className='grid grid-cols-[1fr_auto_1fr] items-center gap-4'>
          <div className='flex min-w-0 items-center gap-3'>
            <span className='bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-md'>
              <HugeiconsIcon icon={Key01Icon} />
            </span>
            <div className='flex min-w-0 flex-col gap-0.5'>
              <span className='text-muted-foreground truncate text-xs'>
                {t('API Keys')}
              </span>
              <span className='text-lg leading-6 font-semibold tabular-nums'>
                {workspace.token_count || 0}
              </span>
            </div>
          </div>
          <Separator orientation='vertical' className='h-10' />
          <div className='flex min-w-0 items-center gap-3'>
            <span className='bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-md'>
              <HugeiconsIcon icon={UserMultiple02Icon} />
            </span>
            <div className='flex min-w-0 flex-col gap-0.5'>
              <span className='text-muted-foreground truncate text-xs'>
                {t('Subaccounts')}
              </span>
              <span className='text-lg leading-6 font-semibold tabular-nums'>
                {members.length}
              </span>
            </div>
          </div>
        </div>

        <Separator />

        <div className='flex min-h-10 min-w-0 items-center justify-between gap-3'>
          <div className='flex min-w-0 flex-col gap-0.5'>
            <span className='text-xs font-medium'>
              {t('Workspace members')}
            </span>
            <span className='text-muted-foreground truncate text-xs'>
              {members.length > 0
                ? visibleMemberNames
                : t('No subaccounts assigned')}
            </span>
          </div>
          {members.length > 0 && (
            <AvatarGroup className='shrink-0'>
              {visibleMembers.map((member) => {
                const name = member.display_name || member.username
                return (
                  <Avatar key={member.id} size='sm' title={name}>
                    <AvatarFallback>{getInitials(name)}</AvatarFallback>
                  </Avatar>
                )
              })}
              {hiddenMemberCount > 0 && (
                <AvatarGroupCount>+{hiddenMemberCount}</AvatarGroupCount>
              )}
            </AvatarGroup>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
