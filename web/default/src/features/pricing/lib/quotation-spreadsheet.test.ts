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
import { type Fill, Workbook } from 'exceljs'
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { defaultGroupDiscountLabels } from '@/lib/group-discount'
import type { PricingModel } from '../types'
import type { QuotationOptions } from './quotation'
import {
  buildQuotationSpreadsheet,
  buildQuotationSpreadsheetRows,
} from './quotation-spreadsheet'

const models: PricingModel[] = [
  {
    id: 1,
    model_name: 'gpt-test',
    vendor_name: 'OpenAI',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 2,
    cache_ratio: 0.5,
    create_cache_ratio: 1.25,
    enable_groups: ['all'],
    group_ratio: { supplierA: 0.5, supplierB: 0.25 },
    input_modalities: ['text'],
    output_modalities: ['text'],
  },
]

function options(overrides: Partial<QuotationOptions> = {}): QuotationOptions {
  return {
    models,
    siteName: 'Modelsell / Enterprise',
    tokenUnit: 'M',
    locale: 'en',
    hasActiveFilters: false,
    sourceUrl: 'https://example.com/pricing',
    usableGroup: { supplierA: 'Supplier A', supplierB: 'Supplier B' },
    groupDisplay: {
      categories: [{ id: 'preferred', name: 'Preferred', order: 1 }],
      groups: [
        { group: 'supplierB', category_id: 'preferred', order: 1 },
        { group: 'supplierA', category_id: 'preferred', order: 2 },
      ],
    },
    discountLabels: defaultGroupDiscountLabels,
    translate: (key) => key,
    generatedAt: new Date(2026, 7, 31, 12, 0),
    ...overrides,
  }
}

function assertPatternFill(fill: Fill, argb: string) {
  assert.equal(fill.type, 'pattern')
  if (fill.type !== 'pattern') assert.fail('Expected a pattern fill')
  assert.equal(fill.fgColor?.argb, argb)
}

describe('quotation spreadsheet', () => {
  test('expands each supplier with its description and numeric discount', () => {
    const rows = buildQuotationSpreadsheetRows(options())

    assert.equal(rows.length, 2)
    assert.deepEqual(
      rows.map((row) => [row.supplier, row.discountRatio, row.description]),
      [
        ['supplierB', 0.25, 'Supplier B'],
        ['supplierA', 0.5, 'Supplier A'],
      ]
    )
    assert.equal(rows[0]?.model, 'gpt-test')
    assert.equal(rows[0]?.modality, 'Text → Text')
    assert.notEqual(rows[0]?.inputPrice, '')
  })

  test('keeps the supplier description blank when group information has no description', () => {
    const rows = buildQuotationSpreadsheetRows(
      options({
        usableGroup: {
          supplierA: { ratio: 0.5 },
          supplierB: { desc: '', ratio: 0.25 },
        },
      })
    )

    assert.deepEqual(
      rows.map((row) => row.description),
      [null, null]
    )
  })

  test('builds a reference-styled XLSX workbook with grouped quote rows', async () => {
    const result = await buildQuotationSpreadsheet(options())
    const workbook = new Workbook()
    await workbook.xlsx.load(result.buffer)

    assert.equal(
      result.filename,
      'Modelsell-Enterprise-pricing-quotation-2026-08-31.xlsx'
    )
    assert.equal(result.rowCount, 2)
    assert.equal(result.sheetCount, 1)
    assert.equal(workbook.worksheets.length, 1)

    const quotation = workbook.getWorksheet('Quotation')
    assert.ok(quotation)
    assert.equal(workbook.getWorksheet('Summary'), undefined)
    assert.equal(quotation.getCell('K1').value, 'Billing discount')
    assert.equal(quotation.getCell('L1').value, 'Description')
    assert.equal(quotation.getCell('K2').value, 0.25)
    assert.equal(quotation.getCell('K3').value, 0.5)
    assert.equal(quotation.getCell('K2').numFmt, '0.####')
    assert.equal(quotation.getCell('L2').value, 'Supplier B')
    assert.equal(quotation.getCell('L3').value, 'Supplier A')
    assert.equal(quotation.getCell('M1').value, null)
    assert.equal(quotation.getCell('F1').value, 'Input price')
    assert.equal(quotation.getCell('G1').value, 'Output price')
    assert.equal(quotation.getCell('H1').value, 'Cache write price')
    assert.equal(quotation.getCell('I1').value, 'Cache read price')
    assert.deepEqual(quotation.getCell('F2').value, {
      richText: [
        { font: { bold: true }, text: '$2' },
        { font: { color: { argb: 'FF6B7280' }, size: 9 }, text: '\n/1M' },
      ],
    })
    assert.deepEqual(quotation.getCell('H2').value, {
      richText: [
        {
          font: { color: { argb: 'FF6B7280' }, size: 9 },
          text: '5m ',
        },
        { font: { bold: true }, text: '$2.5' },
        {
          font: { color: { argb: 'FF6B7280' }, size: 9 },
          text: ' /1M',
        },
        { text: '\n' },
        {
          font: { color: { argb: 'FF6B7280' }, size: 9 },
          text: '1h ',
        },
        { font: { bold: true }, text: '$4' },
        {
          font: { color: { argb: 'FF6B7280' }, size: 9 },
          text: ' /1M',
        },
      ],
    })
    assert.equal(quotation.views[0]?.state, 'frozen')
    assert.equal(quotation.views[0]?.xSplit, 3)
    assert.equal(quotation.views[0]?.ySplit, 1)
    assert.equal(quotation.getRow(1).height, 34)
    assert.equal(quotation.getColumn(3).width, 38)
    assert.equal(quotation.getColumn(12).width, 60)
    assertPatternFill(quotation.getCell('A1').fill, 'FF000000')
    assert.equal(quotation.getCell('A1').font.color?.argb, 'FFFFFFFF')
    assert.equal(quotation.getCell('A1').font.size, 12)
    assertPatternFill(quotation.getCell('A2').fill, 'FFF2F2F2')
    assertPatternFill(quotation.getCell('B2').fill, 'FFFAFAFA')
    assertPatternFill(quotation.getCell('C2').fill, 'FFFFFFFF')
    assert.equal(quotation.getCell('A2').border.bottom?.style, 'thin')
    assert.equal(quotation.getCell('A2').isMerged, true)
    assert.equal(quotation.getCell('B2').isMerged, true)
    assert.equal(quotation.getCell('C2').isMerged, true)
    assert.equal(quotation.getCell('C3').master.address, 'C2')
  })
})
