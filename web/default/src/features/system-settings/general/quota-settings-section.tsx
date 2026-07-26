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
import type { ChangeEvent } from 'react'
import * as z from 'zod'
import { useFieldArray, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Add01Icon, Delete02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

const quotaSchema = z.object({
  QuotaForNewUser: z.coerce.number().min(0),
  PreConsumedQuota: z.coerce.number().min(0),
  QuotaForInviter: z.coerce.number().min(0),
  QuotaForInvitee: z.coerce.number().min(0),
  AffiliateTopUpRewardRatio: z.coerce.number().min(0).max(100),
  AffiliateTopUpRewardLimit: z.coerce.number().int().min(0),
  AffiliateConsumeRewardRatio: z.coerce.number().min(0).max(100),
  AffiliateRoleConfigs: z.array(
    z.object({
      id: z.string().min(1),
      name: z.string().trim().min(1).max(128),
      topup_reward_ratio: z.coerce.number().min(0).max(100).nullish(),
      topup_reward_limit: z.coerce.number().int().min(0).nullish(),
      consume_reward_ratio: z.coerce.number().min(0).max(100).nullish(),
      inviter_reward_quota: z.coerce.number().int().min(0).nullish(),
      invitee_reward_quota: z.coerce.number().int().min(0).nullish(),
    })
  ),
  TopUpLink: z.string(),
  general_setting: z.object({
    docs_link: z.string(),
  }),
  quota_setting: z.object({
    enable_free_model_pre_consume: z.boolean(),
  }),
})

type QuotaFormValues = z.infer<typeof quotaSchema>

type QuotaSettingsSectionProps = {
  defaultValues: QuotaFormValues
  complianceConfirmed?: boolean
}

export function QuotaSettingsSection({
  defaultValues,
  complianceConfirmed = true,
}: QuotaSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const handleNumberChange =
    (onChange: (value: number | string) => void) =>
    (event: ChangeEvent<HTMLInputElement>) => {
      onChange(
        event.target.value === '' ? '' : event.currentTarget.valueAsNumber
      )
    }

  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<QuotaFormValues>({
      resolver: zodResolver(quotaSchema) as Resolver<
        QuotaFormValues,
        unknown,
        QuotaFormValues
      >,
      defaultValues,
      onSubmit: async (_data, changedFields) => {
        for (const [key, value] of Object.entries(changedFields)) {
          await updateOption.mutateAsync({
            key,
            value:
              key === 'AffiliateRoleConfigs'
                ? JSON.stringify(value)
                : (value as string | number | boolean),
          })
        }
      },
    })

  const affiliateRoles = useFieldArray({
    control: form.control,
    name: 'AffiliateRoleConfigs',
    keyName: 'fieldId',
  })

  const handleOptionalNumberChange =
    (onChange: (value: number | undefined) => void) =>
    (event: ChangeEvent<HTMLInputElement>) => {
      onChange(
        event.target.value === ''
          ? undefined
          : event.currentTarget.valueAsNumber
      )
    }

  return (
    <SettingsSection
      title={t('Quota Settings')}
      description={t('Configure user quota allocation and rewards')}
    >
      <FormNavigationGuard when={isDirty} />

      {!complianceConfirmed ? (
        <Alert variant='destructive'>
          <AlertDescription>
            {t(
              'Non-zero invitation rewards require compliance confirmation in Payment Gateway settings.'
            )}
          </AlertDescription>
        </Alert>
      ) : null}

      <Form {...form}>
        <form onSubmit={handleSubmit} className='space-y-6'>
          <FormDirtyIndicator isDirty={isDirty} />
          <FormField
            control={form.control}
            name='QuotaForNewUser'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('New User Quota')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    value={field.value ?? ''}
                    onChange={handleNumberChange(field.onChange)}
                    name={field.name}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t('Initial quota given to new users')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='PreConsumedQuota'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Pre-Consumed Quota')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    value={field.value ?? ''}
                    onChange={handleNumberChange(field.onChange)}
                    name={field.name}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t('Quota consumed before charging users')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='QuotaForInviter'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Inviter Reward')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    value={field.value ?? ''}
                    onChange={handleNumberChange(field.onChange)}
                    name={field.name}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t('Quota given to users who invite others')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='QuotaForInvitee'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Invitee Reward')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    value={field.value ?? ''}
                    onChange={handleNumberChange(field.onChange)}
                    name={field.name}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t('Quota given to invited users')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='AffiliateTopUpRewardRatio'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Invited Top-Up Reward Rate')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    max={100}
                    step='0.01'
                    value={field.value ?? ''}
                    onChange={handleNumberChange(field.onChange)}
                    name={field.name}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Percentage of an invited user top-up awarded to the inviter after 24 hours. Set 0 to disable.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='AffiliateTopUpRewardLimit'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Invited Top-Up Reward Count Limit')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    step='1'
                    value={field.value ?? ''}
                    onChange={handleNumberChange(field.onChange)}
                    name={field.name}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Maximum rewarded top-ups per invited user. Set 0 for no limit.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='AffiliateConsumeRewardRatio'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Invited Consumption Reward Rate')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    max={100}
                    step='0.01'
                    value={field.value ?? ''}
                    onChange={handleNumberChange(field.onChange)}
                    name={field.name}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    "Percentage of each invited user's net consumption awarded to the inviter after daily settlement. Set 0 to disable."
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='flex flex-col gap-4 border-t pt-6'>
            <div className='flex flex-wrap items-start justify-between gap-3'>
              <div className='flex flex-col gap-1'>
                <h3 className='text-sm font-semibold'>
                  {t('Distributor Roles')}
                </h3>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Configure role-specific referral policies. Empty fields inherit the system defaults above.'
                  )}
                </p>
              </div>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() =>
                  affiliateRoles.append({
                    id: `affiliate-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`,
                    name: '',
                  })
                }
              >
                <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
                {t('Add distributor role')}
              </Button>
            </div>

            {affiliateRoles.fields.length === 0 ? (
              <div className='text-muted-foreground rounded-lg border border-dashed px-4 py-6 text-center text-sm'>
                {t(
                  'No distributor roles configured. All users use system defaults.'
                )}
              </div>
            ) : (
              <div className='flex flex-col gap-3'>
                {affiliateRoles.fields.map((role, index) => (
                  <div
                    key={role.fieldId}
                    className='flex flex-col gap-4 rounded-lg border p-4'
                  >
                    <div className='flex items-start gap-3'>
                      <FormField
                        control={form.control}
                        name={`AffiliateRoleConfigs.${index}.name`}
                        render={({ field }) => (
                          <FormItem className='min-w-0 flex-1'>
                            <FormLabel>{t('Role name')}</FormLabel>
                            <FormControl>
                              <Input
                                {...field}
                                placeholder={t('e.g. Partner')}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        className='mt-6 shrink-0'
                        aria-label={t('Delete distributor role')}
                        onClick={() => affiliateRoles.remove(index)}
                      >
                        <HugeiconsIcon icon={Delete02Icon} />
                      </Button>
                    </div>

                    <div className='grid gap-4 sm:grid-cols-2'>
                      <FormField
                        control={form.control}
                        name={`AffiliateRoleConfigs.${index}.topup_reward_ratio`}
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Top-up reward rate (%)')}</FormLabel>
                            <FormControl>
                              <Input
                                type='number'
                                min={0}
                                max={100}
                                step='0.01'
                                value={field.value ?? ''}
                                onChange={handleOptionalNumberChange(
                                  field.onChange
                                )}
                                placeholder={String(
                                  form.getValues('AffiliateTopUpRewardRatio')
                                )}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name={`AffiliateRoleConfigs.${index}.topup_reward_limit`}
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Rewarded top-up count')}</FormLabel>
                            <FormControl>
                              <Input
                                type='number'
                                min={0}
                                step='1'
                                value={field.value ?? ''}
                                onChange={handleOptionalNumberChange(
                                  field.onChange
                                )}
                                placeholder={String(
                                  form.getValues('AffiliateTopUpRewardLimit')
                                )}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name={`AffiliateRoleConfigs.${index}.consume_reward_ratio`}
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {t('Consumption reward rate (%)')}
                            </FormLabel>
                            <FormControl>
                              <Input
                                type='number'
                                min={0}
                                max={100}
                                step='0.01'
                                value={field.value ?? ''}
                                onChange={handleOptionalNumberChange(
                                  field.onChange
                                )}
                                placeholder={String(
                                  form.getValues('AffiliateConsumeRewardRatio')
                                )}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name={`AffiliateRoleConfigs.${index}.inviter_reward_quota`}
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {t('Inviter registration reward')}
                            </FormLabel>
                            <FormControl>
                              <Input
                                type='number'
                                min={0}
                                step='1'
                                value={field.value ?? ''}
                                onChange={handleOptionalNumberChange(
                                  field.onChange
                                )}
                                placeholder={String(
                                  form.getValues('QuotaForInviter')
                                )}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name={`AffiliateRoleConfigs.${index}.invitee_reward_quota`}
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {t('Invitee registration reward')}
                            </FormLabel>
                            <FormControl>
                              <Input
                                type='number'
                                min={0}
                                step='1'
                                value={field.value ?? ''}
                                onChange={handleOptionalNumberChange(
                                  field.onChange
                                )}
                                placeholder={String(
                                  form.getValues('QuotaForInvitee')
                                )}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          <FormField
            control={form.control}
            name='quota_setting.enable_free_model_pre_consume'
            render={({ field }) => (
              <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                <div className='space-y-0.5'>
                  <FormLabel className='text-base'>
                    {t('Pre-Consume for Free Models')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'When enabled, zero-cost models also pre-consume quota before final settlement.'
                    )}
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={updateOption.isPending}
                  />
                </FormControl>
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='TopUpLink'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Top-Up Link')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('https://example.com/topup')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('External link for users to purchase quota')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='general_setting.docs_link'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Documentation Link')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('https://docs.example.com')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('Link to your documentation site')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <Button
            type='submit'
            disabled={updateOption.isPending || isSubmitting}
          >
            {updateOption.isPending ? t('Saving...') : t('Save Changes')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}
