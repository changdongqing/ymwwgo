package owlio

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// RDF/XML 解析器（07 号文档 §2.1）：encoding/xml 递归——rdf:Description / 类型化节点元素、
// rdf:about / rdf:resource / rdf:nodeID / rdf:datatype / xml:lang / rdf:parseType="Collection"，
// 嵌套属性元素。xml:base 支持；相对 IRI 按 base（缺省按文档位置）解析。

type rdfxmlParser struct {
	dec      *xml.Decoder
	base     string
	blankSeq int
	blanks   map[string]Term // rdf:nodeID label → blank term（同 label 复用）
	triples  []Triple
}

// ParseRDFXML 解析 RDF/XML 文本为三元组（保持文件出现序）。
func ParseRDFXML(content []byte) ([]Triple, error) {
	p := &rdfxmlParser{dec: xml.NewDecoder(strings.NewReader(string(content))), blanks: map[string]Term{}}
	if err := p.parseDocument(); err != nil {
		return nil, err
	}
	return p.triples, nil
}

func (p *rdfxmlParser) errf(format string, args ...interface{}) error {
	return fmt.Errorf("RDF/XML 解析失败：%s", sprintf(format, args...))
}

func (p *rdfxmlParser) freshBlank() Term {
	p.blankSeq++
	return Blank(fmt.Sprintf("genid%d", p.blankSeq))
}

func (p *rdfxmlParser) blankByLabel(label string) Term {
	if t, ok := p.blanks[label]; ok {
		return t
	}
	t := p.freshBlank()
	p.blanks[label] = t
	return t
}

