/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { Modality, ModelCategory } from '../types'
import {
  buildQuotationFilename,
  buildQuotationRows,
  type QuotationOptions,
} from './quotation'

const CATEGORY_LABEL_KEYS: Record<ModelCategory, string> = {
  text: 'Text',
  vector: 'Vector',
  image: 'Image',
  audio: 'Audio',
  video: 'Video',
  other: 'Other',
}

const MODALITY_LABEL_KEYS: Record<Modality, string> = {
  text: 'Text',
  image: 'Image',
  audio: 'Audio',
  video: 'Video',
  file: 'File',
}

const HEADER_FILL = 'FF000000'
const HEADER_TEXT = 'FFFFFFFF'
const MUTED_TEXT = 'FF6B7280'
const BODY_TEXT = 'FF111827'
const CATEGORY_FILL = 'FFF2F2F2'
const MANUFACTURER_FILL = 'FFFAFAFA'
const BORDER_COLOR = 'FFD9D9D9'
const HEADER_BORDER_COLOR = 'FF374151'
type Translate = QuotationOptions['translate']

export type QuotationSpreadsheetRow = {
  category: string
  manufacturer: string
  model: string
  modality: string
  billingScenario: string
  inputPrice: string | null
  inputUnit: string | null
  outputPrice: string | null
  outputUnit: string | null
  cacheWritePrice: string | null
  cacheWriteUnit: string | null
  cacheWrite1hPrice: string | null
  cacheWrite1hUnit: string | null
  cacheReadPrice: string | null
  cacheReadUnit: string | null
  supplier: string
  description: string | null
  discountRatio: number | null
}

export type QuotationSpreadsheetResult = {
  filename: string
  rowCount: number
  sheetCount: number
}

export type QuotationSpreadsheetBuildResult = QuotationSpreadsheetResult & {
  buffer: ArrayBuffer
}

function translateList(
  values: readonly string[],
  translate: Translate
): string {
  return values.map((value) => translate(value)).join(', ')
}

function formatModalityFlow(
  input: Modality[],
  output: Modality[],
  translate: Translate
): string {
  const inputLabel = translateList(
    input.map((item) => MODALITY_LABEL_KEYS[item]),
    translate
  )
  const outputLabel = translateList(
    output.map((item) => MODALITY_LABEL_KEYS[item]),
    translate
  )
  return `${inputLabel || '-'} → ${outputLabel || '-'}`
}

function safeWorksheetName(value: string, fallback: string): string {
  const normalized = value
    .replace(/[\\/?*:[\]]/g, ' ')
    .trim()
    .slice(0, 31)
  return normalized || fallback
}

function buildPriceCellValue(price: string | null, unit: string | null) {
  if (!price || !unit) return price
  return {
    richText: [
      { text: price, font: { bold: true } },
      {
        text: `\n/${unit}`,
        font: { color: { argb: MUTED_TEXT }, size: 9 },
      },
    ],
  }
}

function buildCacheWriteCellValue(
  prices: Array<{ ttl: string; price: string | null; unit: string | null }>
) {
  const availablePrices = prices.filter(
    (item): item is { ttl: string; price: string; unit: string | null } =>
      Boolean(item.price)
  )
  if (availablePrices.length === 0) return null

  return {
    richText: availablePrices.flatMap((item, index) => [
      ...(index > 0 ? [{ text: '\n' }] : []),
      {
        text: `${item.ttl} `,
        font: { color: { argb: MUTED_TEXT }, size: 9 },
      },
      { text: item.price, font: { bold: true } },
      ...(item.unit
        ? [
            {
              text: ` /${item.unit}`,
              font: { color: { argb: MUTED_TEXT }, size: 9 },
            },
          ]
        : []),
    ]),
  }
}

function getConfiguredSupplierDescription(
  usableGroup: QuotationOptions['usableGroup'],
  group: string
): string {
  const supplier = usableGroup[group]
  return typeof supplier === 'string' ? supplier : supplier?.desc || ''
}

function translateSupplierDescription(
  options: QuotationOptions,
  group: string
): string {
  const description = getConfiguredSupplierDescription(
    options.usableGroup,
    group
  )
  return description ? options.translate(description) : ''
}

