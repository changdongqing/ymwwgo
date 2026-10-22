package data

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/tx7do/kratos-bootstrap/bootstrap"

	"go-wind-admin/app/admin/service/internal/data/migrations"
)

// RunSchemaMigrations 按 Bootstrap 配置执行 goose 版本化迁移——结构迁移（DDL）的唯一入口。
//
// 设计文档: docs/database/design/ent-goose-migrations.md（全仓铁律 6）。
//   - 由 NewEntClient 在 data.database.migrate: true 时调用（多副本部署可置 false，
//     由部署链先行执行 cmd/migrate up）；
//   - 历史的 Ent 自动迁移（Schema.Create）已移除：改表结构一律走 migrations/ 新增迁移文件，
//     Ent schema 仍保持"结构与代码生成的单一事实源"，CI 漂移守卫校验两者一致；
//   - 只支持 postgres（全仓铁律 4），驱动沿用与 Ent 相同的 "postgres"（lib/pq，已在
//     ent_client.go blank import）。
func RunSchemaMigrations(ctx context.Context, cfg *bootstrap.Context) error {
	dbCfg := cfg.GetConfig().Data.Database
	if drv := dbCfg.GetDriver(); drv != "postgres" {
		return fmt.Errorf("schema migrations: unsupported driver %q (postgres only, 全仓铁律 4)", drv)
	}

	db, err := sql.Open("postgres", dbCfg.GetSource())
	if err != nil {
		return fmt.Errorf("schema migrations: open db: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			fmt.Printf("schema migrations: close db: %v\n", closeErr)
		}
	}()

	n, err := migrations.Up(ctx, db)
	if err != nil {
		return fmt.Errorf("schema migrations: %w", err)
	}
	if n > 0 {
		fmt.Printf("schema migrations: %d 条迁移已应用\n", n)
	}
	return nil
}
