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
import {
  memo,
  useCallback,
  useId,
  useMemo,
  useState,
  type ChangeEvent,
  type FocusEvent,
  type InputHTMLAttributes,
  type MouseEvent as ReactMouseEvent,
} from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, Copy, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { useDebounce } from '@/hooks/use-debounce'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  BILLING_EXTRA_VARS,
  COMMON_TIMEZONES,
  MATCH_CONTAINS,
  MATCH_EQ,
  MATCH_EXISTS,
  MATCH_GT,
  MATCH_GTE,
  MATCH_LT,
  MATCH_LTE,
  MATCH_RANGE,
  SOURCE_HEADER,
  SOURCE_PARAM,
  SOURCE_TIME,
  TIME_FUNCS,
  buildRequestRuleExpr,
  createEmptyCondition,
  createEmptyRuleGroup,
  createEmptyTimeCondition,
  getRequestRuleMatchOptions,
  tryParseRequestRuleExpr,
  type ParamHeaderCondition,
  type RequestCondition,
  type RequestRuleGroup,
  type TimeCondition,
  type TimeFunc,
} from '@/features/pricing/lib/billing-expr'
import {
  createPricingEditorState,
  switchPricingEditorMode,
  editRawPricing,
  editVisualPricing,
  editPricingRules,
} from '@/features/pricing/lib/pricing-editor-state'
import {
  CACHE_MODE_GENERIC,
  CACHE_MODE_OPENAI,
  CACHE_MODE_TIMED,
  type CacheMode,
  type TokenTierCondition,
  type TierConditionInput,
  type VisualConfig,
  type VisualTier,
  createEmptyTokenCondition,
  getTierCacheMode,
  isVisualPriceEnabled,
  normalizeVisualConfig,
  normalizeVisualTier,
  setVisualTierCacheMode,
  setVisualTierPrice,
  setVisualTierPriceEnabled,
} from '@/features/pricing/lib/tier-expr'

const PRICE_SUFFIX = '$/1M tokens'
const CACHE_PRICE_VARS = BILLING_EXTRA_VARS.filter(
  (variable) => variable.group === 'cache'
)
const MEDIA_PRICE_VARS = BILLING_EXTRA_VARS.filter(
  (variable) => variable.group === 'media'
)
const TASK_PRICE_VARS = BILLING_EXTRA_VARS.filter(
  (variable) => variable.group === 'time' || variable.group === 'request'
)
const ESTIMATOR_TOKEN_PRICE_VARS = BILLING_EXTRA_VARS.filter(
  (variable) => !variable.unit || variable.unit === 'tokens'
)
const extraTokenPattern = new RegExp(
  `\\b(${ESTIMATOR_TOKEN_PRICE_VARS.map((v) => v.key).join('|')})\\b`
)

const CONDITION_INPUT_OPTIONS: {
  value: TokenTierCondition['var']
  labelKey: string
}[] = [
  { value: 'len', labelKey: 'Full input length' },
  { value: 'p', labelKey: 'Billable input tokens' },
  { value: 'c', labelKey: 'Billable output tokens' },
  ...ESTIMATOR_TOKEN_PRICE_VARS.map((v) => ({
    value: v.key as TokenTierCondition['var'],
    labelKey: v.shortLabel,
  })),
]
const OPS: TokenTierCondition['op'][] = ['<', '<=', '>', '>=']

type Preset = {
  key: string
  label: string
  expr: string
  requestRules?: RequestRuleGroup[]
}

type PresetGroup = {
  group: string
  presets: Preset[]
}

const PRESET_GROUPS: PresetGroup[] = [
  {
    group: 'Fixed price',
    presets: [
      { key: 'flat', label: 'Flat', expr: 'tier("base", p * 2 + c * 4)' },
      {
        key: 'claude-opus',
        label: 'Claude Opus 4.6',
        expr: 'tier("base", p * 5 + c * 25 + cr * 0.5 + cc * 6.25 + cc1h * 10)',
      },
      {
        key: 'gpt-5.4',
        label: 'GPT-5.4',
        expr: 'len <= 272000 ? tier("standard", p * 2.5 + c * 15 + cr * 0.25) : tier("long_context", p * 5 + c * 22.5 + cr * 0.5)',
      },
    ],
  },
  {
    group: 'Tiered',
    presets: [
      {
        key: 'claude-sonnet',
        label: 'Claude Sonnet 4.5',
        expr: 'len <= 200000 ? tier("standard", p * 3 + c * 15 + cr * 0.3 + cc * 3.75 + cc1h * 6) : tier("long_context", p * 6 + c * 22.5 + cr * 0.6 + cc * 7.5 + cc1h * 12)',
      },
      {
        key: 'qwen3-max',
        label: 'Qwen3 Max',
        expr: 'len <= 32000 ? tier("short", p * 1.2 + c * 6 + cr * 0.24 + cc * 1.5) : len <= 128000 ? tier("mid", p * 2.4 + c * 12 + cr * 0.48 + cc * 3) : tier("long", p * 3 + c * 15 + cr * 0.6 + cc * 3.75)',
      },
      {
        key: 'glm-4.5-air',
        label: 'GLM-4.5 Air',
        expr: 'len < 32000 && c < 200 ? tier("short_output", p * 0.8 + c * 2 + cr * 0.16) : len < 32000 && c >= 200 ? tier("long_output", p * 0.8 + c * 6 + cr * 0.16) : tier("mid_context", p * 1.2 + c * 8 + cr * 0.24)',
      },
      {
        key: 'doubao-seed-1.8',
        label: 'Doubao Seed 1.8',
        expr: 'len <= 32000 && c <= 200 ? tier("discount", p * 0.8 + c * 2 + cr * 0.16 + cc * 0.17) : len <= 32000 ? tier("short", p * 0.8 + c * 8 + cr * 0.16 + cc * 0.17) : len <= 128000 ? tier("mid", p * 1.2 + c * 16 + cr * 0.16 + cc * 0.17) : tier("long", p * 2.4 + c * 24 + cr * 0.16 + cc * 0.17)',
      },
    ],
  },
  {
    group: 'Multimodal',
    presets: [
      {
        key: 'gpt-image-1-mini',
        label: 'GPT Image 1 Mini',
        expr: 'tier("base", p * 2 + c * 8 + img * 2.5)',
      },
      {
        key: 'gemini-2.5-flash',
        label: 'Gemini 2.5 Flash',
        expr: 'tier("base", p * 0.3 + c * 2.5 + cr * 0.03 + ai * 1.0)',
      },
      {
        key: 'gemini-3-pro-image',
        label: 'Gemini 3 Pro Image',
        expr: 'tier("base", p * 2 + c * 12 + img_o * 120)',
      },
      {
        key: 'qwen3-omni-flash',
        label: 'Qwen3 Omni Flash',
        expr: 'tier("base", p * 0.43 + c * 3.06 + img * 0.78 + ai * 3.81 + ao * 15.11)',
      },
      {
        key: 'doubao-seedance-2.0',
        label: 'Doubao Seedance 2.0',
        expr: 'param("resolution") == "1080p" ? (param("content.#(type==\\"video_url\\").video_url.url") != nil ? tier("1080p_video_input", c * 4.3055555556) : tier("1080p_no_video_input", c * 7.0833333333)) : (param("content.#(type==\\"video_url\\").video_url.url") != nil ? tier("480_720p_video_input", c * 3.8888888889) : tier("480_720p_no_video_input", c * 6.3888888889))',
      },
    ],
  },
  {
    group: 'Request rule',
    presets: [
      {
        key: 'claude-opus-fast',
        label: 'Claude Opus 4.6 Fast',
        expr: 'tier("base", p * 5 + c * 25 + cr * 0.5 + cc * 6.25 + cc1h * 10)',
        requestRules: [
          {
            conditions: [
              {
                source: SOURCE_HEADER as 'header',
                path: 'anthropic-beta',
                mode: MATCH_CONTAINS,
                value: 'fast-mode-2026-02-01',
              },
            ],
            multiplier: '6',
          },
        ],
      },
      {
        key: 'gpt-5.4-tiers',
        label: 'GPT-5.4 Priority/Flex',
        expr: 'len <= 272000 ? tier("standard", p * 2.5 + c * 15 + cr * 0.25) : tier("long_context", p * 5 + c * 22.5 + cr * 0.5)',
        requestRules: [
          {
            conditions: [
              {
                source: SOURCE_PARAM as 'param',
                path: 'service_tier',
                mode: MATCH_EQ,
                value: 'priority',
              },
            ],
            multiplier: '2',
          },
          {
            conditions: [
              {
                source: SOURCE_PARAM as 'param',
                path: 'service_tier',
                mode: MATCH_EQ,
                value: 'flex',
              },
            ],
            multiplier: '0.5',
          },
        ],
      },
    ],
  },
  {
    group: 'Time-based',
    presets: [
      {
        key: 'night-discount',
        label: 'Night -50%',
        expr: 'tier("base", p * 3 + c * 15)',
        requestRules: [
          {
            conditions: [
              {
                source: SOURCE_TIME as 'time',
                timeFunc: 'hour',
                timezone: 'Asia/Shanghai',
                mode: MATCH_RANGE,
                value: '',
                rangeStart: '21',
                rangeEnd: '6',
              },
            ],
            multiplier: '0.5',
          },
        ],
      },
      {
        key: 'weekend-discount',
        label: 'Weekend -20%',
        expr: 'tier("base", p * 3 + c * 15)',
        requestRules: [
          {
            conditions: [
              {
                source: SOURCE_TIME as 'time',
                timeFunc: 'weekday',
                timezone: 'Asia/Shanghai',
                mode: MATCH_EQ,
                value: '0',
                rangeStart: '',
                rangeEnd: '',
              },
            ],
            multiplier: '0.8',
          },
          {
            conditions: [
              {
                source: SOURCE_TIME as 'time',
                timeFunc: 'weekday',
                timezone: 'Asia/Shanghai',
                mode: MATCH_EQ,
                value: '6',
                rangeStart: '',
                rangeEnd: '',
              },
            ],
            multiplier: '0.8',
          },
        ],
      },
    ],
  },
]

