const PIPELINE_STATUS = Object.freeze({
  SUCCESS: 'success',
  FAILURE: 'failure',
  ERROR: 'error',
  RUNNING: 'running',
  PENDING: 'pending',
  BLOCKED: 'blocked',
  SKIPPED: 'skipped',
  KILLED: 'killed',
  CANCELED: 'canceled',
  UNKNOWN: 'unknown',
  NOT_RUN: 'not-run'
})

const PIPELINE_STATUS_ALIASES = {
  failed: PIPELINE_STATUS.FAILURE,
  failure: PIPELINE_STATUS.FAILURE,
  cancel: PIPELINE_STATUS.CANCELED,
  cancelled: PIPELINE_STATUS.CANCELED,
  error: PIPELINE_STATUS.ERROR
}

const PIPELINE_STATUS_META = {
  [PIPELINE_STATUS.SUCCESS]: {
    label: '成功',
    className: 'success',
    bulletClass: 'success'
  },
  [PIPELINE_STATUS.FAILURE]: {
    label: '失败',
    className: 'failure',
    bulletClass: 'failure'
  },
  [PIPELINE_STATUS.ERROR]: {
    label: '出错',
    className: 'failure',
    bulletClass: 'failure'
  },
  [PIPELINE_STATUS.KILLED]: {
    label: '终止',
    className: 'killed',
    bulletClass: 'failure'
  },
  [PIPELINE_STATUS.CANCELED]: {
    label: '已取消',
    className: 'killed',
    bulletClass: 'failure'
  },
  [PIPELINE_STATUS.RUNNING]: {
    label: '运行中',
    className: 'running',
    bulletClass: 'running',
    cancellable: true,
    active: true
  },
  [PIPELINE_STATUS.PENDING]: {
    label: '等待',
    className: 'pending',
    bulletClass: 'pending',
    cancellable: true,
    active: true
  },
  [PIPELINE_STATUS.BLOCKED]: {
    label: '等待审批',
    className: 'pending',
    bulletClass: 'pending',
    cancellable: true,
    active: true
  },
  [PIPELINE_STATUS.SKIPPED]: {
    label: '跳过',
    className: 'pending',
    bulletClass: 'pending'
  },
  [PIPELINE_STATUS.NOT_RUN]: {
    label: '未执行',
    className: 'not-run',
    bulletClass: 'not-run',
    bulletEmpty: true
  },
  [PIPELINE_STATUS.UNKNOWN]: {
    label: '未知',
    className: 'pending',
    bulletClass: 'pending'
  }
}

const APPROVAL_STATES = Object.freeze({
  PENDING: 'pending',
  APPROVED: 'approved',
  REJECTED: 'rejected',
  EXPIRED: 'expired'
})

const APPROVAL_STATE_META = {
  [APPROVAL_STATES.PENDING]: { label: '等待审批' },
  [APPROVAL_STATES.APPROVED]: { label: '已通过' },
  [APPROVAL_STATES.REJECTED]: { label: '已拒绝' },
  [APPROVAL_STATES.EXPIRED]: { label: '已超时' }
}

const APPROVAL_ACTION_META = {
  approve: { label: '通过', className: 'approved' },
  approved: { label: '通过', className: 'approved' },
  reject: { label: '拒绝', className: 'rejected' },
  rejected: { label: '拒绝', className: 'rejected' },
  expired: { label: '超时', className: '' }
}

export function normalizePipelineStatus(value) {
  if (value === null || value === undefined) {
    return PIPELINE_STATUS.UNKNOWN
  }
  const normalized = String(value).trim().toLowerCase()
  if (PIPELINE_STATUS_META[normalized]) {
    return normalized
  }
  if (PIPELINE_STATUS_ALIASES[normalized]) {
    return PIPELINE_STATUS_ALIASES[normalized]
  }
  return normalized || PIPELINE_STATUS.UNKNOWN
}

export function getPipelineStatusMeta(value) {
  const normalized = normalizePipelineStatus(value)
  return PIPELINE_STATUS_META[normalized] || PIPELINE_STATUS_META[PIPELINE_STATUS.UNKNOWN]
}

export function formatPipelineStatus(value) {
  return getPipelineStatusMeta(value).label
}

export function getPipelineStatusClass(value) {
  return getPipelineStatusMeta(value).className
}

export function getPipelineBulletClass(value) {
  return getPipelineStatusMeta(value).bulletClass
}

export function isPipelineStatusCancellable(value) {
  return Boolean(getPipelineStatusMeta(value).cancellable)
}

export function isPipelineStatusActive(value) {
  return Boolean(getPipelineStatusMeta(value).active)
}

export function normalizeApprovalState(value) {
  if (!value) return APPROVAL_STATES.PENDING
  const normalized = String(value).trim().toLowerCase()
  if (APPROVAL_STATE_META[normalized]) {
    return normalized
  }
  return APPROVAL_STATES.PENDING
}

export function formatApprovalState(value) {
  const normalized = normalizeApprovalState(value)
  return APPROVAL_STATE_META[normalized].label
}

export function formatApprovalAction(value) {
  if (!value) return '待处理'
  const normalized = String(value).trim().toLowerCase()
  return (APPROVAL_ACTION_META[normalized] && APPROVAL_ACTION_META[normalized].label) || normalized || '待处理'
}

export function getApprovalActionClass(value) {
  if (!value) return ''
  const normalized = String(value).trim().toLowerCase()
  return (APPROVAL_ACTION_META[normalized] && APPROVAL_ACTION_META[normalized].className) || ''
}

export { PIPELINE_STATUS, APPROVAL_STATES }
