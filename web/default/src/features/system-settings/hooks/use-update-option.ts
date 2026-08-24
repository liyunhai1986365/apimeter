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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { toast } from 'sonner'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { applyCustomerServiceScript } from '@/lib/customer-service-script'
import { applyGoogleAnalytics } from '@/lib/google-analytics'
import { updateSystemOption } from '../api'
import type { UpdateOptionRequest } from '../types'

// Configuration keys that require status refresh
const STATUS_RELATED_KEYS = [
  'theme.frontend',
  'HeaderNavModules',
  'SidebarModulesAdmin',
  'CustomerServiceScript',
  'GoogleAnalyticsId',
  'Notice',
  'console_setting.announcements',
  'console_setting.announcements_enabled',
  'LogConsumeEnabled',
  'QuotaPerUnit',
  'USDExchangeRate',
  'DisplayInCurrencyEnabled',
  'DisplayTokenStatEnabled',
  'general_setting.default_user_display_currency',
  'general_setting.quota_display_type',
  'general_setting.custom_currency_symbol',
  'general_setting.custom_currency_exchange_rate',
  'MainlandChinaPresentationEnabled',
  'FooterCompanyName',
]

export function useUpdateOption() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (request: UpdateOptionRequest) => updateSystemOption(request),
    onSuccess: (data, variables) => {
      if (data.success) {
        // Always refresh system-options
        queryClient.invalidateQueries({ queryKey: ['system-options'] })

        if (variables.key === 'AffiliateRoleConfigs') {
          queryClient.invalidateQueries({ queryKey: ['affiliate-roles'] })
        }

        // If updating frontend-display-related config, also refresh status
        if (STATUS_RELATED_KEYS.includes(variables.key)) {
          queryClient.invalidateQueries({ queryKey: ['status'] })
        }

        if (variables.key === 'CustomerServiceScript') {
          const value = String(variables.value ?? '')
          useSystemConfigStore.getState().setConfig({
            customerServiceScript: value,
          })
          applyCustomerServiceScript(value)
        }

        if (variables.key === 'GoogleAnalyticsId') {
          const value = String(variables.value ?? '')
          useSystemConfigStore.getState().setConfig({
            googleAnalyticsId: value,
          })
          applyGoogleAnalytics(value)
        }

        if (variables.key === 'MainlandChinaPresentationEnabled') {
          useSystemConfigStore.getState().setConfig({
            mainlandChinaPresentationEnabled:
              variables.value === true || variables.value === 'true',
          })
        }

        if (variables.key === 'FooterCompanyName') {
          useSystemConfigStore.getState().setConfig({
            footerCompanyName: String(variables.value ?? ''),
          })
        }

        if (variables.key === 'general_setting.default_user_display_currency') {
          useSystemConfigStore.getState().setConfig({
            defaultUserDisplayCurrency:
              variables.value === 'CNY' ? 'CNY' : 'USD',
          })
        }

        toast.success(i18next.t('Setting updated successfully'))
      } else {
        toast.error(data.message || i18next.t('Failed to update setting'))
      }
    },
    onError: (error: Error) => {
      toast.error(error.message || i18next.t('Failed to update setting'))
    },
  })
}
