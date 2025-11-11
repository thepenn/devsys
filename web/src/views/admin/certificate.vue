<template>
  <div v-loading="loading" class="certificate-admin">
    <header class="certificate-admin__header">
      <div class="certificate-admin__nav">
        <button class="button button--ghost" @click="backToDashboard">返回首页</button>
        <button class="button button--ghost" @click="backToAdmin">返回后台</button>
      </div>
      <h1>凭证管理</h1>
      <div class="certificate-admin__spacer" />
      <el-button type="primary" icon="el-icon-plus" :disabled="loading" @click="openCreate">新建凭证</el-button>
    </header>

    <section v-if="error" class="certificate-admin__alert">
      <span>{{ error }}</span>
      <button class="button button--ghost" @click="error = ''">关闭</button>
    </section>

    <section class="certificate-admin__panel">
      <div class="certificate-admin__filters">
        <el-input
          v-model.trim="filters.name"
          placeholder="搜索凭证名称"
          class="certificate-admin__filter-input"
          clearable
        />
        <el-select
          v-model="filters.type"
          placeholder="类型"
          class="certificate-admin__filter-select"
          clearable
        >
          <el-option
            v-for="option in typeOptions"
            :key="option.value"
            :label="option.label"
            :value="option.value"
          />
        </el-select>
        <el-input
          v-model.trim="filters.scope"
          placeholder="作用域（可选）"
          class="certificate-admin__filter-input"
          clearable
        />
        <el-button type="primary" @click="handleSearch">查询</el-button>
        <el-button @click="resetFilters">重置</el-button>
      </div>

      <el-table
        v-loading="tableLoading"
        :data="certificates"
        stripe
        border
        class="certificate-admin__table"
        empty-text="暂无数据"
      >
        <el-table-column prop="name" label="名称" min-width="180" />
        <el-table-column prop="type" label="类型" width="120" />
        <el-table-column prop="scope" label="作用域" min-width="140">
          <template slot-scope="scope">
            <span>{{ scope.row.scope || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="updated" label="最近更新" min-width="180">
          <template slot-scope="scope">
            <span>{{ formatTimestamp(scope.row.updated) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right">
          <template slot-scope="scope">
            <el-button size="mini" @click="openEdit(scope.row)">编辑</el-button>
            <el-button size="mini" type="danger" @click="confirmDelete(scope.row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div v-if="pagination.total > pagination.perPage" class="certificate-admin__pagination">
        <el-pagination
          background
          layout="prev, pager, next"
          :page-size="pagination.perPage"
          :current-page.sync="pagination.page"
          :total="pagination.total"
          @current-change="handlePageChange"
        />
      </div>
    </section>

    <el-dialog :title="dialogTitle" :visible.sync="dialogVisible" width="640px" @close="handleDialogClose">
      <el-form ref="certificateForm" :model="form" :rules="formRules" label-width="120px">
        <el-form-item label="名称" prop="name">
          <el-input v-model.trim="form.name" placeholder="请输入凭证名称" />
        </el-form-item>

        <el-form-item label="类型" prop="type">
          <el-select v-model="form.type" placeholder="请选择凭证类型" @change="handleTypeChange">
            <el-option
              v-for="option in typeOptions"
              :key="option.value"
              :label="option.label"
              :value="option.value"
            />
          </el-select>
        </el-form-item>

        <el-form-item label="作用域">
          <div class="certificate-scope">
            <el-input
              v-model.trim="form.scope"
              :disabled="useGlobalScope"
              placeholder="可选，例如项目名称或环境"
            />
            <el-checkbox v-model="useGlobalScope" @change="handleGlobalScopeChange">
              设为全局凭证
            </el-checkbox>
          </div>
          <p v-if="useGlobalScope" class="certificate-scope__hint">全局凭证可被所有项目直接引用。</p>
        </el-form-item>

        <template v-for="field in activeFields">
          <el-form-item
            :key="field.key"
            :label="field.label"
            :prop="`config.${field.key}`"
            :required="isFieldRequired(field)"
          >
            <component
              :is="resolveComponent(field)"
              v-model="form.config[field.key]"
              v-bind="resolveComponentProps(field)"
              :placeholder="resolvePlaceholder(field)"
            />
          </el-form-item>
        </template>

        <el-alert
          v-if="activeSecretSummary.length"
          type="info"
          :closable="false"
          class="certificate-admin__secret-hint"
        >
          <template slot="title">
            <span>敏感字段仅在创建或修改时发送，未修改时保持原值：</span>
            <span class="certificate-admin__secret-tags">
              <span v-for="name in activeSecretSummary" :key="name" class="certificate-admin__secret-tag">{{ name }}</span>
            </span>
          </template>
        </el-alert>
      </el-form>
      <span slot="footer" class="dialog-footer">
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitForm">保存</el-button>
      </span>
    </el-dialog>
  </div>
</template>

<script>
import { Message } from 'element-ui'
import JSEncrypt from 'jsencrypt'

import { getToken, clearToken } from '@/utils/auth'
import { getRSAPublicKey } from '@/utils/rsa'
import {
  listCertificates,
  createCertificate,
  getCertificate,
  updateCertificate,
  deleteCertificate
} from '@/api/system/certificates'
import { getCurrentUser } from '@/api/system/auth'

const SECRET_KEYS = new Set([
  'password',
  'token',
  'access_token',
  'refresh_token',
  'secret',
  'client_secret',
  'private_key',
  'ssh_key',
  'api_key',
  'auth_token',
  'bearer_token',
  'secret_key',
  'secret_token',
  'service_token',
  'registry_token',
  'kubeconfig'
])

const RSA_CHUNK_PREFIX = 'chunked:v1:'
const RSA_CHUNK_SEPARATOR = '::'

const TYPE_DEFINITIONS = {
  git: {
    label: 'Git 凭证',
    fields: [
      { key: 'username', label: '用户名', component: 'input', required: true },
      { key: 'password', label: '密码', component: 'password', required: true, secret: true }
    ]
  },
  docker: {
    label: 'Docker Registry',
    fields: [
      { key: 'repo', label: 'Registry 地址', component: 'input', required: true },
      { key: 'username', label: '用户名', component: 'input', required: true },
      { key: 'password', label: '密码', component: 'password', required: true, secret: true }
    ]
  },
  mysql: {
    label: 'MySQL 数据库',
    fields: [
      { key: 'host', label: '主机地址', component: 'input', required: true },
      { key: 'port', label: '端口', component: 'number', required: true, default: 3306, min: 1, max: 65535 },
      { key: 'database', label: '数据库', component: 'input', required: true },
      { key: 'username', label: '用户名', component: 'input', required: true },
      { key: 'password', label: '密码', component: 'password', required: true, secret: true }
    ]
  },
  ldap: {
    label: 'LDAP 目录服务',
    fields: [
      { key: 'server', label: '服务器地址', component: 'input', required: true },
      { key: 'port', label: '端口', component: 'number', required: true, default: 389, min: 1, max: 65535 },
      { key: 'base_dn', label: 'Base DN', component: 'input', required: true },
      { key: 'search_base_dn', label: 'Search Base DN', component: 'input', required: false },
      { key: 'bind_dn', label: 'Bind DN', component: 'input', required: true },
      { key: 'password', label: '密码', component: 'password', required: true, secret: true },
      { key: 'user_attr', label: '用户属性', component: 'input', required: false, default: 'uid' },
      { key: 'email_attr', label: '邮箱属性', component: 'input', required: false, default: 'mail' },
      { key: 'group_attr', label: '组属性', component: 'input', required: false, default: 'memberOf' }
    ]
  },
  kubernetes: {
    label: 'Kubernetes 集群',
    fields: [
      { key: 'kubeconfig', label: 'KubeConfig', component: 'textarea', rows: 10, required: true, secret: true }
    ]
  }
}

function buildDefaultConfig(type) {
  const definition = TYPE_DEFINITIONS[type]
  const config = {}
  if (!definition || !Array.isArray(definition.fields)) {
    return config
  }
  definition.fields.forEach((field) => {
    if (field.default !== undefined) {
      config[field.key] = field.default
    } else if (field.component === 'number') {
      config[field.key] = field.min !== undefined ? field.min : 0
    } else {
      config[field.key] = ''
    }
  })
  return config
}

function createEmptyForm(type = 'git') {
  return {
    id: null,
    name: '',
    type,
    scope: '',
    config: buildDefaultConfig(type)
  }
}

function encryptSecretValueLong(encryptor, value) {
  if (!value) return ''
  const key = encryptor.getKey()
  if (!key || !key.n || typeof key.n.bitLength !== 'function') {
    throw new Error('RSA 公钥未就绪')
  }
  const chunkSize = Math.floor((key.n.bitLength() + 7) / 8) - 11
  if (chunkSize <= 0) {
    throw new Error('RSA key size is invalid')
  }
  if (value.length <= chunkSize) {
    const encrypted = encryptor.encrypt(value)
    if (!encrypted) {
      throw new Error('加密失败')
    }
    return encrypted
  }
  const chunks = []
  for (let i = 0; i < value.length; i += chunkSize) {
    const segment = value.slice(i, i + chunkSize)
    const encrypted = encryptor.encrypt(segment)
    if (!encrypted) {
      throw new Error('加密失败')
    }
    chunks.push(encrypted)
  }
  return `${RSA_CHUNK_PREFIX}${chunks.join(RSA_CHUNK_SEPARATOR)}`
}

export default {
  name: 'CertificateAdmin',
  data() {
    return {
      loading: true,
      tableLoading: false,
      saving: false,
      dialogVisible: false,
      isEditing: false,
      error: '',
      token: getToken(),
      user: null,
      certificates: [],
      filters: {
        name: '',
        type: '',
        scope: ''
      },
      pagination: {
        page: 1,
        perPage: 10,
        total: 0
      },
      form: createEmptyForm(),
      useGlobalScope: false,
      existingSecrets: {},
      encryptor: null,
      formRules: {
        name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
        type: [{ required: true, message: '请选择类型', trigger: 'change' }]
      }
    }
  },
  computed: {
    typeOptions() {
      return Object.keys(TYPE_DEFINITIONS).map((key) => ({ value: key, label: TYPE_DEFINITIONS[key].label }))
    },
    dialogTitle() {
      return this.isEditing ? '编辑凭证' : '新建凭证'
    },
    activeFields() {
      const definition = TYPE_DEFINITIONS[this.form.type]
      return definition ? definition.fields : []
    },
    activeSecretSummary() {
      return this.activeFields
        .filter((field) => this.isFieldSecret(field))
        .map((field) => field.label)
    }
  },
  created() {
    this.bootstrap()
  },
  methods: {
    async bootstrap() {
      if (!this.token) {
        this.redirectToLogin('请先登录')
        return
      }

      try {
        const user = await getCurrentUser()
        if (!user || !user.admin) {
          this.$router.replace('/dashboard')
          return
        }
        this.user = user
        await this.loadCertificates()
      } catch (err) {
        this.handleRequestError(err)
      } finally {
        this.loading = false
      }
    },
    async loadCertificates() {
      this.tableLoading = true
      this.error = ''
      try {
        const params = {
          page: this.pagination.page,
          per_page: this.pagination.perPage
        }
        if (this.filters.name) params.name = this.filters.name
        if (this.filters.type) params.type = this.filters.type
        if (this.filters.scope) params.scope = this.filters.scope

        const response = await listCertificates(params)
        this.certificates = Array.isArray(response.items) ? response.items : []
        this.pagination.total = response.total || 0
        this.pagination.page = response.page || this.pagination.page
        this.pagination.perPage = response.per_page || this.pagination.perPage
      } catch (err) {
        this.handleRequestError(err, '加载凭证失败')
      } finally {
        this.tableLoading = false
      }
    },
    handleRequestError(err, fallback = '操作失败') {
      if (!err) {
        this.error = fallback
        return
      }
      const status = err.status || err?.response?.status
      const message = err.message || err?.response?.data?.message || fallback
      if (status === 401) {
        this.redirectToLogin(message)
        return
      }
      this.error = message
    },
    redirectToLogin(message) {
      clearToken()
      const query = message ? { error: message } : undefined
      this.$router.replace({ path: '/login', query })
    },
    backToDashboard() {
      this.$router.push('/dashboard')
    },
    backToAdmin() {
      this.$router.push('/admin')
    },
    handleSearch() {
      this.pagination.page = 1
      this.loadCertificates()
    },
    resetFilters() {
      this.filters = { name: '', type: '', scope: '' }
      this.pagination.page = 1
      this.loadCertificates()
    },
    handlePageChange(page) {
      this.pagination.page = page
      this.loadCertificates()
    },
    openCreate() {
      this.isEditing = false
      this.dialogVisible = true
      this.form = createEmptyForm()
      this.useGlobalScope = false
      this.existingSecrets = {}
      this.encryptor = null
      this.$nextTick(() => {
        if (this.$refs.certificateForm) {
          this.$refs.certificateForm.clearValidate()
        }
      })
    },
    async openEdit(row) {
      try {
        this.tableLoading = true
        const detail = await getCertificate(row.id)
        this.isEditing = true
        this.dialogVisible = true
        this.form = createEmptyForm(detail.type || 'git')
        this.form.id = detail.id
        this.form.name = detail.name
        this.form.type = detail.type || 'git'
        this.form.scope = detail.scope || ''
        this.useGlobalScope = (detail.scope || '').toLowerCase() === 'global'
        if (this.useGlobalScope) {
          this.form.scope = 'global'
        }
        this.form.config = buildDefaultConfig(this.form.type)
        this.existingSecrets = {}
        if (Array.isArray(detail.masked_fields)) {
          detail.masked_fields.forEach((key) => {
            this.$set(this.existingSecrets, key, true)
          })
        }
        Object.entries(detail.config || {}).forEach(([key, value]) => {
          if (this.isFieldSecret({ key })) {
            this.form.config[key] = ''
          } else {
            this.form.config[key] = value
          }
        })
        this.encryptor = null
        this.$nextTick(() => {
          if (this.$refs.certificateForm) {
            this.$refs.certificateForm.clearValidate()
          }
        })
      } catch (err) {
        this.handleRequestError(err, '获取凭证详情失败')
      } finally {
        this.tableLoading = false
      }
    },
    handleDialogClose() {
      this.form = createEmptyForm(this.form.type)
      this.useGlobalScope = false
      this.existingSecrets = {}
      this.encryptor = null
      if (this.$refs.certificateForm) {
        this.$refs.certificateForm.resetFields()
      }
    },
    handleTypeChange() {
      const newConfig = buildDefaultConfig(this.form.type)
      this.form.config = newConfig
      this.existingSecrets = {}
      if (this.$refs.certificateForm) {
        this.$nextTick(() => this.$refs.certificateForm.clearValidate())
      }
    },
    handleGlobalScopeChange(checked) {
      if (checked) {
        this.form.scope = 'global'
      } else if ((this.form.scope || '').toLowerCase() === 'global') {
        this.form.scope = ''
      }
    },
    resolveComponent(field) {
      if (field.component === 'textarea') return 'el-input'
      if (field.component === 'number') return 'el-input-number'
      return 'el-input'
    },
    resolveComponentProps(field) {
      const props = {}
      if (field.component === 'password') {
        props.type = 'password'
        props.autocomplete = 'new-password'
        props.showPassword = true
      }
      if (field.component === 'textarea') {
        props.type = 'textarea'
        props.rows = field.rows || 3
      }
      if (field.component === 'number') {
        if (field.min !== undefined) props.min = field.min
        if (field.max !== undefined) props.max = field.max
        props.controlsPosition = 'right'
      }
      return props
    },
    resolvePlaceholder(field) {
      if (this.isEditing && this.existingSecrets[field.key] && this.isFieldSecret(field)) {
        return '已设置，填写以更新'
      }
      return `请输入${field.label}`
    },
    isFieldSecret(field) {
      if (!field) return false
      if (field.secret) return true
      return SECRET_KEYS.has(field.key)
    },
    isFieldRequired(field) {
      if (!field) return false
      if (!field.required) return false
      if (!this.isEditing) return true
      if (!this.existingSecrets[field.key]) return true
      return !this.isFieldSecret(field)
    },
    validateConfig() {
      const errors = []
      this.activeFields.forEach((field) => {
        if (!field.required) return
        const value = this.form.config[field.key]
        const hasExisting = this.isEditing && this.existingSecrets[field.key]
        if (this.isFieldSecret(field) && hasExisting && !value) {
          return
        }
        if (value === '' || value === null || value === undefined) {
          errors.push(`${field.label} 为必填项`)
        }
      })
      return errors
    },
    async ensureEncryptor() {
      if (this.encryptor) {
        return this.encryptor
      }
      const publicKey = await getRSAPublicKey()
      if (!publicKey) {
        throw new Error('无法获取RSA公钥')
      }
      const encryptor = new JSEncrypt()
      encryptor.setPublicKey(publicKey)
      this.encryptor = encryptor
      return encryptor
    },
    async buildConfigPayload() {
      const payload = {}
      for (const field of this.activeFields) {
        const key = field.key
        const value = this.form.config[key]
        const isSecret = this.isFieldSecret(field)
        if (isSecret) {
          if (!value) {
            continue
          }
          const encryptor = await this.ensureEncryptor()
          const encrypted = encryptSecretValueLong(encryptor, value)
          if (!encrypted) {
            throw new Error(`${field.label} 加密失败`)
          }
          payload[key] = encrypted
          continue
        }

        if (value === '' || value === null || value === undefined) {
          if (this.isEditing) {
            payload[key] = value
          }
        } else {
          payload[key] = value
        }
      }
      return payload
    },
    async submitForm() {
      if (this.$refs.certificateForm) {
        const valid = await new Promise((resolve) => {
          this.$refs.certificateForm.validate((ok) => resolve(ok))
        })
        if (!valid) {
          return
        }
      }

      const errors = this.validateConfig()
      if (errors.length) {
        Message.error(errors[0])
        return
      }

      this.saving = true
      try {
        const configPayload = await this.buildConfigPayload()
        const scopeValue = this.useGlobalScope ? 'global' : (this.form.scope || '').trim()
        const payload = {
          name: this.form.name,
          type: this.form.type,
          scope: scopeValue,
          config: configPayload
        }

        if (!this.isEditing) {
          await createCertificate(payload)
          Message.success('创建成功')
        } else {
          await updateCertificate(this.form.id, payload)
          Message.success('更新成功')
        }

        this.dialogVisible = false
        await this.loadCertificates()
      } catch (err) {
        this.handleRequestError(err, '保存凭证失败')
      } finally {
        this.saving = false
      }
    },
    async confirmDelete(row) {
      try {
        await this.$confirm(`确定要删除凭证 “${row.name}” 吗？`, '提示', {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning'
        })
      } catch (err) {
        if (err !== 'cancel') {
          this.handleRequestError(err)
        }
        return
      }

      try {
        await deleteCertificate(row.id)
        Message.success('删除成功')
        await this.loadCertificates()
      } catch (err) {
        this.handleRequestError(err, '删除失败')
      }
    },
    formatTimestamp(timestamp) {
      if (!timestamp) {
        return '—'
      }
      const date = new Date(timestamp * 1000)
      if (Number.isNaN(date.getTime())) {
        return '—'
      }
      return date.toLocaleString()
    }
  }
}
</script>

<style scoped>
.certificate-admin {
  min-height: 100vh;
  padding: 2rem;
  max-width: 1100px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.certificate-admin__header {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.certificate-admin__nav {
  display: flex;
  gap: 0.5rem;
}

.certificate-admin__header h1 {
  margin: 0;
  font-size: 1.6rem;
  font-weight: 600;
}

.certificate-admin__spacer {
  flex: 1;
}

.certificate-admin__alert {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border: 1px solid #fecaca;
  background: #fee2e2;
  color: #991b1b;
  padding: 0.75rem 1rem;
  border-radius: 8px;
}

.certificate-admin__panel {
  background: #fff;
  border-radius: 12px;
  padding: 1.5rem;
  box-shadow: 0 16px 40px rgba(30, 41, 59, 0.08);
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.certificate-admin__filters {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  align-items: center;
}

.certificate-admin__filter-input,
.certificate-admin__filter-select {
  width: 220px;
}

.certificate-admin__table {
  width: 100%;
}

.certificate-scope {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.certificate-scope__hint {
  margin: 0.4rem 0 0;
  font-size: 0.85rem;
  color: #6b7280;
}

.certificate-admin__pagination {
  display: flex;
  justify-content: flex-end;
}

.certificate-admin__secret-hint {
  margin-top: 1rem;
}

.certificate-admin__secret-tags {
  display: inline-flex;
  gap: 0.5rem;
  flex-wrap: wrap;
  margin-left: 0.5rem;
}

.certificate-admin__secret-tag {
  padding: 0.1rem 0.45rem;
  border-radius: 999px;
  background: rgba(37, 99, 235, 0.12);
  color: #1d4ed8;
  font-size: 0.75rem;
}

@media (max-width: 768px) {
  .certificate-admin {
    padding: 1.5rem 1rem;
  }

  .certificate-admin__filters {
    flex-direction: column;
    align-items: stretch;
  }

  .certificate-admin__filter-input,
  .certificate-admin__filter-select {
    width: 100%;
  }

  .certificate-admin__header {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
