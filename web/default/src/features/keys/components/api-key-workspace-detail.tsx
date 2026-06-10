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
import { KeyRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiKeysPrimaryButtons } from './api-keys-primary-buttons'
import { useApiKeys } from './api-keys-provider'
import { ApiKeysTable } from './api-keys-table'

export function ApiKeyWorkspaceDetail() {
  const { t } = useTranslation()
  const { selectedWorkspace, isLoadingWorkspaces } = useApiKeys()

  if (isLoadingWorkspaces && !selectedWorkspace) {
    return (
      <section className='min-w-0 space-y-3'>
        <div className='flex flex-col gap-3 px-1 py-1 sm:flex-row sm:items-center sm:justify-between'>
          <div className='min-w-0 space-y-2'>
            <Skeleton className='h-5 w-44' />
            <Skeleton className='h-4 w-72 max-w-full' />
          </div>
          <Skeleton className='h-8 w-32' />
        </div>
        <ApiKeysTable />
      </section>
    )
  }

  return (
    <section className='min-w-0 space-y-3'>
      <div className='flex flex-col gap-3 px-1 py-1 sm:flex-row sm:items-center sm:justify-between'>
        <div className='flex min-w-0 items-start gap-3'>
          <div className='bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-md border'>
            <KeyRound className='size-4' />
          </div>
          <div className='min-w-0'>
            <div className='flex min-w-0 flex-wrap items-center gap-2'>
              <h2 className='truncate text-sm font-semibold'>
                {selectedWorkspace
                  ? t('API Keys in {{workspace}}', {
                      workspace: selectedWorkspace.name,
                    })
                  : t('API Keys in selected workspace')}
              </h2>
            </div>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t('Workspace totals are calculated from linked API keys.')}
            </p>
          </div>
        </div>
        <ApiKeysPrimaryButtons />
      </div>

      <ApiKeysTable />
    </section>
  )
}
