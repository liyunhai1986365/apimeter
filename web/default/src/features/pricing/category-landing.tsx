/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Link, useParams } from '@tanstack/react-router'
import { ArrowRight02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { PublicLayout } from '@/components/layout'
import { MODEL_CATEGORIES } from './constants'
import { usePricingData } from './hooks'
import { getSEOCategory } from './seo-categories'

export function PricingCategoryLanding() {
  const { t } = useTranslation()
  const { category: categorySlug } = useParams({
    from: '/pricing/categories/$category/',
  })
  const category = getSEOCategory(categorySlug)
  const { models, isLoading } = usePricingData()
  const categoryModels = models.filter(
    (model) =>
      (model.category || MODEL_CATEGORIES.TEXT).toLowerCase() === category?.slug
  )

  if (!category) return null

  return (
    <PublicLayout showMainContainer={false}>
      <main className='mx-auto flex w-full max-w-7xl flex-col gap-10 px-4 pt-28 pb-16 sm:px-6 lg:px-8'>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink render={<Link to='/pricing' />}>
                {t('Model Price')}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{t(category.titleKey)}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>

        <header className='flex max-w-4xl flex-col gap-4'>
          <p className='text-primary text-sm font-medium'>
            {t('Explore AI model APIs by category')}
          </p>
          <h1 className='text-4xl font-bold tracking-tight sm:text-5xl'>
            {t(category.titleKey)}
          </h1>
          <p className='text-muted-foreground text-base leading-7 sm:text-lg'>
            {t(category.descriptionKey)}
          </p>
          {!isLoading && (
            <p className='text-muted-foreground text-sm'>
              {t('{{count}} available models', {
                count: categoryModels.length,
              })}
            </p>
          )}
        </header>

        <section aria-labelledby='category-model-directory'>
          <div className='mb-5 flex items-end justify-between gap-4'>
            <div className='flex flex-col gap-1'>
              <h2
                id='category-model-directory'
                className='text-2xl font-semibold tracking-tight'
              >
                {t('Compare model API pricing and capabilities')}
              </h2>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Open a model page for current prices, endpoints, context limits and related APIs.'
                )}
              </p>
            </div>
            <Button variant='outline' render={<Link to='/pricing' />}>
              {t('All model pricing')}
            </Button>
          </div>

          {isLoading ? (
            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
              {Array.from({ length: 6 }).map((_, index) => (
                <Skeleton key={index} className='h-44 rounded-xl' />
              ))}
            </div>
          ) : (
            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
              {categoryModels.map((model) => (
                <Card key={model.model_name}>
                  <CardHeader>
                    <CardTitle>{model.model_name}</CardTitle>
                    <CardDescription>
                      {model.description || t(category.descriptionKey)}
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <p className='text-muted-foreground text-xs'>
                      {t('{{count}} supported API endpoints', {
                        count: model.supported_endpoint_types?.length ?? 0,
                      })}
                    </p>
                  </CardContent>
                  <CardFooter>
                    <Button
                      variant='link'
                      render={
                        <Link
                          to='/pricing/$modelId'
                          params={{ modelId: model.model_name }}
                        />
                      }
                    >
                      {t('View API pricing')}
                      <HugeiconsIcon
                        icon={ArrowRight02Icon}
                        data-icon='inline-end'
                      />
                    </Button>
                  </CardFooter>
                </Card>
              ))}
            </div>
          )}
        </section>
      </main>
    </PublicLayout>
  )
}
