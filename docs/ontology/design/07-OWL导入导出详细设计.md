# 本体建模功能详细设计（七）：OWL 导入导出 —— 本体互操作（ymwwgo 版）

| 项 | 内容 |
|---|---|
| 文档版本 | V1.0（2026-10-22 设计定稿，待实现；实现后按 01 号文档 §8 惯例补修订记录） |
| 系列文档 | 01 阶段0 + 本体类（V1.0）→ 02 对象属性 → 03 数据属性 → 04 注解属性 → 05 数据类型 → 06 个体 → **07 OWL 导入导出（本文，系列收官）** |
| 定位 | 六项建模能力齐备后的**互操作门槛**：兑现 01 号文档决策五的预留承诺（"07 期 OWL 导入导出需要 RDF/XML / Turtle 序列化，届时再评估轻量 RDF 库或自写序列化器，由 07 号文档定案"），把本体从"系统内闭环的建模器"变为"可与 Protégé / 任意 OWL 2 工具互通的文档化知识资产" |
| 设计来源 | 源项目 youmingwanwu `docs/ontology/07-OWL导入导出详细设计.md`（V1.1，已实现并验证：17 项新增单测 + 模块全量回归 106 项全绿、export→import→export round-trip 闭环 ×2，含 §12 十项实现修订——**已全部内化进本文正文**）；上游分析依据为 Protégé `protege-editor-owl` 源码（`OntologySaver` / `OntologyLoader` / `UserResolvedIRIMapper`）与 OWL 2 RDF Mapping 规范 |
| 前置文档 | 本文默认 01~06 号文档已定稿；六期行语义总表（各期 §3.1）是本文映射层的唯一口径来源；06 号文档 §10 已预告"60/61/62 行语义均已定稿，RDF 三元组映射的行级契约齐备" |
| 全仓约束 | 铁律 3/4/6 与 01 号文档 §8 转译差异记录对本文全量生效；本期零 DDL、零菜单种子增量 |
| 实现状态 | **设计定稿，未实现** |

---

## 1. 背景与范围

### 1.1 业务背景与产品定位

本体建模模块已落地六项功能，四张 `ont_*` 表 + `ont_serials` 为唯一事实源。当前本体数据的**进出通道只有管理界面**，造成三个现实问题：

1. **数据孤岛**：建模成果无法离开本系统——不能在 Protégé 中打开、不能被外部推理机/三元组库消费、无法作为文档归档或交付；
2. **无批量入口**：外部已有 OWL 文件（历史建模成果、上级下发的词表、Protégé 建的模型）时，只能逐条手工重建——六期文档中反复出现的"批量导入 → 后续迭代"（02/03/05 §1.4）是同一需求的分散表述；
3. **无备份/迁移载体**：数据库之外没有第二份可读副本。

07 期给本体文档页补上**导出 / 导入**两个动作（源评审已定：入口在本体文档页，不新建功能菜单——导入导出不是一类实体，没有管理面）。

### 1.2 07 期在系列中的定位

与 01~06 期"新增一类实体"的增量不同，07 期是**横切期**：

- 零新实体类型、零新公理类型、零 DDL——全部工作在"既有公理行 ↔ RDF 三元组 ↔ OWL 文件"的双向映射上；
- **本仓特有重心**：源项目用 OWLAPI 免费获得"RDF 语法解析 → OWL 公理 → 序列化"全链；本仓无 OWLAPI（01 号文档决策五），**必须自建 RDF 读写层与三元组映射层**——这是全系列最大的单期新增工作量，也是本文的设计重心（§2.1/§2.2）；
- 六项功能各自 V1 的"显式不做"清单决定了导入映射的**支持子集边界**（§3.2 忽略清单）——本期不越界补齐这些能力。

### 1.3 Protégé 对标结论（源码级，实现时对照）

| Protégé（`protege-editor-owl`） | 行为 | 本系统对标 |
|---|---|---|
| `model/io/OntologySaver` | 按"本体 × 格式 × 位置"逐份保存；格式由 storer registry 决定（RDF/XML 缺省） | 导出同步执行（V1 数据量小，无进度条），格式参数化 `rdfxml\|turtle` |
| `model/io/OntologyLoader` | 后台线程加载文件 → OWLOntologyManager；解析失败带原始异常上抛 | `owlio` 解析器（§2.1），失败抛 `OWL_IMPORT_FILE_INVALID`（带解析器原始消息） |
| `UserResolvedIRIMapper` | 加载时把无法解析的 `owl:imports` 交给用户逐个定位 | **V1 不解析 imports**（§1.4 #2）：检测到即报告，依赖文档中的公理不进入 |
| `ui/action/LoadIntoCurrentModelAction` | 把文件公理**合并进当前工作区** | V2 合并导入候选（§1.4）；V1 仅"新建本体导入" |
| `XmlBaseAlgorithm` / `OntologyNameAlgorithm` | 从文件内容推断 xml:base / 本体 IRI | preview 的 `base_iri` / `ontology_name` 自动推断（§2.3，众数命名空间） |
| `ui/action/export/inferred/ExportInferredOntologyAction` | 导出**推理结果** | V2 随推理机期；V1 仅导出 asserted 公理行 |

### 1.4 范围声明

**V1 做**：

1. **导出**：单本体 → 单 OWL 文件，格式 RDF/XML（默认）与 Turtle 可选；全量公理行映射；本体 header（IRI / description / versionInfo）；只读操作（不 + 版本、不写变更日志）；
2. **导入**：**新建本体导入**（V1 唯一通道），两步式——preview（解析 + 映射 + 校验，产出报告，不落库）→ execute（按 reportId 落库）；
3. **映射层**：六项功能全部已建模公理类型的双向映射（§3.2 总表，三元组级）；
4. **宽容策略**：未声明实体自动补声明（§2.5）、不支持公理进忽略清单（用户确认后继续）、62 字面量 datatype 偏差告警（§2.6）；
5. **号池回填**：导入后按存量编码回填 `ont_serials`，保证 06 期自动赋码不重号（§2.7）；
6. 入口与权限：本体文档页操作列（导出）+ 工具栏（导入），权限码 `ontology:ontology:export` / `ontology:ontology:import`（权限点管理页登记，01 号文档 §3.8 可选增强机制——本期**无菜单种子增量**，01 号文档预留的菜单号 118 闲置不用）。

**V1 显式不做**（决策记录，防止范围回潮；与源同款）：

| # | 不做项 | 理由与出路 |
|---|---|---|
| 1 | 合并导入（公理并入既有本体） | 需 IRI 冲突消解向导 + 与六期 frame 全量提交语义对齐的合并事务，复杂度独立成期；V2 候选 |
| 2 | `owl:imports` 依赖解析 | 远程文档拉取不可控（网络/安全）；V1 检测到 imports 即报告"依赖文档不解析"，被引用实体按自动补声明兜底。V2 可选本地文件映射 |
| 3 | Manchester / OWL Functional Syntax / OBO / JSON-LD 格式 | V1 只保 RDF/XML + Turtle 两个最通用格式；`owlio` 的统一三元组中间层使扩格式 = 加一对 parser/writer |
| 4 | 公理级注解（axiom annotations）还原 | `ont_axioms` 行模型无公理级注解存储（04 号文档 §1.4 已声明"届时零迁移"）——带注解公理**保留公理主体、丢弃注解** + warning；V2 随公理级注解能力一并评估 |
| 5 | IRI 型注解值 / 匿名个体 | 04 号文档 §1.4 已声明（需 `value_kind` 判别列）；导入遇 IRI 对象注解值 → 忽略清单 |
| 6 | 类表达式公理（unionOf / intersectionOf / someValuesFrom / hasValue…） | 01 号文档 §4.1 预留表达式基础设施未实现；含表达式公理 → 忽略清单。这是导入丢真最大的单项，预检报告必须醒目展示计数 |
| 7 | sameAs / differentFrom / 负属性断言 / HasKey / SWRL | 06 号文档 §1.4 #1/#4 定案不做；公理号段 63/64 未开——全部忽略清单 |
| 8 | ZIP 多本体打包导出 | 本体间无 imports 关系，单本体单文件已满足 V1；批量导出走行操作逐个下载 |
| 9 | 个体 Excel 批量导入 | 06 号文档 §1.4 #3 的 V2 项，与 OWL 导入是两条独立通道（Excel 面向业务档案、OWL 面向语义互操作），不合并 |

