<template>
  <div class="pipeline-page">
    <section class="panel pipeline-history">
      <header class="pipeline-history__header">
        <h3>构建记录</h3>
        <button class="button button--ghost" :disabled="loadingRuns" @click="loadRuns">刷新</button>
      </header>
      <div v-if="runs.length" class="pipeline-history__table-wrapper">
        <table class="pipeline-history__table">
          <thead>
            <tr>
              <th>状态</th>
              <th>运行编号</th>
              <th>分支</th>
              <th>Commit</th>
              <th>发起人</th>
              <th>耗时</th>
              <th>触发时间</th>
              <th>备注</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="run in runs"
              :key="run.id"
              :class="[
                'pipeline-history__row',
                { 'pipeline-history__row--active': run.id === selectedRunId }
              ]"
            >
              <td>
                <span
                  :class="['pipeline-status', `pipeline-status--${statusClass(run.status)}`, { 'pipeline-history__cell-link': isRunNavigable(run) }]"
                  :role="isRunNavigable(run) ? 'button' : undefined"
                  :tabindex="isRunNavigable(run) ? 0 : -1"
                  @click="isRunNavigable(run) && viewDetails(run)"
                  @keydown.enter.prevent="isRunNavigable(run) && viewDetails(run)"
                >
                  {{ formatStatus(run.status) }}
                </span>
              </td>
              <td>
                <span
                  :class="['pipeline-history__number', { 'pipeline-history__cell-link': isRunNavigable(run) }]"
                  :role="isRunNavigable(run) ? 'button' : undefined"
                  :tabindex="isRunNavigable(run) ? 0 : -1"
                  @click="isRunNavigable(run) && viewDetails(run)"
                  @keydown.enter.prevent="isRunNavigable(run) && viewDetails(run)"
                >
                  #{{ run.number }}
                </span>
              </td>
              <td>{{ run.branch }}</td>
              <td>{{ formatCommit(run.commit) }}</td>
              <td>{{ run.author || '系统' }}</td>
              <td>{{ formatDuration(run.created, run.finished) }}</td>
              <td>{{ formatTime(run.created) }}</td>
              <td>{{ run.message || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="totalPages > 1" class="pipeline-history__pagination">
        <button class="button button--ghost" :disabled="page <= 1 || loadingRuns" @click="prevPage">上一页</button>
        <span class="pipeline-history__pagination-info">第 {{ page }} / {{ totalPages }} 页 · 共 {{ totalRuns }} 条</span>
        <button class="button button--ghost" :disabled="page >= totalPages || loadingRuns" @click="nextPage">下一页</button>
      </div>
    </section>

    <section v-if="error" class="pipeline-error panel">
      <span>{{ error }}</span>
      <button class="button button--ghost" @click="retryAll">重试</button>
    </section>

    <!-- YAML 编辑弹窗 -->
    <div v-if="editorVisible" class="pipeline-modal" @click.self="closeEditor">
      <div class="pipeline-modal__content pipeline-modal__content--yaml">
        <header class="pipeline-modal__header">
          <h3>编辑 YAML</h3>
          <button class="pipeline-modal__close" @click="closeEditor">×</button>
        </header>
        <section class="pipeline-modal__body pipeline-modal__body--fill">
          <!-- eslint-disable-next-line vue/html-self-closing -->
          <textarea ref="yamlEditor" v-model="draft" class="pipeline-modal__textarea" spellcheck="false"></textarea>
        </section>
        <footer class="pipeline-modal__footer">
          <button class="button button--ghost" :disabled="saving" @click="closeEditor">取消</button>
          <button class="button" :disabled="saving" @click="saveYaml">{{ saving ? '保存中…' : '保存' }}</button>
        </footer>
      </div>
    </div>

    <!-- Dockerfile 编辑弹窗 -->
    <div v-if="dockerfileVisible" class="pipeline-modal" @click.self="closeDockerfileEditor">
      <div class="pipeline-modal__content pipeline-modal__content--yaml">
        <header class="pipeline-modal__header">
          <h3>编辑 Dockerfile</h3>
          <button class="pipeline-modal__close" @click="closeDockerfileEditor">×</button>
        </header>
        <section class="pipeline-modal__body pipeline-modal__body--fill">
          <p class="pipeline-modal__hint">
            如果仓库目录中不存在 Dockerfile，将使用这里保存的内容参与构建。
          </p>
          <!-- eslint-disable-next-line vue/html-self-closing -->
          <textarea
            ref="dockerfileEditor"
            v-model="dockerfileDraft"
            class="pipeline-modal__textarea"
            spellcheck="false"
            placeholder="# 在此编写 Dockerfile 内容"
          ></textarea>
        </section>
        <footer class="pipeline-modal__footer">
          <button class="button button--ghost" :disabled="dockerfileSaving" @click="closeDockerfileEditor">取消</button>
          <button class="button" :disabled="dockerfileSaving" @click="saveDockerfile">{{ dockerfileSaving ? '保存中…' : '保存' }}</button>
        </footer>
      </div>
    </div>

    <!-- 运行流水线弹窗 -->
    <div v-if="runModalVisible" class="pipeline-modal" @click.self="closeRunModal">
      <div class="pipeline-modal__content">
        <header class="pipeline-modal__header">
          <h3>运行流水线</h3>
          <button class="pipeline-modal__close" @click="closeRunModal">×</button>
        </header>
        <section class="pipeline-modal__body">
          <label class="modal-field">
            <span>构建分支 *</span>
            <input v-model="runForm.branch" placeholder="例如 main">
          </label>
          <label class="modal-field">
            <span>Commit ID (可选)</span>
            <input v-model="runForm.commit" placeholder="传入具体 commit 时优先使用">
          </label>
          <div class="modal-field">
            <span>运行变量（可选）</span>
            <p class="modal-hint">将以环境变量的形式传入流水线，仅填写需要覆盖的键。</p>
            <div
              v-for="(variable, idx) in runForm.variables"
              :key="`pipeline-run-variable-${idx}`"
              class="run-variable-row"
            >
              <input v-model="variable.key" placeholder="变量名，如 ENV">
              <input v-model="variable.value" placeholder="变量值">
              <button
                type="button"
                class="button button--ghost run-variable-remove"
                @click="removeRunVariable(idx)"
              >删除</button>
            </div>
            <button type="button" class="button button--ghost run-variable-add" @click="addRunVariable">
              + 添加变量
            </button>
          </div>
          <p v-if="runFormError" class="modal-error">{{ runFormError }}</p>
        </section>
        <footer class="pipeline-modal__footer">
          <button class="button button--ghost" :disabled="running" @click="closeRunModal">取消</button>
          <button class="button" :disabled="running" @click="submitRun">
            {{ running ? '提交中…' : '运行' }}
          </button>
        </footer>
      </div>
    </div>

    <!-- 流水线设置弹窗 -->
    <div v-if="settingsVisible" class="pipeline-modal" @click.self="closeSettings">
      <div class="pipeline-modal__content pipeline-modal__content--settings">
        <header class="pipeline-modal__header">
          <h3>流水线设置</h3>
          <button class="pipeline-modal__close" @click="closeSettings">×</button>
        </header>
        <section class="pipeline-modal__body pipeline-settings">
          <div v-if="settingsLoading" class="settings-loading">设置加载中…</div>
          <template v-else>
            <div class="settings-block">
              <h4>构建记录</h4>
              <label class="settings-field checkbox">
                <input v-model="settingsForm.cleanup_enabled" type="checkbox">
                <span>删除过期构建记录</span>
              </label>
              <div class="settings-flex">
                <label class="settings-field">
                  <span>构建记录保留期限 (天)</span>
                  <input v-model.number="settingsForm.retention_days" type="number" min="0">
                </label>
                <label class="settings-field">
                  <span>构建记录最大数量</span>
                  <input v-model.number="settingsForm.max_records" type="number" min="1">
                </label>
              </div>
              <label class="settings-field checkbox">
                <input v-model="settingsForm.disallow_parallel" type="checkbox">
                <span>不允许并发构建</span>
              </label>
            </div>

            <div class="settings-block">
              <h4>构建触发器</h4>
              <div v-if="!cronRows.length" class="settings-empty">暂无定时任务，点击下方添加。</div>
              <div v-for="(cron, idx) in cronRows" :key="`cron-${idx}`" class="settings-flex">
                <input
                  v-model="cronRows[idx]"
                  class="settings-field__input settings-field__input--grow"
                  placeholder="例如：0 0 * * *"
                >
                <button class="button button--ghost" @click="removeCron(idx)">移除</button>
              </div>
              <button class="button button--ghost" @click="addCron">添加 Cron 表达式</button>
              <p class="settings-hint">
                支持 Linux crontab 语法，可按需配置多个表达式。
              </p>
            </div>

          </template>
        </section>
        <footer class="pipeline-modal__footer">
          <button class="button button--ghost" :disabled="settingsSaving" @click="closeSettings">取消</button>
          <button class="button" :disabled="settingsSaving" @click="saveSettings">{{ settingsSaving ? '保存中…' : '保存' }}</button>
        </footer>
      </div>
    </div>
  </div>
</template>

<script>
import 'codemirror/lib/codemirror.css'
import 'codemirror/theme/material.css'
import { listRepositories } from '@/api/project/repos'
import {
  getPipelineConfig,
  updatePipelineConfig,
  listPipelineRuns,
  triggerPipelineRun,
  getPipelineSettings,
  updatePipelineSettings
} from '@/api/project/pipeline'
import { formatPipelineStatus, getPipelineStatusClass } from '@/constants/status'
import { formatTime as formatTimeUtil, formatDuration as formatDurationUtil } from '@/utils/time'
import { normalizeError as normalizeErrorUtil } from '@/utils/error'
import { emptyVariableRow, serializeVariableRows } from '@/utils/pipeline-run'

let CodeMirrorInstance = null
let codeMirrorModesReady = false

export default {
  name: 'ProjectPipeline',
  inject: {
    projectPipelineExpose: {
      default: null
    }
  },
  props: {
    project: {
      type: Object,
      default: null
    },
    isAdmin: {
      type: Boolean,
      default: false
    }
  },
  data() {
    return {
      yamlContent: '',
      draft: '',
      lastSaved: '',
      loadingYaml: true,
      loadingRuns: true,
      error: '',
      saving: false,
      running: false,
      runs: [],
      totalRuns: 0,
      selectedRunId: null,
      page: 1,
      perPage: 20,
      runResult: null,
      editorVisible: false,
      yamlEditor: null,
      dockerfileVisible: false,
      dockerfileEditor: null,
      dockerfileDraft: '',
      dockerfileSaving: false,
      runModalVisible: false,
      runForm: {
        branch: '',
        commit: '',
        variables: [emptyVariableRow()]
      },
      runFormError: '',
      settingsVisible: false,
      settingsSaving: false,
      settingsForm: {
        cleanup_enabled: false,
        retention_days: 7,
        max_records: 10,
        dockerfile: '',
        disallow_parallel: false,
        cron_schedules: []
      },
      cronRows: [],
      localProject: null,
      pendingProjectRequest: null,
      pendingAction: null,
      settingsLoading: false,
      settingsLoaded: false,
      settingsLoadPromise: null
    }
  },
  computed: {
    resolvedProject() {
      return this.project || this.localProject
    },
    totalPages() {
      if (!this.totalRuns || this.perPage <= 0) return 0
      return Math.max(1, Math.ceil(this.totalRuns / this.perPage))
    }
  },
  watch: {
    running() {
      this.notifyExpose()
    },
    loadingRuns() {
      this.notifyExpose()
    },
    project: {
      immediate: true,
      handler() {
        this.page = 1
        this.totalRuns = 0
        this.loadYaml()
        this.loadRuns()
        this.loadSettings()
        this.notifyExpose()
      }
    },
    isAdmin(val) {
      if (val) {
        this.loadSettings()
      } else {
        this.settingsLoaded = false
      }
    },
    '$route.params'(next, prev) {
      if (next.owner !== prev.owner || next.name !== prev.name) {
        this.localProject = null
        this.pendingProjectRequest = null
        this.page = 1
        this.totalRuns = 0
      }
    },
    '$route.query.action': {
      immediate: true,
      handler(action) {
        if (!action) return
        const normalized = String(action).toLowerCase()
        this.handleRouteAction(normalized)
      }
    }
  },
  mounted() {
    this.notifyExpose()
  },
  beforeDestroy() {
    this.$emit('pipeline-expose', null)
    if (typeof this.projectPipelineExpose === 'function') {
      this.projectPipelineExpose(null)
    }
    this.destroyDockerfileEditor()
  },
  methods: {
    formatCommit(commit) {
      if (!commit) return '—'
      return commit.length > 8 ? commit.slice(0, 7) : commit
    },
    nextPage() {
      if (this.page < this.totalPages && !this.loadingRuns) {
        this.page += 1
        this.loadRuns()
      }
    },
    prevPage() {
      if (this.page > 1 && !this.loadingRuns) {
        this.page -= 1
        this.loadRuns()
      }
    },
    async ensureCodeMirror() {
      if (!CodeMirrorInstance) {
        const imported = await import('codemirror')
        const instance = imported.default || imported
        // eslint-disable-next-line require-atomic-updates
        CodeMirrorInstance = instance
      }
      if (!codeMirrorModesReady) {
        await Promise.all([
          import('codemirror/mode/yaml/yaml'),
          import('codemirror/mode/dockerfile/dockerfile')
        ])
        // eslint-disable-next-line require-atomic-updates
        codeMirrorModesReady = true
      }
      return CodeMirrorInstance
    },
    sampleYaml() {
      return ''
    },
    sampleDockerfile() {
      return ''
    },
    async loadYaml() {
      this.loadingYaml = true
      this.error = ''
      try {
        const context = await this.ensureProject()
        if (!context.id) {
          this.yamlContent = this.sampleYaml()
          this.draft = this.yamlContent
          this.lastSaved = ''
          return
        }
        if (context.repo) {
          this.localProject = context.repo
        }
        const data = await getPipelineConfig(context.id)
        this.yamlContent = (data && data.content) ? data.content : this.sampleYaml()
        this.draft = this.yamlContent
        this.lastSaved = data && data.updated_at ? this.formatTime(data.updated_at) : ''
      } catch (err) {
        const error = this.normalizeError(err, '加载流水线失败')
        if (error.status === 404) {
          this.yamlContent = this.sampleYaml()
          this.draft = this.yamlContent
          this.lastSaved = ''
        } else {
          this.error = error.message || '加载流水线失败'
          this.yamlContent = this.sampleYaml()
          this.draft = this.yamlContent
        }
      } finally {
        this.loadingYaml = false
        this.applyPendingAction()
      }
    },
    async loadRuns() {
      this.loadingRuns = true
      try {
        const context = await this.ensureProject()
        if (!context.id) {
          this.runs = []
          return
        }
        if (context.repo) {
          this.localProject = context.repo
        }
        const data = await listPipelineRuns(context.id, {
          page: this.page,
          per_page: this.perPage
        })
        if (context.repo && !context.repo.branch && data.branch) {
          this.localProject = { ...context.repo, branch: data.branch }
        }
        const items = data.items || []
        this.totalRuns = Number(data.total) || 0
        if (this.totalRuns === 0 && items.length) {
          this.totalRuns = items.length
        }
        if (data.per_page) {
          const parsedPerPage = Number(data.per_page)
          if (!Number.isNaN(parsedPerPage) && parsedPerPage > 0) {
            this.perPage = parsedPerPage
          }
        }
        if (data.page) {
          const parsedPage = Number(data.page)
          if (!Number.isNaN(parsedPage) && parsedPage > 0) {
            this.page = parsedPage
          }
        }
        const totalPages = this.totalPages
        if (totalPages > 0 && this.page > totalPages) {
          this.page = totalPages
          await this.loadRuns()
          return
        }
        this.runs = items

        if (items.length) {
          const highlightParam = this.$route.query.highlight
          const highlightId = highlightParam ? Number(highlightParam) : NaN
          if (!Number.isNaN(highlightId) && items.some(run => run.id === highlightId)) {
            this.selectedRunId = highlightId
            const query = { ...this.$route.query }
            delete query.highlight
            const nav = this.$router.replace({ name: this.$route.name, params: this.$route.params, query })
            if (nav && typeof nav.catch === 'function') {
              nav.catch(() => {})
            }
          } else if (!this.selectedRunId) {
            this.selectedRunId = items[0].id
          }
        } else {
          this.selectedRunId = null
        }
      } catch (err) {
        const error = this.normalizeError(err, '加载构建记录失败')
        this.error = error.message || '加载构建记录失败'
        this.runs = []
        this.totalRuns = 0
      } finally {
        this.loadingRuns = false
      }
    },
    viewDetails(run) {
      if (!run || !run.id) return
      this.selectedRunId = run.id
      const { owner, name } = this.$route.params
      const nav = this.$router.push({
        name: 'ProjectPipelineRunDetail',
        params: { owner, name, runId: run.id }
      })
      if (nav && typeof nav.catch === 'function') {
        nav.catch(() => {})
      }
    },
    async loadSettings() {
      if (this.settingsLoading) {
        return this.settingsLoadPromise
      }
      this.settingsLoading = true
      const task = (async() => {
        try {
          const context = await this.ensureProject()
          if (!context.id) {
            this.settingsLoaded = false
            return
          }
          if (context.repo) {
            this.localProject = context.repo
          }
          let data
          try {
            data = await getPipelineSettings(context.id)
          } catch (err) {
            const error = this.normalizeError(err, '加载流水线设置失败')
            if (error.status === 404) {
              data = null
            } else {
              throw error
            }
          }
          const parsed = this.normalizeSettingsResponse(data)
          this.settingsForm = parsed
          this.cronRows = [...parsed.cron_schedules]
          if (!this.dockerfileVisible) {
            this.dockerfileDraft = parsed.dockerfile || ''
          }
          this.settingsLoaded = true
        } catch (err) {
          const error = this.normalizeError(err, '加载流水线设置失败')
          this.error = error.message || '加载流水线设置失败'
          this.settingsLoaded = false
          throw error
        }
      })()
      this.settingsLoadPromise = task
      try {
        await task
      } finally {
        this.settingsLoading = false
        this.settingsLoadPromise = null
      }
    },
    openEditor() {
      this.editorVisible = true
      this.draft = this.yamlContent
      this.$nextTick(async() => {
        const CodeMirror = await this.ensureCodeMirror()
        if (this.yamlEditor) {
          this.yamlEditor.toTextArea()
          this.yamlEditor = null
        }
        this.yamlEditor = CodeMirror.fromTextArea(this.$refs.yamlEditor, {
          mode: 'yaml',
          lineNumbers: true,
          theme: 'material',
          autofocus: true
        })
        this.yamlEditor.on('change', () => {
          this.draft = this.yamlEditor.getValue()
        })
        this.yamlEditor.setValue(this.draft)
        this.yamlEditor.setSize('100%', '100%')
        requestAnimationFrame(() => {
          if (this.yamlEditor) {
            this.yamlEditor.refresh()
            this.yamlEditor.focus()
          }
        })
      })
    },
    closeEditor() {
      this.editorVisible = false
      if (this.yamlEditor) {
        this.draft = this.yamlEditor.getValue()
        this.yamlEditor.toTextArea()
        this.yamlEditor = null
      }
    },
    async saveYaml() {
      const context = await this.ensureProject()
      if (!context.id) {
        this.error = '项目数据尚未加载完成，无法保存配置'
        return
      }
      this.saving = true
      this.error = ''
      const payload = {
        content: this.yamlEditor ? this.yamlEditor.getValue() : this.draft
      }
      try {
        const data = await updatePipelineConfig(context.id, payload)
        this.yamlContent = data.content || payload.content
        this.draft = this.yamlContent
        this.lastSaved = data.updated_at ? this.formatTime(data.updated_at) : this.formatTime(Math.floor(Date.now() / 1000))
        this.editorVisible = false
      } catch (err) {
        const error = this.normalizeError(err, '保存流水线失败')
        this.error = error.message || '保存流水线失败'
      } finally {
        this.saving = false
      }
    },
    async openDockerfileEditor() {
      await this.loadSettings()
      const existing = (this.settingsForm && typeof this.settingsForm.dockerfile === 'string')
        ? this.settingsForm.dockerfile
        : ''
      this.dockerfileDraft = existing || this.sampleDockerfile()
      this.dockerfileVisible = true
      this.$nextTick(async() => {
        try {
          const textarea = this.$refs.dockerfileEditor
          if (!textarea) {
            return
          }
          const CodeMirror = await this.ensureCodeMirror()
          if (this.dockerfileEditor) {
            this.dockerfileEditor.toTextArea()
            this.dockerfileEditor = null
          }
          this.dockerfileEditor = CodeMirror.fromTextArea(textarea, {
            mode: 'dockerfile',
            lineNumbers: true,
            theme: 'material',
            autofocus: true
          })
          this.dockerfileEditor.on('change', () => {
            this.dockerfileDraft = this.dockerfileEditor.getValue()
          })
          this.dockerfileEditor.setValue(this.dockerfileDraft || '')
          this.dockerfileEditor.setSize('100%', '100%')
          requestAnimationFrame(() => {
            if (this.dockerfileEditor) {
              this.dockerfileEditor.refresh()
              this.dockerfileEditor.focus()
            }
          })
        } catch (err) {
          console.warn('初始化 Dockerfile 编辑器失败', err)
        }
      })
    },
    closeDockerfileEditor() {
      this.destroyDockerfileEditor()
      this.dockerfileVisible = false
      const saved = (this.settingsForm && typeof this.settingsForm.dockerfile === 'string')
        ? this.settingsForm.dockerfile
        : ''
      this.dockerfileDraft = saved
    },
    async saveDockerfile() {
      if (this.dockerfileSaving) return
      this.dockerfileSaving = true
      try {
        const context = await this.ensureProject()
        if (!context.id) {
          throw new Error('项目数据尚未加载完成，无法保存 Dockerfile')
        }
        const dockerfileBody = (this.dockerfileEditor && this.dockerfileEditor.getValue()) || this.dockerfileDraft || ''
        const payload = this.buildSettingsPayload()
        payload.dockerfile = dockerfileBody
        const data = await updatePipelineSettings(context.id, payload)
        const parsed = this.normalizeSettingsResponse(data)
        this.settingsForm = parsed
        this.cronRows = [...parsed.cron_schedules]
        this.dockerfileDraft = parsed.dockerfile || dockerfileBody
        this.destroyDockerfileEditor()
        this.dockerfileVisible = false
      } catch (err) {
        const error = this.normalizeError(err, '保存 Dockerfile 失败')
        this.error = error.message || '保存 Dockerfile 失败'
        console.warn(err)
      } finally {
        this.dockerfileSaving = false
        this.notifyExpose()
      }
    },
    destroyDockerfileEditor() {
      if (this.dockerfileEditor) {
        try {
          this.dockerfileDraft = this.dockerfileEditor.getValue()
        } catch (err) {
          // ignore read errors during teardown
        }
        this.dockerfileEditor.toTextArea()
        this.dockerfileEditor = null
      }
    },
    async openRunModal() {
      const context = await this.ensureProject()
      const repo = context.repo || this.resolvedProject
      const branch = this.resolveDefaultBranch(repo)
      this.runForm = {
        branch,
        commit: '',
        variables: []
      }
      this.runFormError = context.id ? '' : '项目数据尚未加载完成，请稍后重试'
      this.runModalVisible = true
    },
    closeRunModal() {
      if (!this.running) {
        this.runModalVisible = false
        this.resetRunForm()
      }
    },
    async submitRun() {
      const context = await this.ensureProject()
      if (!context.id) {
        this.runFormError = '项目数据尚未加载完成，请刷新后再试'
        return
      }
      if (!this.runForm.branch.trim()) {
        this.runFormError = '构建分支为必填项'
        return
      }
      this.runFormError = ''
      this.running = true
      this.error = ''
      let result = null
      try {
        const payload = {
          branch: this.runForm.branch.trim(),
          commit: this.runForm.commit.trim()
        }
        const variablesPayload = serializeVariableRows(this.runForm.variables)
        if (variablesPayload) {
          payload.variables = variablesPayload
        }
        result = await triggerPipelineRun(context.id, payload)
        this.runResult = result
        this.insertOrUpdateRun(result)
      } catch (err) {
        const error = this.normalizeError(err, '触发流水线失败')
        this.runFormError = error.message || '触发流水线失败'
      } finally {
        this.running = false
      }
      if (result && result.id) {
        this.closeRunModal()
        this.loadRuns()
      }
    },
    resetRunForm() {
      this.runForm = {
        branch: '',
        commit: '',
        variables: []
      }
    },
    resolveDefaultBranch(repo) {
      const recentRun = this.runs && this.runs.length ? this.runs[0] : null
      const candidates = [
        repo && repo.branch,
        repo && repo.default_branch,
        recentRun && recentRun.branch,
        'main'
      ]
      for (const candidate of candidates) {
        if (candidate && String(candidate).trim()) {
          return String(candidate).trim()
        }
      }
      return 'main'
    },
    addRunVariable() {
      this.runForm.variables = [...this.runForm.variables, emptyVariableRow()]
    },
    removeRunVariable(index) {
      const next = [...this.runForm.variables]
      next.splice(index, 1)
      this.runForm.variables = next
    },
    insertOrUpdateRun(run) {
      if (!run || !run.id) return
      const normalized = {
        id: run.id,
        number: run.number,
        status: run.status,
        branch: run.branch,
        commit: run.commit,
        author: run.author,
        created: run.created,
        finished: run.finished,
        message: run.message
      }
      const index = this.runs.findIndex(item => item.id === normalized.id)
      if (index >= 0) {
        this.$set(this.runs, index, { ...this.runs[index], ...normalized })
      } else {
        this.runs = [normalized, ...this.runs]
      }
      this.selectRun(normalized, { force: true })
    },
    async ensureProject() {
      if (this.project && this.project.id) {
        return { id: this.project.id, repo: this.project }
      }
      if (this.localProject && this.localProject.id) {
        return { id: this.localProject.id, repo: this.localProject }
      }
      if (this.pendingProjectRequest) {
        return this.pendingProjectRequest
      }
      const { owner, name } = this.$route.params || {}
      if (!owner || !name) {
        return { id: null, repo: null }
      }
      const projectRequest = (async() => {
        try {
          const search = `${owner}/${name}`
          const data = await listRepositories({ search, per_page: 1, page: 1 })
          const repo = (data.items && data.items[0]) || null
          if (repo) {
            this.localProject = repo
            return { id: repo.id || null, repo }
          }
          return { id: null, repo: null }
        } catch (err) {
          const error = this.normalizeError(err, '加载项目信息失败')
          this.error = error.message || '加载项目信息失败'
          return { id: null, repo: null }
        } finally {
          this.pendingProjectRequest = null
        }
      })()
      this.pendingProjectRequest = projectRequest
      return projectRequest
    },
    addCron() {
      this.cronRows.push('')
    },
    removeCron(idx) {
      this.cronRows.splice(idx, 1)
    },
    cleanCronRows() {
      const seen = new Set()
      const result = []
      this.cronRows.forEach(raw => {
        const trimmed = (raw || '').trim()
        if (!trimmed || seen.has(trimmed)) {
          return
        }
        seen.add(trimmed)
        result.push(trimmed)
      })
      return result
    },
    normalizeSettingsResponse(payload) {
      const defaults = {
        cleanup_enabled: false,
        retention_days: 7,
        max_records: 10,
        dockerfile: '',
        disallow_parallel: false,
        cron_schedules: []
      }
      if (!payload) {
        return { ...defaults }
      }
      const schedules = Array.isArray(payload.cron_schedules)
        ? payload.cron_schedules.filter(item => typeof item === 'string' && item.trim() !== '').map(item => item.trim())
        : []
      return {
        cleanup_enabled: Boolean(payload.cleanup_enabled),
        retention_days: Number.isFinite(payload.retention_days) ? payload.retention_days : defaults.retention_days,
        max_records: Number.isFinite(payload.max_records) && payload.max_records > 0 ? payload.max_records : defaults.max_records,
        dockerfile: payload.dockerfile || '',
        disallow_parallel: Boolean(payload.disallow_parallel),
        cron_schedules: schedules
      }
    },
    normalizeError: normalizeErrorUtil,
    async openSettings() {
      this.settingsVisible = true
      try {
        await this.loadSettings()
      } catch (err) {
        // loadSettings 已处理错误提示
      }
      this.cronRows = [...(this.settingsForm.cron_schedules || [])]
    },
    closeSettings(force = false) {
      if (force || !this.settingsSaving) {
        this.settingsVisible = false
      }
    },
    async saveSettings() {
      const context = await this.ensureProject()
      if (!context.id) {
        this.error = '项目数据尚未加载完成，无法保存设置'
        return
      }
      this.settingsSaving = true
      this.error = ''
      const payload = {
        cleanup_enabled: this.settingsForm.cleanup_enabled,
        retention_days: this.settingsForm.retention_days,
        max_records: this.settingsForm.max_records,
        dockerfile: this.settingsForm.dockerfile,
        disallow_parallel: this.settingsForm.disallow_parallel,
        cron_schedules: this.cleanCronRows()
      }
      try {
        const data = await updatePipelineSettings(context.id, payload)
        const parsed = this.normalizeSettingsResponse(data)
        this.settingsForm = parsed
        this.cronRows = [...parsed.cron_schedules]
        if (!this.dockerfileVisible) {
          this.dockerfileDraft = parsed.dockerfile || ''
        }
        this.closeSettings(true)
        if (this.$message && typeof this.$message.success === 'function') {
          this.$message.success('流水线设置已保存')
        }
      } catch (err) {
        const error = this.normalizeError(err, '保存流水线设置失败')
        this.error = error.message || '保存流水线设置失败'
      } finally {
        this.settingsSaving = false
        this.notifyExpose()
      }
    },
    retryAll() {
      this.loadYaml()
      this.loadRuns()
      this.loadSettings()
    },
    formatStatus: formatPipelineStatus,
    statusClass(value) {
      return getPipelineStatusClass(value)
    },
    formatTime: formatTimeUtil,
    formatDuration: formatDurationUtil,
    isRunNavigable(run) {
      return Boolean(run && run.id)
    },
    notifyExpose() {
      const runHandler = (...args) => this.openRunModal(...args)
      const editorHandler = (...args) => this.openEditor(...args)
      const settingsHandler = (...args) => this.openSettings(...args)
      const dockerfileHandler = (...args) => this.openDockerfileEditor(...args)
      const busyHandler = () => this.running || this.loadingRuns
      this.$emit('pipeline-expose', {
        openRunModal: runHandler,
        openEditor: editorHandler,
        openDockerfile: dockerfileHandler,
        openSettings: settingsHandler,
        isBusy: busyHandler
      })
      if (typeof this.projectPipelineExpose === 'function') {
        this.projectPipelineExpose({
          openRunModal: runHandler,
          openEditor: editorHandler,
          openDockerfile: dockerfileHandler,
          openSettings: settingsHandler,
          isBusy: busyHandler
        })
      }
    },
    handleRouteAction(action) {
      if (!action) return
      this.pendingAction = action
      this.$nextTick(async() => {
        await this.ensureProject()
        this.applyPendingAction()
      })
    },
    executeAction(action) {
      switch (action) {
        case 'run':
          this.openRunModal()
          break
        case 'editor':
        case 'edit':
        case 'yaml':
          this.openEditor()
          break
        case 'dockerfile':
        case 'docker':
          this.openDockerfileEditor()
          break
        case 'settings':
        case 'config':
          this.openSettings()
          break
        default:
          break
      }
    },
    applyPendingAction() {
      if (!this.pendingAction) return
      if (this.loadingYaml) return
      const action = this.pendingAction
      this.pendingAction = null
      this.executeAction(action)
      this.clearActionQuery()
    },
    clearActionQuery() {
      const { action, _actionTs, ...rest } = this.$route.query || {}
      if (action === undefined && _actionTs === undefined) return
      this.$nextTick(() => {
        const nav = this.$router.replace({
          name: this.$route.name,
          params: this.$route.params,
          query: rest
        })
        if (nav && typeof nav.catch === 'function') {
          nav.catch(() => {})
        }
      })
    }
  }
}
</script>

<style scoped>
.pipeline-page {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.pipeline-history__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.pipeline-history__table {
  width: 100%;
  border-collapse: collapse;
}

.pipeline-history__table-wrapper {
  overflow-x: auto;
}

.pipeline-history__list {
  flex: 1;
  min-width: 0;
}

.pipeline-history__cell-link {
  cursor: pointer;
  color: #2563eb;
  display: inline-flex;
  align-items: center;
}

.pipeline-history__cell-link:hover {
  text-decoration: underline;
}

.pipeline-history__number {
  font-weight: 600;
}

.pipeline-history__pagination {
  margin-top: 0.75rem;
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 0.75rem;
}

.pipeline-history__pagination-info {
  color: #6b7280;
  font-size: 0.85rem;
}

.pipeline-history__table th,
.pipeline-history__table td {
  padding: 0.85rem;
  border-bottom: 1px solid #e5e7eb;
  text-align: left;
  font-size: 0.95rem;
}

.pipeline-history__row {
  transition: background 0.15s ease;
  cursor: default;
}

.pipeline-history__row--active {
  background: #eef2ff;
}

.pipeline-detail__error {
  color: #b91c1c;
}

.pipeline-detail {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.pipeline-detail__header h4 {
  margin: 0;
  font-size: 1.1rem;
}

.pipeline-detail__cancel:hover {
  background: rgba(185, 28, 28, 0.08);
}

.pipeline-detail__workflow-header h5 {
  margin: 0;
  font-size: 1rem;
}

.pipeline-status {
  display: inline-flex;
  align-items: center;
  padding: 0.2rem 0.6rem;
  border-radius: 999px;
  font-size: 0.8rem;
  background: #e5e7eb;
  color: #374151;
}

.pipeline-status--success {
  background: #ecfdf5;
  color: #047857;
}

.pipeline-status--failure {
  background: #fee2e2;
  color: #b91c1c;
}

.pipeline-status--running {
  background: #e0f2fe;
  color: #0369a1;
}

.pipeline-status--pending {
  background: #fef3c7;
  color: #b45309;
}

.pipeline-status--killed {
  background: #fee2e2;
  color: #b91c1c;
}

.pipeline-log code {
  display: block;
  font-family: ui-monospace, SFMono-Regular, SFMono, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  line-height: 1.4;
}

.pipeline-history__empty {
  margin: 1rem 0 0;
  text-align: center;
  color: #6b7280;
}

.pipeline-error {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.75rem;
  color: #b91c1c;
}

.pipeline-modal {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 1rem;
}

.pipeline-modal__content {
  width: 560px;
  max-width: 100%;
  background: #ffffff;
  border-radius: 16px;
  box-shadow: 0 24px 60px rgba(15, 23, 42, 0.28);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.pipeline-modal__content--yaml {
  width: 85vw;
  max-width: 1200px;
  height: 85vh;
}

.pipeline-modal__content--settings {
  width: 720px;
  max-width: 95%;
}

.pipeline-modal__header,
.pipeline-modal__footer {
  padding: 1rem 1.5rem;
  border-bottom: 1px solid #e5e7eb;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.pipeline-modal__footer {
  border-bottom: none;
  border-top: 1px solid #e5e7eb;
  justify-content: flex-end;
  gap: 0.75rem;
}

.pipeline-modal__body {
  padding: 1.5rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.pipeline-modal__body--fill {
  flex: 1;
  padding: 0 1.5rem 1.5rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.pipeline-modal__textarea {
  width: 100%;
  flex: 1;
  border: 1px solid #1f2937;
  border-radius: 12px;
  background: #0f172a;
  color: #e2e8f0;
  font-family: 'Fira Code', 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 0.92rem;
  line-height: 1.6;
  padding: 1rem;
  resize: none;
}

.pipeline-modal__body--fill :deep(.CodeMirror) {
  flex: 1;
  height: 100% !important;
  background: #0f172a;
  color: #e2e8f0;
  font-size: 0.92rem;
  line-height: 1.6;
}

.pipeline-modal__hint {
  margin-bottom: 0.25rem;
  color: #4b5563;
  font-size: 0.875rem;
}

.pipeline-modal__close {
  border: none;
  background: transparent;
  font-size: 1.5rem;
  cursor: pointer;
}

.modal-field {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.modal-hint {
  margin: -0.2rem 0 0.2rem;
  font-size: 0.85rem;
  color: #6b7280;
}

.modal-field input {
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: 0.6rem 0.8rem;
  font-size: 0.95rem;
}

.run-variable-row {
  display: flex;
  gap: 0.4rem;
  align-items: center;
}

.run-variable-row input {
  flex: 1;
}

.run-variable-remove {
  padding: 0.4rem 0.8rem;
}

.run-variable-add {
  align-self: flex-start;
  padding: 0.35rem 0.8rem;
}

.modal-error {
  color: #b91c1c;
  font-size: 0.85rem;
}

.pipeline-settings {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.settings-block h4 {
  margin: 0 0 0.75rem;
}

.settings-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.settings-field input {
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: 0.55rem 0.75rem;
  font-size: 0.95rem;
}

.settings-field.checkbox {
  flex-direction: row;
  align-items: center;
  gap: 0.5rem;
}

.settings-flex {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.settings-empty {
  color: #6b7280;
  font-size: 0.9rem;
  margin-bottom: 0.5rem;
}

.settings-field__input {
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: 0.55rem 0.75rem;
  font-size: 0.95rem;
}

.settings-field__input--grow {
  flex: 1 1 220px;
}

.settings-hint {
  margin-top: 0.75rem;
  color: #6b7280;
  font-size: 0.85rem;
  line-height: 1.5;
}

.settings-hint code {
  background: rgba(37, 99, 235, 0.12);
  color: #1f2933;
  padding: 0 0.35rem;
  border-radius: 4px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
}

.settings-loading {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 200px;
  color: #6b7280;
  font-size: 0.95rem;
}

@media (max-width: 768px) {
  .pipeline-history__table th,
  .pipeline-history__table td {
    white-space: nowrap;
  }

  .pipeline-history__layout {
    flex-direction: column;
  }

  .pipeline-history__detail {
    padding-left: 0;
    border-left: none;
    width: 100%;
  }

  .pipeline-modal__content {
    width: 95%;
  }

  .pipeline-modal__content--yaml {
    width: 95%;
    height: 85vh;
  }
}
</style>
