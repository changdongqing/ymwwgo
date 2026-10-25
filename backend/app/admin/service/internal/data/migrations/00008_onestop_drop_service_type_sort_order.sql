-- 一站式服务类型：移除独立排序字段 sort_order。
--
-- 背景：服务类型列表固定按 code 升序（2/4/6/8 位编码本身就是层级业务序），
-- 仓库内不再保留第二套排序来源。Ent schema 已同步去掉 mixin.SortOrder。
--
-- +goose Up
ALTER TABLE public.onestop_service_types
    DROP COLUMN IF EXISTS sort_order;

-- +goose Down
ALTER TABLE public.onestop_service_types
    ADD COLUMN IF NOT EXISTS sort_order bigint DEFAULT 0;

COMMENT ON COLUMN public.onestop_service_types.sort_order IS '排序值（已废弃，仅为回滚保留）';
