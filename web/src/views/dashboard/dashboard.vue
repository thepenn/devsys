<template>
  <div class="dashboard-page">
    <div
      v-if="user"
      class="dashboard-account"
      @click.stop="toggleMenu"
    >
      <div class="dashboard-account__trigger">
        <img :src="avatarSrc" alt="avatar" class="dashboard-account__avatar">
        <div class="dashboard-account__meta">
          <span class="dashboard-account__name">{{ user.login }}</span>
          <span class="dashboard-account__email">{{ user.email || '未公开邮箱' }}</span>
        </div>
        <span class="dashboard-account__caret" />
      </div>
      <transition name="fade">
        <ul v-if="menuOpen" class="dashboard-account__menu" @click.stop>
          <li class="dashboard-account__menu-item" @click="viewProfile">
            个人信息
          </li>
          <li
            v-if="isAdmin"
            class="dashboard-account__menu-item"
            @click="openAdmin"
          >
            管理后台
          </li>
          <li
            class="dashboard-account__menu-item dashboard-account__menu-item--danger"
            @click="logout"
          >
            退出登录
          </li>
        </ul>
      </transition>
    </div>

    <main class="dashboard-main">
      <section v-if="error" class="alert alert--error dashboard-alert">
        <span>{{ error }}</span>
        <button class="button button--ghost" @click="error = ''">关闭</button>
      </section>

      <section v-if="loading" class="panel">
        <p>正在加载...</p>
      </section>

      <template v-else>
        <section
          v-if="user"
          class="panel panel--highlight dashboard-profile"
        >
          <div class="dashboard-user">
            <img v-if="user.avatar_url" :src="user.avatar_url" alt="avatar" class="dashboard-user__avatar">
            <div class="dashboard-user__info">
              <h2>{{ user.login }}</h2>
              <p class="dashboard-user__email">{{ user.email || '未公开邮箱' }}</p>
              <span v-if="providerLabel" class="dashboard-tag">{{ providerLabel }}</span>
            </div>
          </div>
        </section>

        <section class="panel">
          <div class="repo-controls">
            <div class="repo-controls__left">
              <div class="repo-search">
                <input
                  v-model="search"
                  class="input input--compact"
                  placeholder="搜索仓库"
                  @keyup.enter="applySearch"
                >
                <button class="button button--ghost" @click="applySearch">搜索</button>
              </div>
              <div class="repo-filter">
                <button
                  class="repo-filter__btn"
                  :class="{ 'repo-filter__btn--active': viewSynced }"
                  @click="setViewSynced(true)"
                >
                  已同步
                </button>
                <button
                  class="repo-filter__btn"
                  :class="{ 'repo-filter__btn--active': !viewSynced }"
                  @click="setViewSynced(false)"
                >
                  未同步
                </button>
              </div>
            </div>
            <button
              v-if="isAdmin"
              class="button button--ghost repo-sync"
              :disabled="syncing"
              @click="syncRepos"
            >
              {{ syncing ? '同步中…' : '同步仓库' }}
            </button>
          </div>

          <div
            v-if="repos.length"
            class="repo-table"
          >
            <div class="repo-table__header">
              <span class="repo-table__cell repo-table__cell--name">仓库</span>
              <span class="repo-table__cell repo-table__cell--visibility">可见性</span>
              <span class="repo-table__cell repo-table__cell--build">构建</span>
              <span class="repo-table__cell repo-table__cell--link">仓库地址</span>
              <span class="repo-table__cell repo-table__cell--actions">操作</span>
            </div>
            <div
              v-for="repo in repos"
              :key="repo.id || repo.full_name"
              class="repo-table__row"
            >
              <div class="repo-table__cell repo-table__cell--name">
                <router-link :to="projectRoute(repo)" class="repo-name-link">
                  {{ repo.full_name }}
                </router-link>
                <div v-if="repo.description" class="repo-description">
                  {{ repo.description }}
                </div>
              </div>
              <div class="repo-table__cell repo-table__cell--visibility">
                <span class="repo-visibility">{{ formatVisibility(repo.visibility) }}</span>
              </div>
              <div class="repo-table__cell repo-table__cell--build">
                <button class="button repo-table__build" @click="openRunModal(repo)">构建</button>
              </div>
              <div class="repo-table__cell repo-table__cell--link">
                <a
                  v-if="repo.forge_url"
                  :href="repo.forge_url"
                  class="button button--ghost repo-link-button"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  查看仓库
                </a>
                <span v-else class="repo-link repo-link--empty">暂无地址</span>
              </div>
              <div
                v-if="isAdmin"
                class="repo-table__cell repo-table__cell--actions"
              >
                <button
                  class="button button--ghost repo-table__action"
                  :disabled="repo.active || isRepoSyncing(repo)"
                  @click="syncOne(repo)"
                >
                  <template v-if="repo.active">
                    已同步
                  </template>
                  <template v-else>
                    {{ isRepoSyncing(repo) ? '同步中…' : '同步此仓库' }}
                  </template>
                </button>
              </div>
              <div
                v-else
                class="repo-table__cell repo-table__cell--actions repo-table__cell--disabled"
              >
                无权限
              </div>
            </div>
          </div>
          <p v-else class="empty">未找到仓库，尝试同步或调整搜索条件。</p>

          <div v-if="totalPages > 1" class="pagination">
            <button
              class="button button--ghost"
              :disabled="page === 1"
              @click="changePage(page - 1)"
            >
              上一页
            </button>
            <span class="pagination__info">第 {{ page }} / {{ totalPages }} 页（共 {{ total }} 个仓库）</span>
            <button
              class="button button--ghost"
              :disabled="page === totalPages"
              @click="changePage(page + 1)"
            >
              下一页
            </button>
          </div>

          <div v-if="runModalVisible" class="dashboard-modal" @click.self="closeRunModal">
            <div class="dashboard-modal__content">
              <header class="dashboard-modal__header">
                <h3>运行流水线</h3>
                <button class="dashboard-modal__close" @click="closeRunModal">×</button>
              </header>
              <section class="dashboard-modal__body">
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
                  <p class="modal-hint">这些键值将同步到流水线，可用于自定义参数。</p>
                  <div
                    v-for="(variable, idx) in runForm.variables"
                    :key="`dashboard-run-var-${idx}`"
                    class="run-variable-row"
                  >
                    <input v-model="variable.key" placeholder="变量名，如 TARGET_ENV">
                    <input v-model="variable.value" placeholder="变量值">
                    <button type="button" class="button button--ghost run-variable-remove" @click="removeRunVariable(idx)">删除</button>
                  </div>
                  <button type="button" class="button button--ghost run-variable-add" @click="addRunVariable">
                    + 添加变量
                  </button>
                </div>
                <p v-if="runFormError" class="modal-error">{{ runFormError }}</p>
              </section>
              <footer class="dashboard-modal__footer">
                <button class="button button--ghost" :disabled="running" @click="closeRunModal">取消</button>
                <button class="button" :disabled="running" @click="submitRun">
                  {{ running ? '提交中…' : '运行' }}
                </button>
              </footer>
            </div>
          </div>
        </section>
      </template>
    </main>
  </div>
