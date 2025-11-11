export const STEP_TYPES = Object.freeze({
  APPROVAL: 'approval'
})

export function normalizeStepType(value) {
  if (!value) return ''
  return String(value).trim().toLowerCase()
}

export function isApprovalStep(step) {
  if (!step) return false
  return normalizeStepType(step.type) === STEP_TYPES.APPROVAL
}
