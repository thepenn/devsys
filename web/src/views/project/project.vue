<template>
  <div class="project-wrapper">
    <aside class="project-nav">
      <div class="project-nav__header">
        <router-link to="/dashboard" class="project-nav__back">← 返回仓库</router-link>
        <h2>{{ projectTitle }}</h2>
        <p class="project-nav__subtitle">{{ repo && repo.visibility ? formatVisibility(repo.visibility) : '加载中…' }}</p>
      </div>
      <nav class="project-nav__links">
        <router-link
          v-for="item in navItems"
          :key="item.name"
          :to="item.to()"
          class="project-nav__link"
          :class="{ 'project-nav__link--active': isLinkActive(item) }"
        >
          <span>{{ item.label }}</span>
        </router-link>
      </nav>
    </aside>

    <main class="project-main">
      <section v-if="error" class="project-error panel">
        <h3>加载失败</h3>
        <p>{{ error }}</p>
        <button class="button button--ghost" @click="loadRepo">重试</button>
      </section>

      <section v-else class="project-content">
        <header v-if="repo" class="project-header panel">
          <div class="project-header__info">
            <h1>{{ repo.full_name }}</h1>
            <p class="project-header__meta">
              <span>Forge ID：{{ repo.forge_remote_id || '未知' }}</span>
              <span>默认分支：{{ repo.branch || '未设置' }}</span>
              <span>可见性：{{ formatVisibility(repo.visibility) }}</span>
            </p>
          </div>
          <div class="project-header__header-actions" @click.stop>
            <span class="project-tag" :class="repo.visibility">{{ formatVisibility(repo.visibility) }}</span>
            <span v-if="repo.active" class="project-tag project-tag--success">已同步</span>
            <span v-else class="project-tag project-tag--warning">未同步</span>
            <div v-if="isPipelineRoute" class="project-header__pipeline-actions">
              <button class="button" :disabled="isPipelineBusy" @click="handleRunPipeline">运行流水线</button>
              <div v-if="isAdmin" class="project-header__dropdown" @click.stop>
                <button class="button button--ghost" @click="togglePipelineMenu">更多操作</button>
                <div v-if="pipelineMenuOpen" class="project-header__dropdown-menu">
                  <button type="button" @click="openPipelineEditor">编辑 YAML</button>
                  <button type="button" @click="openPipelineDockerfile">编辑 Dockerfile</button>
                  <button type="button" @click="openPipelineSettings">流水线设置</button>
                </div>
              </div>
            </div>
          </div>
        </header>

        <section v-if="loading" class="panel project-loading">
          <p>加载项目数据中…</p>
        </section>

        <router-view
          v-else
          v-slot="{ Component }"
        >
          <component
            :is="Component"
            ref="activeChild"
            :project="repo"
            :is-admin="isAdmin"
            @pipeline-expose="handlePipelineExpose"
            @refresh="loadRepo"
          />
        </router-view>
      </section>
    </main>
  </div>
</template>

<script>
import { clearToken, getToken } from '@/utils/auth'
import { getCurrentUser } from '@/api/system/auth'
import { listRepositories } from '@/api/project/repos'
import { normalizeError } from '@/utils/error'

