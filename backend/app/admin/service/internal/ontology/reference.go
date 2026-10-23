package ontology

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 08 期通用反查（引用边查询）域内核：公理号段集合、命中位置、成对去重、属性链合并与
// 排序分页。纯函数（不依赖 ent 查询/HTTP），repo 与 service 各自调用，单测直接喂行切片。
// 设计文档：docs/ontology/design/08-通用反查（引用边查询）详细设计.md §3.2/§4.3。

// ReferencePosition 目标实体在公理行内的命中位置（与 proto ReferencePosition 非零值一一对应；
// 0 = 未指定，按 ReferencePositionObject 处理）。
type ReferencePosition uint8

const (
	ReferencePositionObject      ReferencePosition = 1 // 目标作为宾语（object_id）
	ReferencePositionPredicate   ReferencePosition = 2 // 目标 IRI 作为谓词（predicate_iri；61 断言）
	ReferencePositionChainMember ReferencePosition = 3 // 目标 id 出现在 37 属性链成员 JSON 内
)

// AllAxiomTypes 已启用公理号段全集（请求 axiomTypes 合法性校验；10 声明与 46 定义亦在册，
// 反查命中为空是合法答案）。
var AllAxiomTypes = []uint32{
	AxiomDeclaration,
	AxiomSubClassOf,
	AxiomDisjointWith,
	AxiomEquivalentClasses,
	AxiomSubObjectPropertyOf,
	AxiomObjectPropertyDomain,
	AxiomObjectPropertyRange,
	AxiomObjectPropertyCharacteristic,
	AxiomInverseOf,
	AxiomEquivalentObjectProperties,
	AxiomDisjointObjectProperties,
	AxiomSubPropertyChainOf,
	AxiomSubDataPropertyOf,
	AxiomDataPropertyDomain,
	AxiomDataPropertyRange,
	AxiomDataPropertyCharacteristic,
	AxiomEquivalentDataProperties,
	AxiomDisjointDataProperties,
	AxiomDatatypeDefinition,
	AxiomSubAnnotationPropertyOf,
	AxiomTypeAssertion,
	AxiomObjectPropertyAssertion,
	AxiomDataPropertyAssertion,
	AxiomAnnotationAssertion,
}

// DefaultReferenceAxiomTypes 通用反查默认集（08 号文档 §3.2 反查可用性矩阵的 ✓ 行）：
// 显式排除 33/43（特征谓词是 IRI 文本，无目标实体可反查）、46（定义行以 datatype 为主语，
// 无入边语义）、42 内建轨与 100-IRI 轨（宾语是 IRI 文本，不满足"边两端都是实体"）。
// 37 在册但只在 positions 含 CHAIN_MEMBER 时生效（成员在 value_text JSON 内）。
var DefaultReferenceAxiomTypes = []uint32{
	AxiomSubClassOf,
	AxiomDisjointWith,
	AxiomEquivalentClasses,
	AxiomSubObjectPropertyOf,
	AxiomObjectPropertyDomain,
	AxiomObjectPropertyRange,
	AxiomInverseOf,
	AxiomEquivalentObjectProperties,
	AxiomDisjointObjectProperties,
	AxiomSubPropertyChainOf,
	AxiomSubDataPropertyOf,
	AxiomDataPropertyDomain,
	AxiomDataPropertyRange,
	AxiomEquivalentDataProperties,
	AxiomDisjointDataProperties,
	AxiomSubAnnotationPropertyOf,
	AxiomTypeAssertion,
	AxiomObjectPropertyAssertion,
	AxiomDataPropertyAssertion,
	AxiomAnnotationAssertion,
}

// PairedAxiomTypes 成对存储的公理号段（(A,B)+(B,A) 各一行）：反查须按无序对去重
// （保留 axiom_id 较小者，标记 paired=true），否则 total/分页双计。
var PairedAxiomTypes = []uint32{
	AxiomDisjointWith,
	AxiomEquivalentClasses,
	AxiomInverseOf,
	AxiomEquivalentObjectProperties,
	AxiomDisjointObjectProperties,
	AxiomEquivalentDataProperties,
	AxiomDisjointDataProperties,
}