function unitCostToPrice(uc: number | string): number {
  return Number(uc) || 0
}

function priceToUnitCost(price: number | string): number {
  return Number(price) || 0
}

function formatTokenHint(n: number | string | null | undefined): string {
  if (n == null || n === '' || Number.isNaN(Number(n))) return ''
  const v = Number(n)
  if (v === 0) return '= 0'
  if (v >= 1_000_000) return `= ${(v / 1_000_000).toLocaleString()}M tokens`
  if (v >= 1_000) return `= ${(v / 1_000).toLocaleString()}K tokens`
  return `= ${v.toLocaleString()} tokens`
}

function formatNumberDraft(value: number | string): string {
  if (value === '') return ''
  if (typeof value === 'number')
    return Number.isFinite(value) ? String(value) : '0'
  return value
}

function parseNumberDraft(value: string): number {
  if (value.trim() === '') return 0
  const next = Number(value)
  return Number.isFinite(next) ? next : 0
}

function isZeroDraft(value: string): boolean {
  return value.trim() !== '' && parseNumberDraft(value) === 0
}

type DraftNumberInputProps = Omit<
  InputHTMLAttributes<HTMLInputElement>,
  'type' | 'value' | 'onChange'
> & {
  value: number | string
  onValueChange: (next: number) => void
  selectZeroOnFocus?: boolean
}

function DraftNumberInput({
  value,
  onValueChange,
  selectZeroOnFocus = true,
  onBlur,
  onFocus,
  onMouseUp,
  ...props
}: DraftNumberInputProps) {
  const [draft, setDraft] = useState(() => formatNumberDraft(value))
  const [focused, setFocused] = useState(false)

  const incomingDraft = formatNumberDraft(value)
  if (!focused && draft !== incomingDraft) setDraft(incomingDraft)

  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    const nextDraft = event.target.value
    setDraft(nextDraft)
    onValueChange(parseNumberDraft(nextDraft))
  }

  const handleFocus = (event: FocusEvent<HTMLInputElement>) => {
    setFocused(true)
    onFocus?.(event)
    if (selectZeroOnFocus && isZeroDraft(event.currentTarget.value)) {
      event.currentTarget.select()
    }
  }

  const handleMouseUp = (event: ReactMouseEvent<HTMLInputElement>) => {
    onMouseUp?.(event)
    if (selectZeroOnFocus && isZeroDraft(event.currentTarget.value)) {
      event.preventDefault()
      event.currentTarget.select()
    }
  }

  const handleBlur = (event: FocusEvent<HTMLInputElement>) => {
    const normalized = parseNumberDraft(event.currentTarget.value)
    setFocused(false)
    setDraft(String(normalized))
    // Changes are published while typing; blur must never overwrite a newly loaded value.
    onBlur?.(event)
  }

  return (
    <Input
      {...props}
      type='number'
      step={
        Number(props.step) === 0.0001 &&
        Number(Number(value).toFixed(4)) !== Number(value)
          ? 'any'
          : props.step
      }
      value={draft}
      onChange={handleChange}
      onFocus={handleFocus}
      onMouseUp={handleMouseUp}
      onBlur={handleBlur}
    />
  )
}

// ---------------------------------------------------------------------------
// Price input field
// ---------------------------------------------------------------------------

type PriceFieldProps = {
  label: string
  hint?: string
  value: number
  onChange: (next: number) => void
  enabled?: boolean
  onEnabledChange?: (enabled: boolean) => void
}

function PriceField(props: PriceFieldProps) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <FieldGroup className='w-36 gap-1'>
      <Field className='gap-1' data-disabled={props.enabled === false}>
        <FieldLabel id={`${id}-label`} htmlFor={id}>
          {props.label}
        </FieldLabel>
        <DraftNumberInput
          id={id}
          min={0}
          step={0.0001}
          disabled={props.enabled === false}
          value={Number.isFinite(props.value) ? props.value : 0}
          onValueChange={props.onChange}
          className='h-8 w-full'
        />
      </Field>
      {props.onEnabledChange && (
        <Field orientation='horizontal'>
          <Switch
            id={`${id}-enabled`}
            size='sm'
            checked={props.enabled ?? false}
            onCheckedChange={props.onEnabledChange}
            aria-labelledby={`${id}-label ${id}-enabled-label`}
          />
          <FieldLabel id={`${id}-enabled-label`} htmlFor={`${id}-enabled`}>
            {t('Price separately')}
          </FieldLabel>
        </Field>
      )}
      {props.hint && (
        <p className='text-muted-foreground text-xs'>{props.hint}</p>
      )}
    </FieldGroup>
  )
}

