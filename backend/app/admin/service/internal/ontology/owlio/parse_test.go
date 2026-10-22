package owlio

import (
	"strings"
	"testing"
)

// tripleSet 三元组集合比较辅助（全字段精确相等）。
type tripleSet map[string]struct{}

func tripleKey(t Triple) string {
	return termKey(t.Subject) + "|" + termKey(t.Predicate) + "|" + termKey(t.Object)
}

func toSet(triples []Triple) tripleSet {
	set := tripleSet{}
	for _, t := range triples {
		set[tripleKey(t)] = struct{}{}
	}
	return set
}

func assertTriplesEqual(t *testing.T, got, want []Triple) {
	t.Helper()
	g, w := toSet(got), toSet(want)
	for k := range w {
		if _, ok := g[k]; !ok {
			t.Fatalf("再解析后缺失三元组：%s\n got=%v", prettyTripleKey(k), got)
		}
	}
	for k := range g {
		if _, ok := w[k]; !ok {
			t.Fatalf("再解析后多出三元组：%s", prettyTripleKey(k))
		}
	}
	if len(got) != len(want) {
		t.Fatalf("三元组数量不等：got %d want %d（重复行漂移）", len(got), len(want))
	}
}

func prettyTripleKey(k string) string {
	r := strings.NewReplacer("\x00", "·", "|", "  ⟶  ")
	return r.Replace(k)
}

func TestDetectFormat(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    Format
	}{
		{"RDF/XML 声明头", `<?xml version="1.0"?><rdf:RDF></rdf:RDF>`, FormatRDFXML},
		{"直接元素", `<rdf:RDF>`, FormatRDFXML},
		{"Turtle 前缀", "@prefix ex: <http://e#> .\n", FormatTurtle},
		{"带 BOM 的 Turtle", "\uFEFF@prefix ex: <http://e#> .\n", FormatTurtle},
	}
	for _, c := range cases {
		if got := DetectFormat([]byte(c.content)); got != c.want {
			t.Errorf("%s：got %v want %v", c.name, got, c.want)
		}
	}
}

