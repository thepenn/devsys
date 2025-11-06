<template>
  <div class="login-page">
    <div v-if="error" class="alert alert--error login-alert">
      <span>{{ error }}</span>
      <button class="button button--ghost" @click="clearError">关闭</button>
    </div>
    <div class="login-card">
      <h1>欢迎使用 Go DevOps</h1>
      <p class="login-subtitle">使用您的 Git 账户登录以管理仓库和流水线。</p>
      <button class="button login-button" :disabled="loginPending" @click="login">
        {{ loginPending ? '跳转中…' : '使用 Git 登录' }}
      </button>
    </div>
  </div>
</template>

<script>
import { getToken } from '@/utils/auth'

const API_PREFIX = '/api/v1'

export default {
  name: 'LoginView',
  data() {
    return {
      loginPending: false,
      error: ''
    }
  },
  watch: {
    '$route.query.error'(val) {
      this.error = val || ''
    }
  },
  created() {
    const token = getToken()
    if (token) {
      this.$router.replace('/dashboard')
      return
    }
    this.error = this.$route.query.error || ''
  },
  methods: {
    login() {
      this.loginPending = true
      try {
        const redirect = `${window.location.origin}${window.location.pathname}#/dashboard`
        window.location.href = `${API_PREFIX}/auth/gitlab/login?redirect=${encodeURIComponent(redirect)}`
      } catch (err) {
        this.error = err.message || '无法发起登录请求'
        this.loginPending = false
      }
    },
    clearError() {
      this.error = ''
      if (this.$route.query.error) {
        this.$router.replace({ path: '/login' })
      }
    }
  }
}
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 2rem 1rem;
}

.login-card {
  width: 100%;
  max-width: 400px;
  background: #ffffff;
  border-radius: 18px;
  box-shadow: 0 20px 45px rgba(79, 70, 229, 0.15);
  padding: 2.5rem 2rem;
  text-align: center;
}

.login-card h1 {
  margin: 0;
  font-size: 1.75rem;
  font-weight: 600;
  color: #1f2933;
}

.login-subtitle {
  margin: 0.75rem 0 2rem;
  color: #6b7280;
  font-size: 0.95rem;
}

.login-button {
  width: 100%;
}

.login-alert {
  max-width: 420px;
  width: 100%;
  margin-bottom: 1.5rem;
}

@media (max-width: 480px) {
  .login-card {
    padding: 2rem 1.5rem;
  }
}
</style>
