/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
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
  FieldLabel,
} from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

type MainlandPresentationSectionProps = {
  defaultValue: boolean
}

export function MainlandPresentationSection({
  defaultValue,
}: MainlandPresentationSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [enabled, setEnabled] = useState(defaultValue)

  const handleSave = async () => {
    if (enabled === defaultValue) return
    await updateOption.mutateAsync({
      key: 'MainlandChinaPresentationEnabled',
      value: enabled,
    })
  }

  return (
    <SettingsSection
      title={t('Mainland China presentation')}
      description={t(
        'Configure the model brands and examples shown on public introduction pages.'
      )}
    >
      <Card>
        <CardHeader>
          <CardTitle>{t('Domestic model presentation')}</CardTitle>
          <CardDescription>
            {t(
              'Use domestic model brands and neutral compatibility descriptions on the home page, subscription page, and related static introductions.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Field orientation='horizontal'>
            <FieldContent>
              <FieldLabel htmlFor='mainland-china-presentation'>
                {t('Enable mainland China presentation')}
              </FieldLabel>
              <FieldDescription>
                {t(
                  'This changes public static presentation only. It does not disable channels, models, API routes, or subscription entitlements.'
                )}
              </FieldDescription>
            </FieldContent>
            <Switch
              id='mainland-china-presentation'
              checked={enabled}
              onCheckedChange={setEnabled}
              aria-label={t('Enable mainland China presentation')}
            />
          </Field>
        </CardContent>
        <CardFooter>
          <Button
            type='button'
            onClick={handleSave}
            disabled={updateOption.isPending || enabled === defaultValue}
          >
            {updateOption.isPending ? t('Saving...') : t('Save Changes')}
          </Button>
        </CardFooter>
      </Card>
    </SettingsSection>
  )
}
