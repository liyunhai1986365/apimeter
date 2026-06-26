import { taskActionMapper } from './mappers'
import { formatTimestampToDate } from '@/lib/format'
import { TASK_ACTIONS, TASK_STATUS } from '../constants'
import type { TaskLog, TaskLogProperties } from '../types'

type TranslateFn = (value: string) => string

function parseProperties(properties: TaskLog['properties']): TaskLogProperties {
  if (!properties) return {}
  if (typeof properties === 'string') {
    try {
      const parsed: unknown = JSON.parse(properties)
      return parsed && typeof parsed === 'object'
        ? (parsed as TaskLogProperties)
        : {}
    } catch {
      return {}
    }
  }
  return properties
}

function normalizeModelName(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function isImageTaskModel(modelName: string): boolean {
  const normalized = modelName.toLowerCase()
  if (/^gpt-image-\d+/.test(normalized)) return true
  return /^gemini-[\w.-]+-image(?:-[\w.-]+)?$/.test(normalized)
}

function isVideoTaskAction(action: string): boolean {
  return (
    action === TASK_ACTIONS.GENERATE ||
    action === TASK_ACTIONS.TEXT_GENERATE ||
    action === TASK_ACTIONS.FIRST_TAIL_GENERATE ||
    action === TASK_ACTIONS.REFERENCE_GENERATE ||
    action === TASK_ACTIONS.REMIX_GENERATE
  )
}

function normalizePreviewUrl(value: unknown): string {
  if (typeof value !== 'string') return ''
  const url = value.trim()
  if (!url) return ''
  if (url.startsWith('http://') || url.startsWith('https://')) return url
  if (url.startsWith('/')) return url
  if (url.startsWith('data:video/')) return url
  if (url.startsWith('data:image/')) return url
  return ''
}

function parseRecordData(data: TaskLog['data']): Record<string, unknown> {
  if (!data) return {}
  if (typeof data === 'string') {
    try {
      const parsed: unknown = JSON.parse(data)
      return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
        ? (parsed as Record<string, unknown>)
        : {}
    } catch {
      return {}
    }
  }
  return {}
}

function getAtPath(source: unknown, path: string): unknown {
  return path.split('.').reduce<unknown>((current, segment) => {
    if (current == null) return undefined
    if (Array.isArray(current)) {
      const index = Number(segment)
      return Number.isInteger(index) ? current[index] : undefined
    }
    if (typeof current !== 'object') return undefined
    return (current as Record<string, unknown>)[segment]
  }, source)
}

const VIDEO_PREVIEW_URL_PATHS = [
  'video.url',
  'video_url',
  'url',
  'output.video_url',
  'output.url',
  'output.video.url',
  'output.results.0.url',
  'output.videos.0.url',
  'data.video_url',
  'data.url',
  'data.video.url',
  'data.result.url',
  'data.results.0.url',
  'data.videos.0.url',
  'data.task_result.videos.0.url',
  'result.video_url',
  'result.url',
  'result.video.url',
  'result.videos.0.url',
] as const

const IMAGE_PREVIEW_URL_PATHS = [
  'image.url',
  'image_url',
  'url',
  'output.image_url',
  'output.url',
  'output.image.url',
  'output.results.0.url',
  'output.images.0.url',
  'data.image_url',
  'data.url',
  'data.image.url',
  'data.images.0.url',
  'data.result.url',
  'data.resultUrls.0',
  'data.results.0.url',
  'data.task_result.images.0.url',
  'result.image_url',
  'result.url',
  'result.image.url',
  'result.images.0.url',
  'resultUrls.0',
] as const

export function getTaskLogImageModelName(log: TaskLog): string {
  const properties = parseProperties(log.properties)
  const originModelName = normalizeModelName(properties.origin_model_name)
  const upstreamModelName = normalizeModelName(properties.upstream_model_name)

  if (isImageTaskModel(originModelName)) return originModelName
  if (isImageTaskModel(upstreamModelName)) return upstreamModelName
  return ''
}

export function buildTaskLogSubtitle(log: TaskLog, t: TranslateFn): string {
  const imageModelName = getTaskLogImageModelName(log)
  if (imageModelName) return imageModelName

  return `${t(log.platform)} · ${t(taskActionMapper.getLabel(log.action))}`
}

export function formatDrawingSubmitTime(submitTime?: number): string {
  return formatTimestampToDate(submitTime, 'milliseconds')
}

export function getTaskLogVideoPreviewUrl(log: TaskLog): string {
  if (getTaskLogImageModelName(log)) {
    return ''
  }
  if (log.status !== TASK_STATUS.SUCCESS || !isVideoTaskAction(log.action)) {
    return ''
  }

  const resultUrl = normalizePreviewUrl(log.result_url)
  if (resultUrl) return resultUrl

  const data = parseRecordData(log.data)
  for (const path of VIDEO_PREVIEW_URL_PATHS) {
    const url = normalizePreviewUrl(getAtPath(data, path))
    if (url) return url
  }

  if (normalizePreviewUrl(log.fail_reason) && log.task_id) {
    return `/v1/videos/${encodeURIComponent(log.task_id)}/content`
  }

  return ''
}

export function getTaskLogImagePreviewUrl(log: TaskLog): string {
  if (!getTaskLogImageModelName(log)) {
    return ''
  }
  if (log.status !== TASK_STATUS.SUCCESS) {
    return ''
  }

  const resultUrl = normalizePreviewUrl(log.result_url)
  if (resultUrl) return resultUrl

  const data = parseRecordData(log.data)
  for (const path of IMAGE_PREVIEW_URL_PATHS) {
    const url = normalizePreviewUrl(getAtPath(data, path))
    if (url) return url
  }

  return ''
}
