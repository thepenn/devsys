import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Button, Card, Drawer, Form, Input, Space, Spin, message } from 'antd';
import { listClusters } from '../../api/admin/k8s';
import { createCertificate } from '../../api/system/certificates';

const STORAGE_KEY = 'k8s.activeCluster';

const K8sClusterGuard = ({ children }) => {
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const [checking, setChecking] = useState(true);
  const [clusters, setClusters] = useState([]);
  const [drawerVisible, setDrawerVisible] = useState(false);
  const [drawerLoading, setDrawerLoading] = useState(false);
  const [clusterForm] = Form.useForm();
  const warnedMissingRef = useRef(false);
  const clusterId = params.get('cluster');
  const storedCluster = typeof window !== 'undefined' ? window.localStorage.getItem(STORAGE_KEY) : '';

  const fetchClusters = useCallback(async () => {
    setChecking(true);
    try {
      const resp = await listClusters();
      setClusters(Array.isArray(resp) ? resp : []);
    } catch (err) {
      setClusters([]);
      message.error(err?.message || '加载集群列表失败');
    } finally {
      setChecking(false);
    }
  }, []);

  useEffect(() => {
    fetchClusters();
  }, [fetchClusters]);

  const clusterIds = useMemo(
    () => new Set((clusters || []).map(item => String(item.id))),
    [clusters]
  );
  const hasClusters = clusters.length > 0;

  useEffect(() => {
    if (checking) return;

    const persistCluster = nextCluster => {
      if (typeof window !== 'undefined') {
        window.localStorage.setItem(STORAGE_KEY, nextCluster);
      }
    };

    if (!hasClusters) {
      if (typeof window !== 'undefined') {
        window.localStorage.removeItem(STORAGE_KEY);
      }
      if (clusterId) {
        const next = new URLSearchParams(params);
        next.delete('cluster');
        setParams(next, { replace: true });
      }
      return;
    }

    const candidate = (clusterId || storedCluster || '').trim();
    if (candidate && clusterIds.has(candidate)) {
      persistCluster(candidate);
      if (!clusterId) {
        const next = new URLSearchParams(params);
        next.set('cluster', candidate);
        setParams(next, { replace: true });
      }
      return;
    }

    if (candidate && !clusterIds.has(candidate) && !warnedMissingRef.current) {
      warnedMissingRef.current = true;
      message.warning('当前集群不存在，请重新选择集群');
    }

    const fallback = String(clusters[0].id);
    persistCluster(fallback);
    if (clusterId !== fallback) {
      const next = new URLSearchParams(params);
      next.set('cluster', fallback);
      setParams(next, { replace: true });
    }
  }, [checking, hasClusters, clusterId, storedCluster, clusterIds, clusters, params, setParams]);

  const closeDrawer = () => {
    setDrawerVisible(false);
    clusterForm.resetFields();
  };

  const openDrawer = () => {
    clusterForm.setFieldsValue({ name: '', kubeconfig: '' });
    setDrawerVisible(true);
  };

  const handleSubmitCluster = async () => {
    try {
      const values = await clusterForm.validateFields();
      setDrawerLoading(true);
      const created = await createCertificate({
        name: values.name.trim(),
        type: 'kubernetes',
        config: {
          kubeconfig: values.kubeconfig
        }
      });
      message.success('集群已创建');
      const createdId = created?.id ? String(created.id) : '';
      await fetchClusters();
      closeDrawer();
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new Event('k8s-clusters-updated'));
      }
      if (createdId) {
        const next = new URLSearchParams(params);
        next.set('cluster', createdId);
        setParams(next, { replace: true });
        if (typeof window !== 'undefined') {
          window.localStorage.setItem(STORAGE_KEY, createdId);
        }
      }
    } catch (err) {
      if (!err?.errorFields) {
        message.error(err?.message || '保存集群失败');
      }
    } finally {
      setDrawerLoading(false);
    }
  };

  if (checking) {
    return (
      <Card className="cluster-guard">
        <Spin tip="加载集群中..." />
      </Card>
    );
  }

  if (!hasClusters) {
    return (
      <>
        <Card className="cluster-guard">
          <p>当前未配置 K8s 集群，请先添加集群。</p>
          <Space>
            <Button onClick={() => navigate('/ops/k8s/clusters')}>前往集群列表</Button>
            <Button type="primary" onClick={openDrawer}>添加集群</Button>
          </Space>
        </Card>
        <Drawer
          title="添加集群"
          open={drawerVisible}
          width={520}
          onClose={closeDrawer}
          extra={
            <Space>
              <Button onClick={closeDrawer}>取消</Button>
              <Button type="primary" loading={drawerLoading} onClick={handleSubmitCluster}>
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
      </>
    );
  }

  const resolvedCluster = (clusterId || storedCluster || '').trim();
  const validCluster = resolvedCluster && clusterIds.has(resolvedCluster);
  if (!validCluster) {
    return (
      <Card className="cluster-guard">
        <Spin tip="正在准备集群..." />
      </Card>
    );
  }

  return typeof children === 'function' ? children(resolvedCluster) : children;
};

export default K8sClusterGuard;
