import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/model-billing/')({
  beforeLoad: () => {
    throw redirect({
    to: '/billing/$section',
    params: { section: 'monthly' },
  })
  },
})
