package ontology

import (
	"context"
	"regexp"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// EntityReader 实体只读需求的最小接口（data 层实现，单测桩替换）
type EntityReader interface {
	// IRIExists 本体下该 IRI 是否已有未删实体（服务层前置查重；DB 部分唯一索引兜底并发双写）
	IRIExists(ctx context.Context, ontologyID uint32, iri string) (bool, error)
	// GetEntitiesByIDs 批量取未删实体（同本体过滤在实现内完成，跨本体 id 视为不存在）
	GetEntitiesByIDs(ctx context.Context, ontologyID uint32, ids []uint32) ([]*ent.OntEntity, error)
}

// localNamePattern NCName 宽松版：允许中文/字母/下划线/连字符，首字符非数字。
// Go regexp 无 Unicode \w，用等价的 Unicode 类别写法；与源前端同口径正则
// /^[\p{L}_][\p{L}\p{N}_-]*$/u 完全一致。
var localNamePattern = regexp.MustCompile(`^[\p{L}_][\p{L}\p{N}_-]*$`)

// EntityFactory 实体注册与命名工厂（对应 Protégé OWLEntityFactory / 源 OntEntityFactory）
type EntityFactory struct {
	reader EntityReader
}

func NewEntityFactory(reader EntityReader) *EntityFactory {
	return &EntityFactory{reader: reader}
}

// BuildIRI 拼接完整 IRI：baseIRI 已规范化为以 # 结尾（本体创建时统一补齐）。
func (f *EntityFactory) BuildIRI(baseIRI, localName string) string {
	return baseIRI + localName
}

// ValidateLocalName 局部名合法性（不符 → LOCAL_NAME_INVALID）
func (f *EntityFactory) ValidateLocalName(localName string) error {
	if !localNamePattern.MatchString(localName) {
		return NewError(ReasonLocalNameInvalid, "局部名不合法：%s", localName)
	}
	return nil
}

// EntityDraft 待创建实体草稿：由 ChangeApplier 在事务内落库后取得自增 id。
type EntityDraft struct {
	EntityType uint32
	IRI        string
	LocalName  string
}

// CreateEntityDraft 创建实体草稿：局部名校验 + IRI 查重（ontology_id + iri + 未删）→
// ENTITY_IRI_DUPLICATE（message 带 IRI 明文）；返回草稿与配套的 DECLARATION(10) 公理变更
// （SubjectNewEntityRef=1 即引用 NewEntities 首个草稿，调用方把草稿放进
// EntityChangeSet.NewEntities 的下标 0 处，由 ChangeApplier 同事务落库）。
func (f *EntityFactory) CreateEntityDraft(ctx context.Context, ontologyID uint32, entityType uint32, baseIRI, localName string) (*EntityDraft, AxiomChange, error) {
	if err := f.ValidateLocalName(localName); err != nil {
		return nil, AxiomChange{}, err
	}

	iri := f.BuildIRI(baseIRI, localName)
	exists, err := f.reader.IRIExists(ctx, ontologyID, iri)
	if err != nil {
		return nil, AxiomChange{}, err
	}
	if exists {
		return nil, AxiomChange{}, NewError(ReasonEntityIRIDuplicate, "已存在相同 IRI 的实体：%s", iri)
	}

	draft := &EntityDraft{EntityType: entityType, IRI: iri, LocalName: localName}
	declaration := AxiomChange{
		Op:                  AxiomOpAdd,
		AxiomType:           AxiomDeclaration,
		SubjectNewEntityRef: 1, // 引用 NewEntities[0]（零值安全约定：0=无引用）
	}
	return draft, declaration, nil
}

// ValidateRefEntities 引用实体校验：目标须存在、同本体、指定类型；不符 → ENTITY_TYPE_MISMATCH
// （源修订 #13 通用化；message 带具体 id）。空列表直接通过。
// 自引用校验不在本方法（create 时新实体无 id；update 时父类自引用走 AssertNoCycle 捕获为环，
// 不相交/等价自引用由服务层前置拒绝）。
func (f *EntityFactory) ValidateRefEntities(ctx context.Context, ontologyID uint32, refIDs []uint32, expected uint32) error {
	if len(refIDs) == 0 {
		return nil
	}

	entities, err := f.reader.GetEntitiesByIDs(ctx, ontologyID, refIDs)
	if err != nil {
		return err
	}

	found := make(map[uint32]*ent.OntEntity, len(entities))
	for _, e := range entities {
		found[e.ID] = e
	}

	for _, id := range refIDs {
		e, ok := found[id]
		if !ok {
			return NewError(ReasonEntityTypeMismatch, "引用实体不存在或不在本本体：%d", id)
		}
		if e.EntityType == nil || *e.EntityType != expected {
			return NewError(ReasonEntityTypeMismatch, "引用实体类型不符：%d（期望类型 %d）", id, expected)
		}
	}
	return nil
}