</template>

<script>
import { clearToken, getToken } from '@/utils/auth'
import defaultAvatar from '@/assets/avatar/avatar.gif'
import { getCurrentUser } from '@/api/system/auth'
import { listRepositories, syncRepositories, syncRepository } from '@/api/project/repos'
import { triggerPipelineRun } from '@/api/project/pipeline'
import { normalizeError } from '@/utils/error'
import { emptyVariableRow, normalizeVariableRows, serializeVariableRows } from '@/utils/pipeline-run'

const PROVIDER_LABELS = {
  gitlab: 'GitLab',
  gitea: 'Gitea',
  gitee: 'Gitee'
}

export default {
  name: 'DashboardView',
  data() {
    return {
      token: getToken(),
      user: null,
      repos: [],
      total: 0,
      page: 1,
      perPage: 12,
      search: '',
      syncRepoMap: new Map(),
      loading: true,
      syncing: false,
      error: '',
      menuOpen: false,
      viewSynced: true,
      runModalVisible: false,
      runTargetRepo: null,
      runForm: {
        branch: 'main',
        commit: '',
        variables: [emptyVariableRow()]
      },
      runFormError: '',
      running: false
    }
  },
  computed: {
    totalPages() {
      if (this.perPage <= 0) {
        return 1
      }
      return Math.max(1, Math.ceil(this.total / this.perPage))
    },
    isAdmin() {
      return !!(this.user && this.user.admin)
    },
    avatarSrc() {
      if (this.user && this.user.avatar_url) {
        return this.user.avatar_url
      }
      return defaultAvatar
    },
    providerLabel() {
      if (!this.user || !this.user.provider) {
        return ''
      }
      const key = String(this.user.provider).toLowerCase()
      return PROVIDER_LABELS[key] || key.toUpperCase()
    }
  },
  created() {
    this.bootstrap()
  },
  mounted() {
    document.addEventListener('click', this.handleDocumentClick)
  },
  beforeDestroy() {
    document.removeEventListener('click', this.handleDocumentClick)
  },
  methods: {
    normalizeError,
    async bootstrap() {
      if (!this.token) {
        this.redirectToLogin()
        return
      }
      await this.loadAccount()
    },
    async loadAccount() {
      this.loading = true
      try {
        const user = await getCurrentUser()
        this.user = user
        await this.loadRepos(1)
      } catch (err) {
        this.handleAuthError(this.normalizeError(err, '加载账户信息失败'))
      } finally {
        this.loading = false
      }
    },
    async syncRepos() {
      if (!this.token || !this.isAdmin) return
      this.syncing = true
      this.error = ''
      try {
        await syncRepositories()
        await this.loadRepos(this.page)
      } catch (err) {
        this.handleAuthError(this.normalizeError(err, '同步仓库失败'))
      } finally {
        this.syncing = false
      }
    },
    async loadRepos(page = 1) {
      const params = {
        page,
        per_page: this.perPage,
        synced: this.viewSynced ? 'true' : 'false'
      }
      if (this.search.trim()) {
        params.search = this.search.trim()
      }
      const data = await listRepositories(params)
      if (data) {
        this.repos = data.items || []
        this.total = data.total || 0
        this.page = data.page || page
        this.perPage = data.per_page || this.perPage
      }
    },
    async changePage(newPage) {
      if (newPage === this.page || newPage < 1 || newPage > this.totalPages) {
        return
      }
      try {
        await this.loadRepos(newPage)
      } catch (err) {
        this.handleAuthError(this.normalizeError(err, '加载仓库失败'))
      }
    },
    async applySearch() {
      try {
        await this.loadRepos(1)
      } catch (err) {
        this.handleAuthError(this.normalizeError(err, '搜索仓库失败'))
      }
    },
    projectRoute(repo) {
      if (!repo || !repo.full_name) {
        return '/dashboard'
      }
      const [owner, name] = repo.full_name.split('/')
      return {
        name: 'ProjectPipeline',
        params: { owner, name }
      }
    },
    openRunModal(repo) {
      if (!repo) {
        return
      }
      this.runTargetRepo = repo
      const defaultBranch =
        (repo.branch && repo.branch.trim()) ||
        (repo.default_branch && repo.default_branch.trim()) ||
        'main'
      this.runForm = {
        branch: defaultBranch,
        commit: '',
        variables: normalizeVariableRows()
      }
      this.runFormError = ''
      this.running = false
      this.runModalVisible = true
    },
    closeRunModal() {
      if (this.running) {
        return
      }
      this.runModalVisible = false
      this.runTargetRepo = null
      this.runFormError = ''
      this.resetRunForm()
    },
    async submitRun() {
      const branch = (this.runForm.branch || '').trim()
      if (!branch) {
        this.runFormError = '构建分支为必填项'
        return
      }
      if (!this.runTargetRepo || !this.runTargetRepo.id) {
        this.runFormError = '无法识别仓库，请稍后重试'
        return
      }
      this.runFormError = ''
      this.running = true
      let result = null
      try {
        const payload = {
          branch,
          commit: (this.runForm.commit || '').trim()
        }
        const variablesPayload = serializeVariableRows(this.runForm.variables)
        if (variablesPayload) {
          payload.variables = variablesPayload
        }
        result = await triggerPipelineRun(this.runTargetRepo.id, payload)
      } catch (err) {
        const error = this.normalizeError(err, '触发流水线失败')
        if (this.isUnauthorizedError(error)) {
          this.handleAuthError(error)
        } else {
          this.runFormError = error.message || '触发流水线失败'
        }
      } finally {
        this.running = false
      }
      if (result) {
        const targetRepo = this.runTargetRepo
        const route = targetRepo ? this.projectRoute(targetRepo) : null
        this.closeRunModal()
        if (route) {
          const query = result.id ? { highlight: String(result.id) } : undefined
          this.$router.push({
            name: route.name,
            params: route.params,
            query
          })
        }
      }
    },
    resetRunForm() {
      this.runForm = {
        branch: 'main',
        commit: '',
        variables: normalizeVariableRows()
      }
    },
    addRunVariable() {
      this.runForm.variables.push(emptyVariableRow())
    },
    removeRunVariable(index) {
      if (this.runForm.variables.length <= 1) {
        this.runForm.variables.splice(0, 1, emptyVariableRow())
        return
      }
      this.runForm.variables.splice(index, 1)
    },
    setViewSynced(value) {
      if (this.viewSynced === value) {
        return
      }
      this.viewSynced = value
      this.page = 1
      this.loadRepos(1).catch(err => {
        this.handleAuthError(this.normalizeError(err, '加载仓库失败'))
      })
    },
    async syncOne(repo) {
      if (!this.token || !this.isAdmin || !repo || !repo.forge_remote_id || repo.active) {
        return
      }
      this.syncRepoMap.set(repo.forge_remote_id, true)
      this.syncRepoMap = new Map(this.syncRepoMap)
      try {
        await syncRepository(repo.forge_remote_id)
        await this.loadRepos(this.page)
      } catch (err) {
        this.handleAuthError(this.normalizeError(err, '同步仓库失败'))
      } finally {
        this.syncRepoMap.delete(repo.forge_remote_id)
        this.syncRepoMap = new Map(this.syncRepoMap)
      }
    },
    isRepoSyncing(repo) {
      if (!repo || !repo.forge_remote_id) {
        return false
      }
      return this.syncRepoMap.get(repo.forge_remote_id) === true
    },
    formatVisibility(value) {
      switch (value) {
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
    handleAuthError(err) {
      if (this.isUnauthorizedError(err)) {
        this.redirectToLogin(err.message)
        return
      }
      this.error = err.message || '操作失败'
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
    redirectToLogin(message) {
      clearToken()
      this.token = ''
      this.user = null
      this.repos = []
      const query = message ? { error: message } : undefined
      this.$router.replace({ path: '/login', query })
    },
    logout() {
      this.closeMenu()
      this.redirectToLogin()
    },
    toggleMenu() {
      this.menuOpen = !this.menuOpen
    },
    closeMenu() {
      this.menuOpen = false
    },
    handleDocumentClick() {
      if (this.menuOpen) {
        this.closeMenu()
      }
    },
    viewProfile() {
      this.closeMenu()
      this.$router.push('/profile')
    },
    openAdmin() {
      this.closeMenu()
      this.$router.push('/admin')
    }
  }
}
</script>

<style scoped>
.dashboard-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}

.dashboard-account {
  position: relative;
  align-self: flex-end;
  margin: 1.5rem 2rem 0;
}

.dashboard-account__trigger {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.4rem 0.75rem;
  border: 1px solid rgba(148, 163, 184, 0.4);
  border-radius: 999px;
  background: rgba(249, 250, 251, 0.95);
  box-shadow: 0 8px 20px rgba(15, 23, 42, 0.08);
  cursor: pointer;
  transition: box-shadow 0.2s ease, border-color 0.2s ease, transform 0.2s ease;
}

.dashboard-account__trigger:hover {
  border-color: rgba(79, 70, 229, 0.45);
  box-shadow: 0 10px 28px rgba(79, 70, 229, 0.18);
  transform: translateY(-1px);
}

.dashboard-account__avatar {
  width: 42px;
  height: 42px;
  border-radius: 50%;
  object-fit: cover;
  border: 2px solid rgba(79, 70, 229, 0.25);
}

.dashboard-account__meta {
  display: flex;
  flex-direction: column;
  line-height: 1.2;
}

.dashboard-account__name {
  font-weight: 600;
  font-size: 0.95rem;
  color: #1f2933;
}

.dashboard-account__email {
  font-size: 0.75rem;
  color: #6b7280;
}

.dashboard-account__caret {
  margin-left: 0.4rem;
  border-left: 5px solid transparent;
  border-right: 5px solid transparent;
  border-top: 6px solid #4b5563;
}

.dashboard-account__menu {
  position: absolute;
  right: 0;
  top: calc(100% + 12px);
  width: 180px;
  padding: 0.5rem 0;
  border-radius: 12px;
  list-style: none;
  margin: 0;
  background: #ffffff;
  border: 1px solid #e5e7eb;
  box-shadow: 0 18px 40px rgba(15, 23, 42, 0.12);
  z-index: 20;
}

.dashboard-account__menu::before {
  content: '';
  position: absolute;
  right: 24px;
  top: -7px;
  width: 14px;
  height: 14px;
  background: #ffffff;
  border-left: 1px solid #e5e7eb;
  border-top: 1px solid #e5e7eb;
  transform: rotate(45deg);
  z-index: 0;
}

.dashboard-account__menu-item {
  padding: 0.65rem 1.25rem;
  font-size: 0.9rem;
  color: #1f2933;
  cursor: pointer;
  transition: background 0.2s ease, color 0.2s ease;
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.dashboard-account__menu-item:hover {
  background: rgba(79, 70, 229, 0.08);
  color: #4338ca;
}

.dashboard-account__menu-item--danger {
  color: #b91c1c;
}

.dashboard-account__menu-item--danger:hover {
  background: rgba(248, 113, 113, 0.12);
  color: #b91c1c;
}

.dashboard-main {
  flex: 1;
  padding: 0 2rem 2.5rem;
  max-width: 1150px;
  width: 100%;
  margin: 2rem auto 0;
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.dashboard-alert {
  align-self: center;
  max-width: 680px;
  width: 100%;
}

.dashboard-profile {
  margin-bottom: 0.5rem;
}

.dashboard-user {
  display: flex;
  align-items: center;
  gap: 1.25rem;
}

.dashboard-user__avatar {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  object-fit: cover;
}

.dashboard-user__info h2 {
  margin: 0;
  font-size: 1.4rem;
}

.dashboard-user__email {
  margin: 0.25rem 0 0;
  color: #6b7280;
}

.dashboard-tag {
  display: inline-block;
  margin-top: 0.5rem;
  padding: 0.1rem 0.5rem;
  font-size: 0.75rem;
  border-radius: 999px;
  background: #eef2ff;
  color: #4338ca;
}

.repo-controls {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 0.75rem;
  margin-bottom: 1rem;
}

.repo-controls__left {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-wrap: wrap;
  flex: 1;
  min-width: 0;
}

.repo-search {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex: 1;
  min-width: 220px;
}

.repo-search .input {
  flex: 1;
}

.repo-sync {
  white-space: nowrap;
}

.repo-filter {
  display: inline-flex;
  padding: 0.2rem;
  border-radius: 999px;
  background: #eef2ff;
  gap: 0.2rem;
}

.input--compact {
  min-width: 0;
}

.repo-filter__btn {
  border: none;
  background: transparent;
  color: #4b5563;
  font-size: 0.85rem;
  padding: 0.35rem 0.9rem;
  border-radius: 999px;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}

.repo-filter__btn--active {
  background: #2563eb;
  color: #ffffff;
  box-shadow: 0 6px 18px rgba(37, 99, 235, 0.25);
}

.repo-filter__btn:not(.repo-filter__btn--active):hover {
  background: rgba(37, 99, 235, 0.1);
}

.input {
  flex: 1;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: 0.6rem 0.8rem;
  font-size: 0.95rem;
  outline: none;
  transition: border-color 0.2s ease;
}

.input:focus {
  border-color: #2563eb;
}

.repo-table {
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 10px 24px rgba(15, 23, 42, 0.04);
}

.repo-table__header,
.repo-table__row {
  display: grid;
  grid-template-columns: minmax(0, 2.5fr) minmax(110px, 0.8fr) minmax(120px, 0.9fr) minmax(140px, 1fr) minmax(220px, 1.2fr);
  align-items: center;
  padding: 1rem 1.25rem;
  gap: 1rem;
}

.repo-table__header {
  background: #f8fafc;
  font-weight: 600;
  color: #1f2933;
  font-size: 0.9rem;
}

.repo-table__row {
  background: #ffffff;
  border-top: 1px solid #e5e7eb;
  transition: background 0.15s ease;
}

.repo-table__row:hover {
  background: #f9fafb;
}

.repo-table__cell {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: #4b5563;
  font-size: 0.95rem;
}

.repo-table__cell--name {
  flex-direction: column;
  align-items: flex-start;
  color: #1f2933;
}

.repo-name-link {
  font-weight: 600;
  font-size: 1.05rem;
  color: #2563eb;
  text-decoration: none;
  transition: color 0.15s ease;
}

.repo-name-link:hover {
  color: #1d4ed8;
}

.repo-name {
  font-weight: 600;
  font-size: 1.05rem;
  word-break: break-all;
}

.repo-description {
  font-size: 0.85rem;
  color: #6b7280;
  margin-top: 0.25rem;
}

.repo-visibility {
  padding: 0.2rem 0.75rem;
  border-radius: 999px;
  background: #eef2ff;
  color: #4338ca;
  font-size: 0.8rem;
  font-weight: 600;
}

.repo-link {
  color: #2563eb;
  text-decoration: none;
  font-weight: 500;
}

.repo-link--empty {
  color: #9ca3af;
}

.repo-table__cell--build,
.repo-table__cell--link {
  justify-content: center;
}

.repo-link-button {
  min-width: 110px;
  justify-content: center;
}

.repo-link-button:hover {
  text-decoration: none;
}

.repo-table__build {
  min-width: 100px;
}

.repo-table__cell--actions {
  justify-content: flex-end;
}

.repo-table__cell--disabled {
  color: #9ca3af;
  justify-content: flex-end;
}

.repo-table__action {
  min-width: 120px;
}

.dashboard-modal {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1.5rem;
  background: rgba(15, 23, 42, 0.45);
  backdrop-filter: blur(2px);
  z-index: 2000;
}

.dashboard-modal__content {
  width: min(480px, 100%);
  background: #ffffff;
  border-radius: 16px;
  box-shadow: 0 30px 80px rgba(15, 23, 42, 0.18);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.dashboard-modal__header,
.dashboard-modal__footer {
  padding: 1.25rem 1.5rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.dashboard-modal__header h3 {
  margin: 0;
  font-size: 1.2rem;
  font-weight: 600;
  color: #111827;
}

.dashboard-modal__body {
  padding: 0 1.5rem 1.5rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.dashboard-modal__footer {
  justify-content: flex-end;
  gap: 0.75rem;
}

.dashboard-modal__close {
  border: none;
  background: transparent;
  font-size: 1.5rem;
  line-height: 1;
  cursor: pointer;
  color: #9ca3af;
  transition: color 0.2s ease;
}

.dashboard-modal__close:hover {
  color: #4b5563;
}

.modal-field {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.modal-field span {
  font-size: 0.9rem;
  color: #4b5563;
}

.modal-field input {
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: 0.6rem 0.75rem;
  font-size: 0.95rem;
  outline: none;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

.modal-field input:focus {
  border-color: #2563eb;
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.12);
}

.modal-hint {
  margin: -0.15rem 0 0.2rem;
  font-size: 0.8rem;
  color: #6b7280;
}

.run-variable-row {
  display: flex;
  gap: 0.4rem;
  align-items: center;
}

.run-variable-row input {
  flex: 1;
}

.run-variable-remove,
.run-variable-add {
  align-self: flex-start;
  padding: 0.35rem 0.75rem;
}

.modal-error {
  margin: 0;
  font-size: 0.85rem;
  color: #dc2626;
}

.empty {
  text-align: center;
  color: #6b7280;
}

.pagination {
  margin-top: 1.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1rem;
}

.pagination__info {
  color: #6b7280;
  font-size: 0.95rem;
}

@media (max-width: 768px) {
  .dashboard-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 1rem;
  }

  .dashboard-main {
    padding: 1.5rem;
  }

  .panel {
    padding: 1.5rem;
  }

  .repo-controls {
    flex-direction: column;
  }

  .repo-table {
    border-radius: 10px;
  }

  .repo-table__header {
    display: none;
  }

  .repo-table__row {
    grid-template-columns: 1fr;
    padding: 1rem;
    gap: 0.75rem;
  }

  .repo-table__cell {
    justify-content: space-between;
  }

  .repo-table__cell--name {
    gap: 0.35rem;
  }

  .repo-table__cell--actions {
    justify-content: flex-start;
  }

  .dashboard-account__trigger {
    padding: 0.3rem 0.6rem;
    gap: 0.5rem;
  }

  .dashboard-account__avatar {
    width: 36px;
    height: 36px;
  }
}
</style>
