import React from 'react';
import { Card } from 'antd';
import { useParams } from 'react-router-dom';
import './ProjectPlaceholder.less';

const SECTION_LABELS = {
  pipeline: '流水线',
  deployment: '部署发布',
  monitor: '监控告警'
};

const ProjectPlaceholder = ({ section = 'pipeline' }) => {
  const { owner, name } = useParams();
  const label = SECTION_LABELS[section] || '模块';
  return (
    <div className="project-placeholder">
      <Card className="project-placeholder__card" title={`${label} · ${owner}/${name}`}>
        <p>React 版本的 {label} 页面仍在迁移中。</p>
        <p className="project-placeholder__tip">敬请期待，与旧版 (web.bak) 保持一致后再通知你。</p>
      </Card>
    </div>
  );
};

export default ProjectPlaceholder;
