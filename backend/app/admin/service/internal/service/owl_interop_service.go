package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tx7do/go-utils/trans"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"

	adminV1 "go-wind-admin/api/gen/go/admin/service/v1"
	ontologyV1 "go-wind-admin/api/gen/go/ontology/service/v1"

	"go-wind-admin/app/admin/service/internal/data"
	"go-wind-admin/app/admin/service/internal/data/ent"
	"go-wind-admin/app/admin/service/internal/ontology"
	"go-wind-admin/app/admin/service/internal/ontology/owlio"
)

// OntologyOwlIOService OWL 导入导出服务（07 期 BFF 编排；07 号文档 §4）。
//
// Export 只读（不 +version、不写变更日志）；ImportPreview 不落库；ImportExecute 是唯一写路径
// （新建本体 + 实体逐条 insert + 公理 CreateBulk + 渲染缓存同步 + 一次全量校验 + 号池回填 +
// 摘要日志，单个 ent 事务原子；§2.4）。不走 ChangeApplier.Apply（交互式单实体变更语义）。
type OntologyOwlIOService struct {
	adminV1.OntologyOwlIOServiceHTTPServer

	log          *bLogger.Helper
	ontologyRepo *data.OntologyRepo
	entityRepo   *data.OntEntityRepo
	axiomRepo    *data.OntAxiomRepo
	applier      *ontology.ChangeApplier
	planStore    owlio.ImportPlanStore
}

func NewOntologyOwlIOService(
	ctx *bootstrap.Context,
	ontologyRepo *data.OntologyRepo,
	entityRepo *data.OntEntityRepo,
	axiomRepo *data.OntAxiomRepo,
) *OntologyOwlIOService {
	log := ctx.NewLoggerHelper("ontology-owl-io/service/admin-service")
	return &OntologyOwlIOService{
		log:          log,
		ontologyRepo: ontologyRepo,
		entityRepo:   entityRepo,
		axiomRepo:    axiomRepo,
		applier:      ontology.NewChangeApplier(axiomRepo, log),
		planStore:    owlio.NewMemoryPlanStore(0),
	}
}

// 导入限制与常量（07 号文档 §2.1/§4.4）。
const (
	owlMaxDescriptionLen = 255
	owlMaxNameLen        = 64

	owlAxiomBulkSize = 500 // 公理 CreateBulk 分批
)

// indLocalNamePattern 个体 localName 流水池匹配（06 期 FormatLocalName 形态：i + ≥6 位数字）。
var indLocalNamePattern = regexp.MustCompile(`^i(\d{6,})$`)

// errInvalid 把任意 error 包成 OWL_IMPORT_FILE_INVALID（解析器原始消息截断 500 字符）。
func errInvalid(err error) error {
	msg := err.Error()
	if utf8.RuneCountInString(msg) > 500 {
		msg = string([]rune(msg)[:500])
	}
	return ontologyV1.ErrorOwlImportFileInvalid("OWL 文件解析失败：%s", msg)
}

// ---- Export（§4.2） ----

