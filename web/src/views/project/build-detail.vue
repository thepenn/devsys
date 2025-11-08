<template>
  <div v-loading="loading" class="build-detail">
    <header v-if="detail && detail.pipeline" class="build-detail__header">
      <div class="build-detail__meta">
        <h2>构建 #{{ runNumber }}</h2>
        <p class="build-detail__meta-line">
          <span>分支：{{ detail.pipeline.branch || '—' }}</span>
          <span>Commit：{{ detail.pipeline.commit || '—' }}</span>
          <span>触发时间：{{ formatTime(detail.pipeline.created) || '—' }}</span>
          <span>耗时：{{ formatDuration(detail.pipeline.created, detail.pipeline.finished) }}</span>
        </p>
      </div>
      <div class="build-detail__meta-actions">
        <span :class="['pipeline-status', `pipeline-status--${statusClass}`]">
          {{ statusLabel }}
        </span>
        <button class="button button--ghost build-detail__meta-back" @click="goBack">返回流水线</button>
      </div>
    </header>
    <header v-else class="build-detail__header build-detail__header--empty">
      <div class="build-detail__meta">
        <button class="button button--ghost build-detail__meta-back" @click="goBack">返回流水线</button>
        <h2>构建详情</h2>
        <p class="build-detail__meta-line">正在加载构建信息…</p>
      </div>
    </header>

    <section class="panel build-detail__steps">
      <header class="build-detail__steps-header">
        <button
          v-if="canCancel"
          class="button button--ghost"
          :disabled="canceling"
          @click="cancelRun"
        >
          {{ canceling ? '取消中…' : '取消构建' }}
        </button>
      </header>

      <div v-if="flatSteps.length" class="build-detail__flow">
        <template v-for="(step, idx) in flatSteps">
          <div :key="`step-card-${stepKey(step)}`" :class="['build-detail__flow-step', { 'build-detail__flow-step--active': currentStepId === stepKey(step) }]">
            <span :class="['pipeline-status-bullet', stepBulletClass(step)]" />
            <div class="build-detail__flow-info">
              <span class="build-detail__flow-name">{{ step.name || `Step #${step.pid}` }}</span>
              <span class="build-detail__flow-meta">{{ stepStatusLabel(step) }} · {{ formatDuration(step.started, step.finished) }}</span>
            </div>
          </div>
          <span
            v-if="idx < flatSteps.length - 1"
            :key="`arrow-${stepKey(step)}`"
            class="build-detail__flow-arrow"
          >
            →
          </span>
        </template>
      </div>

      <div class="build-detail__layout">
        <aside class="build-detail__sidebar">
          <template v-if="flatSteps.length">
            <ul class="build-detail__step-list">
              <li
                v-for="step in flatSteps"
                :key="stepKey(step)"
                :class="['build-detail__step-item', { 'build-detail__step-item--active': currentStepId === stepKey(step) }]"
                @click="selectStep(step)"
              >
                <span :class="['pipeline-status-bullet', stepBulletClass(step)]" />
                <span class="build-detail__step-name">{{ step.name || `Step #${step.pid}` }}</span>
              </li>
            </ul>
          </template>
          <div v-else class="build-detail__steps-empty">暂无步骤</div>
        </aside>

        <div class="build-detail__logs">
          <div v-if="!currentStep" class="build-detail__logs-empty">
            请选择左侧的步骤查看日志。
          </div>
          <template v-else-if="currentStepIsApproval && stepHasRun(currentStep)">
            <header class="build-detail__logs-header">
              <div>
                <h4>{{ currentStep.name || `Step #${currentStep.pid}` }}</h4>
                <p class="build-detail__logs-meta">
                  <span>状态：{{ formatApprovalState(currentStepApproval && currentStepApproval.state) }}</span>
                  <span v-if="currentStepApproval && currentStepApproval.expires_at && currentStepApproval.state === 'pending'">超时：{{ formatTime(currentStepApproval.expires_at) }}</span>
                </p>
              </div>
            </header>
            <div class="build-detail__approval-body">
              <p class="build-detail__approval-message">
                {{ (currentStepApproval && currentStepApproval.message) || '等待审批' }}
              </p>
              <div class="build-detail__approval-meta">
                <span>审批策略：{{ formatApprovalStrategy(currentStepApproval && currentStepApproval.strategy) }}</span>
                <span v-if="approvalPendingApprovers.length">剩余审批人：{{ approvalPendingApprovers.join(', ') }}</span>
                <span v-else-if="currentStepApproval && currentStepApproval.approvers && currentStepApproval.approvers.length">审批人：{{ currentStepApproval.approvers.join(', ') }}</span>
              </div>
              <!-- eslint-disable-next-line vue/html-self-closing -->
              <textarea
                v-if="currentStepApproval && (currentStepApproval.can_approve || currentStepApproval.can_reject)"
                v-model="approvalComment"
                class="build-detail__approval-comment"
                placeholder="填写审批备注（可选）"
              >
              </textarea>
              <div
                v-if="currentStepApproval && (currentStepApproval.can_approve || currentStepApproval.can_reject)"
                class="build-detail__approval-actions"
              >
                <button
                  v-if="currentStepApproval.can_approve"
                  class="approval-button approval-button--approve"
                  :disabled="approvalSubmitting === 'approve'"
                  @click="submitApproval('approve')"
                >{{ approvalSubmitting === 'approve' ? '处理中…' : '同意' }}</button>
                <button
                  v-if="currentStepApproval.can_reject"
                  class="approval-button approval-button--reject"
                  :disabled="approvalSubmitting === 'reject'"
                  @click="submitApproval('reject')"
                >{{ approvalSubmitting === 'reject' ? '处理中…' : '拒绝' }}</button>
              </div>
              <div v-if="approvalDecisions.length" class="build-detail__approval-history">
                <h5>审批记录</h5>
                <ul>
                  <li v-for="record in approvalDecisions" :key="`${record.user}-${record.timestamp}`">
                    <span class="build-detail__approval-user">{{ record.user }}</span>
                    <span :class="['build-detail__approval-action', approvalActionClass(record.action)]">{{ formatApprovalAction(record.action) }}</span>
                    <span class="build-detail__approval-time">{{ formatTime(record.timestamp) }}</span>
                    <span v-if="record.comment" class="build-detail__approval-comment-text">{{ record.comment }}</span>
                  </li>
                </ul>
              </div>
            </div>
          </template>
          <template v-else-if="currentStepIsApproval">
            <header class="build-detail__logs-header">
              <div>
                <h4>{{ currentStep.name || `Step #${currentStep.pid}` }}</h4>
                <p class="build-detail__logs-meta">
                  <span>状态：未执行</span>
                  <span>耗时：—</span>
                </p>
              </div>
            </header>
            <div class="build-detail__logs-empty">当前审批步骤尚未进入执行阶段。</div>
          </template>
          <template v-else>
            <header class="build-detail__logs-header">
              <div>
                <h4>{{ currentStep.name || `Step #${currentStep.pid}` }}</h4>
                <p class="build-detail__logs-meta">
                  <span>状态：{{ stepStatusLabel(currentStep) }}</span>
                  <span>耗时：{{ formatDuration(currentStep.started, currentStep.finished) }}</span>
                </p>
              </div>
              <div class="build-detail__logs-actions">
                <button class="button button--ghost" @click="downloadLogs(currentStep)">下载日志</button>
              </div>
            </header>

            <pre v-if="currentLogs.length" class="build-detail__log-viewer">
              <code v-for="log in currentLogs" :key="`${currentStepId}-${log.line}`">{{ log.content }}</code>
            </pre>
            <div v-else class="build-detail__logs-empty">暂无日志</div>
          </template>
        </div>
      </div>
    </section>

    <section v-if="error" class="panel build-detail__error">
      <span>{{ error }}</span>
      <button class="button button--ghost" @click="loadDetail">重试</button>
    </section>
  </div>
