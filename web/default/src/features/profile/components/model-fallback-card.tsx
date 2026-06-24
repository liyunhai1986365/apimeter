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
import { GitBranch, Loader2, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getUserModels } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
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
import { TitledCard } from '@/components/ui/titled-card'
import { updateUserSettings } from '../api'
import { parseUserSettings } from '../lib'
import type {
  ModelFallbackRule,
  UpdateUserSettingsRequest,
  UserModelFallbackMode,
  UserProfile,
} from '../types'

type EditableRule = ModelFallbackRule & { id: string }

type ModelFallbackCardProps = {
  profile: UserProfile | null
  onProfileUpdate: () => void
}

const MODE_OPTIONS: {
  value: UserModelFallbackMode
  label: string
  description: string
}[] = [
  {
    value: 'inherit',
    label: 'Use Global Configuration',
    description: 'Use the administrator configured fallback rules.',
  },
  {
    value: 'custom',
    label: 'Custom Configuration',
    description: 'Use my own primary and fallback model rules.',
  },
  {
    value: 'disabled',
    label: 'Disable Fallback',
    description: 'Always request the primary model only.',
  },
]

function createRule(partial?: Partial<EditableRule>): EditableRule {
  return {
    id: partial?.id ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`,
    primary_model: partial?.primary_model ?? '',
    fallback_model: partial?.fallback_model ?? '',
    enabled: partial?.enabled ?? true,
  }
}

function normalizeRules(rules?: ModelFallbackRule[]): EditableRule[] {
  if (!Array.isArray(rules)) return []
  return rules.map((rule, index) =>
    createRule({
      id: `${index}-${rule.primary_model}-${rule.fallback_model}`,
      primary_model: rule.primary_model || '',
      fallback_model: rule.fallback_model || '',
      enabled: rule.enabled !== false,
    })
  )
}

function buildSettingsPayload(
  profile: UserProfile | null,
  modelFallback: UpdateUserSettingsRequest['model_fallback']
): UpdateUserSettingsRequest {
  const parsed = parseUserSettings(profile?.setting)
  return {
    notify_type: parsed.notify_type || 'email',
    quota_warning_threshold: parsed.quota_warning_threshold ?? 0,
    webhook_url: parsed.webhook_url ?? '',
    webhook_secret: parsed.webhook_secret ?? '',
    notification_email: parsed.notification_email ?? '',
    bark_url: parsed.bark_url ?? '',
    gotify_url: parsed.gotify_url ?? '',
    gotify_token: parsed.gotify_token ?? '',
    gotify_priority: parsed.gotify_priority ?? 5,
    accept_unset_model_ratio_model:
      parsed.accept_unset_model_ratio_model ?? false,
    record_ip_log: parsed.record_ip_log ?? true,
    upstream_model_update_notify_enabled:
      parsed.upstream_model_update_notify_enabled ?? false,
    model_fallback: modelFallback,
  }
}

export function ModelFallbackCard({
  profile,
  onProfileUpdate,
}: ModelFallbackCardProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<UserModelFallbackMode>('inherit')
  const [enabled, setEnabled] = useState(true)
  const [rules, setRules] = useState<EditableRule[]>([])
  const [models, setModels] = useState<string[]>([])
  const [loadingModels, setLoadingModels] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    const parsed = parseUserSettings(profile?.setting)
    const fallback = parsed.model_fallback
    setMode(fallback?.mode || 'inherit')
    setEnabled(fallback?.enabled !== false)
    setRules(normalizeRules(fallback?.rules))
  }, [profile?.setting])

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

  const updateRule = (
    id: string,
    patch: Partial<Omit<EditableRule, 'id'>>
  ) => {
    setRules((prev) =>
      prev.map((rule) => (rule.id === id ? { ...rule, ...patch } : rule))
    )
  }

  const validateRules = () => {
    if (mode !== 'custom') return true
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

    const modelFallback =
      mode === 'custom'
        ? {
            mode,
            enabled,
            rules: rules.map(({ primary_model, fallback_model, enabled }) => ({
              primary_model: primary_model.trim(),
              fallback_model: fallback_model.trim(),
              enabled,
            })),
          }
        : { mode }

    setSaving(true)
    try {
      const response = await updateUserSettings(
        buildSettingsPayload(profile, modelFallback)
      )
      if (response.success) {
        toast.success(t('Settings updated successfully'))
        onProfileUpdate()
      } else {
        toast.error(response.message || t('Failed to update settings'))
      }
    } catch {
      toast.error(t('Failed to update settings'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <TitledCard
      title={t('Model Fallback')}
      description={t('Choose how fallback models are applied to your requests')}
      icon={<GitBranch className='h-4 w-4' />}
    >
      <div className='space-y-5'>
        <RadioGroup
          value={mode}
          onValueChange={(value) => setMode(value as UserModelFallbackMode)}
          className='grid gap-2'
        >
          {MODE_OPTIONS.map((option) => (
            <Label
              key={option.value}
              htmlFor={`model-fallback-${option.value}`}
              className='flex cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors hover:bg-muted/50'
            >
              <RadioGroupItem
                id={`model-fallback-${option.value}`}
                value={option.value}
                className='mt-0.5'
              />
              <span className='space-y-0.5'>
                <span className='block text-sm font-medium'>
                  {t(option.label)}
                </span>
                <span className='text-muted-foreground block text-xs'>
                  {t(option.description)}
                </span>
              </span>
            </Label>
          ))}
        </RadioGroup>

        {mode === 'custom' && (
          <>
            <Separator />
            <div className='flex items-center justify-between gap-4 rounded-lg border p-3'>
              <div className='space-y-0.5'>
                <Label>{t('Enable Custom Fallback')}</Label>
                <p className='text-muted-foreground text-xs'>
                  {t('When disabled, your custom rules are kept but not used.')}
                </p>
              </div>
              <Switch checked={enabled} onCheckedChange={setEnabled} />
            </div>

            <div className='space-y-3'>
              <div className='flex items-center justify-between gap-3'>
                <div>
                  <div className='text-sm font-medium'>{t('Fallback Rules')}</div>
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

              {rules.length === 0 ? (
                <div className='text-muted-foreground rounded-lg border border-dashed p-5 text-center text-sm'>
                  {t('No fallback rules configured')}
                </div>
              ) : (
                <div className='space-y-2'>
                  {rules.map((rule) => (
                    <div key={rule.id} className='space-y-3 rounded-lg border p-3'>
                      <div className='grid gap-2 sm:grid-cols-2'>
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
                      </div>
                      <div className='flex items-center justify-between gap-3'>
                        <div className='flex items-center gap-2'>
                          <Switch
                            checked={rule.enabled}
                            onCheckedChange={(checked) =>
                              updateRule(rule.id, { enabled: checked })
                            }
                          />
                          <span className='text-sm'>{t('Enabled')}</span>
                        </div>
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon'
                          onClick={() =>
                            setRules((prev) =>
                              prev.filter((item) => item.id !== rule.id)
                            )
                          }
                        >
                          <Trash2 className='size-4' />
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </>
        )}

        <Button type='button' onClick={handleSave} disabled={saving}>
          {saving ? t('Saving...') : t('Save Changes')}
        </Button>
      </div>
    </TitledCard>
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
    <div className='flex flex-col gap-2'>
      <Select
        items={selectItems}
        value={value || null}
        onValueChange={(next) => onChange(String(next ?? ''))}
      >
        <SelectTrigger className='w-full'>
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
        className='h-8'
      />
    </div>
  )
}
