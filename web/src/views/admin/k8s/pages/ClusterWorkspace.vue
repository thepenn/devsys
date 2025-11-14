<template>
  <div v-loading="rootLoading" class="k8s-workspace">
    <div v-if="!selectedClusterId" class="workspace-empty panel">
      <el-empty description="请选择要查看的集群">
        <el-button type="primary" @click="goClusterList">返回集群列表</el-button>
      </el-empty>
    </div>

    <div v-else class="k8s-shell">
      <aside class="k8s-shell__nav pipeline-nav">
        <button class="nav-link" type="button" @click="goDashboard">←返回首页</button>
        <button class="nav-link" type="button" @click="resetClusterSelection">←返回集群列表</button>
        <div class="nav-divider" />
        <div class="nav-cluster-card">
          <p class="nav-cluster-card__name">{{ selectedCluster ? selectedCluster.name : '' }}</p>
        </div>
        <div class="nav-divider" />
        <ul class="nav-resource-list">
          <li
            v-for="item in resourceMenu"
            :key="item.key"
            :class="['nav-item', { active: item.key === selectedResourceType }]"
            @click="handleResourceTypeChange(item.key)"
          >
            <span>{{ item.menuLabel }}</span>
          </li>
        </ul>
      </aside>

      <main class="k8s-shell__content">
        <section ref="resourcePanel" class="resource-panel panel">
          <header class="resource-panel__toolbar">
            <el-select
              v-model="selectedNamespace"
              placeholder="选择命名空间"
              filterable
              @change="handleNamespaceChange"
            >
              <el-option :label="'全部命名空间'" :value="allNamespaceValue" />
              <el-option
                v-for="ns in namespaces"
                :key="ns.name"
                :label="ns.name"
                :value="ns.name"
              />
            </el-select>
            <el-input
              v-model="searchKeyword"
              placeholder="搜索名称"
              prefix-icon="el-icon-search"
              clearable
            />
            <el-button icon="el-icon-refresh" circle :loading="resourceLoading" @click="fetchResources" />
            <el-button type="primary" icon="el-icon-plus">创建</el-button>
          </header>

          <div ref="tableWrapper" class="resource-panel__table">
            <el-table
              :data="paginatedResources"
              :height="tableHeight"
              highlight-current-row
              :row-key="resolveRowKey"
              @expand-change="handleRowExpand"
            >
              <el-table-column
                v-if="selectedResourceType === 'workloads'"
                type="expand"
              >
                <template slot-scope="scope">
                  <div class="pod-subtable">
                    <div v-if="isPodsLoading(scope.row)" class="pod-subtable__loading">
                      <el-skeleton :rows="3" animated />
                    </div>
                    <div v-else-if="!getWorkloadPods(scope.row).length" class="pod-subtable__empty">
                      <el-empty description="暂无 Pod" :image-size="80" />
                    </div>
                    <el-table
                      v-else
                      :data="getWorkloadPods(scope.row)"
                      size="mini"
                      class="pod-subtable__table"
                    >
                      <el-table-column label="名称" prop="name" min-width="160" />
                      <el-table-column label="命名空间" prop="namespace" width="140" />
                      <el-table-column label="READY" prop="ready" width="100" />
                      <el-table-column label="状态" prop="status" min-width="140" />
                      <el-table-column label="重启" prop="restarts" width="80" />
                      <el-table-column label="节点" prop="node" width="160" />
                      <el-table-column label="时长" width="140">
                        <template slot-scope="podScope">
                          {{ formatPodAge(podScope.row.created_at) }}
                        </template>
                      </el-table-column>
                      <el-table-column label="操作" width="200">
                        <template slot-scope="podScope">
                          <el-dropdown
                            trigger="click"
                            @command="cmd => handlePodCommand(cmd, scope.row, podScope.row)"
                          >
                            <span class="dropdown-link">
                              操作<i class="el-icon-arrow-down el-icon--right" />
                            </span>
                            <el-dropdown-menu slot="dropdown">
                              <el-dropdown-item command="exec-bash">Exec /bin/bash</el-dropdown-item>
                              <el-dropdown-item command="exec-sh">Exec /bin/sh</el-dropdown-item>
                              <el-dropdown-item command="logs">查看日志</el-dropdown-item>
                              <el-dropdown-item divided command="delete">删除 Pod</el-dropdown-item>
                            </el-dropdown-menu>
                          </el-dropdown>
                        </template>
                      </el-table-column>
                    </el-table>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="名称" min-width="200">
                <template slot-scope="scope">
                  <el-link type="primary" :underline="false" @click.stop="openResourceDetail(scope.row)">
                    {{ scope.row.metadata && scope.row.metadata.name }}
                    <span
                      v-if="scope.row.__imageTag"
                      class="resource-image-tag"
                      :title="scope.row.__images && scope.row.__images.length ? scope.row.__images.join('\n') : ''"
                    >
                      {{ scope.row.__imageTag }}
                    </span>
                  </el-link>
                </template>
              </el-table-column>
              <el-table-column label="命名空间" width="160">
                <template slot-scope="scope">
                  {{ scope.row.__namespace || '-' }}
                </template>
              </el-table-column>
              <el-table-column label="状态" width="180">
                <template slot-scope="scope">
                  <span class="status-chip">
                    <span :class="['status-dot', scope.row.__status && scope.row.__status.type]" />
                    {{ (scope.row.__status && scope.row.__status.text) || '未知' }}
                  </span>
                </template>
              </el-table-column>
              <el-table-column label="资源类型" width="150">
                <template slot-scope="scope">
                  {{ scope.row.__kind || '-' }}
                </template>
              </el-table-column>
              <el-table-column label="创建时间" width="180">
                <template slot-scope="scope">
                  {{ formatTime(scope.row.__createAt) || '未知' }}
                </template>
              </el-table-column>
              <el-table-column label="更新时间" width="180">
                <template slot-scope="scope">
                  {{ formatTime(scope.row.__updateAt) || '未知' }}
                </template>
              </el-table-column>
              <el-table-column label="操作" width="160">
                <template slot-scope="scope">
                  <el-dropdown trigger="click" @command="command => handleRowCommand(command, scope.row)">
                    <span class="dropdown-link">
                      操作<i class="el-icon-arrow-down el-icon--right" />
                    </span>
                    <el-dropdown-menu slot="dropdown">
                      <el-dropdown-item command="editYaml">编辑 YAML</el-dropdown-item>
                      <el-dropdown-item command="recreate">重新创建</el-dropdown-item>
                      <el-dropdown-item v-if="canOpenPods(scope.row)" command="pods">查看 Pod</el-dropdown-item>
                      <el-dropdown-item divided command="delete">删除</el-dropdown-item>
                    </el-dropdown-menu>
                  </el-dropdown>
                </template>
              </el-table-column>
            </el-table>
          </div>

          <div v-if="filteredResources.length" class="resource-panel__footer">
            <div class="resource-panel__page-meta">
              <span>每页显示：</span>
              <el-select
                v-model="pageSize"
                size="mini"
                class="page-size-select"
                @change="handlePageSizeChange"
              >
                <el-option
                  v-for="size in pageSizeOptions"
                  :key="size"
                  :label="size"
                  :value="size"
                />
              </el-select>
              <span>总数：{{ filteredResources.length }}</span>
              <span>当前页：{{ page }} / {{ totalPages }}</span>
            </div>
            <el-pagination
              small
              layout="prev, pager, next"
              :page-size="pageSize"
              :current-page="page"
              :total="filteredResources.length"
              @current-change="handlePageChange"
            />
          </div>
        </section>
      </main>
    </div>

    <el-dialog
      :visible.sync="yamlDialog.visible"
      :title="yamlDialog.title"
      width="60%"
      :close-on-click-modal="false"
      @close="handleYamlClose"
    >
      <div v-loading="yamlDialog.loading">
        <div v-if="yamlDialog.editable" class="yaml-editor">
          <textarea ref="yamlEditorInput" v-model="yamlDialog.content" class="yaml-textarea" />
        </div>
        <div v-else class="yaml-viewer">
          <pre class="k8s-admin__yaml"><code ref="yamlViewer" class="k8s-admin__yaml-code language-yaml">{{ yamlDialog.content }}</code></pre>
        </div>
      </div>
      <span slot="footer">
        <el-button @click="yamlDialog.visible = false">取消</el-button>
        <el-button v-if="yamlDialog.editable" type="primary" :loading="yamlDialog.saving" @click="submitYaml">保存</el-button>
      </span>
    </el-dialog>

    <div v-if="execDialog.visible && execDialog.pod" class="exec-shell-overlay" @keydown.self.prevent="focusExecTerminal">
      <div class="exec-shell">
        <header class="exec-shell__header">
          <div class="exec-shell__breadcrumb">
            <span class="exec-shell__link" @click="resetClusterSelection">Workloads</span>
            <span class="exec-shell__divider">›</span>
            <span>{{ execDialog.pod.name }}</span>
            <span class="exec-shell__divider">›</span>
            <strong>Shell</strong>
          </div>
          <div class="exec-shell__controls">
            <el-select
              v-model="execDialog.container"
              size="mini"
              placeholder="容器"
              @change="handleExecContainerChange"
            >
              <el-option
                v-for="item in execDialog.pod.containers || []"
                :key="item"
                :label="item"
                :value="item"
              />
            </el-select>
            <el-select v-model="execDialog.shell" size="mini" @change="handleExecShellChange">
              <el-option label="/bin/bash" value="bash" />
              <el-option label="/bin/sh" value="sh" />
            </el-select>
            <el-button size="mini" icon="el-icon-refresh" @click="reconnectExecSocket">重连</el-button>
            <el-button size="mini" icon="el-icon-delete" @click="clearExecTerminal">清屏</el-button>
            <el-button size="mini" icon="el-icon-close" @click="handleExecClose">关闭</el-button>
          </div>
        </header>
        <section class="exec-shell__main" @click="focusExecTerminal">
          <div class="exec-shell__terminal-wrapper">
            <div
              ref="execTerminal"
              class="exec-shell__terminal"
              tabindex="0"
            />
          </div>
          <div class="exec-shell__status">
            <span v-if="execDialog.connected" class="exec-shell__status-indicator exec-shell__status-indicator--connected">●</span>
            <span v-else class="exec-shell__status-indicator exec-shell__status-indicator--connecting">●</span>
            {{ execDialog.connected ? '已连接 - 点击终端输入命令' : '正在连接...' }}
          </div>
        </section>
      </div>
    </div>

    <el-dialog
      :visible.sync="logsDialog.visible"
      width="720px"
      title="Pod 日志"
      :close-on-click-modal="false"
      @close="handleLogsClose"
    >
      <div v-if="logsDialog.pod" class="logs-dialog">
        <div class="logs-dialog__meta">
          <div>
            <p class="logs-dialog__pod">Pod：{{ logsDialog.pod.name }}</p>
            <p class="logs-dialog__pod">命名空间：{{ logsDialog.pod.namespace }}</p>
          </div>
          <div class="logs-dialog__controls">
            <el-select
              v-model="logsDialog.container"
              size="mini"
              placeholder="容器"
              @change="handleLogsContainerChange"
            >
              <el-option
                v-for="item in logsDialog.pod.containers || []"
                :key="item"
                :label="item"
                :value="item"
              />
            </el-select>
            <el-switch
              v-model="logsDialog.autoScroll"
              active-text="自动滚动"
              inactive-text="暂停滚动"
              class="logs-dialog__switch"
            />
          </div>
        </div>
        <div ref="logsViewer" class="logs-viewer">
          <pre>{{ logsDialog.content }}</pre>
        </div>
      </div>
      <span slot="footer">
        <el-button @click="handleLogsClose">关闭</el-button>
      </span>
    </el-dialog>

    <el-drawer
      class="workload-drawer"
      :visible.sync="workloadDrawer.visible"
      :title="workloadDrawer.title"
      size="65%"
      :with-header="true"
    >
      <div v-loading="workloadDrawer.loading" class="workload-drawer__body">
        <el-tabs
          v-model="workloadDrawer.activeTab"
          type="border-card"
          @tab-click="handleWorkloadTabClick"
        >
          <el-tab-pane label="概览" name="overview">
            <div class="overview-grid">
              <section class="overview-card">
                <header class="overview-card__header">
                  <h3>基本信息</h3>
                  <span class="overview-card__sub">{{ workloadDrawer.details.workload ? workloadDrawer.details.workload.kind : '-' }}</span>
                </header>
                <dl class="overview-list">
                  <div>
                    <dt>名称</dt>
                    <dd>{{ workloadDrawer.details.overview.name || '-' }}</dd>
                  </div>
                  <div>
                    <dt>命名空间</dt>
                    <dd>{{ workloadDrawer.details.overview.namespace || '-' }}</dd>
                  </div>
                  <div>
                    <dt>Selector</dt>
                    <dd>{{ workloadDrawer.details.overview.selectorText || '-' }}</dd>
                  </div>
                  <div>
                    <dt>策略</dt>
                    <dd>
                      {{ workloadDrawer.details.overview.strategy.type || '-' }}
                      <span v-if="workloadDrawer.details.overview.strategy.maxUnavailable"> · MaxUnavailable {{ workloadDrawer.details.overview.strategy.maxUnavailable }}</span>
                      <span v-if="workloadDrawer.details.overview.strategy.maxSurge"> · MaxSurge {{ workloadDrawer.details.overview.strategy.maxSurge }}</span>
                    </dd>
                  </div>
                </dl>
                <div
                  v-if="mapKeyValuePairs(workloadDrawer.details.overview.labels).length"
                  class="chip-group"
                >
                  <span
                    v-for="item in mapKeyValuePairs(workloadDrawer.details.overview.labels)"
                    :key="item.key"
                    class="chip"
                  >
                    {{ item.key }}={{ item.value }}
                  </span>
                </div>
              </section>
              <section class="overview-card">
                <header class="overview-card__header">
                  <h3>状态</h3>
                  <span class="overview-card__sub">更新时间 {{ workloadDrawer.details.overview.updatedAtText }}</span>
                </header>
                <dl class="overview-list">
                  <div>
                    <dt>副本</dt>
                    <dd>{{ workloadDrawer.details.overview.replica.ready }} / {{ workloadDrawer.details.overview.replica.desired }}</dd>
                  </div>
                  <div>
                    <dt>可用</dt>
                    <dd>{{ workloadDrawer.details.overview.replica.available }}</dd>
                  </div>
                  <div>
                    <dt>已更新</dt>
                    <dd>{{ workloadDrawer.details.overview.replica.updated }}</dd>
                  </div>
                  <div>
                    <dt>创建时间</dt>
                    <dd>{{ workloadDrawer.details.overview.createdAtText || '-' }}</dd>
                  </div>
                </dl>
                <div
                  v-if="workloadDrawer.details.overview.conditions.length"
                  class="chip-group"
                >
                  <span
                    v-for="item in workloadDrawer.details.overview.conditions"
                    :key="item.type"
                    class="chip"
                  >
                    {{ item.type }} · {{ item.status }}
                  </span>
                </div>
              </section>
            </div>
            <section class="overview-card">
              <header class="overview-card__header">
                <h3>容器</h3>
                <span class="overview-card__sub">共 {{ workloadDrawer.details.overview.containers.length }}</span>
              </header>
              <el-table
                :data="workloadDrawer.details.overview.containers"
                size="mini"
                border
                empty-text="暂无容器"
              >
                <el-table-column label="名称" prop="name" min-width="140" show-overflow-tooltip />
                <el-table-column label="镜像" prop="image" min-width="200" show-overflow-tooltip />
                <el-table-column label="类型" width="120">
                  <template slot-scope="scope">
                    {{ scope.row.init ? 'Init Container' : '容器' }}
                  </template>
                </el-table-column>
                <el-table-column label="端口" min-width="140">
                  <template slot-scope="scope">
                    {{ scope.row.ports && scope.row.ports.length ? scope.row.ports.join(', ') : '-' }}
                  </template>
                </el-table-column>
                <el-table-column label="命令" min-width="200" show-overflow-tooltip>
                  <template slot-scope="scope">
                    {{ scope.row.command && scope.row.command.length ? scope.row.command.join(' ') : '-' }}
                  </template>
                </el-table-column>
              </el-table>
            </section>
            <section class="overview-card">
              <header class="overview-card__header">
                <h3>卷</h3>
                <span class="overview-card__sub">模板引用</span>
              </header>
              <el-table
                :data="workloadDrawer.details.volumes"
                size="mini"
                border
                empty-text="暂无 Volume"
              >
                <el-table-column label="卷名" prop="name" min-width="160" show-overflow-tooltip />
                <el-table-column label="类型" prop="kind" width="160" />
                <el-table-column label="来源" prop="source_display" min-width="200" show-overflow-tooltip />
              </el-table>
            </section>
          </el-tab-pane>

          <el-tab-pane label="实例列表" name="pods">
            <el-table
              :data="workloadDrawer.details.pods"
              size="mini"
              border
              empty-text="暂无 Pod"
            >
              <el-table-column label="名称" prop="name" min-width="160" show-overflow-tooltip />
              <el-table-column label="命名空间" prop="namespace" width="140" />
              <el-table-column label="READY" prop="ready" width="100" />
              <el-table-column label="状态" prop="status" min-width="140" show-overflow-tooltip />
              <el-table-column label="重启" prop="restarts" width="80" />
              <el-table-column label="节点" prop="node" min-width="140" show-overflow-tooltip />
              <el-table-column label="容器" prop="containersText" min-width="180" show-overflow-tooltip />
              <el-table-column label="时长" prop="age" width="120" />
            </el-table>
          </el-tab-pane>

          <el-tab-pane label="访问方式" name="access">
            <section class="workload-section">
              <header class="workload-section__header">
                <h3>Service</h3>
                <span class="workload-section__count">{{ workloadDrawer.details.services.length }}</span>
              </header>
              <el-table :data="workloadDrawer.details.services" size="mini" border empty-text="暂无 Service">
                <el-table-column label="名称" prop="name" min-width="160" show-overflow-tooltip />
                <el-table-column label="命名空间" prop="namespace" width="140" />
                <el-table-column label="类型" prop="type" width="120" />
                <el-table-column label="标签" prop="labelsText" min-width="200" show-overflow-tooltip />
                <el-table-column label="操作" width="160">
                  <template slot-scope="scope">
                    <el-dropdown @command="cmd => handleNamedResourceCommand(cmd, scope.row)">
                      <span class="dropdown-link">
                        操作<i class="el-icon-arrow-down el-icon--right" />
                      </span>
                      <el-dropdown-menu slot="dropdown">
                        <el-dropdown-item command="editYaml">编辑 YAML</el-dropdown-item>
                        <el-dropdown-item command="recreate">重新创建</el-dropdown-item>
                        <el-dropdown-item divided command="delete">删除</el-dropdown-item>
                      </el-dropdown-menu>
                    </el-dropdown>
                  </template>
                </el-table-column>
              </el-table>
            </section>
            <section class="workload-section">
              <header class="workload-section__header">
                <h3>Ingress</h3>
                <span class="workload-section__count">{{ workloadDrawer.details.ingresses.length }}</span>
              </header>
              <el-table :data="workloadDrawer.details.ingresses" size="mini" border empty-text="暂无 Ingress">
                <el-table-column label="名称" prop="name" min-width="160" show-overflow-tooltip />
                <el-table-column label="命名空间" prop="namespace" width="140" />
                <el-table-column label="标签" prop="labelsText" min-width="200" show-overflow-tooltip />
              </el-table>
            </section>
            <section class="workload-section">
              <header class="workload-section__header">
                <h3>Endpoints</h3>
                <span class="workload-section__count">{{ workloadDrawer.details.endpoints.length }}</span>
              </header>
              <el-table :data="workloadDrawer.details.endpoints" size="mini" border empty-text="暂无 Endpoint">
                <el-table-column label="名称" prop="name" min-width="160" show-overflow-tooltip />
                <el-table-column label="命名空间" prop="namespace" width="140" />
                <el-table-column label="标签" prop="labelsText" min-width="200" show-overflow-tooltip />
              </el-table>
            </section>
            <section class="workload-section">
              <header class="workload-section__header">
                <h3>配置引用</h3>
              </header>
              <div class="resource-split">
                <div class="resource-split__pane">
                  <h4>ConfigMap</h4>
                  <el-table :data="workloadDrawer.details.configmaps" size="mini" border empty-text="暂无 ConfigMap">
                    <el-table-column label="名称" prop="name" min-width="160" show-overflow-tooltip />
                    <el-table-column label="命名空间" prop="namespace" width="140" />
                  </el-table>
                </div>
                <div class="resource-split__pane">
                  <h4>Secret</h4>
                  <el-table :data="workloadDrawer.details.secrets" size="mini" border empty-text="暂无 Secret">
                    <el-table-column label="名称" prop="name" min-width="160" show-overflow-tooltip />
                    <el-table-column label="命名空间" prop="namespace" width="140" />
                    <el-table-column label="类型" prop="type" min-width="140" />
                  </el-table>
                </div>
              </div>
              <div class="resource-split">
                <div class="resource-split__pane">
                  <h4>PVC</h4>
                  <el-table :data="workloadDrawer.details.pvcs" size="mini" border empty-text="暂无 PVC">
                    <el-table-column label="名称" prop="name" min-width="160" show-overflow-tooltip />
                    <el-table-column label="命名空间" prop="namespace" width="140" />
                  </el-table>
                </div>
              </div>
            </section>
          </el-tab-pane>
          <el-tab-pane label="历史版本" name="history">
            <el-table
              v-loading="workloadDrawer.historyLoading"
              :data="workloadDrawer.history"
              size="mini"
              border
              empty-text="暂无历史版本"
            >
              <el-table-column label="Revision" prop="revision" width="120" />
              <el-table-column label="镜像" min-width="280">
                <template slot-scope="scope">
                  {{ scope.row.images && scope.row.images.length ? scope.row.images.join(', ') : '-' }}
                </template>
              </el-table-column>
              <el-table-column label="创建时间" min-width="180">
                <template slot-scope="scope">
                  {{ scope.row.createdAt ? formatTime(scope.row.createdAt * 1000) : '-' }}
                </template>
              </el-table-column>
              <el-table-column label="操作" width="160">
                <template slot-scope="scope">
                  <el-button type="text" size="mini" @click="handleWorkloadRollback(scope.row)">回滚</el-button>
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>
          <el-tab-pane label="事件" name="events">
            <el-table
              v-loading="workloadDrawer.events.loading"
              :data="workloadDrawer.events.items"
              size="mini"
              border
              empty-text="暂无事件"
            >
              <el-table-column label="时间" min-width="180">
                <template slot-scope="scope">
                  {{ scope.row.last_timestamp ? formatTime(scope.row.last_timestamp * 1000) : '-' }}
                </template>
              </el-table-column>
              <el-table-column label="类型" prop="type" width="120" />
              <el-table-column label="原因" prop="reason" width="160" />
              <el-table-column label="消息" prop="message" min-width="240" show-overflow-tooltip />
              <el-table-column label="次数" prop="count" width="80" />
            </el-table>
          </el-tab-pane>
          <el-tab-pane label="日志" name="logs">
            <div class="logs-toolbar">
              <el-input
                v-model="workloadDrawer.logs.labelSelector"
                placeholder="labelSelector, 例如 app=demo"
                clearable
                class="logs-toolbar__item"
              />
              <el-select
                v-model="workloadDrawer.logs.selectedContainers"
                multiple
                collapse-tags
                filterable
                placeholder="容器（可选）"
                class="logs-toolbar__item"
              >
                <el-option
                  v-for="item in workloadDrawer.logs.availableContainers"
                  :key="item"
                  :label="item"
                  :value="item"
                />
              </el-select>
              <el-switch
                v-model="workloadDrawer.logs.allContainers"
                active-text="全部容器"
                inactive-text="单个容器"
                class="logs-toolbar__item"
              />
              <el-input-number
                v-model="workloadDrawer.logs.tail"
                :min="10"
                :max="1000"
                label="Tail"
                class="logs-toolbar__item"
              />
              <el-button type="primary" :loading="workloadDrawer.logs.loading" @click="fetchWorkloadLogs">
                拉取日志
              </el-button>
            </div>
            <el-alert
              v-if="workloadDrawer.logs.error"
              :title="workloadDrawer.logs.error"
              type="error"
              show-icon
              class="logs-alert"
            />
            <div class="workload-logs-viewer" :class="{ 'workload-logs-viewer--loading': workloadDrawer.logs.loading }">
              <pre>{{ workloadDrawer.logs.content || '暂无日志' }}</pre>
            </div>
            <p v-if="workloadDrawer.logs.lastUpdated" class="workload-logs-meta">
              最近更新时间：{{ formatTime(new Date(workloadDrawer.logs.lastUpdated)) }}
            </p>
          </el-tab-pane>
        </el-tabs>
      </div>
    </el-drawer>
  </div>
