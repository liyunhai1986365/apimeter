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
import * as z from 'zod'
import type { Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { normalizeGoogleAnalyticsId } from '@/lib/google-analytics'
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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

const _systemInfoSchema = z.object({
  theme: z.object({
    frontend: z.enum(['default', 'classic']),
  }),
  SystemName: z.string().min(1),
  ServerAddress: z.string().optional(),
  Logo: z.string().url().optional().or(z.literal('')),
  FooterCompanyName: z.string().max(120).optional(),
  Footer: z.string().optional(),
  About: z.string().optional(),
  HomePageContent: z.string().optional(),
  CustomerServiceScript: z.string().optional(),
  GoogleAnalyticsId: z.string().optional(),
  legal: z.object({
    user_agreement: z.string().optional(),
    privacy_policy: z.string().optional(),
  }),
})

type SystemInfoFormValues = z.infer<typeof _systemInfoSchema>

type SystemInfoSectionProps = {
  defaultValues: SystemInfoFormValues
}

function normalizeValue(value: unknown): string {
  if (value === undefined || value === null) return ''
  return typeof value === 'string' ? value : String(value)
}

export function SystemInfoSection({ defaultValues }: SystemInfoSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const normalizedDefaults: SystemInfoFormValues = {
    theme: {
      frontend:
        defaultValues.theme?.frontend === 'classic' ? 'classic' : 'default',
    },
    SystemName: normalizeValue(defaultValues.SystemName),
    ServerAddress: normalizeValue(defaultValues.ServerAddress),
    Logo: normalizeValue(defaultValues.Logo),
    FooterCompanyName: normalizeValue(defaultValues.FooterCompanyName),
    Footer: normalizeValue(defaultValues.Footer),
    About: normalizeValue(defaultValues.About),
    HomePageContent: normalizeValue(defaultValues.HomePageContent),
    CustomerServiceScript: normalizeValue(defaultValues.CustomerServiceScript),
    GoogleAnalyticsId: normalizeValue(defaultValues.GoogleAnalyticsId),
    legal: {
      user_agreement: normalizeValue(defaultValues.legal?.user_agreement),
      privacy_policy: normalizeValue(defaultValues.legal?.privacy_policy),
    },
  }

  const systemInfoSchemaWithI18n = z.object({
    theme: z.object({
      frontend: z.enum(['default', 'classic']),
    }),
    SystemName: z.string().min(1, {
      error: () => t('System name is required'),
    }),
    ServerAddress: z.string().optional(),
    Logo: z.string().url().optional().or(z.literal('')),
    FooterCompanyName: z.string().max(120, {
      error: () => t('Footer company name must be 120 characters or fewer'),
    }),
    Footer: z.string().optional(),
    About: z.string().optional(),
    HomePageContent: z.string().optional(),
    CustomerServiceScript: z.string().optional(),
    GoogleAnalyticsId: z
      .string()
      .refine(
        (value) => !value.trim() || Boolean(normalizeGoogleAnalyticsId(value)),
        {
          error: () =>
            t(
              'Enter a valid Google Analytics Measurement ID, such as G-6B94BX72EW'
            ),
        }
      ),
    legal: z.object({
      user_agreement: z.string().optional(),
      privacy_policy: z.string().optional(),
    }),
  })

  const { form, handleSubmit, handleReset, isDirty, isSubmitting } =
    useSettingsForm<SystemInfoFormValues>({
      resolver: zodResolver(systemInfoSchemaWithI18n) as Resolver<
        SystemInfoFormValues,
        unknown,
        SystemInfoFormValues
      >,
      defaultValues: normalizedDefaults,
      onSubmit: async (_data, changedFields) => {
        for (const [key, value] of Object.entries(changedFields)) {
          let v = normalizeValue(value)
          if (key === 'ServerAddress') {
            v = v.replace(/\/+$/, '')
          }
          if (key === 'GoogleAnalyticsId') {
            v = normalizeGoogleAnalyticsId(v)
          }
          if (key === 'FooterCompanyName') {
            v = v.trim()
          }
          await updateOption.mutateAsync({
            key,
            value: v,
          })
        }
      },
    })

  return (
    <>
      <FormNavigationGuard when={isDirty} />

      <SettingsSection
        title={t('System Information')}
        description={t('Configure basic system information and branding')}
      >
        <Form {...form}>
          <form onSubmit={handleSubmit} className='space-y-6'>
            <FormDirtyIndicator isDirty={isDirty} />
            <FormField
              control={form.control}
              name='theme.frontend'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Frontend Theme')}</FormLabel>
                  <Select
                    items={[
                      { value: 'default', label: t('Default (New Frontend)') },
                      {
                        value: 'classic',
                        label: t('Classic (Legacy Frontend)'),
                      },
                    ]}
                    onValueChange={field.onChange}
                    value={field.value}
                  >
                    <FormControl>
                      <SelectTrigger className='w-full'>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        <SelectItem value='default'>
                          {t('Default (New Frontend)')}
                        </SelectItem>
                        <SelectItem value='classic'>
                          {t('Classic (Legacy Frontend)')}
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {t(
                      'Switch between the new frontend and the classic frontend. Changes take effect after page reload.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='SystemName'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('System Name')}</FormLabel>
                  <FormControl>
                    <Input placeholder={t('New API')} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t('The name displayed across the application')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ServerAddress'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Server Address')}</FormLabel>
                  <FormControl>
                    <Input placeholder='https://yourdomain.com' {...field} />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'The public URL of your server, used for OAuth callbacks, webhooks, and other external integrations'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='Logo'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Logo URL')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('https://example.com/logo.png')}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('URL to your logo image (optional)')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='FooterCompanyName'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Footer company name')}</FormLabel>
                  <FormControl>
                    <Input placeholder='Axiom Mesh Inc.' {...field} />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Displayed after the year in the default footer. Leave empty to hide the company name; custom footer HTML replaces the default footer.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='Footer'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Footer')}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder={t(
                        '© 2025 Your Company. All rights reserved.'
                      )}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Footer text displayed at the bottom of pages')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='About'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('About')}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder={t(
                        'Enter HTML code (e.g., <p>About us...</p>) or a URL (e.g., https://example.com) to embed as iframe'
                      )}
                      rows={4}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Supports HTML markup or iframe embedding. Enter HTML code directly, or provide a complete URL to automatically embed it as an iframe.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='HomePageContent'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Home Page Content')}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder={t('Welcome to our New API...')}
                      rows={6}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Content displayed on the home page (supports Markdown)'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='CustomerServiceScript'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Customer Service Script')}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder='<script src="//code.tidio.co/your-id.js" async></script>'
                      rows={4}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Paste the customer service script code. The frontend extracts and loads the script src globally; leave empty to disable it.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='GoogleAnalyticsId'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Google Analytics Measurement ID')}</FormLabel>
                  <FormControl>
                    <Input placeholder='G-6B94BX72EW' {...field} />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Enter a GA4 Measurement ID to load Google Analytics on every page. Leave empty to disable it.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='legal.user_agreement'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('User Agreement')}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder={t(
                        'Provide Markdown, HTML, or an external URL for the user agreement'
                      )}
                      rows={6}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Leave empty to disable the agreement requirement. Supports Markdown, HTML, or a full URL to redirect users.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='legal.privacy_policy'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Privacy Policy')}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder={t(
                        'Provide Markdown, HTML, or an external URL for the privacy policy'
                      )}
                      rows={6}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Leave empty to disable the privacy policy requirement. Supports Markdown, HTML, or a full URL to redirect users.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='flex gap-2'>
              <Button
                type='submit'
                disabled={isSubmitting || updateOption.isPending}
              >
                {updateOption.isPending ? t('Saving...') : t('Save Changes')}
              </Button>
              <Button
                type='button'
                variant='outline'
                onClick={handleReset}
                disabled={!isDirty || updateOption.isPending || isSubmitting}
              >
                <RotateCcw className='mr-2 h-4 w-4' />
                {t('Reset')}
              </Button>
            </div>
          </form>
        </Form>
      </SettingsSection>
    </>
  )
}
