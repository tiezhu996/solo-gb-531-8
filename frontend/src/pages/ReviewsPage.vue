<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { ClipboardList, RefreshCw } from 'lucide-vue-next'
import AppShell from '../components/common/AppShell.vue'
import PageHeader from '../components/common/PageHeader.vue'
import ReviewDrawer from '../components/common/ReviewDrawer.vue'
import { useAuth } from '../hooks/useAuth'
import { useSafeguardReviewStore } from '../stores/safeguard-review'
import { getSafeguard } from '../api/safeguard'
import { errorMessage } from '../api/client'
import type { Safeguard } from '../types/safeguard'
import type { SafeguardReview } from '../types/safeguard-review'

const store = useSafeguardReviewStore()
const { canEdit, canReview } = useAuth()
const canWriteReview = computed(() => canEdit.value || canReview.value)
const overdueOnly = ref(false)
const drawer = ref(false)
const target = ref<Safeguard>()

const items = computed(() => store.openItems)
const overdueCount = computed(() => items.value.filter((item) => item.overdue).length)

async function refresh() {
  try { await store.loadOpen({ overdueOnly: overdueOnly.value }) } catch (error) { ElMessage.error(errorMessage(error)) }
}
async function openReview(item: SafeguardReview) {
  try {
    target.value = await getSafeguard(item.safeguard_id)
    drawer.value = true
  } catch (error) { ElMessage.error(errorMessage(error)) }
}
function fmt(value?: string) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—' }
const originLabels: Record<string, string> = { manual: '人工发起', expiry: '到期触发', failure: '不合格跟进' }
onMounted(refresh)
</script>

<template>
  <AppShell>
    <PageHeader eyebrow="RE-REVIEW TASKS" title="待复评任务" description="同一保护层同时只有一项未完成复评；逾期任务优先处理，办结后合格恢复可用，不合格自动生成跟进任务。">
      <el-button :loading="store.loadingOpen" @click="refresh"><RefreshCw :size="16" />刷新</el-button>
    </PageHeader>
    <section class="filter-bar">
      <div>
        <ClipboardList :size="16" />
        <el-switch v-model="overdueOnly" active-text="仅看逾期" inline-prompt @change="refresh" />
      </div>
      <span>{{ store.openTotal }} 项待复评 · {{ overdueCount }} 项已逾期</span>
    </section>
    <section class="data-section">
      <el-table v-loading="store.loadingOpen" :data="items" row-key="id" empty-text="没有待办复评">
        <el-table-column label="保护层" min-width="220">
          <template #default="{ row }">
            <div class="primary-cell">
              <strong>{{ row.safeguard?.name ?? `保护层 #${row.safeguard_id}` }}</strong>
              <span>#{{ row.safeguard_id }} · {{ originLabels[row.origin as string] ?? row.origin }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="复评原因" min-width="260"><template #default="{ row }"><span>{{ row.reason }}</span></template></el-table-column>
        <el-table-column label="到期时间" min-width="180">
          <template #default="{ row }">
            <span class="date-cell" :class="{ expired: row.overdue }">{{ fmt(row.due_at) }}</span>
            <small v-if="row.overdue" class="cell-note">已逾期，请尽快办结</small>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="120">
          <template #default="{ row }"><span class="state-label" :class="row.overdue ? 'failed' : 'analyzed'">{{ row.overdue ? '已逾期' : '待复评' }}</span></template>
        </el-table-column>
        <el-table-column label="发起人 / 时间" min-width="200">
          <template #default="{ row }"><span>{{ row.opened_by_name }}</span><small class="cell-note">{{ fmt(row.opened_at) }}</small></template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }"><el-button type="primary" size="small" @click="openReview(row)">办理复评</el-button></template>
        </el-table-column>
      </el-table>
    </section>
    <ReviewDrawer v-model="drawer" :safeguard="target" :writable="canWriteReview" @changed="refresh" />
  </AppShell>
</template>
