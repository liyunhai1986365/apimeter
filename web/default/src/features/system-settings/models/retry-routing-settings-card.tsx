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
import { useEffect, useMemo, useState } from 'react'
import { Braces, RotateCcw, Save } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import {
  insertRetryPolicyTemplate,
  RETRY_POLICY_TEMPLATES,
} from '@/features/channels/lib'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const TEMPLATE_PLACEHOLDER_VALUE = '__template_placeholder__'

type RetryRoutingSettingsCardProps = {
  defaultValues: {
    AutomaticRetryPolicyRules: string
  }
}

function normalizePolicyRules(value: string) {
  const raw = (value ?? '').trim()
  if (!raw) return '[]'
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

function compactPolicyRules(value: string) {
  const raw = (value ?? '').trim()
  if (!raw) return '[]'
  return JSON.stringify(JSON.parse(raw))
}

export function RetryRoutingSettingsCard({
  defaultValues,
}: RetryRoutingSettingsCardProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const initialPolicy = useMemo(
    () => normalizePolicyRules(defaultValues.AutomaticRetryPolicyRules ?? '[]'),
    [defaultValues.AutomaticRetryPolicyRules]
  )
  const [policy, setPolicy] = useState(initialPolicy)

  useEffect(() => {
    setPolicy(initialPolicy)
  }, [initialPolicy])

  const parsedRules = useMemo(() => {
    try {
      const parsed = JSON.parse(policy || '[]')
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }, [policy])

  const handleFormat = () => {
    try {
      setPolicy(JSON.stringify(JSON.parse(policy || '[]'), null, 2))
    } catch {
      toast.error(t('Invalid JSON format'))
    }
  }

  const handleSave = async () => {
    let nextValue = '[]'
    try {
      nextValue = compactPolicyRules(policy)
    } catch {
      toast.error(t('Invalid JSON format'))
      return
    }

    let currentValue = '[]'
    try {
      currentValue = compactPolicyRules(initialPolicy)
    } catch {
      currentValue = '[]'
    }

    if (nextValue === currentValue) {
      toast.info(t('No changes to save'))
      return
    }

    await updateOption.mutateAsync({
      key: 'AutomaticRetryPolicyRules',
      value: nextValue,
    })
  }

  return (
    <SettingsSection
      title={t('Retry Routing')}
      description={t(
        'Centralize system retry, failover, and skip-retry rules for model routing.'
      )}
    >
      <div className='flex flex-col gap-4'>
        <Card>
          <CardHeader>
            <CardTitle>{t('System retry routing rules')}</CardTitle>
            <CardDescription>
              {t(
                'Match upstream errors and route the next retry to configured backup groups, channel IDs, or channel tags.'
              )}
            </CardDescription>
          </CardHeader>
          <CardContent className='flex flex-col gap-4'>
            <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
              <div className='text-muted-foreground text-sm'>
                {t('Configured rules')}: {parsedRules.length}
              </div>
              <div className='flex flex-wrap gap-2'>
                <Select
                  value={TEMPLATE_PLACEHOLDER_VALUE}
                  onValueChange={(value) => {
                    if (value === TEMPLATE_PLACEHOLDER_VALUE) return
                    const template = RETRY_POLICY_TEMPLATES.find(
                      (item) => item.labelKey === value
                    )
                    if (template) {
                      setPolicy((current) =>
                        insertRetryPolicyTemplate(current, template)
                      )
                    }
                  }}
                >
                  <SelectTrigger className='h-8 w-44'>
                    <SelectValue placeholder={t('Insert template')} />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      <SelectItem value={TEMPLATE_PLACEHOLDER_VALUE}>
                        {t('Insert template')}
                      </SelectItem>
                      {RETRY_POLICY_TEMPLATES.map((template) => (
                        <SelectItem
                          key={template.labelKey}
                          value={template.labelKey}
                        >
                          {t(template.labelKey)}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={handleFormat}
                >
                  <Braces data-icon='inline-start' />
                  {t('Format JSON')}
                </Button>
              </div>
            </div>

            <Textarea
              rows={14}
              value={policy}
              placeholder='[{"action":"failover","status_codes":"429,500-504","targets":{"groups":["backup"],"channel_tags":["stable"]},"strategy":{"max_retries":2,"exclude_failed_channel":true}}]'
              onChange={(event) => setPolicy(event.target.value)}
            />

            <div className='grid gap-3 text-sm md:grid-cols-3'>
              <div className='bg-muted/30 rounded-md border p-3'>
                <div className='font-medium'>{t('Match errors')}</div>
                <p className='text-muted-foreground mt-1'>
                  {t(
                    'Use models, groups, request_paths, stream, token_ids, workspace_ids, channel_ids, channel_types, status_codes, error_codes, and message_contains.'
                  )}
                </p>
              </div>
              <div className='bg-muted/30 rounded-md border p-3'>
                <div className='font-medium'>{t('Route targets')}</div>
                <p className='text-muted-foreground mt-1'>
                  {t(
                    'Use targets.groups, targets.channel_ids, or targets.channel_tags to choose the next channel pool.'
                  )}
                </p>
              </div>
              <div className='bg-muted/30 rounded-md border p-3'>
                <div className='font-medium'>{t('Hit analytics')}</div>
                <p className='text-muted-foreground mt-1'>
                  {t(
                    'Matched retry, failover, and skip-retry decisions are recorded as retry route events for later optimization.'
                  )}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <div className='flex flex-col gap-2 sm:flex-row'>
          <Button
            type='button'
            onClick={handleSave}
            disabled={updateOption.isPending}
          >
            <Save data-icon='inline-start' />
            {updateOption.isPending ? t('Saving...') : t('Save retry routing')}
          </Button>
          <Button
            type='button'
            variant='outline'
            onClick={() => setPolicy(initialPolicy)}
          >
            <RotateCcw data-icon='inline-start' />
            {t('Reset')}
          </Button>
        </div>
      </div>
    </SettingsSection>
  )
}
