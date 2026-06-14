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
import { Radio as RadioPrimitive } from '@base-ui/react/radio'
import { RadioGroup as Radio } from '@base-ui/react/radio-group'
import { CircleCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  CONTENT_LAYOUT_VALUES,
  DEFAULT_THEME_CUSTOMIZATION,
  THEME_PRESETS,
  THEME_PRESET_VALUES,
  THEME_RADIUS_VALUES,
  THEME_SCALE_VALUES,
  type ContentLayout,
  type ThemeCustomization,
  type ThemeRadius,
  type ThemeScale,
} from '@/lib/theme-customization'
import { cn } from '@/lib/utils'

const Item = RadioPrimitive.Root

type ThemeCustomizationEditorProps = {
  value: ThemeCustomization
  onChange: (value: ThemeCustomization) => void
  disabled?: boolean
}

const RADIUS_OPTIONS: { value: ThemeRadius; label: string; preview: string }[] =
  [
    { value: 'default', label: 'Auto', preview: '999px' },
    { value: 'none', label: '0', preview: '0' },
    { value: 'sm', label: '0.3', preview: '0.3rem' },
    { value: 'md', label: '0.5', preview: '0.5rem' },
    { value: 'lg', label: '0.75', preview: '0.75rem' },
    { value: 'xl', label: '1.0', preview: '1rem' },
  ]

const SCALE_OPTIONS: {
  value: ThemeScale
  labelKey: string
  rows: number
  rowGap: string
}[] = [
  { value: 'sm', labelKey: 'Compact', rows: 4, rowGap: '3px' },
  { value: 'default', labelKey: 'Default', rows: 3, rowGap: '6px' },
  { value: 'lg', labelKey: 'Comfortable', rows: 2, rowGap: '10px' },
]

const CONTENT_LAYOUT_OPTIONS: { value: ContentLayout; labelKey: string }[] = [
  { value: 'full', labelKey: 'Full width' },
  { value: 'centered', labelKey: 'Centered' },
]

function isAllowed<T extends string>(
  value: string,
  allowed: ReadonlySet<T>
): value is T {
  return allowed.has(value as T)
}

function PreviewBox(props: {
  children: React.ReactNode
  checked?: boolean
  className?: string
}) {
  return (
    <div
      className={cn(
        'ring-border group-data-checked:ring-primary group-hover:ring-primary/60 relative h-12 rounded-md ring-[1px] transition group-focus-visible:ring-2 group-data-checked:shadow-md',
        props.className
      )}
    >
      <CircleCheck
        className={cn(
          'fill-primary absolute top-0 right-0 z-10 size-5 translate-x-1/2 -translate-y-1/2 stroke-white',
          !props.checked && 'hidden'
        )}
        aria-hidden='true'
      />
      {props.children}
    </div>
  )
}

function ScalePreview(props: { rows: number; rowGap: string }) {
  return (
    <div
      aria-hidden='true'
      className='absolute inset-2.5 flex flex-col justify-center'
      style={{ gap: props.rowGap }}
    >
      {Array.from({ length: props.rows }).map((_, i) => (
        <span
          key={i}
          className='bg-foreground/60 block h-[2px] rounded-full'
          style={{ width: `${85 - i * 10}%` }}
        />
      ))}
    </div>
  )
}

function ContentLayoutPreview(props: { centered: boolean }) {
  return (
    <div aria-hidden='true' className='absolute inset-2 flex flex-col gap-1.5'>
      <span className='bg-foreground/40 block h-1.5 w-full rounded-sm' />
      <div
        className={cn(
          'flex flex-1 flex-col gap-1',
          props.centered ? 'mx-auto w-1/2' : 'w-full'
        )}
      >
        <span className='bg-foreground/60 block h-[2px] w-full rounded-full' />
        <span className='bg-foreground/60 block h-[2px] w-3/4 rounded-full' />
      </div>
    </div>
  )
}

