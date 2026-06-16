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
import { useMemo, useRef } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { Plus, Send, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { parseHttpStatusCodeRules } from '@/lib/http-status-code-rules'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
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
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  insertRetryPolicyTemplate,
  RETRY_POLICY_TEMPLATES,
} from '@/features/channels/lib'
import { sendGlobalWebhookTest } from '../api'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

const numericString = z.string().refine((value) => {
  const trimmed = value.trim()
  if (!trimmed) return true
  return !Number.isNaN(Number(trimmed)) && Number(trimmed) >= 0
}, 'Enter a non-negative number or leave empty')

const jsonArrayString = z.string().refine((value) => {
  const trimmed = value.trim()
  if (!trimmed) return true
  try {
    return Array.isArray(JSON.parse(trimmed))
  } catch {
    return false
  }
}, 'Must be a valid JSON array')

const autoDisableRuleSchema = z
  .object({
    rule_type: z.enum(['error', 'timeout']),
    id: z.string().min(1, 'Rule ID is required'),
    name: z.string().min(1, 'Rule name is required'),
    enabled: z.boolean(),
    model_names: z.array(z.string()),
    error_types: z.array(z.string()),
    status_codes: z.string(),
    first_token_seconds: z.coerce
      .number()
      .int()
      .min(0, 'First token time cannot be negative'),
    first_token_count_threshold: z.coerce
      .number()
      .int()
      .min(0, 'First-token slow count cannot be negative'),
    window_minutes: z.coerce
      .number()
      .int()
      .min(1, 'Window must be at least 1 minute'),
    min_requests: z.coerce
      .number()
      .int()
      .min(0, 'Minimum requests cannot be negative'),
    error_count_threshold: z.coerce
      .number()
      .int()
      .min(0, 'Error count cannot be negative'),
    error_rate: z.coerce
      .number()
      .min(0, 'Error rate cannot be negative')
      .max(100, 'Error rate cannot exceed 100'),
    per_minute_error_threshold: z.coerce
      .number()
      .int()
      .min(0, 'Per-minute errors cannot be negative'),
    protect_last: z.boolean(),
  })
  .superRefine((rule, ctx) => {
    if (rule.rule_type === 'error' && rule.status_codes.trim()) {
      const parsed = parseHttpStatusCodeRules(rule.status_codes)
      if (!parsed.ok) {
        ctx.addIssue({
          code: 'custom',
          path: ['status_codes'],
          message: `Invalid status code rules: ${parsed.invalidTokens.join(
            ', '
          )}`,
        })
      }
    }

    if (rule.rule_type === 'error') {
      if (
        rule.error_count_threshold === 0 &&
        rule.error_rate === 0 &&
        rule.per_minute_error_threshold === 0
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['error_count_threshold'],
          message: 'Configure at least one trigger threshold',
        })
      }
      return
    }

    if (rule.first_token_seconds === 0) {
      ctx.addIssue({
        code: 'custom',
        path: ['first_token_seconds'],
        message: 'Configure first token seconds',
      })
    }

    if (rule.first_token_count_threshold === 0) {
      ctx.addIssue({
        code: 'custom',
        path: ['first_token_count_threshold'],
        message: 'Configure timeout count',
      })
    }
  })

const monitoringSchema = z
  .object({
    ChannelDisableThreshold: numericString,
    QuotaRemindThreshold: numericString,
    AutomaticDisableChannelEnabled: z.boolean(),
    AutomaticEnableChannelEnabled: z.boolean(),
    AutomaticDisableKeywords: z.string(),
    AutomaticDisableStatusCodes: z.string(),
    AutomaticRetryStatusCodes: z.string(),
    AutomaticRetryPolicyRules: jsonArrayString,
    monitor_setting: z.object({
      auto_test_channel_enabled: z.boolean(),
      auto_test_channel_minutes: z.coerce
        .number()
        .int()
        .min(1, 'Interval must be at least 1 minute'),
      channel_auto_operation_enabled: z.boolean(),
      channel_auto_operation_threshold: z.coerce
        .number()
        .int()
        .min(1, 'Threshold must be at least 1'),
      channel_auto_operation_window_minutes: z.coerce
        .number()
        .int()
        .min(1, 'Window must be at least 1 minute'),
      channel_auto_operation_min_requests: z.coerce
        .number()
        .int()
        .min(1, 'Minimum requests must be at least 1'),
      channel_auto_operation_error_rate: z.coerce
        .number()
        .min(1, 'Error rate must be at least 1')
        .max(100, 'Error rate cannot exceed 100'),
      channel_auto_operation_protect_last: z.boolean(),
      channel_auto_disable_rules: z.array(autoDisableRuleSchema),
    }),
    webhook_setting: z.object({
      enabled: z.boolean(),
      url: z.string(),
      secret: z.string(),
      interval_minutes: z.coerce
        .number()
        .int()
        .min(1, 'Interval must be at least 1 minute'),
      suppress_minutes: z.coerce
        .number()
        .int()
        .min(0, 'Suppress window cannot be negative'),
      notify_on_empty_result: z.boolean(),
      model_error_check_enabled: z.boolean(),
      model_error_window_minutes: z.coerce
        .number()
        .int()
        .min(1, 'Window must be at least 1 minute'),
      model_error_threshold: z.coerce
        .number()
        .int()
        .min(1, 'Threshold must be at least 1'),
      model_error_min_requests: z.coerce
        .number()
        .int()
        .min(1, 'Minimum requests must be at least 1'),
      model_error_rate: z.coerce
        .number()
        .min(1, 'Error rate must be at least 1')
        .max(100, 'Error rate cannot exceed 100'),
      channel_test_check_enabled: z.boolean(),
      channel_test_window_minutes: z.coerce
        .number()
        .int()
        .min(1, 'Window must be at least 1 minute'),
    }),
  })
  .superRefine((values, ctx) => {
    const disableParsed = parseHttpStatusCodeRules(
      values.AutomaticDisableStatusCodes
    )
    if (!disableParsed.ok) {
      ctx.addIssue({
        code: 'custom',
        path: ['AutomaticDisableStatusCodes'],
        message: `Invalid status code rules: ${disableParsed.invalidTokens.join(
          ', '
        )}`,
      })
    }

    const retryParsed = parseHttpStatusCodeRules(
      values.AutomaticRetryStatusCodes
    )
    if (!retryParsed.ok) {
      ctx.addIssue({
        code: 'custom',
        path: ['AutomaticRetryStatusCodes'],
        message: `Invalid status code rules: ${retryParsed.invalidTokens.join(
          ', '
        )}`,
      })
    }

    const webhookURL = values.webhook_setting.url.trim()
    if (values.webhook_setting.enabled && webhookURL === '') {
      ctx.addIssue({
        code: 'custom',
        path: ['webhook_setting', 'url'],
        message: 'Webhook URL is required when global webhook is enabled',
      })
    }

    if (webhookURL !== '') {
      try {
        const parsed = new URL(webhookURL)
        if (!['http:', 'https:'].includes(parsed.protocol)) {
          throw new Error('invalid protocol')
        }
      } catch {
        ctx.addIssue({
          code: 'custom',
          path: ['webhook_setting', 'url'],
          message: 'Enter a valid HTTP or HTTPS URL',
        })
      }
    }
  })

