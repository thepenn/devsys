import React from 'react';
import { Card } from 'antd';
import './OpsPlaceholder.less';

const OpsPlaceholder = ({ title, description = '功能正在建设中，敬请期待。' }) => (
  <div className="ops-placeholder">
    <Card className="ops-placeholder__card" title={title} bordered={false}>
      <p>{description}</p>
    </Card>
  </div>
);

export default OpsPlaceholder;