// PredicateReferenceAxiomTypes V1 支持谓词位反查的号段（100 的 IRI 轨 V1 不做，见 §3.2）。
var PredicateReferenceAxiomTypes = []uint32{
	AxiomObjectPropertyAssertion,
}

// IsKnownAxiomType 号段是否在已启用全集内
func IsKnownAxiomType(t uint32) bool {
	for _, v := range AllAxiomTypes {
		if v == t {
			return true
		}
	}
	return false
}

// IsPairedAxiomType 是否成对存储号段
func IsPairedAxiomType(t uint32) bool {
	for _, v := range PairedAxiomTypes {
		if v == t {
			return true
		}
	}
	return false
}

// HasAxiomType 类型集中是否包含指定号段
func HasAxiomType(types []uint32, t uint32) bool {
	for _, v := range types {
		if v == t {
			return true
		}
	}
	return false
}

// IntersectAxiomTypes 交集（保 a 序、去重）
func IntersectAxiomTypes(a, b []uint32) []uint32 {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	set := make(map[uint32]struct{}, len(b))
	for _, v := range b {
		set[v] = struct{}{}
	}
	out := make([]uint32, 0, len(a))
	for _, v := range a {
		if _, ok := set[v]; !ok {
			continue
		}
		if HasAxiomType(out, v) {
			continue
		}
		out = append(out, v)
	}
	return out
}

// RefRow 一条反查边（未删公理行的投影 + 命中位置/成对标记；节点详情由服务层批量填充）。
type RefRow struct {
	AxiomID      uint32
	AxiomType    uint32
	SortOrder    uint32
	SubjectID    uint32
	ObjectID     uint32
	PredicateIRI string
	ValueText    string
	Lang         string
	UpdatedAt    *time.Time
	Position     ReferencePosition
	Paired       bool
}

// RefRowOf 公理行 → 反查边投影（nil 列取零值；软删过滤在校验前由 repo 完成）
func RefRowOf(ax *ent.OntAxiom, position ReferencePosition) RefRow {
	row := RefRow{
		AxiomID:   ax.ID,
		AxiomType: nilUint(ax.AxiomType),
		SortOrder: nilUint(ax.SortOrder),
		SubjectID: nilUint(ax.SubjectID),
		ObjectID:  nilUint(ax.ObjectID),
		Position:  position,
	}
	row.PredicateIRI = nilString(ax.PredicateIri)
	row.ValueText = nilString(ax.ValueText)
	row.Lang = nilString(ax.Lang)
	if ax.UpdatedAt != nil {
		row.UpdatedAt = ax.UpdatedAt
	} else {
		row.UpdatedAt = ax.CreatedAt
	}
	return row
}

// RefRowsOf 公理行切片 → 反查边投影
func RefRowsOf(axioms []*ent.OntAxiom, position ReferencePosition) []RefRow {
	out := make([]RefRow, 0, len(axioms))
	for _, ax := range axioms {
		if ax == nil {
			continue
		}
		out = append(out, RefRowOf(ax, position))
	}
	return out
}

