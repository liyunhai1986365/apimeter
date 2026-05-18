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
import { Loader2, Plus, RotateCcw, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getUserModels } from '@/lib/api'
import { parseHttpStatusCodeRules } from '@/lib/http-status-code-rules'
import { Button } from '@/components/ui/button'
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
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

type ModelFallbackRule = {
  id: string
  primary_model: string
  fallback_model: string
  enabled: boolean
}

type ModelFallbackSettings = {
  'model_fallback.enabled': boolean
  'model_fallback.allow_user_override': boolean
  'model_fallback.failure_status_codes': string
  'model_fallback.rules': string
}

type ModelFallbackSettingsCardProps = {
  defaultValues: ModelFallbackSettings
}

function createRule(partial?: Partial<ModelFallbackRule>): ModelFallbackRule {
  return {
    id: partial?.id ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`,
    primary_model: partial?.primary_model ?? '',
    fallback_model: partial?.fallback_model ?? '',
    enabled: partial?.enabled ?? true,
  }
}

function parseRules(value: string): ModelFallbackRule[] {
  try {
    const parsed = JSON.parse(value || '[]')
    if (!Array.isArray(parsed)) return []
    return parsed.map((rule, index) =>
      createRule({
        id: `${index}-${rule?.primary_model ?? ''}-${rule?.fallback_model ?? ''}`,
        primary_model: String(rule?.primary_model ?? ''),
        fallback_model: String(rule?.fallback_model ?? ''),
        enabled: rule?.enabled !== false,
      })
    )
  } catch {
    return []
  }
}

function serializeRules(rules: ModelFallbackRule[]) {
  return JSON.stringify(
    rules.map(({ primary_model, fallback_model, enabled }) => ({
      primary_model: primary_model.trim(),
      fallback_model: fallback_model.trim(),
      enabled,
    }))
  )
}

function normalizeRulesJson(value: string) {
  try {
    return JSON.stringify(JSON.parse(value || '[]'))
  } catch {
    return '[]'
  }
}

export function ModelFallbackSettingsCard({
  defaultValues,
}: ModelFallbackSettingsCardProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [enabled, setEnabled] = useState(defaultValues['model_fallback.enabled'])
  const [allowUserOverride, setAllowUserOverride] = useState(
    defaultValues['model_fallback.allow_user_override']
  )
  const [failureStatusCodes, setFailureStatusCodes] = useState(
    defaultValues['model_fallback.failure_status_codes']
  )
  const [rules, setRules] = useState<ModelFallbackRule[]>(() =>
    parseRules(defaultValues['model_fallback.rules'])
  )
  const [models, setModels] = useState<string[]>([])
  const [loadingModels, setLoadingModels] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setEnabled(defaultValues['model_fallback.enabled'])
    setAllowUserOverride(defaultValues['model_fallback.allow_user_override'])
    setFailureStatusCodes(defaultValues['model_fallback.failure_status_codes'])
    setRules(parseRules(defaultValues['model_fallback.rules']))
  }, [defaultValues])

  useEffect(() => {
    let cancelled = false
    async function loadModels() {
      setLoadingModels(true)
      try {
        const response = await getUserModels()
        if (!cancelled && response.success) {
          setModels(Array.from(new Set(response.data ?? [])).sort())
        }
      } catch {
        if (!cancelled) toast.error(t('Failed to load models'))
      } finally {
        if (!cancelled) setLoadingModels(false)
      }
    }
    loadModels()
    return () => {
      cancelled = true
    }
  }, [t])

  const modelItems = useMemo(
    () => models.map((model) => ({ value: model, label: model })),
    [models]
  )
  const failureStatusCodesParsed = useMemo(
    () => parseHttpStatusCodeRules(failureStatusCodes),
    [failureStatusCodes]
  )

  const updateRule = (
    id: string,
    patch: Partial<Omit<ModelFallbackRule, 'id'>>
  ) => {
    setRules((prev) =>
      prev.map((rule) => (rule.id === id ? { ...rule, ...patch } : rule))
    )
  }

  const validateRules = () => {
    if (!failureStatusCodesParsed.ok) {
      toast.error(
        `${t('Invalid status code rules:')} ${failureStatusCodesParsed.invalidTokens.join(', ')}`
      )
      return false
    }
    for (const rule of rules) {
      if (!rule.primary_model.trim() || !rule.fallback_model.trim()) {
        toast.error(t('Primary and fallback models are required'))
        return false
      }
      if (rule.primary_model.trim() === rule.fallback_model.trim()) {
        toast.error(t('Primary and fallback models cannot be the same'))
        return false
      }
    }
    return true
  }

  const handleSave = async () => {
    if (!validateRules()) return

    setSaving(true)
    try {
      const rulesJson = serializeRules(rules)
      const updates: { key: string; value: string }[] = []

      if (enabled !== defaultValues['model_fallback.enabled']) {
        updates.push({ key: 'model_fallback.enabled', value: String(enabled) })
      }
      if (
        allowUserOverride !==
        defaultValues['model_fallback.allow_user_override']
      ) {
        updates.push({
          key: 'model_fallback.allow_user_override',
          value: String(allowUserOverride),
        })
      }
      if (
        failureStatusCodesParsed.normalized !==
        parseHttpStatusCodeRules(
          defaultValues['model_fallback.failure_status_codes']
        ).normalized
      ) {
        updates.push({
          key: 'model_fallback.failure_status_codes',
          value: failureStatusCodesParsed.normalized,
        })
      }
      if (
        rulesJson !== normalizeRulesJson(defaultValues['model_fallback.rules'])
      ) {
        updates.push({ key: 'model_fallback.rules', value: rulesJson })
      }

      if (updates.length === 0) {
        toast.info(t('No changes to save'))
        return
      }

      for (const update of updates) {
        await updateOption.mutateAsync(update)
      }
    } finally {
      setSaving(false)
    }
  }

  return (
    <SettingsSection
      title={t('Model Fallback')}
      description={t(
        'Automatically retry a configured fallback model when the primary model fails.'
      )}
    >
      <div className='space-y-6'>
        <div className='grid gap-3 sm:grid-cols-2'>
          <div className='flex items-center justify-between gap-4 rounded-lg border p-4'>
            <div className='space-y-1'>
              <Label>{t('Enable Model Fallback')}</Label>
              <p className='text-muted-foreground text-xs'>
                {t('Fallback is only attempted after the primary model fails.')}
              </p>
            </div>
            <Switch checked={enabled} onCheckedChange={setEnabled} />
          </div>
          <div className='flex items-center justify-between gap-4 rounded-lg border p-4'>
            <div className='space-y-1'>
              <Label>{t('Allow User Overrides')}</Label>
              <p className='text-muted-foreground text-xs'>
                {t('Users can inherit, disable, or define their own rules.')}
              </p>
            </div>
            <Switch
              checked={allowUserOverride}
              onCheckedChange={setAllowUserOverride}
            />
          </div>
        </div>

        <Separator />

        <div className='space-y-2'>
          <Label>{t('Primary Failure Status Codes')}</Label>
          <Input
            value={failureStatusCodes}
            onChange={(event) => setFailureStatusCodes(event.target.value)}
            placeholder={t('e.g. 401, 403, 429, 500-599')}
          />
          <p className='text-muted-foreground text-xs'>
            {t('Accepts comma-separated status codes and inclusive ranges.')}{' '}
            {failureStatusCodesParsed.ok &&
              failureStatusCodesParsed.normalized &&
              failureStatusCodesParsed.normalized !==
                failureStatusCodes.trim() && (
                <span>
                  {t('Normalized:')} {failureStatusCodesParsed.normalized}
                </span>
              )}
          </p>
        </div>

        <Separator />

        <div className='space-y-3'>
          <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
            <div>
              <h3 className='text-sm font-medium'>{t('Fallback Rules')}</h3>
              <p className='text-muted-foreground text-xs'>
                {t('Each request still starts with the primary model.')}
              </p>
            </div>
            <div className='flex items-center gap-2'>
              {loadingModels && (
                <Loader2 className='text-muted-foreground size-4 animate-spin' />
              )}
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => setRules((prev) => [...prev, createRule()])}
              >
                <Plus className='size-4' />
                {t('Add Rule')}
              </Button>
            </div>
          </div>

          <div className='space-y-2'>
            {rules.length === 0 ? (
              <div className='text-muted-foreground rounded-lg border border-dashed p-6 text-center text-sm'>
                {t('No fallback rules configured')}
              </div>
            ) : (
              rules.map((rule) => (
                <div
                  key={rule.id}
                  className='grid gap-2 rounded-lg border p-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto_auto]'
                >
                  <div className='space-y-1.5'>
                    <Label>{t('Primary Model')}</Label>
                    <ModelSelect
                      value={rule.primary_model}
                      models={models}
                      items={modelItems}
                      placeholder={t('Select primary model')}
                      onChange={(value) =>
                        updateRule(rule.id, { primary_model: value })
                      }
                    />
                  </div>
                  <div className='space-y-1.5'>
                    <Label>{t('Fallback Model')}</Label>
                    <ModelSelect
                      value={rule.fallback_model}
                      models={models}
                      items={modelItems}
                      placeholder={t('Select fallback model')}
                      onChange={(value) =>
                        updateRule(rule.id, { fallback_model: value })
                      }
                    />
                  </div>
                  <div className='flex items-end gap-2'>
                    <div className='flex h-8 items-center gap-2'>
                      <Switch
                        checked={rule.enabled}
                        onCheckedChange={(checked) =>
                          updateRule(rule.id, { enabled: checked })
                        }
                      />
                      <span className='text-sm'>{t('Enabled')}</span>
                    </div>
                  </div>
                  <div className='flex items-end justify-end'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon'
                      onClick={() =>
                        setRules((prev) => prev.filter((r) => r.id !== rule.id))
                      }
                    >
                      <Trash2 className='size-4' />
                    </Button>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>

        <div className='flex flex-col gap-2 sm:flex-row'>
          <Button type='button' onClick={handleSave} disabled={saving}>
            {saving ? t('Saving...') : t('Save Changes')}
          </Button>
          <Button
            type='button'
            variant='outline'
            onClick={() => {
              setEnabled(defaultValues['model_fallback.enabled'])
              setAllowUserOverride(
                defaultValues['model_fallback.allow_user_override']
              )
              setFailureStatusCodes(
                defaultValues['model_fallback.failure_status_codes']
              )
              setRules(parseRules(defaultValues['model_fallback.rules']))
            }}
          >
            <RotateCcw className='size-4' />
            {t('Reset')}
          </Button>
        </div>
      </div>
    </SettingsSection>
  )
}

type ModelSelectProps = {
  value: string
  models: string[]
  items: { value: string; label: string }[]
  placeholder: string
  onChange: (value: string) => void
}

function ModelSelect({
  value,
  models,
  items,
  placeholder,
  onChange,
}: ModelSelectProps) {
  const { t } = useTranslation()
  const hasValueInOptions = value && models.includes(value)
  const selectItems = hasValueInOptions
    ? items
    : value
      ? [{ value, label: value }, ...items]
      : items

  return (
    <div className='flex flex-col gap-2 sm:flex-row'>
      <Select
        items={selectItems}
        value={value || null}
        onValueChange={(next) => onChange(String(next ?? ''))}
      >
        <SelectTrigger className='w-full sm:min-w-56'>
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false} className='max-h-72'>
          <SelectGroup>
            {selectItems.map((model) => (
              <SelectItem key={model.value} value={model.value}>
                {model.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <Input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={t('Or enter model name')}
        className='h-8 sm:max-w-56'
      />
    </div>
  )
}
