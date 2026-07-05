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
import { useEffect, useState, useCallback, useMemo } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
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
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { MultiSelect } from '@/components/multi-select'
import { TagInput } from '@/components/tag-input'
import {
  MODEL_CATEGORIES,
  MODEL_CATEGORY_VALUES,
  getModelCategoryLabels,
} from '@/features/pricing/constants'
import { formatDerivedCacheCreate1hValue } from '@/features/pricing/lib/price'
import type {
  ModelCapability,
  ModelCategory,
  Modality,
} from '@/features/pricing/types'
import {
  useSystemOptions,
  getOptionValue,
} from '@/features/system-settings/hooks/use-system-options'
import { useUpdateOption } from '@/features/system-settings/hooks/use-update-option'
import { normalizeJsonString } from '@/features/system-settings/models/utils'
import type { ModelSettings } from '@/features/system-settings/types'
import { safeJsonParse } from '@/features/system-settings/utils/json-parser'
import { createModel, updateModel, getModel, getVendors } from '../../api'
import { getNameRuleOptions } from '../../constants'
import { modelsQueryKeys, vendorsQueryKeys, parseModelTags } from '../../lib'
import {
  convertRatioValuesToPriceValues,
  normalizeModelPricingValuesForInputMode,
  syncModelPricingMaps,
} from '../../lib/model-pricing-sync'
import type { Model } from '../../types'
import { EndpointConfigEditor } from '../endpoint-config-editor'

// Extended schema for ratio configuration (internal form state only)
const extendedModelFormSchema = z.object({
  id: z.number().optional(),
  model_name: z.string().min(1, 'Model name is required'),
  description: z.string(),
  icon: z.string(),
  tags: z.array(z.string()),
  category: z.enum(MODEL_CATEGORY_VALUES),
  vendor_id: z.number().optional(),
  alias_models: z.string(),
  endpoints: z.string(),
  sort_order: z.number().int().min(0),
  context_length: z.number().int().min(0),
  max_output_tokens: z.number().int().min(0),
  knowledge_cutoff: z.string(),
  release_date: z.string(),
  parameter_count: z.string(),
  input_modalities: z.array(z.string()),
  output_modalities: z.array(z.string()),
  capabilities: z.array(z.string()),
  name_rule: z.number(),
  status: z.boolean(),
  sync_official: z.boolean(),
  price: z.string().optional(),
  ratio: z.string().optional(),
  cacheRatio: z.string().optional(),
  createCacheRatio: z.string().optional(),
  completionRatio: z.string().optional(),
  imageRatio: z.string().optional(),
  audioRatio: z.string().optional(),
  audioCompletionRatio: z.string().optional(),
  billingExpr: z.string().optional(),
})

type ExtendedModelFormValues = z.infer<typeof extendedModelFormSchema>

type PricingMode = 'per-token' | 'per-request' | 'tiered_expr'
type PricingSubMode = 'ratio' | 'price'

const MODALITY_VALUES: Modality[] = ['text', 'image', 'audio', 'video', 'file']
const MODALITY_LABEL_KEYS: Record<Modality, string> = {
  text: 'Text',
  image: 'Image',
  audio: 'Audio',
  video: 'Video',
  file: 'File',
}
const CAPABILITY_VALUES: ModelCapability[] = [
  'streaming',
  'function_calling',
  'tools',
  'json_mode',
  'structured_output',
  'vision',
  'reasoning',
  'caching',
  'system_prompt',
  'web_search',
  'code_interpreter',
  'embeddings',
]
const CAPABILITY_LABEL_KEYS: Record<ModelCapability, string> = {
  function_calling: 'Function calling',
  streaming: 'Streaming',
  vision: 'Vision',
  json_mode: 'JSON mode',
  structured_output: 'Structured output',
  reasoning: 'Reasoning',
  tools: 'Tools',
  system_prompt: 'System prompt',
  web_search: 'Web search',
  code_interpreter: 'Code interpreter',
  caching: 'Prompt caching',
  embeddings: 'Embeddings',
}