</template>

<script>
import hljs from 'highlight.js/lib/core'
import yamlLanguage from 'highlight.js/lib/languages/yaml'
import jsyaml from 'js-yaml'
import 'highlight.js/styles/github.css'
import 'codemirror/lib/codemirror.css'
import 'codemirror/theme/material.css'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import 'xterm/css/xterm.css'
import { getToken } from '@/utils/auth'

hljs.registerLanguage('yaml', yamlLanguage)

let CodeMirrorInstance = null
let codeMirrorYamlReady = false
const defaultTextDecoder = typeof TextDecoder !== 'undefined' ? new TextDecoder() : null
import {
  listClusters,
  listNamespaces,
  listResources,
  getResource,
  applyManifest,
  deleteResource,
  listWorkloadPods,
  getWorkloadDetails,
  getWorkloadHistory,
  rollbackWorkload,
  getWorkloadLogs,
  listEvents
} from '@/api/admin/k8s'

const workloadTargets = [
  { kind: 'Deployment', group: 'apps', version: 'v1', resource: 'deployments' },
  { kind: 'StatefulSet', group: 'apps', version: 'v1', resource: 'statefulsets' },
  { kind: 'DaemonSet', group: 'apps', version: 'v1', resource: 'daemonsets' }
]

const emptyWorkloadDetails = () => ({
  workload: null,
  overview: {
    kind: '',
    name: '',
    namespace: '',
    labels: {},
    annotations: {},
    selector: {},
    strategy: {},
    replica: { desired: 0, ready: 0, available: 0, updated: 0 },
    conditions: [],
    containers: [],
    creation_timestamp: 0,
    update_timestamp: 0
  },
  pods: [],
  services: [],
  endpoints: [],
  ingresses: [],
  configmaps: [],
  secrets: [],
  volumes: [],
  pvcs: []
})

