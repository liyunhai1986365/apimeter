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
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { PAYMENT_TYPES } from '../../constants'
import type { PaymentMethod } from '../../types'

interface CryptoNetworkDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  methods: PaymentMethod[]
  selectedMethod?: PaymentMethod
  onNetworkChange: (method: PaymentMethod) => void
  onConfirm: () => void
  topupAmount: number
  paymentAmount: number
  calculating: boolean
  processing: boolean
}

function getNetworkLabel(method: PaymentMethod): string {
  if (method.type === PAYMENT_TYPES.CRYPTO_TRON) {
    return 'TRON'
  }

  if (
    method.type === PAYMENT_TYPES.CRYPTO_EVM &&
    /\b(bsc|bnb)\b/i.test(method.network_name ?? '')
  ) {
    return 'BSC'
  }

  return (
    method.network_name ||
    (method.type === PAYMENT_TYPES.CRYPTO_EVM ? 'EVM' : method.name)
  )
}

export function CryptoNetworkDialog({
  open,
  onOpenChange,
  methods,
  selectedMethod,
  onNetworkChange,
  onConfirm,
  topupAmount,
  paymentAmount,
  calculating,
  processing,
}: CryptoNetworkDialogProps) {
  const { t } = useTranslation()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'>
        <DialogHeader>
          <DialogTitle className='text-xl font-semibold'>
            {t('USDT Transfer')}
          </DialogTitle>
          <DialogDescription>
            {t('Select the network used to send USDT.')}
          </DialogDescription>
        </DialogHeader>

        <div className='flex flex-col gap-4 py-2'>
          <ToggleGroup
            value={selectedMethod ? [selectedMethod.type] : []}
            onValueChange={(value) => {
              const method = methods.find((item) => item.type === value[0])
              if (method) onNetworkChange(method)
            }}
            variant='outline'
            spacing={2}
            className='w-full'
            aria-label={t('Network')}
          >
            {methods.map((method) => (
              <ToggleGroupItem
                key={method.type}
                value={method.type}
                className='h-11 min-w-0 flex-1 font-semibold'
              >
                {getNetworkLabel(method)}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>

          <div className='bg-muted/40 flex flex-col gap-3 rounded-lg border p-4'>
            <div className='flex items-center justify-between gap-3'>
              <span className='text-muted-foreground text-sm'>
                {t('Topup Amount')}
              </span>
              <span className='font-semibold'>
                {formatCurrencyFromUSD(topupAmount, {
                  digitsLarge: 2,
                  digitsSmall: 2,
                  abbreviate: false,
                })}
              </span>
            </div>
            <div className='flex items-center justify-between gap-3'>
              <span className='text-muted-foreground text-sm'>
                {t('Network')}
              </span>
              <span className='font-semibold'>
                {selectedMethod ? getNetworkLabel(selectedMethod) : '—'}
              </span>
            </div>
            <div className='flex items-center justify-between gap-3 border-t pt-3'>
              <span className='text-muted-foreground text-sm'>
                {t('You Pay')}
              </span>
              {calculating ? (
                <Skeleton className='h-6 w-24' />
              ) : (
                <span className='text-lg font-semibold'>
                  {paymentAmount} {selectedMethod?.token_symbol ?? 'USDT'}
                </span>
              )}
            </div>
          </div>
        </div>

        <DialogFooter className='grid grid-cols-2 sm:grid-cols-2'>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={processing}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={onConfirm}
            disabled={!selectedMethod || calculating || processing}
          >
            {processing && <Loader2 className='animate-spin' />}
            {t('Create payment order')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