</template>

<script>
import { listRepositories } from '@/api/project/repos'
import { getPipelineRun, cancelPipelineRun, submitPipelineApproval } from '@/api/project/pipeline'

export default {
  name: 'BuildDetail',
  inject: {
    projectPipelineExpose: {
      default: null
    }
  },
  props: {
    runId: {
      type: [String, Number],
      required: true
    }
  },
  data() {
    return {
      detail: null,
      loading: true,
      canceling: false,
      error: '',
      currentStepId: null,
      pollingTimer: null,
      approvalComment: '',
      approvalSubmitting: ''
    }
  },
  computed: {
    workflows() {
      return (this.detail && this.detail.workflows) || []
    },
    statusClass() {
      if (!this.detail || !this.detail.pipeline) return 'unknown'
      return (this.detail.pipeline.status || '').toLowerCase()
    },
    statusLabel() {
      return this.formatStatus(this.detail && this.detail.pipeline && this.detail.pipeline.status)
    },
    runNumber() {
      return (this.detail && this.detail.pipeline && this.detail.pipeline.number) || '-'
    },
    currentStepIsApproval() {
      return this.currentStep && String(this.currentStep.type || '').toLowerCase() === 'approval'
    },
    currentStepApproval() {
      if (!this.currentStep || !this.currentStep.approval) return null
      return this.currentStep.approval
    },
    approvalDecisions() {
      return (this.currentStepApproval && this.currentStepApproval.decisions) || []
    },
    approvalPendingApprovers() {
      return (this.currentStepApproval && this.currentStepApproval.pending_approvers) || []
    },
    currentStep() {
      if (!this.currentStepId) return null
      const target = String(this.currentStepId)
      for (const wf of this.workflows) {
        const match = (wf.steps || []).find(step => this.stepKey(step) === target)
        if (match) return match
      }
      return null
    },
    currentLogs() {
      if (!this.currentStep) return []
      return this.currentStep.logs || []
    },
    flatSteps() {
      const list = []
      for (const workflow of this.workflows) {
        for (const step of workflow.steps || []) {
          list.push(step)
        }
      }
      return list
    },
    canCancel() {
      if (!this.detail || !this.detail.pipeline) return false
      const status = (this.detail.pipeline.status || '').toLowerCase()
      return status === 'running' || status === 'pending' || status === 'blocked'
    }
  },
  watch: {
    runId: {
      immediate: true,
      handler() {
        this.loadDetail()
      }
    },
    loading() {
      this.notifyExpose()
    },
    canceling() {
      this.notifyExpose()
    }
  },
  mounted() {
    this.notifyExpose()
  },
  beforeDestroy() {
    this.clearPolling()
    this.emitExpose(null)
  },
  methods: {
    emitExpose(payload) {
      this.$emit('pipeline-expose', payload)
      if (typeof this.projectPipelineExpose === 'function') {
        this.projectPipelineExpose(payload)
      }
    },
    notifyExpose() {
      const payload = {
        openRunModal: () => this.navigateWithAction('run', { highlight: this.runId }),
        openEditor: () => this.navigateWithAction('editor', { highlight: this.runId }),
        openSettings: () => this.navigateWithAction('settings', { highlight: this.runId }),
        isBusy: () => this.loading || this.canceling
      }
      this.emitExpose(payload)
    },
    navigateWithAction(action, extraQuery = {}) {
      const { owner, name } = this.$route.params || {}
      if (!owner || !name) {
        return
      }
      const nextQuery = { ...this.$route.query, ...extraQuery }
      if (action) {
        nextQuery.action = action
      }
      const nav = this.$router.push({
        name: 'ProjectPipeline',
        params: { owner, name },
        query: nextQuery
      })
      if (nav && typeof nav.catch === 'function') {
        nav.catch(() => {})
      }
    },
    goBack() {
      const { owner, name } = this.$route.params
      const nav = this.$router.push({
        name: 'ProjectPipeline',
        params: { owner, name },
        query: { highlight: this.runId }
      })
      if (nav && typeof nav.catch === 'function') {
        nav.catch(() => {})
      }
    },
    openRunModal() {
      this.navigateWithAction('run')
    },
    openEditor() {
      this.navigateWithAction('editor')
    },
    openSettings() {
      this.navigateWithAction('settings')
    },
    async ensureProject() {
      const { owner, name } = this.$route.params || {}
      if (!owner || !name) {
        return { id: null, repo: null }
      }
      try {
        const data = await listRepositories({ search: `${owner}/${name}`, per_page: 1, page: 1 })
        const repo = (data.items && data.items[0]) || null
        return repo ? { id: repo.id, repo } : { id: null, repo: null }
      } catch (err) {
        const error = this.normalizeError(err, '加载项目失败')
        this.error = error.message || '加载项目失败'
        return { id: null, repo: null }
      }
    },
    async loadDetail() {
      this.clearPolling()
      this.loading = true
      this.error = ''
      try {
        const context = await this.ensureProject()
        if (!context.id) {
          this.error = '项目数据尚未加载完成'
          return
        }
        const data = await getPipelineRun(context.id, this.runId)
        this.detail = data
        if (!this.currentStepId && data.workflows && data.workflows.length) {
          const firstWorkflow = data.workflows[0]
          if (firstWorkflow.steps && firstWorkflow.steps.length) {
            this.currentStepId = this.stepKey(firstWorkflow.steps[0])
          }
        }
        this.schedulePollingIfNeeded()
      } catch (err) {
        const error = this.normalizeError(err, '加载构建详情失败')
        this.error = error.message || '加载构建详情失败'
      } finally {
        this.loading = false
      }
    },
    schedulePollingIfNeeded() {
      this.clearPolling()
      if (!this.detail || !this.detail.pipeline) {
        return
      }
      const status = (this.detail.pipeline.status || '').toLowerCase()
      if (status === 'running' || status === 'pending' || status === 'blocked') {
        this.pollingTimer = setTimeout(() => this.loadDetail(), 3000)
      }
    },
    clearPolling() {
      if (this.pollingTimer) {
        clearTimeout(this.pollingTimer)
        this.pollingTimer = null
      }
    },
    selectStep(step) {
      if (!step) return
      this.currentStepId = this.stepKey(step)
    },
    async cancelRun() {
      if (!this.canCancel) return
      const context = await this.ensureProject()
      if (!context.id) {
        this.error = '项目数据尚未加载完成'
        return
      }
      this.canceling = true
      try {
        await cancelPipelineRun(context.id, this.detail.pipeline.id)
        await this.loadDetail()
      } catch (err) {
        const error = this.normalizeError(err, '取消构建失败')
        this.error = error.message || '取消构建失败'
      } finally {
        this.canceling = false
      }
    },
    stepKey(step) {
      if (!step) return ''
      const value = step.id || step.pid || ''
      return String(value)
    },

    downloadLogs(step) {
      if (!step) return
      const lines = (step.logs || []).map(log => log.content).join('\n')
      const blob = new Blob([lines], { type: 'text/plain;charset=utf-8' })
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      const key = this.stepKey(step)
      link.download = `${this.$route.params.owner || 'project'}-${this.$route.params.name || 'pipeline'}-step-${key}.log`
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      URL.revokeObjectURL(url)
    },
    normalizeState(value) {
      if (value === null || value === undefined) return ''
      return String(value).trim().toLowerCase()
    },
    stepHasRun(step) {
      if (!step) return false
      return Number(step.started) > 0
    },
    stepVisualState(step) {
      if (!this.stepHasRun(step)) {
        return 'not-run'
      }
      const normalized = this.normalizeState(step && step.state)
      if (normalized) {
        return normalized
      }
      if (step && step.finished) {
        return 'success'
      }
      return 'running'
    },
    stepBulletClass(step) {
      const state = this.stepVisualState(step)
      const classes = {
        [`pipeline-status-bullet--${state}`]: true
      }
      if (state === 'not-run') {
        classes['pipeline-status-bullet--empty'] = true
      }
      return classes
    },
    stepStatusLabel(step) {
      const state = this.stepVisualState(step)
      if (state === 'not-run') {
        return '未执行'
      }
      return this.formatStatus(state)
    },
    formatStatus(value) {
      switch ((value || '').toLowerCase()) {
        case 'success':
          return '成功'
        case 'failure':
        case 'failed':
          return '失败'
        case 'error':
          return '出错'
        case 'killed':
          return '终止'
        case 'canceled':
        case 'cancelled':
          return '已取消'
        case 'running':
          return '运行中'
        case 'pending':
          return '等待'
        case 'blocked':
          return '等待审批'
        case 'skipped':
          return '跳过'
        default:
          return value || '未知'
      }
    },
    formatApprovalState(state) {
      switch ((state || '').toLowerCase()) {
        case 'approved':
          return '已通过'
        case 'rejected':
          return '已拒绝'
        case 'expired':
          return '已超时'
        case 'pending':
        default:
          return '等待审批'
      }
    },
    formatApprovalStrategy(strategy) {
      const normalized = (strategy || '').toLowerCase()
      if (normalized === 'all') return '会签（全部通过）'
      return '或签（任意一人通过）'
    },
    formatApprovalAction(action) {
      const normalized = (action || '').toLowerCase()
      switch (normalized) {
        case 'approve':
        case 'approved':
          return '通过'
        case 'reject':
        case 'rejected':
          return '拒绝'
        case 'expired':
          return '超时'
        default:
          return normalized || '待处理'
      }
    },
    approvalActionClass(action) {
      const normalized = (action || '').toLowerCase()
      if (normalized === 'approve' || normalized === 'approved') return 'approved'
      if (normalized === 'reject' || normalized === 'rejected') return 'rejected'
      return ''
    },
    async submitApproval(action) {
      if (!this.currentStep || !this.currentStepApproval) return
      const context = await this.ensureProject()
      if (!context.id) {
        this.error = '项目数据尚未加载完成'
        return
      }
      this.approvalSubmitting = action
      this.error = ''
      try {
        await submitPipelineApproval(context.id, this.runId, this.currentStep.id, {
          action,
          comment: this.approvalComment
        })
        this.approvalComment = ''
        await this.loadDetail()
      } catch (err) {
        const error = this.normalizeError(err, '审批操作失败')
        this.error = error.message || '审批操作失败'
      } finally {
        this.approvalSubmitting = ''
      }
    },
    normalizeError(err, fallbackMessage) {
      if (!err) {
        const error = new Error(fallbackMessage || '请求失败')
        error.status = 0
        return error
      }
      if (err.response) {
        const { status, data } = err.response
        const message =
          (data && (data.error || data.message)) ||
          err.message ||
          fallbackMessage ||
          '请求失败'
        const error = new Error(message)
        error.status = status
        return error
      }
      if (typeof err.status === 'number') {
        if (!err.message && fallbackMessage) {
          err.message = fallbackMessage
        }
        return err
      }
      const error = err instanceof Error ? err : new Error(fallbackMessage || '请求失败')
      if (typeof error.status !== 'number') {
        error.status = 0
      }
      return error
    },
    formatTime(unix) {
      if (!unix) return ''
      const ts = unix > 1e12 ? unix : unix * 1000
      return new Date(ts).toLocaleString()
    },
    formatDuration(start, finish) {
      if (!start) return '—'
      const startMs = start > 1e12 ? start : start * 1000
      const endMs = finish ? (finish > 1e12 ? finish : finish * 1000) : Date.now()
      const diff = Math.max(0, endMs - startMs)
      const minutes = Math.floor(diff / 60000)
      const seconds = Math.floor((diff % 60000) / 1000)
      if (minutes > 0) {
        return `${minutes}m ${seconds}s`
      }
      return `${seconds}s`
    }
  }
}

