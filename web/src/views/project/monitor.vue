<template>
  <div class="project-panel">
    <header class="panel-header">
      <h2>监控与告警</h2>
      <p>该页面用于展示项目的运行状况，以及维护告警策略。后续可以接入 Prometheus、Grafana 或内部埋点平台。</p>
    </header>

    <section class="monitor-section">
      <h3>实时指标</h3>
      <div class="monitor-grid">
        <div v-for="metric in metrics" :key="metric.label" class="monitor-card">
          <h4>{{ metric.label }}</h4>
          <p class="monitor-value">{{ metric.value }}</p>
          <p class="monitor-desc">{{ metric.description }}</p>
        </div>
      </div>
    </section>

    <section class="monitor-section">
      <h3>告警策略</h3>
      <p class="placeholder">暂无配置，后续可在此新增或编辑项目告警规则。</p>
    </section>
  </div>
</template>

<script>
export default {
  name: 'ProjectMonitor',
  data() {
    return {
      metrics: [
        { label: '最近构建', value: '成功 · 5分钟前', description: '展示最近一次流水线构建状态与时间。' },
        { label: '部署成功率', value: '100%', description: '统计最近 7 天的发布成功率。' },
        { label: '平均部署时长', value: '3m 42s', description: '近 10 次部署的平均耗时。' }
      ]
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

.panel-header h2 {
  margin: 0;
  font-size: 1.6rem;
  font-weight: 600;
}

.panel-header p {
  margin: 0.5rem 0 0;
  color: #6b7280;
}

.monitor-section {
  background: #ffffff;
  border-radius: 16px;
  padding: 1.5rem;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.04);
}

.monitor-section h3 {
  margin: 0 0 1rem;
  font-size: 1.1rem;
  font-weight: 600;
  color: #1f2937;
}

.monitor-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 1rem;
}

.monitor-card {
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  padding: 1rem;
  background: #f9fafb;
}

.monitor-card h4 {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
}

.monitor-value {
  margin: 0.5rem 0;
  font-size: 1.25rem;
  font-weight: 600;
  color: #2563eb;
}

.monitor-desc {
  margin: 0;
  color: #6b7280;
  font-size: 0.85rem;
}

.placeholder {
  margin: 0;
  color: #6b7280;
}
</style>
