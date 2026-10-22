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
      {
        name: 'ontology-object-properties',
        path: 'ontology-object-properties',
        element: createLazyRoute(() => import('@/pages/app/ontology/object-properties')),
        meta: {
          title: 'routes:ontology-object-properties',
          icon: 'lucide:arrow-right-left',
          order: 3,
          authority: ['sys:platform_admin'],
        },
      },
      {
        name: 'ontology-data-properties',
        path: 'ontology-data-properties',
        element: createLazyRoute(() => import('@/pages/app/ontology/data-properties')),
        meta: {
          title: 'routes:ontology-data-properties',
          icon: 'lucide:tags',
          order: 4,
          authority: ['sys:platform_admin'],
        },
      },
      {
        name: 'annotation-properties',
        path: 'annotation-properties',
        element: createLazyRoute(() => import('@/pages/app/ontology/annotation-properties')),
        meta: {
          title: 'routes:annotation-properties',
          icon: 'lucide:tag',
          order: 5,
          authority: ['sys:platform_admin'],
        },
      },
    ],
  },
];

export default ontologyRoutes;