export function buildQuotationSpreadsheetRows(
  options: QuotationOptions
): QuotationSpreadsheetRow[] {
  const rows = buildQuotationRows(options.models, {
    tokenUnit: options.tokenUnit,
    showRechargePrice: false,
    priceRate: options.priceRate,
    usdExchangeRate: options.usdExchangeRate,
    usableGroup: options.usableGroup,
    groupDisplay: options.groupDisplay,
    discountLabels: options.discountLabels,
  })

  return rows.flatMap((row) => {
    const suppliers =
      row.supplierDiscounts.length > 0 ? row.supplierDiscounts : [null]
    const billingScenario = [
      options.translate(row.billingLabelKey),
      row.scenario ? options.translate(row.scenario) : '',
    ]
      .filter(Boolean)
      .join(' — ')

    return suppliers.map((supplier) => ({
      category: options.translate(CATEGORY_LABEL_KEYS[row.category]),
      manufacturer: row.vendorName,
      model: row.modelName,
      modality: formatModalityFlow(
        row.inputModalities,
        row.outputModalities,
        options.translate
      ),
      billingScenario,
      inputPrice: row.requiresOnlineDetails
        ? options.translate('See online details')
        : row.primaryPrice === '-'
          ? null
          : row.primaryPrice,
      inputUnit:
        row.requiresOnlineDetails || row.primaryPrice === '-'
          ? null
          : row.primaryUnitLabel,
      outputPrice: row.outputPrice === '-' ? null : row.outputPrice,
      outputUnit: row.outputPrice === '-' ? null : row.outputUnitLabel,
      cacheWritePrice: row.cacheWritePrice === '-' ? null : row.cacheWritePrice,
      cacheWriteUnit:
        row.cacheWritePrice === '-' ? null : row.cacheWriteUnitLabel,
      cacheWrite1hPrice:
        row.cacheWrite1hPrice === '-' ? null : row.cacheWrite1hPrice,
      cacheWrite1hUnit:
        row.cacheWrite1hPrice === '-' ? null : row.cacheWrite1hUnitLabel,
      cacheReadPrice: row.cachePrice === '-' ? null : row.cachePrice,
      cacheReadUnit: row.cachePrice === '-' ? null : row.cacheUnitLabel,
      supplier: supplier?.group || '',
      description: supplier
        ? translateSupplierDescription(options, supplier.group) || null
        : null,
      discountRatio: supplier?.ratio ?? null,
    }))
  })
}

function mergeConsecutiveRows<T>(
  rows: readonly T[],
  keySelector: (row: T) => string,
  merge: (startRow: number, endRow: number) => void
) {
  let groupStart = 0

  for (let index = 1; index <= rows.length; index += 1) {
    const currentKey = index < rows.length ? keySelector(rows[index]) : null
    const previousKey = keySelector(rows[index - 1])
    if (currentKey === previousKey) continue

    if (index - groupStart > 1) {
      merge(groupStart + 2, index + 1)
    }
    groupStart = index
  }
}