// ---------------------------------------------------------------------------
// Single tier card (visual editor)
// ---------------------------------------------------------------------------

type VisualTierCardProps = {
  tier: VisualTier
  index: number
  total: number
  onChange: (next: VisualTier) => void
  onRemove: () => void
  onAddCondition: () => void
}

function VisualTierCard({
  tier,
  index,
  total,
  onChange,
  onRemove,
  onAddCondition,
}: VisualTierCardProps) {
  const { t } = useTranslation()
  const cacheMode = getTierCacheMode(tier)

  const handleConditionChange = (
    conditionIndex: number,
    next: TierConditionInput
  ) => {
    const conditions = [...tier.conditions]
    conditions[conditionIndex] = next
    onChange({ ...tier, conditions })
  }

  const handleConditionRemove = (conditionIndex: number) => {
    onChange({
      ...tier,
      conditions: tier.conditions.filter((_, i) => i !== conditionIndex),
    })
  }

  const handlePriceChange = (field: string, value: number) => {
    onChange(setVisualTierPrice(tier, field, value))
  }

  const handleCacheModeChange = (mode: CacheMode) => {
    onChange(setVisualTierCacheMode(tier, mode))
  }

  const inputUnitPrice = unitCostToPrice(tier.input_unit_cost)
  const outputUnitPrice = unitCostToPrice(tier.output_unit_cost)
  const hasMediaPricing = MEDIA_PRICE_VARS.some((variable) => {
    return isVisualPriceEnabled(tier, variable.tierField!)
  })
  const [mediaOpen, setMediaOpen] = useState(hasMediaPricing)
  const hasTaskPricing = TASK_PRICE_VARS.some((variable) => {
    return isVisualPriceEnabled(tier, variable.tierField!)
  })
  const [taskOpen, setTaskOpen] = useState(hasTaskPricing)

  const [previousMediaPricing, setPreviousMediaPricing] =
    useState(hasMediaPricing)
  if (previousMediaPricing !== hasMediaPricing) {
    setPreviousMediaPricing(hasMediaPricing)
    if (hasMediaPricing) setMediaOpen(true)
  }
  const [previousTaskPricing, setPreviousTaskPricing] = useState(hasTaskPricing)
  if (previousTaskPricing !== hasTaskPricing) {
    setPreviousTaskPricing(hasTaskPricing)
    if (hasTaskPricing) setTaskOpen(true)
  }

  const renderPriceVariable = (
    variable: (typeof BILLING_EXTRA_VARS)[number]
  ) => {
    const fieldKey = variable.tierField!
    const value = unitCostToPrice((tier[fieldKey] as number | undefined) ?? 0)

    return (
      <PriceField
        key={variable.key}
        label={t(variable.key === 'cc' && cacheMode === CACHE_MODE_OPENAI ? 'Cache Write (30m)' : variable.label)}
        value={value}
        enabled={isVisualPriceEnabled(tier, fieldKey)}
        onEnabledChange={(enabled) =>
          onChange(setVisualTierPriceEnabled(tier, fieldKey, enabled))
        }
        onChange={(next) => handlePriceChange(fieldKey, priceToUnitCost(next))}
      />
    )
  }

  return (
    <div className='space-y-3 rounded-lg border p-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex items-center gap-2'>
          <Badge variant='outline'>
            {t('Tier')} {index + 1} / {total}
          </Badge>
          {tier.conditions.length === 0 && (
            <Badge variant='secondary'>{t('Fallback tier')}</Badge>
          )}
          <Input
            value={tier.label}
            onChange={(event) =>
              onChange({ ...tier, label: event.target.value })
            }
            placeholder={t('Tier name')}
            className='h-7 w-36'
          />
        </div>
        <Button
          variant='ghost'
          size='icon'
          onClick={onRemove}
          disabled={total <= 1}
          aria-label={t('Remove tier')}
        >
          <Trash2 className='text-destructive h-4 w-4' />
        </Button>
      </div>

      {/* Conditions */}
      <div className='space-y-1.5'>
        <div className='flex h-7 items-center justify-between'>
          <Label className='text-xs font-medium'>{t('Tier conditions')}</Label>
          <Button
            variant='ghost'
            size='sm'
            onClick={onAddCondition}
            disabled={tier.conditions.length >= 4}
            className='h-7 px-2 text-xs'
          >
            <Plus className='mr-1 h-3 w-3' />
            {t('Add condition')}
          </Button>
        </div>
        {tier.conditions.length === 0 ? (
          <p className='text-muted-foreground text-xs'>
            {t('Always matches (default tier).')}
          </p>
        ) : (
          tier.conditions.map((condition, conditionIndex) => (
            <RuleConditionRow
              key={conditionIndex}
              condition={condition}
              allowToken
              onChange={(next) => handleConditionChange(conditionIndex, next)}
              onRemove={() => handleConditionRemove(conditionIndex)}
            />
          ))
        )}
      </div>

      <div className='space-y-2'>
        <div className='flex items-center justify-between gap-3'>
          <Label className='text-sm font-semibold'>{t('Token prices')}</Label>
          <span className='bg-muted text-muted-foreground rounded-md px-2 py-1 text-xs'>
            {PRICE_SUFFIX}
          </span>
        </div>

        <div className='space-y-3'>
          <div className='flex flex-wrap gap-x-4 gap-y-2'>
            <PriceField
              label={t('Input price')}
              value={inputUnitPrice}
              onChange={(value) =>
                handlePriceChange('input_unit_cost', priceToUnitCost(value))
              }
            />
            <PriceField
              label={t('Output price')}
              value={outputUnitPrice}
              onChange={(value) =>
                handlePriceChange('output_unit_cost', priceToUnitCost(value))
              }
            />
          </div>

          <div className='space-y-2'>
            <div className='flex h-7 items-center'>
              <Tabs
                value={cacheMode}
                onValueChange={(value) =>
                  value !== null && handleCacheModeChange(value as CacheMode)
                }
              >
                <TabsList className='h-8'>
                  <TabsTrigger
                    value={CACHE_MODE_GENERIC}
                    className='px-2 text-xs'
                  >
                    {t('Generic cache')}
                  </TabsTrigger>
                  <TabsTrigger value={CACHE_MODE_OPENAI} className='px-2 text-xs'>
                    OpenAI (30m / 12h)
                  </TabsTrigger>
                  <TabsTrigger
                    value={CACHE_MODE_TIMED}
                    className='px-2 text-xs'
                  >
                    {t('Time-sliced cache (Claude)')}
                  </TabsTrigger>
                </TabsList>
              </Tabs>
            </div>
            <div className='flex flex-wrap gap-x-4 gap-y-2'>
              {CACHE_PRICE_VARS.map((variable) => {
                if (variable.key === 'cc12h' && cacheMode !== CACHE_MODE_OPENAI) return null
                if (variable.key === 'cc1h' && cacheMode !== CACHE_MODE_TIMED) {
                  return null
                }
                return renderPriceVariable(variable)
              })}
            </div>
          </div>
        </div>
      </div>

      {/* Media prices */}
      <div className='space-y-1.5'>
        <Button
          type='button'
          variant='ghost'
          size='sm'
          className='h-7 px-2 text-xs'
          onClick={() => setMediaOpen((prev) => !prev)}
        >
          <ChevronDown
            className={cn(
              'mr-1 h-3 w-3 transition-transform',
              mediaOpen && 'rotate-180'
            )}
          />
          {t('Media pricing')}
        </Button>
        {mediaOpen && (
          <div className='flex flex-wrap gap-x-4 gap-y-2'>
            {MEDIA_PRICE_VARS.map(renderPriceVariable)}
          </div>
        )}
      </div>

      <div className='space-y-1.5'>
        <Button
          type='button'
          variant='ghost'
          size='sm'
          className='h-7 px-2 text-xs'
          onClick={() => setTaskOpen((prev) => !prev)}
        >
          <ChevronDown
            className={cn('transition-transform', taskOpen && 'rotate-180')}
            data-icon='inline-start'
          />
          {t('Task pricing')}
        </Button>
        {taskOpen && (
          <div className='flex flex-wrap gap-x-4 gap-y-2'>
            {TASK_PRICE_VARS.map(renderPriceVariable)}
          </div>
        )}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Visual editor (list of tiers)
// ---------------------------------------------------------------------------

type VisualEditorProps = {
  visualConfig: VisualConfig | null
  onChange: (next: VisualConfig) => void
}

function VisualEditor({ visualConfig, onChange }: VisualEditorProps) {
  const { t } = useTranslation()
  const config = useMemo(
    () => normalizeVisualConfig(visualConfig),
    [visualConfig]
  )

  const handleTierChange = (index: number, next: VisualTier) => {
    const tiers = [...config.tiers]
    tiers[index] = normalizeVisualTier(next)
    onChange({ ...config, tiers })
  }

  const handleAddTier = () => {
    const tiers = [...config.tiers]
    const lastIndex = tiers.length - 1
    // When adding a new fallback, give the previous catch-all tier a default
    // upper-bound condition so the expression compiles into a sane two-tier
    // shape. Mirrors the classic editor's UX for adding tiers.
    if (lastIndex >= 0 && tiers[lastIndex].conditions.length === 0) {
      tiers[lastIndex] = normalizeVisualTier({
        ...tiers[lastIndex],
        conditions: [createEmptyTokenCondition()],
      })
    }
    tiers.push(
      normalizeVisualTier({
        label: `tier_${tiers.length + 1}`,
        conditions: [],
        input_unit_cost: 0,
        output_unit_cost: 0,
      })
    )
    onChange({ ...config, tiers })
  }

  const handleRemoveTier = (index: number) => {
    const tiers = config.tiers.filter((_, i) => i !== index)
    onChange({ ...config, tiers: tiers.length > 0 ? tiers : config.tiers })
  }

  const handleAddCondition = (index: number) => {
    const tier = config.tiers[index]
    if (tier.conditions.length >= 4) return
    // Prefer `len` (input length) over `p`/`c` for tier conditions because
    // `p` is subject to auto-exclusion when sub-categories like `cr` are
    // priced separately, which can misroute long-input requests into shorter
    // tiers when cache-hits reduce the effective `p`.
    const usedVars = new Set(
      tier.conditions
        .filter(
          (condition): condition is TokenTierCondition =>
            condition.source === 'token'
        )
        .map((condition) => condition.var)
    )
    const nextCondition = createEmptyTokenCondition()
    if (usedVars.has('len')) nextCondition.var = 'c'
    onChange({
      ...config,
      tiers: config.tiers.map((current, i) =>
        i === index
          ? {
              ...current,
              conditions: [...tier.conditions, nextCondition],
            }
          : current
      ),
    })
  }

  return (
    <div className='space-y-2'>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Each tier supports up to 4 conditions from tokens, body params, headers, or time. The last tier without conditions is the fallback.'
        )}
      </p>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Enable separate pricing to set a category price. An enabled price of 0 is free; disabling removes that price term.'
        )}
      </p>
      {config.tiers.map((tier, index) => (
        <VisualTierCard
          key={index}
          tier={tier}
          index={index}
          total={config.tiers.length}
          onChange={(next) => handleTierChange(index, next)}
          onRemove={() => handleRemoveTier(index)}
          onAddCondition={() => handleAddCondition(index)}
        />
      ))}
      <Button
        variant='outline'
        size='sm'
        className='h-9 w-36 justify-center'
        onClick={handleAddTier}
      >
        <Plus className='mr-2 h-4 w-4' />
        {t('Add tier')}
      </Button>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Raw expression editor
