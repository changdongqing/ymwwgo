package owlio

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
)

// RDF/XML 序列化器（07 号文档 §2.1）：规整平铺形态——每实体（主语）一个 rdf:Description
// 节点元素、每三元组一个属性元素（子元素或 rdf:resource 属性）、空白节点 rdf:nodeID。
// 不追求 striped 美观，只求语义正确、可被对应 parser 再解析（Protégé 同样可打开）。

type rdfxmlWriter struct {
	sb      strings.Builder
	ns      map[string]string // 谓词命名空间 → 前缀（固定 + 动态）
	nsOrder []string
}

// WriteRDFXML 序列化三元组（ontologyIRI 非空时作 xml:base）。
func WriteRDFXML(triples []Triple, ontologyIRI string) ([]byte, error) {
	w := &rdfxmlWriter{ns: map[string]string{}}
	for ns, pfx := range map[string]string{
		NSRDF: "rdf", NSRDFS: "rdfs", NSOWL: "owl", NSXSD: "xsd", NSSKOS: "skos",
	} {
		w.ns[ns] = pfx
		w.nsOrder = append(w.nsOrder, ns)
	}
	sort.Strings(w.nsOrder)

	// 按主语分组（保持首次出现序）
	type predRow struct {
		predicate string
		objects   []Term
	}
	var subjectOrder []string
	subjects := map[string][]predRow{}
	subjectTerm := map[string]Term{}
	for _, t := range triples {
		if t.Subject.Kind != TermIRI && t.Subject.Kind != TermBlank {
			return nil, fmt.Errorf("RDF/XML 不支持字面量主语：%v", t)
		}
		skey := termKey(t.Subject)
		if _, ok := subjects[skey]; !ok {
			subjectOrder = append(subjectOrder, skey)
			subjectTerm[skey] = t.Subject
		}
		rows := subjects[skey]
		found := false
		for i := range rows {
			if rows[i].predicate == t.Predicate.Value {
				rows[i].objects = append(rows[i].objects, t.Object)
				found = true
				break
			}
		}
		if !found {
			subjects[skey] = append(rows, predRow{predicate: t.Predicate.Value, objects: []Term{t.Object}})
		}
	}

	// 预热：谓词命名空间全部注册（xmlns 声明在根元素上一次性输出）
	for _, t := range triples {
		w.prefixFor(t.Predicate.Value)
	}

	w.sb.WriteString(xml.Header)
	w.sb.WriteString(`<rdf:RDF`)
	for _, ns := range w.nsOrder {
		w.sb.WriteString(fmt.Sprintf(` xmlns:%s="%s"`, w.ns[ns], escapeXMLAttr(ns)))
	}
	if ontologyIRI != "" {
		w.sb.WriteString(fmt.Sprintf(` xml:base="%s"`, escapeXMLAttr(ontologyIRI)))
	}
	w.sb.WriteString(">\n")

	for _, skey := range subjectOrder {
		rows := subjects[skey]
		subject := subjectTerm[skey]
		switch subject.Kind {
		case TermBlank:
			w.sb.WriteString(fmt.Sprintf("  <rdf:Description rdf:nodeID=\"%s\">\n", escapeXMLAttr(sanitizeBlankLabel(subject.Value))))
		default:
			w.sb.WriteString(fmt.Sprintf("  <rdf:Description rdf:about=\"%s\">\n", escapeXMLAttr(subject.Value)))
		}
		for _, row := range rows {
			qname, err := w.predicateQName(row.predicate)
			if err != nil {
				return nil, err
			}
			for _, obj := range row.objects {
				switch obj.Kind {
				case TermIRI:
					w.sb.WriteString(fmt.Sprintf("    <%s rdf:resource=\"%s\"/>\n", qname, escapeXMLAttr(obj.Value)))
				case TermBlank:
					w.sb.WriteString(fmt.Sprintf("    <%s rdf:nodeID=\"%s\"/>\n", qname, escapeXMLAttr(sanitizeBlankLabel(obj.Value))))
				default:
					attrs := ""
					if obj.Datatype != "" {
						attrs += fmt.Sprintf(` rdf:datatype="%s"`, escapeXMLAttr(obj.Datatype))
					}
					if obj.Lang != "" {
						attrs += fmt.Sprintf(` xml:lang="%s"`, escapeXMLAttr(obj.Lang))
					}
					w.sb.WriteString(fmt.Sprintf("    <%s%s>%s</%s>\n", qname, attrs, escapeXMLText(obj.Value), qname))
				}
			}
		}
		w.sb.WriteString("  </rdf:Description>\n")
	}
	w.sb.WriteString("</rdf:RDF>\n")
	return []byte(w.sb.String()), nil
}

// predicateQName 谓词 IRI → 前缀名（local 必须是合法 XML 元素名，否则报导出失败）。
func (w *rdfxmlWriter) predicateQName(iri string) (string, error) {
	pfx := w.prefixFor(iri)
	if pfx == "" {
		return "", fmt.Errorf("谓词 IRI 无法表达为 RDF/XML 属性元素：%s", iri)
	}
	return pfx, nil
}

// prefixFor 命名空间前缀（固定 + 动态 ns0..；local 段合法性在谓词侧校验）。
func (w *rdfxmlWriter) prefixFor(iri string) string {
	for ns, pfx := range w.ns {
		if strings.HasPrefix(iri, ns) {
			local := iri[len(ns):]
			if local != "" && validXMLName(local) {
				return pfx + ":" + local
			}
			return ""
		}
	}
	ns := iriNamespaceOf(iri)
	if ns == "" {
		return ""
	}
	pfx, ok := w.ns[ns]
	if !ok {
		pfx = fmt.Sprintf("ns%d", len(w.ns)-5)
		w.ns[ns] = pfx
		w.nsOrder = append(w.nsOrder, ns)
	}
	local := iri[len(ns):]
	if local != "" && validXMLName(local) {
		return pfx + ":" + local
	}
	return ""
}

// validXMLName XML 元素名合法性（NCName 保守版：字母/_ 开头，字母/数字/_/-/.）。
func validXMLName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_', c >= 0x80:
		case c >= '0' && c <= '9', c == '-', c == '.':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func escapeXMLAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "\n", "&#10;", "\r", "&#13;", "\t", "&#9;")
	return r.Replace(s)
}

func escapeXMLText(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
