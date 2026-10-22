# onestop 独立 Go module 的代码生成。
# 该 module 不是可运行服务（无 cmd/），由 admin 服务以 Go module 方式引入装配；
# 结构迁移（DDL）统一由 admin 服务的 goose 迁移链承载（全仓铁律 6），本模块不做 DDL。
.PHONY: ent api gen tidy

# 生成 ent 代码（feature 集合与主仓 app.mk 保持一致：Modify(upsert/lock) 等被 go-crud 仓储依赖）
# 走 `go run` 而非全局 ent 二进制：全局二进制与当前工具链组合会报
# "package \"context\" without types"（导出数据加载失败），模块内运行无此问题。
ent:
	go run entgo.io/ent/cmd/ent generate \
		--feature privacy \
		--feature entql \
		--feature sql/modifier \
		--feature sql/upsert \
		--feature sql/lock \
		./data/ent/schema

# 生成 proto Go 代码（vendored 依赖快照见 api/buf.yaml 头注释）
api:
	cd api && buf generate

gen: ent api

tidy:
	go mod tidy