// ---------------------------------------------------------------------------

type RawExprEditorProps = {
  exprString: string
  onChange: (value: string) => void
}

function RawExprEditor({ exprString, onChange }: RawExprEditorProps) {
  const { t } = useTranslation()
  return (
    <div className='space-y-3'>
      <Alert>
        <AlertDescription className='space-y-1 text-xs'>
          <div>
            {t('Variables')}: <code>len</code>, <code>p</code>, <code>c</code>,{' '}
            <code>cr</code>, <code>cc</code>, <code>cc1h</code>, <code>cc12h</code>,{' '}
            <code>img</code>, <code>img_o</code>, <code>ai</code>,{' '}
            <code>ao</code>
          </div>
          <div>
            {t('Functions')}: <code>tier(name, value)</code>, <code>max</code>,{' '}
            <code>min</code>, <code>ceil</code>, <code>floor</code>,{' '}
            <code>abs</code>, <code>header(name)</code>,{' '}
            <code>param(path)</code>, <code>response(path)</code>,{' '}
            <code>has(source, text)</code>, <code>pixels(value)</code>,{' '}
            <code>hour(tz)</code>, <code>minute(tz)</code>,{' '}
            <code>weekday(tz)</code>, <code>month(tz)</code>,{' '}
            <code>day(tz)</code>
          </div>
        </AlertDescription>
      </Alert>
      <Textarea
        value={exprString}
        onChange={(event) => onChange(event.target.value)}
        placeholder='tier("base", p * 3 + c * 15)'
        rows={6}
        className='font-mono text-xs'
        spellCheck={false}
      />
    </div>
  )
}

// ---------------------------------------------------------------------------
// Request rule condition row
// ---------------------------------------------------------------------------

type RuleConditionRowProps = {
  condition: TierConditionInput
  onChange: (next: TierConditionInput) => void
  onRemove: () => void
  allowToken?: boolean
}

