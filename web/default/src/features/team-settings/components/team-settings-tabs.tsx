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
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

export type TeamSettingsSection = 'workspaces' | 'subaccounts'

type TeamSettingsTabsProps = {
  value: TeamSettingsSection
}

export function TeamSettingsTabs({ value }: TeamSettingsTabsProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const handleValueChange = (nextValue: string) => {
    if (nextValue === value) return
    if (nextValue === 'workspaces') {
      void navigate({ to: '/workspaces' })
      return
    }
    void navigate({ to: '/workspace-accounts' })
  }

  return (
    <Tabs value={value} onValueChange={handleValueChange} className='w-full'>
      <TabsList className='grid w-full grid-cols-2 sm:w-fit'>
        <TabsTrigger value='workspaces'>{t('Workspaces')}</TabsTrigger>
        <TabsTrigger value='subaccounts'>{t('Subaccounts')}</TabsTrigger>
      </TabsList>
    </Tabs>
  )
}