// parseDocument 读到根元素，分发节点元素。
func (p *rdfxmlParser) parseDocument() error {
	for {
		tok, err := p.dec.Token()
		if err == io.EOF {
			return p.errf("缺少根元素")
		}
		if err != nil {
			return p.errf("%v", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		// xml:base 属性提升
		for _, a := range se.Attr {
			if a.Name.Space == XMLNSLang && a.Name.Local == "base" {
				p.base = a.Value
			}
		}
		name := p.elementIRI(se.Name)
		if name == IRIRDFRDF {
			// 标准 rdf:RDF 根：子元素为节点元素
			if err := p.parseNodeChildren(); err != nil {
				return err
			}
			return nil
		}
		// 单节点文档（无 rdf:RDF 包装）：根即节点元素；明显非 RDF 词汇（HTML 等）拒绝
		if !strings.HasPrefix(name, NSRDF) && !strings.HasPrefix(name, NSRDFS) &&
			!strings.HasPrefix(name, NSOWL) && !strings.HasPrefix(name, NSSKOS) {
			return p.errf("不是 RDF/XML 文档（根元素 %s）", se.Name.Local)
		}
		if _, err := p.parseNodeElement(se); err != nil {
			return err
		}
		return nil
	}
}

// parseNodeChildren 读 rdf:RDF 内的连续节点元素。
func (p *rdfxmlParser) parseNodeChildren() error {
	for {
		tok, err := p.dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return p.errf("%v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if _, err := p.parseNodeElement(t); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// parseNodeElement 节点元素：subject 取 rdf:about / rdf:nodeID / fresh blank；
// 元素名非 rdf:Description 时产 rdf:type；子元素交给属性元素处理。返回节点主语。
func (p *rdfxmlParser) parseNodeElement(se xml.StartElement) (Term, error) {
	subject := Term{}
	about := ""
	nodeID := ""
	for _, a := range se.Attr {
		switch {
		case a.Name.Space == NSRDF && a.Name.Local == "about":
			about = a.Value
		case a.Name.Space == NSRDF && a.Name.Local == "nodeID":
			nodeID = a.Value
		case a.Name.Space == XMLNSLang && a.Name.Local == "base":
			if p.base == "" {
				p.base = a.Value
			}
		}
	}
	switch {
	case about != "":
		subject = Iri(p.resolveIRI(about))
	case nodeID != "":
		subject = p.blankByLabel(nodeID)
	default:
		subject = p.freshBlank()
	}

	if name := p.elementIRI(se.Name); name != IRIRDFDescription {
		p.triples = append(p.triples, Triple{Subject: subject, Predicate: Iri(IRIRDFType), Object: Iri(name)})
	}

	for {
		tok, err := p.dec.Token()
		if err == io.EOF {
			return subject, nil
		}
		if err != nil {
			return subject, p.errf("%v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if err := p.parsePropertyElement(subject, t); err != nil {
				return subject, err
			}
		case xml.EndElement:
			return subject, nil
		case xml.CharData:
			// 节点元素内的游离文本忽略
		}
	}
}

// parsePropertyElement 属性元素：谓词 = 元素 IRI；宾语按 rdf:resource / rdf:nodeID /
// 嵌套节点元素 / 文本字面量 / parseType 分派。
func (p *rdfxmlParser) parsePropertyElement(subject Term, se xml.StartElement) error {
	predicate := p.elementIRI(se.Name)

	resource := ""
	nodeID := ""
	parseType := ""
	datatype := ""
	lang := ""
	for _, a := range se.Attr {
		switch {
		case a.Name.Space == NSRDF && a.Name.Local == "resource":
			resource = a.Value
		case a.Name.Space == NSRDF && a.Name.Local == "nodeID":
			nodeID = a.Value
		case a.Name.Space == NSRDF && a.Name.Local == "parseType":
			parseType = a.Value
		case a.Name.Space == NSRDF && a.Name.Local == "datatype":
			datatype = a.Value
		case a.Name.Space == XMLNSLang && a.Name.Local == "lang":
			lang = a.Value
		}
	}

	switch {
	case resource != "":
		p.triples = append(p.triples, Triple{Subject: subject, Predicate: Iri(predicate), Object: Iri(p.resolveIRI(resource))})
		// 自闭合属性元素；若有 EndElement 由下方 Token 循环消费
		return p.skipUntilClose(se)
	case nodeID != "":
		p.triples = append(p.triples, Triple{Subject: subject, Predicate: Iri(predicate), Object: p.blankByLabel(nodeID)})
		return p.skipUntilClose(se)
	case parseType == "Collection":
		return p.parseCollection(subject, predicate)
	case parseType == "Resource":
		obj := p.freshBlank()
		p.triples = append(p.triples, Triple{Subject: subject, Predicate: Iri(predicate), Object: obj})
		for {
			tok, err := p.dec.Token()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return p.errf("%v", err)
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if err := p.parsePropertyElement(obj, t); err != nil {
					return err
				}
			case xml.EndElement:
				return nil
			}
		}
	}

	// 文本 / 嵌套节点元素
	var text strings.Builder
	hasText := false
	for {
		tok, err := p.dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return p.errf("%v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			// 嵌套节点元素：对象为该节点主语
			obj, err := p.parseNodeElement(t)
			if err != nil {
				return err
			}
			p.triples = append(p.triples, Triple{Subject: subject, Predicate: Iri(predicate), Object: obj})
			return nil
		case xml.CharData:
			text.Write(t)
			hasText = true
		case xml.EndElement:
			goto done
		}
	}
done:
	if hasText {
		value := strings.TrimSpace(text.String())
		if datatype != "" {
			p.triples = append(p.triples, Triple{Subject: subject, Predicate: Iri(predicate), Object: TypedLit(value, p.resolveIRI(datatype))})
		} else if lang != "" {
			p.triples = append(p.triples, Triple{Subject: subject, Predicate: Iri(predicate), Object: LangLit(value, lang)})
		} else {
			p.triples = append(p.triples, Triple{Subject: subject, Predicate: Iri(predicate), Object: Lit(value)})
		}
	}
	return nil
}

// parseCollection parseType="Collection"：子节点元素序列 → rdf:first/rest 链。
func (p *rdfxmlParser) parseCollection(subject Term, predicate string) error {
	var items []Term
	for {
		tok, err := p.dec.Token()
		if err == io.EOF {
			return p.errf("集合未闭合")
		}
		if err != nil {
			return p.errf("%v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			item, err := p.parseNodeElement(t)
			if err != nil {
				return err
			}
			items = append(items, item)
		case xml.EndElement:
			if len(items) == 0 {
				p.triples = append(p.triples, Triple{Subject: subject, Predicate: Iri(predicate), Object: Iri(IRIRDFNil)})
				return nil
			}
			head := p.freshBlank()
			p.triples = append(p.triples, Triple{Subject: subject, Predicate: Iri(predicate), Object: head})
			cur := head
			for i, item := range items {
				p.triples = append(p.triples, Triple{Subject: cur, Predicate: Iri(IRIRDFFirst), Object: item})
				if i == len(items)-1 {
					p.triples = append(p.triples, Triple{Subject: cur, Predicate: Iri(IRIRDFRest), Object: Iri(IRIRDFNil)})
				} else {
					next := p.freshBlank()
					p.triples = append(p.triples, Triple{Subject: cur, Predicate: Iri(IRIRDFRest), Object: next})
					cur = next
				}
			}
			return nil
		}
	}
}

// skipUntilClose 自闭合/空属性元素的残余 token 消费（EndElement）。
func (p *rdfxmlParser) skipUntilClose(se xml.StartElement) error {
	if se.Name.Space == "" && se.Name.Local == "" {
		return nil
	}
	for {
		tok, err := p.dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return p.errf("%v", err)
		}
		if _, ok := tok.(xml.EndElement); ok {
			return nil
		}
	}
}

// elementIRI 元素名 → IRI（Space 为命名空间 URI；无命名空间且 Local 命中 rdf 核心词按 rdf 处理）。
func (p *rdfxmlParser) elementIRI(name xml.Name) string {
	if name.Space == "" {
		switch name.Local {
		case "RDF", "Description", "type", "about", "resource", "nodeID", "datatype", "parseType", "ID":
			return NSRDF + name.Local
		}
		return name.Local
	}
	return name.Space + name.Local
}

// resolveIRI 相对 IRI 按 xml:base 解析。
func (p *rdfxmlParser) resolveIRI(iri string) string {
	if iri == "" {
		return p.base
	}
	if u, err := url.Parse(iri); err == nil && u.IsAbs() {
		return iri
	}
	if p.base == "" {
		return iri
	}
	base, err := url.Parse(p.base)
	if err != nil {
		return iri
	}
	ref, err := url.Parse(iri)
	if err != nil {
		return iri
	}
	return base.ResolveReference(ref).String()
}
