export type ModelPricingFormValues = {
  price?: string
  ratio?: string
  cacheRatio?: string
  createCacheRatio?: string
  completionRatio?: string
  imageRatio?: string
  audioRatio?: string
  audioCompletionRatio?: string
  billingExpr?: string
}

export type PricingInputMode = 'ratio' | 'price'

export type ModelPricingMaps = {
  priceMap: Record<string, number>
  ratioMap: Record<string, number>
  cacheMap: Record<string, number>
  createCacheMap: Record<string, number>
  completionMap: Record<string, number>
  imageMap: Record<string, number>
  audioMap: Record<string, number>
  audioCompletionMap: Record<string, number>
  billingModeMap: Record<string, string>
  billingExprMap: Record<string, string>
}

export type PricingMode = 'per-token' | 'per-request' | 'tiered_expr'

export type ModelPricingSyncInput = {
  maps: ModelPricingMaps
  values: ModelPricingFormValues
  pricingMode: PricingMode
  pricingInputMode?: PricingInputMode
  finalModelName: string
  oldModelName?: string
  isEditing?: boolean
}

function cloneMaps(maps: ModelPricingMaps): ModelPricingMaps {
  return {
    priceMap: { ...maps.priceMap },
    ratioMap: { ...maps.ratioMap },
    cacheMap: { ...maps.cacheMap },
    createCacheMap: { ...maps.createCacheMap },
    completionMap: { ...maps.completionMap },
    imageMap: { ...maps.imageMap },
    audioMap: { ...maps.audioMap },
    audioCompletionMap: { ...maps.audioCompletionMap },
    billingModeMap: { ...maps.billingModeMap },
    billingExprMap: { ...maps.billingExprMap },
  }
}

function parseOptionalNumber(value?: string): number | undefined {
  const trimmed = value?.trim()
  if (!trimmed) return undefined
  const parsed = Number(trimmed)
  return Number.isFinite(parsed) ? parsed : undefined
}

function formatNumber(value: number): string {
  if (!Number.isFinite(value)) return ''
  return Number(value.toPrecision(12)).toString()
}

function formatOptionalNumber(value: number | undefined): string | undefined {
  if (value === undefined) return undefined
  return formatNumber(value)
}

function divideByBasePrice(
  value: string | undefined,
  basePrice: number | undefined
): string | undefined {
  const parsed = parseOptionalNumber(value)
  if (parsed === undefined) return value
  if (basePrice === undefined || basePrice === 0) return ''
  return formatNumber(parsed / basePrice)
}

function multiplyByBasePrice(
  value: string | undefined,
  basePrice: number | undefined
): string | undefined {
  const parsed = parseOptionalNumber(value)
  if (parsed === undefined) return value
  if (basePrice === undefined) return ''
  return formatNumber(parsed * basePrice)
}

export function normalizeModelPricingValuesForInputMode(
  values: ModelPricingFormValues,
  inputMode: PricingInputMode = 'ratio'
): ModelPricingFormValues {
  if (inputMode !== 'price') return values

  const inputRatio = parseOptionalNumber(values.ratio)
  const inputPrice =
    inputRatio !== undefined ? inputRatio * 2 : parseOptionalNumber(values.price)
  const audioRatioValue = divideByBasePrice(values.audioRatio, inputPrice)
  const audioInputPrice = parseOptionalNumber(values.audioRatio)

  return {
    ...values,
    cacheRatio: divideByBasePrice(values.cacheRatio, inputPrice),
    createCacheRatio: divideByBasePrice(values.createCacheRatio, inputPrice),
    imageRatio: divideByBasePrice(values.imageRatio, inputPrice),
    audioRatio: audioRatioValue,
    audioCompletionRatio: divideByBasePrice(
      values.audioCompletionRatio,
      audioInputPrice
    ),
  }
}

export function convertRatioValuesToPriceValues(
  values: ModelPricingFormValues
): ModelPricingFormValues {
  const inputRatio = parseOptionalNumber(values.ratio)
  const inputPrice = inputRatio !== undefined ? inputRatio * 2 : undefined
  const audioRatio = parseOptionalNumber(values.audioRatio)
  const audioInputPrice =
    inputPrice !== undefined && audioRatio !== undefined
      ? inputPrice * audioRatio
      : undefined

  return {
    ...values,
    cacheRatio: multiplyByBasePrice(values.cacheRatio, inputPrice),
    createCacheRatio: multiplyByBasePrice(values.createCacheRatio, inputPrice),
    imageRatio: multiplyByBasePrice(values.imageRatio, inputPrice),
    audioRatio: formatOptionalNumber(audioInputPrice),
    audioCompletionRatio: multiplyByBasePrice(
      values.audioCompletionRatio,
      audioInputPrice
    ),
  }
}

function hasExplicitPricingInput(
  values: ModelPricingFormValues,
  pricingMode: PricingMode
): boolean {
  if (pricingMode === 'per-request') {
    return parseOptionalNumber(values.price) !== undefined
  }

  if (pricingMode === 'tiered_expr') {
    if (values.billingExpr?.trim()) return true
  }

  return [
    values.ratio,
    values.cacheRatio,
    values.createCacheRatio,
    values.completionRatio,
    values.imageRatio,
    values.audioRatio,
    values.audioCompletionRatio,
  ].some((value) => parseOptionalNumber(value) !== undefined)
}

