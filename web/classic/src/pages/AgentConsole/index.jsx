/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Empty,
  Input,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconCopy,
  IconGlobe,
  IconRefresh,
  IconSave,
} from '@douyinfe/semi-icons';
import { API, copy, showError, showSuccess } from '../../helpers';

const { Text, Title } = Typography;

const parseBranding = (branding) => {
  if (!branding || typeof branding !== 'string') {
    return {};
  }

  try {
    const parsed = JSON.parse(branding);
    if (!parsed || typeof parsed !== 'object') {
      return {};
    }

    return {
      site_name:
        typeof parsed.site_name === 'string' ? parsed.site_name : '',
      logo: typeof parsed.logo === 'string' ? parsed.logo : '',
    };
  } catch (error) {
    return {};
  }
};

const stringifyBranding = ({ siteName, logo }) => {
  const normalizedSiteName = siteName.trim();
  const normalizedLogo = logo.trim();
  if (!normalizedSiteName && !normalizedLogo) {
    return '';
  }

  return JSON.stringify({
    site_name: normalizedSiteName,
    logo: normalizedLogo,
  });
};

const formatQuota = (quota) => (quota ?? 0).toLocaleString();

const getDomainStatus = (status) => {
  if (status === 1) {
    return { color: 'green', text: '已启用' };
  }
  if (status === 2) {
    return { color: 'red', text: '已禁用' };
  }
  return { color: 'orange', text: '待验证' };
};

const InfoItem = ({ label, value }) => (
  <div className='rounded-lg border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-3'>
    <Text type='tertiary' size='small'>
      {label}
    </Text>
    <div className='mt-1 break-all text-sm font-medium text-[var(--semi-color-text-0)]'>
      {value || '-'}
    </div>
  </div>
);

