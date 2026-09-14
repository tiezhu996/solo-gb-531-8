export type SafeguardType = 'alarm' | 'interlock' | 'relief' | 'procedural' | 'containment' | 'detection'
export type SafeguardLifecycle = 'pending' | 'active' | 'expired' | 'invalid'

export interface Safeguard {
  id: number
  name: string
  safeguard_type: SafeguardType
  target_scenario_id: number
  independence_key: string
  effectiveness: number
  test_interval_days: number
  last_verified_at: string | null
  lifecycle_state: SafeguardLifecycle
  evidence_note: string
  verification_expires_at?: string
  verification_expired?: boolean
  target_scenario?: { id: number; parameter: string; scenario_state: string }
  // 复评闭环投影
  open_review_id?: number
  open_review_due_at?: string
  open_review_overdue?: boolean
  review_pending?: boolean
  latest_review_id?: number
  latest_conclusion?: 'pass' | 'fail'
  latest_checked_at?: string
  latest_next_due_at?: string
  latest_responsible?: string
  latest_evidence?: string
  completed_review_count?: number
}

export interface SafeguardInput {
  name: string
  safeguard_type: SafeguardType
  target_scenario_id: number
  independence_key: string
  effectiveness: number
  test_interval_days: number
  last_verified_at: string | null
  evidence_note: string
}