const defaultWorkloadEventsState = () => ({
  loading: false,
  loaded: false,
  items: [],
  page: 1,
  pageSize: 20,
  total: 0
})

const defaultWorkloadLogsState = () => ({
  loading: false,
  content: '',
  labelSelector: '',
  tail: 200,
  allContainers: true,
  availableContainers: [],
  selectedContainers: [],
  lastUpdated: 0,
  error: ''
})

const normalizeClusterId = value => {
  if (value === undefined || value === null || value === '') {
    return null
  }
  const parsed = Number(value)
  return Number.isNaN(parsed) ? value : parsed
}

export default {
  name: 'KubernetesAdmin',
  data() {
    return {
      loading: true,
      clusters: [],
      selectedClusterId: normalizeClusterId(this.$route.params.clusterId),
      namespaces: [],
      allNamespaceValue: '__all_namespaces__',
      selectedNamespace: '__all_namespaces__',
      resourceTypes: [
        { key: 'workloads', menuLabel: 'Workloads', resources: workloadTargets },
        { key: 'pods', menuLabel: 'Pods', resources: [{ kind: 'Pod', group: '', version: 'v1', resource: 'pods' }] },
        { key: 'services', menuLabel: 'Services', resources: [{ kind: 'Service', group: '', version: 'v1', resource: 'services' }] },
        {
          key: 'ingress',
          menuLabel: 'Ingress',
          resources: [{ kind: 'Ingress', group: 'networking.k8s.io', version: 'v1', resource: 'ingresses' }]
        },
        {
          key: 'config',
          menuLabel: 'Config',
          resources: [
            { kind: 'ConfigMap', group: '', version: 'v1', resource: 'configmaps' },
            { kind: 'Secret', group: '', version: 'v1', resource: 'secrets' }
          ]
        },
        {
          key: 'jobs',
          menuLabel: 'Jobs & CronJobs',
          resources: [
            { kind: 'Job', group: 'batch', version: 'v1', resource: 'jobs' },
            { kind: 'CronJob', group: 'batch', version: 'v1', resource: 'cronjobs' }
          ]
        },
        {
          key: 'volumes',
          menuLabel: 'Volumes',
          resources: [
            { kind: 'PersistentVolumeClaim', group: '', version: 'v1', resource: 'persistentvolumeclaims' },
            { kind: 'PersistentVolume', group: '', version: 'v1', resource: 'persistentvolumes', namespaced: false }
          ]
        }
      ],
      selectedResourceType: 'workloads',
      resources: [],
      resourceLoading: false,
      searchKeyword: '',
      yamlDialog: {
        visible: false,
        title: '',
        content: '',
        editable: false,
        loading: false,
        saving: false,
        target: null,
        editor: null
      },
      workloadDrawer: {
        visible: false,
        title: '',
        loading: false,
        activeTab: 'overview',
        context: null,
        details: emptyWorkloadDetails(),
        history: [],
        historyLoading: false,
        events: defaultWorkloadEventsState(),
        logs: defaultWorkloadLogsState()
      },
      workloadPodsCache: {},
      workloadPodsLoading: {},
      page: 1,
      pageSize: 10,
      pageSizeOptions: [10, 20, 50, 100],
      tableHeight: 520,
      execDialog: {
        visible: false,
        shell: 'bash',
        pod: null,
        container: '',
        rowKey: '',
        connected: false
      },
      execTerminal: null,
      execTerminalFitAddon: null,
      execTerminalDisposer: null,
      execSocket: null,
      logsDialog: {
        visible: false,
        pod: null,
        container: '',
        content: '',
        autoScroll: true
      },
      logsSocket: null,
      execTerminalTextarea: null
    }
  },
  computed: {
    rootLoading() {
      return this.loading && !this.execDialog.visible && !this.logsDialog.visible
    },
    selectedCluster() {
      if (this.selectedClusterId === null || this.selectedClusterId === undefined) {
        return null
      }
      const target = String(this.selectedClusterId)
      return this.clusters.find(item => String(item.id) === target) || null
    },
    currentClusterId() {
      return normalizeClusterId(this.$route.params.clusterId)
    },
    currentResourceType() {
      return this.resourceTypes.find(item => item.key === this.selectedResourceType) || this.resourceTypes[0]
    },
    resourceMenu() {
      return this.resourceTypes.map(item => ({ key: item.key, menuLabel: item.menuLabel }))
    },
    filteredResources() {
      const keyword = this.searchKeyword.trim().toLowerCase()
      if (!keyword) {
        return this.resources
      }
      return this.resources.filter(item => {
        const name = item.metadata && item.metadata.name ? item.metadata.name.toLowerCase() : ''
        const namespace = item.metadata && item.metadata.namespace ? item.metadata.namespace.toLowerCase() : ''
        return name.includes(keyword) || namespace.includes(keyword)
      })
    },
    paginatedResources() {
      const start = (this.page - 1) * this.pageSize
      return this.filteredResources.slice(start, start + this.pageSize)
    },
    totalPages() {
      if (!this.filteredResources.length) return 1
      return Math.max(1, Math.ceil(this.filteredResources.length / this.pageSize))
    }
  },
  watch: {
    searchKeyword() {
      this.page = 1
      this.updateTableHeight()
    },
    selectedResourceType() {
      this.resetWorkloadPods()
    },
    currentClusterId(newId, oldId) {
      if (newId === oldId) {
        return
      }
      this.selectedClusterId = newId
      if (!newId) {
        this.resetWorkspace()
        return
      }
      if (this.clusters.length) {
        const exists = this.clusters.some(cluster => this.clusterEquals(cluster.id, newId))
        if (!exists) {
          this.bootstrap()
        } else {
          this.handleClusterChange()
        }
      } else {
        this.bootstrap()
      }
    }
  },
  created() {
    this.bootstrap()
  },
  mounted() {
    this.updateTableHeight()
    window.addEventListener('resize', this.updateTableHeight)
    window.addEventListener('keydown', this.handleGlobalKeydown, true)
  },
  beforeDestroy() {
    window.removeEventListener('resize', this.updateTableHeight)
    window.removeEventListener('keydown', this.handleGlobalKeydown, true)
    this.handleExecClose()
    this.handleLogsClose()
  },
  methods: {
    resolveRowKey(row) {
      if (!row || !row.metadata) return ''
      return row.metadata.uid || row.metadata.name || ''
    },
    async ensureCodeMirror() {
      if (!CodeMirrorInstance) {
        const imported = await import('codemirror')
        // eslint-disable-next-line require-atomic-updates
        CodeMirrorInstance = imported.default || imported
      }
      if (!codeMirrorYamlReady) {
        await import('codemirror/mode/yaml/yaml')
        // eslint-disable-next-line require-atomic-updates
        codeMirrorYamlReady = true
      }
      return CodeMirrorInstance
    },
    goDashboard() {
      this.$router.push('/dashboard')
    },
    goClusterList() {
      this.$router.push({ name: 'AdminK8sClusters' })
    },
    clusterEquals(a, b) {
      if (a === undefined || a === null || b === undefined || b === null) {
        return false
      }
      return String(a) === String(b)
    },
    async bootstrap() {
      this.loading = true
      try {
        const clusterList = await listClusters()
        this.clusters = Array.isArray(clusterList) ? clusterList : []
        if (!this.selectedClusterId) {
          this.resetWorkspace()
          return
        }
        const exists = this.clusters.find(cluster => this.clusterEquals(cluster.id, this.selectedClusterId))
        if (!exists) {
          this.$message.warning('该集群不存在或已删除')
          this.goClusterList()
          return
        }
        await this.handleClusterChange()
      } catch (err) {
        this.$message.error(err.message || '加载集群失败')
      } finally {
        this.loading = false
      }
    },
    async fetchNamespaces() {
      if (!this.selectedClusterId) return
      const list = await listNamespaces(this.selectedClusterId)
      this.namespaces = list || []
      const current = this.selectedNamespace
      if (current && current !== this.allNamespaceValue && this.namespaces.find(ns => ns.name === current)) {
        this.selectedNamespace = current
      } else {
        this.selectedNamespace = this.allNamespaceValue
      }
    },
    async fetchResources(options = {}) {
      if (!this.selectedClusterId) return
      const refreshNamespaces = options.refreshNamespaces !== false
      if (refreshNamespaces) {
        await this.fetchNamespaces()
      }
      const type = this.currentResourceType
      if (!type) return
      const namespaceFilter = this.selectedNamespace === this.allNamespaceValue ? '' : this.selectedNamespace
      const targets = type.resources || []
      this.resourceLoading = true
      try {
        const requests = targets.map(target => {
          return listResources(this.selectedClusterId, {
            group: target.group,
            version: target.version,
            resource: target.resource,
            namespace: target.namespaced === false ? '' : namespaceFilter
          }).then(items => ({ items: items || [], meta: target }))
        })
        const responses = await Promise.all(requests)
        const merged = []
        responses.forEach(({ items, meta }) => {
          items.forEach(item => merged.push(this.decorateResource(item, meta, type.key)))
        })
        this.resources = merged
        this.resetWorkloadPods()
        this.page = 1
        this.updateTableHeight()
      } catch (err) {
        this.$message.error(err.message || '加载资源失败')
      } finally {
        this.resourceLoading = false
        this.updateTableHeight()
      }
    },
    async handleClusterChange() {
      if (!this.selectedClusterId) {
        this.resetWorkspace()
        return
      }
      this.selectedNamespace = this.allNamespaceValue
      this.selectedResourceType = this.resourceTypes[0].key
      this.searchKeyword = ''
      await this.fetchNamespaces()
      await this.fetchResources({ refreshNamespaces: false })
    },
    resetClusterSelection() {
      this.selectedClusterId = null
      this.resetWorkspace()
      this.goClusterList()
    },
    resetWorkspace() {
      this.namespaces = []
      this.selectedNamespace = this.allNamespaceValue
      this.resources = []
      this.searchKeyword = ''
      this.resetWorkloadPods()
    },
    handleNamespaceChange() {
      this.page = 1
      this.resetWorkloadPods()
      this.fetchResources({ refreshNamespaces: false })
    },
    handleResourceTypeChange(key) {
      if (this.selectedResourceType === key) return
      this.selectedResourceType = key
      this.page = 1
      this.resetWorkloadPods()
      this.fetchResources()
    },
    handlePageChange(page) {
      this.page = page
      this.updateTableHeight()
    },
    handlePageSizeChange(size) {
      const parsed = Number(size) || this.pageSizeOptions[0]
      this.pageSize = parsed
      this.page = 1
      this.updateTableHeight()
    },
    updateTableHeight() {
      this.$nextTick(() => {
        const wrapper = this.$refs.tableWrapper
        if (!wrapper) {
          const fallback = window.innerHeight - 200
          this.tableHeight = fallback > 360 ? fallback : 360
          return
        }
        const rect = wrapper.getBoundingClientRect()
        const footerReserve = this.filteredResources.length ? 140 : 48
        const available = window.innerHeight - rect.top - footerReserve
        this.tableHeight = available > 360 ? available : 360
      })
    },
    decorateResource(item, target, typeKey) {
      const decorated = item
      decorated.__gvr = {
        group: target.group || '',
        version: target.version || 'v1',
        resource: target.resource,
        namespaced: target.namespaced !== false
      }
      decorated.__resourceKey = typeKey
      decorated.__kind = item.kind || target.kind || this.inferKindFromResource(target.resource)
      decorated.__namespace = (item.metadata && item.metadata.namespace) || ''
      decorated.__status = this.buildStatus(decorated.__kind, decorated)
      decorated.__createAt = item.metadata && item.metadata.creationTimestamp
      decorated.__updateAt = this.extractUpdateTime(item)
      const images = this.collectImages(decorated)
      decorated.__images = images
      decorated.__imageTag = images.length ? this.extractImageTag(images[0]) : ''
      return decorated
    },
    collectImages(resource) {
      const images = new Set()
      const addFromContainers = containers => {
        if (!Array.isArray(containers)) return
        containers.forEach(container => {
          if (container && container.image) {
            images.add(container.image)
          }
        })
      }
      const addFromPodSpec = spec => {
        if (!spec) return
        addFromContainers(spec.containers)
        addFromContainers(spec.initContainers)
      }
      if (resource.kind === 'Pod') {
        addFromPodSpec(resource.spec)
      }
      if (resource.spec && resource.spec.template) {
        addFromPodSpec(resource.spec.template.spec)
      }
      if (
        resource.spec &&
        resource.spec.jobTemplate &&
        resource.spec.jobTemplate.spec &&
        resource.spec.jobTemplate.spec.template
      ) {
        addFromPodSpec(resource.spec.jobTemplate.spec.template.spec)
      }
      return Array.from(images)
    },
    extractImageTag(image) {
      if (!image) return ''
      const withoutDigest = image.split('@')[0]
      const tail = withoutDigest.split('/').pop() || withoutDigest
      return tail || image
    },
    inferKindFromResource(resource) {
      if (!resource) return 'Unknown'
      return resource.replace(/s$/, '').replace(/(^|-)\w/g, s => s.replace('-', '').toUpperCase())
    },
    buildStatus(kind, resource) {
      if (!kind || !resource) {
        return { text: '未知', type: 'default' }
      }
      const spec = resource.spec || {}
      const status = resource.status || {}
      if (kind === 'Deployment') {
        const desired = typeof spec.replicas === 'number' ? spec.replicas : 0
        const available = status.availableReplicas || 0
        if (desired === 0) {
          return { text: '已停止', type: 'info' }
        }
        if (available >= desired && desired !== 0) {
          return { text: `运行中 (${available}/${desired})`, type: 'success' }
        }
        if (available > 0) {
          return { text: `部分可用 (${available}/${desired})`, type: 'warning' }
        }
        return { text: '未就绪', type: 'danger' }
      }
      if (kind === 'StatefulSet') {
        const ready = status.readyReplicas || 0
        const replicas = typeof spec.replicas === 'number' ? spec.replicas : 0
        return {
          text: replicas ? `运行中 (${ready}/${replicas})` : '运行中',
          type: ready === replicas ? 'success' : 'warning'
        }
      }
      if (kind === 'DaemonSet') {
        const desired = status.desiredNumberScheduled || 0
        const ready = status.numberReady || 0
        return {
          text: desired ? `运行中 (${ready}/${desired})` : '运行中',
          type: ready === desired ? 'success' : 'warning'
        }
      }
      if (kind === 'Pod') {
        const total = Array.isArray(spec.containers) ? spec.containers.length : 0
        const readyContainers = Array.isArray(status.containerStatuses)
          ? status.containerStatuses.filter(cs => cs.ready).length
          : 0
        const phase = status.phase || 'Unknown'
        const mapping = {
          Running: 'success',
          Pending: 'warning',
          Failed: 'danger',
          Succeeded: 'info'
        }
        return { text: `${phase} (${readyContainers}/${total})`, type: mapping[phase] || 'default' }
      }
      if (kind === 'Service') {
        const type = spec.type || 'ClusterIP'
        return { text: type, type: 'info' }
      }
      if (kind === 'ConfigMap' || kind === 'Secret') {
        return { text: '可用', type: 'info' }
      }
      if (kind === 'Job') {
        const succeeded = status.succeeded || 0
        const completions = spec.completions || 0
        const failed = status.failed || 0
        if (succeeded && (!completions || succeeded >= completions)) {
          return { text: `完成 (${succeeded})`, type: 'success' }
        }
        if (failed) {
          return { text: `失败 (${failed})`, type: 'danger' }
        }
        return { text: '运行中', type: 'info' }
      }
      if (kind === 'CronJob') {
        const suspend = spec.suspend
        return suspend ? { text: '已暂停', type: 'warning' } : { text: '调度中', type: 'success' }
      }
      if (kind === 'Ingress') {
        const rules = Array.isArray(spec.rules) ? spec.rules.length : 0
        const tls = Array.isArray(spec.tls) ? spec.tls.length : 0
        return { text: rules ? `规则 ${rules}` : 'Ingress', type: tls ? 'info' : 'success' }
      }
      if (kind === 'PersistentVolumeClaim') {
        const phase = status.phase || 'Unknown'
        const mapping = {
          Bound: 'success',
          Pending: 'warning',
          Lost: 'danger'
        }
        return { text: phase, type: mapping[phase] || 'info' }
      }
      if (kind === 'PersistentVolume') {
        const phase = status.phase || 'Unknown'
        return { text: phase, type: phase === 'Available' ? 'success' : 'info' }
      }
      return { text: kind, type: 'info' }
    },
    extractUpdateTime(resource) {
      const managed = resource.metadata && resource.metadata.managedFields
      if (Array.isArray(managed) && managed.length) {
        const sorted = managed
          .filter(field => !!field.time)
          .sort((a, b) => new Date(b.time).getTime() - new Date(a.time).getTime())
        if (sorted.length && sorted[0].time) {
          return sorted[0].time
        }
      }
      const conditions = resource.status && resource.status.conditions
      if (Array.isArray(conditions) && conditions.length) {
        const sorted = conditions
          .filter(cond => !!cond.lastUpdateTime)
          .sort((a, b) => new Date(b.lastUpdateTime).getTime() - new Date(a.lastUpdateTime).getTime())
        if (sorted.length && sorted[0].lastUpdateTime) {
          return sorted[0].lastUpdateTime
        }
      }
      return (resource.metadata && resource.metadata.creationTimestamp) || ''
    },
    handleRowCommand(command, row) {
      switch (command) {
        case 'editYaml':
          this.openYamlDialog(row, true)
          break
        case 'recreate':
          this.recreateResource(row)
          break
        case 'delete':
          this.deleteResourceRow(row)
          break
        case 'pods':
          this.openResourceDetail(row)
          break
        default:
          break
      }
    },
    handleRowExpand(row) {
      if (this.selectedResourceType !== 'workloads') {
        return
      }
      this.loadWorkloadPods(row)
    },
    async loadWorkloadPods(row, options = {}) {
      if (!this.selectedClusterId || this.selectedResourceType !== 'workloads') {
        return
      }
      const key = this.resolveRowKey(row)
      if (!key || !row || !row.metadata || !row.metadata.name) {
        return
      }
      if (!options.force && Array.isArray(this.workloadPodsCache[key])) {
        return
      }
      const kind = (row.__kind || '').toLowerCase()
      if (!kind) return
      this.$set(this.workloadPodsLoading, key, true)
      try {
        const pods = await listWorkloadPods(this.selectedClusterId, {
          kind,
          namespace: row.__namespace || '',
          name: row.metadata.name
        })
        this.$set(this.workloadPodsCache, key, Array.isArray(pods) ? pods : [])
      } catch (err) {
        this.$message.error(err.message || '加载 Pod 失败')
      } finally {
        this.$set(this.workloadPodsLoading, key, false)
      }
    },
    getWorkloadPods(row) {
      const key = this.resolveRowKey(row)
      if (!key) return []
      return this.workloadPodsCache[key] || []
    },
    isPodsLoading(row) {
      const key = this.resolveRowKey(row)
      if (!key) return false
      return !!this.workloadPodsLoading[key]
    },
    resetWorkloadPods() {
      this.workloadPodsCache = {}
      this.workloadPodsLoading = {}
      this.handleExecClose()
      this.handleLogsClose()
    },
    handlePodCommand(command, row, pod) {
      switch (command) {
        case 'exec-bash':
          this.openExecDialog(row, pod, 'bash')
          break
        case 'exec-sh':
          this.openExecDialog(row, pod, 'sh')
          break
        case 'logs':
          this.openLogsDialog(row, pod)
          break
        case 'delete':
          this.deletePodRow(row, pod)
          break
        default:
          break
      }
    },
    openExecDialog(row, pod, shell) {
      if (!pod) return
      this.execDialog.visible = true
      this.execDialog.shell = shell || 'bash'
      this.execDialog.pod = pod
      this.execDialog.rowKey = this.resolveRowKey(row)
      const containers = Array.isArray(pod.containers) && pod.containers.length ? pod.containers : ['']
      this.execDialog.container = containers[0]
      this.execDialog.connected = false
      this.$nextTick(() => {
        this.initializeExecTerminal()
        this.connectExecSocket()
        // Add window resize listener for terminal
        window.addEventListener('resize', this.handleExecTerminalResize)
      })
    },
    handleExecClose() {
      this.closeExecSocket()
      // Remove window resize listener
      window.removeEventListener('resize', this.handleExecTerminalResize)
      this.execDialog.visible = false
      this.execDialog.pod = null
      this.execDialog.rowKey = ''
      this.execDialog.connected = false
      this.execTerminalTextarea = null
    },
    async deletePodRow(row, pod) {
      if (!pod || !this.selectedClusterId) return
      try {
        await this.$confirm(`确定删除 Pod ${pod.name} 吗？`, '提示', { type: 'warning' })
      } catch (err) {
        return
      }
      try {
        await deleteResource(this.selectedClusterId, {
          group: '',
          version: 'v1',
          resource: 'pods',
          namespace: pod.namespace,
          name: pod.name
        })
        this.$message.success('Pod 已删除')
        await this.loadWorkloadPods(row, { force: true })
      } catch (err) {
        this.$message.error(err.message || '删除失败')
      }
    },
    formatPodAge(timestamp) {
      if (!timestamp) return '-'
      const diff = Date.now() - timestamp * 1000
      if (diff <= 0) return '0s'
      const seconds = Math.floor(diff / 1000)
      const days = Math.floor(seconds / 86400)
      const hours = Math.floor((seconds % 86400) / 3600)
      const minutes = Math.floor((seconds % 3600) / 60)
      if (days > 0) {
        return `${days}d${hours ? ` ${hours}h` : ''}`.trim()
      }
      if (hours > 0) {
        return `${hours}h${minutes ? ` ${minutes}m` : ''}`.trim()
      }
      const secs = seconds % 60
      if (minutes > 0) {
        return `${minutes}m${secs ? ` ${secs}s` : ''}`.trim()
      }
      return `${secs}s`
    },
    handleExecContainerChange() {
      if (this.execDialog.visible) {
        this.connectExecSocket()
      }
    },
    handleExecShellChange() {
      if (this.execDialog.visible) {
        this.connectExecSocket()
      }
    },
    initializeExecTerminal() {
      const mount = this.$refs.execTerminal
      if (!mount) return

      // Cleanup existing terminal disposer
      if (this.execTerminalDisposer) {
        this.execTerminalDisposer.dispose()
        this.execTerminalDisposer = null
      }

      // Create or reuse terminal
      if (!this.execTerminal) {
        this.execTerminal = new Terminal({
          fontSize: 14,
          fontFamily: 'JetBrains Mono, "SF Mono", Menlo, Consolas, "Courier New", monospace',
          theme: {
            background: '#000000',
            foreground: '#e2e8f0',
            cursor: '#60a5fa',
            cursorAccent: '#000000',
            selection: 'rgba(96, 165, 250, 0.3)',
            black: '#000000',
            red: '#f87171',
            green: '#34d399',
            yellow: '#fbbf24',
            blue: '#60a5fa',
            magenta: '#c084fc',
            cyan: '#22d3ee',
            white: '#e2e8f0',
            brightBlack: '#475569',
            brightRed: '#fca5a5',
            brightGreen: '#6ee7b7',
            brightYellow: '#fcd34d',
            brightBlue: '#93c5fd',
            brightMagenta: '#d8b4fe',
            brightCyan: '#67e8f9',
            brightWhite: '#f8fafc'
          },
          cursorBlink: true,
          cursorStyle: 'block',
          convertEol: true,
          scrollback: 1000,
          tabStopWidth: 8
        })

        // Create and load FitAddon
        this.execTerminalFitAddon = new FitAddon()
        this.execTerminal.loadAddon(this.execTerminalFitAddon)

        this.execTerminal.open(mount)
      } else if (this.execTerminal.element && this.execTerminal.element.parentNode !== mount) {
        mount.innerHTML = ''
        this.execTerminal.open(mount)
      }

      // Fit terminal to container & capture textarea reference
      this.$nextTick(() => {
        if (this.execTerminalFitAddon) {
          try {
            this.execTerminalFitAddon.fit()
          } catch (e) {
            console.warn('Failed to fit terminal:', e)
          }
        }
        const helper =
          mount.querySelector && mount.querySelector('.xterm-helper-textarea')
            ? mount.querySelector('.xterm-helper-textarea')
            : document.querySelector('.xterm-helper-textarea')
        if (helper) {
          this.execTerminalTextarea = helper
        } else if (this.execTerminal && this.execTerminal.textarea) {
          this.execTerminalTextarea = this.execTerminal.textarea
        }
        this.focusExecTerminal()
      })

      this.execTerminal.clear()

      // Handle terminal input - send to websocket
      this.execTerminalDisposer = this.execTerminal.onData(data => {
        this.sendExecFrame({ op: 'stdin', data })
      })
    },
    connectExecSocket() {
      this.closeExecSocket()
      if (!this.execDialog.pod || !this.selectedClusterId) {
        return
      }
      this.execDialog.connected = false
      const pod = this.execDialog.pod
      const shell = this.execDialog.shell === 'sh' ? '/bin/sh' : '/bin/bash'
      const path = `/admin/k8s/clusters/${this.selectedClusterId}/pods/${encodeURIComponent(
        pod.namespace
      )}/${encodeURIComponent(pod.name)}/exec/stream`
      const wsUrl = this.buildWebsocketUrl(path, {
        container: this.execDialog.container,
        shell,
        token: getToken()
      })
      const socket = new WebSocket(wsUrl)
      this.execSocket = socket
      socket.onopen = () => {
        this.execDialog.connected = true
        this.execTerminal && this.execTerminal.writeln('\r\n[已连接，请开始输入命令...]')
        this.focusExecTerminal()
        this.sendResizeFrame()
      }
      socket.onmessage = event => {
        if (typeof event.data !== 'string') {
          return
        }
        try {
          const frame = JSON.parse(event.data)
          this.handleExecFrame(frame)
        } catch (err) {
          console.warn('invalid frame', err)
        }
      }
      socket.onclose = () => {
        this.execDialog.connected = false
        this.execTerminal && this.execTerminal.writeln('\r\n[连接已关闭]')
      }
      socket.onerror = () => {
        this.$message.error('终端连接失败')
        this.execTerminal && this.execTerminal.writeln('\r\n[终端连接异常]')
      }
    },
    reconnectExecSocket() {
      if (!this.execDialog.visible) return
      this.execTerminal && this.execTerminal.writeln('\r\n[正在重新连接...]')
      this.connectExecSocket()
      // Re-focus terminal after reconnect
      this.$nextTick(() => {
        this.focusExecTerminal()
      })
    },
    clearExecTerminal() {
      if (this.execTerminal) {
        this.execTerminal.clear()
      }
    },
    handleExecFrame(frame) {
      if (!frame || !frame.op) return
      switch (frame.op) {
        case 'stdout':
        case 'stderr':
          if (frame.data && this.execTerminal) {
            this.execTerminal.write(frame.data)
          }
          break
        case 'status':
        case 'error':
          if (frame.data && this.execTerminal) {
            this.execTerminal.writeln(`\r\n[${frame.data}]`)
          }
          if (frame.op === 'error') {
            this.execDialog.connected = false
          }
          break
        default:
          break
      }
    },
    sendExecFrame(payload) {
      if (!payload || !this.execSocket || this.execSocket.readyState !== WebSocket.OPEN) {
        return
      }
      try {
        this.execSocket.send(JSON.stringify(payload))
      } catch (err) {
        console.warn('send frame failed', err)
      }
    },
    sendResizeFrame() {
      if (!this.execTerminal) return
      this.sendExecFrame({
        op: 'resize',
        cols: this.execTerminal.cols,
        rows: this.execTerminal.rows
      })
    },
    closeExecSocket() {
      if (this.execSocket) {
        try {
          this.sendExecFrame({ op: 'close' })
        } catch (err) {
          // ignore
        }
        try {
          this.execSocket.close()
        } catch (err) {
          // ignore
        }
        this.execSocket = null
      }
      if (this.execTerminalDisposer) {
        this.execTerminalDisposer.dispose()
        this.execTerminalDisposer = null
      }
      this.execDialog.connected = false
    },
    focusExecTerminal() {
      this.$nextTick(() => {
        if (this.execTerminal) {
          this.execTerminal.focus()
        }
        // Also try to focus the underlying textarea element
        let helper = this.execTerminalTextarea
        if (
          !helper &&
          this.$refs.execTerminal &&
          this.$refs.execTerminal.querySelector &&
          this.$refs.execTerminal.querySelector('.xterm-helper-textarea')
        ) {
          helper = this.$refs.execTerminal.querySelector('.xterm-helper-textarea')
        }
        if (!helper) {
          helper = document.querySelector('.xterm-helper-textarea')
        }
        if (helper && typeof helper.focus === 'function') {
          helper.focus()
          this.execTerminalTextarea = helper
        }
      })
    },
    handleGlobalKeydown(evt) {
      if (!this.execDialog.visible || !this.execTerminal) {
        return
      }
      const textarea = this.execTerminalTextarea
      if (textarea && document.activeElement !== textarea) {
        textarea.focus()
      }
    },
    handleExecTerminalResize() {
      if (this.execTerminalFitAddon && this.execTerminal) {
        try {
          this.execTerminalFitAddon.fit()
          // Optionally notify server about terminal resize
          // This is useful for proper display of terminal content
          if (this.execSocket && this.execSocket.readyState === WebSocket.OPEN) {
            this.sendResizeFrame()
          }
        } catch (e) {
          console.warn('Failed to resize terminal:', e)
        }
      }
    },
    buildWebsocketUrl(path, params = {}) {
      const baseApi = process.env.VUE_APP_BASE_API || ''
      const query = new URLSearchParams()
      Object.keys(params || {}).forEach(key => {
        const value = params[key]
        if (value !== undefined && value !== null && value !== '') {
          query.append(key, value)
        }
      })
      let origin = ''
      if (/^https?:/i.test(baseApi)) {
        const baseUrl = new URL(baseApi, window.location.origin)
        const protocol = baseUrl.protocol === 'https:' ? 'wss:' : 'ws:'
        origin = `${protocol}//${baseUrl.host}${baseUrl.pathname.replace(/\/$/, '')}`
      } else {
        const prefix = baseApi.endsWith('/') ? baseApi.slice(0, -1) : baseApi
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
        origin = `${protocol}//${window.location.host}${prefix}`
      }
      const qs = query.toString()
      return `${origin}${path}${qs ? `?${qs}` : ''}`
    },
    openLogsDialog(row, pod) {
      if (!pod) return
      this.logsDialog.visible = true
      this.logsDialog.pod = pod
      this.logsDialog.container =
        (Array.isArray(pod.containers) && pod.containers.length && pod.containers[0]) || ''
      this.logsDialog.content = ''
      this.$nextTick(() => {
        this.connectLogsSocket()
      })
    },
    handleLogsContainerChange() {
      if (this.logsDialog.visible) {
        this.connectLogsSocket()
      }
    },
    connectLogsSocket() {
      this.closeLogsSocket()
      if (!this.logsDialog.pod || !this.selectedClusterId) return
      const pod = this.logsDialog.pod
      const path = `/admin/k8s/clusters/${this.selectedClusterId}/pods/${encodeURIComponent(
        pod.namespace
      )}/${encodeURIComponent(pod.name)}/logs/stream`
      const wsUrl = this.buildWebsocketUrl(path, {
        container: this.logsDialog.container,
        token: getToken()
      })
      const socket = new WebSocket(wsUrl)
      this.logsSocket = socket
      socket.binaryType = 'arraybuffer'
      socket.onmessage = event => {
        let chunk = ''
        if (typeof event.data === 'string') {
          chunk = event.data
        } else if (defaultTextDecoder) {
          chunk = defaultTextDecoder.decode(event.data)
        } else {
          chunk = String(event.data)
        }
        this.appendLogsChunk(chunk)
      }
      socket.onclose = () => {
        this.appendLogsChunk('\n[日志流已结束]')
      }
      socket.onerror = () => {
        this.$message.error('日志连接失败')
      }
    },
    appendLogsChunk(chunk) {
      const maxLength = 20000
      const merged = (this.logsDialog.content + chunk).slice(-maxLength)
      this.logsDialog.content = merged
      this.$nextTick(() => {
        const viewer = this.$refs.logsViewer
        if (viewer && this.logsDialog.autoScroll) {
          viewer.scrollTop = viewer.scrollHeight
        }
      })
    },
    closeLogsSocket() {
      if (this.logsSocket) {
        try {
          this.logsSocket.close()
        } catch (err) {
          // ignore
        }
        this.logsSocket = null
      }
    },
    handleLogsClose() {
      this.closeLogsSocket()
      this.logsDialog.visible = false
      this.logsDialog.pod = null
      this.logsDialog.content = ''
      this.logsDialog.autoScroll = true
    },
    async openYamlDialog(row, editable) {
      this.destroyYamlEditor()
      this.yamlDialog.visible = true
      this.yamlDialog.editable = editable
      this.yamlDialog.loading = true
      this.yamlDialog.saving = false
      this.yamlDialog.target = row
      const resourceName = row.metadata && row.metadata.name ? row.metadata.name : ''
      this.yamlDialog.title = editable ? `编辑 YAML · ${resourceName}` : `查看 YAML · ${resourceName}`
      try {
        const yamlContent = await this.fetchResourceYaml(row)
        this.yamlDialog.content = yamlContent
      } catch (err) {
        this.$message.error(err.message || '加载 YAML 失败')
        this.yamlDialog.visible = false
        return
      } finally {
        this.yamlDialog.loading = false
        this.$nextTick(() => {
          if (editable) {
            this.mountYamlEditor()
          } else {
            this.highlightYaml()
          }
        })
      }
    },
    highlightYaml() {
      const block = this.$refs.yamlViewer
      if (!block) return
      hljs.highlightElement(block)
    },
    async mountYamlEditor() {
      if (!this.yamlDialog.editable) return
      const textarea = this.$refs.yamlEditorInput
      if (!textarea) return
      const CodeMirror = await this.ensureCodeMirror()
      this.destroyYamlEditor()
      this.yamlDialog.editor = CodeMirror.fromTextArea(textarea, {
        mode: 'yaml',
        lineNumbers: true,
        theme: 'material',
        autofocus: true
      })
      this.yamlDialog.editor.on('change', cm => {
        this.yamlDialog.content = cm.getValue()
      })
      this.yamlDialog.editor.setValue(this.yamlDialog.content || '')
      this.yamlDialog.editor.setSize('100%', '440px')
      requestAnimationFrame(() => {
        if (this.yamlDialog.editor) {
          this.yamlDialog.editor.refresh()
          this.yamlDialog.editor.focus()
        }
      })
    },
    destroyYamlEditor() {
      if (this.yamlDialog.editor) {
        this.yamlDialog.content = this.yamlDialog.editor.getValue()
        this.yamlDialog.editor.toTextArea()
        this.yamlDialog.editor = null
      }
    },
    handleYamlClose() {
      this.destroyYamlEditor()
      this.yamlDialog.visible = false
      this.yamlDialog.target = null
    },
    async fetchResourceYaml(row) {
      const query = this.buildResourceQuery(row)
      const resp = await getResource(this.selectedClusterId, query)
      return this.generateManifestYaml(resp)
    },
    buildResourceQuery(row) {
      const gvr = row.__gvr || {}
      return {
        group: gvr.group,
        version: gvr.version,
        resource: gvr.resource,
        namespace: gvr.namespaced === false ? '' : ((row.metadata && row.metadata.namespace) || ''),
        name: row.metadata && row.metadata.name
      }
    },
    generateManifestYaml(resp) {
      if (!resp) return ''
      let obj = null
      if (resp.object) {
        obj = resp.object
      } else if (resp.yaml || resp.YAML) {
        try {
          obj = jsyaml.load(resp.yaml || resp.YAML)
        } catch (err) {
          obj = null
        }
      }
      if (!obj) {
        return resp.yaml || resp.YAML || ''
      }
      const cleaned = this.cleanManifestObject(obj)
      return jsyaml.dump(cleaned, { noRefs: true })
    },
    cleanManifestObject(obj) {
      if (!obj || typeof obj !== 'object') {
        return obj
      }
      const clone = JSON.parse(JSON.stringify(obj))
      delete clone.status
      this.pruneMetadata(clone.metadata)
      if (clone.spec && clone.spec.template) {
        this.pruneMetadata(clone.spec.template.metadata)
      }
      if (
        clone.spec &&
        clone.spec.jobTemplate &&
        clone.spec.jobTemplate.spec &&
        clone.spec.jobTemplate.spec.template
      ) {
        this.pruneMetadata(clone.spec.jobTemplate.spec.template.metadata)
      }
      this.stripNoiseKeys(clone)
      return clone
    },
    pruneMetadata(meta) {
      if (!meta || typeof meta !== 'object') {
        return
      }
      delete meta.creationTimestamp
      delete meta.resourceVersion
      delete meta.selfLink
      delete meta.uid
      delete meta.generation
      delete meta.managedFields
      if (meta.annotations && typeof meta.annotations === 'object') {
        Object.keys(meta.annotations).forEach(key => {
          if (
            key === 'kubectl.kubernetes.io/last-applied-configuration' ||
            key.startsWith('kubectl.kubernetes.io/') ||
            key === 'deployment.kubernetes.io/revision'
          ) {
            delete meta.annotations[key]
          }
        })
        if (!Object.keys(meta.annotations).length) {
          delete meta.annotations
        }
      }
    },
    stripNoiseKeys(target) {
      if (Array.isArray(target)) {
        target.forEach(item => this.stripNoiseKeys(item))
        return
      }
      if (!target || typeof target !== 'object') {
        return
      }
      Object.keys(target).forEach(key => {
        const value = target[key]
        if (/^(f|k):/i.test(key)) {
          delete target[key]
          return
        }
        this.stripNoiseKeys(value)
      })
    },
    validateYamlContent(content) {
      let parsed
      try {
        parsed = jsyaml.load(content)
      } catch (err) {
        throw new Error(`YAML 解析失败: ${err.message}`)
      }
      if (!parsed || typeof parsed !== 'object') {
        throw new Error('YAML 内容无效')
      }
      if (!parsed.apiVersion) {
        throw new Error('YAML 缺少 apiVersion')
      }
      if (!parsed.kind) {
        throw new Error('YAML 缺少 kind 字段')
      }
      if (!parsed.metadata || !parsed.metadata.name) {
        throw new Error('YAML 缺少 metadata.name')
      }
      if (
        this.yamlDialog.target &&
        this.yamlDialog.target.__kind &&
        parsed.kind !== this.yamlDialog.target.__kind
      ) {
        throw new Error(`不允许修改资源 kind（期望 ${this.yamlDialog.target.__kind}）`)
      }
      if (
        parsed.metadata &&
        !parsed.metadata.namespace &&
        this.yamlDialog.target &&
        this.yamlDialog.target.__gvr &&
        this.yamlDialog.target.__gvr.namespaced !== false
      ) {
        parsed.metadata.namespace =
          (this.yamlDialog.target.metadata && this.yamlDialog.target.metadata.namespace) || ''
      }
      return parsed
    },
    async submitYaml() {
      if (!this.yamlDialog.target) return
      this.yamlDialog.saving = true
      try {
        const content = this.yamlDialog.editor ? this.yamlDialog.editor.getValue() : this.yamlDialog.content
        let parsed
        try {
          parsed = this.validateYamlContent(content)
        } catch (err) {
          this.$message.error(err.message)
          return
        }
        const sanitized = this.cleanManifestObject(parsed)
        const manifestYaml = jsyaml.dump(sanitized, { noRefs: true })
        const gvr = this.yamlDialog.target.__gvr || {}
        await applyManifest(this.selectedClusterId, {
          group: gvr.group,
          version: gvr.version,
          resource: gvr.resource,
          namespace:
            gvr.namespaced === false ? '' : ((this.yamlDialog.target.metadata && this.yamlDialog.target.metadata.namespace) || ''),
          manifest: manifestYaml
        })
        this.$message.success('已保存')
        this.yamlDialog.visible = false
        this.handleYamlClose()
        await this.fetchResources()
      } catch (err) {
        this.$message.error(err.message || '保存失败')
      } finally {
        this.yamlDialog.saving = false
      }
    },
    async recreateResource(row) {
      try {
        await this.$confirm('确定要重新创建该资源吗？', '提示', { type: 'warning' })
      } catch (err) {
        return
      }
      try {
        const manifestYaml = await this.fetchResourceYaml(row)
        const gvr = row.__gvr || {}
        await applyManifest(this.selectedClusterId, {
          group: gvr.group,
          version: gvr.version,
          resource: gvr.resource,
          namespace: gvr.namespaced === false ? '' : ((row.metadata && row.metadata.namespace) || ''),
          manifest: manifestYaml
        })
        this.$message.success('已重新创建')
        await this.fetchResources()
      } catch (err) {
        this.$message.error(err.message || '重新创建失败')
      }
    },
    async deleteResourceRow(row) {
      try {
        await this.$confirm('确定删除该资源？', '提示', { type: 'warning' })
      } catch (err) {
        return
      }
      try {
        const query = this.buildResourceQuery(row)
        await deleteResource(this.selectedClusterId, query)
        this.$message.success('已删除')
        await this.fetchResources()
      } catch (err) {
        this.$message.error(err.message || '删除失败')
      }
    },
    canOpenPods(row) {
      return ['Deployment', 'StatefulSet', 'DaemonSet'].includes(row.__kind)
    },
    async openResourceDetail(row) {
      if (!this.canOpenPods(row)) {
        this.$message.info('仅支持 Deployment/StatefulSet/DaemonSet 详情视图')
        return
      }
      const resourceName = (row.metadata && row.metadata.name) || ''
      const namespace = row.__namespace || (row.metadata && row.metadata.namespace) || ''
      this.workloadDrawer.visible = true
      this.workloadDrawer.title = `${row.__kind} · ${resourceName}`
      this.workloadDrawer.loading = true
      this.workloadDrawer.activeTab = 'overview'
      this.workloadDrawer.details = emptyWorkloadDetails()
      this.workloadDrawer.context = {
        kind: row.__kind ? row.__kind.toLowerCase() : '',
        namespace,
        name: resourceName
      }
      this.workloadDrawer.history = []
      this.workloadDrawer.historyLoading = false
      this.workloadDrawer.events = defaultWorkloadEventsState()
      this.workloadDrawer.logs = defaultWorkloadLogsState()
      try {
        await this.loadWorkloadDetails()
      } catch (err) {
        this.$message.error(err.message || '加载详情失败')
        this.workloadDrawer.visible = false
      } finally {
        this.workloadDrawer.loading = false
      }
    },
    async loadWorkloadDetails() {
      if (!this.workloadDrawer.context || !this.selectedClusterId) return
      const details = await getWorkloadDetails(this.selectedClusterId, this.workloadDrawer.context)
      const normalized = this.normalizeWorkloadDetails(details)
      this.workloadDrawer.details = normalized
      const selectorText = this.buildSelectorQuery(normalized.overview.selector)
      this.workloadDrawer.logs.labelSelector = selectorText
      this.workloadDrawer.logs.availableContainers = normalized.overview.containers
        .filter(item => !item.init)
        .map(item => item.name)
      this.workloadDrawer.logs.selectedContainers = []
      this.workloadDrawer.logs.content = ''
      this.workloadDrawer.logs.lastUpdated = 0
      this.workloadDrawer.logs.error = ''
    },
    async fetchWorkloadHistory() {
      if (!this.workloadDrawer.context || !this.selectedClusterId) return
      this.workloadDrawer.historyLoading = true
      try {
        const history = await getWorkloadHistory(this.selectedClusterId, this.workloadDrawer.context)
        this.workloadDrawer.history = Array.isArray(history) ? history : []
      } catch (err) {
        this.$message.error(err.message || '加载历史版本失败')
      } finally {
        this.workloadDrawer.historyLoading = false
      }
    },
    async fetchWorkloadEvents() {
      if (!this.workloadDrawer.context || !this.selectedClusterId) return
      this.workloadDrawer.events.loading = true
      try {
        const payload = await listEvents(this.selectedClusterId, {
          namespace: this.workloadDrawer.context.namespace,
          kind: this.workloadDrawer.details.workload ? this.workloadDrawer.details.workload.kind : '',
          name: this.workloadDrawer.context.name,
          page: this.workloadDrawer.events.page,
          perPage: this.workloadDrawer.events.pageSize
        })
        this.workloadDrawer.events.items = (payload && payload.items) || []
        this.workloadDrawer.events.total = (payload && payload.total) || 0
        this.workloadDrawer.events.loaded = true
      } catch (err) {
        this.$message.error(err.message || '加载事件失败')
      } finally {
        this.workloadDrawer.events.loading = false
      }
    },
    async fetchWorkloadLogs() {
      if (!this.workloadDrawer.context || !this.selectedClusterId) return
      const params = {
        ...this.workloadDrawer.context,
        labelSelector: this.workloadDrawer.logs.labelSelector,
        containers: this.workloadDrawer.logs.selectedContainers,
        allContainers: this.workloadDrawer.logs.allContainers,
        tail: this.workloadDrawer.logs.tail
      }
      this.workloadDrawer.logs.loading = true
      this.workloadDrawer.logs.error = ''
      try {
        const resp = await getWorkloadLogs(this.selectedClusterId, params)
        this.workloadDrawer.logs.content = (resp && resp.content) || ''
        this.workloadDrawer.logs.lastUpdated = Date.now()
      } catch (err) {
        this.workloadDrawer.logs.error = err.message || '获取日志失败'
      } finally {
        this.workloadDrawer.logs.loading = false
      }
    },
    async handleWorkloadRollback(entry) {
      if (!entry || !entry.revision) return
      try {
        await this.$confirm(`确定回滚到 revision ${entry.revision} 吗？`, '提示', { type: 'warning' })
      } catch (err) {
        return
      }
      if (!this.workloadDrawer.context || !this.selectedClusterId) return
      try {
        await rollbackWorkload(this.selectedClusterId, {
          ...this.workloadDrawer.context,
          revision: entry.revision
        })
        this.$message.success('已触发回滚')
        await this.loadWorkloadDetails()
        await this.fetchWorkloadHistory()
      } catch (err) {
        this.$message.error(err.message || '回滚失败')
      }
    },
    handleWorkloadTabClick(tab) {
      const name = tab.name || tab
      if (name === 'history' && !this.workloadDrawer.history.length && !this.workloadDrawer.historyLoading) {
        this.fetchWorkloadHistory()
      }
      if (name === 'events' && !this.workloadDrawer.events.loaded && !this.workloadDrawer.events.loading) {
        this.fetchWorkloadEvents()
      }
      if (name === 'logs' && !this.workloadDrawer.logs.content && !this.workloadDrawer.logs.loading) {
        this.fetchWorkloadLogs()
      }
    },
    buildSelectorQuery(selector = {}) {
      if (!selector || typeof selector !== 'object') {
        return ''
      }
      const pairs = Object.keys(selector)
        .filter(key => selector[key] !== undefined && selector[key] !== null && selector[key] !== '')
        .map(key => `${key}=${selector[key]}`)
      return pairs.join(',')
    },
    createNamedResourceRow(resource) {
      if (!resource) return null
      return {
        metadata: {
          name: resource.name,
          namespace: resource.namespace
        },
        __kind: resource.kind || resource.type || '',
        __namespace: resource.namespace,
        __gvr: {
          group: resource.group || '',
          version: resource.version || 'v1',
          resource: resource.resource,
          namespaced: resource.namespaced !== false
        }
      }
    },
    handleNamedResourceCommand(command, resource) {
      const row = this.createNamedResourceRow(resource)
      if (!row) return
      this.handleRowCommand(command, row)
    },
    normalizeWorkloadDetails(details) {
      const base = emptyWorkloadDetails()
      if (!details || typeof details !== 'object') {
        return base
      }
      const overview = Object.assign({}, base.overview, details.overview || {})
      overview.selectorText = this.buildSelectorQuery(overview.selector)
      overview.labelsText = this.formatLabels(overview.labels)
      overview.annotationsList = this.mapKeyValuePairs(overview.annotations)
      overview.createdAtText = overview.creation_timestamp ? this.formatTime(overview.creation_timestamp * 1000) : '-'
      overview.updatedAtText = overview.update_timestamp ? this.formatTime(overview.update_timestamp * 1000) : '-'
      const pods = Array.isArray(details.pods)
        ? details.pods.map(pod => ({
          ...pod,
          age: this.formatPodAge(pod.created_at),
          containersText:
            Array.isArray(pod.containers) && pod.containers.length ? pod.containers.join(', ') : '-'
        }))
        : []
      const decorateList = list => {
        if (!Array.isArray(list)) return []
        return list.map(item => this.decorateNamedResource(item))
      }
      const volumes = Array.isArray(details.volumes)
        ? details.volumes.map(volume => ({
          ...volume,
          source_display: volume.source_name || '-'
        }))
        : []
      return {
        ...base,
        workload: details.workload || null,
        overview,
        pods,
        services: decorateList(details.services),
        endpoints: decorateList(details.endpoints),
        ingresses: decorateList(details.ingresses),
        configmaps: decorateList(details.configmaps),
        secrets: decorateList(details.secrets),
        volumes,
        pvcs: decorateList(details.pvcs)
      }
    },
    decorateNamedResource(item) {
      if (!item) return null
      return {
        ...item,
        labelsText: this.formatLabels(item.labels),
        __gvr: {
          group: item.group || '',
          version: item.version || 'v1',
          resource: item.resource,
          namespaced: item.namespaced !== false
        }
      }
    },
    formatLabels(labels) {
      if (!labels || typeof labels !== 'object') {
        return '-'
      }
      const parts = Object.keys(labels)
        .filter(key => labels[key] !== undefined && labels[key] !== null)
        .map(key => `${key}=${labels[key]}`)
      return parts.length ? parts.join(', ') : '-'
    },
    mapKeyValuePairs(source) {
      if (!source || typeof source !== 'object') {
        return []
      }
      return Object.keys(source).map(key => ({ key, value: source[key] }))
    },
    formatDuration(timestamp) {
      if (!timestamp) return '-'
      const diff = Date.now() - new Date(timestamp).getTime()
      if (diff <= 0) return '0s'
      const minutes = Math.floor(diff / 60000)
      if (minutes < 60) return `${minutes}m`
      const hours = Math.floor(minutes / 60)
      if (hours < 24) return `${hours}h`
      const days = Math.floor(hours / 24)
      return `${days}d`
    },
    formatTime(value) {
      if (!value) return ''
      if (value instanceof Date) {
        return value.toLocaleString()
      }
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
.k8s-workspace {
  padding: 0;
  background: #f6f8fb;
  min-height: 100%;
}

.workspace-empty {
  margin: 32px;
  padding: 40px 0;
}

.panel {
  background: #fff;
  border-radius: 20px;
  box-shadow: 0 20px 40px rgba(15, 23, 42, 0.08);
}

.k8s-shell {
  display: flex;
  min-height: calc(100vh - 48px);
}

.k8s-shell__nav {
  width: 240px;
  flex: 0 0 240px;
  background: #fff;
  padding: 24px 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  border-right: 1px solid #e5e7eb;
}

.nav-link {
  background: none;
  border: none;
  text-align: left;
  padding: 6px 0;
  color: #2563eb;
  font-weight: 500;
  cursor: pointer;
}

.nav-divider {
  height: 1px;
  background: #f1f5f9;
}

.nav-cluster-card {
  background: #f8fafc;
  border-radius: 12px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.nav-cluster-card__name {
  margin: 0;
  font-weight: 600;
  color: #111827;
}

.nav-resource-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border-radius: 10px;
  cursor: pointer;
  color: #374151;
  transition: background 0.2s ease, color 0.2s ease;
}

.nav-item.active,
.nav-item:hover {
  background: #e0edff;
  color: #1d4ed8;
}

.k8s-shell__content {
  flex: 1;
  padding: 32px;
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.resource-panel {
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  flex: 1;
  min-height: 0;
}

.resource-panel__toolbar {
  display: flex;
  gap: 12px;
  align-items: center;
}

.resource-panel__table {
  flex: 1;
  min-height: 0;
}

.pod-subtable {
  padding: 16px;
  background: #f8fafc;
  border-radius: 12px;
}

.pod-subtable__loading,
.pod-subtable__empty {
  padding: 12px 0;
}

.pod-subtable__table {
  width: 100%;
}

.resource-panel__footer {
  padding-top: 16px;
  border-top: 1px solid #eef2ff;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}

.resource-panel__footer :deep(.el-pagination) {
  flex-shrink: 0;
}

.resource-panel__page-meta {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  color: #6b7280;
  font-size: 13px;
}

.page-size-select {
  width: 90px;
}

.exec-shell-overlay {
  display: flex;
  flex-direction: column;
  position: fixed;
  z-index: 2100;
  inset: 0;
  background: rgba(0, 0, 0, 0.98);
  padding: 24px;
  pointer-events: auto;
}

.exec-shell {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: #000;
  border-radius: 16px;
  box-shadow: 0 25px 60px rgba(0, 0, 0, 0.5);
  color: #e2e8f0;
  overflow: hidden;
}

.exec-shell__header {
  padding: 16px 24px;
  background: #040404;
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  border-bottom: 1px solid #111;
}

.exec-shell__breadcrumb {
  font-size: 13px;
  color: #94a3b8;
  display: flex;
  align-items: center;
  gap: 6px;
}

.exec-shell__link {
  cursor: pointer;
  color: #60a5fa;
}

.exec-shell__divider {
  color: #475569;
}

.exec-shell__controls {
  display: flex;
  gap: 12px;
  align-items: center;
}

.exec-shell__main {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 16px 24px 24px;
  gap: 8px;
  background: #000;
}

.exec-shell__terminal {
  flex: 1;
  background: #000;
  padding: 12px;
  overflow: hidden;
  width: 100%;
  height: 100%;
}

.exec-shell__terminal :deep(.xterm) {
  height: 100%;
  padding: 0;
  background: #000;
  color: #e5e5e5;
}

.exec-shell__terminal :deep(.xterm-viewport) {
  overflow-y: auto;
  background: #000;
}

.exec-shell__terminal :deep(.xterm-screen) {
  cursor: text;
  background: #000;
}

.exec-shell__terminal :deep(.xterm-helper-textarea) {
  position: absolute;
  opacity: 0;
  left: -9999px;
}

.exec-shell__terminal-wrapper {
  flex: 1;
  background: #000;
  border-radius: 12px;
  overflow: hidden;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.05);
  cursor: text;
  min-height: 0;
}

.exec-shell__status {
  font-size: 12px;
  color: #94a3b8;
  display: flex;
  align-items: center;
  gap: 8px;
}

.exec-shell__status-indicator {
  font-size: 8px;
  line-height: 1;
}

.exec-shell__status-indicator--connected {
  color: #34d399;
  animation: pulse-connected 2s ease-in-out infinite;
}

.exec-shell__status-indicator--connecting {
  color: #fbbf24;
  animation: pulse-connecting 1s ease-in-out infinite;
}

@keyframes pulse-connected {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.5;
  }
}

@keyframes pulse-connecting {
  0%, 100% {
    opacity: 0.3;
  }
  50% {
    opacity: 1;
  }
}

.logs-dialog__meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  gap: 16px;
}

.logs-dialog__controls {
  display: flex;
  gap: 12px;
  align-items: center;
}

.logs-dialog__pod {
  margin: 0;
  color: #1f2937;
  font-size: 13px;
}

.logs-dialog__switch {
  margin-left: 8px;
}

.workload-logs-viewer {
  background: #0f172a;
  color: #e2e8f0;
  border-radius: 12px;
  padding: 12px;
  height: 360px;
  overflow: auto;
  font-family: 'JetBrains Mono', Consolas, Menlo, monospace;
  font-size: 13px;
}

.logs-viewer {
  background: #0f172a;
  color: #e2e8f0;
  border-radius: 12px;
  padding: 12px;
  height: 360px;
  overflow: auto;
  font-family: 'JetBrains Mono', Consolas, Menlo, monospace;
  font-size: 13px;
}

.logs-viewer pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
}

.status-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #1f2937;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
  background: #9ca3af;
}

