import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Card, Drawer, Empty, Form, Input, Modal, Space, Table, message } from 'antd';
import { useNavigate } from 'react-router-dom';
import dayjs from 'dayjs';
import { listClusters } from '../../../api/admin/k8s';
import { createCertificate, deleteCertificate, getCertificate, updateCertificate } from '../../../api/system/certificates';
import './cluster-list.less';

const ClusterList = () => {
  const [clusters, setClusters] = useState([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [searchInput, setSearchInput] = useState('');
  const [clusterDrawerVisible, setClusterDrawerVisible] = useState(false);
  const [clusterDrawerLoading, setClusterDrawerLoading] = useState(false);
  const [editingCluster, setEditingCluster] = useState(null);
  const [clusterForm] = Form.useForm();
  const navigate = useNavigate();

  const fetchClusters = useCallback(async () => {
    setLoading(true);
    try {
      const resp = await listClusters();
      setClusters(Array.isArray(resp) ? resp : []);
    } catch (err) {
      message.error(err.message || '加载集群失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchClusters();
  }, [fetchClusters]);

  const filtered = useMemo(() => {
    if (!search.trim()) return clusters;
    const keyword = search.trim().toLowerCase();
    return clusters.filter(item =>
      item.name.toLowerCase().includes(keyword) ||
      (item.server || '').toLowerCase().includes(keyword)
    );
  }, [clusters, search]);

  const openClusterDrawer = async record => {
    setEditingCluster(record || null);
    if (!record) {
      clusterForm.setFieldsValue({ name: '', kubeconfig: '' });
      setClusterDrawerVisible(true);
      return;
    }
    try {
      const detail = await getCertificate(record.id, { reveal: true });
      clusterForm.setFieldsValue({
        name: detail?.name || record.name,
        kubeconfig: (detail?.config && detail.config.kubeconfig) || ''
      });
    } catch (err) {
      clusterForm.setFieldsValue({ name: record.name, kubeconfig: '' });
      message.warning('加载集群凭证失败，请重新粘贴 kubeconfig');
    }
    setClusterDrawerVisible(true);
  };

  const closeClusterDrawer = () => {
    setClusterDrawerVisible(false);
    setEditingCluster(null);
    clusterForm.resetFields();
  };

  const handleClusterSubmit = async () => {
    try {
      const values = await clusterForm.validateFields();
      const payload = {
        name: values.name.trim(),
        type: 'kubernetes',
        config: { kubeconfig: values.kubeconfig }
      };
      setClusterDrawerLoading(true);
      if (editingCluster) {
        await updateCertificate(editingCluster.id, payload);
        message.success('集群已更新');
      } else {
        await createCertificate(payload);
        message.success('集群已创建');
      }
      closeClusterDrawer();
      fetchClusters();
    } catch (err) {
      if (err?.errorFields) return;
      message.error(err.message || '保存集群失败');
    } finally {
      setClusterDrawerLoading(false);
    }
  };

  const handleClusterDelete = record => {
    Modal.confirm({
      title: `删除集群 ${record.name}`,
      content: '该操作将移除绑定的 kubeconfig，确认继续？',
      okText: '删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        try {
          await deleteCertificate(record.id);
          message.success('集群已删除');
          fetchClusters();
        } catch (err) {
          message.error(err.message || '删除失败');
        }
      }
    });
  };

  const columns = [
    {
      title: '集群名称',
      dataIndex: 'name',
      render: text => <strong>{text}</strong>
    },
    {
      title: 'API Server',
      dataIndex: 'server',
      render: value => (value ? <span className="cluster-list__server">{value}</span> : '—')
    },
    {
      title: '最近更新',
      dataIndex: 'updated',
      render: value => (value ? dayjs(value * 1000).format('YYYY-MM-DD HH:mm') : '—')
    },
    {
      title: '操作',
      width: 320,
      render: (_, record) => (
        <Space>
          <Button type="primary" onClick={() => navigate(`/ops/k8s/workloads?cluster=${record.id}`)}>
            进入工作台
          </Button>
          <Button onClick={() => openClusterDrawer(record)}>编辑配置</Button>
          <Button danger onClick={() => handleClusterDelete(record)}>
            删除
          </Button>
        </Space>
      )
    }
  ];

  return (
    <div className="cluster-list">
      <Card
        title="K8s 集群列表"
        extra={
          <Space>
            <Input.Search
              placeholder="搜索名称 / API Server"
              value={searchInput}
              onChange={e => setSearchInput(e.target.value)}
              onSearch={value => {
                setSearch(value);
                setSearchInput(value);
              }}
              allowClear
              style={{ width: 260 }}
            />
            <Button type="primary" onClick={() => openClusterDrawer(null)}>
              添加集群
            </Button>
          </Space>
        }
      >
        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={filtered}
          locale={{ emptyText: <Empty description="暂无集群" /> }}
          pagination={false}
        />
      </Card>

      <Drawer
        title={editingCluster ? '编辑集群' : '添加集群'}
        open={clusterDrawerVisible}
        width={520}
        onClose={closeClusterDrawer}
        extra={
          <Space>
            <Button onClick={closeClusterDrawer}>取消</Button>
            <Button type="primary" loading={clusterDrawerLoading} onClick={handleClusterSubmit}>
              保存
            </Button>
          </Space>
        }
      >
        <Form layout="vertical" form={clusterForm} initialValues={{ name: '', kubeconfig: '' }}>
          <Form.Item
            label="集群名称"
            name="name"
            rules={[{ required: true, message: '请输入集群名称' }]}
          >
            <Input placeholder="例如：prod-cluster" />
          </Form.Item>
          <Form.Item
            label="Kubeconfig"
            name="kubeconfig"
            rules={[{ required: true, message: '请粘贴 kubeconfig 内容' }]}
          >
            <Input.TextArea rows={14} placeholder="粘贴 kubeconfig YAML 内容" />
          </Form.Item>
        </Form>
      </Drawer>
    </div>
  );
};

export default ClusterList;
