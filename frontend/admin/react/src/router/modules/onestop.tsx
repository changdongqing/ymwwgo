import type { AppRouteObject } from '@/core/router/types';
import { createLazyRoute } from '@/core/router';

export const oneStopRoutes: AppRouteObject[] = [
  {
    name: 'onestop',
    path: 'onestop',
    meta: {
      title: 'routes:onestop',
      icon: 'lucide:concierge-bell',
      order: 2100,
      keepAlive: true,
      authority: ['sys:platform_admin', 'sys:tenant_manager'],
    },
    children: [
      {
        name: 'onestop-service-types',
        path: 'service-types',
        element: createLazyRoute(() => import('@/pages/app/onestop/service-type')),
        meta: {
          title: 'routes:onestop-service-types',
          icon: 'lucide:layout-list',
          order: 1,
          authority: ['sys:platform_admin'],
        },
      },
      {
        // 工单管理：统一工单底座（事件/计划任务/运送三形态），坐席工作台。
        // 工单是租户业务数据，租户管理员亦可进入（菜单表种子同口径，见
        // default_data.go 的 onestop 菜单分组）
        name: 'onestop-work-orders',
        path: 'work-orders',
        element: createLazyRoute(() => import('@/pages/app/onestop/work-order')),
        meta: {
          title: 'routes:onestop-work-orders',
          icon: 'lucide:clipboard-list',
          order: 2,
          authority: ['sys:platform_admin', 'sys:tenant_manager'],
        },
      },
    ],
  },
];

export default oneStopRoutes;