function RuleConditionRow({
  condition,
  onChange,
  onRemove,
  allowToken = false,
}: RuleConditionRowProps) {
  const { t } = useTranslation()
  if (condition.source === 'expression') {
    return (
      <div className='flex items-start gap-2'>
        <div className='flex min-w-0 flex-1 flex-col gap-1'>
          <Label className='text-xs'>{t('Compound condition')}</Label>
          <Textarea
            aria-label={t('Compound condition')}
            value={condition.expression}
            onChange={(event) =>
              onChange({ ...condition, expression: event.target.value })
            }
            rows={3}
            spellCheck={false}
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'Keep parentheses when editing AND/OR conditions. Prices can be edited separately.'
            )}
          </p>
        </div>
        <Button
          variant='ghost'
          size='icon'
          onClick={onRemove}
          aria-label={t('Remove condition')}
        >
          <Trash2 className='text-destructive size-4' />
        </Button>
      </div>
    )
  }
  const matchOptions =
    condition.source === 'token'
      ? []
      : getRequestRuleMatchOptions(condition.source)
  const getMatchLabel = (mode: string) => {
    switch (mode) {
      case MATCH_EQ:
        return t('Equals')
      case MATCH_CONTAINS:
        return t('Contains')
      case MATCH_EXISTS:
        return t('Exists')
      case MATCH_GT:
        return t('Greater than')
      case MATCH_GTE:
        return t('Greater than or equal')
      case MATCH_LT:
        return t('Less than')
      case MATCH_LTE:
        return t('Less than or equal')
      case MATCH_RANGE:
        return t('Overnight range')
      default:
        return mode
    }
  }
  const getTimeFuncLabel = (timeFunc: TimeFunc) => {
    switch (timeFunc) {
      case 'hour':
        return t('Hour of day')
      case 'minute':
        return t('Minute')
      case 'weekday':
        return t('Weekday')
      case 'month':
        return t('Month number')
      case 'day':
        return t('Day of month')
      default:
        return timeFunc
    }
  }
  const sourceLabel =
    condition.source === 'token'
      ? t('Token count')
      : condition.source === SOURCE_PARAM
        ? t('Body param')
        : condition.source === SOURCE_HEADER
          ? t('Header')
          : t('Time')

  const handleSourceChange = (source: string) => {
    if (source === 'token') {
      onChange(createEmptyTokenCondition())
    } else if (source === SOURCE_TIME) {
      onChange(createEmptyTimeCondition())
    } else if (source === SOURCE_HEADER || source === SOURCE_PARAM) {
      onChange({
        ...createEmptyCondition(),
        source: source as 'param' | 'header',
      })
    }
  }

  const handleModeChange = (mode: string) => {
    if (condition.source !== 'token') {
      onChange({ ...condition, mode } as RequestCondition)
    }
  }

  const renderTokenCondition = (tokenCondition: TokenTierCondition) => {
    const currentInputOption = CONDITION_INPUT_OPTIONS.find(
      (option) => option.value === tokenCondition.var
    )
    return (
      <>
        <Select
          items={CONDITION_INPUT_OPTIONS.map((option) => ({
            value: option.value,
            label: t(option.labelKey),
          }))}
          value={tokenCondition.var}
          onValueChange={(value) =>
            onChange({
              ...tokenCondition,
              var: value as TokenTierCondition['var'],
            })
          }
        >
          <SelectTrigger className='w-40' size='sm'>
            <SelectValue>
              {currentInputOption
                ? t(currentInputOption.labelKey)
                : tokenCondition.var}
            </SelectValue>
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              {CONDITION_INPUT_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {t(option.labelKey)}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Select
          items={OPS.map((op) => ({ value: op, label: op }))}
          value={tokenCondition.op}
          onValueChange={(value) =>
            onChange({
              ...tokenCondition,
              op: value as TokenTierCondition['op'],
            })
          }
        >
          <SelectTrigger className='w-20' size='sm'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              {OPS.map((op) => (
                <SelectItem key={op} value={op}>
                  {op}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <DraftNumberInput
          min={0}
          value={tokenCondition.value}
          onValueChange={(value) => onChange({ ...tokenCondition, value })}
          placeholder='tokens'
          className='w-32'
        />
        <span className='text-muted-foreground text-xs'>
          {formatTokenHint(tokenCondition.value)}
        </span>
      </>
    )
  }

  const renderTimeCondition = (timeCond: TimeCondition) => (
    <>
      <Select
        items={[
          ...TIME_FUNCS.map((fn) => ({
            value: fn,
            label: getTimeFuncLabel(fn),
          })),
        ]}
        value={timeCond.timeFunc}
        onValueChange={(value) =>
          onChange({ ...timeCond, timeFunc: value as TimeFunc })
        }
      >
        <SelectTrigger className='w-32' size='sm'>
          <SelectValue>{getTimeFuncLabel(timeCond.timeFunc)}</SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {TIME_FUNCS.map((fn) => (
              <SelectItem key={fn} value={fn}>
                {getTimeFuncLabel(fn)}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <Select
        items={[
          ...COMMON_TIMEZONES.map((tz) => ({
            value: tz.value,
            label: tz.label,
          })),
        ]}
        value={timeCond.timezone}
        onValueChange={(value) =>
          value !== null && onChange({ ...timeCond, timezone: value })
        }
      >
        <SelectTrigger className='w-56' size='sm'>
          <SelectValue>
            {COMMON_TIMEZONES.find((tz) => tz.value === timeCond.timezone)
              ?.label ?? timeCond.timezone}
          </SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {COMMON_TIMEZONES.map((tz) => (
              <SelectItem key={tz.value} value={tz.value}>
                {tz.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <Select
        items={[
          ...matchOptions.map((option) => ({
            value: option.value,
            label: getMatchLabel(option.value),
          })),
        ]}
        value={timeCond.mode}
        onValueChange={(v) => v !== null && handleModeChange(v)}
      >
        <SelectTrigger className='w-32' size='sm'>
          <SelectValue>{getMatchLabel(timeCond.mode)}</SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {matchOptions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {getMatchLabel(option.value)}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      {timeCond.mode === MATCH_RANGE ? (
        <>
          <DraftNumberInput
            value={timeCond.rangeStart}
            onValueChange={(value) =>
              onChange({ ...timeCond, rangeStart: String(value) })
            }
            placeholder={t('Start')}
            className='w-20'
          />
          <span className='text-muted-foreground text-xs'>~</span>
          <DraftNumberInput
            value={timeCond.rangeEnd}
            onValueChange={(value) =>
              onChange({ ...timeCond, rangeEnd: String(value) })
            }
            placeholder={t('End')}
            className='w-20'
          />
        </>
      ) : (
        <DraftNumberInput
          value={timeCond.value}
          onValueChange={(value) =>
            onChange({ ...timeCond, value: String(value) })
          }
          placeholder={t('Value')}
          className='w-24'
        />
      )}
    </>
  )

  const renderParamHeaderCondition = (phCond: ParamHeaderCondition) => (
    <>
      <Input
        value={phCond.path}
        onChange={(event) => onChange({ ...phCond, path: event.target.value })}
        placeholder={
          phCond.source === SOURCE_HEADER ? 'X-Header-Name' : 'service_tier'
        }
        className='w-44'
      />
      <Select
        items={[
          ...matchOptions.map((option) => ({
            value: option.value,
            label: getMatchLabel(option.value),
          })),
        ]}
        value={phCond.mode}
        onValueChange={(v) => v !== null && handleModeChange(v)}
      >
        <SelectTrigger className='w-32' size='sm'>
          <SelectValue>{getMatchLabel(phCond.mode)}</SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {matchOptions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {getMatchLabel(option.value)}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      {phCond.mode !== MATCH_EXISTS && (
        <Input
          value={phCond.value}
          onChange={(event) =>
            onChange({ ...phCond, value: event.target.value })
          }
          placeholder={t('Value')}
          className='w-44'
        />
      )}
    </>
  )

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <Select
        items={[
          ...(allowToken ? [{ value: 'token', label: t('Token count') }] : []),
          { value: SOURCE_PARAM, label: t('Body param') },
          { value: SOURCE_HEADER, label: t('Header') },
          { value: SOURCE_TIME, label: t('Time') },
        ]}
        value={condition.source}
        onValueChange={(v) => v !== null && handleSourceChange(v)}
      >
        <SelectTrigger className='w-28' size='sm'>
          <SelectValue>{sourceLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {allowToken && (
              <SelectItem value='token'>{t('Token count')}</SelectItem>
            )}
            <SelectItem value={SOURCE_PARAM}>{t('Body param')}</SelectItem>
            <SelectItem value={SOURCE_HEADER}>{t('Header')}</SelectItem>
            <SelectItem value={SOURCE_TIME}>{t('Time')}</SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>
      {condition.source === 'token'
        ? renderTokenCondition(condition)
        : condition.source === SOURCE_TIME
          ? renderTimeCondition(condition as TimeCondition)
          : renderParamHeaderCondition(condition as ParamHeaderCondition)}
      <Button
        variant='ghost'
        size='icon'
        onClick={onRemove}
        aria-label={t('Remove condition')}
        className='ml-auto'
      >
        <Trash2 className='text-destructive h-4 w-4' />
      </Button>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Request rule group card
// ---------------------------------------------------------------------------

type RuleGroupCardProps = {
  group: RequestRuleGroup
  index: number
  onChange: (next: RequestRuleGroup) => void
  onRemove: () => void
}

function RuleGroupCard({
  group,
  index,
  onChange,
  onRemove,
}: RuleGroupCardProps) {
  const { t } = useTranslation()

  const handleConditionChange = (
    conditionIndex: number,
    next: RequestCondition
  ) => {
    const conditions = [...group.conditions]
    conditions[conditionIndex] = next
    onChange({ ...group, conditions })
  }

  const handleAddCondition = (timeMode: boolean) => {
    onChange({
      ...group,
      conditions: [
        ...group.conditions,
        timeMode ? createEmptyTimeCondition() : createEmptyCondition(),
      ],
    })
  }

  return (
    <div className='bg-muted/30 space-y-3 rounded-md border p-3'>
      <div className='flex items-center justify-between gap-2'>
        <Badge variant='outline'>
          {t('Rule group')} #{index + 1}
        </Badge>
        <Button
          variant='ghost'
          size='icon'
          onClick={onRemove}
          aria-label={t('Remove rule group')}
        >
          <Trash2 className='text-destructive h-4 w-4' />
        </Button>
      </div>

      <div className='space-y-2'>
        {group.conditions.map((condition, conditionIndex) => (
          <RuleConditionRow
            key={conditionIndex}
            condition={condition}
            onChange={(next) => {
              if (next.source !== 'token') {
                handleConditionChange(conditionIndex, next)
              }
            }}
            onRemove={() =>
              onChange({
                ...group,
                conditions: group.conditions.filter(
                  (_, i) => i !== conditionIndex
                ),
              })
            }
          />
        ))}
        <div className='flex flex-wrap gap-2'>
          <Button
            variant='ghost'
            size='sm'
            onClick={() => handleAddCondition(false)}
          >
            <Plus className='mr-1 h-3 w-3' />
            {t('Add param/header')}
          </Button>
          <Button
            variant='ghost'
            size='sm'
            onClick={() => handleAddCondition(true)}
          >
            <Plus className='mr-1 h-3 w-3' />
            {t('Add time condition')}
          </Button>
        </div>
      </div>

      <div className='flex items-center gap-2'>
        <Label className='text-xs'>{t('Multiplier')}</Label>
        <DraftNumberInput
          min={0}
          step={0.0001}
          value={group.multiplier}
          onValueChange={(value) =>
            onChange({ ...group, multiplier: String(value) })
          }
          className='w-32'
          placeholder='1.0'
        />
        <span className='text-muted-foreground text-xs'>
          {t('Final cost = base × multiplier when conditions match')}
        </span>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Preset section
// ---------------------------------------------------------------------------

type PresetSectionProps = {
  applyPreset: (preset: Preset) => void
}

function PresetSection({ applyPreset }: PresetSectionProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const visible = expanded ? PRESET_GROUPS : PRESET_GROUPS.slice(0, 2)
  const hasMore = PRESET_GROUPS.length > 2

  return (
    <div className='space-y-2'>
      <div className='flex items-center gap-2'>
        <span className='text-muted-foreground text-xs'>
          {t('Preset templates')}
        </span>
        {hasMore && (
          <Button
            variant='ghost'
            size='sm'
            className='h-6 px-2 text-xs'
            onClick={() => setExpanded((prev) => !prev)}
          >
            {expanded ? t('Collapse') : t('More templates...')}
          </Button>
        )}
      </div>
      <div className='space-y-1'>
        {visible.map((presetGroup) => (
          <div
            key={presetGroup.group}
            className='flex flex-wrap items-center gap-2'
          >
            <Badge variant='secondary' className='min-w-[60px] justify-center'>
              {t(presetGroup.group)}
            </Badge>
            {presetGroup.presets.map((preset) => (
              <Button
                key={preset.key}
                variant='outline'
                size='sm'
                className='h-7 text-xs'
                onClick={() => applyPreset(preset)}
              >
                {preset.label}
              </Button>
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Cost estimator
// ---------------------------------------------------------------------------

type EstimatorProps = {
  effectiveExpr: string
}

function CostEstimator({ effectiveExpr }: EstimatorProps) {
  const { t } = useTranslation()
  const [format, setFormat] = useState<'openai' | 'anthropic'>('openai')
  const [promptTokens, setPromptTokens] = useState(0)
  const [completionTokens, setCompletionTokens] = useState(0)
  const [extras, setExtras] = useState<Record<string, number>>({})
  const usesExtras =
    extraTokenPattern.test(effectiveExpr) ||
    (format === 'anthropic' && /\blen\b/.test(effectiveExpr))
  const sample = useMemo(
    () => ({
      expression: effectiveExpr.trim(),
      format,
      p: promptTokens,
      c: completionTokens,
      ...extras,
      cc1h: format === 'anthropic' ? extras.cc1h : undefined,
      cc12h: format === 'openai' ? extras.cc12h : undefined,
    }),
    [effectiveExpr, format, promptTokens, completionTokens, extras]
  )
  const debounced = useDebounce(sample, 300)
  const ready = !!sample.expression && debounced === sample
  const preview = useQuery({
    queryKey: ['billing-preview', debounced],
    enabled: ready,
    retry: false,
    gcTime: 0,
    staleTime: 0,
    queryFn: async ({ signal }) => {
      const options = {
        signal,
        skipBusinessError: true,
        skipErrorHandler: true,
      }
      const { data } = await api.post<{
        success: boolean
        message: string
        data: { quota: number; tier: string }
      }>('/api/option/billing-preview', debounced, options)
      if (!data.success) throw new Error(data.message || t('Request failed'))
      return data.data
    },
  })
  const result = ready && !preview.isFetching ? preview.data : undefined
  const error = ready ? preview.error : null

  return (
    <div className='bg-muted/30 space-y-3 rounded-md border p-3'>
      <div className='space-y-1'>
        <h4 className='text-sm font-medium'>{t('Token estimator')}</h4>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Enter token counts to preview the estimated cost (excluding group multipliers).'
          )}
        </p>
      </div>
      <Field>
        <FieldLabel>{t('Upstream usage format')}</FieldLabel>
        <ToggleGroup
          aria-label={t('Upstream usage format')}
          variant='outline'
          size='sm'
          value={[format]}
          onValueChange={([value]) => {
            if (value === 'openai' || value === 'anthropic') setFormat(value)
          }}
        >
          <ToggleGroupItem value='openai'>OpenAI</ToggleGroupItem>
          <ToggleGroupItem value='anthropic'>Claude</ToggleGroupItem>
        </ToggleGroup>
      </Field>
      <div className='grid grid-cols-2 gap-3'>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('Input tokens')}</Label>
          <DraftNumberInput
            min={0}
            value={promptTokens}
            onValueChange={setPromptTokens}
          />
        </div>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('Output tokens')}</Label>
          <DraftNumberInput
            min={0}
            value={completionTokens}
            onValueChange={setCompletionTokens}
          />
        </div>
      </div>
      {usesExtras && (
        <div className='grid grid-cols-2 gap-3'>
          {ESTIMATOR_TOKEN_PRICE_VARS.map((variable) => {
            if (variable.key === 'cc12h' && format !== 'openai') return null
            if (variable.key === 'cc1h' && format !== 'anthropic') return null
            return (
              <div key={variable.key} className='space-y-1'>
                <Label className='text-xs'>{t(variable.key === 'cc' && format === 'openai' ? 'Cache Write (30m)' : variable.shortLabel)}</Label>
                <DraftNumberInput
                  min={0}
                  value={extras[variable.key] ?? 0}
                  onValueChange={(value) =>
                    setExtras((prev) => ({
                      ...prev,
                      [variable.key]: value,
                    }))
                  }
                />
              </div>
            )
          })}
        </div>
      )}
      <div
        className={cn(
          'rounded-md border p-3 text-sm',
          error
            ? 'border-destructive/50 bg-destructive/10 text-destructive'
            : 'border-primary/50 bg-primary/10'
        )}
      >
        {error ? (
          <span>
            {t('Expression error')}: {error.message}
          </span>
        ) : result ? (
          <div className='flex items-center gap-2'>
            <span className='font-medium'>
              {t('Estimated quota cost')}: {result.quota.toLocaleString()}
            </span>
            {result.tier && (
              <Badge variant='outline' className='text-xs'>
                {t('Hit tier')}: {result.tier}
              </Badge>
            )}
          </div>
        ) : sample.expression ? (
          t('Loading...')
        ) : (
          '—'
        )}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// LLM prompt helper
// ---------------------------------------------------------------------------

const LLM_PROMPT_TEMPLATE = `You are an AI API billing expression design assistant. The user needs help designing a billing expression for an AI API gateway.

## Expression Language

Expressions are based on standard arithmetic with ternary operators.

### Token Variables

Input side:
- p — input token count (for pricing). Automatically excludes sub-categories priced separately (e.g., if cr is used, cache tokens are deducted from p)
- len — total input context length (for condition checks). Not affected by auto-exclusion; always reflects the full input length. Use in tier conditions
- cr — cache-hit (read) token count
- cc — cache-create token count (5-min TTL)
- cc12h — 12-hour OpenAI cache writes. When used, cc counts remaining writes.
- cc1h — cache-create token count (1-hour TTL, Claude-specific)
- img — image input token count
- ai — audio input token count

Output side:
- c — output token count. Also auto-excludes sub-categories priced separately
- img_o — image output token count
- ao — audio output token count

### p/c Auto-exclusion

p and c are fallback variables representing all tokens not separately priced in the expression. If the expression uses a sub-category variable (e.g., cr), those tokens are deducted from p to avoid double-billing. Unused sub-category tokens remain in p/c at base price.

Important: len is NOT affected by auto-exclusion. Tier conditions should use len instead of p to prevent cache hits from lowering p and misidentifying the tier.

### Built-in Functions

- tier(name, value) — labels the billing tier; must wrap the cost expression
- max(a, b), min(a, b) — maximum/minimum
- ceil(x), floor(x), abs(x) — ceiling, floor, absolute value
- header(name) — reads a request header
- param(path) — reads a request body JSON path (gjson syntax)
- has(source, substr) — substring check
- hour(tz), minute(tz), weekday(tz), month(tz), day(tz) — time functions, tz is a timezone like "Asia/Shanghai"

### Price Coefficients

Numbers in the expression are $/1M tokens prices. For example, p * 2.5 means input $2.50/1M tokens.

## Expression Examples

Simple pricing:
tier("base", p * 2.5 + c * 15)

With cache:
tier("base", p * 2.5 + c * 15 + cr * 0.25)

Multi-tier (use len for conditions):
len <= 200000
  ? tier("standard", p * 3 + c * 15 + cr * 0.3 + cc * 3.75 + cc1h * 6)
  : tier("long_context", p * 6 + c * 22.5 + cr * 0.6 + cc * 7.5 + cc1h * 12)

Image model:
tier("base", p * 2 + c * 8 + img * 2.5)

Multimodal with audio:
tier("base", p * 0.43 + c * 3.06 + img * 0.78 + ai * 3.81 + ao * 15.11)

Three-tier example:
len <= 128000
  ? tier("standard", p * 1.1 + c * 4.4)
  : (len <= 1000000
    ? tier("medium", p * 2.2 + c * 8.8)
    : tier("long", p * 4.4 + c * 17.6))

## Rules

1. Every leaf branch must be wrapped in tier("name", cost_expr)
2. Use English tier names, e.g. "base", "standard", "long_context"
3. Use len for tier conditions (not p), supports <, <=, >, >=
4. Multi-tier uses nested ternary: cond1 ? tier(...) : (cond2 ? tier(...) : tier(...))
5. Price coefficients are the provider's official $/1M tokens prices
6. If cache/image/audio don't need separate pricing, omit those variables; their tokens are included in p/c automatically

Please generate a billing expression based on the model information and pricing requirements provided.`

type LlmPromptHelperProps = {
  modelName?: string
}

function LlmPromptHelper({ modelName }: LlmPromptHelperProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  const prompt = useMemo(() => {
    if (modelName) {
      return LLM_PROMPT_TEMPLATE + `\n\nCurrent model: ${modelName}`
    }
    return LLM_PROMPT_TEMPLATE
  }, [modelName])

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(prompt)
      toast.success(t('Copied to clipboard'))
    } catch {
      toast.error(t('Failed to copy'))
    }
  }, [prompt, t])

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger
        render={
          <Button variant='ghost' size='sm' className='h-7 px-2 text-xs' />
        }
      >
        <Copy className='mr-1.5 h-3 w-3' />
        {t('LLM prompt helper')}
      </CollapsibleTrigger>
      <CollapsibleContent className='mt-2'>
        <div className='bg-muted/30 rounded-md border p-3'>
          <div className='mb-2 flex items-center justify-between'>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Copy this prompt and send it to an LLM (e.g. ChatGPT / Claude) to help design your billing expression.'
              )}
            </p>
            <Button
              variant='outline'
              size='sm'
              className='ml-3 shrink-0'
              onClick={handleCopy}
            >
              <Copy className='mr-1.5 h-3 w-3' />
              {t('Copy prompt')}
            </Button>
          </div>
          <Textarea
            value={prompt}
            readOnly
            rows={8}
            className='font-mono text-xs'
            spellCheck={false}
          />
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

// ---------------------------------------------------------------------------
// Main editor
// ---------------------------------------------------------------------------

export type TieredPricingEditorProps = {
  modelName?: string
  billingExpr: string
  requestRuleExpr: string
  onBillingExprChange: (next: string) => void
  onRequestRuleExprChange: (next: string) => void
}

type EditorMode = 'visual' | 'raw'

export const TieredPricingEditor = memo(function TieredPricingEditor({
  modelName = '',
  billingExpr: currentExpr,
  requestRuleExpr: currentRequestRuleExpr,
  onBillingExprChange,
  onRequestRuleExprChange,
}: TieredPricingEditorProps) {
  const { t } = useTranslation()
  const [state, setState] = useState(() =>
    createPricingEditorState(modelName, currentExpr, currentRequestRuleExpr)
  )
  // Reset only when the controlled source changes (including asynchronously loaded data).
  // Edits update both this snapshot and the parent in the same event.
  if (
    state.modelName !== modelName ||
    state.billingExpr !== currentExpr ||
    state.requestRuleExpr !== currentRequestRuleExpr
  ) {
    setState(
      createPricingEditorState(modelName, currentExpr, currentRequestRuleExpr)
    )
  }
  const editorMode = state.mode
  const visualConfig = state.visualConfig
  const rawExpr = state.rawExpr
  const requestRuleGroups = state.ruleGroups
  const canUseVisualRules =
    tryParseRequestRuleExpr(currentRequestRuleExpr) !== null
  const effectiveExpr = state.rawExpr

  const publishEdit = (next: typeof state) => {
    setState(next)
    if (next.billingExpr !== currentExpr) onBillingExprChange(next.billingExpr)
    if (next.requestRuleExpr !== currentRequestRuleExpr)
      onRequestRuleExprChange(next.requestRuleExpr)
  }

  const handleVisualChange = (next: VisualConfig) =>
    publishEdit(editVisualPricing(state, next))
  const handleRawChange = (value: string) =>
    publishEdit(editRawPricing(state, value))
  const handleRuleGroupsChange = (next: RequestRuleGroup[]) =>
    publishEdit(editPricingRules(state, next))
  const handleModeChange = (next: EditorMode) => {
    const updated = switchPricingEditorMode(state, next)
    if (updated.mode !== next) {
      toast.info(
        t(
          'This expression is too complex for the visual editor. Please switch to expression mode to edit.'
        )
      )
      return
    }
    setState(updated)
  }
  const applyPreset = (preset: Preset) => {
    publishEdit(
      createPricingEditorState(
        modelName,
        preset.expr,
        buildRequestRuleExpr(preset.requestRules || [])
      )
    )
  }

  return (
    <div className='space-y-4'>
      <div className='flex items-center justify-between gap-2'>
        <Label className='text-xs'>{t('Editor mode')}</Label>
        <Select
          items={[
            { value: 'visual', label: t('Visual editor') },
            { value: 'raw', label: t('Expression editor') },
          ]}
          value={editorMode}
          onValueChange={(value) => handleModeChange(value as EditorMode)}
        >
          <SelectTrigger className='w-44' size='sm'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value='visual'>{t('Visual editor')}</SelectItem>
              <SelectItem value='raw'>{t('Expression editor')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>

      <div className='flex flex-wrap items-start gap-x-4 gap-y-1'>
        <div className='flex-1'>
          <PresetSection applyPreset={applyPreset} />
        </div>
        {editorMode === 'raw' && <LlmPromptHelper modelName={modelName} />}
      </div>

      {editorMode === 'raw' &&
        switchPricingEditorMode(state, 'visual').mode === 'raw' && (
          <Alert>
            <AlertDescription>
              {t(
                'This expression is too complex for the visual editor. Please switch to expression mode to edit.'
              )}
            </AlertDescription>
          </Alert>
        )}
      <div className='bg-muted/30 space-y-3 rounded-md border p-3'>
        {editorMode === 'visual' ? (
          <VisualEditor
            visualConfig={visualConfig}
            onChange={handleVisualChange}
          />
        ) : (
          <RawExprEditor exprString={rawExpr} onChange={handleRawChange} />
        )}

        {editorMode === 'visual' && (
          <div className='space-y-3 border-t pt-3'>
            <div className='space-y-1'>
              <h4 className='text-sm font-medium'>
                {t('Request rule pricing')}
              </h4>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'When conditions match, the final price is multiplied by X. Multiple matches multiply together; values < 1 act as discounts.'
                )}
              </p>
            </div>

            {currentRequestRuleExpr && !canUseVisualRules ? (
              <Alert>
                <AlertDescription className='text-xs'>
                  {t(
                    'This expression is too complex for the visual editor. Please switch to expression mode to edit.'
                  )}
                </AlertDescription>
              </Alert>
            ) : (
              <>
                {requestRuleGroups.map((group, groupIndex) => (
                  <RuleGroupCard
                    key={groupIndex}
                    group={group}
                    index={groupIndex}
                    onChange={(next) => {
                      const updated = [...requestRuleGroups]
                      updated[groupIndex] = next
                      handleRuleGroupsChange(updated)
                    }}
                    onRemove={() =>
                      handleRuleGroupsChange(
                        requestRuleGroups.filter((_, i) => i !== groupIndex)
                      )
                    }
                  />
                ))}
                <Button
                  variant='outline'
                  size='sm'
                  className='h-9 w-36 justify-center'
                  onClick={() =>
                    handleRuleGroupsChange([
                      ...requestRuleGroups,
                      createEmptyRuleGroup(),
                    ])
                  }
                >
                  <Plus className='mr-2 h-4 w-4' />
                  {t('Add rule group')}
                </Button>
              </>
            )}
          </div>
        )}
      </div>

      <CostEstimator effectiveExpr={effectiveExpr} />
    </div>
  )
})