const AgentConsole = () => {
  const [loading, setLoading] = useState(true);
  const [savingBranding, setSavingBranding] = useState(false);
  const [creatingDomain, setCreatingDomain] = useState(false);
  const [self, setSelf] = useState(null);
  const [domains, setDomains] = useState([]);
  const [siteName, setSiteName] = useState('');
  const [logo, setLogo] = useState('');
  const [newDomain, setNewDomain] = useState('');

  const loadAgent = async () => {
    setLoading(true);
    try {
      const [selfRes, domainsRes] = await Promise.all([
        API.get('/api/agent/self'),
        API.get('/api/agent/domains', {
          params: { p: 1, page_size: 50 },
        }),
      ]);

      if (!selfRes.data?.success) {
        throw new Error(selfRes.data?.message || '加载代理信息失败');
      }

      if (!domainsRes.data?.success) {
        throw new Error(domainsRes.data?.message || '加载代理域名失败');
      }

      const nextSelf = selfRes.data.data;
      const branding = parseBranding(nextSelf?.agent?.branding);
      setSelf(nextSelf);
      setSiteName(branding.site_name || '');
      setLogo(branding.logo || '');
      setDomains(domainsRes.data.data?.items || []);
    } catch (error) {
      showError(error);
    } finally {
      setLoading(false);
    }
  };

  const saveBranding = async () => {
    setSavingBranding(true);
    try {
      const res = await API.put('/api/agent/self/branding', {
        branding: stringifyBranding({ siteName, logo }),
      });

      if (!res.data?.success) {
        throw new Error(res.data?.message || '保存品牌配置失败');
      }

      showSuccess('品牌配置已保存');
      await loadAgent();
    } catch (error) {
      showError(error);
    } finally {
      setSavingBranding(false);
    }
  };

  const createDomain = async () => {
    const domain = newDomain.trim();
    if (!domain) {
      showError('请输入域名');
      return;
    }

    setCreatingDomain(true);
    try {
      const res = await API.post('/api/agent/domains', { domain });

      if (!res.data?.success) {
        throw new Error(res.data?.message || '新增域名失败');
      }

      showSuccess('域名已提交');
      setNewDomain('');
      await loadAgent();
    } catch (error) {
      showError(error);
    } finally {
      setCreatingDomain(false);
    }
  };

  const copyCNAMEtarget = async (target) => {
    if (!target) {
      return;
    }
    if (await copy(target)) {
      showSuccess('CNAME 目标已复制');
    }
  };

  useEffect(() => {
    loadAgent();
  }, []);

  const columns = useMemo(
    () => [
      {
        title: '域名',
        dataIndex: 'domain',
        render: (value) => (
          <Space spacing={8}>
            <IconGlobe />
            <Text strong copyable={false}>
              {value}
            </Text>
          </Space>
        ),
      },
      {
        title: '状态',
        dataIndex: 'status',
        width: 120,
        render: (value) => {
          const status = getDomainStatus(value);
          return <Tag color={status.color}>{status.text}</Tag>;
        },
      },
      {
        title: 'CNAME 目标',
        dataIndex: 'cname_target',
        render: (value) => (
          <Space spacing={8} align='center'>
            <Text code ellipsis={{ showTooltip: true }} style={{ maxWidth: 260 }}>
              {value || '-'}
            </Text>
            {value ? (
              <Button
                icon={<IconCopy />}
                size='small'
                theme='borderless'
                type='tertiary'
                onClick={() => copyCNAMEtarget(value)}
              />
            ) : null}
          </Space>
        ),
      },
    ],
    [],
  );

  const agent = self?.agent;
  const balance = self?.balance;
  const agentDomain = self?.context?.Domain || agent?.slug || '-';

  return (
    <div className='mt-[60px] px-2 pb-6'>
      <div className='mx-auto flex max-w-[1180px] flex-col gap-4'>
        <div className='flex flex-col gap-3 md:flex-row md:items-center md:justify-between'>
          <div>
            <Title heading={3} style={{ margin: 0 }}>
              代理后台
            </Title>
            <Text type='tertiary'>
              管理代理站点名称、Logo 和 CNAME 绑定域名。
            </Text>
          </div>
          <Button
            icon={<IconRefresh />}
            theme='outline'
            onClick={loadAgent}
            loading={loading}
          >
            刷新
          </Button>
        </div>

        <Spin spinning={loading}>
          <div className='grid gap-3 md:grid-cols-4'>
            <InfoItem label='代理名称' value={agent?.name} />
            <InfoItem label='代理标识' value={agent?.slug} />
            <InfoItem label='代理域名' value={agentDomain} />
            <InfoItem
              label='可用余额'
              value={formatQuota(balance?.available_quota)}
            />
          </div>

          <div className='mt-4 grid gap-4 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]'>
            <Card
              title='网站品牌'
              headerExtraContent={
                <Button
                  icon={<IconSave />}
                  type='primary'
                  onClick={saveBranding}
                  loading={savingBranding}
                >
                  保存
                </Button>
              }
              bodyStyle={{ display: 'flex', flexDirection: 'column', gap: 16 }}
            >
              <div>
                <Text type='secondary'>网站名称</Text>
                <Input
                  className='mt-2'
                  value={siteName}
                  placeholder='显示在代理站点上的名称'
                  onChange={setSiteName}
                />
              </div>
              <div>
                <Text type='secondary'>Logo URL</Text>
                <Input
                  className='mt-2'
                  value={logo}
                  placeholder='https://example.com/logo.png'
                  onChange={setLogo}
                />
              </div>
              <div className='rounded-lg border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-3'>
                <Text type='tertiary' size='small'>
                  预览
                </Text>
                <div className='mt-3 flex items-center gap-3'>
                  <div className='flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-1)]'>
                    {logo ? (
                      <img
                        src={logo}
                        alt='Agent logo preview'
                        className='h-full w-full object-contain'
                      />
                    ) : (
                      <IconGlobe />
                    )}
                  </div>
                  <div className='min-w-0'>
                    <div className='truncate text-base font-semibold text-[var(--semi-color-text-0)]'>
                      {siteName || agent?.name || '代理站点'}
                    </div>
                    <Text type='tertiary' ellipsis={{ showTooltip: true }}>
                      {agentDomain}
                    </Text>
                  </div>
                </div>
              </div>
            </Card>

            <Card
              title='自定义域名'
              headerExtraContent={
                <Space>
                  <Input
                    value={newDomain}
                    placeholder='agent.example.com'
                    onChange={setNewDomain}
                    onEnterPress={createDomain}
                    style={{ width: 260 }}
                  />
                  <Button
                    icon={<IconGlobe />}
                    type='primary'
                    onClick={createDomain}
                    loading={creatingDomain}
                    disabled={!newDomain.trim()}
                  >
                    新增域名
                  </Button>
                </Space>
              }
            >
              <Table
                columns={columns}
                dataSource={domains}
                rowKey='id'
                pagination={false}
                empty={
                  <Empty
                    title='暂无域名'
                    description='添加域名后，将 CNAME 指向目标地址。'
                  />
                }
              />
            </Card>
          </div>
        </Spin>
      </div>
    </div>
  );
};

export default AgentConsole;
