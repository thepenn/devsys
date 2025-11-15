import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { message } from 'antd';
import { useNavigate } from 'react-router-dom';
import defaultAvatar from 'assets/avatar/avatar.gif';
import { getCurrentUser } from 'api/system/auth';
import { listRepositories, syncRepositories, syncRepository } from 'api/project/repos';
import { triggerPipelineRun } from 'api/project/pipeline';
import { clearToken } from 'utils/auth';
import { normalizeError } from 'utils/error';
import { emptyVariableRow, normalizeVariableRows, serializeVariableRows } from 'utils/pipelineRun';
import './dashboard.less';

const PROVIDER_LABELS = {
  gitlab: 'GitLab',
  gitea: 'Gitea',
  gitee: 'Gitee'
};

const DashboardPage = () => {
  const navigate = useNavigate();
  const accountRef = useRef(null);
  const [user, setUser] = useState(null);
  const [repos, setRepos] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(12);
  const [search, setSearch] = useState('');
  const [viewSynced, setViewSynced] = useState(true);
  const [loading, setLoading] = useState(true);
  const [syncing, setSyncing] = useState(false);
  const [repoSyncing, setRepoSyncing] = useState({});
  const [error, setError] = useState('');
  const [menuOpen, setMenuOpen] = useState(false);
  const [runModalVisible, setRunModalVisible] = useState(false);
  const [runTargetRepo, setRunTargetRepo] = useState(null);
  const [runForm, setRunForm] = useState({
    branch: 'main',
    commit: '',
    variables: normalizeVariableRows()
  });
  const [runFormError, setRunFormError] = useState('');
  const [running, setRunning] = useState(false);

  const totalPages = useMemo(() => {
    if (perPage <= 0) return 1;
    return Math.max(1, Math.ceil(total / perPage));
  }, [perPage, total]);

  const providerLabel = useMemo(() => {
    if (!user?.provider) return '';
    const key = String(user.provider).toLowerCase();
    return PROVIDER_LABELS[key] || key.toUpperCase();
  }, [user]);

  const redirectToLogin = useCallback(
    messageText => {
      clearToken();
      const query = messageText ? `?error=${encodeURIComponent(messageText)}` : '';
      navigate(`/login${query}`, { replace: true });
    },
    [navigate]
  );

  const handleAuthError = useCallback(
    err => {
      if (!err) return false;
      const status = typeof err.status === 'number' ? err.status : err.response?.status;
      if (status === 401 || (err.message && /401|未授权|unauthorized/i.test(err.message))) {
        redirectToLogin(err.message || '请先登录');
        return true;
      }
      return false;
    },
    [redirectToLogin]
  );

  const loadRepos = useCallback(
    async (targetPage = 1, overrides = {}) => {
      const nextSearch = overrides.search !== undefined ? overrides.search : search;
      const nextViewSynced = overrides.viewSynced !== undefined ? overrides.viewSynced : viewSynced;
      const params = {
        page: targetPage,
        per_page: overrides.perPage || perPage,
        synced: nextViewSynced ? 'true' : 'false'
      };
      if (nextSearch && nextSearch.trim()) {
        params.search = nextSearch.trim();
      }
      setError('');
      try {
        const data = await listRepositories(params);
        setRepos(Array.isArray(data?.items) ? data.items : []);
        setTotal(data?.total || 0);
        setPage(data?.page || targetPage);
        if (data?.per_page) {
          setPerPage(data.per_page);
        }
      } catch (err) {
        const normalized = normalizeError(err, '加载仓库失败');
        if (!handleAuthError(normalized)) {
          setError(normalized.message);
        }
        throw normalized;
      }
    },
    [handleAuthError, perPage, search, viewSynced]
  );

  useEffect(() => {
    let cancelled = false;
    const bootstrap = async () => {
      setLoading(true);
      try {
        const userInfo = await getCurrentUser();
        if (cancelled) return;
        setUser(userInfo);
        const data = await listRepositories({ page: 1, per_page: 12, synced: 'true' });
        if (cancelled) return;
        setRepos(Array.isArray(data?.items) ? data.items : []);
        setTotal(data?.total || 0);
        setPage(data?.page || 1);
        setPerPage(data?.per_page || 12);
      } catch (err) {
        const normalized = normalizeError(err, '加载账户信息失败');
        if (!cancelled && !handleAuthError(normalized)) {
          setError(normalized.message);
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };
    bootstrap();
    return () => {
      cancelled = true;
    };
  }, [handleAuthError]);

  useEffect(() => {
    const handleClick = event => {
      if (!accountRef.current) return;
      if (accountRef.current.contains(event.target)) {
        return;
      }
      setMenuOpen(false);
    };
    document.addEventListener('click', handleClick);
    return () => document.removeEventListener('click', handleClick);
  }, []);

  const applySearch = useCallback(() => {
    loadRepos(1, { search });
  }, [loadRepos, search]);

  const changePage = newPage => {
    if (newPage === page || newPage < 1 || newPage > totalPages) {
      return;
    }
    loadRepos(newPage).catch(() => {});
  };

  const changeViewSynced = value => {
    if (viewSynced === value) return;
    setViewSynced(value);
    setPage(1);
    loadRepos(1, { viewSynced: value }).catch(() => {});
  };

  const syncAllRepos = async () => {
    if (!user?.admin) return;
    setSyncing(true);
    try {
      await syncRepositories();
      message.success('已触发仓库同步');
      await loadRepos(page);
    } catch (err) {
      const normalized = normalizeError(err, '同步仓库失败');
      if (!handleAuthError(normalized)) {
        message.error(normalized.message);
      }
    } finally {
      setSyncing(false);
    }
  };

  const syncOneRepo = async repo => {
    if (!user?.admin || !repo?.forge_remote_id || repo.active) {
      return;
    }
    setRepoSyncing(prev => ({ ...prev, [repo.forge_remote_id]: true }));
    try {
      await syncRepository(repo.forge_remote_id);
      message.success('该仓库同步任务已触发');
      await loadRepos(page);
    } catch (err) {
      const normalized = normalizeError(err, '同步仓库失败');
      if (!handleAuthError(normalized)) {
        message.error(normalized.message);
      }
    } finally {
      setRepoSyncing(prev => {
        const next = { ...prev };
        delete next[repo.forge_remote_id];
        return next;
      });
    }
  };

  const projectPath = repo => {
    if (!repo?.full_name) return '/dev/dashboard';
    const [owner, name] = repo.full_name.split('/');
    if (!owner || !name) {
      return '/dev/dashboard';
    }
    return `/dev/projects/${owner}/${name}/pipeline`;
  };

  const openRunModal = repo => {
    if (!repo) return;
    const defaultBranch =
      (repo.branch && repo.branch.trim()) ||
      (repo.default_branch && repo.default_branch.trim()) ||
      'main';
    setRunTargetRepo(repo);
    setRunForm({
      branch: defaultBranch,
      commit: '',
      variables: normalizeVariableRows()
    });
    setRunFormError('');
    setRunModalVisible(true);
  };

  const closeRunModal = () => {
    if (running) return;
    setRunModalVisible(false);
    setRunTargetRepo(null);
    setRunForm({
      branch: 'main',
      commit: '',
      variables: normalizeVariableRows()
    });
    setRunFormError('');
  };

  const updateVariable = (index, field, value) => {
    setRunForm(prev => {
      const variables = prev.variables.map((row, idx) =>
        idx === index ? { ...row, [field]: value } : row
      );
      return { ...prev, variables };
    });
  };

  const addVariableRow = () => {
    setRunForm(prev => ({
      ...prev,
      variables: [...prev.variables, emptyVariableRow()]
    }));
  };

  const removeVariableRow = idx => {
    setRunForm(prev => {
      if (prev.variables.length <= 1) {
        return { ...prev, variables: [emptyVariableRow()] };
      }
      return {
        ...prev,
        variables: prev.variables.filter((_, index) => index !== idx)
      };
    });
  };

  const submitRun = async () => {
    const branch = (runForm.branch || '').trim();
    if (!branch) {
      setRunFormError('构建分支为必填项');
      return;
    }
    if (!runTargetRepo?.id) {
      setRunFormError('无法识别仓库，请稍后重试');
      return;
    }
    setRunFormError('');
    setRunning(true);
    try {
      const payload = { branch };
      if (runForm.commit && runForm.commit.trim()) {
        payload.commit = runForm.commit.trim();
      }
      const variablesPayload = serializeVariableRows(runForm.variables);
      if (variablesPayload) {
        payload.variables = variablesPayload;
      }
      const result = await triggerPipelineRun(runTargetRepo.id, payload);
      closeRunModal();
      const path = projectPath(runTargetRepo);
      if (result?.id) {
        navigate(`${path}?highlight=${encodeURIComponent(result.id)}`);
      } else {
        navigate(path);
      }
    } catch (err) {
      const normalized = normalizeError(err, '触发流水线失败');
      if (!handleAuthError(normalized)) {
        setRunFormError(normalized.message);
      }
    } finally {
      setRunning(false);
    }
  };

  const logout = () => {
    setMenuOpen(false);
    redirectToLogin();
  };

  const viewProfile = () => {
    setMenuOpen(false);
    navigate('/dev/profile');
  };

  const openAdmin = () => {
    setMenuOpen(false);
    navigate('/ops');
  };

  const formatVisibility = value => {
    switch (value) {
      case 'public':
        return '公开';
      case 'internal':
        return '内部';
      case 'private':
        return '私有';
      default:
        return value || '未知';
    }
  };

  return (
    <div className="dashboard-page">
      {user && (
        <div className="dashboard-account" ref={accountRef}>
          <div className="dashboard-account__trigger" onClick={() => setMenuOpen(open => !open)}>
            <img src={user.avatar_url || defaultAvatar} alt="avatar" className="dashboard-account__avatar" />
            <div className="dashboard-account__meta">
              <span className="dashboard-account__name">{user.login}</span>
              <span className="dashboard-account__email">{user.email || '未公开邮箱'}</span>
            </div>
            <span className="dashboard-account__caret" />
          </div>
          {menuOpen && (
            <ul className="dashboard-account__menu">
              <li className="dashboard-account__menu-item" onClick={viewProfile}>
                个人信息
              </li>
              {user.admin && (
                <li className="dashboard-account__menu-item" onClick={openAdmin}>
                  管理后台
                </li>
              )}
              <li className="dashboard-account__menu-item dashboard-account__menu-item--danger" onClick={logout}>
                退出登录
              </li>
            </ul>
          )}
        </div>
      )}

      <main className="dashboard-main">
        {error && (
          <section className="alert alert--error dashboard-alert">
            <span>{error}</span>
            <button type="button" className="button button--ghost" onClick={() => setError('')}>
              关闭
            </button>
          </section>
        )}

        {loading ? (
          <section className="panel">
            <p>正在加载...</p>
          </section>
        ) : (
          <>
            {user && (
              <section className="panel panel--highlight dashboard-profile">
                <div className="dashboard-user">
                  {user.avatar_url && <img src={user.avatar_url} alt="avatar" className="dashboard-user__avatar" />}
                  <div className="dashboard-user__info">
                    <h2>{user.login}</h2>
                    <p className="dashboard-user__email">{user.email || '未公开邮箱'}</p>
                    {providerLabel && <span className="dashboard-tag">{providerLabel}</span>}
                  </div>
                </div>
              </section>
            )}

            <section className="panel">
              <div className="repo-controls">
                <div className="repo-controls__left">
                  <div className="repo-search">
                    <input
                      value={search}
                      className="input input--compact"
                      placeholder="搜索仓库"
                      onChange={e => setSearch(e.target.value)}
                      onKeyDown={e => {
                        if (e.key === 'Enter') {
                          applySearch();
                        }
                      }}
                    />
                    <button className="button button--ghost" onClick={applySearch}>
                      搜索
                    </button>
                  </div>
                  <div className="repo-filter">
                    <button
                      className={`repo-filter__btn ${viewSynced ? 'repo-filter__btn--active' : ''}`}
                      onClick={() => changeViewSynced(true)}
                    >
                      已同步
                    </button>
                    <button
                      className={`repo-filter__btn ${!viewSynced ? 'repo-filter__btn--active' : ''}`}
                      onClick={() => changeViewSynced(false)}
                    >
                      未同步
                    </button>
                  </div>
                </div>
                {user?.admin && (
                  <button className="button button--ghost repo-sync" disabled={syncing} onClick={syncAllRepos}>
                    {syncing ? '同步中…' : '同步仓库'}
                  </button>
                )}
              </div>

              {repos.length ? (
                <div className="repo-table">
                  <div className="repo-table__header">
                    <span className="repo-table__cell repo-table__cell--name">仓库</span>
                    <span className="repo-table__cell repo-table__cell--visibility">可见性</span>
                    <span className="repo-table__cell repo-table__cell--build">构建</span>
                    <span className="repo-table__cell repo-table__cell--link">仓库地址</span>
                    <span className="repo-table__cell repo-table__cell--actions">操作</span>
                  </div>
                  {repos.map(repo => (
                    <div key={repo.id || repo.full_name} className="repo-table__row">
                      <div className="repo-table__cell repo-table__cell--name">
                        <button type="button" className="repo-name-link" onClick={() => navigate(projectPath(repo))}>
                          {repo.full_name}
                        </button>
                        {repo.description && <div className="repo-description">{repo.description}</div>}
                      </div>
                      <div className="repo-table__cell repo-table__cell--visibility">
                        <span className="repo-visibility">{formatVisibility(repo.visibility)}</span>
                      </div>
                      <div className="repo-table__cell repo-table__cell--build">
                        <button className="button repo-table__build" onClick={() => openRunModal(repo)}>
                          构建
                        </button>
                      </div>
                      <div className="repo-table__cell repo-table__cell--link">
                        {repo.forge_url ? (
                          <a
                            href={repo.forge_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="button button--ghost repo-link-button"
                          >
                            查看仓库
                          </a>
                        ) : (
                          <span className="repo-link repo-link--empty">暂无地址</span>
                        )}
                      </div>
                      <div className="repo-table__cell repo-table__cell--actions">
                        {user?.admin ? (
                          <button
                            className="button button--ghost repo-table__action"
                            disabled={repo.active || !!repoSyncing[repo.forge_remote_id]}
                            onClick={() => syncOneRepo(repo)}
                          >
                            {repo.active ? '已同步' : repoSyncing[repo.forge_remote_id] ? '同步中…' : '同步此仓库'}
                          </button>
                        ) : (
                          <span className="repo-table__label">无权限</span>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="empty">未找到仓库，尝试同步或调整搜索条件。</p>
              )}

              {totalPages > 1 && (
                <div className="pagination">
                  <button className="button button--ghost" disabled={page === 1} onClick={() => changePage(page - 1)}>
                    上一页
                  </button>
                  <span className="pagination__info">
                    第 {page} / {totalPages} 页（共 {total} 个仓库）
                  </span>
                  <button
                    className="button button--ghost"
                    disabled={page === totalPages}
                    onClick={() => changePage(page + 1)}
                  >
                    下一页
                  </button>
                </div>
              )}
            </section>
          </>
        )}
      </main>

      {runModalVisible && (
        <div className="dashboard-modal" onClick={closeRunModal}>
          <div className="dashboard-modal__content" onClick={event => event.stopPropagation()}>
            <header className="dashboard-modal__header">
              <h3>运行流水线</h3>
              <button className="dashboard-modal__close" onClick={closeRunModal}>
                ×
              </button>
            </header>
            <section className="dashboard-modal__body">
              <label className="modal-field">
                <span>构建分支 *</span>
                <input
                  value={runForm.branch}
                  onChange={e => setRunForm(prev => ({ ...prev, branch: e.target.value }))}
                  placeholder="例如 main"
                />
              </label>
              <label className="modal-field">
                <span>Commit ID (可选)</span>
                <input
                  value={runForm.commit}
                  onChange={e => setRunForm(prev => ({ ...prev, commit: e.target.value }))}
                  placeholder="传入具体 commit 时优先使用"
                />
              </label>
              <div className="modal-field">
                <span>运行变量（可选）</span>
                <p className="modal-hint">这些键值将同步到流水线，可用于自定义参数。</p>
                {runForm.variables.map((variable, idx) => (
                  <div key={`dashboard-run-var-${idx}`} className="run-variable-row">
                    <input
                      value={variable.key}
                      onChange={e => updateVariable(idx, 'key', e.target.value)}
                      placeholder="变量名，如 TARGET_ENV"
                    />
                    <input
                      value={variable.value}
                      onChange={e => updateVariable(idx, 'value', e.target.value)}
                      placeholder="变量值"
                    />
                    <button
                      type="button"
                      className="button button--ghost run-variable-remove"
                      onClick={() => removeVariableRow(idx)}
                    >
                      删除
                    </button>
                  </div>
                ))}
                <button type="button" className="button button--ghost run-variable-add" onClick={addVariableRow}>
                  + 添加变量
                </button>
              </div>
              {runFormError && <p className="modal-error">{runFormError}</p>}
            </section>
            <footer className="dashboard-modal__footer">
              <button className="button button--ghost" disabled={running} onClick={closeRunModal}>
                取消
              </button>
              <button className="button" disabled={running} onClick={submitRun}>
                {running ? '提交中…' : '运行'}
              </button>
            </footer>
          </div>
        </div>
      )}
    </div>
  );
};

export default DashboardPage;
