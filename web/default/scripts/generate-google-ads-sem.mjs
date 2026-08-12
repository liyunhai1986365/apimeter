import fs from 'node:fs/promises'
import path from 'node:path'

const DEFAULTS = {
  api: 'https://apimeter.ai/api/pricing',
  campaignId: '24089073282',
  campaign: '搜索｜热门模型 API｜中国',
  adGroup: '广告组 1',
  output: 'apimeter-google-ads-keywords.csv',
}

const vendorAliases = {
  Anthropic: ['Claude'],
  Google: ['Gemini'],
  ByteDance: ['字节跳动', '火山引擎', '豆包'],
  xAI: ['Grok'],
  Kling: ['可灵'],
  Alibaba: ['阿里云百炼', '通义千问'],
  讯飞: ['科大讯飞', '星火'],
  'Z.ai': ['智谱', 'GLM'],
  Tencent: ['腾讯云'],
  Xiaomi: ['小米'],
  Moonshot: ['月之暗面', 'Kimi'],
}

function readArg(name, fallback) {
  const prefix = `--${name}=`
  const value = process.argv.slice(2).find((arg) => arg.startsWith(prefix))
  return value ? value.slice(prefix.length) : fallback
}

function csvCell(value) {
  const text = String(value ?? '')
  return /[",\r\n]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text
}

function keywordRow({ campaignId, campaign, adGroup, keyword, type, url, suffix }) {
  return [
    'Keyword',
    'Add',
    'Enabled',
    campaignId,
    campaign,
    '',
    adGroup,
    '',
    keyword,
    type,
    '',
    '',
    '',
    url,
    '',
    suffix,
    '',
    '',
  ]
}

const api = readArg('api', DEFAULTS.api)
const campaignId = readArg('campaign-id', DEFAULTS.campaignId)
const campaign = readArg('campaign', DEFAULTS.campaign)
const adGroup = readArg('ad-group', DEFAULTS.adGroup)
const output = path.resolve(readArg('output', DEFAULTS.output))

const response = await fetch(api)
if (!response.ok) {
  throw new Error(`Pricing API returned HTTP ${response.status}`)
}

const payload = await response.json()
if (!payload?.success || !Array.isArray(payload.data) || !Array.isArray(payload.vendors)) {
  throw new Error('Pricing API response does not contain model and vendor arrays')
}

const vendorById = new Map(payload.vendors.map((vendor) => [vendor.id, vendor.name]))
const usedVendors = new Set()
const rows = []
const seen = new Set()

const addKeyword = ({ keyword, url, content }) => {
  const normalized = keyword.trim().toLowerCase()
  if (!normalized || seen.has(normalized)) return
  seen.add(normalized)

  const suffix = [
    'sem=1',
    'utm_source=google',
    'utm_medium=cpc',
    'utm_campaign=model_api_catalog',
    `utm_content=${content}`,
    'utm_term={keyword}',
    'campaign_id={campaignid}',
    'adgroup_id={adgroupid}',
    'creative_id={creative}',
    'match_type={matchtype}',
    'network={network}',
    'device={device}',
  ].join('&')

  for (const type of ['Exact', 'Phrase']) {
    rows.push(
      keywordRow({
        campaignId,
        campaign,
        adGroup,
        keyword,
        type,
        url,
        suffix,
      })
    )
  }
}

for (const model of payload.data) {
  if (!model?.model_name) continue
  const vendorName = vendorById.get(model.vendor_id)
  if (vendorName) usedVendors.add(vendorName)
  addKeyword({
    keyword: `${model.model_name} api`,
    url: `https://apimeter.ai/pricing/${encodeURIComponent(model.model_name)}`,
    content: 'model',
  })
}

for (const vendor of [...usedVendors].sort((a, b) => a.localeCompare(b))) {
  const names = [vendor, ...(vendorAliases[vendor] ?? [])]
  for (const name of names) {
    addKeyword({
      keyword: `${name} api`,
      url: `https://apimeter.ai/pricing?vendor=${encodeURIComponent(vendor)}`,
      content: 'vendor',
    })
  }
}

const header = [
  'Row Type',
  'Action',
  'Keyword status',
  'Campaign ID',
  'Campaign',
  'Ad group ID',
  'Ad group',
  'Keyword ID',
  'Keyword',
  'Type',
  'Label',
  'Default max. CPC',
  'Max. CPV',
  'Final URL',
  'Mobile final URL',
  'Final URL suffix',
  'Tracking template',
  'Custom parameter',
]

const csv = [header, ...rows].map((row) => row.map(csvCell).join(',')).join('\r\n')
await fs.writeFile(output, `\uFEFF${csv}\r\n`)

console.log(
  JSON.stringify(
    {
      output,
      models: payload.data.length,
      vendors: usedVendors.size,
      uniqueKeywordTexts: seen.size,
      rows: rows.length,
    },
    null,
    2
  )
)
