package ontology

import (
	"encoding/json"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 数据类型定义 / facet 统一件（05 号文档 §2.4：源 OntDatatypeSupport 的定义半边；
// 词法半边在 lexical.go）。调用方：validator.go（46 规则 / 62 词法链三段）、
// ontology_datatype_service.go（定义保存校验与 diff）、ontology_data_property_service.go
// （42 双轨三分支解析）。

// DatatypeFacet facet 定义项（46 行 JSON {"facet","value"} 的域形态）。
type DatatypeFacet struct {
	Facet string
	Value string
}

// DatatypeDefinition datatype 定义（46 行 value_text JSON 的域形态）：
// 内建基类型 + facet 约束，单层派生（datatype 实体不作基类型，05 号文档 §2.3）。
type DatatypeDefinition struct {
	BaseIRI string
	Facets  []DatatypeFacet
}

// IsEmpty 显式空定义（base 未设且 facets 空）= 清空语义（05 号文档 §4.3）。
func (d DatatypeDefinition) IsEmpty() bool {
	return strings.TrimSpace(d.BaseIRI) == "" && len(d.Facets) == 0
}

// definitionJSON / definitionFacetJSON 固定键序序列化载体（encoding/json 按字段声明序输出）。
type definitionJSON struct {
	BaseIri string              `json:"baseIri"`
	Facets  []definitionFacetJSON `json:"facets"`
}

type definitionFacetJSON struct {
	Facet string `json:"facet"`
	Value string `json:"value"`
}

// FacetWhitelist facet 白名单（10 种；剔除 whiteSpace——词法校验零产出、langRange——
// 语言文本走 62 行 lang 列，与 03 号文档同源理由，05 号文档 §3.2）。
var FacetWhitelist = []string{
	"length", "minLength", "maxLength",
	"pattern",
	"minInclusive", "minExclusive", "maxInclusive", "maxExclusive",
	"totalDigits", "fractionDigits",
}

// IsWhitelistedFacet facet 名是否在白名单内。
func IsWhitelistedFacet(name string) bool {
	for _, f := range FacetWhitelist {
		if f == name {
			return true
		}
	}
	return false
}

// FacetFamilies facet 适用族矩阵（前端 FACET_META 镜像同一矩阵，同源维护）。
// pattern 适用全部族（键缺失即全族适用，与"全部族"语义等价的查表实现）。
var FacetFamilies = map[string][]DatatypeFamily{
	"length":         {FamilyString, FamilyBinary},
	"minLength":      {FamilyString, FamilyBinary},
	"maxLength":      {FamilyString, FamilyBinary},
	"minInclusive":   {FamilyNumericDecimal, FamilyNumericFloat, FamilyTemporal},
	"minExclusive":   {FamilyNumericDecimal, FamilyNumericFloat, FamilyTemporal},
	"maxInclusive":   {FamilyNumericDecimal, FamilyNumericFloat, FamilyTemporal},
	"maxExclusive":   {FamilyNumericDecimal, FamilyNumericFloat, FamilyTemporal},
	"totalDigits":    {FamilyNumericDecimal},
	"fractionDigits": {FamilyNumericDecimal},
}

// familyAllowsFacet facet 是否适用于基类型族（pattern 未入表 = 全族适用）。
func familyAllowsFacet(facet string, family DatatypeFamily) bool {
	families, ok := FacetFamilies[facet]
	if !ok {
		return true // pattern：全部族
	}
	for _, f := range families {
		if f == family {
			return true
		}
	}
	return false
}

// ParseDefinition 46 行 value_text JSON → 定义（脏 JSON / 形态不符 →
// DATATYPE_DEFINITION_JSON_INVALID）。未知键容忍（§1.4 预留 kind 判别键的向前兼容）；
// facets 元素缺 facet 名或非对象按形态损坏处理。
func ParseDefinition(raw string) (DatatypeDefinition, error) {
	probe := strings.TrimSpace(raw)
	if probe == "" {
		return DatatypeDefinition{}, NewError(ReasonDatatypeDefinitionJSONInvalid, "数据类型定义为空")
	}
	var top any
	if err := json.Unmarshal([]byte(probe), &top); err != nil {
		return DatatypeDefinition{}, NewError(ReasonDatatypeDefinitionJSONInvalid, "数据类型定义不是合法 JSON")
	}
	if _, ok := top.(map[string]any); !ok {
		return DatatypeDefinition{}, NewError(ReasonDatatypeDefinitionJSONInvalid, "数据类型定义必须是 JSON 对象")
	}
	var parsed struct {
		BaseIri string               `json:"baseIri"`
		Facets  []definitionFacetJSON `json:"facets"`
	}
	if err := json.Unmarshal([]byte(probe), &parsed); err != nil {
		return DatatypeDefinition{}, NewError(ReasonDatatypeDefinitionJSONInvalid, "数据类型定义结构不符")
	}
	def := DatatypeDefinition{BaseIRI: parsed.BaseIri}
	for _, f := range parsed.Facets {
		if f.Facet == "" {
			return DatatypeDefinition{}, NewError(ReasonDatatypeDefinitionJSONInvalid, "facet 项缺少 facet 名")
		}
		def.Facets = append(def.Facets, DatatypeFacet{Facet: f.Facet, Value: f.Value})
	}
	return def, nil
}

// NormalizeDefinition 归一化并序列化（落库形态）：固定键序 baseIri,facets；facet 名去重
// （后者覆盖，保留首次出现位置）、value trim、facet 名或值空白项剔除、facets 保提交序。
func NormalizeDefinition(def DatatypeDefinition) string {
	norm := definitionJSON{BaseIri: strings.TrimSpace(def.BaseIRI)}
	seen := make(map[string]int, len(def.Facets))
	for _, f := range def.Facets {
		name := strings.TrimSpace(f.Facet)
		value := strings.TrimSpace(f.Value)
		if name == "" || value == "" {
			continue
		}
		if idx, dup := seen[name]; dup {
			norm.Facets[idx].Value = value // 后者覆盖，位置不变
			continue
		}
		seen[name] = len(norm.Facets)
		norm.Facets = append(norm.Facets, definitionFacetJSON{Facet: name, Value: value})
	}
	// facets 空时输出 []（与 null 区分，diff 键稳定）
	if norm.Facets == nil {
		norm.Facets = []definitionFacetJSON{}
	}
	out, err := json.Marshal(norm)
	if err != nil {
		return "{}" // 结构固定不可达；防御占位
	}
	return string(out)
}

// DiffKeyDefinition diff 比对键：NormalizeDefinition 后 facets 按（名,值）字典序重排再序列化
// ——调序提交等值 → 无操作（落库形态仍保提交序，源修订 #7 内化，05 号文档 §4.3）。
func DiffKeyDefinition(def DatatypeDefinition) string {
	var key definitionJSON
	if err := json.Unmarshal([]byte(NormalizeDefinition(def)), &key); err != nil {
		return "{}"
	}
	sort.Slice(key.Facets, func(i, j int) bool {
		if key.Facets[i].Facet != key.Facets[j].Facet {
			return key.Facets[i].Facet < key.Facets[j].Facet
		}
		return key.Facets[i].Value < key.Facets[j].Value
	})
	out, err := json.Marshal(key)
	if err != nil {
		return "{}"
	}
	return string(out)
}

// ValidateDefinition 定义保存校验（05 号文档 §4.2 第 4 步 / §3.2 五规则）：
// base 白名单 → DATATYPE_BASE_INVALID；facet 白名单 / 适用族 / 值词法 → DATATYPE_FACET_INVALID；
// 静态冲突五规则 → DATATYPE_FACET_CONFLICT。空定义（仅声明态）直接通过。
func ValidateDefinition(def DatatypeDefinition) error {
	if def.IsEmpty() {
		return nil
	}
	base, ok := IsBuiltinDatatype(strings.TrimSpace(def.BaseIRI))
	if !ok {
		// "facets 非空而 base_iri 缺失"（§3.2 边界 #5）同码拦截：空串不在白名单
		return NewError(ReasonDatatypeBaseInvalid, "数据类型基类型必须是内建数据类型：%s", strings.TrimSpace(def.BaseIRI))
	}
	for _, f := range def.Facets {
		if err := validateFacetValue(base, f); err != nil {
			return err
		}
	}
	return validateFacetConflicts(def)
}

// validateFacetValue 单 facet 校验：白名单 / 适用族 / 值词法。
func validateFacetValue(base BuiltinDatatype, f DatatypeFacet) error {
	if !IsWhitelistedFacet(f.Facet) {
		return NewError(ReasonDatatypeFacetInvalid, "facet %s 不在白名单内", f.Facet)
	}
	if !familyAllowsFacet(f.Facet, base.Family) {
		return NewError(ReasonDatatypeFacetInvalid, "facet %s 不适用于基类型族 %s", f.Facet, base.Family)
	}
	value := strings.TrimSpace(f.Value)
	switch f.Facet {
	case "length", "minLength", "maxLength", "totalDigits", "fractionDigits":
		if _, ok := parseNonNeg(value); !ok {
			return NewError(ReasonDatatypeFacetInvalid, "facet %s 的值 %q 必须是非负整数", f.Facet, f.Value)
		}
	case "pattern":
		if _, err := regexp.Compile(value); err != nil {
			return NewError(ReasonDatatypeFacetInvalid, "facet pattern 的值 %q 不是合法正则：%v", value, err)
		}
	default: // minInclusive/minExclusive/maxInclusive/maxExclusive：值 ∈ 基类型词法空间
		if !ValidateLexical(base.IRI, value) {
			return NewError(ReasonDatatypeFacetInvalid, "facet %s 的值 %q 不在基类型 %s 的词法空间内", f.Facet, value, base.IRI)
		}
	}
	return nil
}

// parseNonNeg 非负整数词形解析（长度/位数 facet 值口径；ParseUint 已拒负号与小数点）。
func parseNonNeg(value string) (uint64, bool) {
	n, err := strconv.ParseUint(value, 10, 64)
	return n, err == nil
}

// validateFacetConflicts 静态冲突五规则（05 号文档 §3.2 表）。
func validateFacetConflicts(def DatatypeDefinition) error {
	facets := facetMap(def)
	base, _ := IsBuiltinDatatype(strings.TrimSpace(def.BaseIRI))

	// 规则 1/2：min/max 上下界二选一
	if has(facets, "minInclusive") && has(facets, "minExclusive") {
		return NewError(ReasonDatatypeFacetConflict, "minInclusive 与 minExclusive 不能并用（下界二选一）")
	}
	if has(facets, "maxInclusive") && has(facets, "maxExclusive") {
		return NewError(ReasonDatatypeFacetConflict, "maxInclusive 与 maxExclusive 不能并用（上界二选一）")
	}
	// 规则 3：定长与区间二选一
	if has(facets, "length") && (has(facets, "minLength") || has(facets, "maxLength")) {
		return NewError(ReasonDatatypeFacetConflict, "length 与 minLength/maxLength 不能并用（定长与区间二选一）")
	}
	// 规则 4：min > max 恒冲突；min == max 且任一 exclusive 亦冲突（值空间为空）
	if min, minOK := boundFacet(facets, "minInclusive", "minExclusive"); minOK {
		if max, maxOK := boundFacet(facets, "maxInclusive", "maxExclusive"); maxOK {
			cmp, ok := compareFacetValues(base, min.Value, max.Value)
			if !ok {
				return NewError(ReasonDatatypeFacetConflict, "下界 %q 与上界 %q 无法比较", min.Value, max.Value)
			}
			minExclusive := has(facets, "minExclusive")
			maxExclusive := has(facets, "maxExclusive")
			if cmp > 0 || (cmp == 0 && (minExclusive || maxExclusive)) {
				return NewError(ReasonDatatypeFacetConflict,
					"值空间为空：下界 %q%s 上界 %q%s", min.Value,
					map[bool]string{true: "（含）", false: "（不含）"}[!minExclusive],
					max.Value,
					map[bool]string{true: "（含）", false: "（不含）"}[!maxExclusive])
			}
		}
	}
	// 规则 5：minLength > maxLength
	if minLength, ok := nonNegFacet(facets, "minLength"); ok {
		if maxLength, ok2 := nonNegFacet(facets, "maxLength"); ok2 && minLength > maxLength {
			return NewError(ReasonDatatypeFacetConflict, "minLength %d 大于 maxLength %d（值空间为空）", minLength, maxLength)
		}
	}
	return nil
}

func facetMap(def DatatypeDefinition) map[string]string {
	out := make(map[string]string, len(def.Facets))
	for _, f := range def.Facets { // 同名 facet 后者覆盖（与 NormalizeDefinition 口径一致）
		out[strings.TrimSpace(f.Facet)] = strings.TrimSpace(f.Value)
	}
	return out
}

func has(m map[string]string, key string) bool {
	_, ok := m[key]
	return ok
}

// boundFacet 取边界 facet 值（inclusive 优先返回；二者并存已在冲突规则 1/2 拦截）。
func boundFacet(m map[string]string, inclusive, exclusive string) (DatatypeFacet, bool) {
	if v, ok := m[inclusive]; ok {
		return DatatypeFacet{Facet: inclusive, Value: v}, true
	}
	if v, ok := m[exclusive]; ok {
		return DatatypeFacet{Facet: exclusive, Value: v}, true
	}
	return DatatypeFacet{}, false
}

func nonNegFacet(m map[string]string, key string) (uint64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// compareFacetValues 边界值比较（静态冲突规则 4 用；族分派）：
// NUMERIC_DECIMAL 按 big.Rat 精确比较；NUMERIC_FLOAT 按 float64；
// TEMPORAL 按基类型 layout（dateTime 墙钟 / dateTimeStamp 绝对时刻，05 号文档 §3.2）。
func compareFacetValues(base BuiltinDatatype, a, b string) (int, bool) {
	switch base.Family {
	case FamilyNumericDecimal:
		ra, okA := new(big.Rat).SetString(a)
		rb, okB := new(big.Rat).SetString(b)
		if !okA || !okB {
			return 0, false
		}
		return ra.Cmp(rb), true
	case FamilyNumericFloat:
		fa, errA := strconv.ParseFloat(a, 64)
		fb, errB := strconv.ParseFloat(b, 64)
		if errA != nil || errB != nil {
			return 0, false
		}
		switch {
		case fa < fb:
			return -1, true
		case fa > fb:
			return 1, true
		default:
			return 0, true
		}
	case FamilyTemporal:
		return compareTemporal(base.Kind, a, b)
	default:
		return 0, false
	}
}

// compareTemporal 时间边界比较：date 按日历、time 按墙钟秒、dateTime 墙钟（忽略时区偏移，
// 同基类型两侧同解析器、行为确定——源修订 #10 简化口径）、dateTimeStamp 绝对时刻（时区感知）。
func compareTemporal(kind DatatypeKind, a, b string) (int, bool) {
	switch kind {
	case KindDate:
		ta, okA := parseDateComparable(a)
		tb, okB := parseDateComparable(b)
		if !okA || !okB {
			return 0, false
		}
		return strings.Compare(ta, tb), true
	case KindTime:
		ta, okA := parseTimeComparable(a)
		tb, okB := parseTimeComparable(b)
		if !okA || !okB {
			return 0, false
		}
		switch {
		case ta < tb:
			return -1, true
		case ta > tb:
			return 1, true
		default:
			return 0, true
		}
	case KindDateTime:
		ta, okA := parseRFC3339(a)
		tb, okB := parseRFC3339(b)
		if !okA || !okB {
			return 0, false
		}
		// 墙钟键：Format 按 Time 自身时区渲染 → 忽略偏移；定宽字段字典序可比
		return strings.Compare(ta.Format("20060102150405.999999999"), tb.Format("20060102150405.999999999")), true
	case KindDateTimeStamp:
		ta, okA := parseRFC3339(a)
		tb, okB := parseRFC3339(b)
		if !okA || !okB {
			return 0, false
		}
		return ta.Compare(tb), true
	default:
		return 0, false
	}
}

func parseRFC3339(v string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, v)
	return t, err == nil
}

// parseDateComparable xsd:date 日历部分 → 定宽键（异形年/超 4 位年无法定序，比较不成立 →
// facetAllows fail-closed；词法空间仍由 base 词法校验放行）
func parseDateComparable(v string) (string, bool) {
	m := datePattern.FindStringSubmatch(v)
	if m == nil {
		return "", false
	}
	calendar := m[1]
	if !dateCalendarPattern.MatchString(calendar) {
		return "", false
	}
	if _, err := time.Parse("2006-01-02", calendar); err != nil {
		return "", false
	}
	return strings.ReplaceAll(calendar, "-", ""), true
}

// parseTimeComparable xsd:time → 墙钟秒数（含小数秒；忽略时区偏移，同 dateTime 口径）
func parseTimeComparable(v string) (float64, bool) {
	m := timePattern.FindStringSubmatch(v)
	if m == nil {
		return 0, false
	}
	h, _ := strconv.ParseFloat(m[1], 64)
	mi, _ := strconv.ParseFloat(m[2], 64)
	s, _ := strconv.ParseFloat(m[3], 64)
	if m[4] != "" {
		if frac, err := strconv.ParseFloat("0"+m[4], 64); err == nil {
			s += frac
		}
	}
	return h*3600 + mi*60 + s, true
}

// ValidateLexicalDerived 62 断言派生轨词法校验（05 号文档 §2.6）：词形 ∈ base 词法空间，
// 且逐个满足 facet 约束。base 白名单外（不应发生，46 规则已拦）返回 false。
// facet 值假设已过 ValidateDefinition（保存期校验）；运行期正则编译失败等脏形态 fail-closed。
func ValidateLexicalDerived(lexical string, def DatatypeDefinition) bool {
	base, ok := IsBuiltinDatatype(strings.TrimSpace(def.BaseIRI))
	if !ok {
		return false
	}
	if !ValidateLexical(base.IRI, lexical) {
		return false
	}
	for _, f := range def.Facets {
		if !facetAllows(base, f, lexical) {
			return false
		}
	}
	return true
}

// facetAllows 单 facet 词法检查（05 号文档 §3.2 "62 断言 facet 检查规则" 表）。
func facetAllows(base BuiltinDatatype, f DatatypeFacet, lexical string) bool {
	value := strings.TrimSpace(f.Value)
	switch f.Facet {
	case "pattern":
		re, err := regexp.Compile(value)
		if err != nil {
			return false
		}
		return re.MatchString(lexical)
	case "length":
		n, ok := parseNonNeg(value)
		return ok && utf8.RuneCountInString(lexical) == int(n) // 码点计；二进制族按字节计数记为后续精化项（源修订 #9）
	case "minLength":
		n, ok := parseNonNeg(value)
		return ok && utf8.RuneCountInString(lexical) >= int(n)
	case "maxLength":
		n, ok := parseNonNeg(value)
		return ok && utf8.RuneCountInString(lexical) <= int(n)
	case "totalDigits":
		n, ok := parseNonNeg(value)
		return ok && writtenDigits(lexical, false) <= int(n)
	case "fractionDigits":
		n, ok := parseNonNeg(value)
		return ok && writtenDigits(lexical, true) <= int(n)
	case "minInclusive", "minExclusive", "maxInclusive", "maxExclusive":
		cmp, ok := compareFacetValues(base, lexical, value)
		if !ok {
			return false
		}
		switch f.Facet {
		case "minInclusive":
			return cmp >= 0
		case "minExclusive":
			return cmp > 0
		case "maxInclusive":
			return cmp <= 0
		default: // maxExclusive
			return cmp < 0
		}
	default:
		return false // 白名单外（保存期已拦；运行期 fail-closed）
	}
}

// writtenDigits 按写形计位数（源修订 #11 内化，05 号文档 §3.2）：
// totalDigits = 去符号与小数点后的写形位数；fractionDigits = 小数点后写形位数
// （"1.50" → fraction 2、"5." → total 1；XSD"尾零不计"语义记为后续精化项）。
func writtenDigits(lexical string, fractionOnly bool) int {
	s := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(lexical), "+"), "-")
	if idx := strings.IndexByte(s, '.'); idx >= 0 {
		if fractionOnly {
			return len(s) - idx - 1
		}
		return idx + (len(s) - idx - 1)
	}
	if fractionOnly {
		return 0
	}
	return len(s)
}

