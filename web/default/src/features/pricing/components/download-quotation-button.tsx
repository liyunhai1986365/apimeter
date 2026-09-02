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
import { useCallback, useState } from 'react'
import { FileSpreadsheetIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { trackGoogleAnalyticsEvent } from '@/lib/google-analytics'
import { useGroupDiscountLabels } from '@/hooks/use-group-discount-labels'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import type { QuotationOptions } from '../lib/quotation'
import { downloadQuotationSpreadsheet } from '../lib/quotation-spreadsheet'
import type {
  PricingGroupDisplayConfig,
  PricingModel,
  TokenUnit,
} from '../types'

type DownloadQuotationButtonProps = {
  models: PricingModel[]
  tokenUnit: TokenUnit
  priceRate: number
  usdExchangeRate: number
  userGroup?: string
  hasActiveFilters: boolean
  usableGroup: Record<string, string | { desc?: string; ratio?: number }>
  groupDisplay?: PricingGroupDisplayConfig
}

export function DownloadQuotationButton(props: DownloadQuotationButtonProps) {
  const { t, i18n } = useTranslation()
  const systemName = useSystemConfigStore((state) => state.config.systemName)
  const discountLabels = useGroupDiscountLabels()
  const [isDownloading, setIsDownloading] = useState(false)

  const generateQuotation = useCallback(async () => {
    if (isDownloading || props.models.length === 0) return
    setIsDownloading(true)

    try {
      const quotationOptions: QuotationOptions = {
        models: props.models,
        siteName: systemName,
        tokenUnit: props.tokenUnit,
        priceRate: props.priceRate,
        usdExchangeRate: props.usdExchangeRate,
        locale: i18n.resolvedLanguage || i18n.language || 'en',
        hasActiveFilters: props.hasActiveFilters,
        sourceUrl: new URL('/pricing', window.location.origin).toString(),
        usableGroup: props.usableGroup,
        groupDisplay: props.groupDisplay,
        discountLabels,
        userGroup: props.userGroup,
        translate: (key, values) => String(t(key, values)),
      }
      const result = await downloadQuotationSpreadsheet(quotationOptions)

      trackGoogleAnalyticsEvent('pricing_quotation_download', {
        format: 'xlsx',
        model_count: props.models.length,
        row_count: result.rowCount,
        sheet_count: result.sheetCount,
        filtered: props.hasActiveFilters,
        user_group: props.userGroup || undefined,
      })
      toast.success(t('Quotation spreadsheet downloaded'))
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to generate pricing quotation spreadsheet:', error)
      toast.error(t('Unable to generate quotation spreadsheet'))
    } finally {
      setIsDownloading(false)
    }
  }, [
    discountLabels,
    i18n.language,
    i18n.resolvedLanguage,
    isDownloading,
    props.groupDisplay,
    props.hasActiveFilters,
    props.models,
    props.priceRate,
    props.tokenUnit,
    props.usableGroup,
    props.usdExchangeRate,
    props.userGroup,
    systemName,
    t,
  ])

  return (
    <Button
      type='button'
      variant='outline'
      size='sm'
      className='h-7 rounded-md px-2.5 text-xs'
      disabled={isDownloading || props.models.length === 0}
      title={t('Includes the models in the current filtered result.')}
      aria-label={
        isDownloading ? t('Preparing quotation...') : t('Download quotation')
      }
      onClick={() => void generateQuotation()}
    >
      {isDownloading ? (
        <Spinner data-icon='inline-start' />
      ) : (
        <HugeiconsIcon icon={FileSpreadsheetIcon} data-icon='inline-start' />
      )}
      <span className='hidden md:inline'>
        {isDownloading ? t('Preparing quotation...') : t('Download quotation')}
      </span>
    </Button>
  )
}