---

## 2. 关键架构决策

### 2.1 决策一（本仓核心定案）：自建 `internal/ontology/owlio` RDF 读写层，零第三方依赖

01 号文档决策五预留的定案义务在此兑现。候选方案对比：

| | 方案 A：`knakk/rdf` 等现成库 | **方案 B：自写 `owlio` 包（采纳）** |
|---|---|---|
| Turtle 解析/序列化 | 成熟 | 自写（实用子集，见下） |
| **RDF/XML 解析/序列化** | **不支持**（仅 Turtle/N-Triples）——OWL 生态最通用格式（Protégé 默认）缺失 | 自写（Go `encoding/xml`，规整形态） |
| 依赖 | 引第三方（覆盖面仍不足） | 零第三方 |
| 工作量 | 仍需自写 RDF/XML 半边 | 全自写，但导出侧 writer 与解析侧 parser 各自规整 |
| 风险 | 库维护状态一般 | 解析器边界用例需充分单测（Turtle 语法较宽） |

**定案：方案 B。** `internal/ontology/owlio` 包，统一三元组中间表示：

```go
// owlio/triple.go —— 统一中间表示（两格式共用）
type Term struct {          // RDF 项：IRI / 字面量 / 空白节点
    Kind    TermKind        // TermIRI / TermLiteral / TermBlank
    Value   string          // IRI 全文 / 字面量词形 / 空白节点标签
    Datatype string         // 字面量 datatype IRI（空 = plain/rdf:langString）
    Lang    string          // 语言标签（可空）
}
type Triple struct{ Subject, Predicate Term; Object Term }
```

四个读写器（解析器按内容探测分流：首字符 `<` / `<?xml` → RDF/XML，否则按 Turtle 解析）：

| 文件 | 组件 | 覆盖子集（对齐 Protégé 导出形态） |
|---|---|---|
| Turtle 解析 | `turtle_parser.go`（词法 + 递归下降） | `@prefix`/`@base`/大小写两式、`a`、`;`/`,` 缩写、IRI（`<>` 与前缀名）、字面量四形态（单/三引号、`@lang`、`^^type`、数字/布尔缩写）、空白节点（`[]`、`_:b`）、**集合 `(...)`**（37 链 / n-ary / withRestrictions 需要）、注释 `#` |
| Turtle 序列化 | `turtle_writer.go` | 规整输出：`@prefix owl/rdfs/xsd/skos/rdf` 声明 + 按主语分组 + 谓语缄约（`;`）——只求语义正确可再解析 |
| RDF/XML 解析 | `rdfxml_parser.go` | `encoding/xml` 递归：`rdf:Description` / 类型化节点元素、`rdf:about`/`rdf:resource`/`rdf:nodeID`/`rdf:datatype`/`xml:lang`/`rdf:parseType="Collection"`、嵌套属性元素 |
| RDF/XML 序列化 | `rdfxml_writer.go` | 规整平铺形态：每实体一个节点元素、每三元组一个属性元素（子元素或 `rdf:resource` 属性）、空白节点 `rdf:nodeID`；不追求 striped 美观 |

**性能边界**（V1 声明，超限拒绝并提示拆分，与源同款）：文件 ≤ 5 MB、实体 ≤ 10 000、公理 ≤ 20 000。本仓定位（POC 级建模）远低于此。

### 2.2 决策二：导出 = "行 → 三元组"映射与各期行语义同源（源 project() 上提的本仓对等物）

源项目把 `OntAxiomCompiler.project()`（公理行 → 内存 OWLOntology 的全量投影）上提为导出/校验共用入口，保证"能编译的本体必然导得出、导出的文件必然导得回"。本仓无 OWLOntology 内存模型，对等物是**单一映射层**：

```
internal/ontology/owlio/mapper.go
  ├─ RowsToTriples(ontology, entities, axioms) []Triple     // 导出方向：按各期行语义表（01~06 §3.1）产三元组
  └─ TriplesToPlan(triples) (*ImportPlan, error)            // 导入方向：§3.2 总表反向映射
```

**同源纪律**：`RowsToTriples` 的每个 axiom_type 分支与校验器（`validator.go`）的对应规则、`types.go` 的号段常量**引用同一份定义**；导入预检的"零编译失败不变式"（§2.6）直接复用 `lexical.go` / `datatype.go` / 白名单常量做逐行预校验——导出、校验、导入三方向共享行语义，杜绝"两张皮"。

header 注解由导出侧在三元组层追加（不属于公理范畴）：`rdfs:comment` = 本体 description（非空时）、`owl:versionInfo` = 版本号字符串（导入侧不识别、不落库，进忽略清单报告——round-trip 时报 known-diff）。

### 2.3 决策三：导入 = "新建本体 + 三元组反向映射"，IRI 保真、baseIri 众数推断

V1 导入的唯一形态是**新建一个本体文档**并灌入文件内容。三个定案（与源同款，含其修订 #2）：

1. **IRI 保真**：`ont_entities.iri` 本就存完整 IRI（不要求 = base_iri + local_name），导入的实体 IRI 原样落库。`base_iri` 只服务"今后在本系统新建实体时的 IRI 生成"，不约束导入内容——文件里实体 IRI 是任意命名空间都能导进来，不需要"改写外部 IRI"这类有损操作；
2. **base_iri / 本体名推断**（preview 推断默认值，三者均可在执行前编辑，校验沿用本体创建规则）：
   - `ontology_iri` = 文件 `owl:Ontology` 主体的 IRI（有则用）；
   - `base_iri` = **实体 IRI 命名空间的众数**（按计数取最长，并列取更长）——源实现修订 #2 定案：最长公共前缀会被单个跨命名空间实体（如 RDF/XML 写出器为 skos:notation 补的 Declaration）拉塌到 `http://`，众数对离群实体稳健；无实体时回退 `https://{域名}/ontology/{ontologyName 拼音或 slug}#`；
   - `ontology_name` 默认 = ontology_iri 尾段，缺省 = 文件名去扩展名；
3. **localName 提取**：取 IRI 最后一个 `#` 后段，无 `#` 取最后一个 `/` 后段；命中 `LOCAL_NAME_PATTERN`（`^[\p{L}_][\p{L}\p{N}_-]*$`）则用之；**提取失败或不合法 → 自动替补 `e1`、`e2`…（按文件出现顺序），IRI 不变，报告 warning 列出映射**。localName 只是展示辅助（displayLabel 优先），身份载体是 IRI，不值得为它阻断导入。

### 2.4 决策四：导入不走 `ChangeApplier` —— 批量直插 + 一次全量校验 + 单条摘要日志

`ChangeApplier.Apply` 的语义是"交互式单实体变更"（逐条 insert、乐观锁、diff 驱动）。导入是**一次性灌入**，对齐其语义代价不成比例，故走独立批量通道：