// Export 本体 → OWL 文件（bytes 响应；文件名随响应体下发——Kratos v2 的
// DefaultResponseEncoder 对 google.api.HttpBody 无二进制特判、Content-Disposition 头在
// proto 管线内不可用，07 号文档 §7 #19 预案回退 bytes 响应，protojson 下 content 即 base64）。
func (s *OntologyOwlIOService) Export(ctx context.Context, req *ontologyV1.OwlExportRequest) (*ontologyV1.OwlExportResponse, error) {
	if req == nil || req.GetOntologyId() == 0 {
		return nil, adminV1.ErrorBadRequest("ontology id is required")
	}
	format := strings.ToLower(strings.TrimSpace(req.GetFormat()))
	if format == "" {
		format = string(owlio.FormatRDFXML)
	}
	if format != string(owlio.FormatRDFXML) && format != string(owlio.FormatTurtle) {
		return nil, ontologyV1.ErrorOwlExportFormatInvalid("不支持的导出格式：%s", req.GetFormat())
	}

	ont, err := s.ontologyRepo.GetEntityByID(ctx, req.GetOntologyId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ontologyV1.ErrorOntologyNotFound("本体不存在")
		}
		return nil, adminV1.ErrorInternalServerError("query ontology failed")
	}
	if ont.BaseIri == nil || ont.Version == nil {
		return nil, ontologyV1.ErrorOntologyNotFound("本体数据不完整：%d", req.GetOntologyId())
	}

	entities, axioms, err := s.loadOntologySnapshot(ctx, req.GetOntologyId())
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("load ontology snapshot failed")
	}

	triples, warnings := owlio.RowsToTriples(entities, axioms)
	for _, w := range warnings {
		s.log.Warnf(ctx, "owl export: %s", w)
	}
	version := derefU32(ont.Version)
	triples = owlio.AppendHeaderTriples(triples, owlio.OntologyHeader{
		IRI:         derefStr(ont.OntologyIri),
		Version:     version,
		Description: derefStr(ont.Description),
	})

	content, err := owlio.Write(triples, owlio.Format(format), derefStr(ont.OntologyIri))
	if err != nil {
		return nil, ontologyV1.ErrorOwlExportFailed("导出序列化失败：%s", err.Error())
	}

	ext := ".owl"
	if format == string(owlio.FormatTurtle) {
		ext = ".ttl"
	}
	fileName := fmt.Sprintf("%s_v%d%s", derefStr(ont.OntologyName), version, ext)

	return &ontologyV1.OwlExportResponse{
		FileName:    fileName,
		ContentType: "application/octet-stream",
		Content:     content,
	}, nil
}

// loadOntologySnapshot 单条 SELECT 全量快照（导出只读不做乐观锁；无中间态）。
func (s *OntologyOwlIOService) loadOntologySnapshot(ctx context.Context, ontologyID uint32) ([]*ent.OntEntity, []*ent.OntAxiom, error) {
	entities, err := s.entityRepo.ListAllEntities(ctx, ontologyID)
	if err != nil {
		return nil, nil, err
	}
	axioms, err := s.axiomRepo.ListByOntology(ctx, ontologyID)
	if err != nil {
		return nil, nil, err
	}
	return entities, axioms, nil
}

// ---- ImportPreview（§4.3） ----

// ImportPreview 解析 + 反向映射 + 逐行预校验，产出报告（不落库）。
func (s *OntologyOwlIOService) ImportPreview(ctx context.Context, req *ontologyV1.OwlImportPreviewRequest) (*ontologyV1.OwlImportReport, error) {
	if req == nil || len(req.GetFileContent()) == 0 {
		return nil, adminV1.ErrorBadRequest("file content is required")
	}
	if len(req.GetFileContent()) > owlio.MaxImportBytes {
		return nil, ontologyV1.ErrorOwlImportFileTooLarge("文件超过导入上限（5MB），请拆分后导入")
	}

	format := owlio.DetectFormat(req.GetFileContent())
	triples, err := owlio.Parse(req.GetFileContent())
	if err != nil {
		return nil, errInvalid(err)
	}

	plan, err := owlio.TriplesToPlan(triples, req.GetFileName())
	if err != nil {
		return nil, mapDomainError(err)
	}
	plan.SourceFormat = format
	plan.Report.SourceFormat = format.Label()

	if plan.Report.ImportsCount > 0 {
		plan.Report.Warnings = append(plan.Report.Warnings,
			fmt.Sprintf("检测到 %d 条 owl:imports，依赖文档未解析，被引用实体已按自动补声明兜底", plan.Report.ImportsCount))
	}

	if _, err := s.planStore.Put(plan); err != nil {
		return nil, adminV1.ErrorInternalServerError("store import plan failed")
	}
	return reportToProto(plan), nil
}

// ---- ImportExecute（§4.4） ----

