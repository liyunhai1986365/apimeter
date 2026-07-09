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
import { useCallback } from 'react'
import { useNavigate, useSearch } from '@tanstack/react-router'
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

export function Pricing() {
  const navigate = useNavigate()
  const search = useSearch({ from: '/pricing/' })

  const {
    models,
    vendors,
    groupRatio,
    usableGroup,
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
            models={models || []}
            hasActiveFilters={hasActiveFilters}
            activeFilterCount={activeFilterCount}
            onClearFilters={clearAllFilters}
            className={`lg:sticky lg:overflow-y-auto ${PRICING_SIDEBAR_STICKY_CLASS}`}
          />

          <main className='min-w-0'>
            <div
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
              />
            </div>

            {renderPricingContent()}
          </main>
        </div>
      </PageTransition>
    </PublicLayout>
  )
}
