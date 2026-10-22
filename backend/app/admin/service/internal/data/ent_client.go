package data

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/lib/pq"

	entCrud "github.com/tx7do/go-crud/entgo"

	"github.com/tx7do/kratos-bootstrap/bootstrap"
	entBootstrap "github.com/tx7do/kratos-bootstrap/database/ent"

	"go-wind-admin/app/admin/service/internal/data/ent"
	_ "go-wind-admin/app/admin/service/internal/data/ent/runtime"
)

// NewEntClient 创建Ent ORM数据库客户端
func NewEntClient(ctx *bootstrap.Context) (*entCrud.EntClient[*ent.Client], func(), error) {
	l := ctx.NewLoggerHelper("ent/data/admin-service")

	cfg := ctx.GetConfig()
	if cfg == nil || cfg.Data == nil {
		l.Errorf(context.Background(), "[ENT] failed getting config")
		panic("[ENT] failed getting config")
	}

	// 结构迁移：goose 版本化迁移是 DDL 的唯一通道（docs/database/design/ent-goose-migrations.md，
	// 全仓铁律 6）。历史的 Ent 自动迁移（Schema.Create）已移除——改表结构一律在
	// internal/data/migrations/ 新增迁移文件，CI 漂移守卫校验迁移与 Ent schema 一致。
	// migrate=false 时不做任何 DDL，由部署链先行执行 cmd/migrate up（多副本场景）。
	if cfg.Data.Database.GetMigrate() {
		if err := RunSchemaMigrations(ctx.Context(), ctx); err != nil {
			l.Errorf(context.Background(), "[ENT] failed running schema migrations: %v", err)
			panic("[ENT] failed running schema migrations")
		}
	}

	cli, err := entBootstrap.NewEntClient(cfg, func(drv *sql.Driver) *ent.Client {
		client := ent.NewClient(
			ent.Driver(&auditDriver{drv}),
			ent.Log(func(a ...any) {
				l.Debug(context.Background(), fmt.Sprint(a...))
			}),
		)
		if client == nil {
			l.Errorf(context.Background(), "[ENT] failed creating ent client")
			panic("[ENT] failed creating ent client")
		}

		return client
	})
	if err != nil {
		l.Errorf(context.Background(), "[ENT] failed creating ent client: %v", err)
		panic("[ENT] failed creating ent client")
	}

	return cli, func() {
		if cleanErr := cli.Close(); cleanErr != nil {
			l.Errorf(context.Background(), "[ENT] failed closing ent client: %v", cleanErr)
		}
	}, nil
}
