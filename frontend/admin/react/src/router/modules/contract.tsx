import type { AppRouteObject } from '@/core/router/types';
import { createLazyRoute } from '@/core/router';

// 合同管理（独立 Go module go-wind-admin/contract）：类型字典 + 统一合同底座。
// 类型字典是平台级配置（平台管理员专属）；合同是租户业务数据（双角色可用，
// 菜单表种子同口径，见 default_data.go 的 contract 菜单分组）。
export const contractRoutes: AppRouteObject[] = [
  {
    name: 'contract',
    path: 'contract',
    meta: {
      title: 'routes:contract',
      icon: 'lucide:file-signature',
      order: 2200,
      keepAlive: true,
      authority: ['sys:platform_admin', 'sys:tenant_manager'],
    },
    children: [
      {
        name: 'contract-types',
        path: 'types',
        element: createLazyRoute(() => import('@/pages/app/contract/type')),
        meta: {
          title: 'routes:contract-types',
          icon: 'lucide:layout-list',
          order: 1,
          authority: ['sys:platform_admin'],
        },
      },
      {
        name: 'contract-list',
        path: 'list',
        element: createLazyRoute(() => import('@/pages/app/contract/list')),
        meta: {
          title: 'routes:contract-list',
          icon: 'lucide:clipboard-list',
          order: 2,
          authority: ['sys:platform_admin', 'sys:tenant_manager'],
        },
      },
    ],
  },
];

export default contractRoutes;