// ImportExecute 按 reportId 落库（新建本体；reportId 一次性消费）。
func (s *OntologyOwlIOService) ImportExecute(ctx context.Context, req *ontologyV1.OwlImportExecuteRequest) (*ontologyV1.OwlImportExecuteResponse, error) {
	if req == nil || strings.TrimSpace(req.GetReportId()) == "" {
		return nil, adminV1.ErrorBadRequest("report id is required")
	}
	name := strings.TrimSpace(req.GetOntologyName())
	if name == "" {
		return nil, adminV1.ErrorBadRequest("ontology name is required")
	}
	if utf8.RuneCountInString(name) > owlMaxNameLen {
		return nil, adminV1.ErrorBadRequest("ontology name too long (≤%d)", owlMaxNameLen)
	}
	if strings.TrimSpace(req.GetBaseIri()) == "" {
		return nil, adminV1.ErrorBadRequest("base iri is required")
	}
	if utf8.RuneCountInString(req.GetDescription()) > owlMaxDescriptionLen {
		return nil, adminV1.ErrorBadRequest("description too long (≤%d)", owlMaxDescriptionLen)
	}

	// 1. get/remove 两段式（源修订 #4）：校验未通过不消费计划，补参重跑可用
	plan := s.planStore.Get(req.GetReportId())
	if plan == nil {
		return nil, ontologyV1.ErrorOwlImportReportExpired("预检报告已过期，请重新上传文件")
	}

	// 2. 名称查重 / base_iri 规范化（复用本体创建校验；plan 未消费，失败可重跑）
	if exists, err := s.ontologyRepo.ExistsName(ctx, name); err != nil {
		return nil, adminV1.ErrorInternalServerError("check ontology name failed")
	} else if exists {
		return nil, ontologyV1.ErrorOntologyNameDuplicate("已存在同名本体：%s", name)
	}
	baseIRI, err := data.NormalizeBaseIRI(req.GetBaseIri())
	if err != nil {
		return nil, mapDomainError(err)
	}
	// 本体 IRI：缺省取文件 header 主体；全局唯一，被占用时用户改参重跑（plan 未消费）
	ontologyIRI := strings.TrimSpace(req.GetOntologyIri())
	if ontologyIRI == "" {
		ontologyIRI = plan.OntologyIRI
	}
	if ontologyIRI != "" {
		if exists, err := s.ontologyRepo.ExistsIRI(ctx, ontologyIRI); err != nil {
			return nil, adminV1.ErrorInternalServerError("check ontology iri failed")
		} else if exists {
			return nil, ontologyV1.ErrorOntologyIriDuplicate("已存在相同 IRI 的本体：%s", ontologyIRI)
		}
	}

	// 3. 忽略清单确认纪律（§4.3）：确认通过后才消费 reportId（remove 后二次提交报 EXPIRED）
	if plan.Report.IgnoredCount > 0 && !req.GetConfirmedIgnore() {
		return nil, ontologyV1.ErrorOwlImportNotConfirmed(
			"存在将被忽略的公理（%d 条），请确认忽略清单后重试", plan.Report.IgnoredCount)
	}
	s.planStore.Remove(req.GetReportId())

	operator := operatorUserID(ctx)
	ontologyID := uint32(0)

	err = s.axiomRepo.WithRawTx(ctx, func(ctx context.Context, tx ontology.AxiomTx, raw *ent.Tx) error {
		id, verr := s.executeInTx(ctx, tx, raw, plan, ontologyIRI, name, baseIRI, req.GetDescription(), operator)
		if verr != nil {
			return verr
		}
		ontologyID = id
		return nil
	})
	if err != nil {
		return nil, mapDomainError(err)
	}

	s.log.Infof(ctx, "owl import done: ontology=%d entities=%d axioms=%d ignored=%d",
		ontologyID, len(plan.Entities), len(plan.Axioms), plan.Report.IgnoredCount)
	return &ontologyV1.OwlImportExecuteResponse{OntologyId: ontologyID}, nil
}