| 步骤 | 内容 | 复用/新增 |
|---|---|---|
| 1 | 本体行 insert（复用名称查重 + base_iri 规范化，version=0） | 本体服务同款校验逻辑（服务内联） |
| 2 | 实体行**逐条 insert**（id 回填是公理引用重写的硬依赖——源修订 #1 定案：批量模式主键回填行为不作赌注；1 万上限下性能可接受） | ent 标准 Create |
| 3 | 公理行批量 CreateBulk（分批 500；草稿序号 → 实体 id 重写） | ent CreateBulk |
| 4 | 渲染缓存同步（display_label / deprecated） | **applier 的 `syncAnnotationCache` 导出为 `SyncAnnotationCache(ctx, tx, subjectIDs)`**（原私有，行为零变化，既有单测回归） |
| 5 | 全量校验一次 | `validator.Validate(ontologyID)` |
| 6 | 号池回填（§2.7） | `ontology_serial_repo` 直插/更新池行 |
| 7 | 写一条 `ont_change_logs`（operation=`IMPORT_OWL`，detail=摘要 JSON：来源格式/实体数/公理数/忽略数/补声明数），version 0→1 | 直插（不走 Apply 的明细 diff——几万行明细塞 JSON 无意义） |

**为什么 execute 必然能通过 validate**：preview 阶段已执行**逐行预校验**（"导入零校验失败不变式"，§2.6）——凡是落行后可能令校验器拒绝的公理（词形不符、值域白名单外、facet 非法、自相矛盾引用）在 preview 就进忽略清单而不落行。execute 时的 validate 是**防御性断言**（理论上必过，过不了说明 plan 与库状态间有并发写，整体回滚报错重试）。

事务边界：execute 单个 ent Tx（本体 + 实体 + 公理 + 号池 + 日志原子）；validate 失败即全量回滚，不留半截本体。

### 2.5 决策五：宽容策略 —— 未声明实体自动补声明（Protégé 同款），报告强制展示

OWL 文件允许"用了未声明"（property assertion 的谓词、domain/range 的宾语经常没有显式 Declaration——Protégé 打开这些文件照常工作）。本仓行模型**要求**每个公理两端实体有 `ont_entities` 行（校验器 require 规则必拒），故：

**凡公理引用了未注册的 IRI，按公理位置的类型上下文自动补 `DECLARATION(10)` 行 + 实体行，并计入报告"自动补声明"清单**（源修订 #6：映射后处理**为每个存活实体补 DECLARATION 行**——行模型事实源要求每实体恰一条声明，同时使文件写出器补的 Declaration round-trip 对等）。

类型上下文推断表（公理位置 → 补声明实体类型）：

| 公理位置 | 补声明类型 |
|---|---|
| 20/21/22 两端、31/32 宾语、60 宾语 | CLASS(10) |
| 30/34/35/36/37 主语与链成员、61 predicate_iri | OBJECT_PROPERTY(20) |
| 40/44/45 两端、62 object_id（谓词） | DATA_PROPERTY(30) |
| 50 两端、100 谓词（object_id 轨） | ANNOTATION_PROPERTY(40) |
| 46 主语、42 派生轨宾语 | DATATYPE(50) |
| 60/61/62 主语 | INDIVIDUAL(60) |

例外与边界：① **内建谓词不补**：`rdfs:label` / `rdfs:comment` / `owl:deprecated` / `skos:notation` 走 100 行内建谓词轨，不注册实体（04 号文档决策三"内建谓词不入库"）；② **内建 datatype 不补**：IRI ∈ `BuiltinDatatypes` 白名单 → 跳过不建实体（05 号文档口径）；③ **同 IRI 类型冲突**（同一 IRI 既被声明为类又被当数据属性引用）：文件自身不一致，preview 报 **error 阻断**（`OWL_IMPORT_TYPE_CONFLICT`，不属于宽容范围）；④ **owl:imports 目标里的实体**：统一按"出现即补声明"——报告单列"因 imports 未解析而补声明的引用"计数。

自动补声明与 06 期"编码属性懒创建"是同一机制的泛化，语义上等价于 Protégé "出现在文件中即存在"的开放世界行为。

### 2.6 决策六：n-ary 展开、成对存储、词法偏差告警与"零校验失败不变式"

1. **n-ary 展开**：`owl:AllDisjointClasses(A,B,C)` / `owl:AllDisjointProperties` / n-ary `owl:equivalentClass` 链（RDF 集合形态，解析侧展开 Collection）→ **全部两两有序对**展开落行（(A,B)+(B,A)+…），对齐既有"对称关系成对存储"惯例。展开只发生在导入方向；**导出方向逐对输出**（`C owl:disjointWith D` 等，语义等价、实现最简，不聚合回 n-ary）；
2. **成对公理**（InverseOf / Equivalent(Obj/Data)Properties / Disjoint(Obj/Data)Properties）：按 (A,B)+(B,A) 双行落库（与 02/03 期服务层写法一致）；
3. **62 字面量 datatype 偏差**：typed literal 的 datatype ≠ 按首条 42 行推断的 datatype（05 号文档三段分派）时——**按推断 datatype 落行**（62 行模型无 datatype 列，值文本保真），warning 报告列出（"字面量声明 xsd:int，按值域 xsd:integer 落行"）。语义等价或子超类关系场景无损；真正冲突场景进入下一条的预校验兜底；
4. **导入零校验失败不变式**：preview 对每条待落行公理做**预校验**（与校验器同口径：`lexical.go` 词形、白名单、引用存在性、`datatype.go` facet 合法性）。任何一条过不了 → 该行进**忽略清单**（带原因），不落行。忽略清单在执行前强制展示，用户勾选确认（`confirmed_ignore=true`）方可执行——与 06 期 `force=true` 同款"两段式确认"语义；
5. **62 lang 轨豁免**（源修订 #7）：带语言标签的字面量不受值域词法闸门约束（与校验器"语言文本按 rdf:langString 处理"同口径），不进忽略清单；
6. **owl:deprecated 值归一**：literal "true"/"false"（含 `^^xsd:boolean`）→ 落行 `value_text`；非布尔词形 → 忽略清单（渲染缓存同步依赖布尔语义）。导出侧特判输出 typed `^^xsd:boolean`（源修订 #5：plain literal 对外部工具语义不正确）。

### 2.7 决策七：导入后号池回填 —— 06 期赋码体系的闭环义务

06 期自动赋码 = 类 skos:notation 前缀 + `ont_serials` 流水。若导入文件里已有编码个体（62 断言值 `A0111-0001..0047`）而号池未回填，下一次 AUTO 赋码会从 0001 重新发号 → 撞 `INDIVIDUAL_CODE_DUPLICATE`——**导入破坏了取号器的连续性假设，必须在本期闭环**：

- 导入完成后（同一事务），按"编码前缀"聚合扫描全部 62 断言值：匹配 `^{prefix}-(\d+)$`（prefix = 各主类目 skos:notation 值；前缀含 `-` 时以**最后一次 `-` 分割**取流水段，流水段必须全数字——与 06 期编码形态自洽）→ 取最大流水 N → 直插/更新号池行 `next_no = N + 1`（purpose=`code`）；
- 不匹配该模式的编码（手工自由编码）不参与回填——号池仍从 1 起步，与手工编码撞号时由 06 期既有查重兜底（`INDIVIDUAL_CODE_DUPLICATE`），报告 warning 提示；
- localName 流水池（purpose=`ind`，伪前缀 `$ind`）同理回填：扫描导入个体 localName 匹配 `^i(\d{6,})$` 取最大 +1（06 期 6 位自然扩位口径）；
- 回填幂等：池行存在则 UPDATE，不存在则 INSERT（无并发对手方——导入事务持有该本体全部行的写权）。

