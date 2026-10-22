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
      {
        // 数据类型（05 期，系列首个无左树全宽列表页——OWL 2 无 subDatatypeOf 层级）
        name: 'datatypes',
        path: 'datatypes',
        element: createLazyRoute(() => import('@/pages/app/ontology/datatypes')),
        meta: {
          title: 'routes:datatypes',
          icon: 'lucide:filter',
          order: 6,
          authority: ['sys:platform_admin'],
        },
      },
      {
        // 个体（06 期，左右联动：左类树 + 右实例列表，身份与赋码/取号器）
        name: 'individuals',
        path: 'individuals',
        element: createLazyRoute(() => import('@/pages/app/ontology/individuals')),
        meta: {
          title: 'routes:individuals',
          icon: 'lucide:user',
          order: 7,
          authority: ['sys:platform_admin'],
        },
      },
    ],
  },
];

export default ontologyRoutes;
