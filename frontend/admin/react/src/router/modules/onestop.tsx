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
    ],
  },
];

export default oneStopRoutes;
