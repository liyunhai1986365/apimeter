import { type FormEvent, useState } from 'react'
import { BankIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
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
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  formatWithdrawalAccount,
  type WithdrawalMethod,
} from '../withdrawal-account'

export type WithdrawalDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  availableAmount: number
  minimumAmount: number
  formatAmount: (amount: number) => string
  pending: boolean
  onSubmit: (amount: number, accountInfo: string) => void
}

export function WithdrawalDialog({
  open,
  onOpenChange,
  availableAmount,
  minimumAmount,
  formatAmount,
  pending,
  onSubmit,
}: WithdrawalDialogProps) {
  const { t } = useTranslation()
  const [amount, setAmount] = useState(() => String(minimumAmount))
  const [accountInfo, setAccountInfo] = useState('')
  const [withdrawalMethod, setWithdrawalMethod] =
    useState<WithdrawalMethod>('alipay')
  const [usdtNetwork, setUsdtNetwork] = useState('')

  const numericAmount = Number(amount)
  const amountError =
    amount.trim() === '' ||
    !Number.isFinite(numericAmount) ||
    numericAmount <= 0
      ? t('Amount must be greater than 0')
      : numericAmount < minimumAmount
        ? t('Minimum action amount: {{amount}}', {
            amount: formatAmount(minimumAmount),
          })
        : numericAmount > availableAmount
          ? t('Amount exceeds available balance')
          : null
  const accountInvalid = accountInfo.trim() === ''
  const networkInvalid =
    withdrawalMethod === 'usdt' && usdtNetwork.trim() === ''

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (amountError || accountInvalid || networkInvalid || pending) return
    onSubmit(
      numericAmount,
      formatWithdrawalAccount(withdrawalMethod, accountInfo, usdtNetwork)
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <form
          className='flex flex-col gap-4'
          onSubmit={handleSubmit}
          noValidate
        >
          <DialogHeader>
            <DialogTitle className='flex items-center gap-2'>
              <HugeiconsIcon icon={BankIcon} className='size-5' />
              {t('Request withdrawal')}
            </DialogTitle>
            <DialogDescription>
              {t(
                'Provide an Alipay account or USDT transfer address for administrator review.'
              )}
            </DialogDescription>
          </DialogHeader>

          <FieldGroup className='py-2'>
            <Field
              data-invalid={Boolean(amountError) || undefined}
              data-disabled={pending || undefined}
            >
              <FieldLabel htmlFor='withdrawal-amount'>{t('Amount')}</FieldLabel>
              <Input
                id='withdrawal-amount'
                type='number'
                min={minimumAmount}
                max={availableAmount}
                step='any'
                value={amount}
                onChange={(event) => setAmount(event.target.value)}
                aria-invalid={Boolean(amountError) || undefined}
                disabled={pending}
                autoFocus
              />
              <FieldDescription>
                {t('Available: {{amount}} · Minimum: {{minimum}}', {
                  amount: formatAmount(availableAmount),
                  minimum: formatAmount(minimumAmount),
                })}
              </FieldDescription>
              {amountError ? <FieldError>{amountError}</FieldError> : null}
            </Field>

            <Field data-disabled={pending || undefined}>
              <FieldLabel id='withdrawal-method-label'>
                {t('Withdrawal method')}
              </FieldLabel>
              <ToggleGroup
                value={[withdrawalMethod]}
                onValueChange={(value) => {
                  const next = value[0] as WithdrawalMethod | undefined
                  if (!next || next === withdrawalMethod) return
                  setWithdrawalMethod(next)
                  setAccountInfo('')
                  setUsdtNetwork('')
                }}
                variant='outline'
                spacing={2}
                className='w-full'
                aria-labelledby='withdrawal-method-label'
                disabled={pending}
              >
                <ToggleGroupItem value='alipay' className='flex-1'>
                  {t('Alipay')}
                </ToggleGroupItem>
                <ToggleGroupItem value='usdt' className='flex-1'>
                  USDT
                </ToggleGroupItem>
              </ToggleGroup>
            </Field>

            <Field
              data-invalid={accountInvalid || undefined}
              data-disabled={pending || undefined}
            >
              <FieldLabel htmlFor='withdrawal-account'>
                {withdrawalMethod === 'alipay'
                  ? t('Alipay account')
                  : t('USDT transfer address')}
              </FieldLabel>
              <Input
                id='withdrawal-account'
                value={accountInfo}
                onChange={(event) => setAccountInfo(event.target.value)}
                placeholder={
                  withdrawalMethod === 'alipay'
                    ? t('Enter your Alipay account')
                    : t('Enter your USDT transfer address')
                }
                maxLength={512}
                aria-invalid={accountInvalid || undefined}
                disabled={pending}
              />
              <FieldDescription>
                {t('The administrator will use these details to send payment.')}
              </FieldDescription>
              {accountInvalid ? (
                <FieldError>
                  {withdrawalMethod === 'alipay'
                    ? t('Enter your Alipay account')
                    : t('Enter your USDT transfer address')}
                </FieldError>
              ) : null}
            </Field>

            {withdrawalMethod === 'usdt' ? (
              <Field
                data-invalid={networkInvalid || undefined}
                data-disabled={pending || undefined}
              >
                <FieldLabel htmlFor='withdrawal-network'>
                  {t('USDT network')}
                </FieldLabel>
                <Input
                  id='withdrawal-network'
                  value={usdtNetwork}
                  onChange={(event) => setUsdtNetwork(event.target.value)}
                  placeholder={t('For example: TRC20')}
                  maxLength={64}
                  aria-invalid={networkInvalid || undefined}
                  disabled={pending}
                />
                {networkInvalid ? (
                  <FieldError>
                    {t('Enter the USDT transfer network')}
                  </FieldError>
                ) : null}
              </Field>
            ) : null}
          </FieldGroup>

          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={() => onOpenChange(false)}
              disabled={pending}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='submit'
              disabled={
                Boolean(amountError) ||
                accountInvalid ||
                networkInvalid ||
                pending
              }
            >
              {pending ? <Spinner data-icon='inline-start' /> : null}
              {t('Submit Withdrawal')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
