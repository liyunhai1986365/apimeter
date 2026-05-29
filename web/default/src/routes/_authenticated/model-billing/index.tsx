import { createFileRoute } from '@tanstack/react-router'
import { ModelBilling } from '@/features/model-billing'

export const Route = createFileRoute('/_authenticated/model-billing/')({
  component: ModelBilling,
})
