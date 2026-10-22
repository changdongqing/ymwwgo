package ontology

import "testing"

// 03 期内建 datatype 词法校验单测（03 号文档 §8：逐 Kind 词法用例）。

func TestValidateLexical_String(t *testing.T) {
	for _, v := range []string{"", "hello", "中文文本", "multi\nline", "  spaced  "} {
		if !ValidateLexical(IRIXSDNamespace+"string", v) {
			t.Fatalf("string 无词法约束，%q 应通过", v)
		}
	}
}

func TestValidateLexical_Boolean(t *testing.T) {
	// 四形态：true/false/1/0
	for _, v := range []string{"true", "false", "1", "0"} {
		if !ValidateLexical(IRIXSDNamespace+"boolean", v) {
			t.Fatalf("boolean %q 应通过", v)
		}
	}
	for _, v := range []string{"TRUE", "True", "yes", "2", "", " true"} {
		if ValidateLexical(IRIXSDNamespace+"boolean", v) {
			t.Fatalf("boolean %q 应拒绝", v)
		}
	}
}

func TestValidateLexical_Decimal(t *testing.T) {
	for _, v := range []string{"0", "5", "+5", "-5", "5.25", "-5.25", ".5", "5.", "0.0"} {
		if !ValidateLexical(IRIXSDNamespace+"decimal", v) {
			t.Fatalf("decimal %q 应通过", v)
		}
	}
	for _, v := range []string{"", "5,25", "1e3", "abc", "--5", "5.5.5"} {
		if ValidateLexical(IRIXSDNamespace+"decimal", v) {
			t.Fatalf("decimal %q 应拒绝", v)
		}
	}
}

func TestValidateLexical_Integer(t *testing.T) {
	for _, v := range []string{"0", "42", "+7", "-1", "007"} {
		if !ValidateLexical(IRIXSDNamespace+"integer", v) {
			t.Fatalf("integer %q 应通过", v)
		}
	}
	// 负数合法、小数/非数字拒绝、64 位溢出拒绝
	if !ValidateLexical(IRIXSDNamespace+"long", "-9223372036854775808") {
		t.Fatal("long 下界应通过")
	}
	for _, v := range []string{"abc", "1.5", "", "9223372036854775808"} { // 2^63 溢出
		if ValidateLexical(IRIXSDNamespace+"integer", v) {
			t.Fatalf("integer %q 应拒绝", v)
		}
	}
	// byte/int/short/long 同 Kind：64 位解析通过即可（宽度语义差异 V1 不细分）
	if !ValidateLexical(IRIXSDNamespace+"byte", "300") {
		t.Fatal("byte 按 64 位解析（V1 不细分宽度），300 应通过")
	}
}

func TestValidateLexical_NonNegativeInteger(t *testing.T) {
	for _, v := range []string{"0", "3", "0042"} {
		if !ValidateLexical(IRIXSDNamespace+"nonNegativeInteger", v) {
			t.Fatalf("nonNegativeInteger %q 应通过", v)
		}
	}
	for _, v := range []string{"-1", "-0.1", "abc"} {
		if ValidateLexical(IRIXSDNamespace+"nonNegativeInteger", v) {
			t.Fatalf("nonNegativeInteger %q 应拒绝", v)
		}
	}
}

func TestValidateLexical_Float(t *testing.T) {
	for _, v := range []string{"0", "3.14", "-2.5", "1e10", "1.5E-3", "+0.5"} {
		if !ValidateLexical(IRIXSDNamespace+"float", v) {
			t.Fatalf("float %q 应通过", v)
		}
		if !ValidateLexical(IRIXSDNamespace+"double", v) {
			t.Fatalf("double %q 应通过", v)
		}
	}
	for _, v := range []string{"", "abc", "1.2.3"} {
		if ValidateLexical(IRIXSDNamespace+"float", v) {
			t.Fatalf("float %q 应拒绝", v)
		}
	}
}

func TestValidateLexical_Date(t *testing.T) {
	// 无时区 / 带时区（Z 与 ±HH:MM）
	for _, v := range []string{"2026-10-22", "2026-10-22Z", "2026-10-22+08:00", "2026-10-22-05:30", "0001-01-01"} {
		if !ValidateLexical(IRIXSDNamespace+"date", v) {
			t.Fatalf("date %q 应通过", v)
		}
	}
	// 月份/日范围由 time.Parse 精确校验
	for _, v := range []string{"2026-02-30", "2026-13-01", "2026-00-10", "20261022", "2026-10", "", "abc"} {
		if ValidateLexical(IRIXSDNamespace+"date", v) {
			t.Fatalf("date %q 应拒绝", v)
		}
	}
	// 负年/超 4 位年：正则通过即接受（词法空间内）
	if !ValidateLexical(IRIXSDNamespace+"date", "-0500-01-01") {
		t.Fatal("负年 date 应通过（词法空间内）")
	}
}