// executeInTx 事务内落库管线（§4.4 步骤 2~8；任何一步出错整体回滚）。
func (s *OntologyOwlIOService) executeInTx(
	ctx context.Context, tx ontology.AxiomTx, raw *ent.Tx,
	plan *owlio.ImportPlan, ontologyIRI, name, baseIRI, description string, operator uint32,
) (uint32, error) {
	now := time.Now()

	// 2. 本体行 insert（version=0）
	builder := raw.OntOntology.Create().
		SetOntologyName(name).
		SetBaseIri(baseIRI).
		SetDescription(description).
		SetVersion(0).
		SetNillableCreatedBy(trans.Ptr(operator)).
		SetCreatedAt(now)
	if ontologyIRI != "" {
		builder = builder.SetOntologyIri(ontologyIRI)
	}
	ontRow, err := builder.Save(ctx)
	if err != nil {
		s.log.Errorf(ctx, "insert ontology failed: %s", err.Error())
		if ent.IsConstraintError(err) {
			return 0, ontology.NewError(ontology.ReasonOntologyIRIDuplicate, "已存在相同 IRI 的本体：%s", ontologyIRI)
		}
		return 0, err
	}
	ontologyID := ontRow.ID

	// 3. 实体行逐条 insert（id 回填是公理引用重写的硬依赖——源修订 #1）
	entityIDs := make([]uint32, len(plan.Entities))
	subjects := make(map[uint32]struct{}, len(plan.Entities))
	for i := range plan.Entities {
		d := plan.Entities[i]
		id, err := tx.InsertEntity(ctx, ontologyID, &ontology.EntityDraft{EntityType: d.EntityType, IRI: d.IRI, LocalName: d.LocalName}, operator)
		if err != nil {
			return 0, err
		}
		entityIDs[i] = id
		subjects[id] = struct{}{}
	}

	// 4. 公理草稿重写（草稿下标 → 实体 id）+ CreateBulk 分批 500
	builders := make([]*ent.OntAxiomCreate, 0, owlAxiomBulkSize)
	flush := func() error {
		if len(builders) == 0 {
			return nil
		}
		if err := raw.OntAxiom.CreateBulk(builders...).Exec(ctx); err != nil {
			s.log.Errorf(ctx, "bulk insert axioms failed: %s", err.Error())
			return err
		}
		builders = builders[:0]
		return nil
	}
	for i := range plan.Axioms {
		d := plan.Axioms[i]
		b := raw.OntAxiom.Create().
			SetOntologyID(ontologyID).
			SetAxiomType(d.AxiomType).
			SetSubjectID(entityIDs[d.SubjectRef]).
			SetSortOrder(uint32SortOrder(d.SortOrder)).
			SetNillableCreatedBy(trans.Ptr(operator)).
			SetCreatedAt(now)
		if d.ObjectRef >= 0 {
			b.SetObjectID(entityIDs[d.ObjectRef])
		}
		if d.PredicateIRI != "" {
			b.SetPredicateIri(d.PredicateIRI)
		}
		if d.PredicateRef >= 0 {
			b.SetObjectID(entityIDs[d.PredicateRef])
		}
		if d.AxiomType == ontology.AxiomSubPropertyChainOf {
			// 37 链：成员下标 → 实体 id JSON 数组（保序）
			ids := make([]uint32, 0, len(d.MemberRefs))
			for _, ref := range d.MemberRefs {
				ids = append(ids, entityIDs[ref])
			}
			b.SetValueText(ontology.EncodeChainMemberIDs(ids))
		} else if d.ValueText != "" {
			b.SetValueText(d.ValueText)
		}
		if d.Lang != "" {
			b.SetLang(d.Lang)
		}
		builders = append(builders, b)
		if len(builders) >= owlAxiomBulkSize {
			if err := flush(); err != nil {
				return 0, err
			}
		}
	}
	if err := flush(); err != nil {
		return 0, err
	}

	// 5. 渲染缓存同步（display_label / deprecated；全部实体）
	if err := s.applier.SyncAnnotationCache(ctx, tx, ontologyID, subjects); err != nil {
		return 0, err
	}

	// 6. 全量校验（防御性断言：preview 已逐行预校验；失败 = plan 与库状态并发冲突，整体回滚）
	entities, err := tx.ListEntities(ctx, ontologyID)
	if err != nil {
		return 0, err
	}
	axioms, err := tx.ListAxioms(ctx, ontologyID)
	if err != nil {
		return 0, err
	}
	if problems := ontology.ValidateAxioms(ctx, baseIRI, entities, axioms, s.log); len(problems) > 0 {
		msgs := make([]string, 0, len(problems))
		for _, p := range problems {
			msgs = append(msgs, p.Error())
		}
		return 0, ontology.NewError(ontology.ReasonAxiomInvalid, "导入校验失败：%s", joinProblemMsgs(msgs))
	}

	// 7. 版本 0→1
	if err := tx.BumpVersion(ctx, ontologyID, 1); err != nil {
		return 0, err
	}

	// 8. 号池回填（§2.7）
	backfillWarnings, err := s.backfillSerialPools(ctx, raw, ontologyID, entities, axioms)
	if err != nil {
		return 0, err
	}

	// 9. 变更日志（operation=IMPORT_OWL，detail=摘要 JSON；before 0 → after 1）
	detail, err := owlImportDetailJSON(plan, backfillWarnings)
	if err != nil {
		return 0, err
	}
	if err := tx.InsertChangeLog(ctx, &ontology.ChangeLogEntry{
		Operation:     ontology.OpImportOwl,
		ChangeDetail:  detail,
		BeforeVersion: 0,
		AfterVersion:  1,
	}, ontologyID, operator); err != nil {
		return 0, err
	}
	return ontologyID, nil
}

