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
import { ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

export type RankingsDataVisibility = 'masked' | 'hidden_exact' | 'exact'

type RankingsVisibilitySectionProps = {
  defaultValue: RankingsDataVisibility
}

const VISIBILITY_OPTIONS: Array<{
  value: RankingsDataVisibility
  label: string
  description: string
}> = [
  {
    value: 'masked',
    label: 'Public masked data',
    description:
      'Everyone sees popularity indexes instead of exact usage volume.',
  },
  {
    value: 'hidden_exact',
    label: 'Admins see exact data',
    description:
      'Admins see exact usage volume; other visitors see popularity indexes.',
  },
  {
    value: 'exact',
    label: 'Public exact data',
    description: 'Everyone can see exact usage volume on the rankings page.',
  },
]

export function RankingsVisibilitySection({
  defaultValue,
}: RankingsVisibilitySectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [value, setValue] = useState<RankingsDataVisibility>(defaultValue)
  const selected = VISIBILITY_OPTIONS.find((item) => item.value === value)

  const onSubmit = async () => {
    if (value === defaultValue) {
      toast.info(t('No changes to save'))
      return
    }
    await updateOption.mutateAsync({
      key: 'RankingsDataVisibility',
      value,
    })
    toast.success(t('Saved successfully'))
  }

  return (
    <SettingsSection
      title={t('Rankings data visibility')}
      description={t(
        'Control whether the public rankings page exposes exact model usage data.'
      )}
    >
      <div className='space-y-4'>
        <div className='grid gap-2'>
          <Label>{t('Data visibility')}</Label>
          <Select
            value={value}
            onValueChange={(next) =>
              setValue(next as RankingsDataVisibility)
            }
          >
            <SelectTrigger className='max-w-md'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {VISIBILITY_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {t(option.label)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className='text-muted-foreground max-w-2xl text-sm'>
            {selected ? t(selected.description) : null}
          </p>
        </div>

        <div className='bg-muted/40 flex items-start gap-3 rounded-lg border p-3'>
          <ShieldCheck className='text-primary mt-0.5 size-4 shrink-0' />
          <p className='text-muted-foreground text-sm'>
            {t(
              'Masked mode keeps rankings useful while preventing visitors from reading exact per-model traffic.'
            )}
          </p>
        </div>

        <Button
          type='button'
          onClick={onSubmit}
          disabled={updateOption.isPending}
        >
          {updateOption.isPending ? t('Saving...') : t('Save visibility')}
        </Button>
      </div>
    </SettingsSection>
  )
}