// ChainRefRows 37 属性链成员反查：value_text 的 JSON 数组成员命中目标 id 的链行。
// 解析失败行 WARN 跳过（与 ChainReferrers 同口径；成员可重复出现，按行去重）。
func ChainRefRows(ctx context.Context, axioms []*ent.OntAxiom, targetID uint32, log Logger) []RefRow {
	out := make([]RefRow, 0)
	for _, ax := range axioms {
		if ax == nil || ax.AxiomType == nil || *ax.AxiomType != AxiomSubPropertyChainOf {
			continue
		}
		if ax.SubjectID == nil || ax.ValueText == nil {
			continue
		}
		members, err := ParseChainMemberIDs(*ax.ValueText)
		if err != nil {
			warnf(log, ctx, "ontology reference: 链行 %d 的 value_text 解析失败（%v），跳过反查", ax.ID, err)
			continue
		}
		hit := false
		for _, m := range members {
			if m == targetID {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		row := RefRowOf(ax, ReferencePositionChainMember)
		out = append(out, row)
	}
	return out
}

// MergePairedRows 成对公理无序对去重：primary = 宾语侧命中行，counterparts = 主语侧命中的
// 对称行（(target,other)）。同一无序对保留 axiom_id 较小者并标记 Paired；异常数据（同一对
// 多行）同样收敛为一行，保证 total 与分页一致。
func MergePairedRows(primary, counterparts []RefRow) []RefRow {
	type pairKey struct{ lo, hi uint32 }

	best := make(map[pairKey]RefRow, len(primary)+len(counterparts))
	pick := func(row RefRow) {
		key := pairKey{lo: row.SubjectID, hi: row.ObjectID}
		if key.lo > key.hi {
			key.lo, key.hi = key.hi, key.lo
		}
		cur, ok := best[key]
		if !ok || row.AxiomID < cur.AxiomID {
			best[key] = row
		}
	}
	for _, row := range primary {
		pick(row)
	}
	for _, row := range counterparts {
		pick(row)
	}

	out := make([]RefRow, 0, len(best))
	for _, row := range best {
		row.Paired = true
		out = append(out, row)
	}
	return out
}

// RefOrderField 反查排序字段（白名单只允许 id / updated_at）
type RefOrderField struct {
	Field string
	Desc  bool
}

// ParseRefOrder 解析 order_by（JSON 数组字符串，"-" 前缀 = 倒序；白名单外忽略）。
// 缺省/非法/白名单空集 → [{-id}]（边是流水语义，稳定分页优先于时间序——§4.2 #7）。
func ParseRefOrder(orderBy string) []RefOrderField {
	defaultOrder := []RefOrderField{{Field: "id", Desc: true}}
	trimmed := strings.TrimSpace(orderBy)
	if trimmed == "" {
		return defaultOrder
	}
	var fields []string
	if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
		return defaultOrder
	}
	out := make([]RefOrderField, 0, len(fields))
	for _, f := range fields {
		name := strings.TrimPrefix(strings.TrimSpace(f), "-")
		if name != "id" && name != "updated_at" {
			continue
		}
		out = append(out, RefOrderField{Field: name, Desc: strings.HasPrefix(f, "-")})
	}
	if len(out) == 0 {
		return defaultOrder
	}
	return out
}

// SortRefRows 按解析后的白名单字段排序（稳定；同 id 不可能重复，末位仍以 id 升序兜底）
func SortRefRows(rows []RefRow, order []RefOrderField) {
	sort.SliceStable(rows, func(i, j int) bool {
		for _, f := range order {
			switch f.Field {
			case "updated_at":
				ti, tj := refRowTime(rows[i]), refRowTime(rows[j])
				if ti == tj {
					continue
				}
				if f.Desc {
					return ti > tj
				}
				return ti < tj
			default: // id
				if rows[i].AxiomID == rows[j].AxiomID {
					continue
				}
				if f.Desc {
					return rows[i].AxiomID > rows[j].AxiomID
				}
				return rows[i].AxiomID < rows[j].AxiomID
			}
		}
		return rows[i].AxiomID < rows[j].AxiomID
	})
}

// refRowTime 更新时间取 Unix 纳秒（nil 按零值——老行未回填 updated_at 时排在最旧端）
func refRowTime(row RefRow) int64 {
	if row.UpdatedAt == nil {
		return 0
	}
	return row.UpdatedAt.UnixNano()
}

// PageRefRows 内存分页（page 从 1 起；越界返回空页）
func PageRefRows(rows []RefRow, page, pageSize int) []RefRow {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= len(rows) {
		return nil
	}
	end := start + pageSize
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end]
}
