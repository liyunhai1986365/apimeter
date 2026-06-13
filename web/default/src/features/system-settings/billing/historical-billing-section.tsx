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
import { useState } from 'react'
import { DatabaseZap, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { generateRecentMonthlyBillingStatements } from '../api'
import type { HistoricalBillingGenerationResult } from '../types'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { SettingsSection } from '../components/settings-section'

function formatUnixDate(value: number) {
  if (!value) return '-'
  return new Date(value * 1000).toISOString().slice(0, 10)
}

export function HistoricalBillingSection() {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [result, setResult] =
    useState<HistoricalBillingGenerationResult | null>(null)

  async function handleGenerate() {
    setLoading(true)
    try {
      const response = await generateRecentMonthlyBillingStatements()
      if (response.success && response.data) {
        setResult(response.data)
        toast.success(t('Historical billing generated'))
      } else {
        toast.error(response.message || t('Request failed'))
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <SettingsSection
      title={t('Historical Billing')}
      description={t('Generate the most recent completed monthly bill')}
    >
      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2 text-base'>
            <DatabaseZap className='size-4' aria-hidden='true' />
            {t('Recent monthly bill')}
          </CardTitle>
        </CardHeader>
        <CardContent className='space-y-4'>
          <Alert>
            <AlertDescription>
              {t(
                'This action backfills logs and generates statements only for the previous completed month.'
              )}
            </AlertDescription>
          </Alert>
          <Button onClick={handleGenerate} disabled={loading}>
            {loading ? (
              <Loader2 className='size-4 animate-spin' aria-hidden='true' />
            ) : (
              <DatabaseZap className='size-4' aria-hidden='true' />
            )}
            {t('Generate recent monthly bill')}
          </Button>
          {result && (
            <div className='grid gap-3 rounded-lg border bg-muted/20 p-3 text-sm sm:grid-cols-2 xl:grid-cols-4'>
              <div>
                <div className='text-muted-foreground text-xs'>
                  {t('Month')}
                </div>
                <div className='font-medium'>{result.month}</div>
              </div>
              <div>
                <div className='text-muted-foreground text-xs'>
                  {t('Period')}
                </div>
                <div className='font-medium'>
                  {formatUnixDate(result.period_start)} -{' '}
                  {formatUnixDate(result.period_end)}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground text-xs'>
                  {t('Generated statements')}
                </div>
                <div className='font-medium'>{result.statement_count}</div>
              </div>
              <div>
                <div className='text-muted-foreground text-xs'>
                  {t('Failed users')}
                </div>
                <div className='font-medium'>
                  {result.failed_users.length
                    ? result.failed_users.join(', ')
                    : '-'}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground text-xs'>
                  {t('Scanned logs')}
                </div>
                <div className='font-medium'>{result.backfill.scanned}</div>
              </div>
              <div>
                <div className='text-muted-foreground text-xs'>
                  {t('Usage facts created')}
                </div>
                <div className='font-medium'>
                  {result.backfill.usage_created}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground text-xs'>
                  {t('Ledger entries created')}
                </div>
                <div className='font-medium'>
                  {result.backfill.ledger_created}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground text-xs'>
                  {t('Backfill failures')}
                </div>
                <div className='font-medium'>{result.backfill.failed}</div>
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </SettingsSection>
  )
}