type MonitoringFormValues = z.output<typeof monitoringSchema>
type MonitoringFormInput = z.input<typeof monitoringSchema>
type AutoDisableRuleForm = z.output<typeof autoDisableRuleSchema>

type MonitoringSettingsSectionProps = {
  defaultValues: {
    ChannelDisableThreshold: string
    QuotaRemindThreshold: string
    AutomaticDisableChannelEnabled: boolean
    AutomaticEnableChannelEnabled: boolean
    AutomaticDisableKeywords: string
    AutomaticDisableStatusCodes: string
    AutomaticRetryStatusCodes: string
    AutomaticRetryPolicyRules: string
    'monitor_setting.auto_test_channel_enabled': boolean
    'monitor_setting.auto_test_channel_minutes': number
    'monitor_setting.channel_auto_operation_enabled': boolean
    'monitor_setting.channel_auto_operation_threshold': number
    'monitor_setting.channel_auto_operation_window_minutes': number
    'monitor_setting.channel_auto_operation_min_requests': number
    'monitor_setting.channel_auto_operation_error_rate': number
    'monitor_setting.channel_auto_operation_protect_last': boolean
    'monitor_setting.channel_auto_disable_rules': string
    'webhook_setting.enabled': boolean
    'webhook_setting.url': string
    'webhook_setting.secret': string
    'webhook_setting.interval_minutes': number
    'webhook_setting.suppress_minutes': number
    'webhook_setting.notify_on_empty_result': boolean
    'webhook_setting.model_error_check_enabled': boolean
    'webhook_setting.model_error_window_minutes': number
    'webhook_setting.model_error_threshold': number
    'webhook_setting.model_error_min_requests': number
    'webhook_setting.model_error_rate': number
    'webhook_setting.channel_test_check_enabled': boolean
    'webhook_setting.channel_test_window_minutes': number
  }
}

const defaultAutoDisableRule = (): AutoDisableRuleForm => ({
  rule_type: 'error',
  id: `rule-${Date.now().toString(36)}`,
  name: 'New auto-disable rule',
  enabled: true,
  model_names: [],
  error_types: [],
  status_codes: '',
  first_token_seconds: 0,
  first_token_count_threshold: 0,
  window_minutes: 10,
  min_requests: 5,
  error_count_threshold: 3,
  error_rate: 50,
  per_minute_error_threshold: 0,
  protect_last: true,
})

function normalizeLineEndings(value: string) {
  return value.replace(/\r\n/g, '\n')
}

