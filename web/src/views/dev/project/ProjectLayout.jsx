import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, NavLink, Outlet, useLocation, useParams } from 'react-router-dom';
import { Card, Alert, Spin } from 'antd';
import { getCurrentUser } from 'api/system/auth';
import { listRepositories } from 'api/project/repos';
import { ProjectContext } from './ProjectContext';
import './ProjectLayout.less';

const visibilityLabel = visibility => {
  if (!visibility) return '未知';
  const normalized = String(visibility).toLowerCase();
  if (normalized === 'private') return '私有';
  if (normalized === 'internal') return '内部';
  if (normalized === 'public') return '公开';
  return visibility;
};

const navItems = [
  { key: 'pipeline', label: '流水线构建', to: ({ owner, name }) => `/dev/projects/${owner}/${name}/pipeline` },
  { key: 'deployment', label: '部署发布', to: ({ owner, name }) => `/dev/projects/${owner}/${name}/deployment` },
  { key: 'monitor', label: '监控告警', to: ({ owner, name }) => `/dev/projects/${owner}/${name}/monitor` }
];

const ProjectLayout = () => {
  const { owner, name } = useParams();
  const location = useLocation();
  const [user, setUser] = useState(null);
  const [repo, setRepo] = useState(null);
  const [loadingRepo, setLoadingRepo] = useState(true);
  const [repoError, setRepoError] = useState('');
  const repoPromiseRef = useRef(null);

  const fetchRepo = useCallback(async () => {
    if (!owner || !name) {
      setRepo(null);
      setRepoError('缺少项目信息');
      setLoadingRepo(false);
      return null;
    }
    setLoadingRepo(true);
    setRepoError('');
    try {
      const search = `${owner}/${name}`;
      const data = await listRepositories({ search, per_page: 1, page: 1 });
      const found = data?.items?.[0] || null;
      if (!found) {
        setRepo(null);
        setRepoError('未找到对应项目，可能尚未同步');
        return null;
      }
      setRepo(found);
      return found;
    } catch (err) {
      const message = err?.message || '加载项目失败';
      setRepoError(message);
      setRepo(null);
      return null;
    } finally {
      setLoadingRepo(false);
    }
  }, [owner, name]);

  const ensureRepo = useCallback(async () => {
    if (repo?.id) return repo;
    if (repoPromiseRef.current) {
      return repoPromiseRef.current;
    }
    const promise = (async () => {
      const result = await fetchRepo();
      repoPromiseRef.current = null;
      return result;
    })();
    repoPromiseRef.current = promise;
    return promise;
  }, [repo, fetchRepo]);

  useEffect(() => {
    fetchRepo();
  }, [fetchRepo]);

  useEffect(() => {
    let mounted = true;
    (async () => {
      try {
        const info = await getCurrentUser();
        if (mounted) {
          setUser(info || null);
        }
      } catch (err) {
        if (mounted) {
          setUser(null);
        }
      }
    })();
    return () => {
      mounted = false;
    };
  }, []);

  useEffect(() => {
    setRepo(null);
    setRepoError('');
    fetchRepo();
  }, [owner, name, fetchRepo]);

  const contextValue = useMemo(() => ({
    owner,
    name,
    repo,
    isAdmin: Boolean(user?.admin),
    reloadRepo: fetchRepo,
    ensureRepo
  }), [owner, name, repo, user, fetchRepo, ensureRepo]);

  const currentPath = location.pathname;

  return (
    <ProjectContext.Provider value={contextValue}>
      <div className="project-wrapper">
        <aside className="project-nav">
          <div className="project-nav__header">
            <Link to="/dev/dashboard" className="project-nav__back">
              ← 返回仓库
            </Link>
            <h2>{repo?.name || `${owner}/${name}`}</h2>
            <p className="project-nav__subtitle">
              {repo ? visibilityLabel(repo.visibility) : loadingRepo ? '加载中…' : '未同步'}
            </p>
          </div>
          <nav className="project-nav__links">
            {navItems.map(item => (
              <NavLink
                key={item.key}
                to={item.to({ owner, name })}
                className={({ isActive }) =>
                  `project-nav__link${isActive ? ' project-nav__link--active' : ''}`
                }
              >
                {item.label}
              </NavLink>
            ))}
          </nav>
        </aside>

        <main className="project-main">
          {repoError && (
            <Alert type="error" message={repoError} showIcon className="project-error" />
          )}

          <Card className="project-header" loading={loadingRepo}>
            <div className="project-header__info">
              <h1>{repo?.full_name || `${owner}/${name}`}</h1>
              <p className="project-header__meta">
                <span>Forge ID：{repo?.forge_remote_id || '未知'}</span>
                <span>默认分支：{repo?.branch || '未设置'}</span>
                <span>可见性：{visibilityLabel(repo?.visibility)}</span>
              </p>
            </div>
            <div className="project-header__badges">
              {repo ? (
                <span className={`project-tag project-tag--${repo.visibility || 'unknown'}`}>
                  {visibilityLabel(repo.visibility)}
                </span>
              ) : null}
              {repo && (
                <span className={`project-tag ${repo.active ? 'project-tag--success' : 'project-tag--warning'}`}>
                  {repo.active ? '已同步' : '未同步'}
                </span>
              )}
            </div>
          </Card>

          {loadingRepo && !repo ? (
            <div className="project-loading">
              <Spin tip="正在加载项目信息" />
            </div>
          ) : (
            <div className="project-content">
              <Outlet context={{ currentPath }} />
            </div>
          )}
        </main>
      </div>
    </ProjectContext.Provider>
  );
};

export default ProjectLayout;
