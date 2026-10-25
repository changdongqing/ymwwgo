-- 一站式服务类型「统一配置模板」+ 巡检计划泛化为周期计划（本期）。
--
-- 1) 类型表 onestop_service_types：
--    a. 维修岗位 repair_post 与运送岗位 transport_post 合并为 execution_post
--       （两列分属 0101/0106 不同子树，同行不可能双非空，COALESCE 搬迁后删旧列）；
--    b. inspection_items 改名 standard_items（标准作业项目库，计划点表模板来源）；
--    c. 新增 trigger_mode 任务触发方式（周期计划统一开关）：
--       NULL = 未设置（细类回退小类，默认按需），存量按播种矩阵回填 SCHEDULED；
--    d. 语义放宽的列注释同步（common_phrases / requires_approval / cargo_types）。
-- 2) 计划表：onestop_inspection_plans 改名 onestop_periodic_plans（pkey/索引/
--    identity 序列一并改名，与 ent 全新建表命名对齐），字段沿用、语义放宽为
--    通用周期计划（挂载门控从 0103 前缀改为类别触发模式 = 计划生成）。
-- 列 COMMENT 与 ent schema 的 Comment 逐字一致（漂移守卫比对注释文本）。

-- +goose Up
-- —— 类型表：岗位列合并 ——
ALTER TABLE public.onestop_service_types
    ADD COLUMN IF NOT EXISTS execution_post character varying;

UPDATE public.onestop_service_types
   SET execution_post = COALESCE(repair_post, transport_post)
 WHERE execution_post IS NULL;

ALTER TABLE public.onestop_service_types
    DROP COLUMN IF EXISTS repair_post,
    DROP COLUMN IF EXISTS transport_post;

COMMENT ON COLUMN public.onestop_service_types.execution_post IS '执行岗位（派单目标岗位，如：电工/运送员/保洁员）';

-- —— 类型表：标准项目库改名 ——
ALTER TABLE public.onestop_service_types
    RENAME COLUMN inspection_items TO standard_items;

COMMENT ON COLUMN public.onestop_service_types.standard_items IS '标准作业项目库：周期计划点表模板';

-- —— 类型表：任务触发方式 ——
ALTER TABLE public.onestop_service_types
    ADD COLUMN IF NOT EXISTS trigger_mode character varying;

COMMENT ON COLUMN public.onestop_service_types.trigger_mode IS '任务触发方式 TRIGGER_ON_DEMAND:按需发起 TRIGGER_SCHEDULED:计划生成（未设置回退上级，默认按需）';

-- 存量库按播种矩阵回填 SCHEDULED（种子守卫只在空表播种，已部署库靠这里；
-- 前缀清单与 docs/yizhanshi/onestop-unified-service-type-config.md §7 矩阵一致，
-- 须同步维护）。只回填 SCHEDULED：其余节点保持 NULL，由解析链默认按需，
-- 不显式写 ON_DEMAND，以免污染「NULL = 未设置，向上继承」的语义。
UPDATE public.onestop_service_types SET trigger_mode = 'TRIGGER_SCHEDULED'
 WHERE code LIKE '0102%' OR code LIKE '0103%'
    OR code LIKE '010401%' OR code LIKE '01040301%' OR code LIKE '01040303%'
    OR code LIKE '01040401%' OR code LIKE '010405%'
    OR code LIKE '010503%' OR code LIKE '010505%' OR code LIKE '010506%'
    OR code LIKE '01060201%' OR code LIKE '01060301%' OR code LIKE '01060502%'
    OR code LIKE '01080402%'
    OR code LIKE '01090102%' OR code LIKE '010903%'
    OR code LIKE '011001%' OR code LIKE '01100201%'
    OR code LIKE '01120402%' OR code LIKE '01120403%';

-- —— 类型表：语义放宽的列注释同步 ——
COMMENT ON COLUMN public.onestop_service_types.common_phrases IS '常见语句清单：发起端快捷语句，兼作语义归型语料';

COMMENT ON COLUMN public.onestop_service_types.requires_approval IS '是否需事前审批（搬入搬出押运、动火作业、资源申请等管控类服务）';

COMMENT ON COLUMN public.onestop_service_types.cargo_types IS '运送物品细目清单：该类型承接的物品范围（推荐 0106 运送配送子树填写）';

