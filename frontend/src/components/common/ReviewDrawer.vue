<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { ClipboardList, X } from 'lucide-vue-next'
import { errorMessage } from '../../api/client'
import { useSafeguardReviewStore } from '../../stores/safeguard-review'
import type { Safeguard } from '../../types/safeguard'

type SafeguardContext = Pick<Safeguard, 'id' | 'name' | 'test_interval_days' | 'verification_expires_at' | 'verification_expired' | 'latest_conclusion' | 'latest_checked_at' | 'completed_review_count'>

const props = defineProps<{ modelValue: boolean; safeguard?: SafeguardContext; writable: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; changed: [] }>()

const store = useSafeguardReviewStore()
const open = ref(props.modelValue)
watch(() => props.modelValue, (value) => { open.value = value })
watch(open, (value) => emit('update:modelValue', value))

const records = computed(() => (props.safeguard ? store.history(props.safeguard.id) : []))
const openReview = computed(() => records.value.find((item) => item.status === 'open'))
const loading = computed(() => store.loadingHistory)

watch(() => [props.modelValue, props.safeguard?.id] as const, async ([visible, id]) => {
  if (visible && id) {
    try { await store.loadHistory(id) } catch (error) { ElMessage.error(errorMessage(error)) }
  } else {
    showOpenForm.value = false
    showCompleteForm.value = false
  }
})

const showOpenForm = ref(false)
const showCompleteForm = ref(false)
const submitting = ref(false)
const openForm = ref({ reason: '', due_at: defaultDue(30) })
const completeForm = ref({
  checked_at: new Date().toISOString(),
  conclusion: 'pass' as 'pass' | 'fail',
  evidence: '',
  responsible: '',
  next_due_at: defaultDue(365),
})

function defaultDue(days: number) {
  const date = new Date()
  date.setUTCDate(date.getUTCDate() + days)
  return date.toISOString()
}
function resetOpenForm() { openForm.value = { reason: '', due_at: defaultDue(30) } }
function resetCompleteForm(conclusion: 'pass' | 'fail' = 'pass') {
  completeForm.value = { checked_at: new Date().toISOString(), conclusion, evidence: '', responsible: '', next_due_at: defaultDue(conclusion === 'pass' ? (props.safeguard?.test_interval_days ?? 365) : 30) }
}
async function submitOpen() {
  if (!props.safeguard || !openForm.value.reason.trim() || !openForm.value.due_at) {
    ElMessage.warning('请填写复评原因和到期时间'); return
  }
  submitting.value = true
  try {
    await store.openReview(props.safeguard.id, { reason: openForm.value.reason.trim(), due_at: openForm.value.due_at })
    ElMessage.success('复评任务已登记')
    showOpenForm.value = false
    resetOpenForm()
    emit('changed')
  } catch (error) { ElMessage.error(errorMessage(error)) } finally { submitting.value = false }
}
async function submitComplete() {
  if (!props.safeguard || !openReview.value) return
  const form = completeForm.value
  if (!form.evidence.trim() || !form.responsible.trim() || !form.checked_at || !form.next_due_at) {
    ElMessage.warning('请完整填写校验时间、结论、证据、责任人和下次到期时间'); return
  }
  submitting.value = true
  try {
    await store.completeReview(props.safeguard.id, openReview.value.id, {
      checked_at: form.checked_at, conclusion: form.conclusion, evidence: form.evidence.trim(),
      responsible: form.responsible.trim(), next_due_at: form.next_due_at,
    })
    ElMessage.success(form.conclusion === 'pass' ? '复评合格，台账有效期已更新' : '复评不合格，保护层保持不可用并生成跟进任务')
    showCompleteForm.value = false
    emit('changed')
  } catch (error) { ElMessage.error(errorMessage(error)) } finally { submitting.value = false }
}
function startComplete() { resetCompleteForm('pass'); showCompleteForm.value = true }
function fmt(value?: string) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—' }
function day(value?: string) { return value ? new Date(value).toLocaleDateString('zh-CN') : '—' }
const originLabels: Record<string, string> = { manual: '人工发起', expiry: '到期触发', failure: '不合格跟进' }
</script>

