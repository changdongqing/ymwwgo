-- ============================================================
-- 脚本: 00005_ontology_uom_seed.sql
-- 目的: 计量单位库种子建模——并入「有名万物」本体(id=7)，多顶层类架构本仓首次落地：
--       万物薄根 NamedThing + 单位 Units 域容器类 + 计量单位 Unit/量纲类型 QuantityKind 两叶子类，
--       52 量纲个体 + 91 单位个体（园区/医院/商场楼控与消防场景，QUDT 3.4.0 裁剪）
-- 设计: docs/ontology/建模/建模设计/数据单位清单及种子建模分析.md V1.0
-- 来源: 源项目 Flyway V20261015_07/_08 的本仓移植（QUDT 元数据沿用其一次性提取产物，保持源词形）
-- 备注:
--   1. id 编配: 实体 301~491（code 兜底占 322）/ 公理 5001~6330（单位容器类续编 6327~6330）；
--      源项目组织域挂接行(5172/5173/5176~5183) 本仓无组织域不落，号段空缺以便跨仓对照
--   2. 双路径兼容: 全部 INSERT 带 ON CONFLICT DO NOTHING 可重复执行；种子不写变更日志，
--      本体 version 不动；尾部 setval 校准 identity 序列
--   3. code 属性双环境兼容: dev 老库已有 UI 懒创建 base#code（dev 实测 id=77）——按 IRI
--      幂等跳过（裸 ON CONFLICT），自身骨架公理仅当本次新插入(id=322)时落行，单位编码 62 行
--      以 IRI 子查询定位 object_id；新库补种 id=322
--   4. 单位数值元数据(symbol/ucumCode/conversionMultiplier/conversionOffset)提取自
--      QUDT 3.4.0 qudt-all.ttl 原文、保持源词形，溯源经 qudtIri 注解
--   5. 缺口兜底(分析文档 §5.4.5): %LEL 用 PERCENT、pH 用 ONE 承载，PER-HR(换气次数)
--      挂 Frequency 语义；量纲层级(QUDT specializationOf)不落个体层
--   6. 单位个体按 06 期 MANUAL 语义落编码 U-<QUDT局部名>，编码行 sort_order=0 占首位
--      （个体页/frame 编码解析按 code 属性行优先，无编码行时首条 62 行会被误当编码）
--   7. 换算链 scalingOf 保持 QUDT SI 锚定不改按习惯改写；国内习惯基准独立落于
--      conventionalUnit（函数性走 33 号段——对象属性特征，与数据属性特征 43 号段不可混用）
-- ============================================================

-- +goose Up

-- ----------------------------
-- 1. 本体文档定位更新（描述扩为"平台通用词表（多顶层类架构）"定位；原描述：
--    '内置种子本体：有名万物——万物皆有名，以名指物'）
-- ----------------------------
UPDATE ont_ontologies SET description =
    '平台通用词表本体（多顶层类架构）：以「万物」为薄根、按域扩展顶层类——计量域（单位容器类下辖计量单位/量纲类型，对齐 QUDT 3.4.0，按楼控与消防场景裁剪）已落地，组织/空间/设备/表单等后续按序增补。',
    updated_at = now()
WHERE id = 7 AND deleted_at IS NULL;

-- ----------------------------
-- 1.5 code 编码属性兜底补种（对齐 06 期 UI 懒创建形态：仅声明 + 值域 xsd:string；
--     dev 老库已有懒创建行时按 IRI 幂等跳过，新库落 id=322）
-- ----------------------------
INSERT INTO ont_entities (id, ontology_id, entity_type, iri, local_name, display_label, deprecated, created_by, updated_by, created_at, updated_at)
VALUES (322, 7, 30, 'https://youmingwanwu.com/ontology/youmingwanwu#code', 'code', '编码', false, 1, 1, now(), now())
ON CONFLICT DO NOTHING;

UPDATE ont_entities SET display_label = '编码', updated_at = now()
WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL
  AND (display_label IS NULL OR display_label = '');

-- ----------------------------
-- 1.6 code 属性自身骨架公理（声明/值域/label/comment）——仅当本次新插入(id=322)时落行，
--     老库懒创建行已自带声明与值域，不重复
-- ----------------------------
INSERT INTO ont_axioms (id, ontology_id, axiom_type, subject_id, object_id, predicate_iri, value_text, lang, sort_order, created_by, updated_by, created_at, updated_at)
SELECT v.id, 7, v.t, 322, v.oid, v.p, v.val, v.lang, 0, 1, 1, now(), now()
FROM (VALUES
    (5007, 10,  NULL::bigint, NULL::text, NULL::text, NULL::text),
    (5162, 100, NULL::bigint, 'http://www.w3.org/2000/01/rdf-schema#label',   '编码', 'zh'::varchar),
    (5163, 100, NULL::bigint, 'http://www.w3.org/2000/01/rdf-schema#comment',
     '个体业务编码（06 期默认编码属性 base#code，UI 懒创建的同名属性由此种子预置）；单位种子按 MANUAL 语义落码 U-<QUDT局部名>，遵循「编码行 sort_order=0 占首位」约定。', 'zh'::varchar),
    (5192, 42,  NULL::bigint, NULL::text, NULL::text, 'http://www.w3.org/2001/XMLSchema#string')
) AS v(id, t, oid, p, val, lang)
WHERE EXISTS (SELECT 1 FROM ont_entities e
              WHERE e.ontology_id = 7 AND e.iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND e.id = 322 AND e.deleted_at IS NULL)
ON CONFLICT DO NOTHING;

