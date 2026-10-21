import { Splitter } from 'antd';
import { useState } from 'react';

import ContentContainer from '@/layouts/components/PageContainer/ContentContainer';
import OrgList from './OrgList';
import UserList from './UserList';

/**
 * 用户管理页面
 * 使用 Splitter 实现左右分栏布局：左侧组织树（定宽 256px，可拖拽在 200–400 内调整），
 * 右侧自动占满剩余宽度
 */
const UserManagement = () => {
  const [currentTenantId, setCurrentTenantId] = useState<number | undefined>(undefined);
  const [currentOrgUnitId, setCurrentOrgUnitId] = useState<number | undefined>(undefined);

  return (
    <ContentContainer heightMode="fixed" padding="16px" bottomMargin={0}>
      <Splitter style={{ height: '100%', flex: 1, minHeight: 0 }}>
        <Splitter.Panel collapsible defaultSize={256} min={200} max={400} style={{ display: 'flex', flexDirection: 'column' }}>
          <OrgList
            currentOrgUnitId={currentOrgUnitId}
            currentTenantId={currentTenantId}
            onTenantSelect={setCurrentTenantId}
            onOrgSelect={setCurrentOrgUnitId}
          />
        </Splitter.Panel>
        <Splitter.Panel style={{ display: 'flex', flexDirection: 'column' }}>
          <UserList tenantId={currentTenantId} orgUnitId={currentOrgUnitId} />
        </Splitter.Panel>
      </Splitter>
    </ContentContainer>
  );
};

export default UserManagement;
