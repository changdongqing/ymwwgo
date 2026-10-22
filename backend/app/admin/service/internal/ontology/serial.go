package ontology

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
)

// 取号器纯函数层（06 号文档 §4.1）：格式化与字符集校验。
// 号池 SQL（两段式取号、唯一索引兜底）在 data 层 OntSerialRepo；本文件只做可单测的纯函数。

// SerialTx 号池取号事务端（data 层 ontologyTx 实现）：取号只在 ApplyInTx 传入的事务上
// 执行、不自带事务（与公理写同事务，回滚不跳号）。回调内以接口断言取用：
// tx.(SerialTx).NextNo(...)。
type SerialTx interface {
	NextNo(ctx context.Context, ontologyID uint32, purpose, prefix string) (uint64, error)
}

// SerialPurpose 号池用途域（ont_serials.purpose）。
const (
	// SerialPurposeInd 个体 localName 自动生成的伪前缀号池（prefix 固定 SerialIndPrefix）。
	// "$" 非法于编码字符集与 localName，天然不与真实前缀相撞。
	SerialPurposeInd = "ind"
	// SerialPurposeCode 业务编码流水号池（prefix = 主类目 skos:notation）。
	SerialPurposeCode = "code"
)

// SerialIndPrefix 个体 localName 伪前缀（purpose=ind 时 ont_serials.prefix 固定值）。
const SerialIndPrefix = "$ind"

// codeCharsetPattern 编码字符集（06 号文档 §3.3 定案，前缀与流水拼接后整体校验）：
// 允许字母/数字/点/下划线/连字符，首字符为字母或数字——允许纯数字前缀（行政区划码），
// 编码不受 NCName 约束（路线二红利）。01 号文档 §3.6 规则 5 的 notation 字符集以本定案为准。
var codeCharsetPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateCodeCharset 编码字符集校验（不符 → INDIVIDUAL_CODE_PREFIX_INVALID）。
func ValidateCodeCharset(code string) error {
	if !codeCharsetPattern.MatchString(code) {
		return NewError(ReasonIndividualCodePrefixInvalid,
			"编码含非法字符：%s（允许字母/数字/点/下划线/连字符，首字符为字母或数字）", code)
	}
	return nil
}

// FormatCode 编码格式化：前缀 + "-" + LPAD(流水, 4, '0')；超 9999 自然扩位（LPAD 不截断）。
func FormatCode(prefix string, seq uint64) string {
	return prefix + "-" + padLeft(seq, 4)
}

// FormatLocalName 个体 localName 自动格式化："i" + LPAD(流水, 6, '0')；超 999999 自然扩位。
func FormatLocalName(seq uint64) string {
	return "i" + padLeft(seq, 6)
}

// padLeft 十进制左补零到 width 位（超宽自然扩位，不截断）。
func padLeft(seq uint64, width int) string {
	s := strconv.FormatUint(seq, 10)
	if len(s) >= width {
		return s
	}
	return fmt.Sprintf("%0*d", width, seq)
}
