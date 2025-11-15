import React, { useEffect, useMemo, useRef, useState } from 'react';
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import clsx from 'clsx';
import { opsNavItems } from './Navigation';
import './ops-layout.less';
import { useAuth } from '../../context/AuthContext';
import { clearToken } from '../../utils/auth';
import defaultAvatar from '../../assets/avatar/avatar.gif';

const OpsLayout = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const { user } = useAuth();
  const avatarSrc = (user && user.avatar_url) || defaultAvatar;
  const displayName = user?.login || user?.name || '管理员';
  const resolvedSection = useMemo(() => {
    const matched = opsNavItems.find(section =>
      section.children.some(item => location.pathname.startsWith(item.path))
    );
    return matched?.key || opsNavItems[0]?.key || null;
  }, [location.pathname]);
  const [expandedKey, setExpandedKey] = useState(resolvedSection);
  const lastPathRef = useRef(location.pathname);
  const profileRef = useRef(null);
  const [profileOpen, setProfileOpen] = useState(false);

  useEffect(() => {
    if (lastPathRef.current === location.pathname) {
      return;
    }
    lastPathRef.current = location.pathname;
    if (resolvedSection && resolvedSection !== expandedKey) {
      setExpandedKey(resolvedSection);
    }
  }, [location.pathname, resolvedSection, expandedKey]);

  const isActive = path => location.pathname.startsWith(path);
  const toggleSection = key => {
    setExpandedKey(prev => (prev === key ? null : key));
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
          {opsNavItems.map(section => (
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
                      <NavLink to={item.path}>{item.label}</NavLink>
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