### 2.8 推理影响声明

与 01~06 期同款：V1 无推理机、不做一致性检查。导入不做**语义**校验（矛盾公理如 `A ⊑ ¬A`、不相交类同时被断型——原样落行，结构校验可过，矛盾留给推理期），只做**结构**校验（可落行性）。导出同理：导出的是 asserted 公理，不是推理闭包。

---

## 3. 数据模型

### 3.1 本期零 DDL、零种子增量

五表（四表 + `ont_serials`）完全复用，无新列、无新表、无新索引；菜单零增量（导入导出是本体文档页动作，非独立页面；按钮权限码走权限点管理页登记）。01 号文档预留的菜单号 118 闲置不用。

### 3.2 三元组 ↔ 公理行映射总表（双向，本文核心）

导出方向 = `RowsToTriples`（按各期行语义产三元组）；下表以 **Turtle 记法**给出导入方向的匹配模式（`TriplesToPlan`）。行语义列引用各期设计文档定稿。

| RDF 三元组模式（Turtle 记法） | axiom_type | 导入落行规则 |
|---|---|---|
| `S rdf:type owl:Class / owl:ObjectProperty / owl:DataProperty / owl:AnnotationProperty / rdfs:Datatype / owl:NamedIndividual` | 10 | 建实体行（类型按声明种类）；内建 datatype 白名单内跳过（§2.5 #2） |
| `A rdfs:subClassOf B`（两端均为**命名**类） | 20 | subject=A, object_id=B |
| `A owl:disjointWith B`；`[a owl:AllDisjointClasses; owl:members (A B …)]` | 21 | 两两有序对展开（§2.6 #1） |
| `A owl:equivalentClass B`（两端命名类） | 22 | 两两展开 |
| `P rdfs:subPropertyOf Q`（P/Q 按已定类型分流） | 30 / 40 / 50 | subject=子, object_id=父 |
| `P rdfs:domain C` | 31 / 41（按 P 类型） | object_id=类 |
| `P rdfs:range C`（C 命名类）/ `P rdfs:range xsd:integer`（白名单内）/ `P rdfs:range DT`（datatype 实体） | 32 / 42（内建轨 value_text / 派生轨 object_id） | 白名单外 IRI → 忽略清单（"值域 IRI 不在白名单"） |
| `P rdf:type owl:FunctionalProperty / owl:SymmetricProperty / …（七特征之一）`（按 P 类型分流） | 33 / 43 | predicate_iri = 特征 IRI |
| `P owl:inverseOf Q` | 34 | (P,Q)+(Q,P) 双行 |
| `P owl:equivalentProperty Q` | 35 / 44 | 双行 |
| `P owl:propertyDisjointWith Q`；`[a owl:AllDisjointProperties; owl:members (…)]` | 36 / 45 | 双行 / 两两展开 |
| `P owl:propertyChainAxiom ( Q1 Q2 … )`（RDF 集合） | 37 | subject=超属性, value_text=链成员**实体 id JSON 数组**（保序；先实体后公理，序在 IRI→id 映射后写） |
| `[a rdfs:Datatype; owl:onDatatype xsd:integer; owl:withRestrictions ( [xsd:minInclusive "0"] … )]`，`D owl:equivalentClass 该节点` | 46 | subject=DT 实体，value_text=定义 JSON（`NormalizeDefinition` 落库形态）；base 白名单外 / facet 名非法 / facet 值不过校验 → 忽略该行（datatype 实体保留、无定义 = 放行轨）；`DataOneOf` / union / intersection 形态 → 忽略清单 |
| `I rdf:type C`（C 命名类） | 60 | subject=个体, object_id=C；sort_order = 文件内该个体第 i 条类型断言（0 起），**首位即主类目**（06 号文档 §2.4；报告提示） |
| `S P O`（P 为已声明/补声明对象属性，S/O 个体） | 61 | subject=S, predicate_iri=P 的 IRI, object_id=O |
| `S P "lit"`（P 为数据属性） | 62 | 带语言标签 → lang 轨；否则按首条 42 行推断 datatype 落行（偏差告警 §2.6 #3）；**词形预校验失败 → 忽略清单**（lang 轨豁免，§2.6 #5） |
| `S rdfs:label "x"@zh` / `S rdfs:comment "…"` | 100 | 内建谓词轨：predicate_iri=谓词, value_text=词形, lang=字面量语言标签 |
| `S skos:notation "A0111"` | 100 | 内建谓词轨，**lang 强制空**（06 号文档口径；带 lang 的 notation → 按 warning 剥离 lang 落行） |
| `S owl:deprecated "true"^^xsd:boolean` | 100 | 内建谓词轨；非布尔词形 → 忽略清单（§2.6 #6） |
| `S AP "v"`（AP 为自定义注解属性实体） | 100 | object_id 轨（谓词 = 已声明/补声明的 AP 实体）；IRI 对象值 → 忽略清单（§1.4 #5） |
| **本体 header**（`<ontIRI> a owl:Ontology; rdfs:comment "…"`） | — | ontology IRI → `ont_ontologies.ontology_iri`；header `rdfs:comment` → `description`；其余 header 注解（versionInfo / label / 自定义）→ 忽略清单 |
| **忽略清单全集**（进报告，确认后跳过） | — | 类表达式公理（`rdfs:subClassOf`/`owl:equivalentClass` 宾语为 restriction 节点——`owl:someValuesFrom`/`allValuesFrom`/`unionOf`/`intersectionOf`/`hasValue`/`cardinality`…）、SameIndividual / DifferentIndividuals、NegativeObject/DataPropertyAssertion、HasKey、SWRL 规则、匿名个体相关公理、公理级注解（`owl:annotated*` 重写声明形态——识别即整组丢弃 + warning）、`owl:imports` 三元组（不解析，报告提示）、IRI 型注解值、非布尔 deprecated、白名单外值域/基类型/facet |

**表达式识别规则**（忽略判定的实现口径）：宾语 Term 是**空白节点**且该节点的类型/属性命中表达式词汇表（`owl:Restriction`、`owl:unionOf`、`owl:intersectionOf`、`owl:complementOf`、`owl:oneOf`、`owl:onProperty` 等）→ 该三元组及其关联子图进忽略清单；唯一例外是 §3.2 中 46 定义的 datatype restriction 形态。

### 3.3 错误码增量（80~88 号段，01 号文档预留段恰 9 槽）

```proto
// ===== OWL 导入导出（源 1-071-008 段，9 码含源修订 #8 的 EXPORT_FAILED）=====
OWL_IMPORT_FILE_INVALID   = 80 [(errors.code) = 422]; // OWL 文件解析失败：{}（带解析器原始消息，截断 500 字符）
OWL_IMPORT_FILE_TOO_LARGE = 81 [(errors.code) = 422]; // 文件超过导入上限（5MB），请拆分后导入
OWL_IMPORT_REPORT_EXPIRED = 82 [(errors.code) = 409]; // 预检报告已过期，请重新上传文件
OWL_IMPORT_NOT_CONFIRMED  = 83 [(errors.code) = 422]; // 存在将被忽略的公理，请确认忽略清单后重试
OWL_IMPORT_EMPTY          = 84 [(errors.code) = 422]; // 文件中未发现可导入的实体
OWL_IMPORT_TYPE_CONFLICT  = 85 [(errors.code) = 422]; // 文件中实体 {} 的类型声明冲突，无法导入
OWL_IMPORT_LIMIT_EXCEEDED = 86 [(errors.code) = 422]; // 文件内容超过导入上限（实体 {} / 公理 {}）
OWL_EXPORT_FORMAT_INVALID = 87 [(errors.code) = 422]; // 不支持的导出格式：{}
OWL_EXPORT_FAILED         = 88 [(errors.code) = 500]; // 导出序列化失败：{}（writer 兜底）
```

