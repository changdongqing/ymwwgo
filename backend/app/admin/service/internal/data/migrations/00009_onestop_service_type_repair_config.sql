-- 一站式服务类型：报修维修类型配置详情（本期）。
--
-- 8 个可空配置列：仅 0101 报修维修子树的小类（岗位/时限/指导）与细类
-- （工时/语句/工具/备件）填写，未配置 = NULL，其余类型不启用。
-- jsonb 数组列语义与 sys_org_units.subtype_path 一致。
-- 列 COMMENT 与 ent schema 的 Comment 逐字一致（漂移守卫比对注释文本）。

-- +goose Up
ALTER TABLE public.onestop_service_types
    ADD COLUMN IF NOT EXISTS repair_post character varying,
    ADD COLUMN IF NOT EXISTS standard_work_hours double precision,
    ADD COLUMN IF NOT EXISTS response_time_limit bigint,
    ADD COLUMN IF NOT EXISTS completion_time_limit bigint,
    ADD COLUMN IF NOT EXISTS guidance character varying,
    ADD COLUMN IF NOT EXISTS common_phrases jsonb,
    ADD COLUMN IF NOT EXISTS standard_tools jsonb,
    ADD COLUMN IF NOT EXISTS spare_parts jsonb;

COMMENT ON COLUMN public.onestop_service_types.repair_post IS '维修岗位（派单目标岗位，如：电工/管工/空调工）';
COMMENT ON COLUMN public.onestop_service_types.standard_work_hours IS '标准工时（小时）：单次作业标准人时，绩效与负载核算用';
COMMENT ON COLUMN public.onestop_service_types.response_time_limit IS '响应时限（分钟）：受理到到场的 SLA';
COMMENT ON COLUMN public.onestop_service_types.completion_time_limit IS '完成时限（分钟）：受理到完工的 SLA';
COMMENT ON COLUMN public.onestop_service_types.guidance IS '标准指导说明：作业 SOP 与安全注意事项';
COMMENT ON COLUMN public.onestop_service_types.common_phrases IS '常见报修语句清单：报修端快捷语句，兼作语义归型语料';
COMMENT ON COLUMN public.onestop_service_types.standard_tools IS '服务标准工具清单：出工标配工具';
COMMENT ON COLUMN public.onestop_service_types.spare_parts IS '常用备件/耗材清单：出工常备，配合库存领用';

-- +goose Down
ALTER TABLE public.onestop_service_types
    DROP COLUMN IF EXISTS repair_post,
    DROP COLUMN IF EXISTS standard_work_hours,
    DROP COLUMN IF EXISTS response_time_limit,
    DROP COLUMN IF EXISTS completion_time_limit,
    DROP COLUMN IF EXISTS guidance,
    DROP COLUMN IF EXISTS common_phrases,
    DROP COLUMN IF EXISTS standard_tools,
    DROP COLUMN IF EXISTS spare_parts;
