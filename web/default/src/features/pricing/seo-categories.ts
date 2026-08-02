/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { MODEL_CATEGORIES } from './constants'
import type { ModelCategory } from './types'

export type SEOCategoryDefinition = {
  slug: ModelCategory
  titleKey: string
  descriptionKey: string
}

export const SEO_CATEGORIES: SEOCategoryDefinition[] = [
  {
    slug: MODEL_CATEGORIES.TEXT,
    titleKey: 'Text & Language model APIs',
    descriptionKey:
      'Chat, reasoning, coding and multimodal language models for assistants, agents and production applications.',
  },
  {
    slug: MODEL_CATEGORIES.VECTOR,
    titleKey: 'Embedding & Rerank model APIs',
    descriptionKey:
      'Embedding and reranking models for semantic search, retrieval, recommendations and RAG pipelines.',
  },
  {
    slug: MODEL_CATEGORIES.IMAGE,
    titleKey: 'Image Generation model APIs',
    descriptionKey:
      'Image generation and editing models for creative workflows, product experiences and automated content production.',
  },
  {
    slug: MODEL_CATEGORIES.AUDIO,
    titleKey: 'Audio & Speech model APIs',
    descriptionKey:
      'Speech recognition, text-to-speech and audio models for transcription, voice and real-time experiences.',
  },
  {
    slug: MODEL_CATEGORIES.VIDEO,
    titleKey: 'Video Generation model APIs',
    descriptionKey:
      'Video generation and editing models for text-to-video, image-to-video and automated media workflows.',
  },
  {
    slug: MODEL_CATEGORIES.OTHER,
    titleKey: 'Specialized AI model APIs',
    descriptionKey:
      'Specialized AI model APIs and utilities that extend beyond the main text, vector, image, audio and video categories.',
  },
]

export function getSEOCategory(
  slug: string
): SEOCategoryDefinition | undefined {
  return SEO_CATEGORIES.find((category) => category.slug === slug)
}

export function isSEOCategory(slug: string): slug is ModelCategory {
  return Boolean(getSEOCategory(slug))
}
