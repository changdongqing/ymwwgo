package owlio

import (
	"fmt"
	"sort"
	"strings"
)

// Turtle 序列化器（07 号文档 §2.1）：规整输出——固定前缀声明 + 实体命名空间动态前缀
// + 按主语分组 + 谓语 ';' 缩约 + 同谓语宾语 ',' 缩约。只求语义正确、可被对应 parser 再解析。

// writer 前缀表（固定 + 动态 ns0..）。写 IRI 时按最长命名空间匹配。
type turtleWriter struct {
	sb      strings.Builder
	ns      map[string]string // 命名空间 → 前缀（固定 + 动态）
	nsOrder []string          // 声明序
}

// WriteTurtle 序列化三元组（ontologyIRI 非空时作 @base）。
func WriteTurtle(triples []Triple, ontologyIRI string) ([]byte, error) {
	w := &turtleWriter{ns: map[string]string{}}
	for ns, pfx := range map[string]string{
		NSRDF: "rdf", NSRDFS: "rdfs", NSOWL: "owl", NSXSD: "xsd", NSSKOS: "skos",
	} {
		w.ns[ns] = pfx
		w.nsOrder = append(w.nsOrder, ns)
	}
	sort.Strings(w.nsOrder)

	// 按主语分组（保持首次出现序）；谓语保持首次出现序；同谓语宾语列表
	type predRow struct {
		predicate string
		objects   []Term
	}
	var subjectOrder []string
	subjects := map[string][]predRow{}
	subjectTerm := map[string]Term{}
	for _, t := range triples {
		if t.Subject.Kind == TermIRI && t.Subject.Value == "" {
			return nil, fmt.Errorf("三元组缺少主语")
		}
		skey := termKey(t.Subject)
		if _, ok := subjects[skey]; !ok {
			subjectOrder = append(subjectOrder, skey)
			subjectTerm[skey] = t.Subject
		}
		pkey := t.Predicate.Value
		rows := subjects[skey]
		found := false
		for i := range rows {
			if rows[i].predicate == pkey {
				rows[i].objects = append(rows[i].objects, t.Object)
				found = true
				break
			}
		}
		if !found {
			subjects[skey] = append(rows, predRow{predicate: pkey, objects: []Term{t.Object}})
		}
	}

	// 预热：先注册全部 IRI 前缀（@prefix 声明块在三元组之前输出，动态前缀必须就位）
	for _, t := range triples {
		for _, term := range [3]Term{t.Subject, t.Predicate, t.Object} {
			if term.Kind == TermIRI {
				w.prefixFor(term.Value)
			}
		}
	}

	if ontologyIRI != "" {
		w.sb.WriteString("@base <" + escapeTurtleIRI(ontologyIRI) + "> .\n")
	}
	for _, ns := range w.nsOrder {
		w.sb.WriteString(fmt.Sprintf("@prefix %s: <%s> .\n", w.ns[ns], escapeTurtleIRI(ns)))
	}
	w.sb.WriteString("\n")

	for _, skey := range subjectOrder {
		rows := subjects[skey]
		w.sb.WriteString(w.term(subjectTerm[skey]))
		w.sb.WriteString("\n")
		for _, row := range rows {
			w.sb.WriteString("\t" + w.term(Iri(row.predicate)) + " ")
			for i, obj := range row.objects {
				if i > 0 {
					w.sb.WriteString(", ")
				}
				w.sb.WriteString(w.term(obj))
			}
			w.sb.WriteString(" ;\n")
		}
		w.sb.WriteString("\t.\n\n")
	}
	return []byte(w.sb.String()), nil
}

// termKey 三元组项的分组键。
func termKey(t Term) string {
	switch t.Kind {
	case TermLiteral:
		return "L\x00" + t.Value + "\x00" + t.Lang + "\x00" + t.Datatype
	case TermBlank:
		return "B\x00" + t.Value
	default:
		return "I\x00" + t.Value
	}
}

// term 项 → Turtle 词形。
func (w *turtleWriter) term(t Term) string {
	switch t.Kind {
	case TermBlank:
		if !validBlankLabel(t.Value) {
			// 非法标签做稳定映射（同一输入标签恒得同一输出，保证再解析后集合相等）
			return "_:b" + sanitizeBlankLabel(t.Value)
		}
		return "_:" + t.Value
	case TermLiteral:
		out := escapeTurtleString(t.Value)
		if t.Lang != "" {
			return out + "@" + t.Lang
		}
		if t.Datatype != "" && t.Datatype != NSXSD+"string" {
			return out + "^^" + w.term(Iri(t.Datatype))
		}
		return out
	default:
		if pfx := w.prefixFor(t.Value); pfx != "" {
			return pfx
		}
		return "<" + escapeTurtleIRI(t.Value) + ">"
	}
}

// prefixFor 命名空间前缀匹配：命中固定/已声明前缀返回 pfx:local；未声明的重复命名空间
// 注册动态前缀 ns0.. 后返回；不可分派的返回空（全 IRI 输出）。
func (w *turtleWriter) prefixFor(iri string) string {
	for ns, pfx := range w.ns {
		if strings.HasPrefix(iri, ns) {
			local := iri[len(ns):]
			if local != "" && validPNLocal(local) {
				return pfx + ":" + local
			}
			return ""
		}
	}
	ns := iriNamespaceOf(iri)
	if ns == "" {
		return ""
	}
	pfx := fmt.Sprintf("ns%d", len(w.ns)-5)
	w.ns[ns] = pfx
	w.nsOrder = append(w.nsOrder, ns)
	local := iri[len(ns):]
	if local != "" && validPNLocal(local) {
		return pfx + ":" + local
	}
	return ""
}

// iriNamespaceOf IRI 的命名空间（最后 '#' 或 '/' 处截断；# 优先）。
func iriNamespaceOf(iri string) string {
	if idx := lastByteIndex(iri, '#'); idx >= 0 {
		return iri[:idx+1]
	}
	idx := lastByteIndex(iri, '/')
	if idx < 0 {
		return ""
	}
	return iri[:idx+1]
}

// validPNLocal 前缀名 local 段的保守词法（字母/数字/_/- 开头非数字，中间可 -/./字母数字/_）。
func validPNLocal(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		case c == '-' || c == '.':
			if i == 0 || i == len(s)-1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// validBlankLabel 空白节点标签合法性（writer 原样输出，非法标签稳定替换）。
func validBlankLabel(s string) bool {
	return s != "" && !strings.ContainsAny(s, " \t\n\r\"'<>()")
}

// sanitizeBlankLabel 非法空白节点标签 → 合法标签（确定性映射）。
func sanitizeBlankLabel(s string) string {
	var out strings.Builder
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '"', '\'', '<', '>', '(', ')':
			out.WriteByte('_')
		default:
			out.WriteRune(r)
		}
	}
	if out.Len() == 0 {
		return "b"
	}
	return out.String()
}

func escapeTurtleIRI(s string) string {
	r := strings.NewReplacer("\\", "\\\\", ">", "\\>", "\n", "\\n", "\r", "\\r")
	return r.Replace(s)
}

// escapeTurtleString 单行字符串词形（控制字符 \n \r \t 转义 + 引号反斜杠）。
func escapeTurtleString(s string) string {
	var out strings.Builder
	out.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\\':
			out.WriteString(`\\`)
		case '"':
			out.WriteString(`\"`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			out.WriteRune(r)
		}
	}
	return `"` + out.String() + `"`
}
