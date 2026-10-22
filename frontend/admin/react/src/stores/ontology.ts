import { create } from 'zustand';

import { fetchListSimpleOntologies } from '@/api/hooks/ontology';
import type { ontologyservicev1_OntologyBrief } from '@/api/generated/admin/service/v1';

/**
 * 活动本体上下文（对应源 Pinia views/ontology/store/ontology.ts，"账套上下文"模式）。
 *
 * 设计文档 §3.2：不设服务端会话状态——活动本体 = 前端全局上下文，所有实体级 API
 * 显式携带 ontologyId，服务端无状态、天然支持多标签页。
 * localStorage 仅本人便利（key: ontology-current-id），不跨端同步；
 * 业务数据（本体列表）归 TanStack Query，store 只存 id 与简况。
 */
const STORAGE_KEY = 'ontology-current-id';

interface OntologyContextState {
  currentOntologyId: number | null;
  ontologyList: ontologyservicev1_OntologyBrief[];
  loaded: boolean;
  loading: boolean;

  /** 拉取本体简况列表；缓存 id 失效时回退第一项并写 localStorage */
  loadOntologyList: (force?: boolean) => Promise<void>;
  setCurrentOntologyId: (id: number | null) => void;
}

function readStoredId(): number | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const id = Number(raw);
    return Number.isFinite(id) && id > 0 ? id : null;
  } catch (error) {
    // localStorage 不可用（隐私模式等）只影响"记住上次选择"的便利性
    console.error('[ontology store] read stored ontology id failed:', error);
    return null;
  }
}

function writeStoredId(id: number | null) {
  try {
    if (id == null) {
      localStorage.removeItem(STORAGE_KEY);
    } else {
      localStorage.setItem(STORAGE_KEY, String(id));
    }
  } catch (error) {
    console.error('[ontology store] persist ontology id failed:', error);
  }
}

export const useOntologyStore = create<OntologyContextState>((set, get) => ({
  currentOntologyId: null,
  ontologyList: [],
  loaded: false,
  loading: false,

  loadOntologyList: async (force) => {
    if (get().loading) return;
    if (get().loaded && !force) return;
    set({ loading: true });
    try {
      const response = await fetchListSimpleOntologies();
      const list = response.items ?? [];
      let current = get().currentOntologyId ?? readStoredId();
      // 缓存 id 失效（被删/不存在于列表）时回退第一项
      if (current != null && !list.some((item) => item.id === current)) {
        current = null;
      }
      if (current == null && list.length > 0) {
        current = list[0].id ?? null;
      }
      writeStoredId(current);
      set({ ontologyList: list, currentOntologyId: current, loaded: true, loading: false });
    } catch (error) {
      // 铁律 1：不吞错——带出原始错误对象后回退（页面呈现"未选择本体"空态）
      console.error('[ontology store] load ontology list failed:', error);
      set({ loading: false });
    }
  },

  setCurrentOntologyId: (id) => {
    writeStoredId(id);
    set({ currentOntologyId: id });
  },
}));