.status-dot.success {
  background: #34d399;
}

.status-dot.warning {
  background: #fbbf24;
}

.status-dot.danger {
  background: #f87171;
}

.status-dot.info {
  background: #38bdf8;
}

.dropdown-link {
  color: #2563eb;
  cursor: pointer;
}

.resource-image-tag {
  margin-left: 8px;
  padding: 2px 6px;
  font-size: 11px;
  border-radius: 8px;
  background: #e0f2fe;
  color: #0369a1;
}

.yaml-viewer {
  max-height: 520px;
  overflow: auto;
}

.k8s-admin__yaml {
  background: #0f172a;
  color: #e2e8f0;
  padding: 16px;
  border-radius: 12px;
  max-height: 520px;
  overflow: auto;
  max-width: 100%;
}

.k8s-admin__yaml-code {
  font-family: 'JetBrains Mono', Consolas, Menlo, monospace;
  font-size: 13px;
  line-height: 1.5;
  display: block;
  white-space: pre-wrap;
  word-break: break-all;
  overflow-wrap: anywhere;
  color: #f8fafc;
}

.yaml-textarea >>> textarea {
  font-family: 'JetBrains Mono', Consolas, Menlo, monospace;
}

.yaml-editor :deep(.CodeMirror) {
  height: 440px;
  border-radius: 12px;
  font-family: 'JetBrains Mono', Consolas, Menlo, monospace;
  background: #0f172a;
  color: #e2e8f0;
}