// backfillSerialPools 号池回填（§4.5）：
//   - code 池：以本本体全部 skos:notation 值为前缀候选，62 断言值匹配 ^{p}-(\d+)$
//     （前缀含 '-' 时由前缀集合匹配天然按最后一次 '-' 分割，流水段必须全数字）取 max N →
//     upsert(purpose=code, next_no=N+1)；无匹配的前缀不建池；未匹配前缀形态的编码 warning。
//   - ind 池：导入个体 localName 匹配 ^i(\d{6,})$ 取 max → upsert(ind, $ind, max+1)。
func (s *OntologyOwlIOService) backfillSerialPools(ctx context.Context, raw *ent.Tx, ontologyID uint32, entities []*ent.OntEntity, axioms []*ent.OntAxiom) ([]string, error) {
	warnings := make([]string, 0, 2)

	// 前缀候选：全部 skos:notation 注解值（06 期编码口径——主类目编码）
	prefixes := make([]string, 0, 4)
	seen := map[string]struct{}{}
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomAnnotationAssertion {
			continue
		}
		if ax.PredicateIri == nil || ax.ValueText == nil || *ax.ValueText == "" {
			continue
		}
		if *ax.PredicateIri != ontology.PredicateSKOSNotation {
			continue
		}
		if _, dup := seen[*ax.ValueText]; dup {
			continue
		}
		seen[*ax.ValueText] = struct{}{}
		prefixes = append(prefixes, *ax.ValueText)
	}

	// 62 断言值 → 按前缀聚合取最大流水
	maxByPrefix := map[string]uint64{}
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomDataPropertyAssertion || ax.ValueText == nil {
			continue
		}
		code := *ax.ValueText
		matched := false
		for _, p := range prefixes {
			if !strings.HasPrefix(code, p+"-") {
				continue
			}
			serial := code[len(p)+1:]
			n, err := strconv.ParseUint(serial, 10, 64)
			if err != nil {
				continue // 流水段非全数字：不匹配
			}
			if n > maxByPrefix[p] {
				maxByPrefix[p] = n
			}
			matched = true
			break
		}
		if !matched {
			warnings = append(warnings, fmt.Sprintf("编码 %s 未匹配任何前缀池形态，号池未回填（与手工编码撞号由既有查重兜底）", code))
		}
	}
	orderedPrefixes := make([]string, 0, len(maxByPrefix))
	for _, p := range prefixes {
		if _, ok := maxByPrefix[p]; ok {
			orderedPrefixes = append(orderedPrefixes, p)
		}
	}
	for _, p := range orderedPrefixes {
		if err := s.axiomRepo.UpsertSerialPool(ctx, raw, ontologyID, ontology.SerialPurposeCode, p, maxByPrefix[p]+1); err != nil {
			return warnings, err
		}
	}

	// ind 池：个体 localName 流水
	maxInd := uint64(0)
	for _, e := range entities {
		if e.EntityType == nil || *e.EntityType != ontology.EntityTypeIndividual || e.LocalName == nil {
			continue
		}
		if m := indLocalNamePattern.FindStringSubmatch(*e.LocalName); m != nil {
			if n, err := strconv.ParseUint(m[1], 10, 64); err == nil && n > maxInd {
				maxInd = n
			}
		}
	}
	if maxInd > 0 {
		if err := s.axiomRepo.UpsertSerialPool(ctx, raw, ontologyID, ontology.SerialPurposeInd, ontology.SerialIndPrefix, maxInd+1); err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

// owlImportDetailJSON 导入摘要 JSON（变更日志 detail）。
func owlImportDetailJSON(plan *owlio.ImportPlan, warnings []string) (string, error) {
	parts := []string{
		`"sourceFormat":` + strconv.Quote(plan.SourceFormat.Label()),
		fmt.Sprintf(`"entities":%d`, len(plan.Entities)),
		fmt.Sprintf(`"axioms":%d`, len(plan.Axioms)),
		fmt.Sprintf(`"ignored":%d`, plan.Report.IgnoredCount),
		fmt.Sprintf(`"autoDeclared":%d`, len(plan.Report.AutoDeclared)),
	}
	if len(warnings) > 0 {
		quoted := make([]string, 0, len(warnings))
		for _, w := range warnings {
			quoted = append(quoted, strconv.Quote(w))
		}
		parts = append(parts, `"warnings":[`+strings.Join(quoted, ",")+`]`)
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}

// joinProblemMsgs 校验失败明细拼接（截断防超长）。
func joinProblemMsgs(msgs []string) string {
	const max = 8
	if len(msgs) > max {
		return fmt.Sprintf("%s 等 %d 处", msgs[:max], len(msgs))
	}
	return strings.Join(msgs, "；")
}

func uint32SortOrder(v int32) uint32 {
	if v < 0 {
		return 0
	}
	return uint32(v)
}

// reportToProto owlio.ImportReport → proto 消息（§4.3 展示契约）。
func reportToProto(plan *owlio.ImportPlan) *ontologyV1.OwlImportReport {
	r := &plan.Report
	out := &ontologyV1.OwlImportReport{
		ReportId:     r.ReportID,
		SourceFormat: r.SourceFormat,
		EntityStats:  map[string]int32{},
		AxiomStats:   map[uint32]int32{},
		Warnings:     make([]string, 0, len(r.Warnings)),
		ImportsCount: int32(r.ImportsCount),
		Ignored: &ontologyV1.OwlIgnoredSummary{
			Count:    int32(r.IgnoredCount),
			ByReason: map[string]int32{},
			Samples:  make([]*ontologyV1.OwlIgnoredSample, 0, len(r.IgnoredSamples)),
		},
	}
	out.Suggested = &ontologyV1.OwlImportSuggested{
		OntologyName: r.SuggestedName,
		BaseIri:      r.SuggestedBaseIRI,
	}
	if r.SuggestedDescription != "" {
		out.Suggested.Description = &r.SuggestedDescription
	}
	if r.SuggestedOntologyIRI != "" {
		out.Suggested.OntologyIri = &r.SuggestedOntologyIRI
	}
	for k, v := range r.EntityStats {
		out.EntityStats[k] = int32(v)
	}
	for k, v := range r.AxiomStats {
		out.AxiomStats[k] = int32(v)
	}
	for _, a := range r.AutoDeclared {
		out.AutoDeclared = append(out.AutoDeclared, &ontologyV1.OwlAutoDeclaredRef{
			Iri:        a.IRI,
			EntityType: a.EntityType,
			Reason:     a.Reason,
		})
	}
	for _, rn := range r.RenamedLocalNames {
		out.RenamedLocalNames = append(out.RenamedLocalNames, &ontologyV1.OwlRenamedLocalName{
			Iri:       rn.IRI,
			LocalName: rn.LocalName,
			Original:  rn.Original,
		})
	}
	out.Warnings = append(out.Warnings, r.Warnings...)
	for k, v := range r.IgnoredByReason {
		out.Ignored.ByReason[k] = int32(v)
	}
	for _, sm := range r.IgnoredSamples {
		out.Ignored.Samples = append(out.Ignored.Samples, &ontologyV1.OwlIgnoredSample{
			Reason: sm.Reason,
			Detail: sm.Detail,
		})
	}
	return out
}
