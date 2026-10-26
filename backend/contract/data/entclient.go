package data

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"

	_ "github.com/lib/pq" // postgres 驱动（铁律 4：关系型存储只允许 PostgreSQL）

	entCrud "github.com/tx7do/go-crud/entgo"

	"github.com/tx7do/kratos-bootstrap/bootstrap"
	entBootstrap "github.com/tx7do/kratos-bootstrap/database/ent"

	"go-wind-admin/contract/data/ent"
	_ "go-wind-admin/contract/data/ent/runtime"
)

// NewEntClient 创建合同管理模块的 Ent ORM 数据库客户端。
//
// 结构迁移（DDL）不在本模块执行：goose 版本化迁移由 admin 服务统一承载
// （app/admin/service/internal/data/migrations/，全仓铁律 6），本模块只做
// 连接构建；data.database.migrate 语义与表结构归 admin 服务所有。
func NewEntClient(ctx *bootstrap.Context) (*entCrud.EntClient[*ent.Client], func(), error) {
	l := ctx.NewLoggerHelper("ent/data/contract")

	cfg := ctx.GetConfig()
	if cfg == nil || cfg.Data == nil {
		l.Errorf(context.Background(), "[ENT] failed getting config")
		panic("[ENT] failed getting config")
	}

	cli, err := entBootstrap.NewEntClient(cfg, func(drv *sql.Driver) *ent.Client {
		client := ent.NewClient(
			ent.Driver(drv),
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