func TestValidateLexical_DateTime(t *testing.T) {
	// RFC3339 同构（要求时区）
	for _, v := range []string{"2026-10-22T09:30:00Z", "2026-10-22T09:30:00+08:00", "2026-10-22T09:30:00.123456Z"} {
		if !ValidateLexical(IRIXSDNamespace+"dateTime", v) {
			t.Fatalf("dateTime %q 应通过", v)
		}
	}
	// 缺时区 / 缺时间部分 / 纯日期
	for _, v := range []string{"2026-10-22T09:30:00", "2026-10-22", "2026-10-22 09:30:00Z"} {
		if ValidateLexical(IRIXSDNamespace+"dateTime", v) {
			t.Fatalf("dateTime %q 应拒绝", v)
		}
	}
}

func TestValidateLexical_Time(t *testing.T) {
	// 带毫秒 / 带 Z 时区 / 带偏移时区 / 24:00:00（XSD 合法词形）/ 闰秒 60
	for _, v := range []string{"09:30:00", "09:30:00.123", "09:30:00Z", "09:30:00+08:00", "23:59:59.999-05:00", "24:00:00", "23:59:60"} {
		if !ValidateLexical(IRIXSDNamespace+"time", v) {
			t.Fatalf("time %q 应通过", v)
		}
	}
	for _, v := range []string{"24:00:01", "09:60:00", "09:30", "9:30:00", "", "abc", "09:30:00."} {
		if ValidateLexical(IRIXSDNamespace+"time", v) {
			t.Fatalf("time %q 应拒绝", v)
		}
	}
}

func TestValidateLexical_AnyURI(t *testing.T) {
	for _, v := range []string{"http://example.com/x", "urn:isbn:0451450523", "/relative/path", "中文URI"} {
		if !ValidateLexical(IRIXSDNamespace+"anyURI", v) {
			t.Fatalf("anyURI %q 应通过", v)
		}
	}
	// 空串拒绝；控制字符拒绝
	for _, v := range []string{"", "a\nb", "a\tb"} {
		if ValidateLexical(IRIXSDNamespace+"anyURI", v) {
			t.Fatalf("anyURI %q 应拒绝", v)
		}
	}
}

func TestValidateLexical_UnknownDatatype(t *testing.T) {
	// 白名单外 datatype 一律拒绝（rdf:langString 不进值域白名单，03 号文档 §3.2）
	if ValidateLexical(IRIRDFLangString, "anything") {
		t.Fatal("rdf:langString 应拒绝（不走值域声明）")
	}
	if ValidateLexical("http://example.com/unknown", "x") {
		t.Fatal("未知 datatype 应拒绝")
	}
	// 空 datatype 不在白名单（62 断言的缺省 xsd:string 由 datatypeRangeOf 兜底，不走本函数）
	if ValidateLexical("", "任意文本") {
		t.Fatal("空 datatype 应拒绝（白名单外）")
	}
}

func TestBuiltinDatatypes_Whitelist(t *testing.T) {
	// 白名单 31 项（05 期接管扩展 15→31，05 号文档 §3.2；03 期既有 15 项 IRI/Kind 不变）
	if len(BuiltinDatatypes) != 31 {
		t.Fatalf("白名单应为 31 项：%d", len(BuiltinDatatypes))
	}
	if dt, ok := IsBuiltinDatatype(IRIXSDNamespace + "integer"); !ok || dt.Kind != KindInteger {
		t.Fatalf("integer 应命中且 Kind=Integer：%+v %v", dt, ok)
	}
	if dt, ok := IsBuiltinDatatype(IRIXSDNamespace + "unsignedByte"); !ok || dt.Family != FamilyNumericDecimal {
		t.Fatalf("05 期新增 unsignedByte 应命中且族=NUMERIC_DECIMAL：%+v %v", dt, ok)
	}
	if _, ok := IsBuiltinDatatype(IRIOWLTopDataProperty); ok {
		t.Fatal("非 datatype IRI 不应命中")
	}
	// 刻意剔除项记档（05 号文档 §2.4）：rdfs:Literal / rdf:langString 不进值域白名单
	for _, iri := range []string{
		"http://www.w3.org/2000/01/rdf-schema#Literal",
		"http://www.w3.org/1999/02/22-rdf-syntax-ns#langString",
		"http://www.w3.org/2002/07/owl#real",
	} {
		if _, ok := IsBuiltinDatatype(iri); ok {
			t.Fatalf("%s 应被刻意剔除", iri)
		}
	}
}
