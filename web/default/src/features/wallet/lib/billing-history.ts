import type { TopupRecord } from '../types'

export interface BillingHistorySummary {
  currentPageCount: number
  successAmount: number
  paymentTotal: number
  pendingCount: number
}

export function summarizeBillingHistory(
  records: TopupRecord[]
): BillingHistorySummary {
  return records.reduce<BillingHistorySummary>(
    (summary, record) => {
      summary.paymentTotal += Number(record.money) || 0
      if (record.status === 'success') {
        summary.successAmount += Number(record.amount) || 0
      }
      if (record.status === 'pending') {
        summary.pendingCount += 1
      }
      return summary
    },
    {
      currentPageCount: records.length,
      successAmount: 0,
      paymentTotal: 0,
      pendingCount: 0,
    }
  )
}