.yaml-editor :deep(.CodeMirror-scroll) {
  max-height: 440px;
}

.workload-drawer >>> .el-drawer__body {
  padding: 0;
}

.workload-drawer__body {
  height: calc(100vh - 120px);
  overflow-y: auto;
  padding: 0 24px 24px;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 16px;
}

.overview-card {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 16px;
  padding: 16px 20px;
  box-shadow: 0 20px 40px rgba(15, 23, 42, 0.04);
}

.overview-card__header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-bottom: 12px;
}

.overview-card__header h3 {
  margin: 0;
  font-size: 16px;
  color: #0f172a;
}

.overview-card__sub {
  font-size: 12px;
  color: #94a3b8;
}

.overview-list {
  margin: 0;
  padding: 0;
}

.overview-list > div {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  padding: 6px 0;
  border-bottom: 1px dashed #e2e8f0;
}

.overview-list dt {
  font-weight: 600;
  color: #475569;
}

.overview-list dd {
  margin: 0;
  color: #0f172a;
}

.chip-group {
  margin-top: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.chip {
  padding: 4px 10px;
  background: #edf2ff;
  color: #334155;
  border-radius: 999px;
  font-size: 12px;
}

.workload-section {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 16px;
  padding: 16px 20px;
  box-shadow: 0 20px 40px rgba(15, 23, 42, 0.04);
  margin-bottom: 20px;
}

.workload-section__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.workload-section__header h3 {
  margin: 0;
  font-size: 16px;
}

.workload-section__count {
  font-size: 12px;
  color: #94a3b8;
}

.resource-split {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
  margin-top: 12px;
}

.resource-split__pane {
  flex: 1;
  min-width: 220px;
}

.resource-split__pane h4 {
  margin: 0 0 8px;
  font-size: 14px;
  color: #0f172a;
}

.logs-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 12px;
}

.logs-toolbar__item {
  min-width: 200px;
}

.logs-alert {
  margin-bottom: 12px;
}

.workload-logs-viewer {
  background: #0f172a;
  color: #e2e8f0;
  border-radius: 12px;
  padding: 12px;
  height: 360px;
  overflow: auto;
  font-family: 'JetBrains Mono', Consolas, Menlo, monospace;
  font-size: 13px;
  border: 1px solid #1e293b;
}

.workload-logs-viewer--loading {
  opacity: 0.7;
}

.workload-logs-meta {
  margin-top: 8px;
  font-size: 12px;
  color: #94a3b8;
}
</style>
