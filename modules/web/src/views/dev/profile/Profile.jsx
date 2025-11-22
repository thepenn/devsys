import React from 'react';
import { Card, Descriptions } from 'antd';
import './profile.less';

const ProfilePage = () => {
  return (
    <div className="profile-page">
      <Card title="个人资料">
        <Descriptions column={1} bordered>
          <Descriptions.Item label="姓名">暂未接入</Descriptions.Item>
          <Descriptions.Item label="邮箱">-</Descriptions.Item>
        </Descriptions>
      </Card>
    </div>
  );
};

export default ProfilePage;
