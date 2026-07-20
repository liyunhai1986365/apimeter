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
import { Textarea } from '@/components/ui/textarea'

type AgentBalanceDialogProps = {
  open: boolean
  title: string
  description: string
  amount: string
  currency: string
  availableAmount?: string
  remark?: string
  showRemark?: boolean
  isPending: boolean
  onOpenChange: (open: boolean) => void
  onAmountChange: (value: string) => void
  onRemarkChange?: (value: string) => void
  onSubmit: () => void
}

export function AgentBalanceDialog(props: AgentBalanceDialogProps) {
  const { t } = useTranslation()
  const canSubmit = Number(props.amount) > 0

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{props.title}</DialogTitle>
          <DialogDescription>{props.description}</DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor='agent-balance-amount'>
              {t('Amount')} ({props.currency})
            </FieldLabel>
            <Input
              id='agent-balance-amount'
              type='number'
              min='0.01'
              step='0.01'
              value={props.amount}
              onChange={(event) => props.onAmountChange(event.target.value)}
              placeholder='0.00'
            />
            {props.availableAmount ? (
              <FieldDescription>
                {t('Available settlement balance: {{amount}}', {
                  amount: props.availableAmount,
                })}
              </FieldDescription>
            ) : null}
          </Field>
          {props.showRemark ? (
            <Field>
              <FieldLabel htmlFor='agent-balance-remark'>
                {t('Remark')}
              </FieldLabel>
              <Textarea
                id='agent-balance-remark'
                value={props.remark ?? ''}
                onChange={(event) => props.onRemarkChange?.(event.target.value)}
                placeholder={t('Optional funding note')}
              />
            </Field>
          ) : null}
        </FieldGroup>
        <DialogFooter>
          <Button
            variant='outline'
            disabled={props.isPending}
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            disabled={!canSubmit || props.isPending}
            onClick={props.onSubmit}
          >
            {props.isPending ? t('Processing...') : t('Confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