export default {
  name: 'ProjectLayout',
  provide() {
    return {
      projectPipelineExpose: payload => this.handlePipelineExpose(payload)
    }
  },
  data() {
    const { owner, name } = this.$route.params
    return {
      owner,
      name,
      token: getToken(),
      repo: null,
      user: null,
      loading: true,
      error: '',
      pipelineMenuOpen: false,
      pipelineActions: null,
      navItems: [
        {
          name: 'ProjectPipeline',
          label: '流水线构建',
          to: () => this.projectRoute('pipeline')
        },
        {
          name: 'ProjectDeployment',
          label: '部署发布',
          to: () => this.projectRoute('deployment')
        },
        {
          name: 'ProjectMonitor',
          label: '监控告警',
          to: () => this.projectRoute('monitor')
        }
      ]
    }
  },
  computed: {
    projectTitle() {
      if (this.repo) return this.repo.name
      return `${this.owner}/${this.name}`
    },
    isPipelineRoute() {
      const name = (this.$route && this.$route.name) || ''
      return name === 'ProjectPipeline' || name === 'ProjectPipelineRunDetail'
    },
    isAdmin() {
      return !!(this.user && this.user.admin)
    },
    pipelineComponent() {
      const ref = this.$refs.activeChild
      if (!ref) return null
      if (Array.isArray(ref)) {
        return ref[0] || null
      }
      return ref
    },
    isPipelineBusy() {
      if (this.pipelineActions && typeof this.pipelineActions.isBusy === 'function') {
        return this.pipelineActions.isBusy()
      }
      const comp = this.pipelineComponent
      if (!comp) return false
      const exposed = comp.exposed || null
      if (exposed && typeof exposed.isBusy === 'function') {
        return exposed.isBusy()
      }
      return Boolean(comp.running || comp.loadingRuns)
    }
  },
  watch: {
    '$route.params'(next, prev) {
      if (next.owner !== prev.owner || next.name !== prev.name) {
        this.owner = next.owner
        this.name = next.name
        this.loadRepo()
      }
    },
    '$route.name'() {
      this.pipelineMenuOpen = false
      if (!this.isPipelineRoute) {
        this.pipelineActions = null
      }
    }
  },
  created() {
    this.bootstrap()
    document.addEventListener('click', this.handleGlobalClick)
  },
  beforeDestroy() {
    document.removeEventListener('click', this.handleGlobalClick)
  },
  methods: {
    normalizeError,
    async bootstrap() {
      if (!this.token) {
        this.redirectToLogin('请先登录')
        return
      }
      this.loading = true
      try {
        await this.loadAccount()
        await this.loadRepo()
      } finally {
        this.loading = false
      }
    },
    isLinkActive(item) {
      if (!item || !item.name) return false
      const current = (this.$route && this.$route.name) || ''
      if (item.name === 'ProjectPipeline') {
        return current === 'ProjectPipeline' || current === 'ProjectPipelineRunDetail'
      }
      return current === item.name
    },
    projectRoute(child) {
      return { name: `Project${child.charAt(0).toUpperCase()}${child.slice(1)}`, params: { owner: this.owner, name: this.name }}
    },
    async loadRepo() {
      this.error = ''
      try {
        const search = `${this.owner}/${this.name}`
        const data = await listRepositories({ search, per_page: 1, page: 1 })
        const repo = (data.items && data.items[0]) || null
        if (!repo) {
          this.error = '未找到对应项目，可能尚未同步。'
          this.repo = null
        } else {
          this.repo = repo
        }
      } catch (err) {
        const error = this.normalizeError(err, '加载项目失败')
        if (this.isUnauthorizedError(error)) {
          this.redirectToLogin(error.message)
          return
        }
        this.error = error.message || '加载项目失败'
      }
    },
    async loadAccount() {
      try {
        this.user = await getCurrentUser()
      } catch (err) {
        const error = this.normalizeError(err, '加载账户信息失败')
        this.user = null
        if (this.isUnauthorizedError(error)) {
          this.redirectToLogin(error.message)
          return
        }
        this.error = error.message || '加载账户信息失败'
      }
    },
    redirectToLogin(message) {
      clearToken()
      this.$router.replace({ path: '/login', query: { error: message }})
    },
    formatVisibility(value) {
      switch ((value || '').toLowerCase()) {
        case 'public':
          return '公开'
        case 'internal':
          return '内部'
        case 'private':
          return '私有'
        default:
          return value || '未知'
      }
    },
    handleGlobalClick() {
      this.pipelineMenuOpen = false
    },
    handlePipelineExpose(payload) {
      this.pipelineActions = payload || null
      const comp = this.pipelineComponent
      if (comp) {
        comp.exposed = payload || null
      }
    },
    togglePipelineMenu() {
      this.pipelineMenuOpen = !this.pipelineMenuOpen
    },
    isUnauthorizedError(err) {
      if (!err) return false
      const status = typeof err.status === 'number' ? err.status : (err.response && err.response.status)
      if (status === 401) return true
      if (typeof err.message === 'string' && /(401|未授权|unauthorized)/i.test(err.message)) {
        return true
      }
      return false
    },
    handleRunPipeline() {
      const comp = this.pipelineComponent
      this.pipelineMenuOpen = false
      let handled = false
      if (this.pipelineActions && typeof this.pipelineActions.openRunModal === 'function') {
        const result = this.pipelineActions.openRunModal()
        handled = true
        if (result && typeof result.catch === 'function') {
          result.catch(err => console.warn('openRunModal failed', err))
        }
      } else if (comp) {
        if (comp.exposed && typeof comp.exposed.openRunModal === 'function') {
          const result = comp.exposed.openRunModal()
          handled = true
          if (result && typeof result.catch === 'function') {
            result.catch(err => console.warn('openRunModal failed', err))
          }
        } else if (typeof comp.openRunModal === 'function') {
          const result = comp.openRunModal()
          handled = true
          if (result && typeof result.catch === 'function') {
            result.catch(err => console.warn('openRunModal failed', err))
          }
        }
      }
      if (!handled) {
        console.warn('pipeline run modal handler not available')
        this.enqueuePipelineAction('run')
      }
    },
    openPipelineEditor() {
      const comp = this.pipelineComponent
      this.pipelineMenuOpen = false
      let handled = false
      if (this.pipelineActions && typeof this.pipelineActions.openEditor === 'function') {
        const result = this.pipelineActions.openEditor()
        handled = true
        if (result && typeof result.catch === 'function') {
          result.catch(err => console.warn('openEditor failed', err))
        }
      } else if (comp) {
        if (comp.exposed && typeof comp.exposed.openEditor === 'function') {
          const result = comp.exposed.openEditor()
          handled = true
          if (result && typeof result.catch === 'function') {
            result.catch(err => console.warn('openEditor failed', err))
          }
        } else if (typeof comp.openEditor === 'function') {
          const result = comp.openEditor()
          handled = true
          if (result && typeof result.catch === 'function') {
            result.catch(err => console.warn('openEditor failed', err))
          }
        }
      }
      if (!handled) {
        console.warn('pipeline editor handler not available')
        this.enqueuePipelineAction('editor')
      }
    },
    openPipelineSettings() {
      const comp = this.pipelineComponent
      this.pipelineMenuOpen = false
      let handled = false
      if (this.pipelineActions && typeof this.pipelineActions.openSettings === 'function') {
        const result = this.pipelineActions.openSettings()
        handled = true
        if (result && typeof result.catch === 'function') {
          result.catch(err => console.warn('openSettings failed', err))
        }
      } else if (comp) {
        if (comp.exposed && typeof comp.exposed.openSettings === 'function') {
          const result = comp.exposed.openSettings()
          handled = true
          if (result && typeof result.catch === 'function') {
            result.catch(err => console.warn('openSettings failed', err))
          }
        } else if (typeof comp.openSettings === 'function') {
          const result = comp.openSettings()
          handled = true
          if (result && typeof result.catch === 'function') {
            result.catch(err => console.warn('openSettings failed', err))
          }
        }
      }
      if (!handled) {
        console.warn('pipeline settings handler not available')
      }
      this.enqueuePipelineAction('settings')
    },
    openPipelineDockerfile() {
      const comp = this.pipelineComponent
      this.pipelineMenuOpen = false
      let handled = false
      if (this.pipelineActions && typeof this.pipelineActions.openDockerfile === 'function') {
        const result = this.pipelineActions.openDockerfile()
        handled = true
        if (result && typeof result.catch === 'function') {
          result.catch(err => console.warn('openDockerfile failed', err))
        }
      } else if (comp) {
        if (comp.exposed && typeof comp.exposed.openDockerfile === 'function') {
          const result = comp.exposed.openDockerfile()
          handled = true
          if (result && typeof result.catch === 'function') {
            result.catch(err => console.warn('openDockerfile failed', err))
          }
        } else if (typeof comp.openDockerfile === 'function') {
          const result = comp.openDockerfile()
          handled = true
          if (result && typeof result.catch === 'function') {
            result.catch(err => console.warn('openDockerfile failed', err))
          }
        }
      }
      if (!handled) {
        console.warn('pipeline dockerfile handler not available')
      }
      this.enqueuePipelineAction('dockerfile')
    },
    enqueuePipelineAction(action, extraQuery = {}) {
      const owner = this.owner
      const name = this.name
      const query = {
        ...this.$route.query,
        ...extraQuery,
        action,
        _actionTs: `${Date.now()}`
      }
      const target = {
        name: 'ProjectPipeline',
        params: { owner, name },
        query
      }
      const nav = this.$router.push(target)
      if (nav && typeof nav.catch === 'function') {
        nav.catch(() => {})
      }
    }
  }
}
</script>