### 3.4 检索与索引

导出全量读取走既有索引（实体 `idx_ont_entities_ontology_type`、公理按 subject 前缀 `idx_ont_axioms_subject`）；号池回填扫描 62 行走 `idx_ont_axioms_object`（编码属性）+ 内存过滤 value_text（1 万上限可控）。零新增。

---

## 4. 服务与流程详细设计

### 4.1 包结构与复用清单

```
internal/ontology/owlio/                # 本期新增（纯函数包，完全可单测）
├── triple.go            # Term / Triple 中间表示
├── turtle_parser.go     # Turtle 解析（§2.1 子集）
├── turtle_writer.go     # Turtle 序列化
├── rdfxml_parser.go     # RDF/XML 解析
├── rdfxml_writer.go     # RDF/XML 序列化
├── detect.go            # 格式探测（按内容首字符）
├── mapper.go            # RowsToTriples / TriplesToPlan（§2.2/§3.2 映射总表）
├── plan.go              # ImportPlan（草稿实体行 + 草稿公理行[序号引用]）与 ImportReport 模型
├── planstore.go         # ImportPlanStore 接口 + 内存 TTL 实现（30 分钟；多副本部署换 Redis 实现，§12 #4）
└── *_test.go            # 解析器/序列化器/映射器单测（round-trip 闭环不依赖 DB）

internal/service/
├── owl_interop_service.go              # 新增：export / preview / execute 编排（BFF）
└── ontology_service.go                 # 增量：无（导出导入是独立服务，本体页仅前端入口）

internal/ontology/
├── applier.go           # 改造：syncAnnotationCache → 导出 SyncAnnotationCache（行为零变化，回归守护）
└── types.go / lexical.go / datatype.go # 复用：号段常量、词法校验、facet 校验（预校验与导出同源）
```

| 复用件 | 来源 | 用途 |
|---|---|---|
| `lexical.go` / `datatype.go` / 白名单常量 | 03/05 号文档 | 62 词形预校验；46 定义 JSON 双向；零校验失败不变式 |
| `EntityFactory.LOCAL_NAME_PATTERN` / base_iri 规范化 | 01 号文档 | localName 提取校验、本体行组装（导入批量组装不经工厂逐条查库，静态引用同款常量） |
| 号池 purpose 常量与编码形态（`FormatCode` 口径） | 06 号文档 | 号池回填（§2.7） |
| `ChangeApplier.SyncAnnotationCache` | 01 号文档（本期导出） | 导入后渲染缓存同步 |
| 公理行草稿结构 | `changeset.go` | 导入行组装复用同款结构与缺省（sort_order 等） |

### 4.2 导出管线（`owl_interop_service.go` 的 Export）

```
export(ctx, ontologyID, format) : (fileName string, content []byte)
  1. 本体存在性校验（ONTOLOGY_NOT_FOUND）；format ∈ {rdfxml, turtle} → OWL_EXPORT_FORMAT_INVALID
  2. 全量读取实体行 + 公理行（单条 SELECT 全量快照，无中间态——导出不做乐观锁）
  3. triples := owlio.RowsToTriples(ontology, entities, axioms)     // §2.2 映射层
     + header 三元组（rdfs:comment / owl:versionInfo）
  4. content := writer(triples)   // rdfxml → rdfxml_writer / turtle → turtle_writer；失败 → OWL_EXPORT_FAILED
  5. 文件名 {ontologyName}_v{version}.owl / .ttl
```

- 空本体（无实体无公理）合法导出：文件仅含 header——外部工具可正常打开；
- **导出为只读操作**：不 +version、不写变更日志、不受乐观锁约束；文件以导出时的 version 标注；
- 导出细节定案：46 行 facet 值按**基类型** typed literal 输出（源修订 #3：数值比较 facet 不再一律 xsd:decimal，避免 round-trip datatype 漂移）；`owl:deprecated` 输出 `^^xsd:boolean`（§2.6 #6）；62 行按首条 42 值域产 typed literal（03 号文档口径）。

### 4.3 导入预检管线（Preview）

```
preview(ctx, file) : ImportReport
  1. 体积校验（5MB → OWL_IMPORT_FILE_TOO_LARGE）
  2. detect 格式（§2.1）→ parser 解析为三元组集；解析失败 → OWL_IMPORT_FILE_INVALID（带解析器消息）
     owl:imports 三元组不解析（§1.4 #2），报告提示"检测到 N 条 imports，依赖文档未解析"
  3. TriplesToPlan（§3.2 总表）：
     ├─ 实体注册：Declaration 定类型；引用型出现按 §2.5 类型上下文补声明（含存活实体补 10 行）
     ├─ 同 IRI 多类型冲突 → OWL_IMPORT_TYPE_CONFLICT（阻断）
     ├─ localName 提取 / 替补（§2.3 #3）
     ├─ n-ary / 成对展开（§2.6）；表达式公理识别 → 忽略清单
     ├─ 每条待落行过预校验（词形/白名单/facet/引用，lang 轨豁免）→ 失败进忽略清单
     └─ 实体数 > 10000 / 支持公理数 > 20000 → OWL_IMPORT_LIMIT_EXCEEDED
  4. 推断 ontology_name / base_iri（众数命名空间，§2.3 #2）；零实体 → OWL_IMPORT_EMPTY
  5. 组装 ImportPlan → ImportPlanStore 存（TTL 30 分钟），返回 reportId
```

`ImportReport`（前端展示契约，proto 消息对齐）：

```jsonc
{
  "reportId": "…",
  "sourceFormat": "RDF/XML",          // 探测结果
  "suggested": { "ontologyName": "…", "baseIri": "…", "description": "…" },
  "stats": {                          // 六类实体 × axiom_type 统计
    "entities": { "CLASS": 12, "OBJECT_PROPERTY": 5, "…": 0, "INDIVIDUAL": 340 },
    "axioms":    { "20": 11, "42": 8, "60": 351, "61": 76, "62": 480, "100": 210 }
  },
  "autoDeclared":  [ { "iri": "…", "entityType": "OBJECT_PROPERTY", "reason": "61 断言谓词未声明" } ],  // 明细上限 200
  "renamedLocalNames": [ { "iri": "…", "localName": "e1", "original": "123" } ],
  "warnings": [ "字面量声明 xsd:int，按值域 xsd:integer 落行（个体 …）" ],
  "ignored": { "count": 17, "byReason": { "类表达式公理": 12, "词形与值域不符": 3, "公理级注解丢弃": 2 }, "samples": [ … ] }
}
```

**忽略清单确认纪律**：`ignored.count > 0` 时 execute 必须携带 `confirmed_ignore=true`，否则 `OWL_IMPORT_NOT_CONFIRMED`——两段式确认，把数据丢失的决定权显式交给用户。

### 4.4 导入执行管线（Execute）

