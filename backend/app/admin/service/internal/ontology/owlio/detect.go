package owlio

import (
	"strings"
)

// 导入上限（07 号文档 §2.1 性能边界；超限拒绝并提示拆分）。
const (
	MaxImportBytes   = 5 << 20 // 5 MB
	MaxImportEntites = 10000
	MaxImportAxioms  = 20000
)

// Format 序列化格式枚举（服务层 format 参数与探测结果共用）。
type Format string

const (
	FormatRDFXML Format = "rdfxml"
	FormatTurtle Format = "turtle"
)

// Label 供报告展示的格式名。
func (f Format) Label() string {
	switch f {
	case FormatTurtle:
		return "Turtle"
	default:
		return "RDF/XML"
	}
}

// DetectFormat 按内容探测格式（07 号文档 §2.1：剥 BOM 与前导空白后，首字符 '<'
// 或 '<?xml' 开头 → RDF/XML，否则按 Turtle 解析）。
func DetectFormat(content []byte) Format {
	data := strings.TrimLeft(strings.TrimPrefix(string(content), "\uFEFF"), " \t\r\n")
	if strings.HasPrefix(data, "<") {
		return FormatRDFXML
	}
	return FormatTurtle
}

// Parse 按探测结果分流解析（07 号文档 §2.1 四读写器的统一入口）。
// 返回三元组（文件出现序）。任何解析错误返回 error，由服务层包成
// OWL_IMPORT_FILE_INVALID（带原始消息）。
func Parse(content []byte) ([]Triple, error) {
	switch DetectFormat(content) {
	case FormatRDFXML:
		return ParseRDFXML(content)
	default:
		return ParseTurtle(content)
	}
}

// Write 按格式序列化。
func Write(triples []Triple, format Format, ontologyIRI string) ([]byte, error) {
	switch format {
	case FormatTurtle:
		return WriteTurtle(triples, ontologyIRI)
	default:
		return WriteRDFXML(triples, ontologyIRI)
	}
}
