import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as reviewApi from '../api/safeguard-review'
import type { CompleteReviewInput, OpenReviewInput, SafeguardReview } from '../types/safeguard-review'

export const useSafeguardReviewStore = defineStore('safeguard-reviews', () => {
  // 按保护层 ID 缓存历次记录（含未办结任务，时间倒序）。
  const historyBySafeguard = ref<Record<number, SafeguardReview[]>>({})
  const openItems = ref<SafeguardReview[]>([])
  const openTotal = ref(0)
  const loadingHistory = ref(false)
  const loadingOpen = ref(false)

  async function loadHistory(safeguardId: number) {
    loadingHistory.value = true
    try {
      const result = await reviewApi.listSafeguardReviews(safeguardId)
      historyBySafeguard.value[safeguardId] = result.items
      return result.items
    } finally {
      loadingHistory.value = false
    }
  }
  async function loadOpen(params: { safeguardId?: number; overdueOnly?: boolean } = {}) {
    loadingOpen.value = true
    try {
      const result = await reviewApi.listOpenReviews({
        safeguard_id: params.safeguardId,
        overdue_only: params.overdueOnly,
        page_size: 100,
      })
      openItems.value = result.items
      openTotal.value = result.total
      return result
    } finally {
      loadingOpen.value = false
    }
  }
  async function openReview(safeguardId: number, input: OpenReviewInput) {
    const item = await reviewApi.openSafeguardReview(safeguardId, input)
    await loadHistory(safeguardId)
    return item
  }
  async function completeReview(safeguardId: number, reviewId: number, input: CompleteReviewInput) {
    const item = await reviewApi.completeSafeguardReview(safeguardId, reviewId, input)
    await loadHistory(safeguardId)
    return item
  }
  function history(safeguardId: number) {
    return historyBySafeguard.value[safeguardId] ?? []
  }

  return { historyBySafeguard, openItems, openTotal, loadingHistory, loadingOpen, loadHistory, loadOpen, openReview, completeReview, history }
})
