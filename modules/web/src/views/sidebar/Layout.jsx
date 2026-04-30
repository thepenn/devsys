import React, { useEffect, useMemo, useRef, useState } from 'react';
import { NavLink, Outlet, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import clsx from 'clsx';
import { opsNavItems } from './Navigation';
import './Layout.less';
import { useAuth } from '../../context/AuthContext';
import { clearToken } from '../../utils/auth';
import defaultAvatar from '../../assets/avatar/avatar.gif';
import { listClusters } from '../../api/admin/k8s';

const OpsLayout = () => {
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { user, hasAnyLabel } = useAuth();
  const avatarSrc = (user && user.avatar_url) || defaultAvatar;
  const displayName = user?.login || user?.name || '管理员';
  const storedCluster = useMemo(() => {
    if (typeof window === 'undefined') {
      return '';
    }
    return window.localStorage.getItem('k8s.activeCluster') || '';
  }, []);
  const activeCluster = searchParams.get('cluster') || storedCluster || '';
  const resolvedSection = useMemo(() => {
    let path = location.pathname;
    if (path.startsWith('/ops/projects/build')) {
      path = '/ops/projects/pipeline';
    }
    if (path.startsWith('/ops/pipeline-templates/')) {
      // 详情页 /:id 与列表页归属同一 nav 节, 高亮列表入口.
      path = '/ops/pipeline-templates';
    }
    if (path.startsWith('/ops/pipeline-jobs/')) {
      // 同上, Job 编辑器 / 运行详情都视作独立 Job 子菜单.
      path = '/ops/pipeline-jobs';
    }
    const matched = opsNavItems.find(section =>
      section.children.some(item => path.startsWith(item.path))
    );
    return matched?.key || opsNavItems[0]?.key || null;
  }, [location.pathname]);
  const [expandedKey, setExpandedKey] = useState(resolvedSection);
  const [hasK8sClusters, setHasK8sClusters] = useState(null);
  const lastPathRef = useRef(location.pathname);
  const profileRef = useRef(null);
  const [profileOpen, setProfileOpen] = useState(false);
  const displayNavItems = useMemo(
    () =>
      opsNavItems
        .map(section => {
          // 1) 先按 RBAC label 过滤每个 child (无 labels 字段视为任何登录用户可见)
          let children = section.children.filter(item =>
            !item.labels || item.labels.length === 0 || hasAnyLabel(item.labels)
          );
          // 2) K8s 分区在没有任何集群时仅保留集群列表入口, 引导用户先添加集群
          if (section.key === 'k8s' && hasK8sClusters === false) {
            children = children.filter(item => item.key === 'clusters');
          }
          return { ...section, children };
        })
        // 3) 子项被全部过滤掉的 section 整段隐藏
        .filter(section => section.children.length > 0),
    [hasK8sClusters, hasAnyLabel]
  );

  useEffect(() => {
    let cancelled = false;
    const loadClusters = async () => {
      try {
        const list = await listClusters();
        if (!cancelled) {
          setHasK8sClusters(Array.isArray(list) ? list.length > 0 : false);
        }
      } catch (err) {
        if (!cancelled) {
          setHasK8sClusters(null);
        }
      }
    };
    loadClusters();
    const onClustersUpdated = () => {
      loadClusters();
    };
    if (typeof window !== 'undefined') {
      window.addEventListener('k8s-clusters-updated', onClustersUpdated);
    }
    return () => {
      cancelled = true;
      if (typeof window !== 'undefined') {
        window.removeEventListener('k8s-clusters-updated', onClustersUpdated);
      }
    };
  }, []);

  useEffect(() => {
    if (lastPathRef.current === location.pathname) {
      return;
    }
    lastPathRef.current = location.pathname;
    if (resolvedSection && resolvedSection !== expandedKey) {
      setExpandedKey(resolvedSection);
    }
  }, [location.pathname, resolvedSection, expandedKey]);

  const isActive = path => {
    if (path === '/ops/projects/pipeline') {
      return location.pathname.startsWith(path) || location.pathname.startsWith('/ops/projects/build');
    }
    if (path === '/ops/pipeline-templates' || path === '/ops/pipeline-jobs') {
      return location.pathname === path || location.pathname.startsWith(`${path}/`);
    }
    return location.pathname.startsWith(path);
  };
  const toggleSection = key => {
    setExpandedKey(prev => (prev === key ? null : key));
  };

  const buildNavPath = (sectionKey, item) => {
    if (sectionKey === 'k8s' && item.key !== 'clusters' && activeCluster) {
      const hasQuery = item.path.includes('?');
      const separator = hasQuery ? '&' : '?';
      return `${item.path}${separator}cluster=${activeCluster}`;
    }
    return item.path;
  };

  const handleLogout = () => {
    setProfileOpen(false);
    clearToken();
    window.location.href = '/#/login';
  };

  const goProfile = () => {
    setProfileOpen(false);
    navigate('/ops/profile');
  };

  useEffect(() => {
    if (!profileOpen) return undefined;
    const handler = event => {
      if (profileRef.current && !profileRef.current.contains(event.target)) {
        setProfileOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [profileOpen]);

  return (
    <div className="ops-layout">
      <aside className="ops-sidebar">
        <div className="ops-sidebar__header">
          <h1>运维控制台</h1>
          <p>DevOps Platform</p>
        </div>
        <div className="ops-sidebar__menu">
          {displayNavItems.map(section => (
            <div key={section.key} className="ops-sidebar__section">
              <button
                type="button"
                className={clsx('ops-sidebar__section-header', {
                  'ops-sidebar__section-header--active': expandedKey === section.key
                })}
                onClick={() => toggleSection(section.key)}
              >
                <span className="ops-sidebar__section-title">{section.label}</span>
                <span
                  className={clsx('ops-sidebar__caret', {
                    'ops-sidebar__caret--open': expandedKey === section.key
                  })}
                />
              </button>
              {expandedKey === section.key && (
                <ul>
                  {section.children.map(item => (
                    <li
                      key={item.key}
                      className={clsx('ops-sidebar__item', {
                        'ops-sidebar__item--active': isActive(item.path)
                      })}
                    >
                      <NavLink to={buildNavPath(section.key, item)}>{item.label}</NavLink>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          ))}
        </div>
      </aside>
      <main className="ops-main">
        <header className="ops-main__header">
          <div />
          <div className="ops-user" ref={profileRef} onClick={() => setProfileOpen(open => !open)}>
            <img src={avatarSrc} alt="avatar" />
            <div className="ops-user__meta">
              <strong>{displayName}</strong>
              <span>{user?.email || user?.login || 'Admin'}</span>
            </div>
            <span className={clsx('ops-user__caret', { 'ops-user__caret--open': profileOpen })} />
            {profileOpen && (
              <div className="ops-user__menu" onClick={e => e.stopPropagation()}>
                <button type="button" onClick={goProfile}>个人信息</button>
                <button type="button" className="danger" onClick={handleLogout}>退出登录</button>
              </div>
            )}
          </div>
        </header>
        <div className="ops-main__body">
          <Outlet />
        </div>
      </main>
    </div>
  );
};

export default OpsLayout;
