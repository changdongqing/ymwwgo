package ontology

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 内建 datatype 词法校验（03 号文档 §4.4）：按 BuiltinDatatype.Kind 分派，Go 标准库实现。
//
// 与源项目的差异（03 号文档 §12 #1）：源基于 OWLAPI `OWL2Datatype.isInLexicalSpace` +
// java.time 兜底，且曾踩"OWLAPI getOWLLiteral 不校验词形"与"xsd:date/time 不在 OWL 2 DL
// datatype map"两个坑；本仓无 OWLAPI，词法校验在本文件显式实现，两坑天然不存在。
// 纪律与源一致：词法校验只在校验器执行（ValidateAxioms 的 62 断言分支），服务层不做词法
// 预校验。05 期 facet 扩展（minInclusive/pattern 等）在本文件之上叠加。
//
// 校验口径为"词法空间"而非"值空间"：如 xsd:integer 接受前导零与前导 +、
// xsd:double 接受 INF/NaN 以外的科学计数——超出口径的边界（24:00:00、闰秒等）按宽松处理，
// 语义矛盾由推理期暴露（03 号文档 §2.7）。

var (
	// xsd:decimal：可选符号 + 数字 + 可选小数（.5 / 5. / 5 均合法）
	decimalPattern = regexp.MustCompile(`^[+-]?(\d+(\.\d*)?|\.\d+)$`)
	// xsd:date：日历部分（捕获组）+ 可选时区（Z | ±HH:MM）
	datePattern = regexp.MustCompile(`^(-?\d{4,}-\d{2}-\d{2})(Z|[+-]\d{2}:\d{2})?$`)
	// xsd:time：HH:MM:SS 可选小数秒 可选时区（Z | ±HH:MM）
	timePattern = regexp.MustCompile(`^(\d{2}):(\d{2}):(\d{2})(\.\d+)?(Z|[+-]\d{2}:\d{2})?$`)
	// xsd:date 的日历部分（time.Parse 精确校验月/日范围用；仅 4 位非负年可走 time.Parse）
	dateCalendarPattern = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)

	// ===== 05 期新增（05 号文档 §3.2 词法实现）=====

	// xsd:language：语言标记（BCP-47 的 XSD 收紧版：段长 1~8，首段纯字母）
	languagePattern = regexp.MustCompile(`^[a-zA-Z]{1,8}(-[a-zA-Z0-9]{1,8})*$`)
	// xsd:Name：首字符 字母/_/:，后续 名称字符（字母/数字/./-/_/:）。XSD 允许全 Unicode
	// 字母（与 entity_factory 的 localNamePattern 同口径用 \p{L}）。
	namePattern = regexp.MustCompile(`^[\p{L}_:][\p{L}\p{N}.\-_:]*$`)
	// xsd:NCName：Name 去冒号（无命名空间前缀的限定名）
	ncNamePattern = regexp.MustCompile(`^[\p{L}_][\p{L}\p{N}.\-_]*$`)
	// xsd:NMTOKEN：名称字符任意混合（≥1 个）
	nmTokenPattern = regexp.MustCompile(`^[\p{L}\p{N}.\-_:]+$`)
	// xsd:hexBinary：偶数个十六进制位（空串 = 0 字节，合法）
	hexBinaryPattern = regexp.MustCompile(`^([0-9A-Fa-f]{2})*$`)
	// xsd:base64Binary 的紧凑形态（剔除 RFC 2045 允许夹入的 ASCII 空白后匹配）：
	// 4 的倍数个 base64 字符，结尾 0/1 个 "="（余 3 字符）或 2 个 "="（余 2 字符）；空串合法
	base64BinaryPattern = regexp.MustCompile(`^([A-Za-z0-9+/]{4})*([A-Za-z0-9+/]{3}=|[A-Za-z0-9+/]{2}==)?$`)
	base64WSReplacer    = strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "")
)

// ValidateLexical 字面量词形是否落在 datatype 的词法空间内。
// iri 为空按 xsd:string 处理（62 断言无值域行时的缺省，03 号文档 §2.6）；
// 白名单外的 datatype 返回 false（调用方决定报错口径）。
func ValidateLexical(datatypeIRI, lexical string) bool {
	dt, ok := IsBuiltinDatatype(datatypeIRI)
	if !ok {
		return false
	}
	return validateLexicalByKind(dt.Kind, lexical)
}