function movePricingEntry(
  map: Record<string, number>,
  oldModelName: string,
  finalModelName: string
): void {
  if (oldModelName === finalModelName) return
  if (map[finalModelName] !== undefined) return
  const oldValue = map[oldModelName]
  if (oldValue === undefined) return
  map[finalModelName] = oldValue
  delete map[oldModelName]
}

function deletePricingEntries(
  maps: ModelPricingMaps,
  modelName: string
): void {
  delete maps.priceMap[modelName]
  delete maps.ratioMap[modelName]
  delete maps.cacheMap[modelName]
  delete maps.createCacheMap[modelName]
  delete maps.completionMap[modelName]
  delete maps.imageMap[modelName]
  delete maps.audioMap[modelName]
  delete maps.audioCompletionMap[modelName]
  delete maps.billingModeMap[modelName]
  delete maps.billingExprMap[modelName]
}

function migrateModelPricing(
  maps: ModelPricingMaps,
  oldModelName: string | undefined,
  finalModelName: string
): void {
  if (!oldModelName || oldModelName === finalModelName) return

  movePricingEntry(maps.priceMap, oldModelName, finalModelName)
  movePricingEntry(maps.ratioMap, oldModelName, finalModelName)
  movePricingEntry(maps.cacheMap, oldModelName, finalModelName)
  movePricingEntry(maps.createCacheMap, oldModelName, finalModelName)
  movePricingEntry(maps.completionMap, oldModelName, finalModelName)
  movePricingEntry(maps.imageMap, oldModelName, finalModelName)
  movePricingEntry(maps.audioMap, oldModelName, finalModelName)
  movePricingEntry(maps.audioCompletionMap, oldModelName, finalModelName)

  if (maps.billingModeMap[finalModelName] === undefined) {
    const oldMode = maps.billingModeMap[oldModelName]
    if (oldMode !== undefined) {
      maps.billingModeMap[finalModelName] = oldMode
      delete maps.billingModeMap[oldModelName]
    }
  }
  if (maps.billingExprMap[finalModelName] === undefined) {
    const oldExpr = maps.billingExprMap[oldModelName]
    if (oldExpr !== undefined) {
      maps.billingExprMap[finalModelName] = oldExpr
      delete maps.billingExprMap[oldModelName]
    }
  }
}

export function syncModelPricingMaps(
  input: ModelPricingSyncInput
): ModelPricingMaps {
  const maps = cloneMaps(input.maps)
  const finalModelName = input.finalModelName.trim()
  const oldModelName = input.oldModelName?.trim()
  if (!finalModelName) return maps

  const values = normalizeModelPricingValuesForInputMode(
    input.values,
    input.pricingInputMode
  )
  const hasNewPricing = hasExplicitPricingInput(values, input.pricingMode)
  if (!hasNewPricing) {
    if (input.isEditing) {
      migrateModelPricing(maps, oldModelName, finalModelName)
    }
    return maps
  }

  if (input.isEditing && oldModelName && oldModelName !== finalModelName) {
    deletePricingEntries(maps, oldModelName)
  }
  deletePricingEntries(maps, finalModelName)

  if (input.pricingMode === 'per-request') {
    const price = parseOptionalNumber(values.price)
    if (price !== undefined) {
      maps.priceMap[finalModelName] = price
    }
    return maps
  }

  if (input.pricingMode === 'tiered_expr') {
    const billingExpr = values.billingExpr?.trim()
    if (billingExpr) {
      maps.billingModeMap[finalModelName] = 'tiered_expr'
      maps.billingExprMap[finalModelName] = billingExpr
    }
  }

  const ratio = parseOptionalNumber(values.ratio)
  if (ratio !== undefined) {
    maps.ratioMap[finalModelName] = ratio
  }

  const cacheRatio = parseOptionalNumber(values.cacheRatio)
  if (cacheRatio !== undefined) {
    maps.cacheMap[finalModelName] = cacheRatio
  }

  const createCacheRatio = parseOptionalNumber(values.createCacheRatio)
  if (createCacheRatio !== undefined) {
    maps.createCacheMap[finalModelName] = createCacheRatio
  }

  const completionRatio = parseOptionalNumber(values.completionRatio)
  if (completionRatio !== undefined) {
    maps.completionMap[finalModelName] = completionRatio
  }

  const imageRatio = parseOptionalNumber(values.imageRatio)
  if (imageRatio !== undefined) {
    maps.imageMap[finalModelName] = imageRatio
  }

  const audioRatio = parseOptionalNumber(values.audioRatio)
  if (audioRatio !== undefined) {
    maps.audioMap[finalModelName] = audioRatio
  }

  const audioCompletionRatio = parseOptionalNumber(
    values.audioCompletionRatio
  )
  if (audioCompletionRatio !== undefined) {
    maps.audioCompletionMap[finalModelName] = audioCompletionRatio
  }

  return maps
}