</script>

<style scoped>
.build-detail {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
  width: 100%;
}

.build-detail__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
}

.build-detail__meta {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.build-detail__meta-actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.build-detail__meta-back {
  padding: 0.25rem 0.9rem;
}

.build-detail__meta-line {
  margin: 0;
  color: #6b7280;
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.build-detail__steps {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.build-detail__flow {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.8rem;
  padding: 1rem;
  background: #f8fafc;
  border-radius: 12px;
}

.build-detail__flow-step {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.6rem 0.8rem;
  background: #fff;
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.35);
  box-shadow: 0 4px 14px rgba(15, 23, 42, 0.08);
  cursor: pointer;
  transition: box-shadow 0.15s ease, border-color 0.15s ease;
}

.build-detail__flow-step:hover {
  box-shadow: 0 10px 26px rgba(15, 23, 42, 0.15);
}

.build-detail__flow-step--active {
  border-color: rgba(37, 99, 235, 0.55);
  box-shadow: 0 16px 34px rgba(37, 99, 235, 0.2);
}

.build-detail__flow-info {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.build-detail__flow-name {
  font-size: 0.9rem;
  font-weight: 600;
  color: #1f2937;
}

.build-detail__flow-meta {
  font-size: 0.75rem;
  color: #6b7280;
}

.build-detail__flow-arrow {
  color: #94a3b8;
  font-size: 1.4rem;
}

.build-detail__steps-header {
  display: flex;
  justify-content: flex-end;
  align-items: center;
}

.build-detail__layout {
  display: flex;
  gap: 1.5rem;
  min-height: 320px;
  align-items: stretch;
}

.build-detail__sidebar {
  width: 220px;
  background: #f9fafb;
  padding: 1rem;
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.build-detail__step-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.build-detail__steps-empty {
  color: #6b7280;
  font-size: 0.9rem;
}

.build-detail__step-item {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  padding: 0.45rem 0.55rem;
  border-radius: 10px;
  background: transparent;
  cursor: pointer;
  transition: background 0.15s ease;
}

.build-detail__step-item:hover {
  background: rgba(79, 70, 229, 0.08);
}

.build-detail__step-item--active {
  background: rgba(79, 70, 229, 0.15);
}

.pipeline-status-bullet {
  width: 9px;
  height: 9px;
  border-radius: 50%;
}

.pipeline-status-bullet--success {
  background: #22c55e;
}

.pipeline-status-bullet--running {
  background: #3b82f6;
}

.pipeline-status-bullet--failure,
.pipeline-status-bullet--failed,
.pipeline-status-bullet--error {
  background: #ef4444;
}

.pipeline-status-bullet--pending {
  background: #facc15;
}

.pipeline-status-bullet--empty {
  visibility: hidden;
}
.pipeline-status-bullet--not-run {
  background: transparent;
  border: 1px dashed rgba(148, 163, 184, 0.7);
}

.build-detail__logs {
  flex: 1;
  background: #000;
  color: #e2e8f0;
  border-radius: 12px;
  padding: 1.1rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
  min-height: 280px;
  max-height: 52vh;
  overflow: hidden;
}

.build-detail__logs-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.build-detail__logs-header h4 {
  margin: 0 0 0.35rem;
  color: #f8fafc;
}

.build-detail__logs-meta {
  margin: 0;
  display: flex;
  gap: 1rem;
  font-size: 0.85rem;
}

.build-detail__logs-actions--approval {
  gap: 0.5rem;
}

.button--danger {
  background: rgba(185, 28, 28, 0.85);
  color: #fff;
}

.button--danger:disabled {
  opacity: 0.6;
}

.build-detail__approval-body {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  flex: 1;
}

.build-detail__approval-message {
  margin: 0;
  font-size: 1rem;
  font-weight: 500;
  color: #f8fafc;
}

.build-detail__approval-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
  color: #d1d5db;
  font-size: 0.85rem;
}

.build-detail__approval-comment {
  width: 100%;
  min-height: 80px;
  border-radius: 10px;
  border: 1px solid rgba(148, 163, 184, 0.35);
  background: rgba(17, 24, 39, 0.75);
  color: #e2e8f0;
  padding: 0.75rem;
  resize: vertical;
  font-family: inherit;
}
.build-detail__approval-actions {
  display: flex;
  justify-content: center;
  gap: 1.5rem;
  margin-top: 1.2rem;
}

.approval-button {
  padding: 0.65rem 1.6rem;
  border-radius: 999px;
  border: none;
  font-size: 0.95rem;
  font-weight: 600;
  color: #fff;
  cursor: pointer;
  transition: transform 0.15s ease, opacity 0.15s ease;
}

.approval-button:disabled {
  opacity: 0.65;
  cursor: not-allowed;
}

.approval-button--approve {
  background: #16a34a;
  box-shadow: 0 10px 25px rgba(22, 163, 74, 0.35);
}

.approval-button--approve:not(:disabled):hover {
  transform: translateY(-1px);
  box-shadow: 0 12px 28px rgba(21, 128, 61, 0.4);
}

.approval-button--reject {
  background: #dc2626;
  box-shadow: 0 10px 25px rgba(220, 38, 38, 0.35);
}

.approval-button--reject:not(:disabled):hover {
  transform: translateY(-1px);
  box-shadow: 0 12px 28px rgba(185, 28, 28, 0.4);
}

.build-detail__approval-history h5 {
  margin: 0 0 0.5rem;
  font-size: 0.95rem;
  color: #f8fafc;
}

.build-detail__approval-history ul {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.build-detail__approval-history li {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  font-size: 0.85rem;
  color: #cbd5f5;
}

.build-detail__approval-user {
  font-weight: 600;
}

.build-detail__approval-action {
  color: #fcd34d;
}

.build-detail__approval-action.approved {
  color: #4ade80;
}

.build-detail__approval-action.rejected {
  color: #f87171;
}

.build-detail__approval-time {
  color: #94a3b8;
}

.build-detail__approval-comment-text {
  color: #e2e8f0;
}

.build-detail__log-viewer {
  margin: 0;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  font-size: 0.85rem;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
  overflow-wrap: anywhere;
  background: transparent;
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  max-height: calc(52vh - 120px);
}

.build-detail__log-viewer code {
  display: block;
  white-space: inherit;
  word-break: inherit;
  overflow-wrap: inherit;
}

.build-detail__logs-empty {
  color: #94a3b8;
  text-align: center;
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.build-detail__error {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
}

.pipeline-status {
  padding: 0.35rem 0.75rem;
  border-radius: 999px;
  font-size: 0.85rem;
  font-weight: 600;
  color: #1f2937;
  background: #e5e7eb;
}

.pipeline-status--success {
  background: rgba(34, 197, 94, 0.16);
  color: #14532d;
}

.pipeline-status--failure,
.pipeline-status--failed,
.pipeline-status--error {
  background: rgba(239, 68, 68, 0.16);
  color: #7f1d1d;
}

.pipeline-status--running {
  background: rgba(59, 130, 246, 0.16);
  color: #1e3a8a;
}

.pipeline-status--pending {
  background: rgba(250, 204, 21, 0.16);
  color: #713f12;
}

.button.button--ghost {
  min-width: auto;
}

@media (max-width: 960px) {
  .build-detail__layout {
    flex-direction: column;
  }

  .build-detail__sidebar {
    width: 100%;
    flex-direction: row;
    overflow-x: auto;
  }

  .build-detail__workflow-group {
    min-width: 180px;
  }
}
</style>
