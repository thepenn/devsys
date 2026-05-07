import React from 'react';
import { Card } from 'antd';
import clsx from 'clsx';
import './OpsPageCard.less';

export function opsPageTitle(section: string, page: string) {
  return `${section} · ${page}`;
}

export type OpsPageCardProps = {
  title: React.ReactNode;
  extra?: React.ReactNode;
  /** tableFlush: 表格式主列表（head 紧凑、body 无内边距）；standard：走 Layout 默认 Card 内边距 */
  bodyVariant?: 'tableFlush' | 'standard';
  className?: string;
  bodyStyle?: React.CSSProperties;
  loading?: boolean;
  bordered?: boolean;
  children?: React.ReactNode;
};

export function OpsPageCard({
  title,
  extra,
  bodyVariant = 'standard',
  className,
  bodyStyle,
  loading,
  bordered,
  children
}: OpsPageCardProps) {
  return (
    <Card
      className={clsx(
        'ops-page-card',
        bodyVariant === 'tableFlush' && 'ops-page-card--table-flush',
        className
      )}
      title={title}
      extra={extra}
      bodyStyle={bodyStyle}
      loading={loading}
      bordered={bordered}
    >
      {children}
    </Card>
  );
}

export default OpsPageCard;
