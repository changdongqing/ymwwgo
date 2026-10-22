// Package migrations 持有数据库结构的版本化迁移（goose），是全库 DDL 的唯一执行通道。
//
// 设计文档: docs/database/design/ent-goose-migrations.md（全仓铁律 6）。
//   - 迁移 SQL 经 go:embed 打进二进制：服务启动（data.database.migrate: true）与
//     cmd/migrate CLI 共用同一份；
//   - 改 Ent schema 必须在同一 commit 里新增迁移文件，CI 漂移守卫校验两者一致；
//   - 只支持 postgres（全仓铁律 4），驱动由调用方按配置装配。
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	"github.com/pressly/goose/v3/lock"
)

//go:embed *.sql
var files embed.FS

// versionTable goose 版本表名（goose 默认值）。显式写出以便 baseline 纳管与运维查询。
const versionTable = "goose_db_version"

// newProvider 装配 goose Provider：postgres 方言 + 会话级 advisory lock
// （多副本同时启动时迁移互斥，由 goose 以 pg_try_advisory_lock 实现）。
// verbose 打开：goose 解析/执行轨迹走标准 log，配合 MigrationResult.Empty 排查"迁移空跑"。
func newProvider(db *sql.DB) (*goose.Provider, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("migrations: create session locker: %w", err)
	}
	return goose.NewProvider(goose.DialectPostgres, db, files,
		goose.WithSessionLocker(locker),
		goose.WithVerbose(true),
	)
}

// Up 应用全部未执行的迁移，返回本次实际应用的条数。
func Up(ctx context.Context, db *sql.DB) (int, error) {
	p, err := newProvider(db)
	if err != nil {
		return 0, err
	}
	results, err := p.Up(ctx)
	if err != nil {
		return 0, err
	}
	for _, r := range results {
		if r.Error != nil {
			return 0, fmt.Errorf("migrations: apply %s: %w", r.Source.Path, r.Error)
		}
		if r.Empty {
			// goose 判定该迁移没有任何可执行语句（解析为空）：结构不会落库，必须显式暴露。
			return 0, fmt.Errorf("migrations: %s 解析结果为空（无任何 SQL 语句），拒绝记版本", r.Source.Path)
		}
		fmt.Printf("goose: applied %05d %s (%s)\n", r.Source.Version, r.Source.Path, r.Duration)
	}
	return len(results), nil
}

// Status 打印各迁移的已应用/待应用状态。
func Status(ctx context.Context, db *sql.DB) error {
	store, err := database.NewStore(goose.DialectPostgres, versionTable)
	if err != nil {
		return err
	}
	rows, err := store.ListMigrations(ctx, db)
	if err != nil {
		return fmt.Errorf("migrations: list version table: %w", err)
	}
	applied := make(map[int64]bool, len(rows))
	for _, r := range rows {
		if r.IsApplied {
			applied[r.Version] = true
		}
	}
	list, err := files.ReadDir(".")
	if err != nil {
		return err
	}
	for _, e := range list {
		v, err := fileVersion(e.Name())
		if err != nil {
			return err
		}
		if applied[v] {
			fmt.Printf("applied  %05d  %s\n", v, e.Name())
		} else {
			fmt.Printf("pending  %05d  %s\n", v, e.Name())
		}
	}
	return nil
}

// EnsureBaseline 把 version 标记为"已应用"但不执行任何 SQL——给 goose 引入前就存在的
// 存量库纳管用（其结构已是基线状态，只补版本记录，见设计文档 §8）。
// 幂等：版本表/基线记录已存在时不重复创建。
func EnsureBaseline(ctx context.Context, db *sql.DB, version int64) error {
	store, err := database.NewStore(goose.DialectPostgres, versionTable)
	if err != nil {
		return err
	}

	// 版本表可能已存在（重复执行，或服务启动的迁移抢先建过）：按存在性判定。
	var exists bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1)`,
		versionTable,
	).Scan(&exists); err != nil {
		return fmt.Errorf("migrations: inspect version table: %w", err)
	}
	if !exists {
		if err := store.CreateVersionTable(ctx, db); err != nil {
			return fmt.Errorf("migrations: create version table: %w", err)
		}
		// 与 goose Provider 自身的建表行为保持一致：先补 version=0 初始行。
		if err := store.Insert(ctx, db, database.InsertRequest{Version: 0}); err != nil {
			return fmt.Errorf("migrations: seed version 0: %w", err)
		}
	}

	if _, err := store.GetMigration(ctx, db, version); err == nil {
		fmt.Printf("goose: version %d already baselined\n", version)
		return nil
	}
	if err := store.Insert(ctx, db, database.InsertRequest{Version: version}); err != nil {
		return fmt.Errorf("migrations: insert baseline version %d: %w", version, err)
	}
	fmt.Printf("goose: version %d marked as applied (baseline)\n", version)
	return nil
}

// MaxVersion 返回内嵌迁移的最大版本号（baseline 的默认目标）。
func MaxVersion() (int64, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return 0, err
	}
	var max int64
	for _, e := range entries {
		v, err := fileVersion(e.Name())
		if err != nil {
			return 0, err
		}
		if v > max {
			max = v
		}
	}
	if max == 0 {
		return 0, fmt.Errorf("migrations: no migration files embedded")
	}
	return max, nil
}

// fileVersion 从迁移文件名解析版本号（约定 "000NN_<module>_<what>.sql"）。
func fileVersion(name string) (int64, error) {
	base, _, _ := strings.Cut(name, "_")
	v, err := strconv.ParseInt(base, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("migrations: 迁移文件 %s 不符合 000NN_<module>_<what>.sql 命名约定", name)
	}
	return v, nil
}