export async function buildQuotationSpreadsheet(
  options: QuotationOptions
): Promise<QuotationSpreadsheetBuildResult> {
  const { Workbook } = await import('exceljs')
  const generatedAt = options.generatedAt || new Date()
  const rows = buildQuotationSpreadsheetRows(options)
  const workbook = new Workbook()
  const { translate: t } = options

  workbook.creator = options.siteName
  workbook.lastModifiedBy = options.siteName
  workbook.created = generatedAt
  workbook.modified = generatedAt

  const quotation = workbook.addWorksheet(
    safeWorksheetName(t('Quotation'), 'Quotation'),
    {
      views: [
        {
          state: 'frozen',
          xSplit: 3,
          ySplit: 1,
          showGridLines: false,
        },
      ],
      pageSetup: {
        paperSize: 9,
        orientation: 'landscape',
        fitToPage: true,
        fitToWidth: 1,
        fitToHeight: 0,
        printTitlesRow: '1:1',
      },
    }
  )

  const headers = [
    t('Category'),
    t('Manufacturer'),
    t('Model'),
    t('Modality'),
    t('Billing / Scenario'),
    t('Input price'),
    t('Output price'),
    t('Cache write price'),
    t('Cache read price'),
    t('Supplier'),
    t('Billing discount'),
    t('Description'),
  ]
  const tableRows = rows.map((row) => [
    row.category,
    row.manufacturer,
    row.model,
    row.modality,
    row.billingScenario,
    buildPriceCellValue(row.inputPrice, row.inputUnit),
    buildPriceCellValue(row.outputPrice, row.outputUnit),
    buildCacheWriteCellValue([
      {
        ttl: '5m',
        price: row.cacheWritePrice,
        unit: row.cacheWriteUnit,
      },
      {
        ttl: '1h',
        price: row.cacheWrite1hPrice,
        unit: row.cacheWrite1hUnit,
      },
    ]),
    buildPriceCellValue(row.cacheReadPrice, row.cacheReadUnit),
    row.supplier,
    row.discountRatio,
    row.description,
  ])

  quotation.addRows([headers, ...tableRows])
  const widths = [12, 18, 38, 22, 26, 21, 21, 24, 21, 22, 14, 60]
  widths.forEach((width, index) => {
    quotation.getColumn(index + 1).width = width
  })

  const header = quotation.getRow(1)
  header.height = 34
  header.eachCell((cell) => {
    cell.fill = {
      type: 'pattern',
      pattern: 'solid',
      fgColor: { argb: HEADER_FILL },
    }
    cell.font = {
      name: 'Arial',
      size: 12,
      bold: true,
      color: { argb: HEADER_TEXT },
    }
    cell.alignment = {
      vertical: 'middle',
      horizontal: 'center',
      wrapText: true,
    }
    cell.border = {
      top: { style: 'thin', color: { argb: HEADER_BORDER_COLOR } },
      bottom: { style: 'thin', color: { argb: HEADER_BORDER_COLOR } },
      left: { style: 'thin', color: { argb: HEADER_BORDER_COLOR } },
      right: { style: 'thin', color: { argb: HEADER_BORDER_COLOR } },
    }
  })

  for (let rowNumber = 2; rowNumber <= rows.length + 1; rowNumber += 1) {
    const row = quotation.getRow(rowNumber)
    const spreadsheetRow = rows[rowNumber - 2]
    const descriptionLines = spreadsheetRow.description
      ? Math.ceil(spreadsheetRow.description.length / 45)
      : 1
    const cacheWriteLines = spreadsheetRow.cacheWrite1hPrice ? 2 : 1
    row.height = Math.min(
      72,
      Math.max(34, descriptionLines * 18 + 8, cacheWriteLines * 18 + 8)
    )
    row.eachCell({ includeEmpty: true }, (cell, columnNumber) => {
      if (columnNumber === 1) {
        cell.fill = {
          type: 'pattern',
          pattern: 'solid',
          fgColor: { argb: CATEGORY_FILL },
        }
      } else if (columnNumber === 2) {
        cell.fill = {
          type: 'pattern',
          pattern: 'solid',
          fgColor: { argb: MANUFACTURER_FILL },
        }
      } else {
        cell.fill = {
          type: 'pattern',
          pattern: 'solid',
          fgColor: { argb: 'FFFFFFFF' },
        }
      }
      cell.font = { name: 'Arial', size: 12, color: { argb: BODY_TEXT } }
      cell.alignment = {
        vertical: 'middle',
        horizontal:
          columnNumber <= 2 ||
          (columnNumber >= 4 && columnNumber <= 9) ||
          columnNumber === 11
            ? 'center'
            : 'left',
        wrapText: true,
      }
      cell.border = {
        top: { style: 'thin', color: { argb: BORDER_COLOR } },
        bottom: { style: 'thin', color: { argb: BORDER_COLOR } },
        left: { style: 'thin', color: { argb: BORDER_COLOR } },
        right: { style: 'thin', color: { argb: BORDER_COLOR } },
      }
    })
    const discountCell = row.getCell(11)
    discountCell.numFmt = '0.####'
    discountCell.alignment = { vertical: 'middle', horizontal: 'center' }
  }

  mergeConsecutiveRows(
    rows,
    (row) => row.category,
    (startRow, endRow) => quotation.mergeCells(startRow, 1, endRow, 1)
  )
  mergeConsecutiveRows(
    rows,
    (row) => `${row.category}\u0000${row.manufacturer}`,
    (startRow, endRow) => quotation.mergeCells(startRow, 2, endRow, 2)
  )
  mergeConsecutiveRows(
    rows,
    (row) =>
      [
        row.category,
        row.manufacturer,
        row.model,
        row.modality,
        row.billingScenario,
        row.inputPrice,
        row.inputUnit,
        row.outputPrice,
        row.outputUnit,
        row.cacheWritePrice,
        row.cacheWriteUnit,
        row.cacheWrite1hPrice,
        row.cacheWrite1hUnit,
        row.cacheReadPrice,
        row.cacheReadUnit,
      ].join('\u0000'),
    (startRow, endRow) => {
      for (let columnNumber = 3; columnNumber <= 9; columnNumber += 1) {
        quotation.mergeCells(startRow, columnNumber, endRow, columnNumber)
      }
    }
  )

  const buffer = await workbook.xlsx.writeBuffer()
  return {
    buffer,
    filename: buildQuotationFilename(
      options.siteName,
      generatedAt,
      options.userGroup
    ),
    rowCount: rows.length,
    sheetCount: workbook.worksheets.length,
  }
}

function saveSpreadsheetBlob(blob: Blob, filename: string) {
  const objectUrl = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = objectUrl
  link.download = filename
  link.hidden = true
  document.body.appendChild(link)
  link.click()
  link.remove()
  window.setTimeout(() => URL.revokeObjectURL(objectUrl), 1000)
}

export async function downloadQuotationSpreadsheet(
  options: QuotationOptions
): Promise<QuotationSpreadsheetResult> {
  const result = await buildQuotationSpreadsheet(options)
  saveSpreadsheetBlob(
    new Blob([result.buffer], {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    }),
    result.filename
  )
  return {
    filename: result.filename,
    rowCount: result.rowCount,
    sheetCount: result.sheetCount,
  }
}
