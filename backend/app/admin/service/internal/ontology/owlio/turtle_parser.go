package owlio

import (
	"fmt"
	"net/url"
	"strings"
)

// Turtle 解析器（07 号文档 §2.1 覆盖子集）：@prefix/@base 大小写两式、`a`、`;`/`,` 缩写、
// IRI（<> 与前缀名、相对 IRI 按 @base 解析）、字面量四形态（单/三引号、@lang、^^type、
// 数字/布尔缩写）、空白节点（[]、_:b）、集合 (...)、注释 #。
// 未覆盖形态解析失败上抛错误（由服务层包 OWL_IMPORT_FILE_INVALID），不静默丢行。

type turtleParser struct {
	data     string
	pos      int
	line     int
	base     string
	prefixes map[string]string
	blankSeq int
	triples  []Triple
}

// ParseTurtle 解析 Turtle 文本为三元组（保持文件出现序）。
func ParseTurtle(content []byte) ([]Triple, error) {
	p := &turtleParser{data: string(content), line: 1, prefixes: map[string]string{}}
	if err := p.parseDoc(); err != nil {
		return nil, err
	}
	return p.triples, nil
}

// ---- 基础扫描 ----

func (p *turtleParser) errf(format string, args ...interface{}) error {
	return fmt.Errorf("Turtle 解析失败（第 %d 行）：%s", p.line, sprintf(format, args...))
}

func (p *turtleParser) eof() bool { return p.pos >= len(p.data) }

func (p *turtleParser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.data[p.pos]
}

