import { useTranslation } from 'react-i18next'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { getDisplayName, getUserInitials } from '../lib'
import type { UserProfile } from '../types'

interface WorkspaceAccountProfileProps {
  profile: UserProfile | null
  loading: boolean
}

export function WorkspaceAccountProfile({
  profile,
  loading,
}: WorkspaceAccountProfileProps) {
  const { t } = useTranslation()

  if (loading) {
    return (
      <Card>
        <CardHeader>
          <Skeleton className='h-5 w-36' />
          <Skeleton className='h-4 w-64 max-w-full' />
        </CardHeader>
        <CardContent>
          <Skeleton className='h-20 w-full' />
        </CardContent>
      </Card>
    )
  }

  if (!profile) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Workspace account profile')}</CardTitle>
        <CardDescription>
          {t('Your account identity is managed by the main account.')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className='flex flex-col gap-4 sm:flex-row sm:items-center'>
          <Avatar className='size-12 rounded-lg'>
            <AvatarFallback className='rounded-lg'>
              {getUserInitials(profile)}
            </AvatarFallback>
          </Avatar>
          <dl className='grid min-w-0 flex-1 gap-3 sm:grid-cols-3'>
            <div className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>{t('Name')}</dt>
              <dd className='truncate font-medium'>
                {getDisplayName(profile)}
              </dd>
            </div>
            <div className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>{t('Username')}</dt>
              <dd className='truncate font-medium'>{profile.username}</dd>
            </div>
            <div className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>{t('Email')}</dt>
              <dd className='truncate font-medium'>
                {profile.email || t('Not set')}
              </dd>
            </div>
          </dl>
        </div>
      </CardContent>
    </Card>
  )
}
