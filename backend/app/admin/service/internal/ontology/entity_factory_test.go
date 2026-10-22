package ontology

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// stubEntityReader EntityReader 桩
type stubEntityReader struct {
	iris     map[string]bool        // ontologyID:iri → exists
	entities map[uint32]*ent.OntEntity
}

func newStubEntityReader() *stubEntityReader {
	return &stubEntityReader{
		iris:     map[string]bool{},
		entities: map[uint32]*ent.OntEntity{},
	}
}

func (s *stubEntityReader) IRIExists(_ context.Context, ontologyID uint32, iri string) (bool, error) {
	return s.iris[fmt.Sprintf("%d:%s", ontologyID, iri)], nil
}

func (s *stubEntityReader) GetEntitiesByIDs(_ context.Context, ontologyID uint32, ids []uint32) ([]*ent.OntEntity, error) {
	out := make([]*ent.OntEntity, 0, len(ids))
	for _, id := range ids {
		if e, ok := s.entities[id]; ok && e.OntologyID != nil && *e.OntologyID == ontologyID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *stubEntityReader) addEntity(id, ontologyID, entityType uint32, localName string) *ent.OntEntity {
	e := &ent.OntEntity{
		ID:         id,
		OntologyID: &ontologyID,
		EntityType: &entityType,
		LocalName:  &localName,
	}
	s.entities[id] = e
	return e
}

func newTestFactory() (*EntityFactory, *stubEntityReader) {
	reader := newStubEntityReader()
	return NewEntityFactory(reader), reader
}

func TestValidateLocalName(t *testing.T) {
	f, _ := newTestFactory()

	valid := []string{"设备", "ColdChainUnit", "cold_chain_unit", "冷链_机组-01x", "_private", "a"}
	for _, name := range valid {
		if err := f.ValidateLocalName(name); err != nil {
			t.Errorf("局部名 %q 应合法，却报 %v", name, err)
		}
	}

	invalid := []string{
		"1abc",   // 首字符数字
		"-lead",  // 首字符连字符
		"含 空格",   // 保留字符（空格）
		"a/b",    // 保留字符（斜杠）
		"a#b",    // 保留字符（#）
		"",       // 空串
	}
	for _, name := range invalid {
		if err := f.ValidateLocalName(name); err == nil {
			t.Errorf("局部名 %q 应不合法", name)
		} else if de, ok := err.(*DomainError); !ok || de.Reason != ReasonLocalNameInvalid {
			t.Errorf("局部名 %q 的错误 reason 应为 LOCAL_NAME_INVALID，实得 %v", name, err)
		}
	}
}

func TestBuildIRI(t *testing.T) {
	f, _ := newTestFactory()
	got := f.BuildIRI("http://example.com/ont#", "设备")
	if got != "http://example.com/ont#设备" {
		t.Fatalf("IRI 拼接错误：%s", got)
	}
}

func TestCreateEntityDraft_DuplicateIRI(t *testing.T) {
	f, reader := newTestFactory()
	reader.iris["7:http://example.com/#设备"] = true

	_, _, err := f.CreateEntityDraft(context.Background(), 7, EntityTypeClass, "http://example.com/#", "设备")
	if err == nil {
		t.Fatal("重复 IRI 应报错")
	}
	de, ok := err.(*DomainError)
	if !ok || de.Reason != ReasonEntityIRIDuplicate {
		t.Fatalf("reason 应为 ENTITY_IRI_DUPLICATE，实得 %v", err)
	}
	if !strings.Contains(de.Message, "http://example.com/#设备") {
		t.Fatalf("报错应带 IRI 明文：%s", de.Message)
	}
}

func TestCreateEntityDraft_OK(t *testing.T) {
	f, _ := newTestFactory()
	draft, decl, err := f.CreateEntityDraft(context.Background(), 7, EntityTypeClass, "http://example.com/#", "设备")
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	if draft.IRI != "http://example.com/#设备" || draft.EntityType != EntityTypeClass {
		t.Fatalf("草稿字段错误：%+v", draft)
	}
	if decl.Op != AxiomOpAdd || decl.AxiomType != AxiomDeclaration || decl.SubjectNewEntityRef != 1 {
		t.Fatalf("声明公理变更错误：%+v", decl)
	}
}

func TestValidateRefEntities(t *testing.T) {
	f, reader := newTestFactory()
	// 实体 1/2 属本体 7，为类；实体 3 属本体 8；实体 4 是对象属性
	reader.addEntity(1, 7, EntityTypeClass, "A")
	reader.addEntity(2, 7, EntityTypeClass, "B")
	reader.addEntity(3, 8, EntityTypeClass, "Other")
	reader.addEntity(4, 7, EntityTypeObjectProperty, "prop")

	// 通过：类型相符
	if err := f.ValidateRefEntities(context.Background(), 7, []uint32{1, 2}, EntityTypeClass); err != nil {
		t.Fatalf("合法引用不应报错：%v", err)
	}
	// 通过：空列表
	if err := f.ValidateRefEntities(context.Background(), 7, nil, EntityTypeClass); err != nil {
		t.Fatalf("空引用不应报错：%v", err)
	}
	// 拒绝：跨本体（3 不在本体 7）
	if err := f.ValidateRefEntities(context.Background(), 7, []uint32{1, 3}, EntityTypeClass); err == nil {
		t.Fatal("跨本体引用应报错")
	} else if de := err.(*DomainError); de.Reason != ReasonEntityTypeMismatch {
		t.Fatalf("reason 应为 ENTITY_TYPE_MISMATCH：%v", err)
	}
	// 拒绝：类型不符
	if err := f.ValidateRefEntities(context.Background(), 7, []uint32{4}, EntityTypeClass); err == nil {
		t.Fatal("类型不符引用应报错")
	}
	// 拒绝：不存在
	if err := f.ValidateRefEntities(context.Background(), 7, []uint32{99}, EntityTypeClass); err == nil {
		t.Fatal("不存在的引用应报错")
	}
	// 自身跳过：调用方约定（父类自引用由 AssertNoCycle 捕获），本方法不做自引用拦截
	if err := f.ValidateRefEntities(context.Background(), 7, []uint32{1}, EntityTypeClass); err != nil {
		t.Fatalf("单引用不应报错：%v", err)
	}
}
