// migrate 是数据库结构迁移的运维 CLI：goose 版本化迁移的手工执行入口。
//
// 设计文档: docs/database/design/ent-goose-migrations.md。
//
//	 go run ./app/admin/service/cmd/migrate up            -dsn <dsn>   # 应用全部未执行迁移
//	 go run ./app/admin/service/cmd/migrate status        -dsn <dsn>   # 查看迁移状态
//	 go run ./app/admin/service/cmd/migrate baseline      -dsn <dsn>   # 存量库纳管：把基线标记为已应用(不执行)
//	 go run ./app/admin/service/cmd/migrate automigrate-ent -dsn <dsn> # 仅空库：按 Ent schema 全量建表(漂移守卫用)
//
// 服务启动时的自动迁移走 data.NewEntClient（data.database.migrate: true），与本 CLI 共用
// 同一份内嵌 SQL。
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "github.com/lib/pq" // postgres 驱动（配置/DSN 的 driver 名即 "postgres"）

	entsql "entgo.io/ent/dialect/sql"

	"go-wind-admin/app/admin/service/internal/data/ent"
	entmigrate "go-wind-admin/app/admin/service/internal/data/ent/migrate"
	"go-wind-admin/app/admin/service/internal/data/migrations"

	onestopent "go-wind-admin/onestop/data/ent"
	onestopentmigrate "go-wind-admin/onestop/data/ent/migrate"

	contractent "go-wind-admin/contract/data/ent"
	contractentmigrate "go-wind-admin/contract/data/ent/migrate"
)

const usage = `migrate <command> [flags]

commands:
  up               应用全部未执行的迁移
  status           查看各迁移的已应用/待应用状态
  baseline         存量库纳管：把基线标记为已应用而不执行（goose 引入前已存在的库跑一次即可）
  automigrate-ent  仅限空库：按 Ent schema 全量建表（CI 漂移守卫的"期望结构"一侧）

flags:
  -dsn string   postgres DSN（key=value 形式），缺省读 GWA_DATABASE_DSN
  -to int       baseline 目标版本号，缺省取内嵌迁移最大版本
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	// 命令必须在最前：Go 的 flag 包遇到第一个非 flag 参数即停止解析，
	// 这里手动摘出命令，兼容 `migrate up -dsn ...` 与 `migrate -dsn ... up` 两种写法。
	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dsn := fs.String("dsn", os.Getenv("GWA_DATABASE_DSN"), "")
	to := fs.Int64("to", 0, "")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if cmd == "" {
		fs.Usage()
		return errors.New("缺少命令")
	}

	switch cmd {
	case "automigrate-ent":
		db, closeDB, err := openDB(ctx, *dsn)
		if err != nil {
			return err
		}
		defer closeDB()
		return automigrateEnt(ctx, db)
	case "up", "status", "baseline":
		db, closeDB, err := openDB(ctx, *dsn)
		if err != nil {
			return err
		}
		defer closeDB()
		switch cmd {
		case "up":
			n, err := migrations.Up(ctx, db)
			if err != nil {
				return err
			}
			fmt.Printf("goose: %d 条迁移已应用\n", n)
			return nil
		case "status":
			return migrations.Status(ctx, db)
		case "baseline":
			version := *to
			if version == 0 {
				if version, err = migrations.MaxVersion(); err != nil {
					return err
				}
			}
			return migrations.EnsureBaseline(ctx, db, version)
		}
		return nil
	default:
		fs.Usage()
		return fmt.Errorf("未知命令: %s", cmd)
	}
}

// openDB 打开 postgres 连接（只支持 postgres，铁律 4/6）。
func openDB(ctx context.Context, dsn string) (*sql.DB, func(), error) {
	if dsn == "" {
		return nil, nil, errors.New("缺少 -dsn（或环境变量 GWA_DATABASE_DSN）")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("打开数据库: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("连接数据库: %w", err)
	}
	return db, func() { db.Close() }, nil
}

// automigrateEnt 按 Ent schema 全量建表，复刻历史启动行为（Schema.Create）。
// 仅限空库：这是 CI 漂移守卫"期望结构"一侧的生成器，不是常规执行通道。
func automigrateEnt(ctx context.Context, db *sql.DB) error {
	// AI 模块的向量列依赖 pgvector；无该扩展的环境（不部署 AI）忽略此步，
	// 后续 Schema.Create 若真的缺扩展会以明确错误失败。
	if _, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		fmt.Fprintln(os.Stderr, "migrate: 预建 pgvector 扩展失败（无 AI 模块的环境可忽略）:", err)
	}
	drv := entsql.OpenDB("postgres", db)
	client := ent.NewClient(ent.Driver(drv))
	if err := client.Schema.Create(ctx, entmigrate.WithForeignKeys(true)); err != nil {
		return err
	}

	// onestop 独立 module 的 ent schema（独立 runtime，两张 schema 集合互不引用，
	// 无跨模块外键，先后顺序无关）；goose 侧的对应表在 000NN_onestop_*.sql。
	oneStopDrv := entsql.OpenDB("postgres", db)
	oneStopClient := onestopent.NewClient(onestopent.Driver(oneStopDrv))
	if err := oneStopClient.Schema.Create(ctx, onestopentmigrate.WithForeignKeys(true)); err != nil {
		return err
	}

	// contract 独立 module 的 ent schema（同 onestop 口径）；
	// goose 侧的对应表在 000NN_contract_*.sql。
	contractDrv := entsql.OpenDB("postgres", db)
	contractClient := contractent.NewClient(contractent.Driver(contractDrv))
	return contractClient.Schema.Create(ctx, contractentmigrate.WithForeignKeys(true))
}
