import { api, json, query } from './client'
import type { PageData } from '../types/common'
import type { CompleteReviewInput, OpenReviewInput, SafeguardReview } from '../types/safeguard-review'

export function listSafeguardReviews(safeguardId: number): Promise<{ items: SafeguardReview[]; total: number }> {
  return api(`/safeguards/${safeguardId}/reviews`)
}
export function listOpenReviews(params: { safeguard_id?: number; overdue_only?: boolean; page?: number; page_size?: number } = {}): Promise<PageData<SafeguardReview>> {
  return api<PageData<SafeguardReview>>(`/safeguard-reviews${query({
    safeguard_id: params.safeguard_id,
    overdue_only: params.overdue_only === undefined ? undefined : String(params.overdue_only),
    page: params.page,
    page_size: params.page_size,
  })}`)
}
export const getSafeguardReview = (safeguardId: number, reviewId: number) =>
  api<SafeguardReview>(`/safeguards/${safeguardId}/reviews/${reviewId}`)
export const openSafeguardReview = (safeguardId: number, input: OpenReviewInput) =>
  api<SafeguardReview>(`/safeguards/${safeguardId}/reviews`, json('POST', input))
export const completeSafeguardReview = (safeguardId: number, reviewId: number, input: CompleteReviewInput) =>
  api<SafeguardReview>(`/safeguards/${safeguardId}/reviews/${reviewId}/complete`, json('POST', input))
