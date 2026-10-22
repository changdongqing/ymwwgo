package ontology

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 对象属性域内核增量（02 期）：属性链编解码与投影、链成员反查。
// 设计文档：docs/ontology/design/02-对象属性详细设计.md §3.1 / §4.3 / §4.4。

// EncodeChainMemberIDs 链成员序列 → value_text（JSON 字符串数组，保序不去重，如 ["12","13"]）。
func EncodeChainMemberIDs(ids []uint32) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	b, err := json.Marshal(parts)
	if err != nil {
		return "" // strconv 产物恒可序列化，防御分支
	}
	return string(b)
}

// ParseChainMemberIDs value_text → 链成员序列（保序不去重）。
// 主形态为 JSON 字符串数组；直改库写入的数字数组同样接受（防御兼容）。
// 空串/非法 JSON/元素非十进制整数 → 错误（校验器报 AXIOM_INVALID，usage 查询按可解析行处理）。
func ParseChainMemberIDs(valueText string) ([]uint32, error) {
	trimmed := strings.TrimSpace(valueText)
	if trimmed == "" {
		return nil, NewError(ReasonAxiomInvalid, "属性链成员为空")
	}
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, NewError(ReasonAxiomInvalid, "属性链成员不是合法 JSON 数组：%s", trimmed)
	}
	out := make([]uint32, 0, len(raw))
	for _, el := range raw {
		var s string
		if err := json.Unmarshal(el, &s); err != nil {
			// 数字元素（直改库形态）
			var n float64
			if err2 := json.Unmarshal(el, &n); err2 != nil || n != float64(uint32(n)) {
				return nil, NewError(ReasonAxiomInvalid, "属性链成员 %s 不是实体 id", string(el))
			}
			out = append(out, uint32(n))
			continue
		}
		v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
		if err != nil {
			return nil, NewError(ReasonAxiomInvalid, "属性链成员 %q 不是实体 id", s)
		}
		out = append(out, uint32(v))
	}
	return out, nil
}

// PropertyChain 属性链投影（37 型公理行；changeset.go 的 DiffChains 以此为输入）。
// 放本文件避免 changeset.go 与链编解码互相纠缠。AxiomID=0 表示新增链（无行 id）。
type PropertyChain struct {
	AxiomID   uint32
	MemberIDs []uint32 // 保序、不去重（允许 P∘P，允许包含超属性自身——OWL 2 语法合法）
}

// PropertyChainsOf 投影某超属性名下的 37 型链行（diff 基准 / frame 回显共用）。
// 解析失败的行保留条目（MemberIDs=nil）：diff 视为"未被引用"随后续保存删除（自愈）；
// 校验器对同一行报 AXIOM_INVALID（§4.4）。
func PropertyChainsOf(ctx context.Context, axioms []*ent.OntAxiom, superPropertyID uint32, log Logger) []PropertyChain {
	out := make([]PropertyChain, 0)
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != AxiomSubPropertyChainOf {
			continue
		}
		if ax.SubjectID == nil || *ax.SubjectID != superPropertyID {
			continue
		}
		pc := PropertyChain{AxiomID: ax.ID}
		if ax.ValueText != nil {
			if members, err := ParseChainMemberIDs(*ax.ValueText); err == nil {
				pc.MemberIDs = members
			} else {
				warnf(log, ctx, "ontology chain: 公理 %d 的 value_text 解析失败（%v），按空链处理", ax.ID, err)
			}
		}
		out = append(out, pc)
	}
	return out
}

// ChainRef 链行反查命中：超属性（引用方）+ 链公理行。
type ChainRef struct {
	SuperPropertyID uint32
	AxiomID         uint32
}

// ChainReferrers 链成员反查：value_text 包含 targetID 的 37 型行（usage 反查与级联删除用，
// §4.3——链行数量小，全量扫描可接受）。解析失败行 WARN 跳过（§7 #17）。
func ChainReferrers(ctx context.Context, axioms []*ent.OntAxiom, targetID uint32, log Logger) []ChainRef {
	var out []ChainRef
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != AxiomSubPropertyChainOf {
			continue
		}
		if ax.SubjectID == nil || ax.ValueText == nil {
			continue
		}
		members, err := ParseChainMemberIDs(*ax.ValueText)
		if err != nil {
			warnf(log, ctx, "ontology chain: usage 反查跳过解析失败的链行 %d（%v）", ax.ID, err)
			continue
		}
		for _, m := range members {
			if m == targetID {
				out = append(out, ChainRef{SuperPropertyID: *ax.SubjectID, AxiomID: ax.ID})
				break
			}
		}
	}
	return out
}

// CharacteristicShortName 特征 IRI 的紧缩渲染名（owl:TransitiveProperty → transitive）；
// 白名单外返回完整 IRI。
func CharacteristicShortName(iri string) string {
	const owlPrefix = "http://www.w3.org/2002/07/owl#"
	fragment := strings.TrimPrefix(iri, owlPrefix)
	name := strings.TrimSuffix(fragment, "Property")
	if name == "" || name == fragment {
		return iri
	}
	return strings.ToLower(name[:1]) + name[1:]
}