-- ----------------------------
-- 2. 实体注册表（骨架/量纲/单位 155 行；code 兜底 322 见上文单列）
-- ----------------------------
INSERT INTO ont_entities (id, ontology_id, entity_type, iri, local_name, display_label, deprecated, created_by, updated_by, created_at, updated_at) VALUES
-- 类(10)
(301, 7, 10, 'https://youmingwanwu.com/ontology/youmingwanwu#NamedThing', 'NamedThing', '万物', false, 1, 1, now(), now()),
(304, 7, 10, 'https://youmingwanwu.com/ontology/youmingwanwu#Units', 'Units', '单位', false, 1, 1, now(), now()),
(302, 7, 10, 'https://youmingwanwu.com/ontology/youmingwanwu#Unit', 'Unit', '计量单位', false, 1, 1, now(), now()),
(303, 7, 10, 'https://youmingwanwu.com/ontology/youmingwanwu#QuantityKind', 'QuantityKind', '量纲类型', false, 1, 1, now(), now()),
-- 对象属性(20)
(310, 7, 20, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', 'hasQuantityKind', '挂接量纲', false, 1, 1, now(), now()),
-- 数据属性(30)
(320, 7, 30, 'https://youmingwanwu.com/ontology/youmingwanwu#conversionMultiplier', 'conversionMultiplier', '换算倍率', false, 1, 1, now(), now()),
(321, 7, 30, 'https://youmingwanwu.com/ontology/youmingwanwu#conversionOffset', 'conversionOffset', '换算偏移', false, 1, 1, now(), now()),
-- 注解属性(40)
(330, 7, 40, 'https://youmingwanwu.com/ontology/youmingwanwu#symbol', 'symbol', '单位符号', false, 1, 1, now(), now()),
(331, 7, 40, 'https://youmingwanwu.com/ontology/youmingwanwu#ucumCode', 'ucumCode', 'UCUM编码', false, 1, 1, now(), now()),
(332, 7, 40, 'https://youmingwanwu.com/ontology/youmingwanwu#qudtIri', 'qudtIri', 'QUDT溯源', false, 1, 1, now(), now()),
-- 个体(60)
(341, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Temperature', 'Temperature', '温度', false, 1, 1, now(), now()),
(342, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#TemperatureDifference', 'TemperatureDifference', '温差', false, 1, 1, now(), now()),
(343, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#CorrelatedColorTemperature', 'CorrelatedColorTemperature', '相关色温', false, 1, 1, now(), now()),
(344, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#RelativeHumidity', 'RelativeHumidity', '相对湿度', false, 1, 1, now(), now()),
(345, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#AbsoluteHumidity', 'AbsoluteHumidity', '绝对湿度', false, 1, 1, now(), now()),
(346, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Pressure', 'Pressure', '压力', false, 1, 1, now(), now()),
(347, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#GaugePressure', 'GaugePressure', '表压', false, 1, 1, now(), now()),
(348, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#StaticPressure', 'StaticPressure', '静压', false, 1, 1, now(), now()),
(349, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#DynamicPressure', 'DynamicPressure', '动压', false, 1, 1, now(), now()),
(350, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#AtmosphericPressure', 'AtmosphericPressure', '大气压', false, 1, 1, now(), now()),
(351, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#VolumeFlowRate', 'VolumeFlowRate', '体积流量', false, 1, 1, now(), now()),
(352, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MassFlowRate', 'MassFlowRate', '质量流量', false, 1, 1, now(), now()),
(353, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Length', 'Length', '长度', false, 1, 1, now(), now()),
(354, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#LiquidLevel', 'LiquidLevel', '液位', false, 1, 1, now(), now()),
(355, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Volume', 'Volume', '体积', false, 1, 1, now(), now()),
(356, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Area', 'Area', '面积', false, 1, 1, now(), now()),
(357, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Speed', 'Speed', '速度', false, 1, 1, now(), now()),
(358, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#RotationalFrequency', 'RotationalFrequency', '旋转频率', false, 1, 1, now(), now()),
(359, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#PlaneAngle', 'PlaneAngle', '平面角', false, 1, 1, now(), now()),
(360, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Time', 'Time', '时间', false, 1, 1, now(), now()),
(361, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Mass', 'Mass', '质量', false, 1, 1, now(), now()),
(362, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#ElectricCurrent', 'ElectricCurrent', '电流', false, 1, 1, now(), now()),
(363, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Voltage', 'Voltage', '电压', false, 1, 1, now(), now()),
(364, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Frequency', 'Frequency', '频率', false, 1, 1, now(), now()),
(365, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#ElectricPower', 'ElectricPower', '有功功率', false, 1, 1, now(), now()),
(366, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#ApparentPower', 'ApparentPower', '视在功率', false, 1, 1, now(), now()),
(367, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#ReactivePower', 'ReactivePower', '无功功率', false, 1, 1, now(), now()),
(368, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#ElectricEnergy', 'ElectricEnergy', '有功电能', false, 1, 1, now(), now()),
(369, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#ReactiveEnergy', 'ReactiveEnergy', '无功电能', false, 1, 1, now(), now()),
(370, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#ApparentEnergy', 'ApparentEnergy', '视在电能', false, 1, 1, now(), now()),
(371, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#ThermalEnergy', 'ThermalEnergy', '热能', false, 1, 1, now(), now()),
(372, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Energy', 'Energy', '能量', false, 1, 1, now(), now()),
(373, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Power', 'Power', '功率', false, 1, 1, now(), now()),
(374, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#HeatFlowRate', 'HeatFlowRate', '热流量', false, 1, 1, now(), now()),
(375, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#BatteryCapacity', 'BatteryCapacity', '电池容量', false, 1, 1, now(), now()),
(376, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Resistance', 'Resistance', '电阻', false, 1, 1, now(), now()),
(377, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#ElectricConductivity', 'ElectricConductivity', '电导率', false, 1, 1, now(), now()),
(378, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Illuminance', 'Illuminance', '照度', false, 1, 1, now(), now()),
(379, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#LuminousFlux', 'LuminousFlux', '光通量', false, 1, 1, now(), now()),
(380, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#LuminousIntensity', 'LuminousIntensity', '发光强度', false, 1, 1, now(), now()),
(381, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#LuminousEfficacy', 'LuminousEfficacy', '光效', false, 1, 1, now(), now()),
(382, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MassConcentration', 'MassConcentration', '质量浓度', false, 1, 1, now(), now()),
(383, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#DimensionlessRatio', 'DimensionlessRatio', '无量纲比', false, 1, 1, now(), now()),
(384, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#VolumeFraction', 'VolumeFraction', '体积分数', false, 1, 1, now(), now()),
(385, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MoleFraction', 'MoleFraction', '摩尔分数', false, 1, 1, now(), now()),
(386, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#AttenuationCoefficient', 'AttenuationCoefficient', '衰减系数', false, 1, 1, now(), now()),
(387, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Turbidity', 'Turbidity', '浊度', false, 1, 1, now(), now()),
(388, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#SoundPressureLevel', 'SoundPressureLevel', '声压级', false, 1, 1, now(), now()),
(389, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Count', 'Count', '计数', false, 1, 1, now(), now()),
(390, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Torque', 'Torque', '扭矩', false, 1, 1, now(), now()),
(391, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#Irradiance', 'Irradiance', '辐照度', false, 1, 1, now(), now()),
(392, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#EnergyPerArea', 'EnergyPerArea', '单位面积能量', false, 1, 1, now(), now()),
(401, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#DEG_C', 'DEG_C', '摄氏度', false, 1, 1, now(), now()),
(402, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#K', 'K', '开尔文', false, 1, 1, now(), now()),
(403, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#DEG_F', 'DEG_F', '华氏度', false, 1, 1, now(), now()),
(404, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#PERCENT_RH', 'PERCENT_RH', '相对湿度（%RH）', false, 1, 1, now(), now()),
(405, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#GM-PER-M3', 'GM-PER-M3', '克每立方米', false, 1, 1, now(), now()),
(406, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#PA', 'PA', '帕斯卡', false, 1, 1, now(), now()),
(407, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloPA', 'KiloPA', '千帕', false, 1, 1, now(), now()),
(408, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MegaPA', 'MegaPA', '兆帕', false, 1, 1, now(), now()),
(409, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#BAR', 'BAR', '巴', false, 1, 1, now(), now()),
(410, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MilliM_H2O', 'MilliM_H2O', '毫米水柱', false, 1, 1, now(), now()),
(411, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#M_H2O', 'M_H2O', '米水柱', false, 1, 1, now(), now()),
(412, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#IN_H2O', 'IN_H2O', '英寸水柱', false, 1, 1, now(), now()),
(413, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MilliM_HG', 'MilliM_HG', '毫米汞柱', false, 1, 1, now(), now()),
(414, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#PSI', 'PSI', '磅力每平方英寸', false, 1, 1, now(), now()),
(415, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#HectoPA', 'HectoPA', '百帕', false, 1, 1, now(), now()),
(416, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#M3-PER-HR', 'M3-PER-HR', '立方米每小时', false, 1, 1, now(), now()),
(417, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#M3-PER-SEC', 'M3-PER-SEC', '立方米每秒', false, 1, 1, now(), now()),
(418, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#L-PER-SEC', 'L-PER-SEC', '升每秒', false, 1, 1, now(), now()),
(419, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#L-PER-MIN', 'L-PER-MIN', '升每分钟', false, 1, 1, now(), now()),
(420, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#M3', 'M3', '立方米', false, 1, 1, now(), now()),
(421, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#M-PER-SEC', 'M-PER-SEC', '米每秒', false, 1, 1, now(), now()),
(422, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloM-PER-HR', 'KiloM-PER-HR', '千米每小时', false, 1, 1, now(), now()),
(423, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#REV-PER-MIN', 'REV-PER-MIN', '转每分钟', false, 1, 1, now(), now()),
(424, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#TONNE-PER-HR', 'TONNE-PER-HR', '吨每小时', false, 1, 1, now(), now()),
(425, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloGM-PER-SEC', 'KiloGM-PER-SEC', '千克每秒', false, 1, 1, now(), now()),
(426, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#PER-HR', 'PER-HR', '每小时', false, 1, 1, now(), now()),
(427, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#PPM', 'PPM', '百万分率', false, 1, 1, now(), now()),
(428, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#PPB', 'PPB', '十亿分率', false, 1, 1, now(), now()),
(429, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#PERCENT', 'PERCENT', '百分比', false, 1, 1, now(), now()),
(430, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MicroGM-PER-M3', 'MicroGM-PER-M3', '微克每立方米', false, 1, 1, now(), now()),
(431, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MilliGM-PER-M3', 'MilliGM-PER-M3', '毫克每立方米', false, 1, 1, now(), now()),
(432, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#PERCENT-PER-M', 'PERCENT-PER-M', '百分比每米', false, 1, 1, now(), now()),
(433, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#DeciB-PER-M', 'DeciB-PER-M', '分贝每米', false, 1, 1, now(), now()),
(434, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#NTU', 'NTU', '浊度（NTU）', false, 1, 1, now(), now()),
(435, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MilliGM-PER-L', 'MilliGM-PER-L', '毫克每升', false, 1, 1, now(), now()),
(436, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#GM-PER-L', 'GM-PER-L', '克每升', false, 1, 1, now(), now()),
(437, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MicroS-PER-CentiM', 'MicroS-PER-CentiM', '微西门子每厘米', false, 1, 1, now(), now()),
(438, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#V', 'V', '伏特', false, 1, 1, now(), now()),
(439, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MilliV', 'MilliV', '毫伏', false, 1, 1, now(), now()),
(440, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloV', 'KiloV', '千伏', false, 1, 1, now(), now()),
(441, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#A', 'A', '安培', false, 1, 1, now(), now()),
(442, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MilliA', 'MilliA', '毫安', false, 1, 1, now(), now()),
(443, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#W', 'W', '瓦特', false, 1, 1, now(), now()),
(444, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloW', 'KiloW', '千瓦', false, 1, 1, now(), now()),
(445, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MegaW', 'MegaW', '兆瓦', false, 1, 1, now(), now()),
(446, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#V-A', 'V-A', '伏安', false, 1, 1, now(), now()),
(447, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloV-A', 'KiloV-A', '千伏安', false, 1, 1, now(), now()),
(448, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#VAR', 'VAR', '乏', false, 1, 1, now(), now()),
(449, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloVAR', 'KiloVAR', '千乏', false, 1, 1, now(), now()),
(450, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloW-HR', 'KiloW-HR', '千瓦时', false, 1, 1, now(), now()),
(451, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MegaW-HR', 'MegaW-HR', '兆瓦时', false, 1, 1, now(), now()),
(452, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloVAR-HR', 'KiloVAR-HR', '千乏时', false, 1, 1, now(), now()),
(453, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloV-A-HR', 'KiloV-A-HR', '千伏安时', false, 1, 1, now(), now()),
(454, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#HZ', 'HZ', '赫兹', false, 1, 1, now(), now()),
(455, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#OHM', 'OHM', '欧姆', false, 1, 1, now(), now()),
(456, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MegaOHM', 'MegaOHM', '兆欧', false, 1, 1, now(), now()),
(457, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#A-HR', 'A-HR', '安时', false, 1, 1, now(), now()),
(458, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#W-PER-M2', 'W-PER-M2', '瓦每平方米', false, 1, 1, now(), now()),
(459, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloW-HR-PER-M2', 'KiloW-HR-PER-M2', '千瓦时每平方米', false, 1, 1, now(), now()),
(460, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#LUX', 'LUX', '勒克斯', false, 1, 1, now(), now()),
(461, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#LM', 'LM', '流明', false, 1, 1, now(), now()),
(462, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#LM-PER-W', 'LM-PER-W', '流明每瓦', false, 1, 1, now(), now()),
(463, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#CD', 'CD', '坎德拉', false, 1, 1, now(), now()),
(464, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#M', 'M', '米', false, 1, 1, now(), now()),
(465, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MilliM', 'MilliM', '毫米', false, 1, 1, now(), now()),
(466, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#CentiM', 'CentiM', '厘米', false, 1, 1, now(), now()),
(467, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloGM', 'KiloGM', '千克', false, 1, 1, now(), now()),
(468, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#GM', 'GM', '克', false, 1, 1, now(), now()),
(469, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#TONNE', 'TONNE', '吨', false, 1, 1, now(), now()),
(470, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#SEC', 'SEC', '秒', false, 1, 1, now(), now()),
(471, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MIN', 'MIN', '分钟', false, 1, 1, now(), now()),
(472, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#HR', 'HR', '小时', false, 1, 1, now(), now()),
(473, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#DAY', 'DAY', '天', false, 1, 1, now(), now()),
(474, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#DEG', 'DEG', '度（平面角）', false, 1, 1, now(), now()),
(475, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#RAD', 'RAD', '弧度', false, 1, 1, now(), now()),
(476, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#ONE', 'ONE', '一（计数）', false, 1, 1, now(), now()),
(477, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#DeciB', 'DeciB', '分贝', false, 1, 1, now(), now()),
(478, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#DeciB_A', 'DeciB_A', 'A计权分贝', false, 1, 1, now(), now()),
(479, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#L', 'L', '升', false, 1, 1, now(), now()),
(480, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloL', 'KiloL', '千升', false, 1, 1, now(), now()),
(481, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloJ', 'KiloJ', '千焦', false, 1, 1, now(), now()),
(482, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#MegaJ', 'MegaJ', '兆焦', false, 1, 1, now(), now()),
(483, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#GigaJ', 'GigaJ', '吉焦', false, 1, 1, now(), now()),
(484, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#KiloCAL', 'KiloCAL', '千卡', false, 1, 1, now(), now()),
(485, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#TON_FG', 'TON_FG', '冷吨', false, 1, 1, now(), now()),
(486, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#BTU_IT-PER-HR', 'BTU_IT-PER-HR', '英热单位每小时', false, 1, 1, now(), now()),
(487, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#N-M', 'N-M', '牛顿米', false, 1, 1, now(), now())
ON CONFLICT (id) DO NOTHING;

-- ----------------------------
-- 3. 公理（V7 段 1 173 行 + V8 段 139 行 + 单位容器类 4 行；code 骨架 4 行条件落库见上文）
-- ----------------------------
INSERT INTO ont_axioms (id, ontology_id, axiom_type, subject_id, object_id, predicate_iri, value_text, lang, sort_order, created_by, updated_by, created_at, updated_at) VALUES
-- 实体声明(10)
(5001, 7, 10, 301, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5002, 7, 10, 302, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5003, 7, 10, 303, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5004, 7, 10, 310, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5005, 7, 10, 320, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5006, 7, 10, 321, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5008, 7, 10, 330, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5009, 7, 10, 331, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5010, 7, 10, 332, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5011, 7, 10, 341, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5012, 7, 10, 342, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5013, 7, 10, 343, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5014, 7, 10, 344, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5015, 7, 10, 345, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5016, 7, 10, 346, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5017, 7, 10, 347, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5018, 7, 10, 348, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5019, 7, 10, 349, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5020, 7, 10, 350, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5021, 7, 10, 351, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5022, 7, 10, 352, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5023, 7, 10, 353, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5024, 7, 10, 354, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5025, 7, 10, 355, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5026, 7, 10, 356, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5027, 7, 10, 357, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5028, 7, 10, 358, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5029, 7, 10, 359, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5030, 7, 10, 360, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5031, 7, 10, 361, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5032, 7, 10, 362, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5033, 7, 10, 363, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5034, 7, 10, 364, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5035, 7, 10, 365, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5036, 7, 10, 366, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5037, 7, 10, 367, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5038, 7, 10, 368, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5039, 7, 10, 369, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5040, 7, 10, 370, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5041, 7, 10, 371, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5042, 7, 10, 372, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5043, 7, 10, 373, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5044, 7, 10, 374, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5045, 7, 10, 375, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5046, 7, 10, 376, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5047, 7, 10, 377, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5048, 7, 10, 378, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5049, 7, 10, 379, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5050, 7, 10, 380, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5051, 7, 10, 381, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5052, 7, 10, 382, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5053, 7, 10, 383, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5054, 7, 10, 384, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5055, 7, 10, 385, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5056, 7, 10, 386, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5057, 7, 10, 387, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5058, 7, 10, 388, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5059, 7, 10, 389, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5060, 7, 10, 390, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5061, 7, 10, 391, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5062, 7, 10, 392, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5063, 7, 10, 401, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5064, 7, 10, 402, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5065, 7, 10, 403, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5066, 7, 10, 404, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5067, 7, 10, 405, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5068, 7, 10, 406, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5069, 7, 10, 407, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5070, 7, 10, 408, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5071, 7, 10, 409, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5072, 7, 10, 410, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5073, 7, 10, 411, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5074, 7, 10, 412, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5075, 7, 10, 413, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5076, 7, 10, 414, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5077, 7, 10, 415, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5078, 7, 10, 416, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5079, 7, 10, 417, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5080, 7, 10, 418, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5081, 7, 10, 419, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5082, 7, 10, 420, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5083, 7, 10, 421, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5084, 7, 10, 422, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5085, 7, 10, 423, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5086, 7, 10, 424, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5087, 7, 10, 425, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5088, 7, 10, 426, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5089, 7, 10, 427, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5090, 7, 10, 428, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5091, 7, 10, 429, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5092, 7, 10, 430, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5093, 7, 10, 431, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5094, 7, 10, 432, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5095, 7, 10, 433, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5096, 7, 10, 434, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5097, 7, 10, 435, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5098, 7, 10, 436, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5099, 7, 10, 437, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5100, 7, 10, 438, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5101, 7, 10, 439, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5102, 7, 10, 440, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5103, 7, 10, 441, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5104, 7, 10, 442, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5105, 7, 10, 443, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5106, 7, 10, 444, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5107, 7, 10, 445, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5108, 7, 10, 446, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5109, 7, 10, 447, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5110, 7, 10, 448, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5111, 7, 10, 449, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5112, 7, 10, 450, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5113, 7, 10, 451, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5114, 7, 10, 452, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5115, 7, 10, 453, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5116, 7, 10, 454, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5117, 7, 10, 455, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5118, 7, 10, 456, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5119, 7, 10, 457, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5120, 7, 10, 458, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5121, 7, 10, 459, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5122, 7, 10, 460, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5123, 7, 10, 461, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5124, 7, 10, 462, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5125, 7, 10, 463, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5126, 7, 10, 464, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5127, 7, 10, 465, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5128, 7, 10, 466, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5129, 7, 10, 467, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5130, 7, 10, 468, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5131, 7, 10, 469, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5132, 7, 10, 470, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5133, 7, 10, 471, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5134, 7, 10, 472, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5135, 7, 10, 473, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5136, 7, 10, 474, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5137, 7, 10, 475, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5138, 7, 10, 476, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5139, 7, 10, 477, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5140, 7, 10, 478, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5141, 7, 10, 479, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5142, 7, 10, 480, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5143, 7, 10, 481, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5144, 7, 10, 482, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5145, 7, 10, 483, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5146, 7, 10, 484, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5147, 7, 10, 485, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5148, 7, 10, 486, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5149, 7, 10, 487, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
-- 注解断言(100)
(5150, 7, 100, 301, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '万物', 'zh', 0, 1, 1, now(), now()),
(5151, 7, 100, 301, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '平台薄根类：owl:Thing 之下所有顶层域类之父，跨域属性定义域与顶层不相交的锚点，刻意零属性。', 'zh', 0, 1, 1, now(), now()),
(5152, 7, 100, 302, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '计量单位', 'zh', 0, 1, 1, now(), now()),
(5153, 7, 100, 302, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '单位个体主类目；个体取 QUDT 3.4.0 局部名，符号/UCUM/换算因子以注解与数据断言承载。', 'zh', 0, 1, 1, now(), now()),
(5154, 7, 100, 303, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '量纲类型', 'zh', 0, 1, 1, now(), now()),
(5155, 7, 100, 303, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '量纲个体主类目（对应 qudt:QuantityKind），本期扁平入库不做量纲树。', 'zh', 0, 1, 1, now(), now()),
(5156, 7, 100, 310, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '挂接量纲', 'zh', 0, 1, 1, now(), now()),
(5157, 7, 100, 310, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '单位所属物理量纲；对应 qudt:hasQuantityKind，定义域计量单位、值域量纲类型。', 'zh', 0, 1, 1, now(), now()),
(5158, 7, 100, 320, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '换算倍率', 'zh', 0, 1, 1, now(), now()),
(5159, 7, 100, 320, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '单位换算到 SI 基准的乘法因子（对应 qudt:conversionMultiplier），函数性。', 'zh', 0, 1, 1, now(), now()),
(5160, 7, 100, 321, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '换算偏移', 'zh', 0, 1, 1, now(), now()),
(5161, 7, 100, 321, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '单位换算到 SI 基准的加法偏移（对应 qudt:conversionOffset，如摄氏度 273.15），函数性。', 'zh', 0, 1, 1, now(), now()),
(5164, 7, 100, 330, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '单位符号', 'zh', 0, 1, 1, now(), now()),
(5165, 7, 100, 330, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '单位显示符号（对应 qudt:symbol），如 °C、kW·h。', 'zh', 0, 1, 1, now(), now()),
(5166, 7, 100, 331, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', 'UCUM编码', 'zh', 0, 1, 1, now(), now()),
(5167, 7, 100, 331, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', 'UCUM 交换码（对应 qudt:ucumCode），跨单位词表对齐的事实键。', 'zh', 0, 1, 1, now(), now()),
(5168, 7, 100, 332, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', 'QUDT溯源', 'zh', 0, 1, 1, now(), now()),
(5169, 7, 100, 332, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', 'QUDT 3.4.0 原文 IRI（http://qudt.org/vocab/...），因本系统 IRI=baseIri+localName 约束以字面量注解留存。', 'zh', 0, 1, 1, now(), now()),
-- 子类(20)
(5170, 7, 20, 302, 304, NULL, NULL, NULL, 0, 1, 1, now(), now()),  -- 计量单位 ⊑ 单位
(5171, 7, 20, 303, 304, NULL, NULL, NULL, 0, 1, 1, now(), now()),  -- 量纲类型 ⊑ 单位
-- 不相交(21)
(5174, 7, 21, 302, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),  -- 单位⊥量纲
(5175, 7, 21, 303, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),  -- 单位⊥量纲
-- 对象属性定义域(31)
(5184, 7, 31, 310, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),  -- hasQuantityKind 定义域=计量单位
-- 对象属性值域(32)
(5185, 7, 32, 310, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),  -- hasQuantityKind 值域=量纲类型
-- 数据属性定义域(41)
(5186, 7, 41, 320, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),  -- 换算倍率 定义域=计量单位
-- 数据属性值域(42)
(5187, 7, 42, 320, NULL, NULL, 'http://www.w3.org/2001/XMLSchema#decimal', NULL, 0, 1, 1, now(), now()),  -- 换算倍率 值域=xsd:decimal(内建轨)
-- 数据属性特征(43)
(5188, 7, 43, 320, NULL, 'http://www.w3.org/2002/07/owl#FunctionalProperty', NULL, NULL, 0, 1, 1, now(), now()),  -- 换算倍率 函数性
-- 数据属性定义域(41)
(5189, 7, 41, 321, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),  -- 换算偏移 定义域=计量单位
-- 数据属性值域(42)
(5190, 7, 42, 321, NULL, NULL, 'http://www.w3.org/2001/XMLSchema#decimal', NULL, 0, 1, 1, now(), now()),  -- 换算偏移 值域=xsd:decimal(内建轨)
-- 数据属性特征(43)
(5191, 7, 43, 321, NULL, 'http://www.w3.org/2002/07/owl#FunctionalProperty', NULL, NULL, 0, 1, 1, now(), now()),  -- 换算偏移 函数性
-- 数据属性值域(42)
-- 类型断言(60)
(5193, 7, 60, 341, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5194, 7, 60, 342, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5195, 7, 60, 343, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5196, 7, 60, 344, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5197, 7, 60, 345, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5198, 7, 60, 346, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5199, 7, 60, 347, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5200, 7, 60, 348, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5201, 7, 60, 349, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5202, 7, 60, 350, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5203, 7, 60, 351, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5204, 7, 60, 352, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5205, 7, 60, 353, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5206, 7, 60, 354, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5207, 7, 60, 355, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5208, 7, 60, 356, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5209, 7, 60, 357, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5210, 7, 60, 358, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5211, 7, 60, 359, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5212, 7, 60, 360, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5213, 7, 60, 361, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5214, 7, 60, 362, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5215, 7, 60, 363, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5216, 7, 60, 364, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5217, 7, 60, 365, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5218, 7, 60, 366, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5219, 7, 60, 367, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5220, 7, 60, 368, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5221, 7, 60, 369, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5222, 7, 60, 370, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5223, 7, 60, 371, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5224, 7, 60, 372, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5225, 7, 60, 373, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5226, 7, 60, 374, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5227, 7, 60, 375, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5228, 7, 60, 376, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5229, 7, 60, 377, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5230, 7, 60, 378, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5231, 7, 60, 379, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5232, 7, 60, 380, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5233, 7, 60, 381, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5234, 7, 60, 382, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5235, 7, 60, 383, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5236, 7, 60, 384, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5237, 7, 60, 385, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5238, 7, 60, 386, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5239, 7, 60, 387, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5240, 7, 60, 388, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5241, 7, 60, 389, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5242, 7, 60, 390, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5243, 7, 60, 391, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5244, 7, 60, 392, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),
-- 注解断言(100)
(5245, 7, 100, 341, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '温度', 'zh', 0, 1, 1, now(), now()),
(5246, 7, 100, 341, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '温度测点与感温探测主量纲。', 'zh', 0, 1, 1, now(), now()),
(5247, 7, 100, 341, 332, NULL, 'http://qudt.org/vocab/quantitykind/Temperature', NULL, 0, 1, 1, now(), now()),
(5248, 7, 100, 342, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '温差', 'zh', 0, 1, 1, now(), now()),
(5249, 7, 100, 342, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '两个温度之差；供回水温差等差分量用 K 计量。', 'zh', 0, 1, 1, now(), now()),
(5250, 7, 100, 342, 332, NULL, 'http://qudt.org/vocab/quantitykind/TemperatureDifference', NULL, 0, 1, 1, now(), now()),
(5251, 7, 100, 343, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '相关色温', 'zh', 0, 1, 1, now(), now()),
(5252, 7, 100, 343, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '光源色温，如 LED 4000K。', 'zh', 0, 1, 1, now(), now()),
(5253, 7, 100, 343, 332, NULL, 'http://qudt.org/vocab/quantitykind/CorrelatedColorTemperature', NULL, 0, 1, 1, now(), now()),
(5254, 7, 100, 344, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '相对湿度', 'zh', 0, 1, 1, now(), now()),
(5255, 7, 100, 344, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '空气中水汽压与饱和水汽压之比，单位 %RH。', 'zh', 0, 1, 1, now(), now()),
(5256, 7, 100, 344, 332, NULL, 'http://qudt.org/vocab/quantitykind/RelativeHumidity', NULL, 0, 1, 1, now(), now()),
(5257, 7, 100, 345, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '绝对湿度', 'zh', 0, 1, 1, now(), now()),
(5258, 7, 100, 345, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '单位体积空气中水汽质量。', 'zh', 0, 1, 1, now(), now()),
(5259, 7, 100, 345, 332, NULL, 'http://qudt.org/vocab/quantitykind/AbsoluteHumidity', NULL, 0, 1, 1, now(), now()),
(5260, 7, 100, 346, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '压力', 'zh', 0, 1, 1, now(), now()),
(5261, 7, 100, 346, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '压强总类：水系统/管网/冷媒压力。', 'zh', 0, 1, 1, now(), now()),
(5262, 7, 100, 346, 332, NULL, 'http://qudt.org/vocab/quantitykind/Pressure', NULL, 0, 1, 1, now(), now()),
(5263, 7, 100, 347, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '表压', 'zh', 0, 1, 1, now(), now()),
(5264, 7, 100, 347, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '以大气压为零点的表压力。', 'zh', 0, 1, 1, now(), now()),
(5265, 7, 100, 347, 332, NULL, 'http://qudt.org/vocab/quantitykind/GaugePressure', NULL, 0, 1, 1, now(), now()),
(5266, 7, 100, 348, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '静压', 'zh', 0, 1, 1, now(), now()),
(5267, 7, 100, 348, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '流体静压：风管静压、过滤器压差、正压送风压差。', 'zh', 0, 1, 1, now(), now()),
(5268, 7, 100, 348, 332, NULL, 'http://qudt.org/vocab/quantitykind/StaticPressure', NULL, 0, 1, 1, now(), now()),
(5269, 7, 100, 349, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '动压', 'zh', 0, 1, 1, now(), now()),
(5270, 7, 100, 349, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '流体动压（毕托管测速）。', 'zh', 0, 1, 1, now(), now()),
(5271, 7, 100, 349, 332, NULL, 'http://qudt.org/vocab/quantitykind/DynamicPressure', NULL, 0, 1, 1, now(), now()),
(5272, 7, 100, 350, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '大气压', 'zh', 0, 1, 1, now(), now()),
(5273, 7, 100, 350, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '室外大气压力，hPa。', 'zh', 0, 1, 1, now(), now()),
(5274, 7, 100, 350, 332, NULL, 'http://qudt.org/vocab/quantitykind/AtmosphericPressure', NULL, 0, 1, 1, now(), now()),
(5275, 7, 100, 351, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '体积流量', 'zh', 0, 1, 1, now(), now()),
(5276, 7, 100, 351, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '单位时间流过体积：风量/水量/消防流量。', 'zh', 0, 1, 1, now(), now()),
(5277, 7, 100, 351, 332, NULL, 'http://qudt.org/vocab/quantitykind/VolumeFlowRate', NULL, 0, 1, 1, now(), now()),
(5278, 7, 100, 352, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '质量流量', 'zh', 0, 1, 1, now(), now()),
(5279, 7, 100, 352, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '单位时间流过质量：蒸汽/冷冻水。', 'zh', 0, 1, 1, now(), now()),
(5280, 7, 100, 352, 332, NULL, 'http://qudt.org/vocab/quantitykind/MassFlowRate', NULL, 0, 1, 1, now(), now()),
(5281, 7, 100, 353, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '长度', 'zh', 0, 1, 1, now(), now()),
(5282, 7, 100, 353, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '距离/尺寸/能见度总类。', 'zh', 0, 1, 1, now(), now()),
(5283, 7, 100, 353, 332, NULL, 'http://qudt.org/vocab/quantitykind/Length', NULL, 0, 1, 1, now(), now()),
(5284, 7, 100, 354, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '液位', 'zh', 0, 1, 1, now(), now()),
(5285, 7, 100, 354, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '水箱/水池液位，QUDT 中为长度的特化。', 'zh', 0, 1, 1, now(), now()),
(5286, 7, 100, 354, 332, NULL, 'http://qudt.org/vocab/quantitykind/LiquidLevel', NULL, 0, 1, 1, now(), now()),
(5287, 7, 100, 355, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '体积', 'zh', 0, 1, 1, now(), now()),
(5288, 7, 100, 355, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '容积：累计流量、水池有效容积。', 'zh', 0, 1, 1, now(), now()),
(5289, 7, 100, 355, 332, NULL, 'http://qudt.org/vocab/quantitykind/Volume', NULL, 0, 1, 1, now(), now()),
(5290, 7, 100, 356, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '面积', 'zh', 0, 1, 1, now(), now()),
(5291, 7, 100, 356, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '建筑面积/房间面积等。', 'zh', 0, 1, 1, now(), now()),
(5292, 7, 100, 356, 332, NULL, 'http://qudt.org/vocab/quantitykind/Area', NULL, 0, 1, 1, now(), now()),
(5293, 7, 100, 357, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '速度', 'zh', 0, 1, 1, now(), now()),
(5294, 7, 100, 357, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '风速/水流速/电梯速度。', 'zh', 0, 1, 1, now(), now()),
(5295, 7, 100, 357, 332, NULL, 'http://qudt.org/vocab/quantitykind/Speed', NULL, 0, 1, 1, now(), now()),
(5296, 7, 100, 358, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '旋转频率', 'zh', 0, 1, 1, now(), now()),
(5297, 7, 100, 358, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '转速，r/min。', 'zh', 0, 1, 1, now(), now()),
(5298, 7, 100, 358, 332, NULL, 'http://qudt.org/vocab/quantitykind/RotationalFrequency', NULL, 0, 1, 1, now(), now()),
(5299, 7, 100, 359, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '平面角', 'zh', 0, 1, 1, now(), now()),
(5300, 7, 100, 359, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '风向/相角/阀门角度。', 'zh', 0, 1, 1, now(), now()),
(5301, 7, 100, 359, 332, NULL, 'http://qudt.org/vocab/quantitykind/PlaneAngle', NULL, 0, 1, 1, now(), now()),
(5302, 7, 100, 360, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '时间', 'zh', 0, 1, 1, now(), now()),
(5303, 7, 100, 360, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '时长/延时/周期。', 'zh', 0, 1, 1, now(), now()),
(5304, 7, 100, 360, 332, NULL, 'http://qudt.org/vocab/quantitykind/Time', NULL, 0, 1, 1, now(), now()),
(5305, 7, 100, 361, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '质量', 'zh', 0, 1, 1, now(), now()),
(5306, 7, 100, 361, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '重量/载重/药剂量。', 'zh', 0, 1, 1, now(), now()),
(5307, 7, 100, 361, 332, NULL, 'http://qudt.org/vocab/quantitykind/Mass', NULL, 0, 1, 1, now(), now()),
(5308, 7, 100, 362, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '电流', 'zh', 0, 1, 1, now(), now()),
(5309, 7, 100, 362, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '回路/总线电流。', 'zh', 0, 1, 1, now(), now()),
(5310, 7, 100, 362, 332, NULL, 'http://qudt.org/vocab/quantitykind/ElectricCurrent', NULL, 0, 1, 1, now(), now()),
(5311, 7, 100, 363, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '电压', 'zh', 0, 1, 1, now(), now()),
(5312, 7, 100, 363, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '交直流电压。', 'zh', 0, 1, 1, now(), now()),
(5313, 7, 100, 363, 332, NULL, 'http://qudt.org/vocab/quantitykind/Voltage', NULL, 0, 1, 1, now(), now()),
(5314, 7, 100, 364, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '频率', 'zh', 0, 1, 1, now(), now()),
(5315, 7, 100, 364, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '电网频率；换气次数(1/h)借用本量纲语义。', 'zh', 0, 1, 1, now(), now()),
(5316, 7, 100, 364, 332, NULL, 'http://qudt.org/vocab/quantitykind/Frequency', NULL, 0, 1, 1, now(), now()),
(5317, 7, 100, 365, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '有功功率', 'zh', 0, 1, 1, now(), now()),
(5318, 7, 100, 365, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '有功功率，kW。', 'zh', 0, 1, 1, now(), now()),
(5319, 7, 100, 365, 332, NULL, 'http://qudt.org/vocab/quantitykind/ElectricPower', NULL, 0, 1, 1, now(), now()),
(5320, 7, 100, 366, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '视在功率', 'zh', 0, 1, 1, now(), now()),
(5321, 7, 100, 366, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '视在容量，kV·A。', 'zh', 0, 1, 1, now(), now()),
(5322, 7, 100, 366, 332, NULL, 'http://qudt.org/vocab/quantitykind/ApparentPower', NULL, 0, 1, 1, now(), now()),
(5323, 7, 100, 367, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '无功功率', 'zh', 0, 1, 1, now(), now()),
(5324, 7, 100, 367, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '无功功率，kvar。', 'zh', 0, 1, 1, now(), now()),
(5325, 7, 100, 367, 332, NULL, 'http://qudt.org/vocab/quantitykind/ReactivePower', NULL, 0, 1, 1, now(), now()),
(5326, 7, 100, 368, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '有功电能', 'zh', 0, 1, 1, now(), now()),
(5327, 7, 100, 368, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '电度，kW·h。', 'zh', 0, 1, 1, now(), now()),
(5328, 7, 100, 368, 332, NULL, 'http://qudt.org/vocab/quantitykind/ElectricEnergy', NULL, 0, 1, 1, now(), now()),
(5329, 7, 100, 369, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '无功电能', 'zh', 0, 1, 1, now(), now()),
(5330, 7, 100, 369, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '无功电能，kvar·h。', 'zh', 0, 1, 1, now(), now()),
(5331, 7, 100, 369, 332, NULL, 'http://qudt.org/vocab/quantitykind/ReactiveEnergy', NULL, 0, 1, 1, now(), now()),
(5332, 7, 100, 370, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '视在电能', 'zh', 0, 1, 1, now(), now()),
(5333, 7, 100, 370, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '视在电能，kV·A·h。', 'zh', 0, 1, 1, now(), now()),
(5334, 7, 100, 370, 332, NULL, 'http://qudt.org/vocab/quantitykind/ApparentEnergy', NULL, 0, 1, 1, now(), now()),
(5335, 7, 100, 371, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '热能', 'zh', 0, 1, 1, now(), now()),
(5336, 7, 100, 371, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '热量/冷量结算，GJ/kcal。', 'zh', 0, 1, 1, now(), now()),
(5337, 7, 100, 371, 332, NULL, 'http://qudt.org/vocab/quantitykind/ThermalEnergy', NULL, 0, 1, 1, now(), now()),
(5338, 7, 100, 372, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '能量', 'zh', 0, 1, 1, now(), now()),
(5339, 7, 100, 372, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '能量总类。', 'zh', 0, 1, 1, now(), now()),
(5340, 7, 100, 372, 332, NULL, 'http://qudt.org/vocab/quantitykind/Energy', NULL, 0, 1, 1, now(), now()),
(5341, 7, 100, 373, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '功率', 'zh', 0, 1, 1, now(), now()),
(5342, 7, 100, 373, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '功率总类（热流量之上位）。', 'zh', 0, 1, 1, now(), now()),
(5343, 7, 100, 373, 332, NULL, 'http://qudt.org/vocab/quantitykind/Power', NULL, 0, 1, 1, now(), now()),
(5344, 7, 100, 374, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '热流量', 'zh', 0, 1, 1, now(), now()),
(5345, 7, 100, 374, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '单位时间热量：制冷量/制热量。', 'zh', 0, 1, 1, now(), now()),
(5346, 7, 100, 374, 332, NULL, 'http://qudt.org/vocab/quantitykind/HeatFlowRate', NULL, 0, 1, 1, now(), now()),
(5347, 7, 100, 375, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '电池容量', 'zh', 0, 1, 1, now(), now()),
(5348, 7, 100, 375, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '蓄电池容量，A·h。', 'zh', 0, 1, 1, now(), now()),
(5349, 7, 100, 375, 332, NULL, 'http://qudt.org/vocab/quantitykind/BatteryCapacity', NULL, 0, 1, 1, now(), now()),
(5350, 7, 100, 376, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '电阻', 'zh', 0, 1, 1, now(), now()),
(5351, 7, 100, 376, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '接地电阻/绝缘电阻。', 'zh', 0, 1, 1, now(), now()),
(5352, 7, 100, 376, 332, NULL, 'http://qudt.org/vocab/quantitykind/Resistance', NULL, 0, 1, 1, now(), now()),
(5353, 7, 100, 377, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '电导率', 'zh', 0, 1, 1, now(), now()),
(5354, 7, 100, 377, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '水质电导率。', 'zh', 0, 1, 1, now(), now()),
(5355, 7, 100, 377, 332, NULL, 'http://qudt.org/vocab/quantitykind/ElectricConductivity', NULL, 0, 1, 1, now(), now()),
(5356, 7, 100, 378, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '照度', 'zh', 0, 1, 1, now(), now()),
(5357, 7, 100, 378, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '照度，lx。', 'zh', 0, 1, 1, now(), now()),
(5358, 7, 100, 378, 332, NULL, 'http://qudt.org/vocab/quantitykind/Illuminance', NULL, 0, 1, 1, now(), now()),
(5359, 7, 100, 379, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '光通量', 'zh', 0, 1, 1, now(), now()),
(5360, 7, 100, 379, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '光通量，lm。', 'zh', 0, 1, 1, now(), now()),
(5361, 7, 100, 379, 332, NULL, 'http://qudt.org/vocab/quantitykind/LuminousFlux', NULL, 0, 1, 1, now(), now()),
(5362, 7, 100, 380, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '发光强度', 'zh', 0, 1, 1, now(), now()),
(5363, 7, 100, 380, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '发光强度，cd。', 'zh', 0, 1, 1, now(), now()),
(5364, 7, 100, 380, 332, NULL, 'http://qudt.org/vocab/quantitykind/LuminousIntensity', NULL, 0, 1, 1, now(), now()),
(5365, 7, 100, 381, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '光效', 'zh', 0, 1, 1, now(), now()),
(5366, 7, 100, 381, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '光效，lm/W。', 'zh', 0, 1, 1, now(), now()),
(5367, 7, 100, 381, 332, NULL, 'http://qudt.org/vocab/quantitykind/LuminousEfficacy', NULL, 0, 1, 1, now(), now()),
(5368, 7, 100, 382, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '质量浓度', 'zh', 0, 1, 1, now(), now()),
(5369, 7, 100, 382, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '颗粒物/水质等质量浓度。', 'zh', 0, 1, 1, now(), now()),
(5370, 7, 100, 382, 332, NULL, 'http://qudt.org/vocab/quantitykind/MassConcentration', NULL, 0, 1, 1, now(), now()),
(5371, 7, 100, 383, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '无量纲比', 'zh', 0, 1, 1, now(), now()),
(5372, 7, 100, 383, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '百分比/ppm 等比值总类。', 'zh', 0, 1, 1, now(), now()),
(5373, 7, 100, 383, 332, NULL, 'http://qudt.org/vocab/quantitykind/DimensionlessRatio', NULL, 0, 1, 1, now(), now()),
(5374, 7, 100, 384, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '体积分数', 'zh', 0, 1, 1, now(), now()),
(5375, 7, 100, 384, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '气体 %VOL/%LEL 语义。', 'zh', 0, 1, 1, now(), now()),
(5376, 7, 100, 384, 332, NULL, 'http://qudt.org/vocab/quantitykind/VolumeFraction', NULL, 0, 1, 1, now(), now()),
(5377, 7, 100, 385, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '摩尔分数', 'zh', 0, 1, 1, now(), now()),
(5378, 7, 100, 385, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', 'ppm 的摩尔比语义。', 'zh', 0, 1, 1, now(), now()),
(5379, 7, 100, 385, 332, NULL, 'http://qudt.org/vocab/quantitykind/MoleFraction', NULL, 0, 1, 1, now(), now()),
(5380, 7, 100, 386, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '衰减系数', 'zh', 0, 1, 1, now(), now()),
(5381, 7, 100, 386, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '感烟减光率（%/m、dB/m）。', 'zh', 0, 1, 1, now(), now()),
(5382, 7, 100, 386, 332, NULL, 'http://qudt.org/vocab/quantitykind/AttenuationCoefficient', NULL, 0, 1, 1, now(), now()),
(5383, 7, 100, 387, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '浊度', 'zh', 0, 1, 1, now(), now()),
(5384, 7, 100, 387, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '散射浊度，NTU。', 'zh', 0, 1, 1, now(), now()),
(5385, 7, 100, 387, 332, NULL, 'http://qudt.org/vocab/quantitykind/Turbidity', NULL, 0, 1, 1, now(), now()),
(5386, 7, 100, 388, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '声压级', 'zh', 0, 1, 1, now(), now()),
(5387, 7, 100, 388, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '噪声/消防广播声压级，dB。', 'zh', 0, 1, 1, now(), now()),
(5388, 7, 100, 388, 332, NULL, 'http://qudt.org/vocab/quantitykind/SoundPressureLevel', NULL, 0, 1, 1, now(), now()),
(5389, 7, 100, 389, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '计数', 'zh', 0, 1, 1, now(), now()),
(5390, 7, 100, 389, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '人数/次数等无量纲计数。', 'zh', 0, 1, 1, now(), now()),
(5391, 7, 100, 389, 332, NULL, 'http://qudt.org/vocab/quantitykind/Count', NULL, 0, 1, 1, now(), now()),
(5392, 7, 100, 390, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '扭矩', 'zh', 0, 1, 1, now(), now()),
(5393, 7, 100, 390, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '电动阀力矩，N·m。', 'zh', 0, 1, 1, now(), now()),
(5394, 7, 100, 390, 332, NULL, 'http://qudt.org/vocab/quantitykind/Torque', NULL, 0, 1, 1, now(), now()),
(5395, 7, 100, 391, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '辐照度', 'zh', 0, 1, 1, now(), now()),
(5396, 7, 100, 391, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '太阳辐照/热流密度，W/m²。', 'zh', 0, 1, 1, now(), now()),
(5397, 7, 100, 391, 332, NULL, 'http://qudt.org/vocab/quantitykind/Irradiance', NULL, 0, 1, 1, now(), now()),
(5398, 7, 100, 392, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '单位面积能量', 'zh', 0, 1, 1, now(), now()),
(5399, 7, 100, 392, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '能耗强度，kW·h/m²。', 'zh', 0, 1, 1, now(), now()),
(5400, 7, 100, 392, 332, NULL, 'http://qudt.org/vocab/quantitykind/EnergyPerArea', NULL, 0, 1, 1, now(), now()),
-- 类型断言(60)
(5401, 7, 60, 401, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5402, 7, 60, 402, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5403, 7, 60, 403, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5404, 7, 60, 404, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5405, 7, 60, 405, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5406, 7, 60, 406, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5407, 7, 60, 407, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5408, 7, 60, 408, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5409, 7, 60, 409, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5410, 7, 60, 410, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5411, 7, 60, 411, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5412, 7, 60, 412, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5413, 7, 60, 413, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5414, 7, 60, 414, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5415, 7, 60, 415, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5416, 7, 60, 416, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5417, 7, 60, 417, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5418, 7, 60, 418, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5419, 7, 60, 419, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5420, 7, 60, 420, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5421, 7, 60, 421, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5422, 7, 60, 422, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5423, 7, 60, 423, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5424, 7, 60, 424, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5425, 7, 60, 425, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5426, 7, 60, 426, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5427, 7, 60, 427, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5428, 7, 60, 428, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5429, 7, 60, 429, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5430, 7, 60, 430, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5431, 7, 60, 431, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5432, 7, 60, 432, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5433, 7, 60, 433, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5434, 7, 60, 434, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5435, 7, 60, 435, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5436, 7, 60, 436, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5437, 7, 60, 437, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5438, 7, 60, 438, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5439, 7, 60, 439, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5440, 7, 60, 440, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5441, 7, 60, 441, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5442, 7, 60, 442, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5443, 7, 60, 443, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5444, 7, 60, 444, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5445, 7, 60, 445, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5446, 7, 60, 446, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5447, 7, 60, 447, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5448, 7, 60, 448, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5449, 7, 60, 449, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5450, 7, 60, 450, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5451, 7, 60, 451, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5452, 7, 60, 452, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5453, 7, 60, 453, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5454, 7, 60, 454, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5455, 7, 60, 455, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5456, 7, 60, 456, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5457, 7, 60, 457, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5458, 7, 60, 458, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5459, 7, 60, 459, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5460, 7, 60, 460, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5461, 7, 60, 461, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5462, 7, 60, 462, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5463, 7, 60, 463, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5464, 7, 60, 464, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5465, 7, 60, 465, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5466, 7, 60, 466, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5467, 7, 60, 467, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5468, 7, 60, 468, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5469, 7, 60, 469, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5470, 7, 60, 470, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5471, 7, 60, 471, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5472, 7, 60, 472, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5473, 7, 60, 473, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5474, 7, 60, 474, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5475, 7, 60, 475, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5476, 7, 60, 476, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5477, 7, 60, 477, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5478, 7, 60, 478, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5479, 7, 60, 479, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5480, 7, 60, 480, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5481, 7, 60, 481, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5482, 7, 60, 482, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5483, 7, 60, 483, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5484, 7, 60, 484, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5485, 7, 60, 485, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5486, 7, 60, 486, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(5487, 7, 60, 487, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
-- 对象属性断言(61)
(5488, 7, 61, 401, 341, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- DEG_C → Temperature
(5489, 7, 61, 402, 342, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- K → TemperatureDifference
(5490, 7, 61, 402, 343, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 1, 1, 1, now(), now()),  -- K → CorrelatedColorTemperature
(5491, 7, 61, 403, 341, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- DEG_F → Temperature
(5492, 7, 61, 404, 344, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- PERCENT_RH → RelativeHumidity
(5493, 7, 61, 405, 345, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- GM-PER-M3 → AbsoluteHumidity
(5494, 7, 61, 406, 348, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- PA → StaticPressure
(5495, 7, 61, 406, 349, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 1, 1, 1, now(), now()),  -- PA → DynamicPressure
(5496, 7, 61, 407, 346, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloPA → Pressure
(5497, 7, 61, 408, 346, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MegaPA → Pressure
(5498, 7, 61, 409, 346, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- BAR → Pressure
(5499, 7, 61, 410, 346, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MilliM_H2O → Pressure
(5500, 7, 61, 411, 346, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- M_H2O → Pressure
(5501, 7, 61, 412, 346, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- IN_H2O → Pressure
(5502, 7, 61, 413, 346, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MilliM_HG → Pressure
(5503, 7, 61, 414, 346, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- PSI → Pressure
(5504, 7, 61, 415, 350, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- HectoPA → AtmosphericPressure
(5505, 7, 61, 416, 351, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- M3-PER-HR → VolumeFlowRate
(5506, 7, 61, 417, 351, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- M3-PER-SEC → VolumeFlowRate
(5507, 7, 61, 418, 351, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- L-PER-SEC → VolumeFlowRate
(5508, 7, 61, 419, 351, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- L-PER-MIN → VolumeFlowRate
(5509, 7, 61, 420, 355, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- M3 → Volume
(5510, 7, 61, 421, 357, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- M-PER-SEC → Speed
(5511, 7, 61, 422, 357, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloM-PER-HR → Speed
(5512, 7, 61, 423, 358, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- REV-PER-MIN → RotationalFrequency
(5513, 7, 61, 424, 352, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- TONNE-PER-HR → MassFlowRate
(5514, 7, 61, 425, 352, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloGM-PER-SEC → MassFlowRate
(5515, 7, 61, 426, 364, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- PER-HR → Frequency
(5516, 7, 61, 427, 383, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- PPM → DimensionlessRatio
(5517, 7, 61, 427, 385, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 1, 1, 1, now(), now()),  -- PPM → MoleFraction
(5518, 7, 61, 428, 383, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- PPB → DimensionlessRatio
(5519, 7, 61, 429, 383, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- PERCENT → DimensionlessRatio
(5520, 7, 61, 429, 384, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 1, 1, 1, now(), now()),  -- PERCENT → VolumeFraction
(5521, 7, 61, 430, 382, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MicroGM-PER-M3 → MassConcentration
(5522, 7, 61, 431, 382, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MilliGM-PER-M3 → MassConcentration
(5523, 7, 61, 432, 386, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- PERCENT-PER-M → AttenuationCoefficient
(5524, 7, 61, 433, 386, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- DeciB-PER-M → AttenuationCoefficient
(5525, 7, 61, 434, 387, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- NTU → Turbidity
(5526, 7, 61, 435, 382, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MilliGM-PER-L → MassConcentration
(5527, 7, 61, 436, 382, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- GM-PER-L → MassConcentration
(5528, 7, 61, 437, 377, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MicroS-PER-CentiM → ElectricConductivity
(5529, 7, 61, 438, 363, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- V → Voltage
(5530, 7, 61, 439, 363, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MilliV → Voltage
(5531, 7, 61, 440, 363, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloV → Voltage
(5532, 7, 61, 441, 362, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- A → ElectricCurrent
(5533, 7, 61, 442, 362, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MilliA → ElectricCurrent
(5534, 7, 61, 443, 373, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- W → Power
(5535, 7, 61, 444, 365, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloW → ElectricPower
(5536, 7, 61, 444, 374, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 1, 1, 1, now(), now()),  -- KiloW → HeatFlowRate
(5537, 7, 61, 445, 373, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MegaW → Power
(5538, 7, 61, 446, 366, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- V-A → ApparentPower
(5539, 7, 61, 447, 366, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloV-A → ApparentPower
(5540, 7, 61, 448, 367, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- VAR → ReactivePower
(5541, 7, 61, 449, 367, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloVAR → ReactivePower
(5542, 7, 61, 450, 368, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloW-HR → ElectricEnergy
(5543, 7, 61, 451, 368, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MegaW-HR → ElectricEnergy
(5544, 7, 61, 452, 369, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloVAR-HR → ReactiveEnergy
(5545, 7, 61, 453, 370, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloV-A-HR → ApparentEnergy
(5546, 7, 61, 454, 364, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- HZ → Frequency
(5547, 7, 61, 455, 376, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- OHM → Resistance
(5548, 7, 61, 456, 376, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MegaOHM → Resistance
(5549, 7, 61, 457, 375, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- A-HR → BatteryCapacity
(5550, 7, 61, 458, 391, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- W-PER-M2 → Irradiance
(5551, 7, 61, 459, 392, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloW-HR-PER-M2 → EnergyPerArea
(5552, 7, 61, 460, 378, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- LUX → Illuminance
(5553, 7, 61, 461, 379, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- LM → LuminousFlux
(5554, 7, 61, 462, 381, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- LM-PER-W → LuminousEfficacy
(5555, 7, 61, 463, 380, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- CD → LuminousIntensity
(5556, 7, 61, 464, 353, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- M → Length
(5557, 7, 61, 464, 354, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 1, 1, 1, now(), now()),  -- M → LiquidLevel
(5558, 7, 61, 465, 353, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MilliM → Length
(5559, 7, 61, 466, 353, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- CentiM → Length
(5560, 7, 61, 467, 361, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloGM → Mass
(5561, 7, 61, 468, 361, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- GM → Mass
(5562, 7, 61, 469, 361, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- TONNE → Mass
(5563, 7, 61, 470, 360, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- SEC → Time
(5564, 7, 61, 471, 360, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MIN → Time
(5565, 7, 61, 472, 360, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- HR → Time
(5566, 7, 61, 473, 360, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- DAY → Time
(5567, 7, 61, 474, 359, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- DEG → PlaneAngle
(5568, 7, 61, 475, 359, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- RAD → PlaneAngle
(5569, 7, 61, 476, 389, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- ONE → Count
(5570, 7, 61, 477, 388, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- DeciB → SoundPressureLevel
(5571, 7, 61, 478, 388, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- DeciB_A → SoundPressureLevel
(5572, 7, 61, 479, 355, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- L → Volume
(5573, 7, 61, 480, 355, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloL → Volume
(5574, 7, 61, 481, 372, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloJ → Energy
(5575, 7, 61, 482, 372, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- MegaJ → Energy
(5576, 7, 61, 483, 371, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- GigaJ → ThermalEnergy
(5577, 7, 61, 484, 371, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloCAL → ThermalEnergy
(5578, 7, 61, 485, 374, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- TON_FG → HeatFlowRate
(5579, 7, 61, 485, 373, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 1, 1, 1, now(), now()),  -- TON_FG → Power
(5580, 7, 61, 486, 374, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- BTU_IT-PER-HR → HeatFlowRate
(5581, 7, 61, 486, 373, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 1, 1, 1, now(), now()),  -- BTU_IT-PER-HR → Power
(5582, 7, 61, 487, 390, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- N-M → Torque
-- 数据属性断言(62)
(5583, 7, 62, 401, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-DEG_C', NULL, 0, 1, 1, now(), now()),  -- DEG_C 编码
(5584, 7, 62, 401, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- DEG_C multiplier
(5585, 7, 62, 401, 321, NULL, '273.15', NULL, 2, 1, 1, now(), now()),  -- DEG_C offset
(5586, 7, 62, 402, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-K', NULL, 0, 1, 1, now(), now()),  -- K 编码
(5587, 7, 62, 402, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- K multiplier
(5588, 7, 62, 403, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-DEG_F', NULL, 0, 1, 1, now(), now()),  -- DEG_F 编码
(5589, 7, 62, 403, 320, NULL, '0.5555555555555555555555555555555556', NULL, 1, 1, 1, now(), now()),  -- DEG_F multiplier
(5590, 7, 62, 403, 321, NULL, '459.67', NULL, 2, 1, 1, now(), now()),  -- DEG_F offset
(5591, 7, 62, 404, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-PERCENT_RH', NULL, 0, 1, 1, now(), now()),  -- PERCENT_RH 编码
(5592, 7, 62, 404, 320, NULL, '0.01', NULL, 1, 1, 1, now(), now()),  -- PERCENT_RH multiplier
(5593, 7, 62, 405, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-GM-PER-M3', NULL, 0, 1, 1, now(), now()),  -- GM-PER-M3 编码
(5594, 7, 62, 405, 320, NULL, '0.001', NULL, 1, 1, 1, now(), now()),  -- GM-PER-M3 multiplier
(5595, 7, 62, 406, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-PA', NULL, 0, 1, 1, now(), now()),  -- PA 编码
(5596, 7, 62, 406, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- PA multiplier
(5597, 7, 62, 407, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloPA', NULL, 0, 1, 1, now(), now()),  -- KiloPA 编码
(5598, 7, 62, 407, 320, NULL, '1000.0', NULL, 1, 1, 1, now(), now()),  -- KiloPA multiplier
(5599, 7, 62, 408, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MegaPA', NULL, 0, 1, 1, now(), now()),  -- MegaPA 编码
(5600, 7, 62, 408, 320, NULL, '1000000.0', NULL, 1, 1, 1, now(), now()),  -- MegaPA multiplier
(5601, 7, 62, 409, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-BAR', NULL, 0, 1, 1, now(), now()),  -- BAR 编码
(5602, 7, 62, 409, 320, NULL, '100000.0', NULL, 1, 1, 1, now(), now()),  -- BAR multiplier
(5603, 7, 62, 410, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MilliM_H2O', NULL, 0, 1, 1, now(), now()),  -- MilliM_H2O 编码
(5604, 7, 62, 410, 320, NULL, '9.80665', NULL, 1, 1, 1, now(), now()),  -- MilliM_H2O multiplier
(5605, 7, 62, 411, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-M_H2O', NULL, 0, 1, 1, now(), now()),  -- M_H2O 编码
(5606, 7, 62, 411, 320, NULL, '9806.65', NULL, 1, 1, 1, now(), now()),  -- M_H2O multiplier
(5607, 7, 62, 412, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-IN_H2O', NULL, 0, 1, 1, now(), now()),  -- IN_H2O 编码
(5608, 7, 62, 412, 320, NULL, '249.0889', NULL, 1, 1, 1, now(), now()),  -- IN_H2O multiplier
(5609, 7, 62, 413, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MilliM_HG', NULL, 0, 1, 1, now(), now()),  -- MilliM_HG 编码
(5610, 7, 62, 413, 320, NULL, '133.322387415', NULL, 1, 1, 1, now(), now()),  -- MilliM_HG multiplier
(5611, 7, 62, 414, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-PSI', NULL, 0, 1, 1, now(), now()),  -- PSI 编码
(5612, 7, 62, 414, 320, NULL, '6894.757293168361336722673445346887', NULL, 1, 1, 1, now(), now()),  -- PSI multiplier
(5613, 7, 62, 415, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-HectoPA', NULL, 0, 1, 1, now(), now()),  -- HectoPA 编码
(5614, 7, 62, 415, 320, NULL, '100.0', NULL, 1, 1, 1, now(), now()),  -- HectoPA multiplier
(5615, 7, 62, 416, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-M3-PER-HR', NULL, 0, 1, 1, now(), now()),  -- M3-PER-HR 编码
(5616, 7, 62, 416, 320, NULL, '0.0002777777777777777777777777777777778', NULL, 1, 1, 1, now(), now()),  -- M3-PER-HR multiplier
(5617, 7, 62, 417, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-M3-PER-SEC', NULL, 0, 1, 1, now(), now()),  -- M3-PER-SEC 编码
(5618, 7, 62, 417, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- M3-PER-SEC multiplier
(5619, 7, 62, 418, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-L-PER-SEC', NULL, 0, 1, 1, now(), now()),  -- L-PER-SEC 编码
(5620, 7, 62, 418, 320, NULL, '0.001', NULL, 1, 1, 1, now(), now()),  -- L-PER-SEC multiplier
(5621, 7, 62, 419, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-L-PER-MIN', NULL, 0, 1, 1, now(), now()),  -- L-PER-MIN 编码
(5622, 7, 62, 419, 320, NULL, '0.00001666666666666666666666666666666667', NULL, 1, 1, 1, now(), now()),  -- L-PER-MIN multiplier
(5623, 7, 62, 420, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-M3', NULL, 0, 1, 1, now(), now()),  -- M3 编码
(5624, 7, 62, 420, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- M3 multiplier
(5625, 7, 62, 421, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-M-PER-SEC', NULL, 0, 1, 1, now(), now()),  -- M-PER-SEC 编码
(5626, 7, 62, 421, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- M-PER-SEC multiplier
(5627, 7, 62, 422, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloM-PER-HR', NULL, 0, 1, 1, now(), now()),  -- KiloM-PER-HR 编码
(5628, 7, 62, 422, 320, NULL, '0.2777777777777777777777777777777778', NULL, 1, 1, 1, now(), now()),  -- KiloM-PER-HR multiplier
(5629, 7, 62, 423, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-REV-PER-MIN', NULL, 0, 1, 1, now(), now()),  -- REV-PER-MIN 编码
(5630, 7, 62, 423, 320, NULL, '0.1047197551196597746154214461093168', NULL, 1, 1, 1, now(), now()),  -- REV-PER-MIN multiplier
(5631, 7, 62, 424, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-TONNE-PER-HR', NULL, 0, 1, 1, now(), now()),  -- TONNE-PER-HR 编码
(5632, 7, 62, 424, 320, NULL, '0.2777777777777777777777777777777778', NULL, 1, 1, 1, now(), now()),  -- TONNE-PER-HR multiplier
(5633, 7, 62, 425, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloGM-PER-SEC', NULL, 0, 1, 1, now(), now()),  -- KiloGM-PER-SEC 编码
(5634, 7, 62, 425, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- KiloGM-PER-SEC multiplier
(5635, 7, 62, 426, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-PER-HR', NULL, 0, 1, 1, now(), now()),  -- PER-HR 编码
(5636, 7, 62, 426, 320, NULL, '0.0002777777777777777777777777777777778', NULL, 1, 1, 1, now(), now()),  -- PER-HR multiplier
(5637, 7, 62, 427, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-PPM', NULL, 0, 1, 1, now(), now()),  -- PPM 编码
(5638, 7, 62, 427, 320, NULL, '0.000001', NULL, 1, 1, 1, now(), now()),  -- PPM multiplier
(5639, 7, 62, 428, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-PPB', NULL, 0, 1, 1, now(), now()),  -- PPB 编码
(5640, 7, 62, 428, 320, NULL, '0.000000001', NULL, 1, 1, 1, now(), now()),  -- PPB multiplier
(5641, 7, 62, 429, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-PERCENT', NULL, 0, 1, 1, now(), now()),  -- PERCENT 编码
(5642, 7, 62, 429, 320, NULL, '0.01', NULL, 1, 1, 1, now(), now()),  -- PERCENT multiplier
(5643, 7, 62, 430, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MicroGM-PER-M3', NULL, 0, 1, 1, now(), now()),  -- MicroGM-PER-M3 编码
(5644, 7, 62, 430, 320, NULL, '0.000000001', NULL, 1, 1, 1, now(), now()),  -- MicroGM-PER-M3 multiplier
(5645, 7, 62, 431, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MilliGM-PER-M3', NULL, 0, 1, 1, now(), now()),  -- MilliGM-PER-M3 编码
(5646, 7, 62, 431, 320, NULL, '0.000001', NULL, 1, 1, 1, now(), now()),  -- MilliGM-PER-M3 multiplier
(5647, 7, 62, 432, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-PERCENT-PER-M', NULL, 0, 1, 1, now(), now()),  -- PERCENT-PER-M 编码
(5648, 7, 62, 432, 320, NULL, '0.01', NULL, 1, 1, 1, now(), now()),  -- PERCENT-PER-M multiplier
(5649, 7, 62, 433, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-DeciB-PER-M', NULL, 0, 1, 1, now(), now()),  -- DeciB-PER-M 编码
(5650, 7, 62, 433, 320, NULL, '0.0', NULL, 1, 1, 1, now(), now()),  -- DeciB-PER-M multiplier
(5651, 7, 62, 434, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-NTU', NULL, 0, 1, 1, now(), now()),  -- NTU 编码
(5652, 7, 62, 434, 320, NULL, '0.0', NULL, 1, 1, 1, now(), now()),  -- NTU multiplier
(5653, 7, 62, 435, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MilliGM-PER-L', NULL, 0, 1, 1, now(), now()),  -- MilliGM-PER-L 编码
(5654, 7, 62, 435, 320, NULL, '0.001', NULL, 1, 1, 1, now(), now()),  -- MilliGM-PER-L multiplier
(5655, 7, 62, 436, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-GM-PER-L', NULL, 0, 1, 1, now(), now()),  -- GM-PER-L 编码
(5656, 7, 62, 436, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- GM-PER-L multiplier
(5657, 7, 62, 437, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MicroS-PER-CentiM', NULL, 0, 1, 1, now(), now()),  -- MicroS-PER-CentiM 编码
(5658, 7, 62, 437, 320, NULL, '0.0001', NULL, 1, 1, 1, now(), now()),  -- MicroS-PER-CentiM multiplier
(5659, 7, 62, 438, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-V', NULL, 0, 1, 1, now(), now()),  -- V 编码
(5660, 7, 62, 438, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- V multiplier
(5661, 7, 62, 439, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MilliV', NULL, 0, 1, 1, now(), now()),  -- MilliV 编码
(5662, 7, 62, 439, 320, NULL, '0.001', NULL, 1, 1, 1, now(), now()),  -- MilliV multiplier
(5663, 7, 62, 440, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloV', NULL, 0, 1, 1, now(), now()),  -- KiloV 编码
(5664, 7, 62, 440, 320, NULL, '1000.0', NULL, 1, 1, 1, now(), now()),  -- KiloV multiplier
(5665, 7, 62, 441, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-A', NULL, 0, 1, 1, now(), now()),  -- A 编码
(5666, 7, 62, 441, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- A multiplier
(5667, 7, 62, 442, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MilliA', NULL, 0, 1, 1, now(), now()),  -- MilliA 编码
(5668, 7, 62, 442, 320, NULL, '0.001', NULL, 1, 1, 1, now(), now()),  -- MilliA multiplier
(5669, 7, 62, 443, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-W', NULL, 0, 1, 1, now(), now()),  -- W 编码
(5670, 7, 62, 443, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- W multiplier
(5671, 7, 62, 444, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloW', NULL, 0, 1, 1, now(), now()),  -- KiloW 编码
(5672, 7, 62, 444, 320, NULL, '1000.0', NULL, 1, 1, 1, now(), now()),  -- KiloW multiplier
(5673, 7, 62, 445, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MegaW', NULL, 0, 1, 1, now(), now()),  -- MegaW 编码
(5674, 7, 62, 445, 320, NULL, '1000000.0', NULL, 1, 1, 1, now(), now()),  -- MegaW multiplier
(5675, 7, 62, 446, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-V-A', NULL, 0, 1, 1, now(), now()),  -- V-A 编码
(5676, 7, 62, 446, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- V-A multiplier
(5677, 7, 62, 447, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloV-A', NULL, 0, 1, 1, now(), now()),  -- KiloV-A 编码
(5678, 7, 62, 447, 320, NULL, '1000.0', NULL, 1, 1, 1, now(), now()),  -- KiloV-A multiplier
(5679, 7, 62, 448, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-VAR', NULL, 0, 1, 1, now(), now()),  -- VAR 编码
(5680, 7, 62, 448, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- VAR multiplier
(5681, 7, 62, 449, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloVAR', NULL, 0, 1, 1, now(), now()),  -- KiloVAR 编码
(5682, 7, 62, 449, 320, NULL, '1000.0', NULL, 1, 1, 1, now(), now()),  -- KiloVAR multiplier
(5683, 7, 62, 450, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloW-HR', NULL, 0, 1, 1, now(), now()),  -- KiloW-HR 编码
(5684, 7, 62, 450, 320, NULL, '3600000.0', NULL, 1, 1, 1, now(), now()),  -- KiloW-HR multiplier
(5685, 7, 62, 451, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MegaW-HR', NULL, 0, 1, 1, now(), now()),  -- MegaW-HR 编码
(5686, 7, 62, 451, 320, NULL, '3600000000.0', NULL, 1, 1, 1, now(), now()),  -- MegaW-HR multiplier
(5687, 7, 62, 452, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloVAR-HR', NULL, 0, 1, 1, now(), now()),  -- KiloVAR-HR 编码
(5688, 7, 62, 452, 320, NULL, '3600000.0', NULL, 1, 1, 1, now(), now()),  -- KiloVAR-HR multiplier
(5689, 7, 62, 453, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloV-A-HR', NULL, 0, 1, 1, now(), now()),  -- KiloV-A-HR 编码
(5690, 7, 62, 453, 320, NULL, '3600000.0', NULL, 1, 1, 1, now(), now()),  -- KiloV-A-HR multiplier
(5691, 7, 62, 454, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-HZ', NULL, 0, 1, 1, now(), now()),  -- HZ 编码
(5692, 7, 62, 454, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- HZ multiplier
(5693, 7, 62, 455, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-OHM', NULL, 0, 1, 1, now(), now()),  -- OHM 编码
(5694, 7, 62, 455, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- OHM multiplier
(5695, 7, 62, 456, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MegaOHM', NULL, 0, 1, 1, now(), now()),  -- MegaOHM 编码
(5696, 7, 62, 456, 320, NULL, '1000000.0', NULL, 1, 1, 1, now(), now()),  -- MegaOHM multiplier
(5697, 7, 62, 457, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-A-HR', NULL, 0, 1, 1, now(), now()),  -- A-HR 编码
(5698, 7, 62, 457, 320, NULL, '3600.0', NULL, 1, 1, 1, now(), now()),  -- A-HR multiplier
(5699, 7, 62, 458, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-W-PER-M2', NULL, 0, 1, 1, now(), now()),  -- W-PER-M2 编码
(5700, 7, 62, 458, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- W-PER-M2 multiplier
(5701, 7, 62, 459, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloW-HR-PER-M2', NULL, 0, 1, 1, now(), now()),  -- KiloW-HR-PER-M2 编码
(5702, 7, 62, 459, 320, NULL, '3600000.0', NULL, 1, 1, 1, now(), now()),  -- KiloW-HR-PER-M2 multiplier
(5703, 7, 62, 460, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-LUX', NULL, 0, 1, 1, now(), now()),  -- LUX 编码
(5704, 7, 62, 460, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- LUX multiplier
(5705, 7, 62, 461, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-LM', NULL, 0, 1, 1, now(), now()),  -- LM 编码
(5706, 7, 62, 461, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- LM multiplier
(5707, 7, 62, 462, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-LM-PER-W', NULL, 0, 1, 1, now(), now()),  -- LM-PER-W 编码
(5708, 7, 62, 462, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- LM-PER-W multiplier
(5709, 7, 62, 463, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-CD', NULL, 0, 1, 1, now(), now()),  -- CD 编码
(5710, 7, 62, 463, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- CD multiplier
(5711, 7, 62, 464, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-M', NULL, 0, 1, 1, now(), now()),  -- M 编码
(5712, 7, 62, 464, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- M multiplier
(5713, 7, 62, 465, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MilliM', NULL, 0, 1, 1, now(), now()),  -- MilliM 编码
(5714, 7, 62, 465, 320, NULL, '0.001', NULL, 1, 1, 1, now(), now()),  -- MilliM multiplier
(5715, 7, 62, 466, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-CentiM', NULL, 0, 1, 1, now(), now()),  -- CentiM 编码
(5716, 7, 62, 466, 320, NULL, '0.01', NULL, 1, 1, 1, now(), now()),  -- CentiM multiplier
(5717, 7, 62, 467, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloGM', NULL, 0, 1, 1, now(), now()),  -- KiloGM 编码
(5718, 7, 62, 467, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- KiloGM multiplier
(5719, 7, 62, 468, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-GM', NULL, 0, 1, 1, now(), now()),  -- GM 编码
(5720, 7, 62, 468, 320, NULL, '0.001', NULL, 1, 1, 1, now(), now()),  -- GM multiplier
(5721, 7, 62, 469, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-TONNE', NULL, 0, 1, 1, now(), now()),  -- TONNE 编码
(5722, 7, 62, 469, 320, NULL, '1000.0', NULL, 1, 1, 1, now(), now()),  -- TONNE multiplier
(5723, 7, 62, 470, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-SEC', NULL, 0, 1, 1, now(), now()),  -- SEC 编码
(5724, 7, 62, 470, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- SEC multiplier
(5725, 7, 62, 471, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MIN', NULL, 0, 1, 1, now(), now()),  -- MIN 编码
(5726, 7, 62, 471, 320, NULL, '60.0', NULL, 1, 1, 1, now(), now()),  -- MIN multiplier
(5727, 7, 62, 472, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-HR', NULL, 0, 1, 1, now(), now()),  -- HR 编码
(5728, 7, 62, 472, 320, NULL, '3600.0', NULL, 1, 1, 1, now(), now()),  -- HR multiplier
(5729, 7, 62, 473, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-DAY', NULL, 0, 1, 1, now(), now()),  -- DAY 编码
(5730, 7, 62, 473, 320, NULL, '86400.0', NULL, 1, 1, 1, now(), now()),  -- DAY multiplier
(5731, 7, 62, 474, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-DEG', NULL, 0, 1, 1, now(), now()),  -- DEG 编码
(5732, 7, 62, 474, 320, NULL, '0.01745329251994329576923690768488613', NULL, 1, 1, 1, now(), now()),  -- DEG multiplier
(5733, 7, 62, 475, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-RAD', NULL, 0, 1, 1, now(), now()),  -- RAD 编码
(5734, 7, 62, 475, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- RAD multiplier
(5735, 7, 62, 476, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-ONE', NULL, 0, 1, 1, now(), now()),  -- ONE 编码
(5736, 7, 62, 476, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- ONE multiplier
(5737, 7, 62, 477, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-DeciB', NULL, 0, 1, 1, now(), now()),  -- DeciB 编码
(5738, 7, 62, 477, 320, NULL, '0.0', NULL, 1, 1, 1, now(), now()),  -- DeciB multiplier
(5739, 7, 62, 478, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-DeciB_A', NULL, 0, 1, 1, now(), now()),  -- DeciB_A 编码
(5740, 7, 62, 478, 320, NULL, '0.0', NULL, 1, 1, 1, now(), now()),  -- DeciB_A multiplier
(5741, 7, 62, 479, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-L', NULL, 0, 1, 1, now(), now()),  -- L 编码
(5742, 7, 62, 479, 320, NULL, '0.001', NULL, 1, 1, 1, now(), now()),  -- L multiplier
(5743, 7, 62, 480, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloL', NULL, 0, 1, 1, now(), now()),  -- KiloL 编码
(5744, 7, 62, 480, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- KiloL multiplier
(5745, 7, 62, 481, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloJ', NULL, 0, 1, 1, now(), now()),  -- KiloJ 编码
(5746, 7, 62, 481, 320, NULL, '1000.0', NULL, 1, 1, 1, now(), now()),  -- KiloJ multiplier
(5747, 7, 62, 482, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-MegaJ', NULL, 0, 1, 1, now(), now()),  -- MegaJ 编码
(5748, 7, 62, 482, 320, NULL, '1000000.0', NULL, 1, 1, 1, now(), now()),  -- MegaJ multiplier
(5749, 7, 62, 483, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-GigaJ', NULL, 0, 1, 1, now(), now()),  -- GigaJ 编码
(5750, 7, 62, 483, 320, NULL, '1000000000.0', NULL, 1, 1, 1, now(), now()),  -- GigaJ multiplier
(5751, 7, 62, 484, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-KiloCAL', NULL, 0, 1, 1, now(), now()),  -- KiloCAL 编码
(5752, 7, 62, 484, 320, NULL, '4184.0', NULL, 1, 1, 1, now(), now()),  -- KiloCAL multiplier
(5753, 7, 62, 485, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-TON_FG', NULL, 0, 1, 1, now(), now()),  -- TON_FG 编码
(5754, 7, 62, 485, 320, NULL, '3516.853', NULL, 1, 1, 1, now(), now()),  -- TON_FG multiplier
(5755, 7, 62, 486, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-BTU_IT-PER-HR', NULL, 0, 1, 1, now(), now()),  -- BTU_IT-PER-HR 编码
(5756, 7, 62, 486, 320, NULL, '0.2930710701722222222222222222222222', NULL, 1, 1, 1, now(), now()),  -- BTU_IT-PER-HR multiplier
(5757, 7, 62, 487, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-N-M', NULL, 0, 1, 1, now(), now()),  -- N-M 编码
(5758, 7, 62, 487, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- N-M multiplier
-- 注解断言(100)
(5759, 7, 100, 401, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '摄氏度', 'zh', 0, 1, 1, now(), now()),
(5760, 7, 100, 401, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '温度测点主用单位；SI 换算偏移 273.15，作差量单位时改用 K。', 'zh', 0, 1, 1, now(), now()),
(5761, 7, 100, 401, 332, NULL, 'http://qudt.org/vocab/unit/DEG_C', NULL, 0, 1, 1, now(), now()),
(5762, 7, 100, 401, 330, NULL, '°C', NULL, 0, 1, 1, now(), now()),
(5763, 7, 100, 401, 331, NULL, 'Cel', NULL, 0, 1, 1, now(), now()),
(5764, 7, 100, 402, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '开尔文', 'zh', 0, 1, 1, now(), now()),
(5765, 7, 100, 402, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '热力学温度 SI 单位；供回水温差等差分量与 LED 色温亦用本单位。', 'zh', 0, 1, 1, now(), now()),
(5766, 7, 100, 402, 332, NULL, 'http://qudt.org/vocab/unit/K', NULL, 0, 1, 1, now(), now()),
(5767, 7, 100, 402, 330, NULL, 'K', NULL, 0, 1, 1, now(), now()),
(5768, 7, 100, 402, 331, NULL, 'K', NULL, 0, 1, 1, now(), now()),
(5769, 7, 100, 403, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '华氏度', 'zh', 0, 1, 1, now(), now()),
(5770, 7, 100, 403, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '进口机组面板显示兼容。', 'zh', 0, 1, 1, now(), now()),
(5771, 7, 100, 403, 332, NULL, 'http://qudt.org/vocab/unit/DEG_F', NULL, 0, 1, 1, now(), now()),
(5772, 7, 100, 403, 330, NULL, '°F', NULL, 0, 1, 1, now(), now()),
(5773, 7, 100, 403, 331, NULL, '[degF]', NULL, 0, 1, 1, now(), now()),
(5774, 7, 100, 404, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '相对湿度（%RH）', 'zh', 0, 1, 1, now(), now()),
(5775, 7, 100, 404, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '相对湿度专用单位，与普通百分比区分。', 'zh', 0, 1, 1, now(), now()),
(5776, 7, 100, 404, 332, NULL, 'http://qudt.org/vocab/unit/PERCENT_RH', NULL, 0, 1, 1, now(), now()),
(5777, 7, 100, 404, 330, NULL, '%RH', NULL, 0, 1, 1, now(), now()),
(5778, 7, 100, 405, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '克每立方米', 'zh', 0, 1, 1, now(), now()),
(5779, 7, 100, 405, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '绝对湿度/含湿量。', 'zh', 0, 1, 1, now(), now()),
(5780, 7, 100, 405, 332, NULL, 'http://qudt.org/vocab/unit/GM-PER-M3', NULL, 0, 1, 1, now(), now()),
(5781, 7, 100, 405, 330, NULL, 'g/m³', NULL, 0, 1, 1, now(), now()),
(5782, 7, 100, 405, 331, NULL, 'g.m-3', NULL, 0, 1, 1, now(), now()),
(5783, 7, 100, 406, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '帕斯卡', 'zh', 0, 1, 1, now(), now()),
(5784, 7, 100, 406, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '压差/静压/动压主用单位：过滤器压差、风管静压、正压送风压差（前室 25~30Pa）。', 'zh', 0, 1, 1, now(), now()),
(5785, 7, 100, 406, 332, NULL, 'http://qudt.org/vocab/unit/PA', NULL, 0, 1, 1, now(), now()),
(5786, 7, 100, 406, 330, NULL, 'Pa', NULL, 0, 1, 1, now(), now()),
(5787, 7, 100, 406, 331, NULL, 'Pa', NULL, 0, 1, 1, now(), now()),
(5788, 7, 100, 407, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千帕', 'zh', 0, 1, 1, now(), now()),
(5789, 7, 100, 407, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '空调水系统压力、稳压泵启停定值、末端试水压力。', 'zh', 0, 1, 1, now(), now()),
(5790, 7, 100, 407, 332, NULL, 'http://qudt.org/vocab/unit/KiloPA', NULL, 0, 1, 1, now(), now()),
(5791, 7, 100, 407, 330, NULL, 'kPa', NULL, 0, 1, 1, now(), now()),
(5792, 7, 100, 407, 331, NULL, 'kPa', NULL, 0, 1, 1, now(), now()),
(5793, 7, 100, 408, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '兆帕', 'zh', 0, 1, 1, now(), now()),
(5794, 7, 100, 408, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '高压管网：消防泵出口（0.8~1.6MPa）、气体灭火瓶组（15/20MPa）、蒸汽。', 'zh', 0, 1, 1, now(), now()),
(5795, 7, 100, 408, 332, NULL, 'http://qudt.org/vocab/unit/MegaPA', NULL, 0, 1, 1, now(), now()),
(5796, 7, 100, 408, 330, NULL, 'MPa', NULL, 0, 1, 1, now(), now()),
(5797, 7, 100, 408, 331, NULL, 'MPa', NULL, 0, 1, 1, now(), now()),
(5798, 7, 100, 409, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '巴', 'zh', 0, 1, 1, now(), now()),
(5799, 7, 100, 409, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '制冷机组冷媒压力表惯用单位。', 'zh', 0, 1, 1, now(), now()),
(5800, 7, 100, 409, 332, NULL, 'http://qudt.org/vocab/unit/BAR', NULL, 0, 1, 1, now(), now()),
(5801, 7, 100, 409, 330, NULL, 'bar', NULL, 0, 1, 1, now(), now()),
(5802, 7, 100, 409, 331, NULL, 'bar', NULL, 0, 1, 1, now(), now()),
(5803, 7, 100, 410, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '毫米水柱', 'zh', 0, 1, 1, now(), now()),
(5804, 7, 100, 410, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '老规范风机压头/炉膛负压表达（国内图纸遗留）。', 'zh', 0, 1, 1, now(), now()),
(5805, 7, 100, 410, 332, NULL, 'http://qudt.org/vocab/unit/MilliM_H2O', NULL, 0, 1, 1, now(), now()),
(5806, 7, 100, 410, 330, NULL, 'mmH₂0', NULL, 0, 1, 1, now(), now()),
(5807, 7, 100, 410, 331, NULL, 'mm[H2O]', NULL, 0, 1, 1, now(), now()),
(5808, 7, 100, 411, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '米水柱', 'zh', 0, 1, 1, now(), now()),
(5809, 7, 100, 411, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '水泵扬程惯用表达。', 'zh', 0, 1, 1, now(), now()),
(5810, 7, 100, 411, 332, NULL, 'http://qudt.org/vocab/unit/M_H2O', NULL, 0, 1, 1, now(), now()),
(5811, 7, 100, 411, 330, NULL, 'mH₂0', NULL, 0, 1, 1, now(), now()),
(5812, 7, 100, 412, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '英寸水柱', 'zh', 0, 1, 1, now(), now()),
(5813, 7, 100, 412, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '进口差压变送器。', 'zh', 0, 1, 1, now(), now()),
(5814, 7, 100, 412, 332, NULL, 'http://qudt.org/vocab/unit/IN_H2O', NULL, 0, 1, 1, now(), now()),
(5815, 7, 100, 412, 330, NULL, 'inH₂O', NULL, 0, 1, 1, now(), now()),
(5816, 7, 100, 412, 331, NULL, '[in_i''H2O]', NULL, 0, 1, 1, now(), now()),
(5817, 7, 100, 413, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '毫米汞柱', 'zh', 0, 1, 1, now(), now()),
(5818, 7, 100, 413, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '医用真空/大气压惯用表达。', 'zh', 0, 1, 1, now(), now()),
(5819, 7, 100, 413, 332, NULL, 'http://qudt.org/vocab/unit/MilliM_HG', NULL, 0, 1, 1, now(), now()),
(5820, 7, 100, 413, 330, NULL, 'mmHg', NULL, 0, 1, 1, now(), now()),
(5821, 7, 100, 413, 331, NULL, 'mm[Hg]', NULL, 0, 1, 1, now(), now()),
(5822, 7, 100, 414, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '磅力每平方英寸', 'zh', 0, 1, 1, now(), now()),
(5823, 7, 100, 414, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '进口设备压力。', 'zh', 0, 1, 1, now(), now()),
(5824, 7, 100, 414, 332, NULL, 'http://qudt.org/vocab/unit/PSI', NULL, 0, 1, 1, now(), now()),
(5825, 7, 100, 414, 330, NULL, 'psi', NULL, 0, 1, 1, now(), now()),
(5826, 7, 100, 414, 331, NULL, '[psi]', NULL, 0, 1, 1, now(), now()),
(5827, 7, 100, 415, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '百帕', 'zh', 0, 1, 1, now(), now()),
(5828, 7, 100, 415, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '室外气象站大气压。', 'zh', 0, 1, 1, now(), now()),
(5829, 7, 100, 415, 332, NULL, 'http://qudt.org/vocab/unit/HectoPA', NULL, 0, 1, 1, now(), now()),
(5830, 7, 100, 415, 330, NULL, 'hPa', NULL, 0, 1, 1, now(), now()),
(5831, 7, 100, 415, 331, NULL, 'hPa', NULL, 0, 1, 1, now(), now()),
(5832, 7, 100, 416, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '立方米每小时', 'zh', 0, 1, 1, now(), now()),
(5833, 7, 100, 416, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '风量/水流量主用单位：空调机组风量、排烟风量、水流量、新风量。', 'zh', 0, 1, 1, now(), now()),
(5834, 7, 100, 416, 332, NULL, 'http://qudt.org/vocab/unit/M3-PER-HR', NULL, 0, 1, 1, now(), now()),
(5835, 7, 100, 416, 330, NULL, 'm³/h', NULL, 0, 1, 1, now(), now()),
(5836, 7, 100, 416, 331, NULL, 'm3.h-1', NULL, 0, 1, 1, now(), now()),
(5837, 7, 100, 417, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '立方米每秒', 'zh', 0, 1, 1, now(), now()),
(5838, 7, 100, 417, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '大风量风机风量。', 'zh', 0, 1, 1, now(), now()),
(5839, 7, 100, 417, 332, NULL, 'http://qudt.org/vocab/unit/M3-PER-SEC', NULL, 0, 1, 1, now(), now()),
(5840, 7, 100, 417, 330, NULL, 'm³/s', NULL, 0, 1, 1, now(), now()),
(5841, 7, 100, 417, 331, NULL, 'm3.s-1', NULL, 0, 1, 1, now(), now()),
(5842, 7, 100, 418, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '升每秒', 'zh', 0, 1, 1, now(), now()),
(5843, 7, 100, 418, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '消防泵/消火栓流量（40L/s 级）、小口径水管流量。', 'zh', 0, 1, 1, now(), now()),
(5844, 7, 100, 418, 332, NULL, 'http://qudt.org/vocab/unit/L-PER-SEC', NULL, 0, 1, 1, now(), now()),
(5845, 7, 100, 418, 330, NULL, 'L/s', NULL, 0, 1, 1, now(), now()),
(5846, 7, 100, 418, 331, NULL, 'L.s-1', NULL, 0, 1, 1, now(), now()),
(5847, 7, 100, 419, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '升每分钟', 'zh', 0, 1, 1, now(), now()),
(5848, 7, 100, 419, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '医用气体、小流量计量。', 'zh', 0, 1, 1, now(), now()),
(5849, 7, 100, 419, 332, NULL, 'http://qudt.org/vocab/unit/L-PER-MIN', NULL, 0, 1, 1, now(), now()),
(5850, 7, 100, 419, 330, NULL, 'L/min', NULL, 0, 1, 1, now(), now()),
(5851, 7, 100, 419, 331, NULL, 'L.min-1', NULL, 0, 1, 1, now(), now()),
(5852, 7, 100, 420, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '立方米', 'zh', 0, 1, 1, now(), now()),
(5853, 7, 100, 420, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '累计流量（水/气/热表）与水池有效容积。', 'zh', 0, 1, 1, now(), now()),
(5854, 7, 100, 420, 332, NULL, 'http://qudt.org/vocab/unit/M3', NULL, 0, 1, 1, now(), now()),
(5855, 7, 100, 420, 330, NULL, 'm³', NULL, 0, 1, 1, now(), now()),
(5856, 7, 100, 420, 331, NULL, 'm3', NULL, 0, 1, 1, now(), now()),
(5857, 7, 100, 421, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '米每秒', 'zh', 0, 1, 1, now(), now()),
(5858, 7, 100, 421, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '风速、水流速、电梯速度；排烟口风速限值 10m/s。', 'zh', 0, 1, 1, now(), now()),
(5859, 7, 100, 421, 332, NULL, 'http://qudt.org/vocab/unit/M-PER-SEC', NULL, 0, 1, 1, now(), now()),
(5860, 7, 100, 421, 330, NULL, 'm/s', NULL, 0, 1, 1, now(), now()),
(5861, 7, 100, 421, 331, NULL, 'm.s-1', NULL, 0, 1, 1, now(), now()),
(5862, 7, 100, 422, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千米每小时', 'zh', 0, 1, 1, now(), now()),
(5863, 7, 100, 422, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '室外风速的气象表达。', 'zh', 0, 1, 1, now(), now()),
(5864, 7, 100, 422, 332, NULL, 'http://qudt.org/vocab/unit/KiloM-PER-HR', NULL, 0, 1, 1, now(), now()),
(5865, 7, 100, 422, 330, NULL, 'km/h', NULL, 0, 1, 1, now(), now()),
(5866, 7, 100, 422, 331, NULL, 'km.h-1', NULL, 0, 1, 1, now(), now()),
(5867, 7, 100, 423, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '转每分钟', 'zh', 0, 1, 1, now(), now()),
(5868, 7, 100, 423, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '风机/水泵/电机转速。', 'zh', 0, 1, 1, now(), now()),
(5869, 7, 100, 423, 332, NULL, 'http://qudt.org/vocab/unit/REV-PER-MIN', NULL, 0, 1, 1, now(), now()),
(5870, 7, 100, 423, 330, NULL, 'rev/min', NULL, 0, 1, 1, now(), now()),
(5871, 7, 100, 423, 331, NULL, '{#}.min-1', NULL, 0, 1, 1, now(), now()),
(5872, 7, 100, 424, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '吨每小时', 'zh', 0, 1, 1, now(), now()),
(5873, 7, 100, 424, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '蒸汽流量（锅炉房）。', 'zh', 0, 1, 1, now(), now()),
(5874, 7, 100, 424, 332, NULL, 'http://qudt.org/vocab/unit/TONNE-PER-HR', NULL, 0, 1, 1, now(), now()),
(5875, 7, 100, 424, 330, NULL, 't/h', NULL, 0, 1, 1, now(), now()),
(5876, 7, 100, 424, 331, NULL, 't.h-1', NULL, 0, 1, 1, now(), now()),
(5877, 7, 100, 425, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千克每秒', 'zh', 0, 1, 1, now(), now()),
(5878, 7, 100, 425, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '冷冻水质量流量（冷机性能）。', 'zh', 0, 1, 1, now(), now()),
(5879, 7, 100, 425, 332, NULL, 'http://qudt.org/vocab/unit/KiloGM-PER-SEC', NULL, 0, 1, 1, now(), now()),
(5880, 7, 100, 425, 330, NULL, 'kg/s', NULL, 0, 1, 1, now(), now()),
(5881, 7, 100, 425, 331, NULL, 'kg.s-1', NULL, 0, 1, 1, now(), now()),
(5882, 7, 100, 426, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '每小时', 'zh', 0, 1, 1, now(), now()),
(5883, 7, 100, 426, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '换气次数 ACH（1/h）；QUDT 无专用量纲，挂频率语义（见分析文档缺口说明）。', 'zh', 0, 1, 1, now(), now()),
(5884, 7, 100, 426, 332, NULL, 'http://qudt.org/vocab/unit/PER-HR', NULL, 0, 1, 1, now(), now()),
(5885, 7, 100, 426, 330, NULL, '/h', NULL, 0, 1, 1, now(), now()),
(5886, 7, 100, 426, 331, NULL, 'h-1', NULL, 0, 1, 1, now(), now()),
(5887, 7, 100, 427, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '百万分率', 'zh', 0, 1, 1, now(), now()),
(5888, 7, 100, 427, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '气体浓度主用单位：CO2/CO/TVOC/可燃气体。', 'zh', 0, 1, 1, now(), now()),
(5889, 7, 100, 427, 332, NULL, 'http://qudt.org/vocab/unit/PPM', NULL, 0, 1, 1, now(), now()),
(5890, 7, 100, 427, 330, NULL, 'PPM', NULL, 0, 1, 1, now(), now()),
(5891, 7, 100, 427, 331, NULL, '[ppm]', NULL, 0, 1, 1, now(), now()),
(5892, 7, 100, 428, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '十亿分率', 'zh', 0, 1, 1, now(), now()),
(5893, 7, 100, 428, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '高精度气体传感器。', 'zh', 0, 1, 1, now(), now()),
(5894, 7, 100, 428, 332, NULL, 'http://qudt.org/vocab/unit/PPB', NULL, 0, 1, 1, now(), now()),
(5895, 7, 100, 428, 330, NULL, 'PPB', NULL, 0, 1, 1, now(), now()),
(5896, 7, 100, 428, 331, NULL, '[ppb]', NULL, 0, 1, 1, now(), now()),
(5897, 7, 100, 429, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '百分比', 'zh', 0, 1, 1, now(), now()),
(5898, 7, 100, 429, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '氧气浓度、可燃气体 %LEL、药剂 %VOL、开度/载荷/占比等通用无量纲量。', 'zh', 0, 1, 1, now(), now()),
(5899, 7, 100, 429, 332, NULL, 'http://qudt.org/vocab/unit/PERCENT', NULL, 0, 1, 1, now(), now()),
(5900, 7, 100, 429, 330, NULL, '%', NULL, 0, 1, 1, now(), now()),
(5901, 7, 100, 429, 331, NULL, '%', NULL, 0, 1, 1, now(), now()),
(5902, 7, 100, 430, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '微克每立方米', 'zh', 0, 1, 1, now(), now()),
(5903, 7, 100, 430, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', 'PM2.5/PM10 颗粒物浓度。', 'zh', 0, 1, 1, now(), now()),
(5904, 7, 100, 430, 332, NULL, 'http://qudt.org/vocab/unit/MicroGM-PER-M3', NULL, 0, 1, 1, now(), now()),
(5905, 7, 100, 430, 330, NULL, 'μg/m³', NULL, 0, 1, 1, now(), now()),
(5906, 7, 100, 430, 331, NULL, 'ug.m-3', NULL, 0, 1, 1, now(), now()),
(5907, 7, 100, 431, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '毫克每立方米', 'zh', 0, 1, 1, now(), now()),
(5908, 7, 100, 431, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', 'TVOC、甲醛浓度（0.1mg/m³ 国标限值）。', 'zh', 0, 1, 1, now(), now()),
(5909, 7, 100, 431, 332, NULL, 'http://qudt.org/vocab/unit/MilliGM-PER-M3', NULL, 0, 1, 1, now(), now()),
(5910, 7, 100, 431, 330, NULL, 'mg/m³', NULL, 0, 1, 1, now(), now()),
(5911, 7, 100, 431, 331, NULL, 'mg.m-3', NULL, 0, 1, 1, now(), now()),
(5912, 7, 100, 432, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '百分比每米', 'zh', 0, 1, 1, now(), now()),
(5913, 7, 100, 432, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '光电感烟探测器减光率（GB 4715 遮光率 0.1~1.0%/m）。', 'zh', 0, 1, 1, now(), now()),
(5914, 7, 100, 432, 332, NULL, 'http://qudt.org/vocab/unit/PERCENT-PER-M', NULL, 0, 1, 1, now(), now()),
(5915, 7, 100, 432, 330, NULL, '%/m', NULL, 0, 1, 1, now(), now()),
(5916, 7, 100, 432, 331, NULL, '%.m-1', NULL, 0, 1, 1, now(), now()),
(5917, 7, 100, 433, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '分贝每米', 'zh', 0, 1, 1, now(), now()),
(5918, 7, 100, 433, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '减光率的分贝表达，与 %/m 并存。', 'zh', 0, 1, 1, now(), now()),
(5919, 7, 100, 433, 332, NULL, 'http://qudt.org/vocab/unit/DeciB-PER-M', NULL, 0, 1, 1, now(), now()),
(5920, 7, 100, 433, 330, NULL, 'dB/m', NULL, 0, 1, 1, now(), now()),
(5921, 7, 100, 433, 331, NULL, 'dB.m-1', NULL, 0, 1, 1, now(), now()),
(5922, 7, 100, 434, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '浊度（NTU）', 'zh', 0, 1, 1, now(), now()),
(5923, 7, 100, 434, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '散射浊度单位，二次供水水质。', 'zh', 0, 1, 1, now(), now()),
(5924, 7, 100, 434, 332, NULL, 'http://qudt.org/vocab/unit/NTU', NULL, 0, 1, 1, now(), now()),
(5925, 7, 100, 434, 330, NULL, 'NTU', NULL, 0, 1, 1, now(), now()),
(5926, 7, 100, 435, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '毫克每升', 'zh', 0, 1, 1, now(), now()),
(5927, 7, 100, 435, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '水质浓度：余氯（0.3~0.5mg/L）等。', 'zh', 0, 1, 1, now(), now()),
(5928, 7, 100, 435, 332, NULL, 'http://qudt.org/vocab/unit/MilliGM-PER-L', NULL, 0, 1, 1, now(), now()),
(5929, 7, 100, 435, 330, NULL, 'mg/L', NULL, 0, 1, 1, now(), now()),
(5930, 7, 100, 435, 331, NULL, 'mg.L-1', NULL, 0, 1, 1, now(), now()),
(5931, 7, 100, 436, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '克每升', 'zh', 0, 1, 1, now(), now()),
(5932, 7, 100, 436, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '高浓度水质指标。', 'zh', 0, 1, 1, now(), now()),
(5933, 7, 100, 436, 332, NULL, 'http://qudt.org/vocab/unit/GM-PER-L', NULL, 0, 1, 1, now(), now()),
(5934, 7, 100, 436, 330, NULL, 'g/L', NULL, 0, 1, 1, now(), now()),
(5935, 7, 100, 436, 331, NULL, 'g.L-1', NULL, 0, 1, 1, now(), now()),
(5936, 7, 100, 437, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '微西门子每厘米', 'zh', 0, 1, 1, now(), now()),
(5937, 7, 100, 437, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '水电导率（锅炉补水/冷却水）。', 'zh', 0, 1, 1, now(), now()),
(5938, 7, 100, 437, 332, NULL, 'http://qudt.org/vocab/unit/MicroS-PER-CentiM', NULL, 0, 1, 1, now(), now()),
(5939, 7, 100, 437, 330, NULL, 'μS/cm', NULL, 0, 1, 1, now(), now()),
(5940, 7, 100, 437, 331, NULL, 'uS.cm-1', NULL, 0, 1, 1, now(), now()),
(5941, 7, 100, 438, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '伏特', 'zh', 0, 1, 1, now(), now()),
(5942, 7, 100, 438, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '相电压 220V、消防模块 DC24V、备用电源电压。', 'zh', 0, 1, 1, now(), now()),
(5943, 7, 100, 438, 332, NULL, 'http://qudt.org/vocab/unit/V', NULL, 0, 1, 1, now(), now()),
(5944, 7, 100, 438, 330, NULL, 'V', NULL, 0, 1, 1, now(), now()),
(5945, 7, 100, 438, 331, NULL, 'V', NULL, 0, 1, 1, now(), now()),
(5946, 7, 100, 439, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '毫伏', 'zh', 0, 1, 1, now(), now()),
(5947, 7, 100, 439, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '毫伏级传感信号。', 'zh', 0, 1, 1, now(), now()),
(5948, 7, 100, 439, 332, NULL, 'http://qudt.org/vocab/unit/MilliV', NULL, 0, 1, 1, now(), now()),
(5949, 7, 100, 439, 330, NULL, 'mV', NULL, 0, 1, 1, now(), now()),
(5950, 7, 100, 439, 331, NULL, 'mV', NULL, 0, 1, 1, now(), now()),
(5951, 7, 100, 440, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千伏', 'zh', 0, 1, 1, now(), now()),
(5952, 7, 100, 440, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '高压配电 10kV/35kV。', 'zh', 0, 1, 1, now(), now()),
(5953, 7, 100, 440, 332, NULL, 'http://qudt.org/vocab/unit/KiloV', NULL, 0, 1, 1, now(), now()),
(5954, 7, 100, 440, 330, NULL, 'kV', NULL, 0, 1, 1, now(), now()),
(5955, 7, 100, 440, 331, NULL, 'kV', NULL, 0, 1, 1, now(), now()),
(5956, 7, 100, 441, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '安培', 'zh', 0, 1, 1, now(), now()),
(5957, 7, 100, 441, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '回路电流、消防泵电流。', 'zh', 0, 1, 1, now(), now()),
(5958, 7, 100, 441, 332, NULL, 'http://qudt.org/vocab/unit/A', NULL, 0, 1, 1, now(), now()),
(5959, 7, 100, 441, 330, NULL, 'A', NULL, 0, 1, 1, now(), now()),
(5960, 7, 100, 441, 331, NULL, 'A', NULL, 0, 1, 1, now(), now()),
(5961, 7, 100, 442, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '毫安', 'zh', 0, 1, 1, now(), now()),
(5962, 7, 100, 442, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '4-20mA 变送器信号、报警总线巡检电流。', 'zh', 0, 1, 1, now(), now()),
(5963, 7, 100, 442, 332, NULL, 'http://qudt.org/vocab/unit/MilliA', NULL, 0, 1, 1, now(), now()),
(5964, 7, 100, 442, 330, NULL, 'mA', NULL, 0, 1, 1, now(), now()),
(5965, 7, 100, 442, 331, NULL, 'mA', NULL, 0, 1, 1, now(), now()),
(5966, 7, 100, 443, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '瓦特', 'zh', 0, 1, 1, now(), now()),
(5967, 7, 100, 443, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '设备功率。', 'zh', 0, 1, 1, now(), now()),
(5968, 7, 100, 443, 332, NULL, 'http://qudt.org/vocab/unit/W', NULL, 0, 1, 1, now(), now()),
(5969, 7, 100, 443, 330, NULL, 'W', NULL, 0, 1, 1, now(), now()),
(5970, 7, 100, 443, 331, NULL, 'W', NULL, 0, 1, 1, now(), now()),
(5971, 7, 100, 444, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千瓦', 'zh', 0, 1, 1, now(), now()),
(5972, 7, 100, 444, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '功率/制冷量/制热量：消防泵功率、充电桩功率、冷机冷量。', 'zh', 0, 1, 1, now(), now()),
(5973, 7, 100, 444, 332, NULL, 'http://qudt.org/vocab/unit/KiloW', NULL, 0, 1, 1, now(), now()),
(5974, 7, 100, 444, 330, NULL, 'kW', NULL, 0, 1, 1, now(), now()),
(5975, 7, 100, 444, 331, NULL, 'kW', NULL, 0, 1, 1, now(), now()),
(5976, 7, 100, 445, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '兆瓦', 'zh', 0, 1, 1, now(), now()),
(5977, 7, 100, 445, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '园区总负荷、光伏电站容量。', 'zh', 0, 1, 1, now(), now()),
(5978, 7, 100, 445, 332, NULL, 'http://qudt.org/vocab/unit/MegaW', NULL, 0, 1, 1, now(), now()),
(5979, 7, 100, 445, 330, NULL, 'MW', NULL, 0, 1, 1, now(), now()),
(5980, 7, 100, 445, 331, NULL, 'MW', NULL, 0, 1, 1, now(), now()),
(5981, 7, 100, 446, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '伏安', 'zh', 0, 1, 1, now(), now()),
(5982, 7, 100, 446, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '小容量 UPS 视在功率。', 'zh', 0, 1, 1, now(), now()),
(5983, 7, 100, 446, 332, NULL, 'http://qudt.org/vocab/unit/V-A', NULL, 0, 1, 1, now(), now()),
(5984, 7, 100, 446, 330, NULL, 'V·A', NULL, 0, 1, 1, now(), now()),
(5985, 7, 100, 446, 331, NULL, 'V.A', NULL, 0, 1, 1, now(), now()),
(5986, 7, 100, 447, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千伏安', 'zh', 0, 1, 1, now(), now()),
(5987, 7, 100, 447, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '变压器容量（1250kVA）、UPS/EPS 容量。', 'zh', 0, 1, 1, now(), now()),
(5988, 7, 100, 447, 332, NULL, 'http://qudt.org/vocab/unit/KiloV-A', NULL, 0, 1, 1, now(), now()),
(5989, 7, 100, 447, 330, NULL, 'kV·A', NULL, 0, 1, 1, now(), now()),
(5990, 7, 100, 447, 331, NULL, 'kV.A', NULL, 0, 1, 1, now(), now()),
(5991, 7, 100, 448, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '乏', 'zh', 0, 1, 1, now(), now()),
(5992, 7, 100, 448, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '小容量无功功率。', 'zh', 0, 1, 1, now(), now()),
(5993, 7, 100, 448, 332, NULL, 'http://qudt.org/vocab/unit/VAR', NULL, 0, 1, 1, now(), now()),
(5994, 7, 100, 448, 330, NULL, 'var', NULL, 0, 1, 1, now(), now()),
(5995, 7, 100, 448, 331, NULL, 'var', NULL, 0, 1, 1, now(), now()),
(5996, 7, 100, 449, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千乏', 'zh', 0, 1, 1, now(), now()),
(5997, 7, 100, 449, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '无功补偿电容柜。', 'zh', 0, 1, 1, now(), now()),
(5998, 7, 100, 449, 332, NULL, 'http://qudt.org/vocab/unit/KiloVAR', NULL, 0, 1, 1, now(), now()),
(5999, 7, 100, 449, 330, NULL, 'kvar', NULL, 0, 1, 1, now(), now()),
(6000, 7, 100, 449, 331, NULL, 'kvar', NULL, 0, 1, 1, now(), now()),
(6001, 7, 100, 450, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千瓦时', 'zh', 0, 1, 1, now(), now()),
(6002, 7, 100, 450, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '电度计量、分项计量（能耗监测核心单位）。', 'zh', 0, 1, 1, now(), now()),
(6003, 7, 100, 450, 332, NULL, 'http://qudt.org/vocab/unit/KiloW-HR', NULL, 0, 1, 1, now(), now()),
(6004, 7, 100, 450, 330, NULL, 'kW·h', NULL, 0, 1, 1, now(), now()),
(6005, 7, 100, 450, 331, NULL, 'kW.h', NULL, 0, 1, 1, now(), now()),
(6006, 7, 100, 451, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '兆瓦时', 'zh', 0, 1, 1, now(), now()),
(6007, 7, 100, 451, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '园区月度累计电量。', 'zh', 0, 1, 1, now(), now()),
(6008, 7, 100, 451, 332, NULL, 'http://qudt.org/vocab/unit/MegaW-HR', NULL, 0, 1, 1, now(), now()),
(6009, 7, 100, 451, 330, NULL, 'MW·h', NULL, 0, 1, 1, now(), now()),
(6010, 7, 100, 451, 331, NULL, 'MW.h', NULL, 0, 1, 1, now(), now()),
(6011, 7, 100, 452, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千乏时', 'zh', 0, 1, 1, now(), now()),
(6012, 7, 100, 452, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '无功电能。', 'zh', 0, 1, 1, now(), now()),
(6013, 7, 100, 452, 332, NULL, 'http://qudt.org/vocab/unit/KiloVAR-HR', NULL, 0, 1, 1, now(), now()),
(6014, 7, 100, 452, 330, NULL, 'kvar·h', NULL, 0, 1, 1, now(), now()),
(6015, 7, 100, 452, 331, NULL, 'kvar.h', NULL, 0, 1, 1, now(), now()),
(6016, 7, 100, 453, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千伏安时', 'zh', 0, 1, 1, now(), now()),
(6017, 7, 100, 453, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '视在电能。', 'zh', 0, 1, 1, now(), now()),
(6018, 7, 100, 453, 332, NULL, 'http://qudt.org/vocab/unit/KiloV-A-HR', NULL, 0, 1, 1, now(), now()),
(6019, 7, 100, 453, 330, NULL, 'kV·A·h', NULL, 0, 1, 1, now(), now()),
(6020, 7, 100, 453, 331, NULL, 'kV.A.h', NULL, 0, 1, 1, now(), now()),
(6021, 7, 100, 454, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '赫兹', 'zh', 0, 1, 1, now(), now()),
(6022, 7, 100, 454, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '电网频率 50Hz、变频器输出频率。', 'zh', 0, 1, 1, now(), now()),
(6023, 7, 100, 454, 332, NULL, 'http://qudt.org/vocab/unit/HZ', NULL, 0, 1, 1, now(), now()),
(6024, 7, 100, 454, 330, NULL, 'Hz', NULL, 0, 1, 1, now(), now()),
(6025, 7, 100, 454, 331, NULL, 'Hz', NULL, 0, 1, 1, now(), now()),
(6026, 7, 100, 455, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '欧姆', 'zh', 0, 1, 1, now(), now()),
(6027, 7, 100, 455, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '接地电阻（≤4Ω 防雷检测）。', 'zh', 0, 1, 1, now(), now()),
(6028, 7, 100, 455, 332, NULL, 'http://qudt.org/vocab/unit/OHM', NULL, 0, 1, 1, now(), now()),
(6029, 7, 100, 455, 330, NULL, 'Ω', NULL, 0, 1, 1, now(), now()),
(6030, 7, 100, 455, 331, NULL, 'Ohm', NULL, 0, 1, 1, now(), now()),
(6031, 7, 100, 456, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '兆欧', 'zh', 0, 1, 1, now(), now()),
(6032, 7, 100, 456, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '绝缘电阻（消防线路检测）。', 'zh', 0, 1, 1, now(), now()),
(6033, 7, 100, 456, 332, NULL, 'http://qudt.org/vocab/unit/MegaOHM', NULL, 0, 1, 1, now(), now()),
(6034, 7, 100, 456, 330, NULL, 'MΩ', NULL, 0, 1, 1, now(), now()),
(6035, 7, 100, 456, 331, NULL, 'MOhm', NULL, 0, 1, 1, now(), now()),
(6036, 7, 100, 457, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '安时', 'zh', 0, 1, 1, now(), now()),
(6037, 7, 100, 457, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '蓄电池容量（UPS/应急照明集中电源）。', 'zh', 0, 1, 1, now(), now()),
(6038, 7, 100, 457, 332, NULL, 'http://qudt.org/vocab/unit/A-HR', NULL, 0, 1, 1, now(), now()),
(6039, 7, 100, 457, 330, NULL, 'A·h', NULL, 0, 1, 1, now(), now()),
(6040, 7, 100, 457, 331, NULL, 'A.h', NULL, 0, 1, 1, now(), now()),
(6041, 7, 100, 458, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '瓦每平方米', 'zh', 0, 1, 1, now(), now()),
(6042, 7, 100, 458, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '光伏辐照度、围护结构热流密度。', 'zh', 0, 1, 1, now(), now()),
(6043, 7, 100, 458, 332, NULL, 'http://qudt.org/vocab/unit/W-PER-M2', NULL, 0, 1, 1, now(), now()),
(6044, 7, 100, 458, 330, NULL, 'W/m²', NULL, 0, 1, 1, now(), now()),
(6045, 7, 100, 458, 331, NULL, 'W.m-2', NULL, 0, 1, 1, now(), now()),
(6046, 7, 100, 459, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千瓦时每平方米', 'zh', 0, 1, 1, now(), now()),
(6047, 7, 100, 459, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '单位面积能耗强度（绿建对标指标）。', 'zh', 0, 1, 1, now(), now()),
(6048, 7, 100, 459, 332, NULL, 'http://qudt.org/vocab/unit/KiloW-HR-PER-M2', NULL, 0, 1, 1, now(), now()),
(6049, 7, 100, 459, 330, NULL, 'kW·h/m²', NULL, 0, 1, 1, now(), now()),
(6050, 7, 100, 459, 331, NULL, 'kW.h.m-2', NULL, 0, 1, 1, now(), now()),
(6051, 7, 100, 460, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '勒克斯', 'zh', 0, 1, 1, now(), now()),
(6052, 7, 100, 460, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '照度：办公 300lx、疏散地面不低于 1lx（GB 51309）、手术室 750lx 以上。', 'zh', 0, 1, 1, now(), now()),
(6053, 7, 100, 460, 332, NULL, 'http://qudt.org/vocab/unit/LUX', NULL, 0, 1, 1, now(), now()),
(6054, 7, 100, 460, 330, NULL, 'lx', NULL, 0, 1, 1, now(), now()),
(6055, 7, 100, 460, 331, NULL, 'lx', NULL, 0, 1, 1, now(), now()),
(6056, 7, 100, 461, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '流明', 'zh', 0, 1, 1, now(), now()),
(6057, 7, 100, 461, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '灯具光通量。', 'zh', 0, 1, 1, now(), now()),
(6058, 7, 100, 461, 332, NULL, 'http://qudt.org/vocab/unit/LM', NULL, 0, 1, 1, now(), now()),
(6059, 7, 100, 461, 330, NULL, 'lm', NULL, 0, 1, 1, now(), now()),
(6060, 7, 100, 461, 331, NULL, 'lm', NULL, 0, 1, 1, now(), now()),
(6061, 7, 100, 462, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '流明每瓦', 'zh', 0, 1, 1, now(), now()),
(6062, 7, 100, 462, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '灯具光效。', 'zh', 0, 1, 1, now(), now()),
(6063, 7, 100, 462, 332, NULL, 'http://qudt.org/vocab/unit/LM-PER-W', NULL, 0, 1, 1, now(), now()),
(6064, 7, 100, 462, 330, NULL, 'lm/W', NULL, 0, 1, 1, now(), now()),
(6065, 7, 100, 462, 331, NULL, 'lm.W-1', NULL, 0, 1, 1, now(), now()),
(6066, 7, 100, 463, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '坎德拉', 'zh', 0, 1, 1, now(), now()),
(6067, 7, 100, 463, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '疏散指示标志亮度（GB 17945）。', 'zh', 0, 1, 1, now(), now()),
(6068, 7, 100, 463, 332, NULL, 'http://qudt.org/vocab/unit/CD', NULL, 0, 1, 1, now(), now()),
(6069, 7, 100, 463, 330, NULL, 'cd', NULL, 0, 1, 1, now(), now()),
(6070, 7, 100, 463, 331, NULL, 'cd', NULL, 0, 1, 1, now(), now()),
(6071, 7, 100, 464, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '米', 'zh', 0, 1, 1, now(), now()),
(6072, 7, 100, 464, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '液位（消防水池/集水坑）、距离、能见度。', 'zh', 0, 1, 1, now(), now()),
(6073, 7, 100, 464, 332, NULL, 'http://qudt.org/vocab/unit/M', NULL, 0, 1, 1, now(), now()),
(6074, 7, 100, 464, 330, NULL, 'm', NULL, 0, 1, 1, now(), now()),
(6075, 7, 100, 464, 331, NULL, 'm', NULL, 0, 1, 1, now(), now()),
(6076, 7, 100, 465, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '毫米', 'zh', 0, 1, 1, now(), now()),
(6077, 7, 100, 465, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '降雨量、液位小量程、管径。', 'zh', 0, 1, 1, now(), now()),
(6078, 7, 100, 465, 332, NULL, 'http://qudt.org/vocab/unit/MilliM', NULL, 0, 1, 1, now(), now()),
(6079, 7, 100, 465, 330, NULL, 'mm', NULL, 0, 1, 1, now(), now()),
(6080, 7, 100, 465, 331, NULL, 'mm', NULL, 0, 1, 1, now(), now()),
(6081, 7, 100, 466, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '厘米', 'zh', 0, 1, 1, now(), now()),
(6082, 7, 100, 466, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '液位小量程。', 'zh', 0, 1, 1, now(), now()),
(6083, 7, 100, 466, 332, NULL, 'http://qudt.org/vocab/unit/CentiM', NULL, 0, 1, 1, now(), now()),
(6084, 7, 100, 466, 330, NULL, 'cm', NULL, 0, 1, 1, now(), now()),
(6085, 7, 100, 466, 331, NULL, 'cm', NULL, 0, 1, 1, now(), now()),
(6086, 7, 100, 467, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千克', 'zh', 0, 1, 1, now(), now()),
(6087, 7, 100, 467, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '气体灭火药剂剩余量（称重）、电梯载重、加药量。', 'zh', 0, 1, 1, now(), now()),
(6088, 7, 100, 467, 332, NULL, 'http://qudt.org/vocab/unit/KiloGM', NULL, 0, 1, 1, now(), now()),
(6089, 7, 100, 467, 330, NULL, 'kg', NULL, 0, 1, 1, now(), now()),
(6090, 7, 100, 467, 331, NULL, 'kg', NULL, 0, 1, 1, now(), now()),
(6091, 7, 100, 468, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '克', 'zh', 0, 1, 1, now(), now()),
(6092, 7, 100, 468, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '小质量计量。', 'zh', 0, 1, 1, now(), now()),
(6093, 7, 100, 468, 332, NULL, 'http://qudt.org/vocab/unit/GM', NULL, 0, 1, 1, now(), now()),
(6094, 7, 100, 468, 330, NULL, 'g', NULL, 0, 1, 1, now(), now()),
(6095, 7, 100, 468, 331, NULL, 'g', NULL, 0, 1, 1, now(), now()),
(6096, 7, 100, 469, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '吨', 'zh', 0, 1, 1, now(), now()),
(6097, 7, 100, 469, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '大宗物料/垃圾量。', 'zh', 0, 1, 1, now(), now()),
(6098, 7, 100, 469, 332, NULL, 'http://qudt.org/vocab/unit/TONNE', NULL, 0, 1, 1, now(), now()),
(6099, 7, 100, 469, 330, NULL, 't', NULL, 0, 1, 1, now(), now()),
(6100, 7, 100, 469, 331, NULL, 't', NULL, 0, 1, 1, now(), now()),
(6101, 7, 100, 470, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '秒', 'zh', 0, 1, 1, now(), now()),
(6102, 7, 100, 470, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '联动延时、报警确认时间、恢复时间。', 'zh', 0, 1, 1, now(), now()),
(6103, 7, 100, 470, 332, NULL, 'http://qudt.org/vocab/unit/SEC', NULL, 0, 1, 1, now(), now()),
(6104, 7, 100, 470, 330, NULL, 's', NULL, 0, 1, 1, now(), now()),
(6105, 7, 100, 470, 331, NULL, 's', NULL, 0, 1, 1, now(), now()),
(6106, 7, 100, 471, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '分钟', 'zh', 0, 1, 1, now(), now()),
(6107, 7, 100, 471, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '应急照明持续供电 90min、巡检周期、浸渍时间。', 'zh', 0, 1, 1, now(), now()),
(6108, 7, 100, 471, 332, NULL, 'http://qudt.org/vocab/unit/MIN', NULL, 0, 1, 1, now(), now()),
(6109, 7, 100, 471, 330, NULL, 'min', NULL, 0, 1, 1, now(), now()),
(6110, 7, 100, 471, 331, NULL, 'min', NULL, 0, 1, 1, now(), now()),
(6111, 7, 100, 472, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '小时', 'zh', 0, 1, 1, now(), now()),
(6112, 7, 100, 472, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '设备运行时长、持续供电 1.0h、能耗统计窗口。', 'zh', 0, 1, 1, now(), now()),
(6113, 7, 100, 472, 332, NULL, 'http://qudt.org/vocab/unit/HR', NULL, 0, 1, 1, now(), now()),
(6114, 7, 100, 472, 330, NULL, 'h', NULL, 0, 1, 1, now(), now()),
(6115, 7, 100, 472, 331, NULL, 'h', NULL, 0, 1, 1, now(), now()),
(6116, 7, 100, 473, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '天', 'zh', 0, 1, 1, now(), now()),
(6117, 7, 100, 473, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '统计周期。', 'zh', 0, 1, 1, now(), now()),
(6118, 7, 100, 473, 332, NULL, 'http://qudt.org/vocab/unit/DAY', NULL, 0, 1, 1, now(), now()),
(6119, 7, 100, 473, 330, NULL, 'd', NULL, 0, 1, 1, now(), now()),
(6120, 7, 100, 473, 331, NULL, 'd', NULL, 0, 1, 1, now(), now()),
(6121, 7, 100, 474, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '度（平面角）', 'zh', 0, 1, 1, now(), now()),
(6122, 7, 100, 474, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '风向、相角、风阀角度。', 'zh', 0, 1, 1, now(), now()),
(6123, 7, 100, 474, 332, NULL, 'http://qudt.org/vocab/unit/DEG', NULL, 0, 1, 1, now(), now()),
(6124, 7, 100, 474, 330, NULL, '°', NULL, 0, 1, 1, now(), now()),
(6125, 7, 100, 474, 331, NULL, 'deg', NULL, 0, 1, 1, now(), now()),
(6126, 7, 100, 475, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '弧度', 'zh', 0, 1, 1, now(), now()),
(6127, 7, 100, 475, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '角度计算。', 'zh', 0, 1, 1, now(), now()),
(6128, 7, 100, 475, 332, NULL, 'http://qudt.org/vocab/unit/RAD', NULL, 0, 1, 1, now(), now()),
(6129, 7, 100, 475, 330, NULL, 'rad', NULL, 0, 1, 1, now(), now()),
(6130, 7, 100, 475, 331, NULL, 'rad', NULL, 0, 1, 1, now(), now()),
(6131, 7, 100, 476, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '一（计数）', 'zh', 0, 1, 1, now(), now()),
(6132, 7, 100, 476, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '人数/次数/状态字等无量纲计数。', 'zh', 0, 1, 1, now(), now()),
(6133, 7, 100, 476, 332, NULL, 'http://qudt.org/vocab/unit/ONE', NULL, 0, 1, 1, now(), now()),
(6134, 7, 100, 476, 330, NULL, 'one', NULL, 0, 1, 1, now(), now()),
(6135, 7, 100, 477, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '分贝', 'zh', 0, 1, 1, now(), now()),
(6136, 7, 100, 477, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '声压级/声级计：消防广播不低于 60dB。', 'zh', 0, 1, 1, now(), now()),
(6137, 7, 100, 477, 332, NULL, 'http://qudt.org/vocab/unit/DeciB', NULL, 0, 1, 1, now(), now()),
(6138, 7, 100, 477, 330, NULL, 'dB', NULL, 0, 1, 1, now(), now()),
(6139, 7, 100, 477, 331, NULL, 'dB', NULL, 0, 1, 1, now(), now()),
(6140, 7, 100, 478, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', 'A计权分贝', 'zh', 0, 1, 1, now(), now()),
(6141, 7, 100, 478, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '环境噪声监测与消防广播声压级（dB(A)）。', 'zh', 0, 1, 1, now(), now()),
(6142, 7, 100, 478, 332, NULL, 'http://qudt.org/vocab/unit/DeciB_A', NULL, 0, 1, 1, now(), now()),
(6143, 7, 100, 478, 330, NULL, 'dB(A)', NULL, 0, 1, 1, now(), now()),
(6144, 7, 100, 479, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '升', 'zh', 0, 1, 1, now(), now()),
(6145, 7, 100, 479, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '气体灭火药剂液位、小容积。', 'zh', 0, 1, 1, now(), now()),
(6146, 7, 100, 479, 332, NULL, 'http://qudt.org/vocab/unit/L', NULL, 0, 1, 1, now(), now()),
(6147, 7, 100, 479, 330, NULL, 'L', NULL, 0, 1, 1, now(), now()),
(6148, 7, 100, 479, 331, NULL, 'L', NULL, 0, 1, 1, now(), now()),
(6149, 7, 100, 480, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千升', 'zh', 0, 1, 1, now(), now()),
(6150, 7, 100, 480, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '大容量水箱。', 'zh', 0, 1, 1, now(), now()),
(6151, 7, 100, 480, 332, NULL, 'http://qudt.org/vocab/unit/KiloL', NULL, 0, 1, 1, now(), now()),
(6152, 7, 100, 480, 330, NULL, 'kL', NULL, 0, 1, 1, now(), now()),
(6153, 7, 100, 480, 331, NULL, 'kL', NULL, 0, 1, 1, now(), now()),
(6154, 7, 100, 481, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千焦', 'zh', 0, 1, 1, now(), now()),
(6155, 7, 100, 481, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '冷量小计。', 'zh', 0, 1, 1, now(), now()),
(6156, 7, 100, 481, 332, NULL, 'http://qudt.org/vocab/unit/KiloJ', NULL, 0, 1, 1, now(), now()),
(6157, 7, 100, 481, 330, NULL, 'kJ', NULL, 0, 1, 1, now(), now()),
(6158, 7, 100, 481, 331, NULL, 'kJ', NULL, 0, 1, 1, now(), now()),
(6159, 7, 100, 482, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '兆焦', 'zh', 0, 1, 1, now(), now()),
(6160, 7, 100, 482, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '中间量。', 'zh', 0, 1, 1, now(), now()),
(6161, 7, 100, 482, 332, NULL, 'http://qudt.org/vocab/unit/MegaJ', NULL, 0, 1, 1, now(), now()),
(6162, 7, 100, 482, 330, NULL, 'MJ', NULL, 0, 1, 1, now(), now()),
(6163, 7, 100, 482, 331, NULL, 'MJ', NULL, 0, 1, 1, now(), now()),
(6164, 7, 100, 483, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '吉焦', 'zh', 0, 1, 1, now(), now()),
(6165, 7, 100, 483, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '热量表结算（集中供热按 GJ）。', 'zh', 0, 1, 1, now(), now()),
(6166, 7, 100, 483, 332, NULL, 'http://qudt.org/vocab/unit/GigaJ', NULL, 0, 1, 1, now(), now()),
(6167, 7, 100, 483, 330, NULL, 'GJ', NULL, 0, 1, 1, now(), now()),
(6168, 7, 100, 483, 331, NULL, 'GJ', NULL, 0, 1, 1, now(), now()),
(6169, 7, 100, 484, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '千卡', 'zh', 0, 1, 1, now(), now()),
(6170, 7, 100, 484, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '老式热量表。', 'zh', 0, 1, 1, now(), now()),
(6171, 7, 100, 484, 332, NULL, 'http://qudt.org/vocab/unit/KiloCAL', NULL, 0, 1, 1, now(), now()),
(6172, 7, 100, 484, 330, NULL, 'kcal', NULL, 0, 1, 1, now(), now()),
(6173, 7, 100, 484, 331, NULL, 'kcal', NULL, 0, 1, 1, now(), now()),
(6174, 7, 100, 485, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '冷吨', 'zh', 0, 1, 1, now(), now()),
(6175, 7, 100, 485, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '冷机额定冷量（500RT 级）、机房空调。', 'zh', 0, 1, 1, now(), now()),
(6176, 7, 100, 485, 332, NULL, 'http://qudt.org/vocab/unit/TON_FG', NULL, 0, 1, 1, now(), now()),
(6177, 7, 100, 485, 330, NULL, 'TOR', NULL, 0, 1, 1, now(), now()),
(6178, 7, 100, 486, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '英热单位每小时', 'zh', 0, 1, 1, now(), now()),
(6179, 7, 100, 486, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '进口冷机冷量。', 'zh', 0, 1, 1, now(), now()),
(6180, 7, 100, 486, 332, NULL, 'http://qudt.org/vocab/unit/BTU_IT-PER-HR', NULL, 0, 1, 1, now(), now()),
(6181, 7, 100, 486, 330, NULL, 'Btu{IT}/h', NULL, 0, 1, 1, now(), now()),
(6182, 7, 100, 486, 331, NULL, '[Btu_IT].h-1', NULL, 0, 1, 1, now(), now()),
(6183, 7, 100, 487, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '牛顿米', 'zh', 0, 1, 1, now(), now()),
(6184, 7, 100, 487, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '电动调节阀力矩。', 'zh', 0, 1, 1, now(), now()),
(6185, 7, 100, 487, 332, NULL, 'http://qudt.org/vocab/unit/N-M', NULL, 0, 1, 1, now(), now()),
(6186, 7, 100, 487, 330, NULL, 'N·m', NULL, 0, 1, 1, now(), now()),
(6187, 7, 100, 487, 331, NULL, 'N.m', NULL, 0, 1, 1, now(), now())
ON CONFLICT (id) DO NOTHING;

-- ----------------------------
-- 4. 序列校准（必须）
-- ----------------------------
-- ============================================================
-- 脚本: V20261015_08__ontology_uom_scaling_conventional.sql
-- 目的: 计量单位库增补——① scalingOf 换算链（QUDT 原样 35 条，回答「倍率怎么来的」）
--       ② conventionalUnit 量纲惯用单位（52 条，国内习惯基准，回答「同一量纲默认用哪个」）
--       ③ 补种 4 个单位个体 UNITLESS/J/J-PER-SEC/M2（补齐缩放链目标与面积惯用单位）
-- 设计: docs/ontology/建模/01-楼控与消防数据单位清单及种子建模分析.md V1.3（§5.2/§5.6/§7 #7）
-- 备注:
--   1. id 编配: 实体 311~312（对象属性段）、488~491（单位个体段，接 V7 的 487）；
--      公理 6188~6326（接 V7 的 6187）
--   2. 换算链保持 QUDT SI 锚定不改按习惯改写——倍率语义（×倍率+偏移=SI 相干单位值）
--      与 qudtIri 溯源一致；国内习惯基准独立落于 conventionalUnit，两层不混
--   3. 基准判定规则（V1.3 §5.6）: 查 conventionalUnit（惯用）→ 查 scalingOf 树根（定义根）
--      → multiplier=1 且无偏移仅是「SI 相干」的必要条件非充分条件（kL/m³ 同为 1.0）
--   4. 双路径兼容: ON CONFLICT (id) DO NOTHING 可重复执行；不写变更日志，version 不动
-- ============================================================

INSERT INTO ont_entities (id, ontology_id, entity_type, iri, local_name, display_label, deprecated, created_by, updated_by, created_at, updated_at) VALUES
(311, 7, 20, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', 'scalingOf', '缩放自', false, 1, 1, now(), now()),
(312, 7, 20, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', 'conventionalUnit', '惯用单位', false, 1, 1, now(), now()),
(488, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#UNITLESS', 'UNITLESS', '无量纲（一）', false, 1, 1, now(), now()),
(489, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#J', 'J', '焦耳', false, 1, 1, now(), now()),
(490, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#J-PER-SEC', 'J-PER-SEC', '焦耳每秒', false, 1, 1, now(), now()),
(491, 7, 60, 'https://youmingwanwu.com/ontology/youmingwanwu#M2', 'M2', '平方米', false, 1, 1, now(), now())
ON CONFLICT (id) DO NOTHING;

-- 公理（139 行）
INSERT INTO ont_axioms (id, ontology_id, axiom_type, subject_id, object_id, predicate_iri, value_text, lang, sort_order, created_by, updated_by, created_at, updated_at) VALUES
-- 实体声明(10)
(6188, 7, 10, 311, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(6189, 7, 10, 312, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(6190, 7, 10, 488, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(6191, 7, 10, 489, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(6192, 7, 10, 490, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
(6193, 7, 10, 491, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),
-- 注解断言(100)
(6194, 7, 100, 311, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '缩放自', 'zh', 0, 1, 1, now(), now()),
(6195, 7, 100, 311, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '对应 qudt:scalingOf：本单位的定义 = 缩放目标单位 × 换算倍率(+偏移)，换算链 SI 锚定、保持 QUDT 原样；无「缩放自」的单位为定义根（SI 相干单位或独立单位如 PSI/L）。', 'zh', 0, 1, 1, now(), now()),
(6196, 7, 100, 312, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '惯用单位', 'zh', 0, 1, 1, now(), now()),
(6197, 7, 100, 312, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '量纲类型的国内惯用基准单位（定义域=量纲类型、值域=计量单位、函数性）——「同一量纲哪个是基准」的显式答案，选取对齐国内楼宇/消防行业点表与规范口径；与换算链分工：本属性管「默认用哪个」，scalingOf 管「怎么换算」。', 'zh', 0, 1, 1, now(), now()),
-- 对象属性定义域(31)
(6198, 7, 31, 311, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),  -- scalingOf 定义域=计量单位
-- 对象属性值域(32)
(6199, 7, 32, 311, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),  -- scalingOf 值域=计量单位
-- 对象属性定义域(31)
(6200, 7, 31, 312, 303, NULL, NULL, NULL, 0, 1, 1, now(), now()),  -- conventionalUnit 定义域=量纲类型
-- 对象属性值域(32)
(6201, 7, 32, 312, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),  -- conventionalUnit 值域=计量单位
-- 对象属性特征(33)
(6202, 7, 33, 312, NULL, 'http://www.w3.org/2002/07/owl#FunctionalProperty', NULL, NULL, 0, 1, 1, now(), now()),  -- conventionalUnit 函数性（每量纲唯一）
-- 类型断言(60)
(6203, 7, 60, 488, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
-- 数据属性断言(62)
(6204, 7, 62, 488, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-UNITLESS', NULL, 0, 1, 1, now(), now()),  -- UNITLESS 编码
(6205, 7, 62, 488, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- UNITLESS multiplier
-- 注解断言(100)
(6206, 7, 100, 488, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '无量纲（一）', 'zh', 0, 1, 1, now(), now()),
(6207, 7, 100, 488, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '无量纲单位的公共定义根：PERCENT/PPM/PPB/%RH 的缩放基准（QUDT 原文符号「一」）。', 'zh', 0, 1, 1, now(), now()),
(6208, 7, 100, 488, 332, NULL, 'http://qudt.org/vocab/unit/UNITLESS', NULL, 0, 1, 1, now(), now()),
(6209, 7, 100, 488, 330, NULL, '一', NULL, 0, 1, 1, now(), now()),
-- 类型断言(60)
(6210, 7, 60, 489, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
-- 数据属性断言(62)
(6211, 7, 62, 489, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-J', NULL, 0, 1, 1, now(), now()),  -- J 编码
(6212, 7, 62, 489, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- J multiplier
-- 注解断言(100)
(6213, 7, 100, 489, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '焦耳', 'zh', 0, 1, 1, now(), now()),
(6214, 7, 100, 489, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', 'SI 相干能量单位（倍率 1.0），kJ/MJ/GJ/kcal 的缩放基准；电能量纲的锚点。', 'zh', 0, 1, 1, now(), now()),
(6215, 7, 100, 489, 332, NULL, 'http://qudt.org/vocab/unit/J', NULL, 0, 1, 1, now(), now()),
(6216, 7, 100, 489, 330, NULL, 'J', NULL, 0, 1, 1, now(), now()),
(6217, 7, 100, 489, 331, NULL, 'J', NULL, 0, 1, 1, now(), now()),
-- 类型断言(60)
(6218, 7, 60, 490, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
-- 数据属性断言(62)
(6219, 7, 62, 490, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-J-PER-SEC', NULL, 0, 1, 1, now(), now()),  -- J-PER-SEC 编码
(6220, 7, 62, 490, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- J-PER-SEC multiplier
-- 注解断言(100)
(6221, 7, 100, 490, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '焦耳每秒', 'zh', 0, 1, 1, now(), now()),
(6222, 7, 100, 490, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '功率量纲的 SI 相干单位（J/s，倍率 1.0），冷吨的缩放基准。', 'zh', 0, 1, 1, now(), now()),
(6223, 7, 100, 490, 332, NULL, 'http://qudt.org/vocab/unit/J-PER-SEC', NULL, 0, 1, 1, now(), now()),
(6224, 7, 100, 490, 330, NULL, 'J/s', NULL, 0, 1, 1, now(), now()),
(6225, 7, 100, 490, 331, NULL, 'J.s-1', NULL, 0, 1, 1, now(), now()),
-- 类型断言(60)
(6226, 7, 60, 491, 302, NULL, NULL, NULL, 0, 1, 1, now(), now()),
-- 数据属性断言(62)
(6227, 7, 62, 491, (SELECT id FROM ont_entities WHERE ontology_id = 7 AND iri = 'https://youmingwanwu.com/ontology/youmingwanwu#code' AND deleted_at IS NULL LIMIT 1), NULL, 'U-M2', NULL, 0, 1, 1, now(), now()),  -- M2 编码
(6228, 7, 62, 491, 320, NULL, '1.0', NULL, 1, 1, 1, now(), now()),  -- M2 multiplier
-- 注解断言(100)
(6229, 7, 100, 491, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '平方米', 'zh', 0, 1, 1, now(), now()),
(6230, 7, 100, 491, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '面积惯用单位（SI 导出），建筑面积与能耗强度分母。', 'zh', 0, 1, 1, now(), now()),
(6231, 7, 100, 491, 332, NULL, 'http://qudt.org/vocab/unit/M2', NULL, 0, 1, 1, now(), now()),
(6232, 7, 100, 491, 330, NULL, 'm²', NULL, 0, 1, 1, now(), now()),
(6233, 7, 100, 491, 331, NULL, 'm2', NULL, 0, 1, 1, now(), now()),
-- 对象属性断言(61)
(6234, 7, 61, 488, 383, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- UNITLESS → DimensionlessRatio
(6235, 7, 61, 489, 372, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- J → Energy
(6236, 7, 61, 489, 371, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- J → ThermalEnergy
(6237, 7, 61, 489, 368, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- J → ElectricEnergy
(6238, 7, 61, 490, 373, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- J-PER-SEC → Power
(6239, 7, 61, 491, 356, 'https://youmingwanwu.com/ontology/youmingwanwu#hasQuantityKind', NULL, NULL, 0, 1, 1, now(), now()),  -- M2 → Area
(6240, 7, 61, 401, 402, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- DEG_C 缩放自 K
(6241, 7, 61, 403, 402, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- DEG_F 缩放自 K
(6242, 7, 61, 404, 488, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- PERCENT_RH 缩放自 UNITLESS
(6243, 7, 61, 407, 406, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloPA 缩放自 PA
(6244, 7, 61, 408, 406, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- MegaPA 缩放自 PA
(6245, 7, 61, 409, 406, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- BAR 缩放自 PA
(6246, 7, 61, 410, 406, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- MilliM_H2O 缩放自 PA
(6247, 7, 61, 411, 406, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- M_H2O 缩放自 PA
(6248, 7, 61, 412, 406, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- IN_H2O 缩放自 PA
(6249, 7, 61, 413, 406, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- MilliM_HG 缩放自 PA
(6250, 7, 61, 415, 406, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- HectoPA 缩放自 PA
(6251, 7, 61, 427, 488, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- PPM 缩放自 UNITLESS
(6252, 7, 61, 428, 488, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- PPB 缩放自 UNITLESS
(6253, 7, 61, 429, 488, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- PERCENT 缩放自 UNITLESS
(6254, 7, 61, 439, 438, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- MilliV 缩放自 V
(6255, 7, 61, 440, 438, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloV 缩放自 V
(6256, 7, 61, 442, 441, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- MilliA 缩放自 A
(6257, 7, 61, 444, 443, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloW 缩放自 W
(6258, 7, 61, 445, 443, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- MegaW 缩放自 W
(6259, 7, 61, 449, 448, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloVAR 缩放自 VAR
(6260, 7, 61, 456, 455, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- MegaOHM 缩放自 OHM
(6261, 7, 61, 465, 464, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- MilliM 缩放自 M
(6262, 7, 61, 466, 464, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- CentiM 缩放自 M
(6263, 7, 61, 468, 467, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- GM 缩放自 KiloGM
(6264, 7, 61, 469, 467, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- TONNE 缩放自 KiloGM
(6265, 7, 61, 471, 470, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- MIN 缩放自 SEC
(6266, 7, 61, 472, 470, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- HR 缩放自 SEC
(6267, 7, 61, 473, 470, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- DAY 缩放自 SEC
(6268, 7, 61, 474, 475, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- DEG 缩放自 RAD
(6269, 7, 61, 480, 479, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloL 缩放自 L
(6270, 7, 61, 481, 489, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloJ 缩放自 J
(6271, 7, 61, 482, 489, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- MegaJ 缩放自 J
(6272, 7, 61, 483, 489, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- GigaJ 缩放自 J
(6273, 7, 61, 484, 489, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- KiloCAL 缩放自 J
(6274, 7, 61, 485, 490, 'https://youmingwanwu.com/ontology/youmingwanwu#scalingOf', NULL, NULL, 0, 1, 1, now(), now()),  -- TON_FG 缩放自 J-PER-SEC
(6275, 7, 61, 341, 401, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 国内法定计量与楼宇点表通用摄氏度
(6276, 7, 61, 342, 401, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 温差习惯用 ℃ 表述（ΔT 数值等效 K，差量计算注意偏移）
(6277, 7, 61, 343, 402, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 色温国内外通用 K
(6278, 7, 61, 344, 404, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 湿度点表通用 %RH
(6279, 7, 61, 345, 405, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 含湿量 g/m³
(6280, 7, 61, 346, 407, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 通用工程压力 kPa（消防管网惯用 MPa、风压差惯用 Pa，按测点选择）
(6281, 7, 61, 347, 408, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 表压惯用 MPa（冷机/锅炉/瓶组）
(6282, 7, 61, 348, 406, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 风管静压/房间压差惯用 Pa
(6283, 7, 61, 349, 406, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 动压惯用 Pa（毕托管）
(6284, 7, 61, 350, 415, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 气象气压惯用 hPa
(6285, 7, 61, 351, 416, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 风量/水量国内绝对主导 m³/h
(6286, 7, 61, 352, 424, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 蒸汽流量国内惯用 t/h
(6287, 7, 61, 353, 464, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 长度 m
(6288, 7, 61, 354, 464, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 液位惯用 m
(6289, 7, 61, 355, 420, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 累计量/容积惯用 m³
(6290, 7, 61, 356, 491, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 面积 m²
(6291, 7, 61, 357, 421, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 风速/梯速 m/s
(6292, 7, 61, 358, 423, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 转速国内惯用 r/min
(6293, 7, 61, 359, 474, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 风向/相角/阀位角度惯用 °
(6294, 7, 61, 360, 470, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 时间基准 s（运行时长报表另用 h）
(6295, 7, 61, 361, 467, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 质量 kg
(6296, 7, 61, 362, 441, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 电流 A
(6297, 7, 61, 363, 438, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 电压 V（高压回路另用 kV）
(6298, 7, 61, 364, 454, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 频率 Hz
(6299, 7, 61, 365, 444, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 有功功率国内惯用 kW
(6300, 7, 61, 366, 447, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 视在容量惯用 kV·A
(6301, 7, 61, 367, 449, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 无功惯用 kvar
(6302, 7, 61, 368, 450, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 电能国内惯用 kW·h（度）
(6303, 7, 61, 369, 452, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 无功电能惯用 kvar·h
(6304, 7, 61, 370, 453, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 视在电能惯用 kV·A·h
(6305, 7, 61, 371, 483, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 热量结算国内法定/惯用 GJ
(6306, 7, 61, 372, 481, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 能量总类取 kJ
(6307, 7, 61, 373, 444, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 功率惯用 kW
(6308, 7, 61, 374, 444, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 制冷/制热量国内惯用 kW（冷吨为进口习惯）
(6309, 7, 61, 375, 457, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 蓄电池容量惯用 A·h
(6310, 7, 61, 376, 455, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 电阻基准 Ω（绝缘电阻 MΩ 为高压档）
(6311, 7, 61, 377, 437, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 水质电导率惯用 µS/cm
(6312, 7, 61, 378, 460, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 照度 lx
(6313, 7, 61, 379, 461, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 光通量 lm
(6314, 7, 61, 380, 463, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 发光强度 cd
(6315, 7, 61, 381, 462, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 光效 lm/W
(6316, 7, 61, 382, 430, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 颗粒物惯用 µg/m³（甲醛/TVOC 国标用 mg/m³，水质 mg/L，按测点选择）
(6317, 7, 61, 383, 429, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 比值惯用 %
(6318, 7, 61, 384, 429, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 体积分数惯用 %VOL
(6319, 7, 61, 385, 427, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 气体浓度惯用 ppm
(6320, 7, 61, 386, 432, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 感烟减光率国标口径 %/m
(6321, 7, 61, 387, 434, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 浊度惯用 NTU
(6322, 7, 61, 388, 478, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 噪声监测惯用 A 计权 dB(A)
(6323, 7, 61, 389, 476, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 计数以「一」为基准
(6324, 7, 61, 390, 487, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 力矩 N·m
(6325, 7, 61, 391, 458, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now()),  -- 辐照度 W/m²
(6326, 7, 61, 392, 459, 'https://youmingwanwu.com/ontology/youmingwanwu#conventionalUnit', NULL, NULL, 0, 1, 1, now(), now())  -- 能耗强度惯用 kW·h/m²
ON CONFLICT (id) DO NOTHING;

-- ----------------------------
-- 3.5 单位域容器类公理（结构调整：万物 → 单位 → {计量单位, 量纲类型}；号段 6327~6330 续编，
--     子类边 5170/5171 已直挂 304，本块只落 304 自身骨架）
-- ----------------------------
INSERT INTO ont_axioms (id, ontology_id, axiom_type, subject_id, object_id, predicate_iri, value_text, lang, sort_order, created_by, updated_by, created_at, updated_at) VALUES
(6327, 7, 10, 304, NULL, NULL, NULL, NULL, 0, 1, 1, now(), now()),  -- 单位 声明
(6328, 7, 20, 304, 301, NULL, NULL, NULL, 0, 1, 1, now(), now()),  -- 单位 ⊑ 万物
(6329, 7, 100, 304, NULL, 'http://www.w3.org/2000/01/rdf-schema#label', '单位', 'zh', 0, 1, 1, now(), now()),
(6330, 7, 100, 304, NULL, 'http://www.w3.org/2000/01/rdf-schema#comment', '计量域容器类：统辖计量单位与量纲类型两个叶子类；行业/业务侧一律把单位个体挂到叶子类，不再另立顶层类，为后续货币等计量外单位域预留扩展位。', 'zh', 0, 1, 1, now(), now())
ON CONFLICT (id) DO NOTHING;

-- ----------------------------
-- 4. 序列校准（必须；GENERATED BY DEFAULT AS IDENTITY 序列名）
-- ----------------------------
SELECT setval('ont_entities_id_seq', GREATEST((SELECT COALESCE(MAX(id), 0) FROM ont_entities), 1));
SELECT setval('ont_axioms_id_seq', GREATEST((SELECT COALESCE(MAX(id), 0) FROM ont_axioms), 1));

-- +goose Down
-- 种子回滚（best-effort）：按本脚本号段删除；若 id>=301/5001 已被后续 UI/接口增量占用会连带删除，生产慎用。
DELETE FROM ont_axioms WHERE ontology_id = 7 AND id >= 5001;
DELETE FROM ont_entities WHERE ontology_id = 7 AND id >= 301;
UPDATE ont_ontologies SET description = '内置种子本体：有名万物——万物皆有名，以名指物', updated_at = now() WHERE id = 7;

