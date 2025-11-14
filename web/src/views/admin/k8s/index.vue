<template>
  <div class="admin-k8s-layout">
    <aside class="admin-k8s-sidebar">
      <div class="admin-k8s-sidebar__header">
        <router-link to="/dashboard" class="sidebar-link">← 返回首页</router-link>
        <h2>Kubernetes 管理</h2>
        <p>集群与资源控制台</p>
      </div>
      <nav class="admin-k8s-sidebar__nav">
        <router-link
          :to="{ name: 'AdminK8sClusters' }"
          class="sidebar-nav__link"
          active-class="sidebar-nav__link--active"
          exact
        >
          集群列表
        </router-link>
        <router-link
          v-if="currentClusterId"
          :to="{ name: 'AdminK8sClusterWorkspace', params: { clusterId: currentClusterId }}"
          class="sidebar-nav__link"
          active-class="sidebar-nav__link--active"
        >
          集群工作台
        </router-link>
      </nav>
    </aside>
    <main class="admin-k8s-main">
      <router-view />
    </main>
  </div>
</template>

<script>
export default {
  name: 'AdminK8sLayout',
  computed: {
    currentClusterId() {
      return this.$route.params.clusterId || ''
    }
  }
}
</script>

<style scoped>
.admin-k8s-layout {
  display: grid;
  grid-template-columns: 280px 1fr;
  min-height: calc(100vh - 40px);
}

.admin-k8s-sidebar {
  background: #0f172a;
  color: #cbd5f5;
  padding: 32px 24px;
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.admin-k8s-sidebar__header h2 {
  margin: 8px 0 4px;
  font-size: 20px;
  color: #fff;
}

.admin-k8s-sidebar__header p {
  margin: 0;
  font-size: 13px;
  color: #94a3b8;
}

.sidebar-link {
  color: #93c5fd;
  text-decoration: none;
  font-size: 14px;
}

.admin-k8s-sidebar__nav {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.sidebar-nav__link {
  color: #cbd5f5;
  text-decoration: none;
  padding: 10px 12px;
  border-radius: 10px;
  transition: background 0.2s ease;
}

.sidebar-nav__link:hover,
.sidebar-nav__link--active {
  background: rgba(148, 163, 184, 0.2);
  color: #fff;
}

.admin-k8s-main {
  background: #f6f8fb;
  min-height: 100vh;
}
</style>