-- —— 计划表改名（pkey/索引/identity 序列一并改名）——
ALTER TABLE public.onestop_inspection_plans RENAME TO onestop_periodic_plans;

ALTER TABLE public.onestop_periodic_plans
    RENAME CONSTRAINT onestop_inspection_plans_pkey TO onestop_periodic_plans_pkey;

ALTER INDEX IF EXISTS inspectionplan_service_type_id RENAME TO periodicplan_service_type_id;

ALTER INDEX IF EXISTS inspectionplan_freq_type RENAME TO periodicplan_freq_type;

ALTER SEQUENCE IF EXISTS public.onestop_inspection_plans_id_seq RENAME TO onestop_periodic_plans_id_seq;

COMMENT ON TABLE public.onestop_periodic_plans IS '一站式周期计划表（计划生成类类别的计划配置：频率/岗位/点表/生效期）';

COMMENT ON COLUMN public.onestop_periodic_plans.service_type_id IS '计划类别ID（onestop_service_types.id，触发模式为计划生成的小类/细类）';

COMMENT ON COLUMN public.onestop_periodic_plans.space_items IS '空间作业点表 [{name,enabled}]';

COMMENT ON COLUMN public.onestop_periodic_plans.equipment_items IS '设备作业点表 [{name,enabled}]';

-- +goose Down
-- 结构回退：岗位值按子树前缀拆回两列（0101/0106 之外的 execution_post 值丢弃），
-- trigger_mode 的存量值不保留——Down 仅用于结构对齐，dev 阶段可接受。
ALTER TABLE public.onestop_periodic_plans RENAME TO onestop_inspection_plans;

ALTER TABLE public.onestop_inspection_plans
    RENAME CONSTRAINT onestop_periodic_plans_pkey TO onestop_inspection_plans_pkey;

ALTER INDEX IF EXISTS periodicplan_service_type_id RENAME TO inspectionplan_service_type_id;

ALTER INDEX IF EXISTS periodicplan_freq_type RENAME TO inspectionplan_freq_type;

ALTER SEQUENCE IF EXISTS public.onestop_periodic_plans_id_seq RENAME TO onestop_inspection_plans_id_seq;

COMMENT ON TABLE public.onestop_inspection_plans IS '一站式巡检计划表（0103 巡检巡查类别的计划配置：频率/岗位/点表/生效期）';

COMMENT ON COLUMN public.onestop_inspection_plans.service_type_id IS '巡检类别ID（onestop_service_types.id，0103 子树）';

COMMENT ON COLUMN public.onestop_inspection_plans.space_items IS '空间巡检点表 [{name,enabled}]';

COMMENT ON COLUMN public.onestop_inspection_plans.equipment_items IS '设备巡检点表 [{name,enabled}]';

ALTER TABLE public.onestop_service_types
    DROP COLUMN IF EXISTS trigger_mode;

ALTER TABLE public.onestop_service_types
    RENAME COLUMN standard_items TO inspection_items;

COMMENT ON COLUMN public.onestop_service_types.inspection_items IS '标准巡检项目库：巡检计划点表模板（仅 0103 巡检巡查子树）';

COMMENT ON COLUMN public.onestop_service_types.common_phrases IS '常见报修语句清单：报修端快捷语句，兼作语义归型语料';

COMMENT ON COLUMN public.onestop_service_types.requires_approval IS '是否需事前审批（搬入搬出押运、麻精药品等管控类运送）';

COMMENT ON COLUMN public.onestop_service_types.cargo_types IS '运送物品细目清单：该类型承接的物品范围（仅 0106 运送配送子树）';

ALTER TABLE public.onestop_service_types
    ADD COLUMN IF NOT EXISTS repair_post character varying,
    ADD COLUMN IF NOT EXISTS transport_post character varying;

UPDATE public.onestop_service_types
   SET repair_post = execution_post WHERE code LIKE '0101%';

UPDATE public.onestop_service_types
   SET transport_post = execution_post WHERE code LIKE '0106%';

COMMENT ON COLUMN public.onestop_service_types.repair_post IS '维修岗位（派单目标岗位，如：电工/管工/空调工）';

COMMENT ON COLUMN public.onestop_service_types.transport_post IS '运送岗位（派单目标岗位，如：运送员/标本运送员/护工）';

ALTER TABLE public.onestop_service_types
    DROP COLUMN IF EXISTS execution_post;