// ============================================================================
// 42 行双轨：range_iris 归一化三分支（05 号文档 §2.2 核心决策）
// ============================================================================

// RangeRef 42 行的双轨目标形态：内建轨（ValueText 非空）或派生轨（ObjectID 非零），
// IRI 为两轨统一的比对键（diff 与回读投影共用）。
type RangeRef struct {
	IRI       string
	ValueText string // 内建轨：内建 datatype IRI（= IRI）
	ObjectID  uint32 // 派生轨：datatype 实体 id
}

// RangeRow 42 行的存量投影形态（DiffRangeRefs 输入）：IRI 为统一比对键——内建轨取
// value_text、派生轨取实体 IRI（批量反查）；悬空 object_id 行 IRI 置空（diff 删除侧清理）。
type RangeRow struct {
	AxiomID   uint32
	IRI       string
	ValueText string
	ObjectID  uint32
	SortOrder int32
}

// IRIEntityLookup IRI 全局反查（跨本体判定需要全局视野；未命中返回 nil 实体）。
type IRIEntityLookup func(iri string) (*ent.OntEntity, error)

// ResolveRangeTargets range_iris 归一化三分支（05 号文档 §2.2）：
//   - 内建 IRI → value_text 轨（03 期存量形态）；
//   - 命中实体且类型 DATATYPE 且同本体 → object_id 轨；
//   - 命中但类型不符或跨本体 → DATATYPE_RANGE_TARGET_INVALID；
//   - 未知 IRI（非内建、全局无实体）→ DATA_PROPERTY_RANGE_INVALID（03 期既有码）。
//
// 输入须先过 normalizeRangeIRIs（去空白/去重/保序）；同名 IRI 重复输入由调用方去重。
func ResolveRangeTargets(iris []string, ontologyID uint32, lookup IRIEntityLookup) ([]RangeRef, error) {
	out := make([]RangeRef, 0, len(iris))
	for _, iri := range iris {
		if _, ok := IsBuiltinDatatype(iri); ok {
			out = append(out, RangeRef{IRI: iri, ValueText: iri})
			continue
		}
		e, err := lookup(iri)
		if err != nil {
			return nil, err
		}
		if e == nil {
			return nil, NewError(ReasonDataPropertyRangeInvalid, "数据属性值域必须是内建数据类型：%s", iri)
		}
		if nilUint(e.OntologyID) != ontologyID {
			return nil, NewError(ReasonDatatypeRangeTargetInvalid, "数据属性值域引用必须是本本体已声明的数据类型：%s（实体属于其他本体）", iri)
		}
		if e.EntityType == nil || *e.EntityType != EntityTypeDatatype {
			return nil, NewError(ReasonDatatypeRangeTargetInvalid, "数据属性值域引用必须是已声明的数据类型：%s（实体类型不符）", iri)
		}
		out = append(out, RangeRef{IRI: iri, ObjectID: e.ID})
	}
	return out, nil
}