export function ThemeCustomizationEditor(props: ThemeCustomizationEditorProps) {
  const { t } = useTranslation()

  const update = (next: Partial<ThemeCustomization>) => {
    props.onChange({ ...props.value, ...next })
  }

  return (
    <div className='space-y-6'>
      <div className='space-y-2'>
        <div className='text-muted-foreground text-sm font-semibold'>
          {t('Color preset')}
        </div>
        <Radio
          value={props.value.preset}
          onValueChange={(value) => {
            if (isAllowed(value, THEME_PRESET_VALUES)) {
              update({ preset: value })
            }
          }}
          className='grid grid-cols-2 gap-3 sm:grid-cols-4'
          aria-label={t('Select color preset')}
          disabled={props.disabled}
        >
          {THEME_PRESETS.map((preset) => (
            <Item
              key={preset.value}
              value={preset.value}
              className='group flex flex-col items-stretch outline-none'
              aria-label={t(`preset.${preset.value}`)}
            >
              <PreviewBox checked={props.value.preset === preset.value}>
                <div
                  aria-hidden='true'
                  className='absolute inset-0 rounded-md'
                  style={
                    preset.value === DEFAULT_THEME_CUSTOMIZATION.preset
                      ? {
                          background:
                            'linear-gradient(135deg, var(--background) 0%, var(--muted) 50%, var(--foreground) 100%)',
                        }
                      : {
                          background: `linear-gradient(135deg, ${preset.swatches[0]} 0%, ${preset.swatches[1] ?? preset.swatches[0]} 100%)`,
                        }
                  }
                />
              </PreviewBox>
              <div className='mt-1.5 truncate text-center text-xs'>
                {t(`preset.${preset.value}`)}
              </div>
            </Item>
          ))}
        </Radio>
      </div>

      <div className='space-y-2'>
        <div className='text-muted-foreground text-sm font-semibold'>
          {t('Border radius')}
        </div>
        <Radio
          value={props.value.radius}
          onValueChange={(value) => {
            if (isAllowed(value, THEME_RADIUS_VALUES)) {
              update({ radius: value })
            }
          }}
          className='grid grid-cols-3 gap-2 sm:grid-cols-6'
          aria-label={t('Select border radius')}
          disabled={props.disabled}
        >
          {RADIUS_OPTIONS.map((option) => (
            <Item
              key={option.value}
              value={option.value}
              className='group flex flex-col items-stretch outline-none'
              aria-label={
                option.value === DEFAULT_THEME_CUSTOMIZATION.radius
                  ? t('System default')
                  : option.label
              }
            >
              <PreviewBox checked={props.value.radius === option.value}>
                <span
                  aria-hidden='true'
                  className='border-foreground/70 absolute top-2.5 left-2.5 size-3.5 border-t-[1.5px] border-l-[1.5px]'
                  style={{ borderTopLeftRadius: option.preview }}
                />
              </PreviewBox>
              <div className='mt-1.5 text-center text-xs'>{option.label}</div>
            </Item>
          ))}
        </Radio>
      </div>

      <div className='grid gap-5 md:grid-cols-2'>
        <div className='space-y-2'>
          <div className='text-muted-foreground text-sm font-semibold'>
            {t('Density')}
          </div>
          <Radio
            value={props.value.scale}
            onValueChange={(value) => {
              if (isAllowed(value, THEME_SCALE_VALUES)) update({ scale: value })
            }}
            className='grid grid-cols-3 gap-3'
            aria-label={t('Select interface density')}
            disabled={props.disabled}
          >
            {SCALE_OPTIONS.map((option) => (
              <Item
                key={option.value}
                value={option.value}
                className='group flex flex-col items-stretch outline-none'
                aria-label={t(option.labelKey)}
              >
                <PreviewBox checked={props.value.scale === option.value}>
                  <ScalePreview rows={option.rows} rowGap={option.rowGap} />
                </PreviewBox>
                <div className='mt-1.5 truncate text-center text-xs'>
                  {t(option.labelKey)}
                </div>
              </Item>
            ))}
          </Radio>
        </div>

        <div className='space-y-2'>
          <div className='text-muted-foreground text-sm font-semibold'>
            {t('Content width')}
          </div>
          <Radio
            value={props.value.contentLayout}
            onValueChange={(value) => {
              if (isAllowed(value, CONTENT_LAYOUT_VALUES)) {
                update({ contentLayout: value })
              }
            }}
            className='grid grid-cols-2 gap-3'
            aria-label={t('Select content width')}
            disabled={props.disabled}
          >
            {CONTENT_LAYOUT_OPTIONS.map((option) => (
              <Item
                key={option.value}
                value={option.value}
                className='group flex flex-col items-stretch outline-none'
                aria-label={t(option.labelKey)}
              >
                <PreviewBox
                  checked={props.value.contentLayout === option.value}
                >
                  <ContentLayoutPreview
                    centered={option.value === 'centered'}
                  />
                </PreviewBox>
                <div className='mt-1.5 truncate text-center text-xs'>
                  {t(option.labelKey)}
                </div>
              </Item>
            ))}
          </Radio>
        </div>
      </div>
    </div>
  )
}
