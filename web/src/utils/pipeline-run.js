export function emptyVariableRow() {
  return { key: '', value: '' }
}

export function normalizeVariableRows(rows) {
  if (!Array.isArray(rows) || rows.length === 0) {
    return [emptyVariableRow()]
  }
  return rows.map(row => ({
    key: row && row.key ? String(row.key) : '',
    value: row && row.value !== undefined ? String(row.value) : ''
  }))
}

export function serializeVariableRows(rows) {
  if (!Array.isArray(rows)) {
    return null
  }
  const payload = {}
  let hasValue = false
  rows.forEach(row => {
    if (!row) return
    const key = row.key ? String(row.key).trim() : ''
    if (!key) return
    payload[key] = row.value !== undefined ? String(row.value) : ''
    hasValue = true
  })
  return hasValue ? payload : null
}
