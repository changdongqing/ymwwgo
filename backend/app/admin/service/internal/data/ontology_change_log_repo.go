package data

import (
	"context"

	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"

	entCrud "github.com/tx7do/go-crud/entgo"

	"go-wind-admin/app/admin/service/internal/data/ent"
	"go-wind-admin/app/admin/service/internal/data/ent/ontchangelog"
)

// OntChangeLogRepo 本体变更日志仓储（对应 Protégé HistoryManager；V1 只记不回滚）。
// 日志写入由公理事务管线（ontologyTx.InsertChangeLog）承载；本体删除时随本体级联软删。
type OntChangeLogRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *bLogger.Helper
}

func NewOntChangeLogRepo(
	ctx *bootstrap.Context,
	entClient *entCrud.EntClient[*ent.Client],
) *OntChangeLogRepo {
	return &OntChangeLogRepo{
		log:       ctx.NewLoggerHelper("ontology-change-log/repo/admin-service"),
		entClient: entClient,
	}
}

// ListByOntology 按本体倒序列出变更日志（V1 无对外端点，排障/后续 History 视图用）
func (r *OntChangeLogRepo) ListByOntology(ctx context.Context, ontologyID uint32, limit int) ([]*ent.OntChangeLog, error) {
	q := r.entClient.Client().OntChangeLog.Query().
		Where(ontchangelog.OntologyIDEQ(ontologyID), ontchangelog.DeletedAtIsNil()).
		Order(ent.Desc(ontchangelog.FieldCreatedAt), ent.Desc(ontchangelog.FieldID))
	if limit > 0 {
		q = q.Limit(limit)
	}
	return q.All(ctx)
}
