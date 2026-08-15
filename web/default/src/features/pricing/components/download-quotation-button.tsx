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
import { Pdf02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { trackGoogleAnalyticsEvent } from '@/lib/google-analytics'
import { useGroupDiscountLabels } from '@/hooks/use-group-discount-labels'
import { useIsAdmin } from '@/hooks/use-admin'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import {
  getPricingForQuotation,
  getQuotationUserGroups,
  hydratePricingModels,
} from '../api'
import { downloadQuotationPdf } from '../lib/quotation-pdf'
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
  const isAdmin = useIsAdmin()
  const [isDownloading, setIsDownloading] = useState(false)
  const [isDialogOpen, setIsDialogOpen] = useState(false)
  const [isLoadingGroups, setIsLoadingGroups] = useState(false)
  const [userGroups, setUserGroups] = useState<string[] | null>(null)
  const [selectedUserGroup, setSelectedUserGroup] = useState('')

  const generateQuotation = useCallback(
    async (options?: { userGroup?: string; reloadPricing?: boolean }) => {
      if (isDownloading || props.models.length === 0) return
      setIsDownloading(true)

      try {
        const userGroup = options?.userGroup
        let models = props.models
        let usableGroup = props.usableGroup
        let groupDisplay = props.groupDisplay

        if (userGroup && options?.reloadPricing) {
          const pricing = await getPricingForQuotation(userGroup)
          const targetModels = hydratePricingModels(pricing)
          if (props.hasActiveFilters) {
            const filteredModelNames = new Set(
              props.models.map((model) => model.model_name)
            )
            models = targetModels.filter((model) =>
              filteredModelNames.has(model.model_name)
            )
          } else {
            models = targetModels
          }
          usableGroup = pricing.usable_group
          groupDisplay = pricing.group_display
        }

        if (models.length === 0) {
          toast.error(t('No models are available for the selected user group.'))
          return
        }

        const result = await downloadQuotationPdf({
          models,
          siteName: systemName,
          tokenUnit: props.tokenUnit,
          priceRate: props.priceRate,
          usdExchangeRate: props.usdExchangeRate,
          locale: i18n.resolvedLanguage || i18n.language || 'en',
          hasActiveFilters: props.hasActiveFilters,
          sourceUrl: new URL('/pricing', window.location.origin).toString(),
          usableGroup,
          groupDisplay,
          discountLabels,
          userGroup,
          translate: (key, values) => String(t(key, values)),
        })

        trackGoogleAnalyticsEvent('pricing_quotation_download', {
          format: 'pdf',
          model_count: models.length,
          row_count: result.rowCount,
          page_count: result.pageCount,
          filtered: props.hasActiveFilters,
          user_group: userGroup || undefined,
        })
        setIsDialogOpen(false)
        toast.success(t('Quotation PDF downloaded'))
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to generate pricing quotation PDF:', error)
        toast.error(t('Unable to generate quotation PDF'))
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
      props.userGroup,
      props.usableGroup,
      props.usdExchangeRate,
      systemName,
      t,
    ]
  )

  const openAdminDialog = async () => {
    if (isDownloading || props.models.length === 0) return
    setIsDialogOpen(true)
    if (userGroups !== null || isLoadingGroups) return

    setIsLoadingGroups(true)
    try {
      const groups = await getQuotationUserGroups()
      setUserGroups(groups)
      setSelectedUserGroup((current) => current || groups[0] || '')
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to load quotation user groups:', error)
      setUserGroups(null)
      toast.error(t('Unable to load user groups'))
    } finally {
      setIsLoadingGroups(false)
    }
  }

  const handleButtonClick = () => {
    if (isAdmin) {
      void openAdminDialog()
      return
    }
    void generateQuotation({ userGroup: props.userGroup })
  }

  const handleAdminDownload = () => {
    if (!selectedUserGroup) return
    void generateQuotation({
      userGroup: selectedUserGroup,
      reloadPricing: true,
    })
  }

  const handleDialogOpenChange = (open: boolean) => {
    if (isDownloading && !open) return
    setIsDialogOpen(open)
  }

  const button = (
    <Button
      type='button'
      variant='outline'
      size='sm'
      className='h-7 rounded-md px-2.5 text-xs'
      disabled={isDownloading || props.models.length === 0}
      onClick={handleButtonClick}
      title={t('Includes the models in the current filtered result.')}
      aria-label={
        isDownloading
          ? t('Preparing quotation PDF...')
          : t('Download quotation')
      }
    >
      {isDownloading ? (
        <Spinner data-icon='inline-start' />
      ) : (
        <HugeiconsIcon icon={Pdf02Icon} data-icon='inline-start' />
      )}
      <span className='hidden md:inline'>
        {isDownloading
          ? t('Preparing quotation PDF...')
          : t('Download quotation')}
      </span>
    </Button>
  )

  if (!isAdmin) return button

  return (
    <>
      {button}
      <Dialog open={isDialogOpen} onOpenChange={handleDialogOpenChange}>
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>{t('Generate quotation')}</DialogTitle>
            <DialogDescription>
              {t(
                'Select a user group. The quotation will use that group’s available models and effective supplier prices.'
              )}
            </DialogDescription>
          </DialogHeader>

          <FieldGroup>
            <Field data-disabled={isLoadingGroups || isDownloading}>
              <FieldLabel>{t('User group')}</FieldLabel>
              <Select
                value={selectedUserGroup}
                onValueChange={(value) => setSelectedUserGroup(value ?? '')}
                disabled={isLoadingGroups || isDownloading}
              >
                <SelectTrigger className='w-full'>
                  <SelectValue
                    placeholder={
                      isLoadingGroups
                        ? t('Loading user groups...')
                        : t('Select a user group')
                    }
                  />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {(userGroups || []).map((group) => (
                      <SelectItem key={group} value={group}>
                        {group}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <FieldDescription>
                {userGroups?.length === 0
                  ? t('No user groups are configured.')
                  : t(
                      'Prices are recalculated from the server before the PDF is generated.'
                    )}
              </FieldDescription>
            </Field>
          </FieldGroup>

          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              disabled={isDownloading}
              onClick={() => setIsDialogOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              disabled={!selectedUserGroup || isLoadingGroups || isDownloading}
              onClick={handleAdminDownload}
            >
              {isDownloading && <Spinner data-icon='inline-start' />}
              {isDownloading
                ? t('Preparing quotation PDF...')
                : t('Generate and download')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
