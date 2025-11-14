<template>
  <div v-loading="loading" class="k8s-cluster-page">
    <header class="k8s-cluster-header">
      <div>
        <button class="nav-link" type="button" @click="goDashboard">← 返回首页</button>
        <h1>集群管理</h1>
      </div>
      <div class="cluster-actions">
        <el-button type="primary" icon="el-icon-refresh" :loading="loading" @click="bootstrap">重新加载</el-button>
      </div>
    </header>

    <section class="panel cluster-panel">
      <div class="cluster-table">
        <div class="cluster-table__row cluster-table__head">
          <div class="cluster-table__cell cluster-table__cell--name">集群</div>
          <div class="cluster-table__cell">范围</div>
          <div class="cluster-table__cell">最近更新</div>
          <div class="cluster-table__cell cluster-table__cell--actions">操作</div>
        </div>
        <el-empty v-if="!clusters.length && !loading" description="暂无集群，请先添加凭证" />
        <div
          v-for="cluster in clusters"
          :key="cluster.id"
          class="cluster-table__row"
          @click="openCluster(cluster.id)"
        >
          <div class="cluster-table__cell cluster-table__cell--name">
            <p class="cluster-name">{{ cluster.name }}</p>
          </div>
          <div class="cluster-table__cell">
            <el-tag size="mini" type="info">{{ cluster.scope || '未定义' }}</el-tag>
          </div>
          <div class="cluster-table__cell">
            {{ formatTime(cluster.updated) || '未知' }}
          </div>
          <div class="cluster-table__cell cluster-table__cell--actions">
            <el-button type="primary" size="mini" @click.stop="openCluster(cluster.id)">进入管理</el-button>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script>
import { listClusters } from '@/api/admin/k8s'

export default {
  name: 'AdminK8sClusterList',
  data() {
    return {
      loading: true,
      clusters: []
    }
  },
  created() {
    this.bootstrap()
  },
  methods: {
    goDashboard() {
      this.$router.push('/dashboard')
    },
    async bootstrap() {
      this.loading = true
      try {
        const clusterList = await listClusters()
        this.clusters = Array.isArray(clusterList) ? clusterList : []
      } catch (err) {
        this.$message.error(err.message || '加载集群失败')
      } finally {
        this.loading = false
      }
    },
    openCluster(id) {
      if (!id) return
      this.$router.push({ name: 'AdminK8sClusterWorkspace', params: { clusterId: id }})
    },
    formatTime(value) {
      if (!value) return ''
      const date = new Date(value)
      if (Number.isNaN(date.getTime())) {
        return ''
      }
      return date.toLocaleString()
    }
  }
}
</script>

<style scoped>
.k8s-cluster-page {
  padding: 32px;
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.k8s-cluster-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.k8s-cluster-header h1 {
  margin: 8px 0 0;
  font-size: 28px;
  font-weight: 600;
}

.nav-link {
  background: none;
  border: none;
  color: #2563eb;
  cursor: pointer;
  font-size: 14px;
}

.cluster-panel {
  padding: 32px;
}

.cluster-actions {
  display: flex;
  gap: 12px;
}

.cluster-table {
  border: 1px solid #e5e7eb;
  border-radius: 16px;
  overflow: hidden;
}

.cluster-table__row {
  display: grid;
  grid-template-columns: 2.5fr 1fr 1.2fr 0.8fr;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid #eef2ff;
  cursor: pointer;
  transition: background 0.2s ease;
}

.cluster-table__row:last-child {
  border-bottom: none;
}

.cluster-table__row:hover {
  background: #f8fbff;
}

.cluster-table__head {
  background: #f1f5f9;
  font-weight: 600;
  cursor: default;
}

.cluster-table__cell {
  color: #1f2937;
  display: flex;
  align-items: center;
  gap: 8px;
}

.cluster-table__cell--name {
  display: flex;
  align-items: center;
  gap: 12px;
}

.cluster-table__cell--actions {
  justify-content: flex-end;
}

.cluster-name {
  margin: 0;
  font-weight: 600;
  color: #111827;
}

.panel {
  background: #fff;
  border-radius: 20px;
  box-shadow: 0 20px 40px rgba(15, 23, 42, 0.08);
}
</style>