function splitCSV(value: string) {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

function joinCSV(values: string[] | undefined) {
  return (values || []).join(', ')
}

function parseAutoDisableRules(
  value: string | undefined
): AutoDisableRuleForm[] {
  if (!value?.trim()) return []
  try {
    const parsed = JSON.parse(value)
    if (!Array.isArray(parsed)) return []
    return parsed.map((rule, index) => {
      const firstTokenCountThreshold = Number(
        rule.first_token_count_threshold || 0
      )
      const ruleType =
        rule.rule_type === 'timeout' || firstTokenCountThreshold > 0
          ? 'timeout'
          : 'error'
      return {
        ...defaultAutoDisableRule(),
        rule_type: ruleType,
        id: String(rule.id || `rule-${index + 1}`),
        name: String(rule.name || `Rule ${index + 1}`),
        enabled: rule.enabled ?? true,
        model_names: Array.isArray(rule.model_names) ? rule.model_names : [],
        error_types: Array.isArray(rule.error_types) ? rule.error_types : [],
        status_codes: String(rule.status_codes || ''),
        first_token_seconds: Number(rule.first_token_seconds || 0),
        first_token_count_threshold: firstTokenCountThreshold,
        window_minutes: Number(rule.window_minutes || 10),
        min_requests: Number(rule.min_requests || 0),
        error_count_threshold: Number(rule.error_count_threshold || 0),
        error_rate: Number(rule.error_rate || 0),
        per_minute_error_threshold: Number(
          rule.per_minute_error_threshold || 0
        ),
        protect_last: rule.protect_last ?? true,
      }
    })
  } catch {
    return []
  }
}

function normalizeAutoDisableRules(rules: AutoDisableRuleForm[]) {
  return JSON.stringify(
    rules.map((rule) => {
      const base = {
        id: rule.id.trim(),
        name: rule.name.trim(),
        enabled: rule.enabled,
        model_names: rule.model_names
          .map((item) => item.trim())
          .filter(Boolean),
        window_minutes: rule.window_minutes,
        min_requests: rule.min_requests,
        protect_last: rule.protect_last,
      }

      if (rule.rule_type === 'timeout') {
        return {
          ...base,
          error_types: [],
          status_codes: '',
          first_token_seconds: rule.first_token_seconds,
          first_token_count_threshold: rule.first_token_count_threshold,
          error_count_threshold: 0,
          error_rate: 0,
          per_minute_error_threshold: 0,
        }
      }

      return {
        ...base,
        error_types: rule.error_types
          .map((item) => item.trim())
          .filter(Boolean),
        status_codes: parseHttpStatusCodeRules(rule.status_codes).normalized,
        first_token_seconds: 0,
        first_token_count_threshold: 0,
        error_count_threshold: rule.error_count_threshold,
        error_rate: rule.error_rate,
        per_minute_error_threshold: rule.per_minute_error_threshold,
      }
    })
  )
}

type NormalizedMonitoringValues = {
  ChannelDisableThreshold: string
  QuotaRemindThreshold: string
  AutomaticDisableChannelEnabled: boolean
  AutomaticEnableChannelEnabled: boolean
  AutomaticDisableKeywords: string
  AutomaticDisableStatusCodes: string
  AutomaticRetryStatusCodes: string
  AutomaticRetryPolicyRules: string
  'monitor_setting.auto_test_channel_enabled': boolean
  'monitor_setting.auto_test_channel_minutes': number
  'monitor_setting.channel_auto_operation_enabled': boolean
  'monitor_setting.channel_auto_operation_threshold': number
  'monitor_setting.channel_auto_operation_window_minutes': number
  'monitor_setting.channel_auto_operation_min_requests': number
  'monitor_setting.channel_auto_operation_error_rate': number
  'monitor_setting.channel_auto_operation_protect_last': boolean
  'monitor_setting.channel_auto_disable_rules': string
  'webhook_setting.enabled': boolean
  'webhook_setting.url': string
  'webhook_setting.secret': string
  'webhook_setting.interval_minutes': number
  'webhook_setting.suppress_minutes': number
  'webhook_setting.notify_on_empty_result': boolean
  'webhook_setting.model_error_check_enabled': boolean
  'webhook_setting.model_error_window_minutes': number
  'webhook_setting.model_error_threshold': number
  'webhook_setting.model_error_min_requests': number
  'webhook_setting.model_error_rate': number
  'webhook_setting.channel_test_check_enabled': boolean
  'webhook_setting.channel_test_window_minutes': number
}

const buildFormDefaults = (
  defaults: MonitoringSettingsSectionProps['defaultValues']
): MonitoringFormInput => ({
  ChannelDisableThreshold: defaults.ChannelDisableThreshold ?? '',
  QuotaRemindThreshold: defaults.QuotaRemindThreshold ?? '',
  AutomaticDisableChannelEnabled: defaults.AutomaticDisableChannelEnabled,
  AutomaticEnableChannelEnabled: defaults.AutomaticEnableChannelEnabled,
  AutomaticDisableKeywords: normalizeLineEndings(
    defaults.AutomaticDisableKeywords ?? ''
  ),
  AutomaticDisableStatusCodes: defaults.AutomaticDisableStatusCodes ?? '',
  AutomaticRetryStatusCodes: defaults.AutomaticRetryStatusCodes ?? '',
  AutomaticRetryPolicyRules: defaults.AutomaticRetryPolicyRules ?? '[]',
  monitor_setting: {
    auto_test_channel_enabled:
      defaults['monitor_setting.auto_test_channel_enabled'],
    auto_test_channel_minutes:
      defaults['monitor_setting.auto_test_channel_minutes'],
    channel_auto_operation_enabled:
      defaults['monitor_setting.channel_auto_operation_enabled'],
    channel_auto_operation_threshold:
      defaults['monitor_setting.channel_auto_operation_threshold'],
    channel_auto_operation_window_minutes:
      defaults['monitor_setting.channel_auto_operation_window_minutes'],
    channel_auto_operation_min_requests:
      defaults['monitor_setting.channel_auto_operation_min_requests'],
    channel_auto_operation_error_rate:
      defaults['monitor_setting.channel_auto_operation_error_rate'],
    channel_auto_operation_protect_last:
      defaults['monitor_setting.channel_auto_operation_protect_last'],
    channel_auto_disable_rules: parseAutoDisableRules(
      defaults['monitor_setting.channel_auto_disable_rules']
    ),
  },
  webhook_setting: {
    enabled: defaults['webhook_setting.enabled'],
    url: defaults['webhook_setting.url'] ?? '',
    secret: defaults['webhook_setting.secret'] ?? '',
    interval_minutes: defaults['webhook_setting.interval_minutes'],
    suppress_minutes: defaults['webhook_setting.suppress_minutes'],
    notify_on_empty_result: defaults['webhook_setting.notify_on_empty_result'],
    model_error_check_enabled:
      defaults['webhook_setting.model_error_check_enabled'],
    model_error_window_minutes:
      defaults['webhook_setting.model_error_window_minutes'],
    model_error_threshold: defaults['webhook_setting.model_error_threshold'],
    model_error_min_requests:
      defaults['webhook_setting.model_error_min_requests'],
    model_error_rate: defaults['webhook_setting.model_error_rate'],
    channel_test_check_enabled:
      defaults['webhook_setting.channel_test_check_enabled'],
    channel_test_window_minutes:
      defaults['webhook_setting.channel_test_window_minutes'],
  },
})

const normalizeDefaults = (
  defaults: MonitoringSettingsSectionProps['defaultValues']
): NormalizedMonitoringValues => ({
  ChannelDisableThreshold: (defaults.ChannelDisableThreshold ?? '').trim(),
  QuotaRemindThreshold: (defaults.QuotaRemindThreshold ?? '').trim(),
  AutomaticDisableChannelEnabled: defaults.AutomaticDisableChannelEnabled,
  AutomaticEnableChannelEnabled: defaults.AutomaticEnableChannelEnabled,
  AutomaticDisableKeywords: normalizeLineEndings(
    defaults.AutomaticDisableKeywords ?? ''
  ),
  AutomaticDisableStatusCodes: parseHttpStatusCodeRules(
    defaults.AutomaticDisableStatusCodes ?? ''
  ).normalized,
  AutomaticRetryStatusCodes: parseHttpStatusCodeRules(
    defaults.AutomaticRetryStatusCodes ?? ''
  ).normalized,
  AutomaticRetryPolicyRules: (
    defaults.AutomaticRetryPolicyRules ?? '[]'
  ).trim(),
  'monitor_setting.auto_test_channel_enabled':
    defaults['monitor_setting.auto_test_channel_enabled'],
  'monitor_setting.auto_test_channel_minutes':
    defaults['monitor_setting.auto_test_channel_minutes'],
  'monitor_setting.channel_auto_operation_enabled':
    defaults['monitor_setting.channel_auto_operation_enabled'],
  'monitor_setting.channel_auto_operation_threshold':
    defaults['monitor_setting.channel_auto_operation_threshold'],
  'monitor_setting.channel_auto_operation_window_minutes':
    defaults['monitor_setting.channel_auto_operation_window_minutes'],
  'monitor_setting.channel_auto_operation_min_requests':
    defaults['monitor_setting.channel_auto_operation_min_requests'],
  'monitor_setting.channel_auto_operation_error_rate':
    defaults['monitor_setting.channel_auto_operation_error_rate'],
  'monitor_setting.channel_auto_operation_protect_last':
    defaults['monitor_setting.channel_auto_operation_protect_last'],
  'monitor_setting.channel_auto_disable_rules': normalizeAutoDisableRules(
    parseAutoDisableRules(
      defaults['monitor_setting.channel_auto_disable_rules']
    )
  ),
  'webhook_setting.enabled': defaults['webhook_setting.enabled'],
  'webhook_setting.url': (defaults['webhook_setting.url'] ?? '').trim(),
  'webhook_setting.secret': defaults['webhook_setting.secret'] ?? '',
  'webhook_setting.interval_minutes':
    defaults['webhook_setting.interval_minutes'],
  'webhook_setting.suppress_minutes':
    defaults['webhook_setting.suppress_minutes'],
  'webhook_setting.notify_on_empty_result':
    defaults['webhook_setting.notify_on_empty_result'],
  'webhook_setting.model_error_check_enabled':
    defaults['webhook_setting.model_error_check_enabled'],
  'webhook_setting.model_error_window_minutes':
    defaults['webhook_setting.model_error_window_minutes'],
  'webhook_setting.model_error_threshold':
    defaults['webhook_setting.model_error_threshold'],
  'webhook_setting.model_error_min_requests':
    defaults['webhook_setting.model_error_min_requests'],
  'webhook_setting.model_error_rate':
    defaults['webhook_setting.model_error_rate'],
  'webhook_setting.channel_test_check_enabled':
    defaults['webhook_setting.channel_test_check_enabled'],
  'webhook_setting.channel_test_window_minutes':
    defaults['webhook_setting.channel_test_window_minutes'],
})

const normalizeFormValues = (
  values: MonitoringFormValues
): NormalizedMonitoringValues => ({
  ChannelDisableThreshold: values.ChannelDisableThreshold.trim(),
  QuotaRemindThreshold: values.QuotaRemindThreshold.trim(),
  AutomaticDisableChannelEnabled: values.AutomaticDisableChannelEnabled,
  AutomaticEnableChannelEnabled: values.AutomaticEnableChannelEnabled,
  AutomaticDisableKeywords: normalizeLineEndings(
    values.AutomaticDisableKeywords
  ),
  AutomaticDisableStatusCodes: parseHttpStatusCodeRules(
    values.AutomaticDisableStatusCodes
  ).normalized,
  AutomaticRetryStatusCodes: parseHttpStatusCodeRules(
    values.AutomaticRetryStatusCodes
  ).normalized,
  AutomaticRetryPolicyRules: values.AutomaticRetryPolicyRules.trim() || '[]',
  'monitor_setting.auto_test_channel_enabled':
    values.monitor_setting.auto_test_channel_enabled,
  'monitor_setting.auto_test_channel_minutes':
    values.monitor_setting.auto_test_channel_minutes,
  'monitor_setting.channel_auto_operation_enabled':
    values.monitor_setting.channel_auto_operation_enabled,
  'monitor_setting.channel_auto_operation_threshold':
    values.monitor_setting.channel_auto_operation_threshold,
  'monitor_setting.channel_auto_operation_window_minutes':
    values.monitor_setting.channel_auto_operation_window_minutes,
  'monitor_setting.channel_auto_operation_min_requests':
    values.monitor_setting.channel_auto_operation_min_requests,
  'monitor_setting.channel_auto_operation_error_rate':
    values.monitor_setting.channel_auto_operation_error_rate,
  'monitor_setting.channel_auto_operation_protect_last':
    values.monitor_setting.channel_auto_operation_protect_last,
  'monitor_setting.channel_auto_disable_rules': normalizeAutoDisableRules(
    values.monitor_setting.channel_auto_disable_rules
  ),
  'webhook_setting.enabled': values.webhook_setting.enabled,
  'webhook_setting.url': values.webhook_setting.url.trim(),
  'webhook_setting.secret': values.webhook_setting.secret,
  'webhook_setting.interval_minutes': values.webhook_setting.interval_minutes,
  'webhook_setting.suppress_minutes': values.webhook_setting.suppress_minutes,
  'webhook_setting.notify_on_empty_result':
    values.webhook_setting.notify_on_empty_result,
  'webhook_setting.model_error_check_enabled':
    values.webhook_setting.model_error_check_enabled,
  'webhook_setting.model_error_window_minutes':
    values.webhook_setting.model_error_window_minutes,
  'webhook_setting.model_error_threshold':
    values.webhook_setting.model_error_threshold,
  'webhook_setting.model_error_min_requests':
    values.webhook_setting.model_error_min_requests,
  'webhook_setting.model_error_rate': values.webhook_setting.model_error_rate,
  'webhook_setting.channel_test_check_enabled':
    values.webhook_setting.channel_test_check_enabled,
  'webhook_setting.channel_test_window_minutes':
    values.webhook_setting.channel_test_window_minutes,
})

export function MonitoringSettingsSection({
  defaultValues,
}: MonitoringSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const sendWebhookTest = useMutation({
    mutationFn: sendGlobalWebhookTest,
    onSuccess: (data) => {
      if (data.success) {
        toast.success(data.message || t('Webhook test alert sent'))
      } else {
        toast.error(data.message || t('Failed to send webhook test alert'))
      }
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to send webhook test alert'))
    },
  })
  const baselineRef = useRef<NormalizedMonitoringValues>(
    normalizeDefaults(defaultValues)
  )

  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )

  const form = useForm<MonitoringFormInput, unknown, MonitoringFormValues>({
    resolver: zodResolver(monitoringSchema),
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  const autoDisableStatusCodes = form.watch('AutomaticDisableStatusCodes')
  const autoRetryStatusCodes = form.watch('AutomaticRetryStatusCodes')
  const channelAutoOperationEnabled = form.watch(
    'monitor_setting.channel_auto_operation_enabled'
  )
  const autoDisableRules = form.watch(
    'monitor_setting.channel_auto_disable_rules'
  ) as AutoDisableRuleForm[]
  const globalWebhookEnabled = form.watch('webhook_setting.enabled')
  const modelErrorCheckEnabled = form.watch(
    'webhook_setting.model_error_check_enabled'
  )
  const channelTestCheckEnabled = form.watch(
    'webhook_setting.channel_test_check_enabled'
  )
  const autoDisableParsed = useMemo(
    () => parseHttpStatusCodeRules(autoDisableStatusCodes),
    [autoDisableStatusCodes]
  )
  const autoRetryParsed = useMemo(
    () => parseHttpStatusCodeRules(autoRetryStatusCodes),
    [autoRetryStatusCodes]
  )

  const onSubmit = async (values: MonitoringFormValues) => {
    const normalized = normalizeFormValues(values)
    const updates = (
      Object.keys(normalized) as Array<keyof NormalizedMonitoringValues>
    ).filter((key) => normalized[key] !== baselineRef.current[key])

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of updates) {
      const value = normalized[key]
      await updateOption.mutateAsync({
        key,
        value,
      })
    }

    baselineRef.current = normalized
  }

  const handleSendWebhookTest = () => {
    if (form.formState.isDirty) {
      toast.info(t('Save webhook settings before sending a test alert'))
      return
    }
    sendWebhookTest.mutate()
  }

  const updateRule = (index: number, patch: Partial<AutoDisableRuleForm>) => {
    const current = form.getValues('monitor_setting.channel_auto_disable_rules')
    const next = current.map((rule, ruleIndex) =>
      ruleIndex === index ? { ...rule, ...patch } : rule
    )
    form.setValue('monitor_setting.channel_auto_disable_rules', next, {
      shouldDirty: true,
      shouldValidate: true,
    })
  }

  const updateRuleType = (
    index: number,
    ruleType: AutoDisableRuleForm['rule_type']
  ) => {
    if (ruleType === 'timeout') {
      updateRule(index, {
        rule_type: ruleType,
        error_types: [],
        status_codes: '',
        error_count_threshold: 0,
        error_rate: 0,
        per_minute_error_threshold: 0,
        first_token_seconds: 30,
        first_token_count_threshold: 3,
      })
      return
    }

    updateRule(index, {
      rule_type: ruleType,
      first_token_seconds: 0,
      first_token_count_threshold: 0,
      error_count_threshold: 3,
      error_rate: 50,
      per_minute_error_threshold: 0,
    })
  }

  const addRule = () => {
    const current = form.getValues('monitor_setting.channel_auto_disable_rules')
    form.setValue(
      'monitor_setting.channel_auto_disable_rules',
      [...current, defaultAutoDisableRule()],
      { shouldDirty: true, shouldValidate: true }
    )
  }

  const removeRule = (index: number) => {
    const current = form.getValues('monitor_setting.channel_auto_disable_rules')
    form.setValue(
      'monitor_setting.channel_auto_disable_rules',
      current.filter((_, ruleIndex) => ruleIndex !== index),
      { shouldDirty: true, shouldValidate: true }
    )
  }

  return (
    <SettingsSection
      title={t('Monitoring & Alerts')}
      description={t(
        'Automatically test channels and notify users when limits are hit'
      )}
    >
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-6'>
          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='monitor_setting.auto_test_channel_enabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Scheduled channel tests')}
                    </FormLabel>
                    <FormDescription>
                      {t('Automatically probe all channels in the background')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='monitor_setting.auto_test_channel_minutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Test interval (minutes)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      value={
                        typeof field.value === 'number' &&
                        Number.isFinite(field.value)
                          ? field.value
                          : ''
                      }
                      onChange={(event) =>
                        field.onChange(event.target.valueAsNumber)
                      }
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('How frequently the system tests all channels')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='grid gap-6 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='monitor_setting.channel_auto_operation_enabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4 md:col-span-1'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Channel auto operations')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Disable channels only after repeated model errors in the selected window'
                      )}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='monitor_setting.channel_auto_operation_threshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Model error threshold')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      disabled={!channelAutoOperationEnabled}
                      value={
                        typeof field.value === 'number' &&
                        Number.isFinite(field.value)
                          ? field.value
                          : ''
                      }
                      onChange={(event) =>
                        field.onChange(event.target.valueAsNumber)
                      }
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Disable the channel when one model reaches this many errors'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='monitor_setting.channel_auto_operation_window_minutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Error window (minutes)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      disabled={!channelAutoOperationEnabled}
                      value={
                        typeof field.value === 'number' &&
                        Number.isFinite(field.value)
                          ? field.value
                          : ''
                      }
                      onChange={(event) =>
                        field.onChange(event.target.valueAsNumber)
                      }
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Only recent errors within this window are counted')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='monitor_setting.channel_auto_operation_min_requests'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Minimum requests')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      disabled={!channelAutoOperationEnabled}
                      value={
                        typeof field.value === 'number' &&
                        Number.isFinite(field.value)
                          ? field.value
                          : ''
                      }
                      onChange={(event) =>
                        field.onChange(event.target.valueAsNumber)
                      }
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Ignore low-volume models until this many requests exist'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='monitor_setting.channel_auto_operation_error_rate'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Error rate threshold (%)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={100}
                      step={1}
                      disabled={!channelAutoOperationEnabled}
                      value={
                        typeof field.value === 'number' &&
                        Number.isFinite(field.value)
                          ? field.value
                          : ''
                      }
                      onChange={(event) =>
                        field.onChange(event.target.valueAsNumber)
                      }
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Disable only when the model error rate reaches this percentage'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='monitor_setting.channel_auto_operation_protect_last'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Protect last channel')}
                    </FormLabel>
                    <FormDescription>
                      {t('Keep at least one available channel for each model')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      disabled={!channelAutoOperationEnabled}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </div>

          <Card>
            <CardHeader>
              <CardTitle>{t('Auto-disable rules')}</CardTitle>
              <CardDescription>
                {t(
                  'Rules run after retries finish and use the final usage or error logs in each statistics window'
                )}
              </CardDescription>
              <CardAction>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={!channelAutoOperationEnabled}
                  onClick={addRule}
                >
                  <Plus data-icon='inline-start' />
                  {t('Add rule')}
                </Button>
              </CardAction>
            </CardHeader>
            <CardContent>
              <div className='flex flex-col gap-4'>
                {autoDisableRules.length === 0 && (
                  <div className='text-muted-foreground rounded-lg border border-dashed p-4 text-sm'>
                    {t(
                      'No detailed rules configured. The legacy global thresholds above will be used.'
                    )}
                  </div>
                )}

                {autoDisableRules.map((rule, index) => (
                  <div
                    key={rule.id || index}
                    className='flex flex-col gap-4 rounded-lg border p-4'
                  >
                    <div className='flex flex-col gap-3 md:flex-row md:items-center md:justify-between'>
                      <div className='grid flex-1 gap-3 md:grid-cols-3'>
                        <FormItem>
                          <FormLabel>{t('Rule name')}</FormLabel>
                          <FormControl>
                            <Input
                              value={rule.name}
                              disabled={!channelAutoOperationEnabled}
                              onChange={(event) =>
                                updateRule(index, {
                                  name: event.target.value,
                                })
                              }
                            />
                          </FormControl>
                        </FormItem>
                        <FormItem>
                          <FormLabel>{t('Trigger type')}</FormLabel>
                          <Select
                            value={rule.rule_type}
                            onValueChange={(value) =>
                              updateRuleType(
                                index,
                                value as AutoDisableRuleForm['rule_type']
                              )
                            }
                            disabled={!channelAutoOperationEnabled}
                          >
                            <FormControl>
                              <SelectTrigger className='w-full'>
                                <SelectValue />
                              </SelectTrigger>
                            </FormControl>
                            <SelectContent>
                              <SelectItem value='error'>
                                {t('Error')}
                              </SelectItem>
                              <SelectItem value='timeout'>
                                {t('Timeout')}
                              </SelectItem>
                            </SelectContent>
                          </Select>
                        </FormItem>
                        <FormItem>
                          <FormLabel>{t('Rule ID')}</FormLabel>
                          <FormControl>
                            <Input
                              value={rule.id}
                              disabled={!channelAutoOperationEnabled}
                              onChange={(event) =>
                                updateRule(index, {
                                  id: event.target.value,
                                })
                              }
                            />
                          </FormControl>
                        </FormItem>
                      </div>

                      <div className='flex items-center gap-3'>
                        <FormItem className='flex flex-row items-center gap-2'>
                          <FormLabel className='text-sm'>
                            {t('Enabled')}
                          </FormLabel>
                          <FormControl>
                            <Switch
                              checked={rule.enabled}
                              disabled={!channelAutoOperationEnabled}
                              onCheckedChange={(checked) =>
                                updateRule(index, { enabled: checked })
                              }
                            />
                          </FormControl>
                        </FormItem>
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon'
                          disabled={!channelAutoOperationEnabled}
                          onClick={() => removeRule(index)}
                          title={t('Delete rule')}
                        >
                          <Trash2 />
                        </Button>
                      </div>
                    </div>

                    <div className='grid gap-4 md:grid-cols-3'>
                      <FormItem>
                        <FormLabel>{t('Models')}</FormLabel>
                        <FormControl>
                          <Input
                            placeholder={t('All models')}
                            value={joinCSV(rule.model_names)}
                            disabled={!channelAutoOperationEnabled}
                            onChange={(event) =>
                              updateRule(index, {
                                model_names: splitCSV(event.target.value),
                              })
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Comma-separated model names, supports prefix*')}
                        </FormDescription>
                      </FormItem>

                      <FormItem>
                        <FormLabel>{t('Window (minutes)')}</FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            min={1}
                            step={1}
                            value={rule.window_minutes}
                            disabled={!channelAutoOperationEnabled}
                            onChange={(event) =>
                              updateRule(index, {
                                window_minutes: event.target.valueAsNumber,
                              })
                            }
                          />
                        </FormControl>
                      </FormItem>

                      <FormItem>
                        <FormLabel>{t('Min requests')}</FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            min={0}
                            step={1}
                            value={rule.min_requests}
                            disabled={!channelAutoOperationEnabled}
                            onChange={(event) =>
                              updateRule(index, {
                                min_requests: event.target.valueAsNumber,
                              })
                            }
                          />
                        </FormControl>
                      </FormItem>
                    </div>

                    {rule.rule_type === 'error' ? (
                      <>
                        <div className='grid gap-4 md:grid-cols-2'>
                          <FormItem>
                            <FormLabel>{t('Error types')}</FormLabel>
                            <FormControl>
                              <Input
                                placeholder={t(
                                  'e.g. first_token, invalid_api_key'
                                )}
                                value={joinCSV(rule.error_types)}
                                disabled={!channelAutoOperationEnabled}
                                onChange={(event) =>
                                  updateRule(index, {
                                    error_types: splitCSV(event.target.value),
                                  })
                                }
                              />
                            </FormControl>
                            <FormDescription>
                              {t('Matches error code or error content')}
                            </FormDescription>
                          </FormItem>

                          <FormItem>
                            <FormLabel>{t('Status codes')}</FormLabel>
                            <FormControl>
                              <Input
                                placeholder={t('e.g. 401, 403, 500-599')}
                                value={rule.status_codes}
                                disabled={!channelAutoOperationEnabled}
                                onChange={(event) =>
                                  updateRule(index, {
                                    status_codes: event.target.value,
                                  })
                                }
                              />
                            </FormControl>
                            <FormDescription>
                              {t('Leave empty to ignore status codes')}
                            </FormDescription>
                          </FormItem>
                        </div>

                        <div className='grid gap-4 md:grid-cols-3'>
                          <FormItem>
                            <FormLabel>{t('Errors')}</FormLabel>
                            <FormControl>
                              <Input
                                type='number'
                                min={0}
                                step={1}
                                value={rule.error_count_threshold}
                                disabled={!channelAutoOperationEnabled}
                                onChange={(event) =>
                                  updateRule(index, {
                                    error_count_threshold:
                                      event.target.valueAsNumber,
                                  })
                                }
                              />
                            </FormControl>
                          </FormItem>

                          <FormItem>
                            <FormLabel>{t('Errors/min')}</FormLabel>
                            <FormControl>
                              <Input
                                type='number'
                                min={0}
                                step={1}
                                value={rule.per_minute_error_threshold}
                                disabled={!channelAutoOperationEnabled}
                                onChange={(event) =>
                                  updateRule(index, {
                                    per_minute_error_threshold:
                                      event.target.valueAsNumber,
                                  })
                                }
                              />
                            </FormControl>
                          </FormItem>

                          <FormItem>
                            <FormLabel>{t('Error rate (%)')}</FormLabel>
                            <FormControl>
                              <Input
                                type='number'
                                min={0}
                                max={100}
                                step={1}
                                value={rule.error_rate}
                                disabled={!channelAutoOperationEnabled}
                                onChange={(event) =>
                                  updateRule(index, {
                                    error_rate: event.target.valueAsNumber,
                                  })
                                }
                              />
                            </FormControl>
                          </FormItem>
                        </div>
                      </>
                    ) : (
                      <div className='grid gap-4 md:grid-cols-2'>
                        <FormItem>
                          <FormLabel>
                            {t('First token timeout (seconds)')}
                          </FormLabel>
                          <FormControl>
                            <Input
                              type='number'
                              min={1}
                              step={1}
                              value={rule.first_token_seconds}
                              disabled={!channelAutoOperationEnabled}
                              onChange={(event) =>
                                updateRule(index, {
                                  first_token_seconds:
                                    event.target.valueAsNumber,
                                })
                              }
                            />
                          </FormControl>
                        </FormItem>

                        <FormItem>
                          <FormLabel>{t('Timeout count')}</FormLabel>
                          <FormControl>
                            <Input
                              type='number'
                              min={1}
                              step={1}
                              value={rule.first_token_count_threshold}
                              disabled={!channelAutoOperationEnabled}
                              onChange={(event) =>
                                updateRule(index, {
                                  first_token_count_threshold:
                                    event.target.valueAsNumber,
                                })
                              }
                            />
                          </FormControl>
                        </FormItem>
                      </div>
                    )}

                    <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                      <div className='flex flex-col gap-0.5'>
                        <FormLabel>{t('Protect last channel')}</FormLabel>
                        <FormDescription>
                          {t(
                            'Skip disabling when this is the last available channel for the model'
                          )}
                        </FormDescription>
                      </div>
                      <FormControl>
                        <Switch
                          checked={rule.protect_last}
                          disabled={!channelAutoOperationEnabled}
                          onCheckedChange={(checked) =>
                            updateRule(index, { protect_last: checked })
                          }
                        />
                      </FormControl>
                    </FormItem>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>

          <div className='space-y-4 rounded-lg border p-4'>
            <FormField
              control={form.control}
              name='webhook_setting.enabled'
              render={({ field }) => (
                <FormItem className='flex flex-col gap-3 md:flex-row md:items-center md:justify-between'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Global webhook alerts')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Push model error, channel test, and automatic disable alerts to a system-level webhook'
                      )}
                    </FormDescription>
                  </div>
                  <div className='flex items-center gap-3'>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      disabled={sendWebhookTest.isPending}
                      onClick={handleSendWebhookTest}
                    >
                      <Send className='mr-2 size-4' />
                      {sendWebhookTest.isPending
                        ? t('Sending...')
                        : t('Send test alert')}
                    </Button>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </div>
                </FormItem>
              )}
            />

            <div className='grid gap-6 md:grid-cols-2'>
              <FormField
                control={form.control}
                name='webhook_setting.url'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Webhook URL')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder='https://example.com/webhook'
                        disabled={!globalWebhookEnabled}
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('The system sends JSON alert payloads to this URL')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='webhook_setting.secret'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Webhook secret')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        autoComplete='new-password'
                        disabled={!globalWebhookEnabled}
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Used to sign payloads with X-Webhook-Signature')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <div className='grid gap-6 md:grid-cols-3'>
              <FormField
                control={form.control}
                name='webhook_setting.interval_minutes'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Push interval (minutes)')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        step={1}
                        disabled={!globalWebhookEnabled}
                        value={
                          typeof field.value === 'number' &&
                          Number.isFinite(field.value)
                            ? field.value
                            : ''
                        }
                        onChange={(event) =>
                          field.onChange(event.target.valueAsNumber)
                        }
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('How often the monitor checks alert conditions')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='webhook_setting.suppress_minutes'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Suppress window (minutes)')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        step={1}
                        disabled={!globalWebhookEnabled}
                        value={
                          typeof field.value === 'number' &&
                          Number.isFinite(field.value)
                            ? field.value
                            : ''
                        }
                        onChange={(event) =>
                          field.onChange(event.target.valueAsNumber)
                        }
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Skip duplicate payloads within this window')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='webhook_setting.notify_on_empty_result'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                    <div className='space-y-0.5'>
                      <FormLabel className='text-base'>
                        {t('Push empty checks')}
                      </FormLabel>
                      <FormDescription>
                        {t('Send a heartbeat when no alert event is found')}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        disabled={!globalWebhookEnabled}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </div>

            <div className='grid gap-6 md:grid-cols-2'>
              <FormField
                control={form.control}
                name='webhook_setting.channel_test_check_enabled'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                    <div className='space-y-0.5'>
                      <FormLabel className='text-base'>
                        {t('Channel test alerts')}
                      </FormLabel>
                      <FormDescription>
                        {t('Push recent failed model-channel test results')}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        disabled={!globalWebhookEnabled}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='webhook_setting.channel_test_window_minutes'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Channel test window (minutes)')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        step={1}
                        disabled={
                          !globalWebhookEnabled || !channelTestCheckEnabled
                        }
                        value={
                          typeof field.value === 'number' &&
                          Number.isFinite(field.value)
                            ? field.value
                            : ''
                        }
                        onChange={(event) =>
                          field.onChange(event.target.valueAsNumber)
                        }
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Only failed tests in this recent window are included'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <div className='grid gap-6 md:grid-cols-4'>
              <FormField
                control={form.control}
                name='webhook_setting.model_error_check_enabled'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                    <div className='space-y-0.5'>
                      <FormLabel className='text-base'>
                        {t('Model error alerts')}
                      </FormLabel>
                      <FormDescription>
                        {t('Push channels with repeated model errors')}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        disabled={!globalWebhookEnabled}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='webhook_setting.model_error_window_minutes'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Model error window (minutes)')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        step={1}
                        disabled={
                          !globalWebhookEnabled || !modelErrorCheckEnabled
                        }
                        value={
                          typeof field.value === 'number' &&
                          Number.isFinite(field.value)
                            ? field.value
                            : ''
                        }
                        onChange={(event) =>
                          field.onChange(event.target.valueAsNumber)
                        }
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Count only model errors in this recent window')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='webhook_setting.model_error_threshold'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Webhook model error threshold')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        step={1}
                        disabled={
                          !globalWebhookEnabled || !modelErrorCheckEnabled
                        }
                        value={
                          typeof field.value === 'number' &&
                          Number.isFinite(field.value)
                            ? field.value
                            : ''
                        }
                        onChange={(event) =>
                          field.onChange(event.target.valueAsNumber)
                        }
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Push when one model reaches this many errors')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='webhook_setting.model_error_min_requests'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Webhook minimum requests')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        step={1}
                        disabled={
                          !globalWebhookEnabled || !modelErrorCheckEnabled
                        }
                        value={
                          typeof field.value === 'number' &&
                          Number.isFinite(field.value)
                            ? field.value
                            : ''
                        }
                        onChange={(event) =>
                          field.onChange(event.target.valueAsNumber)
                        }
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Ignore low-volume models below this request count')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='webhook_setting.model_error_rate'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('Webhook error rate threshold (%)')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        max={100}
                        step={1}
                        disabled={
                          !globalWebhookEnabled || !modelErrorCheckEnabled
                        }
                        value={
                          typeof field.value === 'number' &&
                          Number.isFinite(field.value)
                            ? field.value
                            : ''
                        }
                        onChange={(event) =>
                          field.onChange(event.target.valueAsNumber)
                        }
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Push only when the model error rate reaches this percentage'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </div>

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='ChannelDisableThreshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Disable threshold (seconds)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Automatically disable channels exceeding this response time'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='QuotaRemindThreshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Quota reminder (tokens)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Send email alerts when a user falls below this quota')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='AutomaticDisableChannelEnabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Disable on failure')}
                    </FormLabel>
                    <FormDescription>
                      {t('Automatically disable channels when tests fail')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AutomaticEnableChannelEnabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Re-enable on success')}
                    </FormLabel>
                    <FormDescription>
                      {t('Bring channels back online after successful checks')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='AutomaticDisableKeywords'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Failure keywords')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={6}
                    placeholder={t('one keyword per line')}
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'If an upstream error contains any of these keywords (case insensitive), the channel will be disabled automatically.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='AutomaticDisableStatusCodes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Auto-disable status codes')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('e.g. 401, 403, 429, 500-599')}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Accepts comma-separated status codes and inclusive ranges.'
                    )}{' '}
                    {autoDisableParsed.ok &&
                      autoDisableParsed.normalized &&
                      autoDisableParsed.normalized !== field.value.trim() && (
                        <span className='text-muted-foreground'>
                          {t('Normalized:')} {autoDisableParsed.normalized}
                        </span>
                      )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AutomaticRetryStatusCodes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Auto-retry status codes')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('e.g. 401, 403, 429, 500-599')}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Accepts comma-separated status codes and inclusive ranges.'
                    )}{' '}
                    {autoRetryParsed.ok &&
                      autoRetryParsed.normalized &&
                      autoRetryParsed.normalized !== field.value.trim() && (
                        <span className='text-muted-foreground'>
                          {t('Normalized:')} {autoRetryParsed.normalized}
                        </span>
                      )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='AutomaticRetryPolicyRules'
            render={({ field }) => (
              <FormItem>
                <div className='flex items-center justify-between gap-3'>
                  <FormLabel>{t('Global retry policy')}</FormLabel>
                  <Select
                    value=''
                    onValueChange={(value) => {
                      const template = RETRY_POLICY_TEMPLATES.find(
                        (item) => item.labelKey === value
                      )
                      if (template) {
                        field.onChange(
                          insertRetryPolicyTemplate(field.value, template)
                        )
                      }
                    }}
                  >
                    <SelectTrigger className='h-8 w-40'>
                      <SelectValue placeholder={t('Insert template')} />
                    </SelectTrigger>
                    <SelectContent>
                      {RETRY_POLICY_TEMPLATES.map((template) => (
                        <SelectItem
                          key={template.labelKey}
                          value={template.labelKey}
                        >
                          {t(template.labelKey)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <FormControl>
                  <Textarea
                    rows={6}
                    placeholder='[{"models":["gpt-image-2"],"message_contains":["private ip"],"action":"retry"}]'
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Global retry policy is used when the channel retry policy does not match; status code rules remain the fallback.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <Button type='submit' disabled={updateOption.isPending}>
            {updateOption.isPending
              ? t('Saving...')
              : t('Save monitoring rules')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}