// skipWS 跳过空白与 # 注释。
func (p *turtleParser) skipWS() {
	for !p.eof() {
		c := p.data[p.pos]
		switch {
		case c == '\n':
			p.line++
			p.pos++
		case c == ' ' || c == '\t' || c == '\r':
			p.pos++
		case c == '#':
			for !p.eof() && p.data[p.pos] != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

// tryWord 尝试匹配字面词（大小写敏感），命中则推进。
func (p *turtleParser) tryWord(w string) bool {
	if strings.HasPrefix(p.data[p.pos:], w) {
		p.pos += len(w)
		return true
	}
	return false
}

// ---- 文档结构 ----

func (p *turtleParser) parseDoc() error {
	for {
		p.skipWS()
		if p.eof() {
			return nil
		}
		rest := p.data[p.pos:]
		switch {
		case strings.HasPrefix(rest, "@prefix"):
			p.pos += len("@prefix")
			if err := p.parseDirective(true); err != nil {
				return err
			}
			p.skipWS()
			if !p.tryWord(".") {
				return p.errf("@prefix 指令缺少结尾 '.'")
			}
		case strings.HasPrefix(rest, "@base"):
			p.pos += len("@base")
			if err := p.parseDirective(false); err != nil {
				return err
			}
			p.skipWS()
			if !p.tryWord(".") {
				return p.errf("@base 指令缺少结尾 '.'")
			}
		default:
			// SPARQL 式 PREFIX/BASE（大小写不敏感、结尾无点）或普通三元组语句
			if word := p.matchKeyword("prefix"); word {
				if err := p.parseDirective(true); err != nil {
					return err
				}
				continue
			}
			if word := p.matchKeyword("base"); word {
				if err := p.parseDirective(false); err != nil {
					return err
				}
				continue
			}
			if err := p.parseTriplesStatement(); err != nil {
				return err
			}
			p.skipWS()
			if !p.tryWord(".") {
				return p.errf("三元组语句缺少结尾 '.'")
			}
		}
	}
}

// matchKeyword 大小写不敏感匹配整词（后接空白/换行），命中推进并返回 true。
func (p *turtleParser) matchKeyword(w string) bool {
	end := p.pos + len(w)
	if end > len(p.data) {
		return false
	}
	if !strings.EqualFold(p.data[p.pos:end], w) {
		return false
	}
	if end < len(p.data) {
		c := p.data[end]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
	}
	p.pos = end
	return true
}

func (p *turtleParser) parseDirective(isPrefix bool) error {
	p.skipWS()
	if isPrefix {
		// PNAME_NS：pfx: 或 :
		name, err := p.scanPrefixName()
		if err != nil {
			return err
		}
		p.skipWS()
		iri, err := p.scanIRIRef()
		if err != nil {
			return err
		}
		p.prefixes[strings.TrimSuffix(name, ":")] = p.resolveIRI(iri)
		return nil
	}
	iri, err := p.scanIRIRef()
	if err != nil {
		return err
	}
	p.base = p.resolveIRI(iri)
	return nil
}

func (p *turtleParser) parseTriplesStatement() error {
	subject, hasSubject, err := p.parseSubject()
	if err != nil {
		return err
	}
	if !hasSubject {
		return p.errf("缺少主语")
	}
	// `[...]` 形态主语允许无谓语部分（纯节点声明）
	p.skipWS()
	if subject.Kind == TermBlank && (p.peek() == '.' || p.eof()) {
		return nil
	}
	return p.parsePredicateObjectList(subject)
}

// parseSubject 主语：IRI / 前缀名 / _:label / [ ... ] / ( ... )
func (p *turtleParser) parseSubject() (Term, bool, error) {
	p.skipWS()
	if p.eof() {
		return Term{}, false, nil
	}
	switch p.peek() {
	case '<':
		iri, err := p.scanIRIRef()
		if err != nil {
			return Term{}, false, err
		}
		return Iri(p.resolveIRI(iri)), true, nil
	case '[':
		term, err := p.parseBlankNodePropertyList()
		return term, err == nil, err
	case '(':
		term, err := p.parseCollection()
		return term, err == nil, err
	}
	if strings.HasPrefix(p.data[p.pos:], "_:") {
		label := p.scanBlankLabel()
		return Blank(label), true, nil
	}
	iri, ok := p.scanPrefixedName()
	if !ok {
		return Term{}, false, nil
	}
	resolved, err := p.expandPrefixed(iri)
	if err != nil {
		return Term{}, false, p.errf("%v", err)
	}
	return Iri(resolved), true, nil
}

// parsePredicateObjectList 谓语宾语列表（; 分隔，; 后允许为空尾）
func (p *turtleParser) parsePredicateObjectList(subject Term) error {
	for {
		p.skipWS()
		predicate, err := p.parseVerb()
		if err != nil {
			return err
		}
		for {
			object, err := p.parseObject()
			if err != nil {
				return err
			}
			p.emit(Triple{Subject: subject, Predicate: predicate, Object: object})
			p.skipWS()
			if p.tryWord(",") {
				continue
			}
			break
		}
		p.skipWS()
		if p.tryWord(";") {
			p.skipWS()
			// ';' 后直接 '.' 是合法空尾
			if p.peek() == '.' || p.eof() {
				return nil
			}
			continue
		}
		return nil
	}
}

// parseVerb 谓语：`a` 或 IRI/前缀名。
func (p *turtleParser) parseVerb() (Term, error) {
	p.skipWS()
	if p.eof() {
		return Term{}, p.errf("缺少谓语")
	}
	if p.peek() == 'a' {
		after := byte(' ')
		if p.pos+1 < len(p.data) {
			after = p.data[p.pos+1]
		}
		if after == ' ' || after == '\t' || after == '\n' || after == '\r' {
			p.pos++
			return Iri(IRIRDFType), nil
		}
	}
	if strings.HasPrefix(p.data[p.pos:], "_:") {
		return Term{}, p.errf("谓语不能是空白节点")
	}
	name, ok := p.scanPrefixedName()
	if !ok {
		iri, err := p.scanIRIRef()
		if err != nil {
			return Term{}, err
		}
		return Iri(p.resolveIRI(iri)), nil
	}
	resolved, err := p.expandPrefixed(name)
	if err != nil {
		return Term{}, p.errf("%v", err)
	}
	return Iri(resolved), nil
}

// parseObject 宾语：IRI / blank / 集合 / [] / 字面量。
func (p *turtleParser) parseObject() (Term, error) {
	p.skipWS()
	if p.eof() {
		return Term{}, p.errf("缺少宾语")
	}
	switch c := p.peek(); {
	case c == '<':
		iri, err := p.scanIRIRef()
		if err != nil {
			return Term{}, err
		}
		return Iri(p.resolveIRI(iri)), nil
	case c == '[':
		return p.parseBlankNodePropertyList()
	case c == '(':
		return p.parseCollection()
	case c == '"' || c == '\'':
		return p.parseLiteral()
	case c == '_' && strings.HasPrefix(p.data[p.pos:], "_:"):
		return Blank(p.scanBlankLabel()), nil
	case c == '+' || c == '-' || (c >= '0' && c <= '9') || c == '.':
		if t, ok, err := p.tryNumberOrDot(c); err != nil {
			return Term{}, err
		} else if ok {
			return t, nil
		}
	case c == 't' || c == 'f':
		if b, ok := p.tryBoolean(); ok {
			return b, nil
		}
	}
	name, ok := p.scanPrefixedName()
	if !ok {
		return Term{}, p.errf("无法识别的宾语（位置 %d）", p.pos)
	}
	resolved, err := p.expandPrefixed(name)
	if err != nil {
		return Term{}, p.errf("%v", err)
	}
	return Iri(resolved), nil
}

// parseBlankNodePropertyList `[ p o; p2 o2 ]` → fresh blank + 内嵌三元组。
func (p *turtleParser) parseBlankNodePropertyList() (Term, error) {
	p.pos++ // '['
	p.skipWS()
	node := p.freshBlank()
	for {
		p.skipWS()
		if p.eof() {
			return Term{}, p.errf("[ 未闭合")
		}
		if p.peek() == ']' {
			p.pos++
			return node, nil
		}
		if err := p.parsePredicateObjectList(node); err != nil {
			return Term{}, err
		}
		p.skipWS()
		if p.tryWord(";") {
			continue
		}
		if p.peek() == ']' {
			p.pos++
			return node, nil
		}
		return Term{}, p.errf("[ 内缺少 ';' 或 ']'")
	}
}

// parseCollection `( item1 item2 ... )` → rdf:first/rdf:rest 链；空集 = rdf:nil。
func (p *turtleParser) parseCollection() (Term, error) {
	p.pos++ // '('
	p.skipWS()
	if !p.eof() && p.peek() == ')' {
		p.pos++
		return Iri(IRIRDFNil), nil
	}
	head := p.freshBlank()
	cur := head
	for {
		p.skipWS()
		if p.eof() {
			return Term{}, p.errf("( 未闭合")
		}
		if p.peek() == ')' {
			p.pos++
			p.emit(Triple{Subject: cur, Predicate: Iri(IRIRDFRest), Object: Iri(IRIRDFNil)})
			return head, nil
		}
		item, err := p.parseObject()
		if err != nil {
			return Term{}, err
		}
		p.emit(Triple{Subject: cur, Predicate: Iri(IRIRDFFirst), Object: item})
		p.skipWS()
		if !p.eof() && p.peek() == ')' {
			p.pos++
			p.emit(Triple{Subject: cur, Predicate: Iri(IRIRDFRest), Object: Iri(IRIRDFNil)})
			return head, nil
		}
		next := p.freshBlank()
		p.emit(Triple{Subject: cur, Predicate: Iri(IRIRDFRest), Object: next})
		cur = next
	}
}

func (p *turtleParser) emit(t Triple) { p.triples = append(p.triples, t) }

func (p *turtleParser) freshBlank() Term {
	p.blankSeq++
	return Blank(fmt.Sprintf("genid%d", p.blankSeq))
}

// ---- 词法：IRI / 前缀名 / 字面量 / 数字 / 布尔 ----

// scanIRIRef `<...>` 内文（含 \u 转义）。
func (p *turtleParser) scanIRIRef() (string, error) {
	if p.peek() != '<' {
		return "", p.errf("期望 IRI（'<'）")
	}
	end := strings.IndexByte(p.data[p.pos+1:], '>')
	if end < 0 {
		return "", p.errf("IRI 未闭合")
	}
	raw := p.data[p.pos+1 : p.pos+1+end]
	p.pos += end + 2
	raw = strings.ReplaceAll(raw, "\\\n", "")
	return unescapeTurtleString(raw, '"'), nil
}

// scanBlankLabel `_:label` 的 label（PN_LOCAL 子集：字母/数字/_/-/./%，不以 '.' 结尾）。
func (p *turtleParser) scanBlankLabel() string {
	p.pos += 2
	return p.scanNameTail(true)
}

// scanPrefixName 指令侧 PNAME_NS（仅取前缀段含冒号）。
func (p *turtleParser) scanPrefixName() (string, error) {
	start := p.pos
	for !p.eof() && p.data[p.pos] != ':' && !isTurtleWS(p.data[p.pos]) {
		p.pos++
	}
	if p.eof() || p.data[p.pos] != ':' {
		return "", p.errf("@prefix 缺少前缀名")
	}
	p.pos++ // ':'
	return p.data[start:p.pos], nil
}

// scanPrefixedName 前缀名（pfx:local / :local / pfx:）；`true`/`false` 无冒号天然不命中。
func (p *turtleParser) scanPrefixedName() (string, bool) {
	start := p.pos
	i := p.pos
	// 前缀段（可空，PN_CHARS_BASE 不含 ':'；'@' '^' 等终结符隔离）
	for i < len(p.data) && p.data[i] != ':' && !isTurtleWS(p.data[i]) && !strings.ContainsRune("<>(\"'{}[],;^@=", rune(p.data[i])) {
		i++
	}
	if i >= len(p.data) || p.data[i] != ':' {
		return "", false
	}
	p.pos = i + 1 // 吃 ':'
	local := p.scanNameTail(false)
	return p.data[start:i] + ":" + local, true
}

// scanNameTail PN_LOCAL 词法（字母/数字/_/-/./%XX/PN_LOCAL_ESC，不允许 '.' 结尾）。
func (p *turtleParser) scanNameTail(allowEmpty bool) string {
	buf := make([]byte, 0, 16)
	for !p.eof() {
		c := p.data[p.pos]
		if isPNLocalChar(c) {
			buf = append(buf, c)
			p.pos++
			continue
		}
		if c == '%' && p.pos+2 < len(p.data) && isHex(p.data[p.pos+1]) && isHex(p.data[p.pos+2]) {
			buf = append(buf, p.data[p.pos:p.pos+3]...)
			p.pos += 3
			continue
		}
		if c == '\\' && p.pos+1 < len(p.data) { // PN_LOCAL_ESC：\_ \- \. 等 → 保留转义原字符
			next := p.data[p.pos+1]
			if strings.IndexByte("_~.-!$&'()*+,;=/?#@%", next) >= 0 {
				buf = append(buf, next)
				p.pos += 2
				continue
			}
		}
		break
	}
	// PN_LOCAL 不能以 '.' 结尾：回退句点（连带回退 pos）
	for len(buf) > 0 && buf[len(buf)-1] == '.' {
		buf = buf[:len(buf)-1]
		p.pos--
	}
	if len(buf) == 0 && !allowEmpty {
		return ""
	}
	return string(buf)
}

// parseLiteral 字符串字面量（四形态）+ 可选 @lang / ^^datatype。
func (p *turtleParser) parseLiteral() (Term, error) {
	value, err := p.scanString()
	if err != nil {
		return Term{}, err
	}
	p.skipWSInline()
	switch {
	case p.tryWord("^^"):
		p.skipWSInline()
		if p.peek() == '<' {
			iri, err := p.scanIRIRef()
			if err != nil {
				return Term{}, err
			}
			return TypedLit(value, p.resolveIRI(iri)), nil
		}
		name, ok := p.scanPrefixedName()
		if !ok {
			return Term{}, p.errf("^^ 后缺少 datatype IRI")
		}
		resolved, err := p.expandPrefixed(name)
		if err != nil {
			return Term{}, p.errf("%v", err)
		}
		return TypedLit(value, resolved), nil
	case p.peek() == '@':
		p.pos++
		lang := p.scanLangTag()
		if lang == "" {
			return Term{}, p.errf("@ 后缺少语言标签")
		}
		return LangLit(value, lang), nil
	}
	return Lit(value), nil
}

// scanString 四形态字符串（含长引号多行与全部转义）。
func (p *turtleParser) scanString() (string, error) {
	quote := p.peek()
	if p.tryWord(`"""`) || p.tryWord("'''") {
		triple := string([]byte{quote, quote, quote})
		start := p.pos
		idx := strings.Index(p.data[p.pos:], triple)
		if idx < 0 {
			return "", p.errf("长字符串未闭合")
		}
		p.pos += idx + 3
		return unescapeTurtleString(p.data[start:p.pos-3], 0), nil
	}
	p.pos++ // 单引号
	start := p.pos
	var out strings.Builder
	for {
		if p.eof() {
			return "", p.errf("字符串未闭合")
		}
		c := p.data[p.pos]
		if c == '\n' {
			return "", p.errf("字符串内不允许换行（长文本请用三引号）")
		}
		if c == quote {
			p.pos++
			_ = start
			return out.String(), nil
		}
		if c == '\\' {
			s, adv := p.scanEscape()
			if adv < 0 {
				return "", p.errf("非法转义序列")
			}
			out.WriteString(s)
			p.pos += adv
			continue
		}
		out.WriteByte(c)
		p.pos++
	}
}

// scanEscape 处理 '\' 起始的转义，返回 (展开文本, 消耗字节数)；-1 表示非法。
func (p *turtleParser) scanEscape() (string, int) {
	if p.pos+1 >= len(p.data) {
		return "", -1
	}
	c := p.data[p.pos+1]
	switch c {
	case 't':
		return "\t", 2
	case 'b':
		return "\b", 2
	case 'n':
		return "\n", 2
	case 'r':
		return "\r", 2
	case 'f':
		return "\f", 2
	case '"':
		return "\"", 2
	case '\'':
		return "'", 2
	case '\\':
		return "\\", 2
	case 'u':
		if p.pos+6 <= len(p.data) {
			if v, ok := parseHex4(p.data[p.pos+2 : p.pos+6]); ok {
				return string(rune(v)), 6
			}
		}
		return "", -1
	case 'U':
		if p.pos+10 <= len(p.data) {
			if v, ok := parseHex8(p.data[p.pos+2 : p.pos+10]); ok {
				return string(rune(v)), 10
			}
		}
		return "", -1
	}
	return "", -1
}

// tryNumberOrDot 数字缩写（integer/decimal/double）；'.' 开头且非数字形态返回 false 走句子点。
func (p *turtleParser) tryNumberOrDot(c byte) (Term, bool, error) {
	start := p.pos
	i := p.pos
	isDouble, isDecimal := false, false
	if c == '+' || c == '-' {
		i++
	}
	digits := 0
	for i < len(p.data) && p.data[i] >= '0' && p.data[i] <= '9' {
		i++
		digits++
	}
	if i < len(p.data) && p.data[i] == '.' {
		// '.' 后必须跟数字才是小数（否则该 '.' 是句子终止符，数字到此为止）
		if i+1 < len(p.data) && p.data[i+1] >= '0' && p.data[i+1] <= '9' {
			isDecimal = true
			i++
			for i < len(p.data) && p.data[i] >= '0' && p.data[i] <= '9' {
				i++
				digits++
			}
		}
	}
	if digits == 0 {
		return Term{}, false, nil // 如 ".":句子点，或 ".x" 非法
	}
	if i < len(p.data) && (p.data[i] == 'e' || p.data[i] == 'E') {
		j := i + 1
		if j < len(p.data) && (p.data[j] == '+' || p.data[j] == '-') {
			j++
		}
		expDigits := 0
		for j < len(p.data) && p.data[j] >= '0' && p.data[j] <= '9' {
			j++
			expDigits++
		}
		if expDigits > 0 {
			isDouble = true
			i = j
		}
	}
	text := p.data[start:i]
	p.pos = i
	datatype := NSXSD + "integer"
	if isDouble {
		datatype = NSXSD + "double"
	} else if isDecimal {
		datatype = NSXSD + "decimal"
	}
	return TypedLit(text, datatype), true, nil
}

// tryBoolean true/false 缩写（后接 PN_LOCAL 字符则不是布尔，交回前缀名词法）。
func (p *turtleParser) tryBoolean() (Term, bool) {
	for _, w := range []string{"true", "false"} {
		if !strings.HasPrefix(p.data[p.pos:], w) {
			continue
		}
		after := byte(0)
		if p.pos+len(w) < len(p.data) {
			after = p.data[p.pos+len(w)]
		}
		if isPNLocalChar(after) {
			return Term{}, false
		}
		p.pos += len(w)
		return TypedLit(w, NSXSD+"boolean"), true
	}
	return Term{}, false
}

func (p *turtleParser) skipWSInline() {
	for !p.eof() && (p.data[p.pos] == ' ' || p.data[p.pos] == '\t') {
		p.pos++
	}
}

// ---- IRI 解析与前缀展开 ----

// resolveIRI 绝对 IRI 原样返回；相对 IRI 按 @base（或文档位置）解析。
func (p *turtleParser) resolveIRI(iri string) string {
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

func (p *turtleParser) expandPrefixed(name string) (string, error) {
	idx := strings.IndexByte(name, ':')
	if idx < 0 {
		return "", fmt.Errorf("前缀名缺少冒号：%s", name)
	}
	prefix := name[:idx]
	ns, ok := p.prefixes[prefix]
	if !ok {
		return "", fmt.Errorf("未声明的前缀 %q（%s）", prefix, name)
	}
	return ns + name[idx+1:], nil
}

// ---- 词法辅助 ----

func isTurtleWS(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// isPNLocalChar PN_LOCAL 中间字符的宽松子集（字母/数字/_/-）。
func isPNLocalChar(c byte) bool {
	return c == '_' || c == '-' || (c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isPNName(s string) bool { return s == "true" || s == "false" }

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func parseHex4(s string) (int64, bool) {
	var v int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		var d int64
		switch {
		case c >= '0' && c <= '9':
			d = int64(c - '0')
		case c >= 'a' && c <= 'f':
			d = int64(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = int64(c-'A') + 10
		default:
			return 0, false
		}
		v = v<<4 | d
	}
	return v, true
}

func parseHex8(s string) (int64, bool) {
	v, ok := parseHex4(s[:4])
	if !ok {
		return 0, false
	}
	hi, ok2 := parseHex4(s[4:])
	if !ok2 {
		return 0, false
	}
	return v<<32 | hi, true
}

// unescapeTurtleString 展开 \u/\U 与常见转义（单行字符串内；长字符串保留换行原文）。
func unescapeTurtleString(s string, _ byte) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			out.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case 't':
			out.WriteByte('\t')
			i++
		case 'n':
			out.WriteByte('\n')
			i++
		case 'r':
			out.WriteByte('\r')
			i++
		case '"':
			out.WriteByte('"')
			i++
		case '\'':
			out.WriteByte('\'')
			i++
		case '\\':
			out.WriteByte('\\')
			i++
		case 'u':
			if i+6 <= len(s) {
				if v, ok := parseHex4(s[i+2 : i+6]); ok {
					out.WriteRune(rune(v))
					i += 5
					continue
				}
			}
			out.WriteByte(s[i])
		case 'U':
			if i+10 <= len(s) {
				if v, ok := parseHex8(s[i+2 : i+10]); ok {
					out.WriteRune(rune(v))
					i += 9
					continue
				}
			}
			out.WriteByte(s[i])
		default:
			out.WriteByte(s[i])
		}
	}
	return out.String()
}

func (p *turtleParser) scanLangTag() string {
	start := p.pos
	for !p.eof() {
		c := p.data[p.pos]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			p.pos++
			continue
		}
		break
	}
	return p.data[start:p.pos]
}
