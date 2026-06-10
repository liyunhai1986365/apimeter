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
import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { ApiKeyWorkspaceDetail } from './components/api-key-workspace-detail'
import { ApiKeyWorkspacePanel } from './components/api-key-workspace-panel'
import { ApiKeysDialogs } from './components/api-keys-dialogs'
import { ApiKeysProvider } from './components/api-keys-provider'

export function ApiKeys() {
  const { t } = useTranslation()
  return (
    <ApiKeysProvider>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('API Keys')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Manage your API keys for accessing the service')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Content>
          <div className='min-h-0 space-y-4'>
            <ApiKeyWorkspacePanel />
            <ApiKeyWorkspaceDetail />
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ApiKeysDialogs />
    </ApiKeysProvider>
  )
}