```
execute(ctx, req{reportId, ontologyName, baseIri, description, confirmedIgnore}) : {ontologyId}
  单个 ent Tx：
  1. plan = PlanStore.get(reportId)      // TTL 过期 → OWL_IMPORT_REPORT_EXPIRED
     confirmedIgnore 校验（§4.3）→ 通过后才 PlanStore.remove(reportId)
     （get/remove 两段式——源修订 #4：pop 在确认校验前消耗计划会让用户补勾后无法重试；
       重复提交的幂等保护语义不变：remove 后二次提交报 REPORT_EXPIRED）
  2. 名称查重 / base_iri 规范化（复用本体创建校验）；本体行 insert（version=0）
  3. 实体行逐条 insert（id 回填，§2.4）
  4. 公理行重写（草稿序号 → 实体 id）+ CreateBulk 分批 500
  5. SyncAnnotationCache(全部实体 id)
  6. validator.Validate(ontologyId)      // 防御性断言，失败 = 并发写冲突，整体回滚
  7. 号池回填（§2.7）
  8. ont_change_logs 一条：operation="IMPORT_OWL"，detail=摘要 JSON，before_version=0，after_version=1；version=1
  9. 返回 ontologyId（前端刷新本体列表并切换活动本体）
```

**plan 暂存选型（§12 #4）**：内存 TTL 实现（30 分钟）+ `ImportPlanStore` 接口——本仓当前为单体服务单副本部署形态，内存实现零新外部依赖且本地/演示环境不强制 Redis；多副本部署时以 Redis 实现替换接口（TTL 语义同款）。单测直接注入内存实现。

### 4.5 号池回填细节

```
backfillSerialPools(tx, ontologyID):
  1. code 池：取本本体全部 62 断言（object_id=编码属性实体）+ 各类 skos:notation 值 →
     对每个前缀 p（含 '-' 时以最后一次 '-' 分割取流水段，流水段必须全数字）：
     收集匹配 ^p-(\d+)$ 的流水取 max N → upsert 号池(ontologyId, code, p, next_no=N+1)
     （无任何匹配前缀不建池；前缀-流水拼接宽度口径由 06 期 FormatCode 同源常量保证）
  2. ind 池：导入个体 localName 匹配 ^i(\d{6,})$ 取 max → upsert(ontologyId, ind, $ind, next_no=max+1)，
     无匹配不建池（首建走 SerialAllocator 既有路径）
```

---

## 5. API 契约（`/admin/v1/ontology/owl:*`）

源领域 proto `backend/api/protos/ontology/service/v1/ontology_owl_io.proto` + 门面 `i_ontology_owl_io.proto`。三个自定义 RPC 全部 `body: "*"`：

| 方法 | 路径 | rpc | 入参 → 出参 |
|---|---|---|---|
| POST | `/admin/v1/ontology/owl:export` | `Export` | `{ontology_id, format}` → `google.api.HttpBody`（octet-stream + Content-Disposition RFC 5987，中文本体名） |
| POST | `/admin/v1/ontology/owl:import-preview` | `ImportPreview` | `{file_name, file_content: bytes}`（multipart，绑定形态参照 `i_ai_knowledge.proto` 的 docs/file 端点先例）→ `ImportReport` |
| POST | `/admin/v1/ontology/owl:import-execute` | `ImportExecute` | `{report_id, ontology_name, base_iri, description?, confirmed_ignore}` → `{ontology_id}` |

- **导出响应载体定案**：`google.api.HttpBody`（Kratos v2 原生二进制响应，随 proto 进 openapi 生成与 Api 表同步管线——租户闸门按 (path, method) 匹配，这是不走手动路由的理由）；实现期若发现 HttpBody 与 openapi 生成不兼容，回退手动 `srv.Route` 注册并在 `make openapi` 产物中**手工补录**该路由（接口同步纪律：手动路由不进 proto 管线，须人工确保进 Api 表，否则 fail-closed 403）；
- 导出不用 GET：走 POST + blob 下载（规避 querystring 中文与鉴权令牌传递问题）；
- preview 不落库、不占乐观锁；execute 是唯一写路径，幂等保护 = reportId 一次性消费（get/remove 两段，§4.4）；
- VO 校验：`ontology_name` 必填 ≤64、`base_iri` 必填（规范化补 `#`）、`description` 可选 ≤255——对齐本体创建规则；
- 权限码：`ontology:ontology:export` / `ontology:ontology:import`（权限点管理页登记；服务 tag `OntologyOwlIOService` 进 `module_mapping.go`，挂 SYSTEM）。

---

## 6. 前端页面设计（本体文档页增量，无新路由页面）

### 6.1 导出入口（操作列）

`pages/app/ontology/ontology/index.tsx` 操作列追加"导出"下拉按钮（`Dropdown`：`导出 RDF/XML (.owl)` / `导出 Turtle (.ttl)`）：

- 点击调 `useExportOwl`（`apiClient.ontologyOwlIOService.Export({ontologyId, format}, {responseType: 'blob'})`）→ 前端从响应头取文件名落盘（`URL.createObjectURL` + `<a download>`）；
- `AccessControl codes={['ontology:ontology:export']}` 控制显隐。

### 6.2 导入向导 Drawer（`pages/app/ontology/ontology/components/OwlImportDrawer.tsx`）

工具栏"导入 OWL"按钮打开，`Steps` 四步：

1. **上传**：`Upload.Dragger`（accept `.owl,.rdf,.ttl,.xml`，单文件，beforeUpload 返回 false 手动控制）→ 直接调 preview；解析中 loading；
2. **预检报告**：
   - 顶部统计卡：六类实体数、公理总数、忽略数（忽略数 > 0 黄色警示）；
   - axioms 统计表格（axiom_type → 中文标签 → 行数）；
   - 可折叠明细区（`Collapse`）：自动补声明 / 局部名替补 / warnings / 忽略样例（超限提示"仅展示前 N 条"）；
   - 忽略数 > 0 → 底部 Checkbox「我已知晓 N 条公理将被忽略」勾选后方可下一步（`confirmedIgnore`）；
3. **基本信息**：本体名称 / 实体命名空间（默认推断值预填可改）/ 描述；
4. **执行**：调 execute → 成功提示（"导入完成：实体 X、公理 Y、忽略 Z"）→ 刷新本体列表 + `setCurrentOntologyId(新本体)`。

### 6.3 hooks（`api/hooks/ontologyOwlIO.ts`）

`useExportOwl`（blob）/ `useOwlImportPreview`（multipart：`file_name` + `file_content` 读文件为 bytes，FormData 形态对齐 ai-knowledge 上传先例）/ `useOwlImportExecute`。所有 catch `console.error` 原始错误（铁律 1）。

---

## 7. 异常与边界场景清单

