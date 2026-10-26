# contract 独立 Go module 的代码生成。
# 该 module 不是可运行服务（无 cmd/），由 admin 服务以 Go module 方式引入装配；
# 结构迁移（DDL）统一由 admin 服务的 goose 迁移链承载（全仓铁律 6），本模块不做 DDL。
.PHONY: ent api gen tidy

# 生成 ent 代码（feature 集合与 onestop/主仓保持一致：Modify(upsert/lock) 等被 go-crud 仓储依赖）
# 走 `go run` 而非全局 ent 二进制（理由同 onestop Makefile）。
ent:
	go run entgo.io/ent/cmd/ent generate \
		--feature privacy \
		--feature entql \
		--feature sql/modifier \
		--feature sql/upsert \
		--feature sql/lock \
		./data/ent/schema

# 生成 proto Go 代码。
# 源 proto 与 buf 工作区都在主仓 backend/api（模板 buf.contract.gen.yaml），
# 本 module 没有独立的 buf.yaml；在 backend/api 目录下执行 buf generate。
api:
	cd ../api && buf generate --template buf.contract.gen.yaml

gen: ent api

tidy:
	go mod tidy
