package data

import (
	"context"
	"time"

	"github.com/tx7do/go-utils/trans"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"

	entCrud "github.com/tx7do/go-crud/entgo"

	"go-wind-admin/app/admin/service/internal/data/ent"
	"go-wind-admin/app/admin/service/internal/data/ent/ontserial"
	"go-wind-admin/app/admin/service/internal/ontology"
)

// OntSerialRepo 取号计数仓储（06 号文档 §4.1）。
//
// NextNo 只在传入的 ent 事务上执行（不自带事务）——取号与公理写同事务（ApplyInTx），
// 乐观锁冲突/校验失败整体回滚时 next_no 一并回退，号不损耗。
// 并发语义（06 号文档 §2.3）：两段式（UPDATE 未命中 → INSERT 首行 → 唯一键冲突重试 UPDATE），
// 行锁持至提交保证串行发放；uix_ont_serials_key 完整唯一索引兜底"取号路径唯一命中一行"。
type OntSerialRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *bLogger.Helper
}

func NewOntSerialRepo(
	ctx *bootstrap.Context,
	entClient *entCrud.EntClient[*ent.Client],
) *OntSerialRepo {
	return &OntSerialRepo{
		log:       ctx.NewLoggerHelper("ontology-serial/repo/admin-service"),
		entClient: entClient,
	}
}

// NextNo 两段式取号（06 号文档 §4.1）：
//
// ① UPDATE ont_serials SET next_no = next_no + 1 WHERE ontology_id=? AND purpose=? AND prefix=?
//
//	影响行数=1 → 同 Tx 回读 next_no，本次发放号 = next_no - 1（行锁持至提交，回读稳定）
//
// ② 影响行数=0（该前缀首次使用）→ INSERT 号池行（next_no=2，首号=1）；
//
//	并发首建撞 uix_ont_serials_key（23505）→ 重试 ① 路径（MAX_RETRY=3，必然收敛）；
//	重试耗尽 → INDIVIDUAL_SERIAL_CONFLICT。
func (r *OntSerialRepo) NextNo(ctx context.Context, tx *ent.Tx, ontologyID uint32, purpose, prefix string) (uint64, error) {
	return nextSerialNo(ctx, tx, r.log, ontologyID, purpose, prefix)
}

// PeekNo 不加锁读下一流水号（code-preview 预览用；并发下实际号可能 +N，文案必须写"预计"）。
// 号池行不存在（该前缀从未取过号）返回 1。
func (r *OntSerialRepo) PeekNo(ctx context.Context, ontologyID uint32, purpose, prefix string) (uint64, error) {
	row, err := r.entClient.Client().OntSerial.Query().
		Where(
			ontserial.OntologyIDEQ(ontologyID),
			ontserial.PurposeEQ(purpose),
			ontserial.PrefixEQ(prefix),
			ontserial.DeletedAtIsNil(),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 1, nil
		}
		return 0, err
	}
	if row.NextNo == nil {
		return 1, nil
	}
	return *row.NextNo, nil
}

// nextSerialNo 两段式取号实现（OntSerialRepo.NextNo 与 ontologyTx.NextNo 共用；tx 已在调用方事务内）。
func nextSerialNo(ctx context.Context, tx *ent.Tx, log *bLogger.Helper, ontologyID uint32, purpose, prefix string) (uint64, error) {
	const maxRetry = 3
	for attempt := 0; ; attempt++ {
		// ① UPDATE 路径：行锁持至提交，同 Tx 回读稳定
		n, err := tx.OntSerial.Update().
			Where(
				ontserial.OntologyIDEQ(ontologyID),
				ontserial.PurposeEQ(purpose),
				ontserial.PrefixEQ(prefix),
				ontserial.DeletedAtIsNil(),
			).
			AddNextNo(1).
			Save(ctx)
		if err != nil {
			if log != nil {
				log.Errorf(ctx, "update serial next_no failed: %s", err.Error())
			}
			return 0, err
		}
		if n > 0 {
			row, err := tx.OntSerial.Query().
				Where(
					ontserial.OntologyIDEQ(ontologyID),
					ontserial.PurposeEQ(purpose),
					ontserial.PrefixEQ(prefix),
					ontserial.DeletedAtIsNil(),
				).
				Only(ctx)
			if err != nil {
				if log != nil {
					log.Errorf(ctx, "reread serial next_no failed: %s", err.Error())
				}
				return 0, err
			}
			if row.NextNo == nil {
				return 0, ontology.NewError(ontology.ReasonIndividualSerialConflict, "取号并发冲突，请重试")
			}
			return *row.NextNo - 1, nil
		}

		// ② 首建路径：该 (ontology, purpose, prefix) 尚无号池行，next_no=2（首号=1）
		err = tx.OntSerial.Create().
			SetOntologyID(ontologyID).
			SetPurpose(purpose).
			SetPrefix(prefix).
			SetNextNo(2).
			Exec(ctx)
		if err == nil {
			return 1, nil
		}
		if !ent.IsConstraintError(err) {
			if log != nil {
				log.Errorf(ctx, "insert serial row failed: %s", err.Error())
			}
			return 0, err
		}
		// 唯一键冲突（23505）：并发首建，另一事务已插入并持行锁，重试 ① 路径收敛
		if attempt >= maxRetry-1 {
			if log != nil {
				log.Errorf(ctx, "serial first-insert retry exhausted (attempt %d): %s", attempt+1, err.Error())
			}
			return 0, ontology.NewError(ontology.ReasonIndividualSerialConflict, "取号并发冲突，请重试")
		}
	}
}

// DeleteByOntologies 本体删除级联软删号池行（06 号文档 §3.1：号池行是本体下无实体时的唯一残留；
// 重建同名本体视为全新编号空间）。在 OntologyRepo.Delete 的级联事务内调用。
func DeleteByOntologies(ctx context.Context, tx *ent.Tx, ontologyIDs []uint32, deletedBy uint32, now time.Time) error {
	_, err := tx.OntSerial.Update().
		Where(
			ontserial.OntologyIDIn(ontologyIDs...),
			ontserial.DeletedAtIsNil(),
		).
		SetDeletedAt(now).
		SetNillableDeletedBy(trans.Ptr(deletedBy)).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}