<template>
  <el-drawer v-model="open" size="min(620px, 94vw)" direction="rtl" :with-header="false" class="review-drawer">
    <header class="drawer-header">
      <div><ClipboardList :size="19" /><div><p class="eyebrow">RE-REVIEW LOOP</p><h2>{{ safeguard?.name }} · 复评记录</h2></div></div>
      <el-tooltip content="关闭"><el-button circle text aria-label="关闭" @click="open = false"><X :size="18" /></el-button></el-tooltip>
    </header>

    <div v-if="safeguard" class="review-summary">
      <div><span>台账有效期至</span><strong :class="{ expired: safeguard.verification_expired }">{{ day(safeguard.verification_expires_at) }}</strong></div>
      <div><span>最近结论</span>
        <strong v-if="safeguard.latest_conclusion" :class="safeguard.latest_conclusion === 'pass' ? 'conclusion-pass' : 'conclusion-fail'">
          {{ safeguard.latest_conclusion === 'pass' ? '合格' : '不合格' }} · {{ day(safeguard.latest_checked_at) }}
        </strong>
        <strong v-else class="muted">尚无校验记录</strong>
      </div>
      <div><span>历史校验</span><strong>{{ safeguard.completed_review_count ?? 0 }} 次</strong></div>
    </div>

    <section v-if="openReview" class="open-review-card" :class="{ overdue: openReview.overdue }">
      <header>
        <div class="open-title">
          <span class="state-label" :class="openReview.overdue ? 'expired' : 'analyzed'">{{ openReview.overdue ? '已逾期' : '待复评' }}</span>
          <strong>复评 #{{ openReview.sequence }}</strong>
          <small>{{ originLabels[openReview.origin] }}</small>
        </div>
        <small>到期：{{ fmt(openReview.due_at) }}</small>
      </header>
      <p>{{ openReview.reason }}</p>
      <small class="muted">{{ openReview.opened_by_name }} 于 {{ fmt(openReview.opened_at) }} 发起</small>
      <div v-if="writable && !showCompleteForm" class="card-actions">
        <el-button type="primary" size="small" @click="startComplete">办结复评（登记本次校验）</el-button>
      </div>
    </section>

    <section v-else-if="writable && !showOpenForm" class="card-actions">
      <el-button type="primary" plain size="small" @click="showOpenForm = true">新增复评任务</el-button>
    </section>

    <el-form v-if="showOpenForm" label-position="top" class="review-form" @submit.prevent="submitOpen">
      <h3>新增复评任务</h3>
      <el-form-item label="复评原因" required>
        <el-input v-model="openForm.reason" type="textarea" :rows="2" maxlength="1000" show-word-limit placeholder="例如：年度周期校验到期，需重新做动作试验" />
      </el-form-item>
      <el-form-item label="到期时间" required>
        <el-date-picker v-model="openForm.due_at" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" />
      </el-form-item>
      <div class="form-buttons">
        <el-button @click="showOpenForm = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitOpen">登记任务</el-button>
      </div>
    </el-form>

    <el-form v-if="showCompleteForm" label-position="top" class="review-form" @submit.prevent="submitComplete">
      <h3>办结复评 · 登记本次校验</h3>
      <div class="form-grid two">
        <el-form-item label="校验时间" required>
          <el-date-picker v-model="completeForm.checked_at" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" />
        </el-form-item>
        <el-form-item label="校验结论" required>
          <el-radio-group v-model="completeForm.conclusion">
            <el-radio-button label="pass">合格</el-radio-button>
            <el-radio-button label="fail">不合格</el-radio-button>
          </el-radio-group>
        </el-form-item>
      </div>
      <el-form-item label="证据说明（报告编号、试验结果）" required>
        <el-input v-model="completeForm.evidence" type="textarea" :rows="3" maxlength="4000" show-word-limit placeholder="例如：动作试验合格，报告 PT-2026-018" />
      </el-form-item>
      <div class="form-grid two">
        <el-form-item label="责任人" required>
          <el-input v-model="completeForm.responsible" maxlength="120" placeholder="本次校验责任人" />
        </el-form-item>
        <el-form-item label="下次到期时间" required>
          <el-date-picker v-model="completeForm.next_due_at" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" />
        </el-form-item>
      </div>
      <p class="form-hint">合格：按下次到期时间更新台账有效期并恢复可用；不合格：保护层保持不可用，并自动生成跟进复评任务。下次到期不得晚于台账试验间隔（{{ safeguard?.test_interval_days }} 天）。</p>
      <div class="form-buttons">
        <el-button @click="showCompleteForm = false">取消</el-button>
        <el-button :type="completeForm.conclusion === 'pass' ? 'primary' : 'danger'" :loading="submitting" @click="submitComplete">确认办结</el-button>
      </div>
    </el-form>

    <h3 class="timeline-heading">历次校验与复评（{{ records.length }}）</h3>
    <div v-loading="loading" class="review-timeline">
      <p v-if="!records.length" class="muted">暂无复评记录。</p>
      <article v-for="record in records" :key="record.id" class="timeline-item" :class="[record.status, record.conclusion ?? '']">
        <span class="timeline-dot" />
        <div class="timeline-body">
          <header>
            <strong>#{{ record.sequence }}</strong>
            <span v-if="record.status === 'completed'" class="state-label" :class="record.conclusion === 'pass' ? 'completed' : 'failed'">{{ record.conclusion === 'pass' ? '合格' : '不合格' }}</span>
            <span v-else class="state-label" :class="record.overdue ? 'failed' : 'analyzed'">{{ record.overdue ? '逾期未办结' : '待复评' }}</span>
            <small>{{ originLabels[record.origin] }}</small>
          </header>
          <p v-if="record.status === 'open'">{{ record.reason }}</p>
          <template v-else>
            <p class="timeline-evidence">{{ record.evidence }}</p>
            <dl class="timeline-meta">
              <div><dt>校验时间</dt><dd>{{ fmt(record.checked_at) }}</dd></div>
              <div><dt>责任人</dt><dd>{{ record.responsible }}</dd></div>
              <div><dt>下次到期</dt><dd :class="{ 'date-expired': record.next_due_at && new Date(record.next_due_at).getTime() < Date.now() }">{{ fmt(record.next_due_at) }}</dd></div>
              <div><dt>办结人</dt><dd>{{ record.closed_by_name }}</dd></div>
            </dl>
          </template>
          <small class="muted" v-if="record.status === 'open'">到期 {{ fmt(record.due_at) }} · {{ record.opened_by_name }} 发起</small>
        </div>
      </article>
    </div>
    <footer>复评记录一经办结即为不可覆盖的历史证据，仅用于离线评审与审计追溯。</footer>
  </el-drawer>
</template>