func TestParseTurtleForms(t *testing.T) {
	t.Run("前缀缩写与 a", func(t *testing.T) {
		triples, err := ParseTurtle([]byte(`
			@prefix ex: <http://example.com/ns#> .
			@prefix owl: <http://www.w3.org/2002/07/owl#> .
			@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
			ex:A a owl:Class ; rdfs:subClassOf ex:B .
		`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		want := []Triple{
			{Subject: Iri("http://example.com/ns#A"), Predicate: Iri(IRIRDFType), Object: Iri(IRIOWLClass)},
			{Subject: Iri("http://example.com/ns#A"), Predicate: Iri(IRIRDFSSubClassOf), Object: Iri("http://example.com/ns#B")},
		}
		assertTriplesEqual(t, triples, want)
	})

	t.Run("四种字面量形态", func(t *testing.T) {
		triples, err := ParseTurtle([]byte(`
			@prefix ex: <http://e#> .
			@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
			ex:s ex:p1 "plain" ; ex:p2 "hello"@zh-CN ; ex:p3 "5"^^xsd:int ;
				 ex:p4 42 ; ex:p5 3.14 ; ex:p6 1.0e9 ; ex:p7 true ;
				 ex:p8 """多
行文本""" ; ex:p9 '单引号' .
		`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		want := map[string]Term{
			"p1": Lit("plain"),
			"p2": LangLit("hello", "zh-CN"),
			"p3": TypedLit("5", "http://www.w3.org/2001/XMLSchema#int"),
			"p4": TypedLit("42", NSXSD+"integer"),
			"p5": TypedLit("3.14", NSXSD+"decimal"),
			"p6": TypedLit("1.0e9", NSXSD+"double"),
			"p7": TypedLit("true", NSXSD+"boolean"),
			"p8": Lit("多\n行文本"),
			"p9": Lit("单引号"),
		}
		seen := map[string]Term{}
		for _, tr := range triples {
			seen[lastIRISegment(tr.Predicate.Value)] = tr.Object
		}
		for k, v := range want {
			got, ok := seen[k]
			if !ok {
				t.Fatalf("缺少 %s 的三元组", k)
			}
			if got.Value != v.Value || got.Lang != v.Lang || got.Datatype != v.Datatype {
				t.Errorf("%s：got %+v want %+v", k, got, v)
			}
		}
	})

	t.Run("空白节点与集合", func(t *testing.T) {
		triples, err := ParseTurtle([]byte(`
			@prefix ex: <http://e#> .
			@prefix owl: <http://www.w3.org/2002/07/owl#> .
			@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
			ex:p owl:propertyChainAxiom ( ex:q ex:r ) .
			[ a owl:AllDisjointClasses ; owl:members ( ex:A ex:B ) ] .
		`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		// 断言要点：链结构完整（first q → rest → first r → rest nil）
		var chainHead string
		for _, tr := range triples {
			if tr.Predicate.Value == IRIOWLPropertyChainAxiom {
				chainHead = tr.Object.Value
			}
		}
		if chainHead == "" {
			t.Fatalf("缺少 propertyChainAxiom")
		}
		rest := chainHead
		var items []string
		for steps := 0; steps < 10; steps++ {
			var first, next string
			var hasFirst, isNil bool
			for _, tr := range triples {
				if tr.Subject.Kind != TermBlank || tr.Subject.Value != rest {
					continue
				}
				switch tr.Predicate.Value {
				case IRIRDFFirst:
					first, hasFirst = tr.Object.Value, true
				case IRIRDFRest:
					isNil = tr.Object.Value == IRIRDFNil
					if !isNil {
						next = tr.Object.Value
					}
				}
			}
			if !hasFirst {
				t.Fatalf("链节点 %s 缺少 first", rest)
			}
			items = append(items, first)
			if isNil {
				break
			}
			rest = next
		}
		if len(items) != 2 || items[0] != "http://e#q" || items[1] != "http://e#r" {
			t.Fatalf("链成员保序失败：%v", items)
		}
		// AllDisjoint 结构存在
		found := false
		for _, tr := range triples {
			if tr.Predicate.Value == IRIRDFType && tr.Object.Value == IRIOWLAllDisjointClasses {
				found = true
			}
		}
		if !found {
			t.Fatalf("缺少 AllDisjointClasses 类型三元组")
		}
	})

	t.Run("注释与 @base 相对 IRI", func(t *testing.T) {
		triples, err := ParseTurtle([]byte(`
			@base <http://base.example.com/ont#> .
			@prefix ex: <http://e#> .
			# 行注释
			<A> ex:p <B> . # 尾注释
		`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if len(triples) != 1 {
			t.Fatalf("应得 1 条三元组，got %d", len(triples))
		}
		if triples[0].Subject.Value != "http://base.example.com/A" ||
			triples[0].Object.Value != "http://base.example.com/B" { // RFC 3986:base 路径最后段被替换
			t.Fatalf("相对 IRI 解析错误：%+v", triples[0])
		}
	})

	t.Run("正反例：未声明前缀拒绝", func(t *testing.T) {
		if _, err := ParseTurtle([]byte(`ex:A a ex:B .`)); err == nil {
			t.Fatalf("未声明前缀应报错")
		}
	})

	t.Run("正反例：未闭合字符串拒绝", func(t *testing.T) {
		if _, err := ParseTurtle([]byte(`<http://e#A> <http://e#p> "unclosed .`)); err == nil {
			t.Fatalf("未闭合字符串应报错")
		}
	})
}

func TestTurtleWriterRoundTrip(t *testing.T) {
	source := []Triple{
		{Subject: Iri("http://example.com/ns#A"), Predicate: Iri(IRIRDFType), Object: Iri(IRIOWLClass)},
		{Subject: Iri("http://example.com/ns#A"), Predicate: Iri(IRIRDFSSubClassOf), Object: Iri("http://example.com/ns#B")},
		{Subject: Iri("http://example.com/ns#A"), Predicate: Iri(IRIRDFSLabel), Object: LangLit("甲", "zh")},
		{Subject: Iri("http://example.com/ns#A"), Predicate: Iri(IRIRDFSLabel), Object: Lit("label")},
		{Subject: Iri("http://example.com/ns#dp"), Predicate: Iri(IRIRDFSRange), Object: Iri(NSXSD + "int")},
		{Subject: Iri("http://example.com/ns#i1"), Predicate: Iri("http://example.com/ns#dp"), Object: TypedLit("5", NSXSD+"int")},
		{Subject: Iri("http://example.com/ns#i1"), Predicate: Iri("http://example.com/ns#dp"), Object: LangLit("值", "zh")},
		{Subject: Iri("http://example.com/ns#i1"), Predicate: Iri(IRIOWLDeprecated), Object: TypedLit("true", NSXSD+"boolean")},
		{Subject: Blank("genid1"), Predicate: Iri(IRIRDFFirst), Object: Iri("http://example.com/ns#q")},
		{Subject: Blank("genid1"), Predicate: Iri(IRIRDFRest), Object: Blank("genid2")},
		{Subject: Blank("genid2"), Predicate: Iri(IRIRDFFirst), Object: Iri("http://example.com/ns#r")},
		{Subject: Blank("genid2"), Predicate: Iri(IRIRDFRest), Object: Iri(IRIRDFNil)},
		{Subject: Iri("http://example.com/ns#p"), Predicate: Iri(IRIOWLPropertyChainAxiom), Object: Blank("genid1")},
	}
	content, err := WriteTurtle(source, "http://example.com/ont#")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	parsed, err := ParseTurtle(content)
	if err != nil {
		t.Fatalf("reparse: %v\ncontent:\n%s", err, content)
	}
	assertTriplesEqual(t, parsed, source)
}

func TestParseRDFXMLForms(t *testing.T) {
	t.Run("类型化节点与嵌套属性", func(t *testing.T) {
		content := `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#"
         xmlns:owl="http://www.w3.org/2002/07/owl#"
         xmlns:ex="http://example.com/ns#">
  <owl:Class rdf:about="http://example.com/ns#A">
    <rdfs:subClassOf rdf:resource="http://example.com/ns#B"/>
    <rdfs:label xml:lang="zh">甲</rdfs:label>
  </owl:Class>
</rdf:RDF>`
		triples, err := ParseRDFXML([]byte(content))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		want := []Triple{
			{Subject: Iri("http://example.com/ns#A"), Predicate: Iri(IRIRDFType), Object: Iri(IRIOWLClass)},
			{Subject: Iri("http://example.com/ns#A"), Predicate: Iri(IRIRDFSSubClassOf), Object: Iri("http://example.com/ns#B")},
			{Subject: Iri("http://example.com/ns#A"), Predicate: Iri(IRIRDFSLabel), Object: LangLit("甲", "zh")},
		}
		assertTriplesEqual(t, triples, want)
	})

	t.Run("parseType Collection 与 datatype", func(t *testing.T) {
		content := `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:owl="http://www.w3.org/2002/07/owl#"
         xmlns:xsd="http://www.w3.org/2001/XMLSchema#"
         xmlns:ex="http://example.com/ns#">
  <rdf:Description rdf:about="http://example.com/ns#p">
    <owl:propertyChainAxiom rdf:parseType="Collection">
      <rdf:Description rdf:about="http://example.com/ns#q"/>
      <rdf:Description rdf:about="http://example.com/ns#r"/>
    </owl:propertyChainAxiom>
    <ex:score rdf:datatype="http://www.w3.org/2001/XMLSchema#int">42</ex:score>
  </rdf:Description>
</rdf:RDF>`
		triples, err := ParseRDFXML([]byte(content))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		foundChain, foundTyped := false, false
		for _, tr := range triples {
			if tr.Predicate.Value == IRIOWLPropertyChainAxiom {
				foundChain = true
			}
			if tr.Predicate.Value == "http://example.com/ns#score" &&
				tr.Object.Datatype == NSXSD+"int" && tr.Object.Value == "42" {
				foundTyped = true
			}
		}
		if !foundChain || !foundTyped {
			t.Fatalf("chain=%v typed=%v triples=%v", foundChain, foundTyped, triples)
		}
		// 集合成员保序 q → r
		firsts := map[string]string{}
		rests := map[string]string{}
		var head string
		for _, tr := range triples {
			if tr.Predicate.Value == IRIOWLPropertyChainAxiom {
				head = tr.Object.Value
			}
			if tr.Subject.Kind == TermBlank && tr.Predicate.Value == IRIRDFFirst {
				firsts[tr.Subject.Value] = tr.Object.Value
			}
			if tr.Subject.Kind == TermBlank && tr.Predicate.Value == IRIRDFRest {
				rests[tr.Subject.Value] = tr.Object.Value
			}
		}
		cur := head
		var items []string
		for cur != "" && cur != IRIRDFNil {
			items = append(items, firsts[cur])
			cur = rests[cur]
		}
		if len(items) != 2 || items[0] != "http://example.com/ns#q" || items[1] != "http://example.com/ns#r" {
			t.Fatalf("集合保序失败：%v", items)
		}
	})

	t.Run("非 RDF 文档拒绝", func(t *testing.T) {
		content := `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><body>hi</body></html>`
		if _, err := ParseRDFXML([]byte(content)); err == nil {
			t.Fatalf("HTML 应被拒绝")
		}
	})

	t.Run("xml:base 相对 IRI", func(t *testing.T) {
		content := `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xml:base="http://base.example.com/ont#">
  <rdf:Description rdf:about="A">
    <rdf:type rdf:resource="http://www.w3.org/2002/07/owl#Class"/>
  </rdf:Description>
</rdf:RDF>`
		triples, err := ParseRDFXML([]byte(content))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if triples[0].Subject.Value != "http://base.example.com/A" { // RFC 3986：base 路径最后段被相对引用替换
			t.Fatalf("xml:base 解析失败：%+v", triples[0])
		}
	})
}

func TestRDFXMLWriterRoundTrip(t *testing.T) {
	source := []Triple{
		{Subject: Iri("http://example.com/ns#A"), Predicate: Iri(IRIRDFType), Object: Iri(IRIOWLClass)},
		{Subject: Iri("http://example.com/ns#A"), Predicate: Iri(IRIRDFSSubClassOf), Object: Iri("http://example.com/ns#B")},
		{Subject: Iri("http://example.com/ns#A"), Predicate: Iri(IRIRDFSLabel), Object: LangLit("甲", "zh")},
		{Subject: Iri("http://example.com/ns#i1"), Predicate: Iri("http://example.com/ns#dp"), Object: TypedLit("5", NSXSD+"int")},
		{Subject: Blank("genid1"), Predicate: Iri(IRIRDFFirst), Object: Iri("http://example.com/ns#q")},
		{Subject: Blank("genid1"), Predicate: Iri(IRIRDFRest), Object: Iri(IRIRDFNil)},
		{Subject: Iri("http://example.com/ns#p"), Predicate: Iri(IRIOWLPropertyChainAxiom), Object: Blank("genid1")},
		{Subject: Iri("http://example.com/ns#A"), Predicate: Iri(IRIRDFSComment), Object: Lit("含 <尖> 与 & 号")},
	}
	content, err := WriteRDFXML(source, "http://example.com/ont#")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	parsed, err := ParseRDFXML(content)
	if err != nil {
		t.Fatalf("reparse: %v\ncontent:\n%s", err, content)
	}
	assertTriplesEqual(t, parsed, source)
}
