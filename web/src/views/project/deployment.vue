<template>
  <div class="project-panel">
    <header class="panel-header">
      <h2>部署发布</h2>
      <p>管理项目的部署策略，可对不同环境执行上线、回滚与灰度操作。</p>
      <button class="button" :disabled="deploying" @click="openDeployModal()">{{ deploying ? '部署中…' : '发起部署' }}</button>
    </header>

    <section class="panel">
      <table class="deployment-table">
        <thead>
          <tr>
            <th>环境</th>
            <th>最近部署</th>
            <th>版本</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="env in environments" :key="env.name">
            <td>
              <strong>{{ env.display }}</strong>
              <p class="deployment-desc">{{ env.description }}</p>
            </td>
            <td>{{ env.lastDeployed }}</td>
            <td>{{ env.version || '未记录' }}</td>
            <td>
              <span class="deployment-status" :class="`status-${env.status}`">{{ statusLabel(env.status) }}</span>
            </td>
            <td class="deployment-actions">
              <button class="button button--ghost" @click="openDeployModal(env)">部署</button>
              <button class="button button--ghost" :disabled="!env.version" @click="simulateRollback(env)">回滚</button>
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <div v-if="modal.visible" class="modal-backdrop" @click.self="closeDeployModal">
      <div class="modal">
        <header class="modal__header">
          <h3>部署到 {{ modal.target ? modal.target.display : '环境' }}</h3>
          <button class="modal__close" @click="closeDeployModal">×</button>
        </header>
        <section class="modal__body">
          <label class="modal__field">
            <span>选择版本</span>
            <select v-model="modal.form.version">
              <option disabled value="">请选择版本标签</option>
              <option v-for="tag in mockVersions" :key="tag" :value="tag">{{ tag }}</option>
            </select>
          </label>
          <label class="modal__field">
            <span>部署备注</span>
            <textarea v-model="modal.form.notes" rows="3" placeholder="例如：修复登录接口超时" />
          </label>
          <label class="modal__field checkbox">
            <input v-model="modal.form.canary" type="checkbox">
            <span>先灰度一部分实例</span>
          </label>
        </section>
        <footer class="modal__footer">
          <button class="button button--ghost" @click="closeDeployModal">取消</button>
          <button class="button" @click="triggerDeployment">确认部署</button>
        </footer>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'ProjectDeployment',
  props: {
    project: {
      type: Object,
      default: null
    }
  },
  data() {
    return {
      deploying: false,
      environments: [
        {
          name: 'dev',
          display: '开发环境',
          description: '用于开发自测，自动合并 dev 分支后触发部署。',
          lastDeployed: '2025-03-08 10:24',
          version: 'v1.3.5',
          status: 'success'
        },
        {
          name: 'staging',
          display: '预发布环境',
          description: '与生产保持一致，用于验证回归。',
          lastDeployed: '2025-03-05 19:10',
          version: 'v1.3.4',
          status: 'success'
        },
        {
          name: 'prod',
          display: '生产环境',
          description: '对外提供服务的核心环境，请谨慎操作。',
          lastDeployed: '2025-02-28 22:15',
          version: 'v1.3.2',
          status: 'pending'
        }
      ],
      modal: {
        visible: false,
        target: null,
        form: {
          version: '',
          notes: '',
          canary: false
        }
      },
      mockVersions: ['v1.3.5', 'v1.3.4', 'v1.3.3', 'v1.3.2']
    }
  },
  methods: {
    openDeployModal(env = null) {
      this.modal.visible = true
      this.modal.target = env
      this.modal.form = {
        version: env && env.version ? env.version : this.mockVersions[0],
        notes: '',
        canary: env && env.name === 'prod'
      }
    },
    closeDeployModal() {
      this.modal.visible = false
      this.modal.target = null
      this.modal.form = {
        version: '',
        notes: '',
        canary: false
      }
    },
    triggerDeployment() {
      if (!this.modal.form.version) {
        alert('请选择一个版本进行部署')
        return
      }
      this.deploying = true
      setTimeout(() => {
        const target = this.modal.target
        if (target) {
          target.version = this.modal.form.version
          target.lastDeployed = new Date().toLocaleString()
          target.status = 'success'
        }
        this.deploying = false
        this.closeDeployModal()
        alert('部署任务已创建（模拟）')
      }, 1200)
    },
    simulateRollback(env) {
      if (!env.version) return
      if (confirm(`确定回滚 ${env.display} 至上一版本吗？`)) {
        env.status = 'pending'
        setTimeout(() => {
          env.status = 'success'
          env.lastDeployed = new Date().toLocaleString()
          alert('回滚成功（模拟）')
        }, 1000)
      }
    },
    statusLabel(status) {
      switch (status) {
        case 'success':
          return '运行正常'
        case 'pending':
          return '待发布'
        case 'failed':
          return '失败'
        default:
          return '未知'
      }
    }
  }
}
</script>

<style scoped>
.project-panel {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.panel-header {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.panel-header h2 {
  margin: 0;
  font-size: 1.5rem;
  font-weight: 600;
}

.panel-header p {
  margin: 0;
  color: #6b7280;
  flex: 1;
}

.deployment-table {
  width: 100%;
  border-collapse: collapse;
}

.deployment-table th,
.deployment-table td {
  padding: 0.9rem;
  border-bottom: 1px solid #e5e7eb;
  text-align: left;
}

.deployment-desc {
  margin: 0.35rem 0 0;
  color: #6b7280;
  font-size: 0.85rem;
}

.deployment-actions {
  display: flex;
  gap: 0.5rem;
}

.deployment-status {
  display: inline-flex;
  padding: 0.2rem 0.6rem;
  border-radius: 999px;
  font-size: 0.8rem;
  background: #e5e7eb;
  color: #374151;
}

.status-success {
  background: #ecfdf5;
  color: #047857;
}

.status-pending {
  background: #fef3c7;
  color: #b45309;
}

.status-failed {
  background: #fee2e2;
  color: #b91c1c;
}

.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal {
  width: 520px;
  max-width: 92%;
  background: #fff;
  border-radius: 16px;
  overflow: hidden;
  box-shadow: 0 24px 60px rgba(15, 23, 42, 0.28);
  display: flex;
  flex-direction: column;
}

.modal__header,
.modal__footer {
  padding: 1rem 1.5rem;
  border-bottom: 1px solid #e5e7eb;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.modal__footer {
  border-bottom: none;
  border-top: 1px solid #e5e7eb;
  justify-content: flex-end;
  gap: 0.75rem;
}

.modal__body {
  padding: 1.5rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.modal__field span {
  display: block;
  margin-bottom: 0.4rem;
  font-weight: 600;
  color: #374151;
}

.modal__field input,
.modal__field textarea,
.modal__field select {
  width: 100%;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: 0.6rem 0.75rem;
  font-size: 0.95rem;
}

.modal__field.checkbox {
  flex-direction: row;
  align-items: center;
  gap: 0.5rem;
}

.modal__field.checkbox span {
  margin-bottom: 0;
}

.modal__close {
  border: none;
  background: transparent;
  font-size: 1.5rem;
  cursor: pointer;
}

.button--danger {
  color: #dc2626;
  border-color: rgba(220, 38, 38, 0.4);
}

@media (max-width: 768px) {
  .panel-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .deployment-table th,
  .deployment-table td {
    white-space: nowrap;
  }
}
</style>
