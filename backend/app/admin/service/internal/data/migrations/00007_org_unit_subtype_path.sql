-- 组织架构类型两级化：sys_org_units 增加细分类型路径列
-- 设计: docs/ontology/建模/建模设计/组织架构本体建模与个体同步设计.md §5.2/§5.3
-- 说明: subtype_path 存受控词表 code 数组（从属到叶，最多 3 段），
--       顶层 type 仍是枚举列；新增列可空，存量数据无需回填。
-- 禁止修改本文件；后续变更一律新增 000NN_org_unit_<what>.sql。

-- +goose Up
ALTER TABLE public.sys_org_units ADD COLUMN subtype_path jsonb;
COMMENT ON COLUMN public.sys_org_units.subtype_path IS '细分类型路径（受控词表 code 数组，从属到叶；门店业态/部门职能/项目类型等）';

CREATE INDEX idx_sys_org_units_subtype_path ON public.sys_org_units
    USING gin (subtype_path);

-- +goose Down
DROP INDEX IF EXISTS public.idx_sys_org_units_subtype_path;
ALTER TABLE public.sys_org_units DROP COLUMN IF EXISTS subtype_path;
