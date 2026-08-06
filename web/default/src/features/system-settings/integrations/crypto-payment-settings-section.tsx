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
import * as React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Blockchain01Icon,
  FloppyDiskIcon,
  Loading03Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
  FieldTitle,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { getCryptoPaymentConfig, saveCryptoPaymentConfig } from '../api'
import type { CryptoPaymentConfig } from '../types'

const DEFAULT_CONFIG: CryptoPaymentConfig = {
  enabled: false,
  order_expire_minutes: 30,
  unique_amount_digits: 3,
  evm_enabled: false,
  evm_network_name: 'EVM',
  evm_rpc_url: '',
  evm_rpc_configured: false,
  evm_chain_id: 1,
  evm_wallet_address: '',
  evm_token_contract: '',
  evm_token_symbol: 'USDT',
  evm_token_decimals: 6,
  evm_token_per_usd: '1',
  evm_confirmations: 12,
  tron_enabled: false,
  tron_network_name: 'TRON',
  tron_api_url: 'https://api.trongrid.io',
  tron_api_key: '',
  tron_api_key_configured: false,
  tron_wallet_address: '',
  tron_token_contract: '',
  tron_token_symbol: 'USDT',
  tron_token_decimals: 6,
  tron_token_per_usd: '1',
  tron_confirmation_seconds: 60,
}

type CryptoPaymentSettingsSectionProps = {
  disabled?: boolean
}

type TextConfigKey = {
  [K in keyof CryptoPaymentConfig]: CryptoPaymentConfig[K] extends string
    ? K
    : never
}[keyof CryptoPaymentConfig]

type NumberConfigKey = {
  [K in keyof CryptoPaymentConfig]: CryptoPaymentConfig[K] extends number
    ? K
    : never
}[keyof CryptoPaymentConfig]