function parseListField(value?: string): string[] {
  if (!value) return []
  return value
    .split(/[,;|\n\r]+/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function formatListField(values: string[]): string {
  return values
    .map((item) => item.trim())
    .filter(Boolean)
    .join(',')
}

const pricingFields = [
  'ratio',
  'cacheRatio',
  'createCacheRatio',
  'completionRatio',
  'imageRatio',
  'audioRatio',
  'audioCompletionRatio',
] as const

type ModelMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: Model | null
}

export function ModelMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: ModelMutateDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isEditing = Boolean(currentRow?.id)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [pricingMode, setPricingMode] = useState<PricingMode>('per-token')
  const [pricingSubMode, setPricingSubMode] = useState<PricingSubMode>('ratio')
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [promptPrice, setPromptPrice] = useState('')
  const [completionPrice, setCompletionPrice] = useState('')
  const [oldModelName, setOldModelName] = useState<string>('')

  // Fetch vendors for dropdown
  const { data: vendorsData } = useQuery({
    queryKey: vendorsQueryKeys.list(),
    queryFn: () => getVendors({ page_size: 1000 }),
    enabled: open,
  })

  const vendors = vendorsData?.data?.items || []
  const categoryLabels = getModelCategoryLabels(t)
  const modalityOptions = useMemo(
    () =>
      MODALITY_VALUES.map((value) => ({
        value,
        label: t(MODALITY_LABEL_KEYS[value]),
      })),
    [t]
  )
  const capabilityOptions = useMemo(
    () =>
      CAPABILITY_VALUES.map((value) => ({
        value,
        label: t(CAPABILITY_LABEL_KEYS[value]),
      })),
    [t]
  )

  // Fetch model detail if editing
  const { data: modelData } = useQuery({
    queryKey: modelsQueryKeys.detail(currentRow?.id || 0),
    queryFn: () => getModel(currentRow!.id),
    enabled: open && isEditing,
  })

  // Fetch system options for ratio configuration
  const { data: systemOptionsData } = useSystemOptions()

  const updateOption = useUpdateOption()

  // Get model settings from system options
  const modelSettings = useMemo(() => {
    if (!systemOptionsData?.data) return null
    const defaultModelSettings: ModelSettings = {
      'global.pass_through_request_enabled': false,
      'global.thinking_model_blacklist': '[]',
      'global.chat_completions_to_responses_policy': '{}',
      'general_setting.ping_interval_enabled': false,
      'general_setting.ping_interval_seconds': 60,
      'gemini.safety_settings': '',
      'gemini.version_settings': '',
      'gemini.supported_imagine_models': '',
      'gemini.thinking_adapter_enabled': false,
      'gemini.thinking_adapter_budget_tokens_percentage': 0.6,
      'gemini.function_call_thought_signature_enabled': false,
      'gemini.remove_function_response_id_enabled': true,
      'claude.model_headers_settings': '',
      'claude.default_max_tokens': '',
      'claude.thinking_adapter_enabled': true,
      'claude.thinking_adapter_budget_tokens_percentage': 0.8,
      ModelPrice: '',
      ModelRatio: '',
      CacheRatio: '',
      CompletionRatio: '',
      ImageRatio: '',
      AudioRatio: '',
      AudioCompletionRatio: '',
      ExposeRatioEnabled: false,
      'billing_setting.billing_mode': '{}',
      'billing_setting.billing_expr': '{}',
      'tool_price_setting.prices': '{}',
      TopupGroupRatio: '',
      GroupRatio: '',
      UserUsableGroups: '',
      GroupGroupRatio: '',
      AutoGroups: '',
      GroupDisplayConfig: '{"categories":[],"groups":[]}',
      DefaultUseAutoGroup: false,
      CreateCacheRatio: '',
      'group_ratio_setting.group_special_usable_group': '{}',
      'grok.violation_deduction_enabled': false,
      'grok.violation_deduction_amount': 0,
      'channel_affinity_setting.enabled': false,
      'channel_affinity_setting.switch_on_success': true,
      'channel_affinity_setting.max_entries': 100000,
      'channel_affinity_setting.default_ttl_seconds': 3600,
      'channel_affinity_setting.rules': '[]',
      'model_fallback.enabled': false,
      'model_fallback.allow_user_override': true,
      'model_fallback.failure_status_codes':
        '100-199,300-399,401-407,409-499,500-503,505-523,525-599',
      'model_fallback.rules': '[]',
      'model_deployment.ionet.api_key': '',
      'model_deployment.ionet.enabled': false,
      AutomaticRetryPolicyRules: '[]',
      'routing_strategy_setting.enabled': true,
      'routing_strategy_setting.update_interval_minutes': 10,
      'routing_strategy_setting.window_hours': 24,
      'routing_strategy_setting.min_request_count': 3,
      'routing_strategy_setting.smart_price_weight': 0.4,
      'routing_strategy_setting.smart_speed_weight': 0.25,
      'routing_strategy_setting.smart_success_weight': 0.35,
      'routing_strategy_setting.excluded_groups': '',
      'routing_strategy_setting.pinned_groups': '',
    }
    return getOptionValue(systemOptionsData.data, defaultModelSettings)
  }, [systemOptionsData])

  const form = useForm<ExtendedModelFormValues>({
    resolver: zodResolver(extendedModelFormSchema),
    defaultValues: {
      model_name: '',
      description: '',
      icon: '',
      tags: [],
      category: MODEL_CATEGORIES.TEXT,
      vendor_id: undefined,
      alias_models: '',
      endpoints: '',
      sort_order: 100,
      context_length: 0,
      max_output_tokens: 0,
      knowledge_cutoff: '',
      release_date: '',
      parameter_count: '',
      input_modalities: [],
      output_modalities: [],
      capabilities: [],
      name_rule: 0,
      status: true,
      sync_official: true,
      price: '',
      ratio: '',
      cacheRatio: '',
      createCacheRatio: '',
      completionRatio: '',
      imageRatio: '',
      audioRatio: '',
      audioCompletionRatio: '',
      billingExpr: '',
    },
  })

  const validateNumber = (value: string) => {
    if (value === '') return true
    return !isNaN(parseFloat(value))
  }

  const handlePromptPriceChange = (value: string) => {
    setPromptPrice(value)
    if (value && !isNaN(parseFloat(value))) {
      const ratio = parseFloat(value) / 2
      form.setValue('ratio', ratio.toString(), { shouldDirty: true })
    } else {
      form.setValue('ratio', '', { shouldDirty: true })
    }
  }

  const handleCompletionPriceChange = (value: string) => {
    setCompletionPrice(value)
    if (
      value &&
      !isNaN(parseFloat(value)) &&
      promptPrice &&
      !isNaN(parseFloat(promptPrice)) &&
      parseFloat(promptPrice) > 0
    ) {
      const completionRatio = parseFloat(value) / parseFloat(promptPrice)
      form.setValue('completionRatio', completionRatio.toString(), {
        shouldDirty: true,
      })
    } else {
      form.setValue('completionRatio', '', { shouldDirty: true })
    }
  }

  const handlePricingSubModeChange = (value: string) => {
    const nextMode = value as PricingSubMode
    if (nextMode === pricingSubMode) return

    const currentValues = form.getValues()
    const convertedValues =
      nextMode === 'price'
        ? convertRatioValuesToPriceValues(currentValues)
        : normalizeModelPricingValuesForInputMode(currentValues, 'price')

    pricingFields.forEach((field) => {
      form.setValue(field, convertedValues[field] || '', {
        shouldDirty: true,
        shouldValidate: true,
      })
    })
    setPricingSubMode(nextMode)
  }

  const getAdvancedFieldDescription = (
    ratioDescriptionKey: string,
    priceDescriptionKey: string
  ) => {
    return pricingSubMode === 'price'
      ? t(priceDescriptionKey)
      : t(ratioDescriptionKey)
  }

  const getCacheCreateFieldDescription = (value?: string) => {
    const derived1hValue = formatDerivedCacheCreate1hValue(value)

    if (pricingSubMode === 'price') {
      return derived1hValue
        ? t(
            'Configured cache write price is the 5m price. 1h cache writes are automatically settled at 1.6x: ${{value}} per 1M tokens.',
            { value: derived1hValue }
          )
        : t(
            'Configured cache write price is the 5m price. 1h cache writes are automatically settled at 1.6x.'
          )
    }

    return derived1hValue
      ? t(
          'Configured create cache ratio is the 5m ratio. 1h cache writes are automatically settled at 1.6x: {{value}}.',
          { value: derived1hValue }
        )
      : t(
          'Configured create cache ratio is the 5m ratio. 1h cache writes are automatically settled at 1.6x.'
        )
  }

  // Load model data for editing and ratio configuration
  useEffect(() => {
    if (open && isEditing && modelData?.data) {
      const model = modelData.data
      setOldModelName(model.model_name)
      setPricingSubMode('ratio')
      setPromptPrice('')
      setCompletionPrice('')

      // Base model data reset
      const baseModelData = {
        id: model.id,
        model_name: model.model_name,
        description: model.description || '',
        icon: model.icon || '',
        tags: parseModelTags(model.tags),
        category: model.category || MODEL_CATEGORIES.TEXT,
        vendor_id: model.vendor_id,
        alias_models: model.alias_models || '',
        endpoints: model.endpoints || '',
        sort_order: model.sort_order || 100,
        context_length: model.context_length || 0,
        max_output_tokens: model.max_output_tokens || 0,
        knowledge_cutoff: model.knowledge_cutoff || '',
        release_date: model.release_date || '',
        parameter_count: model.parameter_count || '',
        input_modalities: parseListField(model.input_modalities),
        output_modalities: parseListField(model.output_modalities),
        capabilities: parseListField(model.capabilities),
        name_rule: model.name_rule || 0,
        status: model.status === 1,
        sync_official: model.sync_official === 1,
        price: '',
        ratio: '',
        cacheRatio: '',
        createCacheRatio: '',
        completionRatio: '',
        imageRatio: '',
        audioRatio: '',
        audioCompletionRatio: '',
        billingExpr: '',
      }

      // Parse ratio configurations from system settings if available
      if (modelSettings) {
        const priceMap = safeJsonParse<Record<string, number>>(
          modelSettings.ModelPrice,
          { fallback: {}, silent: true }
        )
        const ratioMap = safeJsonParse<Record<string, number>>(
          modelSettings.ModelRatio,
          { fallback: {}, silent: true }
        )
        const cacheMap = safeJsonParse<Record<string, number>>(
          modelSettings.CacheRatio,
          { fallback: {}, silent: true }
        )
        const createCacheMap = safeJsonParse<Record<string, number>>(
          modelSettings.CreateCacheRatio,
          { fallback: {}, silent: true }
        )
        const completionMap = safeJsonParse<Record<string, number>>(
          modelSettings.CompletionRatio,
          { fallback: {}, silent: true }
        )
        const imageMap = safeJsonParse<Record<string, number>>(
          modelSettings.ImageRatio,
          { fallback: {}, silent: true }
        )
        const audioMap = safeJsonParse<Record<string, number>>(
          modelSettings.AudioRatio,
          { fallback: {}, silent: true }
        )
        const audioCompletionMap = safeJsonParse<Record<string, number>>(
          modelSettings.AudioCompletionRatio,
          { fallback: {}, silent: true }
        )
        const billingModeMap = safeJsonParse<Record<string, string>>(
          modelSettings['billing_setting.billing_mode'],
          { fallback: {}, silent: true }
        )
        const billingExprMap = safeJsonParse<Record<string, string>>(
          modelSettings['billing_setting.billing_expr'],
          { fallback: {}, silent: true }
        )

        // Extract ratio config for this model
        const modelName = model.model_name
        const price = priceMap[modelName]
        const ratio = ratioMap[modelName]
        const cacheRatio = cacheMap[modelName]
        const createCacheRatio = createCacheMap[modelName]
        const completionRatio = completionMap[modelName]
        const imageRatio = imageMap[modelName]
        const audioRatio = audioMap[modelName]
        const audioCompletionRatio = audioCompletionMap[modelName]
        const billingMode = billingModeMap[modelName]

        // Determine pricing mode
        if (billingMode === 'tiered_expr') {
          setPricingMode('tiered_expr')
          setAdvancedOpen(false)
          if (ratio !== undefined && ratio !== null) {
            const tokenPrice = ratio * 2
            setPromptPrice(tokenPrice.toString())
            if (completionRatio !== undefined && completionRatio !== null) {
              const compPrice = tokenPrice * completionRatio
              setCompletionPrice(compPrice.toString())
            }
          }
          form.reset({
            ...baseModelData,
            price: price?.toString() || '',
            ratio: ratio?.toString() || '',
            cacheRatio: cacheRatio?.toString() || '',
            createCacheRatio: createCacheRatio?.toString() || '',
            completionRatio: completionRatio?.toString() || '',
            imageRatio: imageRatio?.toString() || '',
            audioRatio: audioRatio?.toString() || '',
            audioCompletionRatio: audioCompletionRatio?.toString() || '',
            billingExpr: billingExprMap[modelName] || '',
          })
        } else if (price !== undefined && price !== null) {
          setPricingMode('per-request')
          setAdvancedOpen(false)
          form.reset({
            ...baseModelData,
            price: price.toString(),
            billingExpr: billingExprMap[modelName] || '',
          })
        } else {
          setPricingMode('per-token')
          if (ratio !== undefined && ratio !== null) {
            const tokenPrice = ratio * 2
            setPromptPrice(tokenPrice.toString())
            if (completionRatio !== undefined && completionRatio !== null) {
              const compPrice = tokenPrice * completionRatio
              setCompletionPrice(compPrice.toString())
            }
          }
          form.reset({
            ...baseModelData,
            ratio: ratio?.toString() || '',
            cacheRatio: cacheRatio?.toString() || '',
            createCacheRatio: createCacheRatio?.toString() || '',
            completionRatio: completionRatio?.toString() || '',
            imageRatio: imageRatio?.toString() || '',
            audioRatio: audioRatio?.toString() || '',
            audioCompletionRatio: audioCompletionRatio?.toString() || '',
            billingExpr: billingExprMap[modelName] || '',
          })
          setAdvancedOpen(
            !!(
              cacheRatio ||
              createCacheRatio ||
              imageRatio ||
              audioRatio ||
              audioCompletionRatio
            )
          )
        }
      } else {
        // If system settings not loaded yet, just load base model data
        setPricingMode('per-token')
        form.reset(baseModelData)
        setAdvancedOpen(false)
      }
    } else if (open && !isEditing) {
      // Pre-fill model name if passed from missing models
      setOldModelName('')
      setPricingMode('per-token')
      setPricingSubMode('ratio')
      setPromptPrice('')
      setCompletionPrice('')
      setAdvancedOpen(false)
      form.reset({
        model_name: currentRow?.model_name || '',
        description: '',
        icon: '',
        tags: [],
        category: MODEL_CATEGORIES.TEXT,
        vendor_id: undefined,
        alias_models: '',
        endpoints: '',
        sort_order: 100,
        context_length: 0,
        max_output_tokens: 0,
        knowledge_cutoff: '',
        release_date: '',
        parameter_count: '',
        input_modalities: [],
        output_modalities: [],
        capabilities: [],
        name_rule: 0,
        status: true,
        sync_official: true,
        price: '',
        ratio: '',
        cacheRatio: '',
        createCacheRatio: '',
        completionRatio: '',
        imageRatio: '',
        audioRatio: '',
        audioCompletionRatio: '',
        billingExpr: '',
      })
    }
  }, [open, isEditing, modelData, currentRow, form, modelSettings])

  const onSubmit = useCallback(
    async (values: ExtendedModelFormValues): Promise<void> => {
      setIsSubmitting(true)
      try {
        const submitData = {
          ...values,
          id: isEditing ? currentRow!.id : undefined,
          tags: Array.isArray(values.tags) ? values.tags.join(',') : '',
          input_modalities: formatListField(values.input_modalities),
          output_modalities: formatListField(values.output_modalities),
          capabilities: formatListField(values.capabilities),
          status: values.status ? 1 : 0,
          sync_official: values.sync_official ? 1 : 0,
        }

        // Remove ratio fields from model data (they're stored in system settings)
        const {
          price,
          ratio,
          cacheRatio,
          createCacheRatio,
          completionRatio,
          imageRatio,
          audioRatio,
          audioCompletionRatio,
          billingExpr,
          ...modelData
        } = submitData
        const payload = {
          ...modelData,
          category: values.category as ModelCategory,
        }

        const response = isEditing
          ? await updateModel({ ...payload, id: currentRow!.id })
          : await createModel(payload)

        if (response.success) {
          const finalModelName = values.model_name

          // Always process system settings updates if we have modelSettings
          // This keeps explicit pricing edits and model-name changes synchronized.
          if (modelSettings) {
            // Read existing configurations
            const priceMap = safeJsonParse<Record<string, number>>(
              modelSettings.ModelPrice,
              { fallback: {}, silent: true }
            )
            const ratioMap = safeJsonParse<Record<string, number>>(
              modelSettings.ModelRatio,
              { fallback: {}, silent: true }
            )
            const cacheMap = safeJsonParse<Record<string, number>>(
              modelSettings.CacheRatio,
              { fallback: {}, silent: true }
            )
            const createCacheMap = safeJsonParse<Record<string, number>>(
              modelSettings.CreateCacheRatio,
              { fallback: {}, silent: true }
            )
            const completionMap = safeJsonParse<Record<string, number>>(
              modelSettings.CompletionRatio,
              { fallback: {}, silent: true }
            )
            const imageMap = safeJsonParse<Record<string, number>>(
              modelSettings.ImageRatio,
              { fallback: {}, silent: true }
            )
            const audioMap = safeJsonParse<Record<string, number>>(
              modelSettings.AudioRatio,
              { fallback: {}, silent: true }
            )
            const audioCompletionMap = safeJsonParse<Record<string, number>>(
              modelSettings.AudioCompletionRatio,
              { fallback: {}, silent: true }
            )
            const billingModeMap = safeJsonParse<Record<string, string>>(
              modelSettings['billing_setting.billing_mode'],
              { fallback: {}, silent: true }
            )
            const billingExprMap = safeJsonParse<Record<string, string>>(
              modelSettings['billing_setting.billing_expr'],
              { fallback: {}, silent: true }
            )

            const syncedPricingMaps = syncModelPricingMaps({
              maps: {
                priceMap,
                ratioMap,
                cacheMap,
                createCacheMap,
                completionMap,
                imageMap,
                audioMap,
                audioCompletionMap,
                billingModeMap,
                billingExprMap,
              },
              values: {
                price,
                ratio,
                cacheRatio,
                createCacheRatio,
                completionRatio,
                imageRatio,
                audioRatio,
                audioCompletionRatio,
                billingExpr,
              },
              pricingMode,
              pricingInputMode: pricingSubMode,
              finalModelName,
              oldModelName,
              isEditing,
            })

            // Update system options if there are changes
            const updates: Array<{ key: string; value: string }> = []

            const newModelPrice = normalizeJsonString(
              JSON.stringify(syncedPricingMaps.priceMap)
            )
            if (
              newModelPrice !== normalizeJsonString(modelSettings.ModelPrice)
            ) {
              updates.push({ key: 'ModelPrice', value: newModelPrice })
            }

            const newModelRatio = normalizeJsonString(
              JSON.stringify(syncedPricingMaps.ratioMap)
            )
            if (
              newModelRatio !== normalizeJsonString(modelSettings.ModelRatio)
            ) {
              updates.push({ key: 'ModelRatio', value: newModelRatio })
            }

            const newCacheRatio = normalizeJsonString(
              JSON.stringify(syncedPricingMaps.cacheMap)
            )
            if (
              newCacheRatio !== normalizeJsonString(modelSettings.CacheRatio)
            ) {
              updates.push({ key: 'CacheRatio', value: newCacheRatio })
            }

            const newCreateCacheRatio = normalizeJsonString(
              JSON.stringify(syncedPricingMaps.createCacheMap)
            )
            if (
              newCreateCacheRatio !==
              normalizeJsonString(modelSettings.CreateCacheRatio)
            ) {
              updates.push({
                key: 'CreateCacheRatio',
                value: newCreateCacheRatio,
              })
            }

            const newCompletionRatio = normalizeJsonString(
              JSON.stringify(syncedPricingMaps.completionMap)
            )
            if (
              newCompletionRatio !==
              normalizeJsonString(modelSettings.CompletionRatio)
            ) {
              updates.push({
                key: 'CompletionRatio',
                value: newCompletionRatio,
              })
            }

            const newImageRatio = normalizeJsonString(
              JSON.stringify(syncedPricingMaps.imageMap)
            )
            if (
              newImageRatio !== normalizeJsonString(modelSettings.ImageRatio)
            ) {
              updates.push({ key: 'ImageRatio', value: newImageRatio })
            }

            const newAudioRatio = normalizeJsonString(
              JSON.stringify(syncedPricingMaps.audioMap)
            )
            if (
              newAudioRatio !== normalizeJsonString(modelSettings.AudioRatio)
            ) {
              updates.push({ key: 'AudioRatio', value: newAudioRatio })
            }

            const newAudioCompletionRatio = normalizeJsonString(
              JSON.stringify(syncedPricingMaps.audioCompletionMap)
            )
            if (
              newAudioCompletionRatio !==
              normalizeJsonString(modelSettings.AudioCompletionRatio)
            ) {
              updates.push({
                key: 'AudioCompletionRatio',
                value: newAudioCompletionRatio,
              })
            }

            const newBillingMode = normalizeJsonString(
              JSON.stringify(syncedPricingMaps.billingModeMap)
            )
            if (
              newBillingMode !==
              normalizeJsonString(modelSettings['billing_setting.billing_mode'])
            ) {
              updates.push({
                key: 'billing_setting.billing_mode',
                value: newBillingMode,
              })
            }

            const newBillingExpr = normalizeJsonString(
              JSON.stringify(syncedPricingMaps.billingExprMap)
            )
            if (
              newBillingExpr !==
              normalizeJsonString(modelSettings['billing_setting.billing_expr'])
            ) {
              updates.push({
                key: 'billing_setting.billing_expr',
                value: newBillingExpr,
              })
            }

            // Apply all updates (including deletions when clearing fields)
            for (const update of updates) {
              await updateOption.mutateAsync(update)
            }
          }

          toast.success(
            isEditing
              ? 'Model updated successfully'
              : 'Model created successfully'
          )
          queryClient.invalidateQueries({ queryKey: modelsQueryKeys.lists() })
          queryClient.invalidateQueries({ queryKey: ['pricing'] })
          queryClient.invalidateQueries({ queryKey: ['system-options'] })
          onOpenChange(false)
        } else {
          toast.error(response.message || 'Operation failed')
        }
      } catch (error: unknown) {
        toast.error((error as Error)?.message || 'Operation failed')
      } finally {
        setIsSubmitting(false)
      }
    },
    [
      isEditing,
      currentRow,
      queryClient,
      onOpenChange,
      pricingMode,
      pricingSubMode,
      oldModelName,
      modelSettings,
      updateOption,
    ]
  )

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className='flex h-dvh w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl'>
        <SheetHeader className='border-b px-4 py-3 text-start sm:px-6 sm:py-4'>
          <SheetTitle>
            {isEditing ? t('Edit Model') : t('Create Model')}
          </SheetTitle>
          <SheetDescription>
            {isEditing
              ? t("Update model configuration and click save when you're done.")
              : t(
                  'Add a new model to the system by providing the necessary information.'
                )}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            id='model-form'
            onSubmit={form.handleSubmit(
              onSubmit as Parameters<typeof form.handleSubmit>[0]
            )}
            className='flex-1 space-y-4 overflow-y-auto px-3 py-3 pb-4 sm:space-y-6 sm:px-4'
          >
            {/* Basic Information */}
            <div className='space-y-4'>
              <h3 className='text-sm font-semibold'>
                {t('Basic Information')}
              </h3>

              <FormField
                control={form.control}
                name='model_name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Model Name *')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('gpt-4, claude-3-opus, etc.')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('The unique identifier for this model')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='alias_models'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Alias models')}</FormLabel>
                    <FormControl>
                      <Textarea
                        placeholder={t('One model alias per line')}
                        rows={3}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Model square will merge these model names into the main model and show them as aliases.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='description'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Description')}</FormLabel>
                    <FormControl>
                      <Textarea
                        placeholder={t('Describe this model...')}
                        rows={3}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='icon'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Icon')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('OpenAI, Anthropic, etc.')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription className='text-xs'>
                      {t('@lobehub/icons key')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='vendor_id'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Vendor')}</FormLabel>
                    <Select
                      items={[
                        ...vendors.map((vendor) => ({
                          value: String(vendor.id),
                          label: vendor.name,
                        })),
                      ]}
                      onValueChange={(value) =>
                        field.onChange(value ? parseInt(value) : undefined)
                      }
                      value={field.value ? String(field.value) : undefined}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder={t('Select vendor')} />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          {vendors.map((vendor) => (
                            <SelectItem
                              key={vendor.id}
                              value={String(vendor.id)}
                            >
                              {vendor.name}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='tags'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Tags')}</FormLabel>
                    <FormControl>
                      <TagInput
                        value={field.value || []}
                        onChange={field.onChange}
                        placeholder={t('Add tags...')}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Press Enter or comma to add tags')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='category'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Model Category')}</FormLabel>
                    <Select
                      items={MODEL_CATEGORY_VALUES.map((category) => ({
                        value: category,
                        label: categoryLabels[category],
                      }))}
                      onValueChange={field.onChange}
                      value={field.value || MODEL_CATEGORIES.TEXT}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder={t('Select category')} />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          {MODEL_CATEGORY_VALUES.map((category) => (
                            <SelectItem key={category} value={category}>
                              {categoryLabels[category]}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormDescription>
                      {t('Used for model square category filtering.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='sort_order'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Model sort order')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        step={1}
                        value={field.value ?? 0}
                        onChange={(event) =>
                          field.onChange(Number(event.target.value) || 0)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Lower values appear earlier in model management and the model square.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <Separator />

            {/* Model Square Display */}
            <div className='space-y-4'>
              <h3 className='text-sm font-semibold'>
                {t('Model square display')}
              </h3>

              <div className='grid gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='context_length'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Context length')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={0}
                          step={1}
                          value={field.value ?? 0}
                          onChange={(event) =>
                            field.onChange(Number(event.target.value) || 0)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Shown in the model square details. Leave 0 to use inferred data.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='max_output_tokens'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Max output tokens')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={0}
                          step={1}
                          value={field.value ?? 0}
                          onChange={(event) =>
                            field.onChange(Number(event.target.value) || 0)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Shown in the model square details. Leave 0 to use inferred data.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className='grid gap-4 sm:grid-cols-3'>
                <FormField
                  control={form.control}
                  name='knowledge_cutoff'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Knowledge cutoff')}</FormLabel>
                      <FormControl>
                        <Input placeholder='2024-10' {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='release_date'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Release date')}</FormLabel>
                      <FormControl>
                        <Input placeholder='2025-02-15' {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='parameter_count'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Parameter count')}</FormLabel>
                      <FormControl>
                        <Input placeholder='70B' {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name='input_modalities'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Input modalities')}</FormLabel>
                    <FormControl>
                      <MultiSelect
                        options={modalityOptions}
                        selected={field.value || []}
                        onChange={field.onChange}
                        placeholder={t('Select input modalities...')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='output_modalities'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Output modalities')}</FormLabel>
                    <FormControl>
                      <MultiSelect
                        options={modalityOptions}
                        selected={field.value || []}
                        onChange={field.onChange}
                        placeholder={t('Select output modalities...')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='capabilities'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Capabilities')}</FormLabel>
                    <FormControl>
                      <MultiSelect
                        options={capabilityOptions}
                        selected={field.value || []}
                        onChange={field.onChange}
                        placeholder={t('Select capabilities...')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <Separator />

            {/* Matching Configuration */}
            <div className='space-y-4'>
              <h3 className='text-sm font-semibold'>{t('Matching Rules')}</h3>

              <FormField
                control={form.control}
                name='name_rule'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Name Rule')}</FormLabel>
                    <FormControl>
                      <RadioGroup
                        onValueChange={(value) =>
                          field.onChange(parseInt(value))
                        }
                        value={String(field.value)}
                        className='grid grid-cols-2 gap-4'
                      >
                        {getNameRuleOptions(t).map((option) => (
                          <div
                            key={option.value}
                            className='flex items-center space-x-2'
                          >
                            <RadioGroupItem
                              value={String(option.value)}
                              id={`rule-${option.value}`}
                            />
                            <Label
                              htmlFor={`rule-${option.value}`}
                              className='cursor-pointer font-normal'
                            >
                              {option.label}
                            </Label>
                          </div>
                        ))}
                      </RadioGroup>
                    </FormControl>
                    <FormDescription>
                      {t('How this model name should match requests')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <Separator />

            {/* Endpoints Configuration */}
            <div className='space-y-4'>
              <h3 className='text-sm font-semibold'>{t('Supported APIs')}</h3>

              <FormField
                control={form.control}
                name='endpoints'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Endpoint Configuration')}</FormLabel>
                    <FormControl>
                      <EndpointConfigEditor
                        value={field.value || ''}
                        onChange={field.onChange}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Select built-in API modes or add custom modes. Docs URL is shown on the public model detail page.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <Separator />

            {/* Pricing Configuration */}
            <div className='space-y-4'>
              <h3 className='text-sm font-semibold'>
                {t('Pricing Configuration')}
              </h3>

              <div className='space-y-4'>
                <Label>{t('Pricing mode')}</Label>
                <RadioGroup
                  value={pricingMode}
                  onValueChange={(value) =>
                    setPricingMode(value as PricingMode)
                  }
                >
                  <div className='flex items-center space-x-2'>
                    <RadioGroupItem value='per-token' id='per-token' />
                    <Label htmlFor='per-token' className='font-normal'>
                      {t('Per-token (ratio based)')}
                    </Label>
                  </div>
                  <div className='flex items-center space-x-2'>
                    <RadioGroupItem value='per-request' id='per-request' />
                    <Label htmlFor='per-request' className='font-normal'>
                      {t('Per-request (fixed price)')}
                    </Label>
                  </div>
                  <div className='flex items-center space-x-2'>
                    <RadioGroupItem value='tiered_expr' id='tiered-expr' />
                    <Label htmlFor='tiered-expr' className='font-normal'>
                      {t('Expression pricing')}
                    </Label>
                  </div>
                </RadioGroup>
              </div>

              {pricingMode === 'per-request' ? (
                <FormField
                  control={form.control}
                  name='price'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Fixed price (USD)')}</FormLabel>
                      <FormControl>
                        <Input
                          type='text'
                          placeholder='0.01'
                          {...field}
                          onChange={(e) => {
                            const value = e.target.value
                            if (validateNumber(value)) {
                              field.onChange(value)
                            }
                          }}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Cost in USD per request, regardless of tokens used.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              ) : pricingMode === 'tiered_expr' ? (
                <>
                  <FormField
                    control={form.control}
                    name='billingExpr'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Billing expression')}</FormLabel>
                        <FormControl>
                          <Textarea
                            placeholder='tier("base", p * 0 + c * 0)'
                            rows={4}
                            {...field}
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Expression pricing is stored with the same billing fields used by model pricing.'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='ratio'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Fallback model ratio')}</FormLabel>
                        <FormControl>
                          <Input
                            type='text'
                            placeholder='1.0'
                            {...field}
                            onChange={(e) => {
                              const value = e.target.value
                              if (validateNumber(value)) {
                                field.onChange(value)
                              }
                            }}
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Optional fallback used while expression pricing is syncing.'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </>
              ) : (
                <>
                  <div className='space-y-4'>
                    <Label>{t('Input mode')}</Label>
                    <RadioGroup
                      value={pricingSubMode}
                      onValueChange={handlePricingSubModeChange}
                    >
                      <div className='flex items-center space-x-2'>
                        <RadioGroupItem value='ratio' id='ratio' />
                        <Label htmlFor='ratio' className='font-normal'>
                          {t('Ratio mode')}
                        </Label>
                      </div>
                      <div className='flex items-center space-x-2'>
                        <RadioGroupItem value='price' id='price' />
                        <Label htmlFor='price' className='font-normal'>
                          {t('Price mode (USD per 1M tokens)')}
                        </Label>
                      </div>
                    </RadioGroup>
                  </div>

                  {pricingSubMode === 'ratio' ? (
                    <>
                      <FormField
                        control={form.control}
                        name='ratio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Model ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                    if (value) {
                                      setPromptPrice(
                                        (parseFloat(value) * 2).toString()
                                      )
                                    } else {
                                      setPromptPrice('')
                                    }
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {field.value && !isNaN(parseFloat(field.value))
                                ? `Calculated price: $${(parseFloat(field.value) * 2).toFixed(4)} per 1M tokens`
                                : t('Multiplier for prompt tokens.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='completionRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Completion ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                    const ratio = form.getValues('ratio')
                                    if (value && ratio) {
                                      const compPrice =
                                        parseFloat(ratio) *
                                        2 *
                                        parseFloat(value)
                                      setCompletionPrice(compPrice.toString())
                                    } else {
                                      setCompletionPrice('')
                                    }
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {field.value &&
                              !isNaN(parseFloat(field.value)) &&
                              promptPrice &&
                              !isNaN(parseFloat(promptPrice))
                                ? `Calculated price: $${(parseFloat(promptPrice) * parseFloat(field.value)).toFixed(4)} per 1M tokens`
                                : t('Multiplier for completion tokens.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </>
                  ) : (
                    <>
                      <div className='space-y-4'>
                        <div className='space-y-2'>
                          <Label>{t('Prompt price ($/1M tokens)')}</Label>
                          <Input
                            type='text'
                            placeholder='2.0'
                            value={promptPrice}
                            onChange={(e) =>
                              handlePromptPriceChange(e.target.value)
                            }
                          />
                          <p className='text-muted-foreground text-sm'>
                            {promptPrice && !isNaN(parseFloat(promptPrice))
                              ? `Calculated ratio: ${(parseFloat(promptPrice) / 2).toFixed(4)}`
                              : t('Enter Input price to calculate ratio')}
                          </p>
                        </div>

                        <div className='space-y-2'>
                          <Label>{t('Completion price ($/1M tokens)')}</Label>
                          <Input
                            type='text'
                            placeholder='4.0'
                            value={completionPrice}
                            onChange={(e) =>
                              handleCompletionPriceChange(e.target.value)
                            }
                          />
                          <p className='text-muted-foreground text-sm'>
                            {completionPrice &&
                            !isNaN(parseFloat(completionPrice)) &&
                            promptPrice &&
                            !isNaN(parseFloat(promptPrice)) &&
                            parseFloat(promptPrice) > 0
                              ? `Calculated ratio: ${(parseFloat(completionPrice) / parseFloat(promptPrice)).toFixed(4)}`
                              : t('Enter Completion price to calculate ratio')}
                          </p>
                        </div>
                      </div>
                    </>
                  )}

                  <Collapsible
                    open={advancedOpen}
                    onOpenChange={setAdvancedOpen}
                  >
                    <CollapsibleTrigger
                      render={
                        <Button
                          type='button'
                          variant='outline'
                          className='flex w-full items-center justify-between'
                        />
                      }
                    >
                      {t('Advanced options')}
                      <ChevronDown
                        className={`h-4 w-4 transition-transform duration-200 ${
                          advancedOpen ? 'rotate-180' : ''
                        }`}
                      />
                    </CollapsibleTrigger>
                    <CollapsibleContent className='space-y-6 pt-6'>
                      <FormField
                        control={form.control}
                        name='cacheRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {pricingSubMode === 'price'
                                ? t('Cache read price')
                                : t('Cache ratio')}
                            </FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='0.1'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {getAdvancedFieldDescription(
                                'Discount ratio for cache hits.',
                                'Token price for cache reads.'
                              )}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='createCacheRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {pricingSubMode === 'price'
                                ? t('Cache write price (5m)')
                                : t('Create cache ratio (5m)')}
                            </FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {getCacheCreateFieldDescription(field.value)}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='imageRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {pricingSubMode === 'price'
                                ? t('Image input price')
                                : t('Image ratio')}
                            </FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {getAdvancedFieldDescription(
                                'Multiplier for image processing.',
                                'Token price for image input.'
                              )}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='audioRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {pricingSubMode === 'price'
                                ? t('Audio input price')
                                : t('Audio ratio')}
                            </FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {getAdvancedFieldDescription(
                                'Multiplier for audio inputs.',
                                'Token price for audio input.'
                              )}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='audioCompletionRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {pricingSubMode === 'price'
                                ? t('Audio output price')
                                : t('Audio completion ratio')}
                            </FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {getAdvancedFieldDescription(
                                'Multiplier for audio outputs.',
                                'Token price for audio output.'
                              )}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </CollapsibleContent>
                  </Collapsible>
                </>
              )}
            </div>

            <Separator />

            {/* Status & Sync */}
            <div className='space-y-4'>
              <h3 className='text-sm font-semibold'>{t('Status & Sync')}</h3>

              <FormField
                control={form.control}
                name='status'
                render={({ field }) => (
                  <FormItem className='flex items-center justify-between rounded-lg border p-4'>
                    <div className='space-y-0.5'>
                      <FormLabel className='text-base'>
                        {t('Enabled')}
                      </FormLabel>
                      <FormDescription>
                        {t('Enable or disable this model')}
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
                name='sync_official'
                render={({ field }) => (
                  <FormItem className='flex items-center justify-between rounded-lg border p-4'>
                    <div className='space-y-0.5'>
                      <FormLabel className='text-base'>
                        {t('Official Sync')}
                      </FormLabel>
                      <FormDescription>
                        {t('Sync this model with official upstream')}
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
          </form>
        </Form>

        <SheetFooter className='grid grid-cols-2 gap-2 border-t px-4 py-3 sm:flex sm:px-6 sm:py-4'>
          <SheetClose
            render={<Button variant='outline' disabled={isSubmitting} />}
          >
            {t('Cancel')}
          </SheetClose>
          <Button form='model-form' type='submit' disabled={isSubmitting}>
            {isSubmitting && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {isEditing ? t('Update Model') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
