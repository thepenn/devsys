<template>
  <div class="profile-page">
    <header class="profile-header">
      <button class="button button--ghost profile-back" @click="$router.back()">
        返回
      </button>
      <h1>个人信息</h1>
    </header>

    <main class="profile-main">
      <section v-if="error" class="alert alert--error profile-alert">
        <span>{{ error }}</span>
        <button class="button button--ghost" @click="error = ''">关闭</button>
      </section>

      <section v-if="loading" class="panel profile-panel">
        <p>正在加载...</p>
      </section>

      <template v-else>
        <section v-if="user" class="panel profile-panel profile-summary">
          <div class="profile-summary__avatar">
            <img :src="avatarSrc" alt="avatar">
          </div>
          <div class="profile-summary__info">
            <h2>{{ safeUser.login }}</h2>
            <p class="profile-summary__email">{{ safeUser.email || '未公开邮箱' }}</p>
            <span v-if="providerLabel" class="profile-tag">{{ providerLabel }}</span>
          </div>
        </section>

        <section class="panel profile-panel">
          <h3>账户详情</h3>
          <ul class="profile-details">
            <li>
              <span class="profile-details__label">登录名</span>
              <span class="profile-details__value">{{ safeUser.login || '-' }}</span>
            </li>
            <li>
              <span class="profile-details__label">邮箱</span>
              <span class="profile-details__value">{{ safeUser.email || '未公开邮箱' }}</span>
            </li>
            <li>
              <span class="profile-details__label">Forge ID</span>
              <span class="profile-details__value">{{ safeUser.forge_id || '-' }}</span>
            </li>
            <li>
              <span class="profile-details__label">管理员权限</span>
              <span class="profile-details__value">{{ safeUser.admin ? '是' : '否' }}</span>
            </li>
          </ul>
        </section>

        <section class="panel profile-panel profile-actions">
          <h3>快捷操作</h3>
          <div class="profile-action-grid">
            <button class="button button--ghost" @click="goDashboard">
              返回首页
            </button>
            <button
              v-if="safeUser.admin"
              class="button button--ghost"
              @click="syncRepos"
            >
              同步仓库
            </button>
            <button
              v-if="safeUser.admin"
              class="button button--ghost"
              @click="goAdmin"
            >
              管理后台
            </button>
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
import { syncRepositories } from '@/api/project/repos'
import { normalizeError } from '@/utils/error'

const PROVIDER_LABELS = {
  gitlab: 'GitLab',
  gitea: 'Gitea',
  gitee: 'Gitee'
}

export default {
  name: 'ProfileView',
  data() {
    return {
      token: getToken(),
      user: null,
      loading: true,
      syncing: false,
      error: ''
    }
  },
  computed: {
    safeUser() {
      return this.user || {}
    },
    avatarSrc() {
      if (this.user && this.user.avatar_url) {
        return this.user.avatar_url
      }
      return defaultAvatar
    },
    providerLabel() {
      if (!this.safeUser.provider) {
        return ''
      }
      const key = String(this.safeUser.provider).toLowerCase()
      return PROVIDER_LABELS[key] || key.toUpperCase()
    }
  },
  created() {
    this.bootstrap()
  },
  methods: {
    normalizeError,
    async bootstrap() {
      if (!this.token) {
        this.redirectToLogin()
        return
      }
      await this.loadProfile()
    },
    async loadProfile() {
      this.loading = true
      try {
        const user = await getCurrentUser()
        this.user = user
      } catch (err) {
        this.handleAuthError(this.normalizeError(err, '加载个人信息失败'))
      } finally {
        this.loading = false
      }
    },
    async syncRepos() {
      if (this.syncing || !this.safeUser.admin) return
      this.syncing = true
      try {
        await syncRepositories()
        this.$router.push('/dashboard')
      } catch (err) {
        this.handleAuthError(this.normalizeError(err, '同步仓库失败'))
      } finally {
        this.syncing = false
      }
    },
    goDashboard() {
      this.$router.push('/dashboard')
    },
    goAdmin() {
      if (!this.safeUser.admin) return
      this.$router.push('/admin')
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
      const query = message ? { error: message } : undefined
      this.$router.replace({ path: '/login', query })
    }
  }
}
</script>

<style scoped>
.profile-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  padding: 2rem;
  gap: 1.5rem;
  max-width: 960px;
  margin: 0 auto;
}

.profile-header {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.profile-header h1 {
  margin: 0;
  font-size: 1.6rem;
  font-weight: 600;
}

.profile-main {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.profile-alert {
  align-self: flex-start;
}

.profile-panel h3 {
  margin: 0 0 1rem;
  font-size: 1.05rem;
  font-weight: 600;
  color: #1f2933;
}

.profile-summary {
  display: flex;
  align-items: center;
  gap: 1.5rem;
}

.profile-summary__avatar img {
  width: 96px;
  height: 96px;
  border-radius: 50%;
  object-fit: cover;
  border: 3px solid rgba(79, 70, 229, 0.25);
}

.profile-summary__info h2 {
  margin: 0;
  font-size: 1.5rem;
}

.profile-summary__email {
  margin: 0.35rem 0 0;
  color: #6b7280;
}

.profile-tag {
  display: inline-block;
  margin-top: 0.5rem;
  padding: 0.2rem 0.65rem;
  border-radius: 999px;
  background: #eef2ff;
  color: #4338ca;
  font-size: 0.8rem;
  font-weight: 600;
}

.profile-details {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}

.profile-details li {
  display: flex;
  justify-content: space-between;
  font-size: 0.95rem;
  color: #374151;
}

.profile-details__label {
  color: #6b7280;
}

.profile-actions h3 {
  margin-bottom: 0.75rem;
}

.profile-action-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
}

.profile-back {
  padding: 0.5rem 1rem;
}

@media (max-width: 768px) {
  .profile-page {
    padding: 1.5rem 1rem;
  }

  .profile-summary {
    flex-direction: column;
    align-items: flex-start;
  }

  .profile-details li {
    flex-direction: column;
    align-items: flex-start;
    gap: 0.25rem;
  }

  .profile-action-grid {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
