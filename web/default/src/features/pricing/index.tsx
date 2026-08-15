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
import { useCallback, useEffect, useRef } from 'react'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import {
  ArrowRight02Icon,
  CheckmarkCircle02Icon,
  SparklesIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { trackGoogleAnalyticsEvent } from '@/lib/google-analytics'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'
import {
  LoadingSkeleton,
  EmptyState,
  PricingTable,
  PricingFilterBar,
  PricingSidebar,
  PricingToolbar,
  ModelCardGrid,
} from './components'
import {
  PRICING_PAGE_TOP_PADDING_CLASS,
  PRICING_SIDEBAR_STICKY_CLASS,
  PRICING_STICKY_TOP_CLASS,
} from './components/pricing-layout'
import { VIEW_MODES } from './constants'
import { useFilters } from './hooks/use-filters'
import { usePricingData } from './hooks/use-pricing-data'

type SemCatalogAttribution = {
  source?: string
  medium?: string
  campaign?: string
  content?: string
  term?: string
  campaignId?: string
  adGroupId?: string
  creativeId?: string
  matchType?: string
  network?: string
  device?: string
}

function normalizeSemKeyword(term?: string): string {
  return (term || '')
    .replaceAll('[', ' ')
    .replaceAll(']', ' ')
    .replaceAll('"', ' ')
    .replaceAll("'", ' ')
    .replace(/\b(api key|apikey|api|token)\b/gi, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

function getSemCatalogEventParams(
  vendor: string | undefined,
  attribution: SemCatalogAttribution
): Record<string, unknown> {
  return {
    landing_type: 'pricing_catalog',
    vendor: vendor || 'all',
    sem_source: attribution.source || 'unknown',
    sem_medium: attribution.medium || 'unknown',
    sem_campaign: attribution.campaign || 'unknown',
    sem_content: attribution.content || 'unknown',
    sem_term: attribution.term || 'unknown',
    campaign_id: attribution.campaignId || 'unknown',
    ad_group_id: attribution.adGroupId || 'unknown',
    creative_id: attribution.creativeId || 'unknown',
    match_type: attribution.matchType || 'unknown',
    network: attribution.network || 'unknown',
    device: attribution.device || 'unknown',
  }
}

function SemCatalogLanding(props: {
  vendor?: string
  attribution: SemCatalogAttribution
  onBrowse: () => void
}) {
  const { t } = useTranslation()
  const isAuthenticated = useAuthStore((state) => Boolean(state.auth.user))
  const trackedViewRef = useRef(false)

  useEffect(() => {
    if (trackedViewRef.current) return
    trackedViewRef.current = true
    trackGoogleAnalyticsEvent(
      'sem_landing_view',
      getSemCatalogEventParams(props.vendor, props.attribution)
    )
  }, [props.attribution, props.vendor])

  const trackCta = (action: 'sign_up' | 'create_key' | 'browse_models') => {
    trackGoogleAnalyticsEvent('sem_cta_click', {
      action,
      placement: 'catalog_hero',
      ...getSemCatalogEventParams(props.vendor, props.attribution),
    })
  }

  return (
    <Card className='ring-primary/25 shadow-primary/5 relative mb-5 overflow-hidden shadow-lg'>
      <div
        aria-hidden='true'
        className='bg-primary absolute inset-x-0 top-0 h-1'
      />
      <div
        aria-hidden='true'
        className='bg-primary/10 pointer-events-none absolute -top-24 -right-24 size-72 rounded-full blur-3xl'
      />
      <CardHeader className='relative gap-4 pt-5 md:px-6'>
        <div className='flex flex-wrap gap-2'>
          <Badge>
            <HugeiconsIcon icon={SparklesIcon} data-icon='inline-start' />
            {t('Free trial')}
          </Badge>
          <Badge variant='secondary'>{t('One API key')}</Badge>
          <Badge variant='secondary'>{t('Pay-as-you-go')}</Badge>
          <Badge variant='secondary'>{t('Live pricing and uptime')}</Badge>
        </div>
        <CardTitle className='text-2xl leading-tight md:text-3xl'>
          {props.vendor
            ? t('Start using {{model}} API', { model: props.vendor })
            : t('Model Price')}
        </CardTitle>
        <CardDescription className='max-w-3xl text-sm leading-6 md:text-base'>
          {t(
            'Compare and access AI model providers, API pricing, supported endpoints and capabilities on {{site}}.',
            { site: 'Modelsell' }
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='relative grid gap-5 md:grid-cols-[minmax(0,1fr)_minmax(17rem,0.7fr)] md:px-6'>
        <div className='flex flex-col gap-4'>
          <p className='text-base font-semibold'>
            {t('From search to first API call')}
          </p>
          <ol className='grid gap-3 sm:grid-cols-3'>
            {[
              t('Create your Modelsell account'),
              t('Create one API key'),
              t('Copy the endpoint and run the example'),
            ].map((step, index) => (
              <li
                key={step}
                className='bg-background/80 ring-foreground/10 flex items-start gap-2 rounded-xl p-3 text-sm ring-1'
              >
                <Badge variant='outline'>{index + 1}</Badge>
                <span className='pt-0.5 leading-5'>{step}</span>
              </li>
            ))}
          </ol>
        </div>
        <div className='bg-primary/5 ring-primary/20 flex flex-col justify-center gap-3 rounded-2xl p-5 ring-1'>
          <Link
            to={isAuthenticated ? '/keys' : '/sign-up'}
            search={isAuthenticated ? undefined : { redirect: '/keys' }}
            className={cn(
              buttonVariants({ size: 'lg' }),
              'shadow-primary/20 h-11 w-full text-base shadow-lg'
            )}
            onClick={() => trackCta(isAuthenticated ? 'create_key' : 'sign_up')}
          >
            {isAuthenticated ? t('Create API Key') : t('Free trial')}
            <HugeiconsIcon icon={ArrowRight02Icon} data-icon='inline-end' />
          </Link>
          <Button
            type='button'
            size='lg'
            variant='outline'
            className='w-full'
            onClick={() => {
              trackCta('browse_models')
              props.onBrowse()
            }}
          >
            {t('Model Price')}
          </Button>
        </div>
      </CardContent>
      <CardFooter className='text-muted-foreground relative flex flex-wrap gap-x-5 gap-y-2 text-xs md:px-6'>
        <span className='flex items-center gap-1.5'>
          <HugeiconsIcon
            icon={CheckmarkCircle02Icon}
            data-icon='inline-start'
          />
          {t('Provider prices and uptime are visible before sign-up')}
        </span>
        <span className='flex items-center gap-1.5'>
          <HugeiconsIcon
            icon={CheckmarkCircle02Icon}
            data-icon='inline-start'
          />
          {t('Exact model ID shown before you create a key')}
        </span>
      </CardFooter>
    </Card>
  )
}

export function Pricing() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const search = useSearch({ from: '/pricing/' })
  const toOptionalSearchString = (
    value: string | number | boolean | undefined
  ): string | undefined => (value === undefined ? undefined : String(value))
  const semLanding =
    String(search.sem ?? '') === '1' ||
    ['cpc', 'ppc', 'paidsearch'].includes(
      toOptionalSearchString(search.utm_medium)?.toLowerCase() ?? ''
    )
  const semAttribution: SemCatalogAttribution = {
    source: toOptionalSearchString(search.utm_source),
    medium: toOptionalSearchString(search.utm_medium),
    campaign: toOptionalSearchString(search.utm_campaign),
    content: toOptionalSearchString(search.utm_content),
    term: toOptionalSearchString(search.utm_term),
    campaignId: toOptionalSearchString(search.campaign_id),
    adGroupId: toOptionalSearchString(search.adgroup_id),
    creativeId: toOptionalSearchString(search.creative_id),
    matchType: toOptionalSearchString(search.match_type),
    network: toOptionalSearchString(search.network),
    device: toOptionalSearchString(search.device),
  }

  const {
    models,
    vendors,
    userGroup,
    groupRatio,
    usableGroup,
    groupDisplay,
    isLoading,
    priceRate,
    usdExchangeRate,
  } = usePricingData()

  const {
    searchInput,
    sortBy,
    vendorFilter,
    groupFilter,
    quotaTypeFilter,
    endpointTypeFilter,
    categoryFilter,
    inputModalityFilter,
    outputModalityFilter,
    tokenUnit,
    viewMode,
    setSearchInput,
    setSortBy,
    setVendorFilter,
    setGroupFilter,
    setQuotaTypeFilter,
    setEndpointTypeFilter,
    setCategoryFilter,
    setInputModalityFilter,
    setOutputModalityFilter,
    setTokenUnit,
    setViewMode,
    filteredModels,
    hasActiveFilters,
    activeFilterCount,
    clearSearch,
    clearAllFilters,
  } = useFilters(models || [])
  const appliedSemSearchRef = useRef(false)

  useEffect(() => {
    if (
      appliedSemSearchRef.current ||
      !semLanding ||
      search.search ||
      search.vendor ||
      isLoading ||
      !models?.length
    ) {
      return
    }

    appliedSemSearchRef.current = true
    const normalizedTerm = normalizeSemKeyword(semAttribution.term)
    if (!normalizedTerm) return

    const comparableTerm = normalizedTerm.toLocaleLowerCase()
    const hasMatchingModel = models.some((model) =>
      [model.model_name, model.vendor_name, ...(model.alias_models || [])]
        .filter(Boolean)
        .some((value) =>
          String(value).toLocaleLowerCase().includes(comparableTerm)
        )
    )
    if (hasMatchingModel) {
      setSearchInput(normalizedTerm)
    }
  }, [
    isLoading,
    models,
    search.search,
    search.vendor,
    semAttribution.term,
    semLanding,
    setSearchInput,
  ])

  const handleModelClick = useCallback(
    (modelName: string) => {
      if (!modelName) return

      navigate({
        to: '/pricing/$modelId',
        params: { modelId: modelName },
        search,
      })
    },
    [navigate, search]
  )

  const handleClearAll = useCallback(() => {
    clearAllFilters()
  }, [clearAllFilters])

  const renderPricingContent = () => {
    if (filteredModels.length === 0) {
      return (
        <EmptyState
          searchQuery={searchInput}
          hasActiveFilters={hasActiveFilters}
          onClearFilters={handleClearAll}
        />
      )
    }

    if (viewMode === VIEW_MODES.CARD) {
      return (
        <ModelCardGrid
          models={filteredModels}
          onModelClick={handleModelClick}
          priceRate={priceRate}
          usdExchangeRate={usdExchangeRate}
          tokenUnit={tokenUnit}
          showRechargePrice={false}
          groupDisplay={groupDisplay}
        />
      )
    }

    return (
      <PricingTable
        models={filteredModels}
        priceRate={priceRate}
        usdExchangeRate={usdExchangeRate}
        tokenUnit={tokenUnit}
        showRechargePrice={false}
        groupDisplay={groupDisplay}
        onModelClick={handleModelClick}
      />
    )
  }

  if (isLoading) {
    return (
      <PublicLayout showMainContainer={false}>
        <div
          className={`mx-auto w-full max-w-[1800px] px-3 pb-8 sm:px-6 sm:pb-10 xl:px-8 ${PRICING_PAGE_TOP_PADDING_CLASS}`}
        >
          <LoadingSkeleton viewMode={viewMode} />
        </div>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition
        className={`mx-auto w-full max-w-[1800px] px-3 pb-8 sm:px-6 sm:pb-10 xl:px-8 ${PRICING_PAGE_TOP_PADDING_CLASS}`}
      >
        <div className='grid gap-3 lg:grid-cols-[13rem_minmax(0,1fr)] xl:grid-cols-[14rem_minmax(0,1fr)]'>
          <PricingSidebar
            quotaTypeFilter={quotaTypeFilter}
            endpointTypeFilter={endpointTypeFilter}
            vendorFilter={vendorFilter}
            groupFilter={groupFilter}
            inputModalityFilter={inputModalityFilter}
            outputModalityFilter={outputModalityFilter}
            onQuotaTypeChange={setQuotaTypeFilter}
            onEndpointTypeChange={setEndpointTypeFilter}
            onVendorChange={setVendorFilter}
            onGroupChange={setGroupFilter}
            onInputModalityChange={setInputModalityFilter}
            onOutputModalityChange={setOutputModalityFilter}
            vendors={vendors || []}
            groups={Object.keys(usableGroup || {})}
            groupRatios={groupRatio}
            groupDisplay={groupDisplay}
            models={models || []}
            hasActiveFilters={hasActiveFilters}
            activeFilterCount={activeFilterCount}
            onClearFilters={clearAllFilters}
            className={`lg:sticky lg:overflow-y-auto ${PRICING_SIDEBAR_STICKY_CLASS}`}
          />

          <main className='min-w-0'>
            <h1 className='sr-only'>{t('Model Price')}</h1>
            {semLanding && (
              <SemCatalogLanding
                vendor={search.vendor}
                attribution={semAttribution}
                onBrowse={() =>
                  document
                    .getElementById('pricing-results')
                    ?.scrollIntoView({ behavior: 'smooth', block: 'start' })
                }
              />
            )}
            <div
              id='pricing-results'
              className={`bg-background/92 supports-[backdrop-filter]:bg-background/75 sticky z-30 -mx-1 mb-7 flex flex-col gap-2.5 rounded-b-xl px-1 pb-3 shadow-sm backdrop-blur ${PRICING_STICKY_TOP_CLASS}`}
            >
              <PricingFilterBar
                categoryFilter={categoryFilter}
                onCategoryChange={setCategoryFilter}
                models={models || []}
              />

              <PricingToolbar
                filteredCount={filteredModels.length}
                totalCount={models?.length}
                searchValue={searchInput}
                onSearchChange={setSearchInput}
                onClearSearch={clearSearch}
                sortBy={sortBy}
                onSortChange={setSortBy}
                tokenUnit={tokenUnit}
                onTokenUnitChange={setTokenUnit}
                viewMode={viewMode}
                onViewModeChange={setViewMode}
                hasActiveFilters={hasActiveFilters}
                quotationModels={filteredModels}
                priceRate={priceRate}
                usdExchangeRate={usdExchangeRate}
                userGroup={userGroup}
                usableGroup={usableGroup}
                groupDisplay={groupDisplay}
              />
            </div>

            {renderPricingContent()}
          </main>
        </div>
      </PageTransition>
    </PublicLayout>
  )
}