func validateLexicalByKind(kind DatatypeKind, v string) bool {
	switch kind {
	case KindString:
		// 无词法约束（任意 Unicode 文本）
		return true
	case KindBoolean:
		return v == "true" || v == "false" || v == "1" || v == "0"
	case KindDecimal:
		return decimalPattern.MatchString(v)
	case KindInteger:
		_, err := strconv.ParseInt(v, 10, 64)
		return err == nil
	case KindNonNegInteger:
		n, err := strconv.ParseInt(v, 10, 64)
		return err == nil && n >= 0
	case KindNonPosInteger:
		n, err := strconv.ParseInt(v, 10, 64)
		return err == nil && n <= 0
	case KindPositiveInteger:
		n, err := strconv.ParseInt(v, 10, 64)
		return err == nil && n > 0
	case KindNegativeInteger:
		n, err := strconv.ParseInt(v, 10, 64)
		return err == nil && n < 0
	case KindUnsignedLong:
		_, err := strconv.ParseUint(v, 10, 64)
		return err == nil
	case KindUnsignedInt:
		n, err := strconv.ParseUint(v, 10, 64)
		return err == nil && n <= 0xFFFFFFFF
	case KindUnsignedShort:
		n, err := strconv.ParseUint(v, 10, 64)
		return err == nil && n <= 0xFFFF
	case KindUnsignedByte:
		n, err := strconv.ParseUint(v, 10, 64)
		return err == nil && n <= 0xFF
	case KindFloat:
		_, err := strconv.ParseFloat(v, 64)
		return err == nil
	case KindDate:
		return validateLexicalDate(v)
	case KindDateTime:
		// XSD dateTime 要求时区，RFC3339 同构（03 号文档 §4.4）
		_, err := time.Parse(time.RFC3339, v)
		return err == nil
	case KindDateTimeStamp:
		// RFC3339 本身强制时区偏移（ dateTimeStamp = dateTime + 必须时区，05 号文档 §3.2；
		// 与 03 期 dateTime 同口径，独立 Kind 便于后续放宽 dateTime 时差异化）
		_, err := time.Parse(time.RFC3339, v)
		return err == nil
	case KindTime:
		return validateLexicalTime(v)
	case KindNormalizedString:
		// 词法收紧：不得含换行/回车/制表（XSD 全部空白字符替换为空格的源头约束）
		return !strings.ContainsAny(v, "\n\r\t")
	case KindToken:
		// normalizedString 基础上：首尾无空白、无连续空白（多空白折叠为单空格）
		if !validateLexicalByKind(KindNormalizedString, v) {
			return false
		}
		return v == strings.TrimSpace(v) && !strings.Contains(v, "  ")
	case KindLanguage:
		return languagePattern.MatchString(v)
	case KindName:
		return namePattern.MatchString(v)
	case KindNCName:
		return ncNamePattern.MatchString(v)
	case KindNMTOKEN:
		return nmTokenPattern.MatchString(v)
	case KindHexBinary:
		return hexBinaryPattern.MatchString(v)
	case KindBase64Binary:
		// RFC 2045 词法允许在四字符组间夹空白：先剔除空白再匹配紧凑形态
		return base64BinaryPattern.MatchString(base64WSReplacer.Replace(v))
	case KindAnyURI:
		// 非空且无控制字符
		if v == "" {
			return false
		}
		for _, r := range v {
			if r < 0x20 {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// validateLexicalDate xsd:date：正则先行；日历部分为 4 位非负年时再用 time.Parse
// 精确校验月/日范围（如 02-30 拒绝）；异形年（负年/超 4 位）正则通过即接受。
func validateLexicalDate(v string) bool {
	m := datePattern.FindStringSubmatch(v)
	if m == nil {
		return false
	}
	calendar := m[1]
	if dateCalendarPattern.MatchString(calendar) {
		_, err := time.Parse("2006-01-02", calendar)
		return err == nil
	}
	return true // 负年/超 4 位年：正则已过，接受（词法空间内）
}

// validateLexicalTime xsd:time：正则 + 时分秒范围轻校验
// （时 ≤ 24——24 仅整点零分 24:00:00 合法；分 ≤ 59；秒 ≤ 60——含闰秒）。
func validateLexicalTime(v string) bool {
	m := timePattern.FindStringSubmatch(v)
	if m == nil {
		return false
	}
	hour, _ := strconv.Atoi(m[1])
	minute, _ := strconv.Atoi(m[2])
	second, _ := strconv.Atoi(m[3])
	if hour == 24 {
		// 24:00:00（可选全零小数秒）= 当日结束的合法词形；24:00:01 等非法
		if minute != 0 || second != 0 {
			return false
		}
		frac := strings.TrimPrefix(m[4], ".")
		return frac == "" || strings.Trim(frac, "0") == ""
	}
	return hour <= 23 && minute <= 59 && second <= 60
}