export function CryptoPaymentSettingsSection({
  disabled = false,
}: CryptoPaymentSettingsSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [config, setConfig] = React.useState(DEFAULT_CONFIG)

  const configQuery = useQuery({
    queryKey: ['crypto-payment-config'],
    queryFn: getCryptoPaymentConfig,
  })

  React.useEffect(() => {
    if (configQuery.data?.success && configQuery.data.data) {
      setConfig({ ...DEFAULT_CONFIG, ...configQuery.data.data })
    }
  }, [configQuery.data])

  const saveMutation = useMutation({
    mutationFn: saveCryptoPaymentConfig,
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        toast.error(response.message || t('Failed to update setting'))
        return
      }
      setConfig({ ...DEFAULT_CONFIG, ...response.data })
      queryClient.invalidateQueries({ queryKey: ['system-options'] })
      queryClient.invalidateQueries({ queryKey: ['crypto-payment-config'] })
      toast.success(t('Setting updated successfully'))
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to update setting'))
    },
  })

  const setText = (key: TextConfigKey, value: string) => {
    setConfig((current) => ({ ...current, [key]: value }))
  }

  const setNumber = (key: NumberConfigKey, value: number) => {
    setConfig((current) => ({
      ...current,
      [key]: Number.isFinite(value) ? value : 0,
    }))
  }

  if (configQuery.isLoading) {
    return (
      <div className='flex flex-col gap-4'>
        <Skeleton className='h-7 w-48' />
        <Skeleton className='h-32 w-full' />
        <Skeleton className='h-64 w-full' />
      </div>
    )
  }

  if (configQuery.isError) {
    return (
      <Alert variant='destructive'>
        <AlertTitle>{t('Failed to load')}</AlertTitle>
        <AlertDescription className='mt-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => configQuery.refetch()}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <section className='flex flex-col gap-5'>
      <div className='flex items-start gap-3'>
        <HugeiconsIcon icon={Blockchain01Icon} className='mt-0.5 size-5' />
        <div className='flex flex-col gap-1'>
          <h3 className='text-lg font-medium'>{t('Direct Web3 Payment')}</h3>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Accept direct ERC-20 and TRC-20 transfers without an external checkout SDK.'
            )}
          </p>
        </div>
      </div>

      <Alert>
        <AlertTitle>{t('Exact amount matching')}</AlertTitle>
        <AlertDescription>
          {t(
            'Each order receives a small unique decimal suffix. Funds are credited only after the configured chain, token contract, recipient, exact amount, and confirmation rules all match.'
          )}
        </AlertDescription>
      </Alert>

      <Card>
        <CardHeader>
          <CardTitle>{t('Payment availability')}</CardTitle>
          <CardDescription>
            {t('Control order lifetime and automatic payment matching.')}
          </CardDescription>
          <CardAction>
            <Switch
              checked={config.enabled}
              onCheckedChange={(checked) =>
                setConfig((current) => ({ ...current, enabled: checked }))
              }
              disabled={disabled}
              aria-label={t('Enable direct Web3 payment')}
            />
          </CardAction>
        </CardHeader>
        <CardContent>
          <FieldGroup className='md:grid md:grid-cols-2'>
            <Field>
              <FieldLabel htmlFor='crypto-order-expire'>
                {t('Order expiry (minutes)')}
              </FieldLabel>
              <Input
                id='crypto-order-expire'
                type='number'
                min={5}
                max={1440}
                value={config.order_expire_minutes}
                onChange={(event) =>
                  setNumber('order_expire_minutes', event.target.valueAsNumber)
                }
                disabled={disabled}
              />
              <FieldDescription>
                {t('Late payments are not credited automatically.')}
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor='crypto-unique-digits'>
                {t('Payment amount decimal places')}
              </FieldLabel>
              <Input
                id='crypto-unique-digits'
                type='number'
                min={1}
                max={3}
                value={config.unique_amount_digits}
                onChange={(event) =>
                  setNumber('unique_amount_digits', event.target.valueAsNumber)
                }
                disabled={disabled}
              />
              <FieldDescription>
                {t(
                  'Three decimal places use the smallest available amount, from .001 to .999.'
                )}
              </FieldDescription>
            </Field>
          </FieldGroup>
        </CardContent>
      </Card>

      <div className='grid gap-5 xl:grid-cols-2'>
        <Card>
          <CardHeader>
            <CardTitle>{t('EVM / ERC-20')}</CardTitle>
            <CardDescription>
              {t('Use any EVM-compatible JSON-RPC endpoint and ERC-20 token.')}
            </CardDescription>
            <CardAction>
              <Switch
                checked={config.evm_enabled}
                onCheckedChange={(checked) =>
                  setConfig((current) => ({
                    ...current,
                    evm_enabled: checked,
                  }))
                }
                disabled={disabled}
                aria-label={t('Enable EVM payments')}
              />
            </CardAction>
          </CardHeader>
          <CardContent>
            <FieldSet disabled={disabled}>
              <FieldLegend className='sr-only'>{t('EVM settings')}</FieldLegend>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor='crypto-evm-network'>
                    {t('Network name')}
                  </FieldLabel>
                  <Input
                    id='crypto-evm-network'
                    value={config.evm_network_name}
                    onChange={(event) =>
                      setText('evm_network_name', event.target.value)
                    }
                    placeholder='Base'
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor='crypto-evm-rpc'>
                    {t('Backend RPC URLs')}
                  </FieldLabel>
                  <Textarea
                    id='crypto-evm-rpc'
                    value={config.evm_rpc_url}
                    onChange={(event) =>
                      setText('evm_rpc_url', event.target.value)
                    }
                    placeholder={
                      config.evm_rpc_configured
                        ? t('Configured — leave blank to keep unchanged')
                        : 'https://...'
                    }
                    autoComplete='new-password'
                    rows={3}
                  />
                  <FieldDescription>
                    {t(
                      'Enter one RPC URL per line. The scanner automatically tries fallback endpoints in order. URLs are stored on the backend and never returned to users.'
                    )}
                  </FieldDescription>
                </Field>
                <FieldGroup className='md:grid md:grid-cols-2'>
                  <Field>
                    <FieldLabel htmlFor='crypto-evm-chain-id'>
                      {t('Chain ID')}
                    </FieldLabel>
                    <Input
                      id='crypto-evm-chain-id'
                      type='number'
                      min={1}
                      value={config.evm_chain_id}
                      onChange={(event) =>
                        setNumber('evm_chain_id', event.target.valueAsNumber)
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor='crypto-evm-confirmations'>
                      {t('Confirmations')}
                    </FieldLabel>
                    <Input
                      id='crypto-evm-confirmations'
                      type='number'
                      min={1}
                      value={config.evm_confirmations}
                      onChange={(event) =>
                        setNumber(
                          'evm_confirmations',
                          event.target.valueAsNumber
                        )
                      }
                    />
                  </Field>
                </FieldGroup>
                <Field>
                  <FieldLabel htmlFor='crypto-evm-wallet'>
                    {t('Receiving wallet address')}
                  </FieldLabel>
                  <Input
                    id='crypto-evm-wallet'
                    value={config.evm_wallet_address}
                    onChange={(event) =>
                      setText('evm_wallet_address', event.target.value)
                    }
                    placeholder='0x...'
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor='crypto-evm-contract'>
                    {t('Token contract')}
                  </FieldLabel>
                  <Input
                    id='crypto-evm-contract'
                    value={config.evm_token_contract}
                    onChange={(event) =>
                      setText('evm_token_contract', event.target.value)
                    }
                    placeholder='0x...'
                  />
                </Field>
                <FieldGroup className='md:grid md:grid-cols-3'>
                  <Field>
                    <FieldLabel htmlFor='crypto-evm-symbol'>
                      {t('Token symbol')}
                    </FieldLabel>
                    <Input
                      id='crypto-evm-symbol'
                      value={config.evm_token_symbol}
                      onChange={(event) =>
                        setText('evm_token_symbol', event.target.value)
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor='crypto-evm-decimals'>
                      {t('Decimals')}
                    </FieldLabel>
                    <Input
                      id='crypto-evm-decimals'
                      type='number'
                      min={1}
                      max={36}
                      value={config.evm_token_decimals}
                      onChange={(event) =>
                        setNumber(
                          'evm_token_decimals',
                          event.target.valueAsNumber
                        )
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor='crypto-evm-rate'>
                      {t('Token per USD')}
                    </FieldLabel>
                    <Input
                      id='crypto-evm-rate'
                      inputMode='decimal'
                      value={config.evm_token_per_usd}
                      onChange={(event) =>
                        setText('evm_token_per_usd', event.target.value)
                      }
                    />
                  </Field>
                </FieldGroup>
              </FieldGroup>
            </FieldSet>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('TRON / TRC-20')}</CardTitle>
            <CardDescription>
              {t('Use a TronGrid-compatible API or self-hosted event service.')}
            </CardDescription>
            <CardAction>
              <Switch
                checked={config.tron_enabled}
                onCheckedChange={(checked) =>
                  setConfig((current) => ({
                    ...current,
                    tron_enabled: checked,
                  }))
                }
                disabled={disabled}
                aria-label={t('Enable TRON payments')}
              />
            </CardAction>
          </CardHeader>
          <CardContent>
            <FieldSet disabled={disabled}>
              <FieldLegend className='sr-only'>
                {t('TRON settings')}
              </FieldLegend>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor='crypto-tron-network'>
                    {t('Network name')}
                  </FieldLabel>
                  <Input
                    id='crypto-tron-network'
                    value={config.tron_network_name}
                    onChange={(event) =>
                      setText('tron_network_name', event.target.value)
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor='crypto-tron-api'>
                    {t('TRON API URL')}
                  </FieldLabel>
                  <Input
                    id='crypto-tron-api'
                    value={config.tron_api_url}
                    onChange={(event) =>
                      setText('tron_api_url', event.target.value)
                    }
                    placeholder='https://api.trongrid.io'
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor='crypto-tron-key'>
                    {t('TRON API key')}
                  </FieldLabel>
                  <Input
                    id='crypto-tron-key'
                    type='password'
                    value={config.tron_api_key}
                    onChange={(event) =>
                      setText('tron_api_key', event.target.value)
                    }
                    placeholder={
                      config.tron_api_key_configured
                        ? t('Configured — leave blank to keep unchanged')
                        : t('Optional')
                    }
                    autoComplete='new-password'
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor='crypto-tron-wallet'>
                    {t('Receiving wallet address')}
                  </FieldLabel>
                  <Input
                    id='crypto-tron-wallet'
                    value={config.tron_wallet_address}
                    onChange={(event) =>
                      setText('tron_wallet_address', event.target.value)
                    }
                    placeholder='T...'
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor='crypto-tron-contract'>
                    {t('Token contract')}
                  </FieldLabel>
                  <Input
                    id='crypto-tron-contract'
                    value={config.tron_token_contract}
                    onChange={(event) =>
                      setText('tron_token_contract', event.target.value)
                    }
                    placeholder='T...'
                  />
                </Field>
                <FieldGroup className='md:grid md:grid-cols-3'>
                  <Field>
                    <FieldLabel htmlFor='crypto-tron-symbol'>
                      {t('Token symbol')}
                    </FieldLabel>
                    <Input
                      id='crypto-tron-symbol'
                      value={config.tron_token_symbol}
                      onChange={(event) =>
                        setText('tron_token_symbol', event.target.value)
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor='crypto-tron-decimals'>
                      {t('Decimals')}
                    </FieldLabel>
                    <Input
                      id='crypto-tron-decimals'
                      type='number'
                      min={1}
                      max={36}
                      value={config.tron_token_decimals}
                      onChange={(event) =>
                        setNumber(
                          'tron_token_decimals',
                          event.target.valueAsNumber
                        )
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor='crypto-tron-rate'>
                      {t('Token per USD')}
                    </FieldLabel>
                    <Input
                      id='crypto-tron-rate'
                      inputMode='decimal'
                      value={config.tron_token_per_usd}
                      onChange={(event) =>
                        setText('tron_token_per_usd', event.target.value)
                      }
                    />
                  </Field>
                </FieldGroup>
                <Field>
                  <FieldLabel htmlFor='crypto-tron-delay'>
                    {t('Confirmation delay (seconds)')}
                  </FieldLabel>
                  <Input
                    id='crypto-tron-delay'
                    type='number'
                    min={0}
                    max={3600}
                    value={config.tron_confirmation_seconds}
                    onChange={(event) =>
                      setNumber(
                        'tron_confirmation_seconds',
                        event.target.valueAsNumber
                      )
                    }
                  />
                  <FieldDescription>
                    {t(
                      'Applied after the API reports the transfer as confirmed.'
                    )}
                  </FieldDescription>
                </Field>
              </FieldGroup>
            </FieldSet>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardContent>
          <Field orientation='horizontal' data-disabled={disabled}>
            <FieldContent>
              <FieldTitle>{t('Backend-only settlement')}</FieldTitle>
              <FieldDescription>
                {t(
                  'Wallet addresses and token contracts are public. RPC credentials remain backend-only, and no receiving private key is required.'
                )}
              </FieldDescription>
            </FieldContent>
          </Field>
        </CardContent>
        <CardFooter className='justify-end'>
          <Button
            type='button'
            onClick={() => saveMutation.mutate(config)}
            disabled={disabled || saveMutation.isPending}
          >
            {saveMutation.isPending ? (
              <HugeiconsIcon
                icon={Loading03Icon}
                data-icon='inline-start'
                className='animate-spin'
              />
            ) : (
              <HugeiconsIcon icon={FloppyDiskIcon} data-icon='inline-start' />
            )}
            {saveMutation.isPending
              ? t('Saving...')
              : t('Save Web3 payment settings')}
          </Button>
        </CardFooter>
      </Card>
    </section>
  )
}