<style scoped>
.project-wrapper {
  display: flex;
  min-height: 100vh;
  background: #f7f8fa;
}

.project-nav {
  width: 260px;
  background: #ffffff;
  border-right: 1px solid #e5e7eb;
  padding: 2rem 1.5rem;
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.project-nav__header h2 {
  margin: 0.75rem 0 0;
  font-size: 1.3rem;
  font-weight: 600;
  color: #111827;
}

.project-nav__subtitle {
  margin: 0.25rem 0 0;
  color: #6b7280;
  font-size: 0.9rem;
}

.project-nav__back {
  color: #2563eb;
  text-decoration: none;
  font-size: 0.9rem;
}

.project-nav__links {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.project-nav__link {
  padding: 0.65rem 0.75rem;
  border-radius: 8px;
  color: #4b5563;
  text-decoration: none;
  transition: background 0.15s ease, color 0.15s ease;
}

.project-nav__link:hover {
  background: rgba(37, 99, 235, 0.08);
  color: #2563eb;
}

.project-nav__link--active {
  background: #2563eb;
  color: #ffffff;
}

.project-main {
  flex: 1;
  padding: 2rem;
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.panel {
  background: #ffffff;
  border-radius: 16px;
  padding: 1.5rem;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.04);
}

.project-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
}

.project-header__info h1 {
  margin: 0;
  font-size: 1.6rem;
  font-weight: 600;
}

.project-header__meta {
  margin: 0.5rem 0 0;
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  color: #6b7280;
  font-size: 0.9rem;
}

.project-header__header-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.project-header__pipeline-actions {
  display: inline-flex;
  align-items: center;
  gap: 0.75rem;
}

.project-header__dropdown {
  position: relative;
}

.project-header__dropdown-menu {
  position: absolute;
  right: 0;
  top: calc(100% + 6px);
  background: #ffffff;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  box-shadow: 0 12px 30px rgba(15, 23, 42, 0.12);
  padding: 0.3rem 0;
  display: flex;
  flex-direction: column;
  min-width: 150px;
  z-index: 30;
}

.project-header__dropdown-menu button {
  padding: 0.55rem 1rem;
  background: none;
  border: none;
  text-align: left;
  font-size: 0.9rem;
  color: #1f2933;
  cursor: pointer;
}

.project-header__dropdown-menu button:hover {
  background: rgba(37, 99, 235, 0.08);
}

.project-tag {
  display: inline-flex;
  align-items: center;
  padding: 0.2rem 0.75rem;
  border-radius: 999px;
  font-size: 0.8rem;
  background: #eef2ff;
  color: #4338ca;
}

.project-tag--success {
  background: #ecfdf5;
  color: #047857;
}

.project-tag--warning {
  background: #fef3c7;
  color: #b45309;
}

.project-content {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.project-loading {
  text-align: center;
  color: #6b7280;
}

.project-error h3 {
  margin: 0 0 0.5rem;
}

@media (max-width: 960px) {
  .project-wrapper {
    flex-direction: column;
  }

  .project-nav {
    width: 100%;
    border-right: none;
    border-bottom: 1px solid #e5e7eb;
  }
}
</style>