| # | 场景 | 行为 |
|---|---|---|
| 1 | 文件不是 OWL（XML 但非 RDF、损坏、二进制） | 解析器抛错 → `OWL_IMPORT_FILE_INVALID` 带原始消息（截断 500 字符） |
| 2 | 文件含 `owl:imports` | 不解析依赖；报告提示"检测到 N 条 imports，依赖文档未解析"；被引用实体自动补声明兜底 |
| 3 | 文件是 OWL 2 Full（如注解实体当对象属性用） | 不拦截——只做结构校验不做 DL 判定（01~06 同款口径）；照常解析 |
| 4 | 空 ontology（只有 header 无实体） | `OWL_IMPORT_EMPTY`（导入空壳无意义，提示直接"新增本体"） |
| 5 | 实体 IRI 与既有本体撞 IRI | 不校验——导入新建本体，本体间 IRI 重复不违规（IRI 唯一性约束范围是"本体文档内"，01 号文档定案） |
| 6 | 同名本体已存在 | execute 抛 `ONTOLOGY_NAME_DUPLICATE`（复用 01 期码），前端停留第 3 步改名 |
| 7 | preview 后本体被人并发新建同名 | execute 时名称查重兜底同 #6；**plan 未被消费**（get/remove 两段），补参重跑可用 |
| 8 | reportId 过期 / 重复提交 | `OWL_IMPORT_REPORT_EXPIRED`（remove 一次性消费） |
| 9 | 62 词形与值域不符 | 忽略清单（不落行），非阻断（§2.6 #4）；lang 轨豁免（#5） |
| 10 | 46 定义 base/facet 非法 | 忽略该 46 行，datatype 实体保留（放行轨） |
| 11 | 同 IRI 类型冲突 | `OWL_IMPORT_TYPE_CONFLICT` 阻断（preview） |
| 12 | localName 提取失败（数字开头 / 空 / 特殊字符） | 替补 `e{n}` + warning（§2.3 #3） |
| 13 | 带公理级注解的公理（`owl:annotated*` 重写形态） | 识别即整组丢弃 + warning（保留主体在能力外，§1.4 #4） |
| 14 | 导出时本体被并发修改 | 单条 SELECT 全量快照映射，无中间态（导出不做乐观锁，文件标注导出时 version） |
| 15 | 编码含 `-` 的前缀回填歧义 | 以最后一次 `-` 分割（§4.5），warning 提示未匹配项 |
| 16 | 导入个体无任何类型断言 | 不可能（§2.5 类型上下文：60 行主语必为个体；纯 100 注解的 IRI 不补个体声明）——防御：preview 的 INDIVIDUAL 统计仅来自 60 行主语与 Declaration |
| 17 | 超大文件 / 超限 | 5MB / 10000 实体 / 20000 公理上限（§2.1），提示拆分 |
| 18 | Turtle 语法边界（数字缩写、三引号多行、转义、嵌套集合/空白节点） | 解析器单测全形态覆盖（§8）；未覆盖形态解析失败走 #1 报错路径，不静默丢行 |
| 19 | 导出 HttpBody 与 openapi 兼容性 | 实现期验证；不兼容则回退手动路由 + openapi 手工补录（§5） |
| 20 | 乐观锁 / 缓存漂移 / 本体停用 / 误包 `{ data }` | 同系列既有行为（导入不占乐观锁；导出只读；hooks 扁平封装） |

---

## 8. 验收标准与测试要点

**`owlio` 包单测**（纯函数，`go test`——本仓优势：round-trip 闭环不依赖 DB 与外部工具）：

- Turtle 解析：前缀/缩写/四种字面量/空白节点/集合/数字布尔缩写/注释逐形态正反例；RDF/XML 解析：类型化节点/嵌套属性元素/`rdf:parseType="Collection"`/`rdf:datatype`/`xml:lang`；
- 序列化：两种 writer 输出可被对应 parser 再解析且三元组集合相等；
- 映射器：§3.2 总表逐 axiom_type ≥1 正例；n-ary 两两展开计数；成对双行；37 链 value_text 保序；46 定义 JSON 与 `ParseDefinition` 往返；内建谓词轨 / object_id 轨分流；skos:notation 剥 lang；62 lang 轨 / typed 轨 / 词形失败进忽略；表达式识别进忽略；类型冲突阻断；localName 替补；
- **round-trip 闭环**：构造覆盖六类实体全公理类型的样例 → RowsToTriples → writer → parser → TriplesToPlan → 行草稿与原行集合语义相等（零漂移断言）。

**服务层端到端**（本地 dev 栈）：

1. 导出（RDF/XML 与 Turtle）HTTP 200，文件名 `本体名_v版本.owl/.ttl`，Content-Disposition 中文正确；
2. 垃圾文件 preview → `OWL_IMPORT_FILE_INVALID`（带原始消息）；超 5MB 拒绝；
3. preview 报告：格式探测、六类实体与公理统计、本体名/命名空间众数推断、header 注解进忽略计数；
4. execute（`confirmed_ignore=true`）→ 新本体 version=1；`base_iri` 缺失被校验拦截**且 plan 未被消费**（补参重跑成功——get/remove 两段实证）；
5. **round-trip 闭环 ×2**：export → import → export → preview 的实体与公理统计全等（样例覆盖 42 值域 / 46 定义 / 62 断言 / 注解 / 60/61）；
6. `ont_change_logs` 两条 `IMPORT_OWL`（psql 核验）：摘要 JSON、before 0 → after 1；
7. 号池回填：导入带编码个体（`A0111-0001..0047`）→ `ont_serials` code 池 next_no=48；下一 AUTO 赋码 `A0111-0048` 不撞号；`$ind` 池同理；
8. 忽略清单确认纪律：不勾选直接 execute → `OWL_IMPORT_NOT_CONFIRMED`；
9. 按钮权限码控制：无 `ontology:ontology:export` 权限点时导出入口隐藏；
10. **待人工实测**：导出文件在 Protégé 桌面版打开、Protégé 导出文件导入本系统（owlio round-trip 已闭环，风险集中在 Turtle 语法宽边界——解析器单测以 Protégé 导出形态为语料）。

**门禁**：`make build`；`go test ./internal/ontology/...` 全绿（01~06 期回归 + owlio 新增）；`make openapi`（HttpBody 路由核验）；`pnpm run typecheck` 全绿（本期零 DDL）。

---

## 9. Protégé → 本系统映射对照表（07 期增量）

| Protégé | 源项目（Java） | 本仓（Go / React） |
|---|---|---|
| `File → Save As…`（OntologySaver，格式选择） | `OwlExportService`（project() + saveOntology） | `owl_interop_service.Export`（RowsToTriples + owlio writer） |
| `File → Open…`（OntologyLoader） | `OwlImportService.preview/execute`（OWLAPI loadOntology） | `Preview/Execute`（owlio parser + TriplesToPlan） |
| OWLAPI RDF↔公理转换（axiom parser 内置） | 免费（OWLAPI 承担） | **自建 `mapper.go` 三元组映射层**（OWL 2 RDF Mapping 子集，§3.2）——本期核心工作量 |
| `File → Open into current workspace` | V2 合并导入 | V2 |
| `UserResolvedIRIMapper`（imports 用户定位） | V1 不解析，报告提示 | 同款 |
| `XmlBaseAlgorithm` / `OntologyNameAlgorithm` | preview 推断（公共前缀→众数，修订 #2） | 众数命名空间推断（§2.3 #2） |
| OWLAPI storer/parser registry（多格式） | format 枚举 rdfxml/turtle | `owlio` 四读写器 + detect（扩格式 = 加一对读写器） |
| `ExportInferredOntologyAction` | V2 随推理机 | V2 |
| 公理注解面板 | 不支持（丢弃 + warning） | 同款 |
| OWLAPI 序列化（Rio 系） | saveOntology | 自写 writer（RDF/XML / Turtle） |

---

## 10. 对既有期文档的回填与约束

1. **01 号文档 §2.5（决策五预留）**：本期定案兑现——不引入第三方 RDF 库，自建 `owlio`；"DL/Manchester 渲染手写"维持；`expression` 列预留设计不受影响（表达式公理在导入侧进忽略清单）；
2. **02 号文档 §2.6（61 断言谓词义务）**：谓词"须命中已声明对象属性"的校验义务在导入侧由自动补声明前置满足，校验器规则不动；
3. **04 号文档 §1.4（IRI 型注解值）**：`value_kind` 列未实现前，IRI 对象注解值是导入忽略清单常驻项——开放 IRI 值能力时同步解除；
4. **05 号文档（46 定义行 / 词法链）**：`NormalizeDefinition` 落库形态成为导入侧 46 行组装的目标口径；facet 名与 `FacetWhitelist` 词形（"minInclusive"）对齐（与 XSD facet 本地名天然一致，无需映射表）；导出 facet 值按基类型 typed（源修订 #3 口径）；
5. **06 号文档（号池假设）**：号池连续性义务回填——"号池由赋码动作单调推进"扩展为"号池由赋码动作 + 导入回填单调推进"（§2.7）；Excel 批量导入通道（06 §1.4 #3）与 OWL 通道保持独立；
6. **六期"批量导入 → 后续迭代"悬置项**（02/03/05 §1.4）：本期兑现其通用部分（OWL 文件级导入）；各自的 IRI 重命名等能力仍留各自 V2，不因本期关闭；
7. **01 号文档菜单预留**：号 118 闲置不用（本期无新页面），保留在预留池。

