package service

import (
	"strings"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	identityV1 "go-wind-admin/api/gen/go/identity/service/v1"
)

// maxOrgSubtypeDepth 细分类型路径最大深度（顶层 type 之外的部分）。
// 词表可扩展，但深度设上限，避免把组织树或正交属性塞进类型链。
const maxOrgSubtypeDepth = 3

type orgSubtypeNode struct {
	code     string
	children []orgSubtypeNode
}

// orgSubtypeVocab 是顶层 type 之下的受控词表。
// 顶层 type 本身不在此表；新增细分类型时在这里加节点，并同步前端词表与本体叶子类。
var orgSubtypeVocab = map[identityV1.OrgUnit_Type][]orgSubtypeNode{
	identityV1.OrgUnit_STORE: {
		{code: "DEPARTMENT_STORE"},
		{code: "SHOPPING_MALL"},
		{code: "COMMUNITY_COMMERCIAL"},
		{code: "OUTLET_STORE"},
		{code: "STREET_COMMERCIAL"},
		{code: "MIXED_USE_COMPLEX"},
		{code: "OTHER"},
	},
	identityV1.OrgUnit_DEPARTMENT: {
		{code: "LEASING"},
		{code: "OPERATIONS"},
		{code: "MARKETING"},
		{code: "CUSTOMER_SERVICE"},
		{code: "FINANCE"},
		{code: "HR_ADMIN"},
		{code: "ENGINEERING_PROPERTY"},
		{code: "SECURITY"},
		{code: "IT"},
		{code: "GM_OFFICE"},
		{code: "INVESTMENT"},
		{code: "ASSET_MANAGEMENT"},
		{code: "LEGAL_COMPLIANCE"},
		{code: "AUDIT"},
		{code: "OTHER"},
	},
	identityV1.OrgUnit_PROJECT: {
		{code: "PRE_OPENING"},
		{code: "RENOVATION"},
		{code: "ASSET_LIGHT"},
		{code: "SPECIAL"},
		{code: "OTHER"},
	},
	identityV1.OrgUnit_DIVISION: {
		{code: "DEPARTMENT_STORE"},
		{code: "SHOPPING_MALL"},
		{code: "OUTLET"},
		{code: "ASSET_LIGHT"},
		{code: "OTHER"},
	},
	identityV1.OrgUnit_COMMITTEE: {
		{code: "LEASING_DECISION"},
		{code: "BRAND_REVIEW"},
		{code: "SAFETY"},
		{code: "COMPENSATION"},
		{code: "INVESTMENT"},
		{code: "OTHER"},
	},
	identityV1.OrgUnit_TEAM: {
		{code: "LEASING"},
		{code: "OPERATIONS"},
		{code: "ENGINEERING"},
		{code: "OTHER"},
	},
}

// requiredOrgSubtypeTypes 必须选择细分类型的顶层类型。
var requiredOrgSubtypeTypes = map[identityV1.OrgUnit_Type]bool{
	identityV1.OrgUnit_STORE:      true,
	identityV1.OrgUnit_DEPARTMENT: true,
	identityV1.OrgUnit_PROJECT:    true,
}

// normalizeOrgSubtypePath 校验并规范化细分类型路径。
// 返回 nil 表示该类型没有细分类型（调用方应清空列）。
// 路径每一段必须是上一级词表节点的子节点，深度不超过 maxOrgSubtypeDepth。
func normalizeOrgSubtypePath(orgType identityV1.OrgUnit_Type, path []string) ([]string, error) {
	trimmed := make([]string, 0, len(path))
	for _, code := range path {
		code = strings.ToUpper(strings.TrimSpace(code))
		if code != "" {
			trimmed = append(trimmed, code)
		}
	}

	if len(trimmed) == 0 {
		if requiredOrgSubtypeTypes[orgType] {
			return nil, identityV1.ErrorBadRequest("该组织类型必须选择细分类型")
		}
		return nil, nil
	}

	vocab, ok := orgSubtypeVocab[orgType]
	if !ok {
		return nil, identityV1.ErrorBadRequest("该组织类型不支持细分类型")
	}
	if len(trimmed) > maxOrgSubtypeDepth {
		return nil, identityV1.ErrorBadRequest("细分类型层级过深")
	}

	nodes := vocab
	for _, code := range trimmed {
		var next *orgSubtypeNode
		for i := range nodes {
			if nodes[i].code == code {
				next = &nodes[i]
				break
			}
		}
		if next == nil {
			return nil, identityV1.ErrorBadRequest("细分类型不在受控词表内: %s", code)
		}
		nodes = next.children
	}

	return trimmed, nil
}

// updateMaskIncludes 判断字段是否在更新掩码内；掩码为 nil 表示全量更新。
// 兼容 snake_case 与 camelCase 两种路径拼写。
func updateMaskIncludes(mask *fieldmaskpb.FieldMask, field string) bool {
	if mask == nil {
		return true
	}
	target := strings.ToLower(strings.ReplaceAll(field, "_", ""))
	for _, path := range mask.Paths {
		if strings.ToLower(strings.ReplaceAll(path, "_", "")) == target {
			return true
		}
	}
	return false
}
