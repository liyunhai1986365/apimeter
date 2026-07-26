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
import { SlidersHorizontalIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Slider } from '@/components/ui/slider'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { PromptInputButton } from '@/components/ai-elements/prompt-input'
import {
  getParameterValue,
  normalizeParameterNumberValue,
  PLAYGROUND_PARAMETER_CONTROLS,
  type PlaygroundParameterKey,
} from '../lib/playground-parameters'
import type { ParameterEnabled, PlaygroundConfig } from '../types'

type PlaygroundParameterPanelProps = {
  config: PlaygroundConfig
  disabled?: boolean
  onConfigChange: <K extends keyof PlaygroundConfig>(
    key: K,
    value: PlaygroundConfig[K]
  ) => void
  onParameterEnabledChange: (
    key: PlaygroundParameterKey,
    value: boolean
  ) => void
  parameterEnabled: ParameterEnabled
}

export function PlaygroundParameterPanel(props: PlaygroundParameterPanelProps) {
  const { t } = useTranslation()
  const activeCount = PLAYGROUND_PARAMETER_CONTROLS.filter(
    (control) => props.parameterEnabled[control.key]
  ).length

  const updateValue = (key: PlaygroundParameterKey, value: string | number) => {
    const normalized = normalizeParameterNumberValue(key, value)
    if (key === 'seed') {
      props.onConfigChange('seed', normalized)
      return
    }
    props.onConfigChange(key, normalized ?? 0)
  }

  return (
    <Popover>
      <Tooltip>
        <TooltipTrigger
          render={
            <PopoverTrigger
              render={
                <PromptInputButton
                  aria-label={t('Parameters')}
                  disabled={props.disabled}
                  variant='outline'
                />
              }
            />
          }
        >
          <SlidersHorizontalIcon />
          <Badge variant='secondary'>{activeCount}</Badge>
        </TooltipTrigger>
        <TooltipContent>{t('Parameters')}</TooltipContent>
      </Tooltip>

      <PopoverContent
        align='start'
        className='max-h-[min(32rem,calc(100vh-6rem))] w-[22rem] max-w-[calc(100vw-2rem)] overflow-y-auto'
        side='top'
      >
        <PopoverHeader>
          <PopoverTitle>{t('Parameter settings')}</PopoverTitle>
          <PopoverDescription>
            {t('Only enabled parameters are sent with the request.')}
          </PopoverDescription>
        </PopoverHeader>

        <FieldGroup className='gap-4'>
          {PLAYGROUND_PARAMETER_CONTROLS.map((control) => {
            const enabled = props.parameterEnabled[control.key]
            const value = getParameterValue(props.config, control.key)
            const controlId = `playground-${control.key}`
            const switchId = `${controlId}-enabled`

            return (
              <Field
                data-disabled={props.disabled || !enabled}
                key={control.key}
              >
                <div className='flex items-center justify-between gap-4'>
                  <FieldContent>
                    <FieldTitle>{t(control.labelKey)}</FieldTitle>
                    <FieldDescription>
                      {t(control.descriptionKey)}
                    </FieldDescription>
                  </FieldContent>
                  <FieldLabel htmlFor={switchId} className='sr-only'>
                    {t('Enable {{parameter}}', {
                      parameter: t(control.labelKey),
                    })}
                  </FieldLabel>
                  <Switch
                    checked={enabled}
                    disabled={props.disabled}
                    id={switchId}
                    onCheckedChange={(checked) =>
                      props.onParameterEnabledChange(control.key, checked)
                    }
                  />
                </div>

                {control.valueType === 'slider' ? (
                  <Slider
                    aria-label={t(control.labelKey)}
                    disabled={props.disabled || !enabled}
                    id={controlId}
                    max={control.max}
                    min={control.min}
                    onValueChange={(nextValue) => {
                      const next = Array.isArray(nextValue)
                        ? nextValue[0]
                        : nextValue
                      updateValue(control.key, next)
                    }}
                    step={control.step}
                    value={[Number(value)]}
                  />
                ) : (
                  <Input
                    aria-label={t(control.labelKey)}
                    disabled={props.disabled || !enabled}
                    id={controlId}
                    inputMode='numeric'
                    max={control.max}
                    min={control.min}
                    onChange={(event) =>
                      updateValue(control.key, event.target.value)
                    }
                    step={control.step}
                    type='number'
                    value={value ?? ''}
                  />
                )}
              </Field>
            )
          })}
        </FieldGroup>
      </PopoverContent>
    </Popover>
  )
}
