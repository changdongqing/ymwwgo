import type { AppRouteObject } from '@/core/router/types';
import { createLazyRoute } from '@/core/router';

export const ontologyRoutes: AppRouteObject[] = [
  {
    name: 'ontology',
    path: 'ontology',
    meta: {
      title: 'routes:ontology',
      icon: 'lucide:network',
      order: 2200,
      keepAlive: true,
      authority: ['sys:platform_admin'],
    },
    children: [
      {
        name: 'ontology-docs',
        path: 'ontology-docs',
        element: createLazyRoute(() => import('@/pages/app/ontology/ontology')),
        meta: {
          title: 'routes:ontology-docs',
          icon: 'lucide:file-json-2',
          order: 1,
          authority: ['sys:platform_admin'],
        },
      },
      {
        name: 'ontology-classes',
        path: 'ontology-classes',
        element: createLazyRoute(() => import('@/pages/app/ontology/classes')),
        meta: {
          title: 'routes:ontology-classes',
          icon: 'lucide:git-branch',
          order: 2,
          authority: ['sys:platform_admin'],
        },
      },
    ],
  },
];

export default ontologyRoutes;
