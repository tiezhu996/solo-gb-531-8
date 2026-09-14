export type SafeguardReviewStatus = 'open' | 'completed'
export type SafeguardReviewConclusion = 'pass' | 'fail'
export type SafeguardReviewOrigin = 'manual' | 'expiry' | 'failure'

export interface SafeguardReview {
  id: number
  safeguard_id: number
  sequence: number
  status: SafeguardReviewStatus
  origin: SafeguardReviewOrigin
  reason: string
  due_at?: string
  parent_review_id?: number
  opened_at: string
  opened_by: number
  opened_by_name: string
  completed_at?: string
  checked_at?: string
  conclusion?: SafeguardReviewConclusion
  evidence?: string
  responsible?: string
  responsible_id?: number
  next_due_at?: string
  closed_by?: number
  closed_by_name?: string
  overdue: boolean
  created_at: string
  safeguard?: {
    id: number
    name: string
    safeguard_type: string
    lifecycle_state: string
    test_interval_days: number
  }
}

export interface OpenReviewInput {
  reason: string
  due_at: string
}

export interface CompleteReviewInput {
  checked_at: string
  conclusion: SafeguardReviewConclusion
  evidence: string
  responsible: string
  next_due_at: string
}
