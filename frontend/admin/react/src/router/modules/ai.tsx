import type { AppRouteObject } from '@/core/router/types';
import { createLazyRoute } from '@/core/router';

/**
 * AI 路由配置：对话助手 + 模型提供商管理
 */
export const aiRoutes: AppRouteObject[] = [
  {
    name: 'ai',
    path: 'ai',
    meta: {
      title: 'routes:ai',
      icon: 'lucide:sparkles',
      order: 2010,
      keepAlive: true,
    },
    children: [
      {
        name: 'ai-chat',
        path: 'chat',
        element: createLazyRoute(() => import('@/pages/app/ai/chat')),
        meta: {
          title: 'routes:ai',
          icon: 'lucide:message-circle',
          order: 1,
        },
      },
      {
        name: 'ai-agents',
        path: 'agents',
        element: createLazyRoute(() => import('@/pages/app/ai/agent')),
        meta: {
          title: 'routes:ai-agents',
          icon: 'lucide:workflow',
          order: 6,
        },
      },
      {
        name: 'ai-knowledge',
        path: 'knowledge',
        element: createLazyRoute(() => import('@/pages/app/ai/knowledge')),
        meta: {
          title: 'routes:ai-knowledge',
          icon: 'lucide:book-open',
          order: 3,
        },
      },
      {
        name: 'ai-query',
        path: 'query',
        element: createLazyRoute(() => import('@/pages/app/ai/query')),
        meta: {
          title: 'routes:ai-query',
          icon: 'lucide:search-code',
          order: 2,
        },
      },
      {
        name: 'ai-usage',
        path: 'usage',
        element: createLazyRoute(() => import('@/pages/app/ai/usage')),
        meta: {
          title: 'routes:ai-usage',
          icon: 'lucide:bar-chart-3',
          order: 4,
        },
      },
      {
        name: 'ai-providers',
        path: 'providers',
        element: createLazyRoute(() => import('@/pages/app/ai/provider')),
        meta: {
          title: 'routes:ai-providers',
          icon: 'lucide:cpu',
          order: 2,
        },
      },
    ],
  },
];
export default aiRoutes;
