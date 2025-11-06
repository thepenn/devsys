<template>
  <div class="admin-page">
    <header class="admin-header">
      <button class="button button--ghost admin-back" @click="$router.push('/dashboard')">
        返回首页
      </button>
      <h1>管理后台</h1>
    </header>

    <main class="admin-main">
      <section v-if="error" class="alert alert--error admin-alert">
        <span>{{ error }}</span>
        <button class="button button--ghost" @click="error = ''">关闭</button>
      </section>

      <section v-if="loading" class="panel admin-panel">
        <p>正在加载...</p>
      </section>

      <template v-else>
        <section class="panel admin-panel admin-summary">
          <div class="admin-summary__icon">🛡️</div>
          <div class="admin-summary__info">
            <h2>{{ safeUser.login }}</h2>
            <p class="admin-summary__email">{{ safeUser.email || '未公开邮箱' }}</p>
            <div class="admin-summary__meta">
              <span class="admin-chip">管理员</span>
              <span v-if="providerLabel" class="admin-chip">{{ providerLabel }}</span>
            </div>
          </div>
        </section>

        <section class="panel admin-panel">
          <h3>管理入口</h3>
          <div class="admin-action-grid">
            <div
              v-for="action in actions"
              :key="action.key"
              class="admin-action"
            >
              <div class="admin-action__icon">{{ action.icon }}</div>
              <div class="admin-action__body">
                <h4>{{ action.title }}</h4>
                <p>{{ action.description }}</p>
              </div>
              <button
                class="button button--ghost admin-action__button"
                :disabled="action.pending"
                @click="handleAction(action)"
              >
                {{ action.label }}
              </button>
            </div>
          </div>
        </section>
      </template>
    </main>
  </div>
</template>

<script>
import { clearToken, getToken } from '@/utils/auth'

import { getCurrentUser } from '@/api/system/auth'

const PROVIDER_LABELS = {
  gitlab: 'GitLab',
  github: 'GitHub',
  gitea: 'Gitea',
  gitee: 'Gitee'
}

export default {
  name: 'AdminView',
  data() {
    return {
      token: getToken(),
      user: null,
      loading: true,
      error: '',
      actions: [
        {
          key: 'certificates',
          title: '凭证管理',
          description: '集中维护 Git、Docker、数据库等访问凭证。',
          icon: '🔐',
          label: '进入管理',
          pending: false,
          route: '/admin/certificates'
        }
        // 更多功能入口后续开放
      ]
    }
  },
  computed: {
    safeUser() {
      return this.user || {}
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
    async bootstrap() {
      if (!this.token) {
        this.redirectToLogin()
        return
      }
      await this.ensureAdmin()
    },
    async ensureAdmin() {
      this.loading = true
      try {
        const user = await getCurrentUser()
        if (!user || !user.admin) {
          this.$router.replace('/dashboard')
          return
        }
        this.user = user
      } catch (err) {
        this.handleAuthError(this.normalizeError(err, '加载管理员信息失败'))
      } finally {
        this.loading = false
      }
    },
    async handleAction(action) {
      if (action.pending) {
        window.alert('该功能即将上线，敬请期待。')
        return
      }

      if (action.route) {
        this.$router.push(action.route)
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
.admin-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  padding: 2rem;
  gap: 1.5rem;
  max-width: 1100px;
  margin: 0 auto;
}

.admin-header {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.admin-header h1 {
  margin: 0;
  font-size: 1.7rem;
  font-weight: 600;
}

.admin-main {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.admin-alert {
  align-self: flex-start;
}

.admin-panel h3 {
  margin: 0 0 1rem;
  font-size: 1.05rem;
  font-weight: 600;
  color: #1f2933;
}

.admin-summary {
  display: flex;
  align-items: center;
  gap: 1.5rem;
}

.admin-summary__icon {
  font-size: 2.5rem;
}

.admin-summary__info h2 {
  margin: 0;
  font-size: 1.4rem;
}

.admin-summary__email {
  margin: 0.3rem 0 0;
  color: #6b7280;
}

.admin-summary__meta {
  margin-top: 0.6rem;
  display: flex;
  gap: 0.5rem;
}

.admin-chip {
  padding: 0.2rem 0.65rem;
  border-radius: 999px;
  background: #eef2ff;
  color: #4338ca;
  font-size: 0.8rem;
  font-weight: 600;
}

.admin-action-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 1rem;
}

.admin-action {
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.admin-action:hover {
  border-color: rgba(79, 70, 229, 0.35);
  box-shadow: 0 12px 32px rgba(79, 70, 229, 0.12);
}

.admin-action__icon {
  font-size: 1.8rem;
}

.admin-action__body h4 {
  margin: 0;
  font-size: 1.05rem;
  font-weight: 600;
}

.admin-action__body p {
  margin: 0.35rem 0 0;
  color: #6b7280;
  font-size: 0.9rem;
}

.admin-action__button {
  align-self: flex-start;
  padding: 0.45rem 1rem;
}

.admin-guides ul {
  margin: 0;
  padding-left: 1.2rem;
  color: #4b5563;
  line-height: 1.6;
}

.admin-back {
  padding: 0.5rem 1rem;
}

@media (max-width: 768px) {
  .admin-page {
    padding: 1.5rem 1rem;
  }

  .admin-summary {
    flex-direction: column;
    align-items: flex-start;
  }

  .admin-action-grid {
    grid-template-columns: 1fr;
  }
}
</style>
