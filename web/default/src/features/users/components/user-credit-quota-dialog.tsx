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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { formatQuota, parseQuotaFromDollars } from '@/lib/format'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { manageUserCreditQuota } from '../api'
import type { CreditQuotaOperation } from '../types'

interface UserCreditQuotaDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
  currentBalance: number
  currentCreditQuota: number
  onSuccess: () => void
}

export function UserCreditQuotaDialog(props: UserCreditQuotaDialogProps) {
  const { t } = useTranslation()
  const [operation, setOperation] = useState<CreditQuotaOperation>('grant')
  const [amount, setAmount] = useState('')
  const [remark, setRemark] = useState('')
  const [loading, setLoading] = useState(false)

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'
  const amountValue = parseFloat(amount) || 0
  const quotaValue = parseQuotaFromDollars(amountValue)
  const isOverRepayment =
    operation === 'repay' && quotaValue > props.currentCreditQuota
  const amountInvalid = quotaValue <= 0 || isOverRepayment

  const reset = () => {
    setOperation('grant')
    setAmount('')
    setRemark('')
  }

  const handleOpenChange = (open: boolean) => {
    if (!open) reset()
    props.onOpenChange(open)
  }

  const handleConfirm = async () => {
    if (amountInvalid) return
    setLoading(true)
    try {
      const result = await manageUserCreditQuota({
        id: props.userId,
        action: 'manage_credit_quota',
        mode: operation,
        value: quotaValue,
        remark: remark.trim(),
      })
      if (!result.success) {
        toast.error(result.message || t('Failed to update credit quota'))
        return
      }
      toast.success(
        operation === 'grant'
          ? t('Credit granted successfully')
          : t('Repayment recorded successfully')
      )
      reset()
      props.onOpenChange(false)
      props.onSuccess()
    } catch (error: unknown) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to update credit quota')
      )
    } finally {
      setLoading(false)
    }
  }

  const creditAfter =
    operation === 'grant'
      ? props.currentCreditQuota + quotaValue
      : Math.max(0, props.currentCreditQuota - quotaValue)
  const balanceAfter =
    operation === 'grant'
      ? props.currentBalance + quotaValue
      : props.currentBalance

  return (
    <Dialog open={props.open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Credit Control Management')}</DialogTitle>
          <DialogDescription>
            {t('Grant spendable credit or record a credit repayment.')}
          </DialogDescription>
        </DialogHeader>

        <FieldGroup className='gap-4'>
          <Alert>
            <AlertTitle>
              {operation === 'grant'
                ? t('Grant credit')
                : t('Record repayment')}
            </AlertTitle>
            <AlertDescription>
              {operation === 'grant'
                ? t(
                    "Granting credit increases both the user's balance and outstanding credit."
                  )
                : t(
                    'Repayment only reduces outstanding credit and does not change the current balance.'
                  )}
            </AlertDescription>
          </Alert>

          <Field>
            <FieldLabel>{t('Operation')}</FieldLabel>
            <ToggleGroup
              value={[operation]}
              onValueChange={(value) => {
                const next = value[0] as CreditQuotaOperation | undefined
                if (!next) return
                setOperation(next)
                setAmount('')
              }}
              variant='outline'
              spacing={2}
              className='w-full'
            >
              <ToggleGroupItem value='grant' className='flex-1'>
                {t('Grant credit')}
              </ToggleGroupItem>
              <ToggleGroupItem value='repay' className='flex-1'>
                {t('Record repayment')}
              </ToggleGroupItem>
            </ToggleGroup>
          </Field>

          <div className='grid grid-cols-2 gap-3 text-sm'>
            <div>
              <div className='text-muted-foreground'>
                {t('Current Balance')}
              </div>
              <div className='font-medium tabular-nums'>
                {formatQuota(props.currentBalance)}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>{t('Credit Quota')}</div>
              <div className='font-medium tabular-nums'>
                {formatQuota(props.currentCreditQuota)}
              </div>
            </div>
          </div>

          <Field data-invalid={amountInvalid && amount.length > 0}>
            <FieldLabel htmlFor='credit-quota-amount'>
              {t('Amount')} ({currencyLabel})
            </FieldLabel>
            <Input
              id='credit-quota-amount'
              type='number'
              step={tokensOnly ? 1 : 0.000001}
              min={0}
              value={amount}
              placeholder={
                tokensOnly
                  ? t('Enter amount in tokens')
                  : t('Enter amount in {{currency}}', {
                      currency: currencyLabel,
                    })
              }
              aria-invalid={amountInvalid && amount.length > 0}
              onChange={(event) => setAmount(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') handleConfirm()
              }}
            />
            <FieldDescription>
              {isOverRepayment
                ? t('Repayment cannot exceed outstanding credit.')
                : `${t('Balance after operation')}: ${formatQuota(balanceAfter)} · ${t('Outstanding credit after operation')}: ${formatQuota(creditAfter)}`}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor='credit-quota-remark'>{t('Remark')}</FieldLabel>
            <Textarea
              id='credit-quota-remark'
              value={remark}
              maxLength={255}
              rows={3}
              placeholder={t('Optional credit operation note')}
              onChange={(event) => setRemark(event.target.value)}
            />
          </Field>
        </FieldGroup>

        <DialogFooter>
          <Button
            variant='outline'
            onClick={() => handleOpenChange(false)}
            disabled={loading}
          >
            {t('Cancel')}
          </Button>
          <Button onClick={handleConfirm} disabled={loading || amountInvalid}>
            {loading && <Spinner data-icon='inline-start' />}
            {loading
              ? t('Processing...')
              : operation === 'grant'
                ? t('Grant credit')
                : t('Record repayment')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