---

## 11. 开发任务分解（07 期 checklist）

**owlio 包（核心，单测先行）**
- [ ] `triple.go` / `detect.go`（中间表示 + 格式探测）
- [ ] `turtle_parser.go`（§2.1 子集，单测以 Protégé 导出形态为语料）+ `turtle_writer.go`
- [ ] `rdfxml_parser.go` + `rdfxml_writer.go`
- [ ] `mapper.go`（§3.2 总表双向 + n-ary/成对展开 + 表达式识别 + 预校验接入）
- [ ] `plan.go` / `planstore.go`（ImportPlan/ImportReport 模型 + 内存 TTL Store）
- [ ] 单测五组（§8，含 round-trip 零漂移断言）

**服务与契约**
- [ ] 源领域 proto `ontology_owl_io.proto` + 门面 `i_ontology_owl_io.proto`（HttpBody 导出）+ `ontology_error.proto` 追加 80~88
- [ ] `applier.go`：`SyncAnnotationCache` 导出（回归守护）
- [ ] `service/owl_interop_service.go`：Export / Preview / Execute（§4.2~§4.4，含号池回填与摘要日志）
- [ ] `wiring_ent.go` 三锚点 + `rest_server.go` 注册 + `module_mapping.go` 登记 tag
- [ ] `make api` + `make openapi`（HttpBody 路由核验；不兼容则手动路由 + 补录）

**前端（react）**
- [ ] `make ts` → 验证 `apiClient.ontologyOwlIOService` getter
- [ ] `api/hooks/ontologyOwlIO.ts`（blob / multipart / execute）
- [ ] 本体文档页增量：操作列导出下拉 + 工具栏"导入 OWL" + `OwlImportDrawer`（Steps 四步）
- [ ] 权限码 `ontology:ontology:export|import` 权限点登记说明（README/运维手册）

**联调验收**
- [ ] §8 单测 + 端到端 10 项 + 门禁
- [ ] Protégé 互操作实测（人工）；文档 §12 修订回填

依赖序：owlio（triple → writers → parsers → mapper）→ applier 改造 → 服务 → 前端；**mapper 是核心风险项**（映射总表完备性 + Turtle 解析边界），单测先行。

---

## 12. 转译差异记录（07 期增量，对 01~06 号文档的补充）

01~06 号文档已记录的全局差异不重复；源 07 期 §12 十项实现修订已内化正文（#1 实体单条 insert → §2.4；#2 众数推断 → §2.3；#3 facet 按基类型 → §4.2；#4 get/remove 两段 → §4.4；#5 deprecated typed → §2.6/§4.2；#6 补 DECLARATION 行 → §2.5；#7 lang 轨豁免 → §2.6；#8 EXPORT_FAILED → §3.3；#9 OWLAPI 事实确认 → 不适用（无 OWLAPI）；#10 VxeGrid 插槽 → 不适用（react 端））。本期特有转译差异：

| # | 主题 | 源项目 | 本仓定案 | 理由 |
|---|---|---|---|---|
| 1 | **RDF 读写层（最大差异）** | OWLAPI `owlapi-distribution` 承担全部"文件 ↔ OWL 公理"转换（loadOntology / saveOntology / RDF↔公理映射内置） | 自建 `internal/ontology/owlio`：统一三元组中间表示 + 四读写器（Turtle/RDF-XML × 解析/序列化）+ `mapper.go` 三元组↔公理行映射层（OWL 2 RDF Mapping 子集）——**零第三方依赖**（评估过 `knakk/rdf`：不支持 RDF/XML，覆盖面不足且仍需自写半边） | Go 无 OWLAPI（01 号文档决策五预留的定案义务）；本期核心工作量从"编排"移到"读写层 + 映射层"，单测可完全覆盖 |
| 2 | **导出/校验同源机制** | `OntAxiomCompiler.project()` 上提（行 → OWLOntology 投影复用） | `mapper.RowsToTriples` 与校验器/号段常量/词法组件共享同一份行语义定义；预校验直接复用 `lexical.go`/`datatype.go` | 无 OWLOntology 内存模型，"单一投影入口"转译为"单一映射层 + 共享语义常量"，同源纪律等价 |
| 3 | n-ary 导出形态 | OWLAPI 集合自动去重（文件可能聚合） | 导出**逐对输出**（`owl:disjointWith` 等，语义等价）；导入仍解析 AllDisjointClasses/AllDisjointProperties 集合形态并两两展开 | 生成侧聚合无必要（文件略大无害），解析侧必须支持集合形态（Protégé 产物常见） |
| 4 | **plan 暂存** | Redis（TTL 30min，多实例一致性；H2 单测注入内存实现） | 内存 TTL 实现（30min）+ `ImportPlanStore` 接口；多副本部署时换 Redis 实现 | 本仓单体服务单副本部署形态（docker compose），零新外部依赖；接口化保留升级路径；单测天然注入内存实现 |
| 5 | 导出响应载体 | yudao Excel 同款 POST blob（`HttpServletResponse`） | `google.api.HttpBody`（Kratos 原生二进制响应，进 openapi/Api 表同步管线）；回退路径手动 `srv.Route` + openapi 手工补录 | 本仓 Api 表/租户闸门以 openapi.yaml 为源；HttpBody 让导出路由留在 proto 管线内 |
| 6 | multipart 上传形态 | Spring `MultipartFile` | proto `bytes file_content` + `file_name` 字段（绑定形态参照 ai-knowledge docs/file 先例） | Kratos 无 MultipartFile 概念，bytes 字段是其等价物 |
| 7 | 错误码段 | `1-071-008`（9 码） | proto 枚举 80~88（01 号文档预留段恰 9 槽） | 号段映射 |
| 8 | 菜单种子 | Flyway 按钮 13254~13255 | **零种子**：按钮权限码走权限点管理页登记；预留菜单号 118 闲置 | 01 号文档 §3.8 机制（本仓无按钮型菜单种子先例） |
| 9 | round-trip 测试载体 | OWLAPI loadOntology 再解析（依赖库） | `owlio` 自身 parser 再解析 + 三元组集合相等断言——纯单测闭环，不依赖外部工具 | 自建读写层的副产品优势；Protégé 实测降为人工抽检项 |
| 10 | OWLAPI 事实确认项（源修订 #9） | Rio 格式类名 / MissingImportHandlingStrategy / getOWLLiteral(int) 产出 xsd:integer 等四个库事实 | 不适用；对应 Go 事实预记：Turtle 数字缩写解析为 `xsd:integer`/`xsd:decimal`/`xsd:double` typed literal（按词形分流），RDF/XML 必须显式 `rdf:datatype` | 语言/库差异记档免复查 |

---

*本文档由源项目 youmingwanwu `docs/ontology/07-OWL导入导出详细设计.md`（V1.1，含 §12 十项实现修订）转译而来；八项通用修订内化对应章节，两项 Java/前端特有不适用，本仓特有决策集中于 §12 #1~#6（owlio 自建读写层 / 映射层同源机制 / 内存 plan 暂存 / HttpBody 导出）。至此本仓本体建模详细设计系列七份全部定稿（01~07），实施顺序即文档序号。*
