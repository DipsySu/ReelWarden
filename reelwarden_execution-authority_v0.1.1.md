# ReelWarden（影卫）项目执行权威基线

> 文档类型：Project Execution Source of Truth  
> 项目名称：ReelWarden（影卫）  
> 版本：v0.1.1  
> 状态：已冻结，可执行  
> 生效日期：2026-06-24  
> 替代版本：v0.1.0  
> 适用范围：只读 Alpha、后续安全执行器设计与发布门禁  
> 推荐仓库路径：`docs/PROJECT_EXECUTION_AUTHORITY.md`

---

# 0. 文档权威规则

本文档是 ReelWarden 当前阶段唯一的执行基线。

发生冲突时，优先级从高到低为：

1. 本文档中的安全约束、合规 Gate 与发布门槛；
2. 已合并的 ADR（Architecture Decision Record）；
3. 已合并的数据库 Migration 与 OpenAPI 定义；
4. GitHub Issue / Project Board；
5. 代码注释、聊天记录、临时讨论和 AI 建议。

聊天记录、口头约定、外部审查意见和未合并分支均不自动构成正式需求。外部审查只有在写入本文档或已合并 ADR 后才具有执行效力。

任何新功能先判断是否属于 v0.1.1。若不属于，进入 Backlog，不得阻塞当前纵向链路。

## 0.1 v0.1.1 修订摘要

本版本在 v0.1.0 基础上作出以下强制修订：

1. 产品公开定位调整为“确定性媒体整理器 + 可选 AI 助手”；
2. Metadata Provider 抽象进入 v0.1.1，而不是后续版本；
3. 增加 TMDB 与 AI 组合使用的合规 Gate；
4. AI 默认不得接触 Provider 原始内容或派生内容；
5. TMDB 与 AI 的组合 Gate 未通过前，运行时禁止二者同时启用；
6. 代理、缓存、失败降级和离线重试队列进入 v0.1.1；
7. 匹配评分改为分阶段排序，禁止相关标题信号重复计分；
8. v0.1.1 取消任何自动确认，所有最终匹配均需人工确认；
9. SQLite 默认驱动确定为 `modernc.org/sqlite`；
10. 增加 SQLite WAL 备份策略；
11. CJK 电影文件名解析与独立 held-out 测试集进入发布门槛；
12. Ollama 保留 OpenAI-compatible 接入，同时要求运行时能力探测；
13. v0.2 文件执行器增加 TOCTOU、硬链接、UNC、长路径和大小写碰撞要求；
14. 项目名称正式采用 ReelWarden（影卫），但品牌与商标仍需在首次公开发行前复核。

## 0.1.1 修订追加（2026-06-24，原地修订，未升版）

以下为 v0.1.1 冻结后，基于第三方条款核查与解析链设计讨论的原地追加修订。版本号与文件名保持不变；如需正式新基线再行升 v0.1.2。本节不削弱 §0.2 与 §7.3 的任何护栏，仅作澄清与增补。

15. TMDB API 条款已确认存在明确的 AI/ML 使用限制（覆盖 LLM/AI/ML/chatbot 与 "interactive query-response system"，以及训练、数据集收集）。该限制为用途类约束，不以商业性为前提，"非商业 / 开源 / BYOK / 自用" 均不构成豁免。`COMPLIANCE-TMDB-AI` Gate 由此从"待审"升级为"有条款依据"；默认状态仍为 `blocked`，逻辑不变。条款发现记录于 `docs/compliance/TMDB_AI_GATE.md`，当前为网络来源、待一手核对。
16. 明确区分两类 AI 使用：① AI 在 Provider 调用「之前」、仅基于本地不可信数据（文件名、父目录、本地 NFO 的非 Provider 字段）做标题归一化与媒体类型假设——属允许范围；② AI 接触 Provider 返回内容（含 TMDB 派生字段）——仍受 Gate 阻止。AI 只产出供确定性代码使用的假设，不调用 Provider，也不决定最终匹配。
17. 匹配采用按文件「置信度路由的逐级升级阶梯」（R0–R5），整合 §12.6 与 §14，并新增"本地 AI 文件名修复"级（仅本地输入）。详见 §14.9 与 `docs/design/resolver-pipeline.md`。
18. §7.2 AgentView 扩展：新增 `parent_dir_name` 与相对目录上下文，作为本地不可信数据按 §7.4 处理；Provider 字段与绝对路径仍然禁止。

## 0.2 不可妥协原则

- AI 可完全关闭；
- 没有 AI 时核心功能必须完整可用；
- v0.1.1 不提供真实文件写入 API；
- LLM 不直接访问文件系统；
- LLM 不直接访问 Metadata Provider；
- LLM 不读取 Secret；
- LLM 不决定最终匹配；
- 所有计划由确定性代码生成和校验；
- Provider 故障不能阻断本地扫描；
- BYOK 不被视为第三方条款合规的替代方案；
- 未通过合规 Gate 的能力必须由代码阻止，而不是只写警告。

---

# 1. 产品定义

ReelWarden 是一个：

> 完全本地运行、免费开源、可解释、Plan-first 的确定性媒体库整理器，并提供可选且受限的 AI 助手。

它负责媒体文件进入用户存储之后的：

- 扫描；
- 媒体信息探测；
- 文件名解析；
- 元数据候选检索；
- 匹配证据展示；
- 命名规划；
- 人工审核；
- 后续版本中的安全执行、校验与撤销。

核心产品承诺：

- No account required.
- No subscription required.
- No bundled API key.
- No telemetry by default.
- Bring your own metadata-provider credentials.
- Bring your own AI provider credentials.
- AI is optional.
- Provider data and AI are isolated by default.
- Plan first. Touch files safely.
- Every write operation is auditable.
- Destructive operations require explicit authorization.

中文承诺：

> 无账号、无订阅、默认无遥测、用户自带凭证、AI 可选、Provider 与 AI 默认隔离、先规划后改文件。

## 1.1 产品公开表述

首个公开版本统一使用：

> ReelWarden is a deterministic local media organizer with an optional AI assistant.

在 `COMPLIANCE-TMDB-AI` Gate 通过之前，不得使用以下宣传语：

- “AI media manager powered by TMDB”；
- “TMDB AI Agent”；
- “让 AI 读取 TMDB 元数据”；
- 其他暗示 TMDB 内容被用于 AI 推理的表述。

---

# 2. 用户与使用场景

## 2.1 目标用户

首批目标用户：

- 自建 Jellyfin、Kodi、Emby 或 Plex 媒体库的个人用户；
- 使用本地磁盘、NAS、SMB 或 NFS 挂载目录；
- 愿意自行配置 Metadata Provider 凭证；
- 可选配置 OpenAI-compatible Base URL、API Key 和 Model；
- 可能处于 Metadata Provider 访问不稳定的网络环境；
- 重视隐私、可审查、可解释与可撤销。

## 2.2 首个核心场景

用户配置一个电影目录后，系统：

1. 只读扫描视频文件；
2. 通过 `ffprobe` 提取媒体信息；
3. 解析文件名、父目录、年份和版本标签；
4. 通过已启用的 Metadata Provider 检索候选；
5. 计算并展示匹配依据；
6. 允许用户人工确认或重新搜索；
7. 生成规范目标目录和文件名；
8. 输出完整 Dry Run Action Plan；
9. 不修改任何原文件。

## 2.3 AI 场景

合规 Gate 允许时，AI 可以：

- 将自然语言转换为本地资产筛选条件；
- 查询本地扫描状态；
- 创建 Dry Run Plan 请求；
- 解释本地任务状态；
- 汇总错误；
- 将用户意图转换为确定性 Planner 参数。

AI 不负责：

- 搜索 Metadata Provider；
- 查看 Provider 原始响应；
- 查看 Provider 候选标题、简介、演员、图片或其他字段；
- 构造任意绝对路径；
- 执行文件操作；
- 决定最终影视匹配；
- 修改 Provider、代理、TLS 或 Secret 设置。

## 2.4 典型自然语言指令

- “扫描最近新增的电影，先不要改文件。”
- “只列出还没人工确认的文件。”
- “找出解析年份缺失的项目。”
- “按 Jellyfin 风格生成目录和文件名。”
- “为这批已确认项目生成改名前后对照表。”
- “汇总上一次扫描失败的原因。”

候选内容与匹配解释由确定性 Resolver 和 UI 展示，不依赖 LLM。

---

# 3. 项目阶段

## 3.1 v0.1.1：只读识别与计划

目标：

> 证明“目录 → 扫描 → 解析 → Provider 候选 → 分阶段匹配证据 → 人工确认 → 命名预览 → Dry Run Plan”纵向链路可靠。

必须实现：

- 本地单用户；
- Web 管理界面；
- Go 后端；
- SQLite + WAL；
- `modernc.org/sqlite`；
- 电影扫描；
- `ffprobe` 媒体探测；
- Metadata Provider 接口；
- Mock Provider；
- Local NFO Provider；
- TMDB Adapter；
- Provider 代理配置；
- Provider 本地缓存；
- Provider 失败降级；
- Provider 离线重试队列；
- 候选排序；
- 分阶段证据；
- 手动确认；
- 命名模板；
- Action Plan；
- Dry Run；
- 操作历史；
- OpenAI-compatible BYOK，可关闭；
- AI 能力探测；
- AI 与 Provider 内容隔离；
- `COMPLIANCE-TMDB-AI` 运行时 Gate；
- 不执行真实文件移动、删除、覆盖或写入。

## 3.2 v0.2：安全写入执行器

仅在 v0.1.1 达到发布门槛后增加：

- 安全改名；
- 同盘原子移动；
- 跨盘安全复制；
- NFO 写入；
- 图片写入；
- 目标冲突检测；
- Journal；
- 崩溃恢复；
- Undo；
- 隔离区；
- Jellyfin 或 Kodi 刷新集成；
- 执行时再次校验源文件；
- TOCTOU 防护；
- 硬链接识别；
- 软链接逃逸防护；
- UNC 路径测试；
- Windows 长路径处理；
- 大小写不敏感文件系统碰撞检测。

## 3.3 v0.3 以后

候选方向：

- 标准电视剧；
- 动画绝对集数；
- 多集文件；
- 多版本电影；
- Bangumi；
- TheTVDB；
- 字幕；
- MCP；
- 多用户；
- 远程管理；
- 插件系统；
- Ollama Native Adapter。

这些功能不属于当前承诺。

---

# 4. 明确不做

v0.1.1 不做：

- BT、Usenet、下载器；
- Radarr / Sonarr 替代；
- 字幕下载；
- 在线播放；
- 音乐媒体；
- 电视剧；
- 动画绝对集数；
- ISO、BDMV、DVD 光盘结构；
- 多租户；
- 云端账户；
- 手机 App；
- 自动删除；
- 任意 Shell；
- AI 自动浏览网页刮取元数据；
- AI 直接访问 Metadata Provider；
- AI 直接构造或执行任意文件路径；
- 为用户托管或转售 Provider / AI Key；
- 默认遥测；
- 默认使用非官方 TMDB 镜像；
- 将 BYOK 当作许可证或条款合规证明；
- 在合规 Gate 未通过时同时启用 TMDB 与 AI。

遇到上述需求，记录到 Backlog，不进入 v0.1.1 Sprint。

---

# 5. 技术决策

## 5.1 技术栈

后端：

- Go；
- SQLite；
- `modernc.org/sqlite`；
- WAL 模式；
- 单 Writer；
- REST API；
- SSE 用于任务进度；
- `ffprobe` 作为首选媒体探测工具；
- Go `embed.FS` 嵌入生产前端；
- 单二进制优先；
- Docker 为首个标准部署方式。

前端：

- React；
- TypeScript；
- Vite；
- 首版以桌面浏览器为主要目标；
- 组件库不得阻塞核心业务推进。

AI：

- OpenAI-compatible Adapter；
- Ollama 可通过兼容端点接入；
- 用户手动填写 Base URL、API Key、Model；
- 运行时探测 Streaming、Tool Calling、Structured Output；
- 能力缺失时明确降级；
- AI 默认关闭；
- AI 不属于核心匹配链路；
- Ollama Native Adapter 延后。

Metadata：

- Provider interface-first；
- 首版实现 `mock`、`local_nfo`、`tmdb`；
- TMDB 支持 Read Access Token 和 v3 API Key；
- 默认语言 `zh-CN`；
- 回退语言 `en-US`；
- 每个 Provider 独立缓存、超时、限速、代理、重试和队列；
- Provider 故障不能阻断扫描；
- TMDB 默认只允许官方 Endpoint；
- 代理支持是一等配置；
- 非官方 Endpoint Override 不进入普通设置页。

许可证：

- 主项目：AGPL-3.0；
- 文档与示例：CC BY 4.0；
- 第三方依赖逐项保留许可证和 NOTICE；
- 若分发 FFmpeg，必须核验具体构建的 LGPL/GPL 组成。

## 5.2 仓库结构

```text
reelwarden/
├── apps/
│   ├── server/
│   │   └── main.go
│   └── web/
├── internal/
│   ├── api/
│   ├── catalog/
│   ├── scanner/
│   ├── parser/
│   ├── probe/
│   ├── metadata/
│   │   ├── provider.go
│   │   ├── registry.go
│   │   ├── gateway.go
│   │   ├── cache.go
│   │   ├── retry_queue.go
│   │   └── providers/
│   │       ├── mock/
│   │       ├── localnfo/
│   │       └── tmdb/
│   ├── resolver/
│   ├── planner/
│   ├── naming/
│   ├── jobs/
│   ├── ai/
│   │   ├── provider.go
│   │   ├── gateway.go
│   │   ├── capabilities.go
│   │   ├── context_policy.go
│   │   └── providers/
│   │       └── openai_compatible/
│   ├── compliance/
│   ├── config/
│   ├── secrets/
│   └── observability/
├── migrations/
├── testdata/
│   ├── filenames/
│   │   ├── development/
│   │   └── heldout/
│   ├── metadata/
│   └── media/
├── deployments/
│   └── docker/
├── docs/
│   ├── adr/
│   ├── api/
│   ├── reviews/
│   └── compliance/
├── scripts/
├── LICENSE
├── README.md
├── CONTRIBUTING.md
├── SECURITY.md
└── Makefile
```

---

# 6. 架构边界

## 6.1 总体架构

```text
Web UI
  │
  ▼
HTTP API
  │
  ├── Catalog
  ├── Scan Jobs
  ├── Metadata Gateway
  ├── Resolver
  ├── Naming
  ├── Planner
  ├── Settings
  ├── Compliance Gate
  └── AI Gateway
        │
        └── OpenAI-compatible Provider

Metadata Gateway
  ├── Provider Registry
  ├── Mock Provider
  ├── Local NFO Provider
  ├── TMDB Adapter
  ├── Cache
  ├── Proxy
  └── Retry Queue

Local Services
  ├── SQLite
  ├── ffprobe
  ├── Filesystem Scanner
  ├── Secret Store
  └── Audit Log
```

强制数据边界：

```text
Metadata Provider
      │
      ▼
Provider Gateway / Cache
      │
      ├── Resolver
      └── Web UI

AI Gateway  ─────X─────> Provider Gateway / Cache
AI Gateway  ─────X─────> ProviderItem.raw_payload
AI Gateway  ─────X─────> Secret Store
```

## 6.2 模块责任

### scanner

负责：

- 枚举允许目录中的文件；
- 识别视频扩展名；
- 读取大小、修改时间、设备、inode 或平台等价信息；
- 创建或更新 `media_assets`；
- 不解析影视语义；
- 不访问 Metadata Provider；
- 不修改文件。

### probe

负责：

- 调用 `ffprobe`；
- 获取容器、视频、音频、字幕、时长和分辨率；
- 保存原始 JSON；
- 输出规范化结果；
- 对超时和异常做明确标记。

### parser

负责：

- 从路径和文件名提取标题、年份、版本、发布组和技术标签；
- 保留原始输入；
- 输出一个或多个解析假设；
- 支持 CJK 电影标题；
- 不访问网络；
- 不做最终匹配。

### metadata/provider

统一接口负责：

- 声明 Provider 能力；
- 测试连接；
- 搜索电影候选；
- 获取电影详情；
- 返回统一 DTO；
- 声明缓存策略；
- 不向 AI 暴露内部对象。

### metadata/gateway

负责：

- Provider 注册；
- 凭证引用；
- 代理；
- 超时；
- 限速；
- 缓存；
- 请求合并；
- 失败重试；
- 离线队列；
- 审计；
- Compliance Gate 检查。

### metadata/providers/tmdb

负责：

- TMDB 认证；
- 搜索电影；
- 获取电影详情；
- 获取图片配置；
- 将响应规范化；
- 保留原始 Provider ID；
- 使用官方 Endpoint；
- 不访问 AI；
- 不决定匹配。

### resolver

负责：

- 合并解析结果、媒体信息与 Provider 候选；
- 执行硬约束；
- 计算分组证据；
- 排序候选；
- 生成 `candidate_matches`；
- 提供确定性匹配解释；
- 不写文件；
- 不调用 LLM；
- 不自动确认。

### naming

负责：

- 根据模板生成目标目录和文件名；
- 清洗非法字符；
- 检查保留名称、长度与冲突；
- v0.1.1 只生成预览。

### planner

负责：

- 将人工确认结果与命名规则转为不可变 Action Plan；
- 记录输入摘要；
- 生成前后对照；
- 标记风险；
- v0.1.1 的 Plan 不可进入执行状态。

### ai

负责：

- 自然语言理解；
- 本地资产筛选；
- 计划请求参数化；
- 任务状态总结；
- 只读取 `AgentView`；
- 不访问 `ProviderItem`；
- 不访问 Provider Cache；
- 不访问 Secret；
- 不访问任意路径；
- 不执行 Shell。

### compliance

负责：

- 维护编译时和运行时 Gate；
- 阻止不允许的功能组合；
- 输出 Gate 状态和原因；
- 保存决定记录；
- 禁止普通配置绕过 Gate。

---

# 7. 安全与合规模型

## 7.1 AI 能力边界

允许向 AI 暴露的工具：

```text
search_local_assets
inspect_local_asset
filter_local_assets
create_dry_run_plan_request
summarize_job
list_plan_status
```

禁止暴露：

```text
search_metadata_provider
list_provider_candidates
read_provider_item
read_provider_cache
explain_provider_match_with_llm
run_shell
execute_command
delete_file
move_any_file
write_any_file
read_secret
change_base_url
change_proxy
change_api_key
disable_tls
override_compliance_gate
```

AI 只能传递内部对象 ID 和经过 Schema 校验的参数。

## 7.2 AgentView

AI 只允许读取专门生成的 `AgentView`：

```json
{
  "asset_id": "asset_01",
  "sanitized_file_name": "Dune.2021.2160p.mkv",
  "parent_dir_name": "进击的巨人 真人版",
  "relative_dir": "anime/进击的巨人 真人版",
  "parse_state": "parsed",
  "parsed_year": 2021,
  "match_state": "needs_review",
  "score_band": "high",
  "plan_state": "none"
}
```

不得包含：

- Provider 名称；
- Provider Item ID；
- Provider 标题；
- 原始标题；
- 简介；
- 演员；
- 图片；
- Provider 原始 JSON；
- TMDB / TVDB / Bangumi 字段；
- Secret；
- 绝对路径。

`parent_dir_name` 与 `relative_dir` 属本地不可信输入，按 §7.4 处理（不得作为系统指令拼接），仅用于 AI 生成标题归一化与媒体类型假设；绝对路径仍然禁止。基于这些本地字段，AI 只产出供确定性代码使用的「假设」，不调用 Metadata Provider，也不决定最终匹配（见 §14.1、§14.9）。

## 7.3 `COMPLIANCE-TMDB-AI` Gate

默认状态：

```text
blocked
```

运行时规则：

```text
tmdb.enabled = true
AND ai.enabled = true
AND compliance.tmdb_ai != accepted
→ 启动失败或拒绝启用后配置项
→ 返回 COMPLIANCE_PROVIDER_AI_COMBINATION_BLOCKED
```

普通用户配置不能改变 Gate 状态。

本 Gate 的条款依据、风险摘要与允许/禁止数据流记录于 `docs/compliance/TMDB_AI_GATE.md`（当前为网络来源、待一手核对；默认状态 `blocked` 不变）。

Gate 可以通过以下任一方式解除：

1. 获得 Provider 或权利方的明确书面许可或澄清；
2. 经正式法律审查形成可公开存档的结论；
3. 改用明确允许该组合用途的 Provider；
4. 移除产生冲突的产品能力。

以下事项不能单独解除 Gate：

- 用户自带 API Key；
- 非商业；
- 开源；
- 添加免责声明；
- 不收费；
- AI 只看部分字段；
- 用户自行承担责任。

在 Gate 未解除时允许的组合：

```text
TMDB + AI disabled
Local NFO / Mock Provider + AI enabled
No Provider + AI enabled
```

## 7.4 Prompt Injection 与不可信输入

以下全部视为不可信数据：

- 文件名；
- 目录名；
- NFO；
- 字幕；
- Provider 内容；
- 海报文字；
- 用户导入的 JSON；
- 外部 API 错误信息。

不可信数据不得拼接为系统指令。安全保证不依赖提示词过滤，而依赖：

- 最小工具权限；
- Provider 与 AI 隔离；
- 无写入工具；
- 确定性参数校验；
- 人工审核；
- Action Plan；
- 审计记录。

## 7.5 路径安全

所有路径必须：

- 位于用户明确配置的 Library Root 内；
- 使用规范化绝对路径比较；
- 拒绝 `..` 穿越；
- 默认拒绝软链接逃逸；
- 拒绝空目标；
- 拒绝根目录目标；
- 拒绝覆盖未纳入计划的文件；
- 处理 Windows 保留名；
- 处理大小写碰撞；
- 记录路径所在设备；
- v0.2 执行时重新校验；
- v0.2 考虑硬链接和 TOCTOU。

## 7.6 密钥安全

凭证优先级：

1. Docker Secret 或只读 Secret 文件；
2. 系统 Keychain / Credential Manager；
3. 环境变量；
4. 加密本地存储；
5. 禁止明文写入普通配置文件。

Headless NAS 和 Docker 是一等环境，不得假设系统 Keychain 一定存在。

规则：

- 日志不输出 API Key；
- 前端不重新显示完整 Key；
- 诊断包自动脱敏；
- 数据库备份默认不包含 Secret；
- 测试连接只访问目标服务；
- TLS 校验默认开启；
- Provider Token 不发送给 AI；
- AI Key 不发送给 Provider；
- 代理认证信息使用独立 Secret Ref；
- TMDB Token 默认不发送给非官方 Endpoint。

## 7.7 隐私模式

### Local Only

- AI 关闭；
- 所有核心功能可用；
- 匹配采用确定性解析和评分。

### Minimal Context，默认

AI 可接收：

- 用户输入；
- 内部对象 ID；
- 清洗后的本地文件名；
- 本地解析状态；
- 匹配状态；
- 分数区间；
- 计划状态；
- 已脱敏错误。

AI 不接收 Provider 内容。

### Full Local Context

用户可主动允许：

- 相对路径；
- 父目录；
- 本地 NFO 中由用户明确创建的字段；
- 更完整的本地媒体信息。

即使启用 Full Local Context，也不得绕过 Provider 内容隔离和 Compliance Gate。

---

# 8. 配置规范

建议配置：

```yaml
server:
  listen: 127.0.0.1:8787
  data_dir: ./data
  log_level: info

database:
  driver: modernc-sqlite
  path: ./data/reelwarden.db
  wal: true
  max_open_conns: 1
  backup_dir: ./data/backups

library:
  roots:
    - id: movies-main
      path: /media/movies
      mode: read_only

metadata:
  default_provider: local_nfo

  providers:
    mock:
      enabled: true

    local_nfo:
      enabled: true

    tmdb:
      enabled: false
      auth_type: bearer_token
      credential_ref: secret://tmdb/default
      language: zh-CN
      fallback_language: en-US
      region: CN
      official_endpoint_only: true
      timeout_seconds: 15
      max_retries: 2
      requests_per_second: 3
      proxy:
        enabled: false
        url_ref: secret://proxy/tmdb

  cache:
    enabled: true
    search_ttl_hours: 24
    detail_ttl_hours: 168
    negative_ttl_minutes: 15

  retry_queue:
    enabled: true
    max_attempts: 5
    base_delay_seconds: 30
    max_delay_seconds: 3600

ai:
  enabled: false
  provider: openai-compatible
  base_url: http://localhost:11434/v1
  credential_ref: secret://ai/default
  model: qwen3
  protocol: chat-completions
  timeout_seconds: 120
  max_retries: 2

  capabilities:
    streaming: auto
    tool_calling: auto
    structured_output: auto

  privacy:
    mode: minimal
    send_absolute_paths: false
    send_provider_content: false
    send_nfo_content: false

compliance:
  tmdb_ai_gate: blocked
  allow_runtime_override: false

naming:
  profile: jellyfin
  movie_folder: "{{ .Title }} ({{ .Year }})"
  movie_file: "{{ .Title }} ({{ .Year }}){{ .EditionSuffix }}"

features:
  real_file_writes: false
  telemetry: false
```

环境变量：

```bash
REELWARDEN_DATA_DIR=
REELWARDEN_LOG_LEVEL=
REELWARDEN_TMDB_TOKEN=
REELWARDEN_TMDB_PROXY_URL=
REELWARDEN_AI_BASE_URL=
REELWARDEN_AI_API_KEY=
REELWARDEN_AI_MODEL=
```

环境变量覆盖配置文件，但不得将覆盖后的 Secret 回写配置文件。

`compliance.tmdb_ai_gate` 不能通过普通环境变量或用户设置修改。

---

# 9. 领域模型

## 9.1 MediaAsset

表示物理文件。

字段：

- `id`
- `library_root_id`
- `absolute_path`
- `relative_path`
- `file_name`
- `extension`
- `size_bytes`
- `modified_at`
- `device_id`
- `inode`
- `quick_fingerprint`
- `scan_status`
- `probe_status`
- `created_at`
- `updated_at`

## 9.2 ParsedIdentity

表示文件名解析结果。

字段：

- `id`
- `media_asset_id`
- `raw_title`
- `normalized_title`
- `year`
- `edition`
- `release_group`
- `technical_tags_json`
- `parser_version`
- `confidence`
- `created_at`

解析器的 `confidence` 也是启发式分数，不得表示概率。

## 9.3 ProviderItem

表示 Provider 条目快照。

字段：

- `id`
- `provider`
- `provider_item_id`
- `media_type`
- `title`
- `original_title`
- `release_date`
- `year`
- `runtime_minutes`
- `original_language`
- `aliases_json`
- `raw_payload_json`
- `fetched_at`
- `expires_at`
- `ai_export_allowed`

v0.1.1 中 `ai_export_allowed` 固定为 `false`。

## 9.4 CandidateMatch

表示候选匹配及证据。

字段：

- `id`
- `media_asset_id`
- `provider_item_id`
- `rank_score`
- `rank`
- `score_band`
- `evidence_json`
- `warnings_json`
- `resolver_version`
- `created_at`

## 9.5 LibraryItem

表示用户已确认的逻辑媒体条目。

字段：

- `id`
- `media_type`
- `title`
- `original_title`
- `year`
- `provider`
- `provider_item_id`
- `match_state`
- `confirmed_by`
- `confirmed_at`
- `created_at`
- `updated_at`

v0.1.1 不允许系统自动设置 `confirmed_by=system`。

## 9.6 ActionPlan

字段：

- `id`
- `kind`
- `status`
- `scope_json`
- `policy_snapshot_json`
- `input_digest`
- `risk_level`
- `requires_approval`
- `created_by`
- `created_at`
- `approved_at`
- `expires_at`

状态：

```text
draft
ready
approved
rejected
expired
executing
verified
committed
failed
rolled_back
```

v0.1.1 只允许：

```text
draft → ready → approved / rejected / expired
```

不得进入 `executing`。

## 9.7 ProviderQueueItem

字段：

- `id`
- `provider`
- `operation`
- `request_fingerprint`
- `payload_json`
- `status`
- `attempt_count`
- `next_attempt_at`
- `last_error_code`
- `created_at`
- `updated_at`

状态：

```text
queued
running
succeeded
retry_wait
failed
cancelled
```

## 9.8 ComplianceGate

字段：

- `id`
- `status`
- `reason`
- `evidence_ref`
- `decided_by`
- `decided_at`
- `created_at`
- `updated_at`

状态：

```text
blocked
under_review
accepted
retired
```

## 9.9 AgentContextAudit

只记录发送给 AI 的字段类别和摘要，不记录 Secret。

字段：

- `id`
- `request_id`
- `provider`
- `model`
- `field_classes_json`
- `payload_digest`
- `contains_provider_content`
- `created_at`

`contains_provider_content` 在 v0.1.1 必须始终为 `false`。

---

# 10. SQLite 初始 Schema

最终以 Migration 文件为准。

```sql
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

CREATE TABLE library_roots (
    id TEXT PRIMARY KEY,
    path TEXT NOT NULL UNIQUE,
    mode TEXT NOT NULL DEFAULT 'read_only',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE media_assets (
    id TEXT PRIMARY KEY,
    library_root_id TEXT NOT NULL,
    absolute_path TEXT NOT NULL UNIQUE,
    relative_path TEXT NOT NULL,
    file_name TEXT NOT NULL,
    extension TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    modified_at TEXT NOT NULL,
    device_id TEXT,
    inode TEXT,
    quick_fingerprint TEXT,
    scan_status TEXT NOT NULL,
    probe_status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (library_root_id) REFERENCES library_roots(id)
);

CREATE INDEX idx_media_assets_root
ON media_assets(library_root_id);

CREATE INDEX idx_media_assets_fingerprint
ON media_assets(quick_fingerprint);

CREATE TABLE media_probes (
    id TEXT PRIMARY KEY,
    media_asset_id TEXT NOT NULL UNIQUE,
    duration_seconds REAL,
    container_format TEXT,
    width INTEGER,
    height INTEGER,
    video_codec TEXT,
    audio_streams_json TEXT NOT NULL DEFAULT '[]',
    subtitle_streams_json TEXT NOT NULL DEFAULT '[]',
    raw_payload_json TEXT NOT NULL,
    probe_version TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (media_asset_id) REFERENCES media_assets(id)
);

CREATE TABLE parsed_identities (
    id TEXT PRIMARY KEY,
    media_asset_id TEXT NOT NULL,
    raw_title TEXT NOT NULL,
    normalized_title TEXT NOT NULL,
    year INTEGER,
    edition TEXT,
    release_group TEXT,
    technical_tags_json TEXT NOT NULL DEFAULT '[]',
    parser_version TEXT NOT NULL,
    confidence REAL NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (media_asset_id) REFERENCES media_assets(id)
);

CREATE INDEX idx_parsed_asset
ON parsed_identities(media_asset_id);

CREATE TABLE provider_items (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    provider_item_id TEXT NOT NULL,
    media_type TEXT NOT NULL,
    title TEXT NOT NULL,
    original_title TEXT,
    release_date TEXT,
    year INTEGER,
    runtime_minutes INTEGER,
    original_language TEXT,
    aliases_json TEXT NOT NULL DEFAULT '[]',
    raw_payload_json TEXT NOT NULL,
    ai_export_allowed INTEGER NOT NULL DEFAULT 0,
    fetched_at TEXT NOT NULL,
    expires_at TEXT,
    UNIQUE(provider, provider_item_id)
);

CREATE TABLE provider_cache_entries (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    response_json TEXT NOT NULL,
    status_code INTEGER NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(provider, request_fingerprint)
);

CREATE TABLE provider_queue_items (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    operation TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    status TEXT NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT,
    last_error_code TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_provider_queue_due
ON provider_queue_items(status, next_attempt_at);

CREATE TABLE candidate_matches (
    id TEXT PRIMARY KEY,
    media_asset_id TEXT NOT NULL,
    provider_item_id TEXT NOT NULL,
    rank_score REAL NOT NULL,
    rank INTEGER NOT NULL,
    score_band TEXT NOT NULL,
    evidence_json TEXT NOT NULL,
    warnings_json TEXT NOT NULL DEFAULT '[]',
    resolver_version TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (media_asset_id) REFERENCES media_assets(id),
    FOREIGN KEY (provider_item_id) REFERENCES provider_items(id)
);

CREATE INDEX idx_candidate_asset_rank
ON candidate_matches(media_asset_id, rank);

CREATE TABLE library_items (
    id TEXT PRIMARY KEY,
    media_type TEXT NOT NULL,
    title TEXT NOT NULL,
    original_title TEXT,
    year INTEGER,
    provider TEXT NOT NULL,
    provider_item_id TEXT NOT NULL,
    match_state TEXT NOT NULL,
    confirmed_by TEXT,
    confirmed_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(provider, provider_item_id)
);

CREATE TABLE asset_bindings (
    media_asset_id TEXT PRIMARY KEY,
    library_item_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (media_asset_id) REFERENCES media_assets(id),
    FOREIGN KEY (library_item_id) REFERENCES library_items(id)
);

CREATE TABLE action_plans (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    scope_json TEXT NOT NULL,
    policy_snapshot_json TEXT NOT NULL,
    input_digest TEXT NOT NULL,
    risk_level TEXT NOT NULL,
    requires_approval INTEGER NOT NULL,
    created_by TEXT NOT NULL,
    created_at TEXT NOT NULL,
    approved_at TEXT,
    expires_at TEXT
);

CREATE TABLE plan_operations (
    id TEXT PRIMARY KEY,
    plan_id TEXT NOT NULL,
    sequence_no INTEGER NOT NULL,
    operation_type TEXT NOT NULL,
    source_asset_id TEXT,
    source_path TEXT,
    destination_path TEXT,
    preview_json TEXT NOT NULL,
    preconditions_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(plan_id, sequence_no),
    FOREIGN KEY (plan_id) REFERENCES action_plans(id)
);

CREATE TABLE jobs (
    id TEXT PRIMARY KEY,
    job_type TEXT NOT NULL,
    status TEXT NOT NULL,
    progress_current INTEGER NOT NULL DEFAULT 0,
    progress_total INTEGER NOT NULL DEFAULT 0,
    input_json TEXT NOT NULL,
    result_json TEXT,
    error_json TEXT,
    created_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT
);

CREATE TABLE compliance_gates (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    reason TEXT NOT NULL,
    evidence_ref TEXT,
    decided_by TEXT,
    decided_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE agent_context_audits (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    ai_provider TEXT NOT NULL,
    model TEXT NOT NULL,
    field_classes_json TEXT NOT NULL,
    payload_digest TEXT NOT NULL,
    contains_provider_content INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id TEXT,
    object_type TEXT NOT NULL,
    object_id TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
```

## 10.1 数据库连接策略

- `MaxOpenConns = 1`；
- WAL；
- 所有写操作通过统一 Repository 层；
- 长任务不得持有长事务；
- Provider HTTP 请求不得在数据库事务内执行；
- 读 API 可使用短事务或快照查询；
- Migration 启动时串行执行。

## 10.2 备份策略

推荐：

```text
定期 WAL checkpoint
→ VACUUM INTO 临时备份文件
→ 原子重命名为最终备份
→ 保留最近 N 份
```

禁止在活跃 WAL 状态下只复制主 `.db` 文件并宣称是完整备份。

---

# 11. 扫描规范

## 11.1 支持扩展名

v0.1.1 初始支持：

```text
.mkv
.mp4
.m4v
.avi
.mov
.ts
.m2ts
.webm
```

扩展名可配置，但不得默认扫描任意文件。

## 11.2 忽略规则

默认忽略：

```text
.@eaDir
@eaDir
.recycle
.Trash
.Trashes
.Spotlight-V100
lost+found
sample
extras
featurettes
```

对 `sample` 的识别同时考虑：

- 路径片段；
- 文件名；
- 文件大小；
- 时长。

## 11.3 文件身份

首轮扫描使用：

```text
absolute_path + size + modified_at
```

检测移动或重命名时可增加快速指纹：

```text
文件大小
+ 首部固定字节哈希
+ 尾部固定字节哈希
```

v0.1.1 不要求完整文件哈希。

## 11.4 稳定文件检测

对于正在复制的文件：

- 连续两次读取大小一致；
- 修改时间间隔超过稳定窗口；
- 才进入 Probe；
- 否则标记为 `pending_stability`。

## 11.5 Provider 故障隔离

扫描 Job 与 Provider Job 分离：

```text
scan succeeded
metadata pending
```

是合法状态。

Metadata Provider 不可达时：

- 扫描仍成功；
- 本地解析仍成功；
- 资产标记为 `metadata_pending`；
- 请求进入离线重试队列；
- UI 明确显示 Provider 状态；
- 不反复无界重试。

---

# 12. 文件名解析规范

## 12.1 输入保留

必须保留：

- 原始绝对路径；
- 相对路径；
- 原始文件名；
- 去扩展名文件名；
- 父目录名称；
- 清洗后候选标题；
- 被移除的技术标签。

## 12.2 常见技术标签

可识别但不作为标题：

```text
2160p
1080p
720p
BluRay
WEB-DL
WEBRip
HDR
DV
Dolby Vision
x264
x265
H.264
H.265
HEVC
AV1
AAC
DTS
TrueHD
Atmos
REMUX
PROPER
REPACK
UNCUT
EXTENDED
DIRECTORS CUT
```

## 12.3 标题归一化

用于候选检索与评分，不能覆盖展示标题。

处理：

- Unicode 规范化；
- 大小写折叠；
- 句点、下划线与空格统一；
- 多余空格折叠；
- 全角半角转换；
- 简单罗马数字兼容；
- 中英文标点兼容；
- 保留数字；
- 保留 CJK 字符；
- 不擅自翻译标题；
- 不擅自进行简繁转换后覆盖原值。

可以额外生成简繁归一化比较键，但必须保留原始值和转换来源。

## 12.4 年份

优先识别 1900–当前年份后 2 年之间的四位数。

年份必须与标题解析共同判断，不能简单取文件名中的最后一个四位数。

## 12.5 v0.1.1 范围

支持：

- 电影；
- 中英文标题；
- 简体、繁体；
- 日文、韩文；
- 同名不同年份；
- 版本标签；
- 发行组；
- 技术标签。

不支持：

- 标准电视剧季集；
- 日期型剧集；
- 动画绝对集数；
- 多集文件。

## 12.6 解析器评估

语料至少 200 条：

```text
development: 150
heldout: 50
```

Held-out 在发布前不得用于调整规则。

目标：

- 常见模式标题字段准确率 ≥ 95%；
- 有年份样本的年份字段准确率 ≥ 98%；
- 不得因追求召回率而错误剥离标题中的数字；
- 低置信度允许输出多个假设；
- 无法可靠解析时返回 `unresolved`，不得伪造高置信度结果。

---

# 13. Metadata Provider 规范

## 13.1 Provider 接口

```go
type MetadataProvider interface {
    ID() string

    Capabilities() ProviderCapabilities

    Probe(
        ctx context.Context,
        cfg ProviderConfig,
    ) error

    SearchMovies(
        ctx context.Context,
        query MovieSearchQuery,
    ) ([]MovieCandidate, error)

    GetMovie(
        ctx context.Context,
        externalID string,
    ) (*MovieMetadata, error)
}

type ProviderCapabilities struct {
    MovieSearch   bool
    MovieDetails  bool
    Artwork       bool
    LocalOnly     bool
    RequiresAuth  bool
}
```

Resolver 和 UI 只能依赖统一 DTO，不得依赖 TMDB 特有结构。

## 13.2 首版 Provider

### Mock Provider

用途：

- 单元测试；
- 集成测试；
- 无网络演示；
- 复现固定候选排序。

### Local NFO Provider

用途：

- 读取用户本地已有 NFO；
- 不访问网络；
- 为 AI 模式提供不依赖外部 Provider 的可用路径；
- v0.1.1 只读。

### TMDB Adapter

支持：

- Read Access Token，推荐；
- v3 API Key；
- 电影搜索；
- 电影详情；
- 图片配置；
- 本地缓存；
- 限速；
- 代理；
- 离线重试；
- 署名信息。

## 13.3 TMDB Endpoint 与代理

默认：

- 使用官方 Endpoint；
- TLS 校验开启；
- 不允许普通用户修改 Host；
- 支持 HTTP / HTTPS / SOCKS 代理；
- 代理 URL 可引用 Secret；
- 测试连接显示代理是否生效；
- 不把 Token 写入 URL 日志。

非官方镜像或 Endpoint Override：

- 不进入普通设置页面；
- 不属于 v0.1.1 正式支持范围；
- 必须明确提示 Token 泄露和条款风险；
- 不得默认启用。

## 13.4 请求行为

所有 Provider 必须：

- 配置超时；
- 有界重试；
- 指数退避；
- 尊重 429 / Retry-After；
- 记录请求类型与耗时；
- 日志不记录认证头；
- 支持请求缓存；
- 支持同请求并发合并；
- 支持手动取消；
- 失败不阻塞扫描；
- 不在数据库事务内发起网络请求。

## 13.5 缓存

建议：

- 搜索结果：24 小时；
- 详情：7 天；
- 图片配置：7 天；
- 失败响应：15 分钟负缓存；
- 用户手动刷新可绕过缓存；
- 缓存命中仍记录 Provider 来源；
- Provider 内容不得进入 AI Context。

## 13.6 离线重试队列

规则：

- 每个逻辑请求有唯一 fingerprint；
- 同一请求不重复入队；
- 最大尝试 5 次；
- 指数退避；
- 支持手动立即重试；
- 支持取消；
- 达到上限后进入 `failed`；
- 不允许无限循环；
- Provider 恢复后可继续处理；
- 队列状态可在 UI 查看。

## 13.7 合规 Gate

TMDB Adapter 的开发、测试和纯确定性使用可以继续。

以下能力在 `COMPLIANCE-TMDB-AI=blocked` 时禁止：

- TMDB 与 AI 同时启用；
- 将 TMDB 候选字段放入 AI Prompt；
- 让 AI 解释 TMDB 候选；
- 让 AI 基于 TMDB 内容生成推荐或总结；
- 宣传产品为 TMDB 驱动的 AI Agent。

项目必须维护：

```text
docs/compliance/TMDB_AI_GATE.md
```

内容包括：

- 当前条款版本与日期；
- 风险摘要；
- 已发送的澄清请求；
- 收到的回复；
- 最终决定；
- 允许的数据流；
- 禁止的数据流。

---

# 14. 候选匹配与评分

## 14.1 原则

LLM 不是匹配器。

匹配结构：

```text
规则解析
→ Provider 候选检索
→ 硬约束
→ 分组证据排序
→ 冲突惩罚
→ 候选列表
→ 人工确认
```

`rank_score` 只用于排序，不是概率。

## 14.2 阶段 0：显式外部 ID

若文件名或本地 NFO 存在有效外部 ID：

- 对精确 ID 候选给予最高优先级；
- 仍检查媒体类型；
- 仍展示证据；
- v0.1.1 仍需人工确认；
- 无效或冲突 ID 进入警告。

## 14.3 阶段 1：硬约束

硬约束包括：

- 媒体类型；
- 明显非法年份；
- 明显不兼容的候选；
- 用户已明确排除的候选；
- Provider ID 冲突。

硬约束失败的候选应被过滤或强烈降级，而不是依赖加权分数补回来。

## 14.4 阶段 2：标题证据组

相关标题信号只取最强证据，不相加：

```go
titleScore := max(
    exactNormalizedTitle,
    exactOriginalTitle,
    exactAlias,
    fuzzyTitleSimilarity,
)
```

禁止：

```text
exact title
+ alias exact
+ fuzzy similarity
```

同时为同一标题事实重复加分。

## 14.5 阶段 3：辅助证据

可追加：

- 年份精确一致；
- 年份相差 1；
- 时长接近；
- 父目录一致；
- 版本信息一致；
- 本地 NFO 外部 ID；
- 文件夹内同类资产上下文。

辅助证据必须按组限制上限。

## 14.6 阶段 4：冲突惩罚

包括：

- 媒体类型冲突；
- 年份差大于 2；
- 时长严重冲突；
- 标题核心词明显冲突；
- 外部 ID 冲突；
- 用户曾拒绝。

惩罚必须在证据中可见。

## 14.7 分数区间

```text
rank_score >= 0.95
  score_band = high
  默认预选第一候选
  仍需人工确认

0.80 <= rank_score < 0.95
  score_band = medium
  明确要求审核

rank_score < 0.80
  score_band = low
  保持未识别或要求重新搜索
```

v0.1.1 不允许自动确认。

自动确认必须满足：

- 独立标注集验证；
- 精确率门槛；
- 错配成本评估；
- 新 ADR；
- 修改本文档；
- 明确回滚方案。

## 14.8 证据格式

```json
{
  "rank_score": 0.93,
  "score_band": "medium",
  "groups": [
    {
      "group": "title",
      "selected_evidence": {
        "type": "normalized_title_exact",
        "contribution": 0.55,
        "detail": "Dune == Dune"
      },
      "discarded_correlated_evidence": [
        "fuzzy_title_similarity"
      ]
    },
    {
      "group": "year",
      "selected_evidence": {
        "type": "release_year_exact",
        "contribution": 0.25,
        "detail": "2021 == 2021"
      }
    },
    {
      "group": "runtime",
      "selected_evidence": {
        "type": "runtime_close",
        "contribution": 0.08,
        "detail": "file=155.2m, provider=155m"
      }
    }
  ],
  "penalties": [],
  "warnings": []
}
```

## 14.9 置信度路由与逐级升级（R0–R5）

匹配按「每文件、按置信度逐级升级」的阶梯执行：每个文件只爬到足以达到 `score_band` 阈值的那一级即停。该模型整合 §12（解析）、§12.6（多假设 / `unresolved`）与 §14.1–14.8（分阶段证据与分数区间），不替代它们。

```text
R0 输入保留 + 标题归一化              §12.1 / §12.3，总是执行
R1 显式外部 ID / 本地 NFO ID          §14.2 命中即最高优先，按 ID 检索
R2 本地结构化信号                     时长（媒体探测）、媒体类型 token、父目录 / 同级上下文 → 拼带约束查询
R3 对结构化字段确定性打分             §14.3–14.7，输出 rank_score + score_band
R4 本地 AI 文件名修复（仅本地输入）   低置信度兜底：AI 只读本地信号产出归一化 / 判型假设，回灌 R2/R3 重算
R5 unresolved + 多假设交人工          §12.6，不得伪造高置信度
```

约束：

- 置信度是路由器：文件类型事先不可知，阶梯自行发现——带 ID 的在 R1 停，干净的在 R3 停，乱码滑到 R4/R5。
- R4 的 AI 仅消费本地不可信数据（文件名、父目录、本地 NFO 非 Provider 字段，见 §7.2），绝不接触 Provider 返回内容；AI 只产假设，确定性代码做全部检索、打分与选择（守 §14.1「LLM 不是匹配器」、§0.2「LLM 不直接访问 Provider」）。
- 不确定是合法输出：`unresolved` 或「多候选 + 证据」交人工，优先于自信的错配；v0.1.1 不允许自动确认（§14.7）。
- 每级必须产出证据（§14.8），供人工审核与后续学习（§14.6 的「用户曾拒绝」惩罚）。

实现契约（结构、接口、各级退出条件）见 `docs/design/resolver-pipeline.md`。

---

# 15. 命名规范

## 15.1 默认 Jellyfin 风格

目录：

```text
{{ Title }} ({{ Year }})
```

文件：

```text
{{ Title }} ({{ Year }}){{ EditionSuffix }}.{{ Extension }}
```

示例：

```text
Dune (2021)/Dune (2021).mkv
```

## 15.2 模板字段

v0.1.1 支持：

```text
Title
OriginalTitle
Year
Edition
EditionSuffix
ProviderID
Extension
```

默认模板不得写死 `TMDBID`。

Provider 特有 ID 可通过命名空间字段后续扩展：

```text
ExternalIDs.tmdb
ExternalIDs.imdb
```

## 15.3 文件名清洗

必须处理：

- `/`、`\`；
- Windows 非法字符；
- 控制字符；
- 结尾空格与点；
- Windows 保留名；
- 连续空格；
- 路径长度；
- 空标题；
- 同名冲突；
- 大小写仅变化；
- Unicode 规范化差异。

清洗结果必须在预览中展示。

---

# 16. Action Plan

## 16.1 设计原则

Action Plan：

- 不可变；
- 有唯一 ID；
- 有输入摘要；
- 有策略快照；
- 有创建时间与过期时间；
- 有完整操作顺序；
- 有风险等级；
- 有审批状态；
- 有前置条件；
- v0.1.1 永远只读；
- 不保存可由 AI 任意注入的绝对路径；
- 目标路径由 Naming Engine 生成。

## 16.2 示例

```json
{
  "id": "plan_20260624_0001",
  "kind": "organize_movies",
  "status": "ready",
  "risk_level": "read_only",
  "requires_approval": true,
  "input_digest": "sha256:...",
  "scope": {
    "media_asset_ids": ["asset_01"]
  },
  "policy_snapshot": {
    "naming_profile": "jellyfin",
    "overwrite_existing_nfo": false,
    "real_file_writes": false
  },
  "operations": [
    {
      "sequence_no": 1,
      "operation_type": "preview_move",
      "source_asset_id": "asset_01",
      "source_path": "/media/inbox/Dune.2021.2160p.mkv",
      "destination_path": "/media/movies/Dune (2021)/Dune (2021).mkv",
      "preconditions": {
        "size_bytes": 123456789,
        "modified_at": "2026-06-24T10:00:00Z"
      }
    }
  ]
}
```

## 16.3 失效条件

以下任一发生时，Plan 必须失效：

- 源文件大小变化；
- 修改时间变化；
- 路径变化且无法通过指纹确认；
- 匹配结果被修改；
- 命名策略被修改；
- Library Root 被禁用；
- 计划超过过期时间；
- 目标路径规则版本变化；
- Compliance Gate 状态变化影响计划；
- v0.1.1 尝试进入写执行状态。

---

# 17. REST API v0.1.1

前缀：

```text
/api/v1
```

## 17.1 系统

```http
GET /health
GET /version
GET /capabilities
GET /compliance/gates
GET /compliance/gates/{id}
```

Compliance Gate 端点只读。

## 17.2 设置

```http
GET  /settings/public
PUT  /settings/public

GET  /metadata/providers
GET  /metadata/providers/{id}
POST /metadata/providers/{id}/test
PUT  /metadata/providers/{id}/credential
DELETE /metadata/providers/{id}/credential
PUT  /metadata/providers/{id}/proxy
DELETE /metadata/providers/{id}/proxy

POST /settings/ai/test
PUT  /settings/ai/credential
DELETE /settings/ai/credential
```

公开设置响应不得包含完整 Secret。

若用户尝试同时启用 TMDB 与 AI，且 Gate 未通过：

```http
409 Conflict
```

错误码：

```text
COMPLIANCE_PROVIDER_AI_COMBINATION_BLOCKED
```

## 17.3 媒体库

```http
GET    /library-roots
POST   /library-roots
PATCH  /library-roots/{id}
DELETE /library-roots/{id}
POST   /library-roots/{id}/scan
```

删除 Library Root 只删除索引，不删除磁盘文件。

## 17.4 资产

```http
GET /media-assets
GET /media-assets/{id}
GET /media-assets/{id}/probe
GET /media-assets/{id}/parsed-identities
GET /media-assets/{id}/candidates
POST /media-assets/{id}/resolve
POST /media-assets/{id}/confirm-match
POST /media-assets/{id}/clear-match
```

候选 API 是普通确定性 API，不属于 AI 工具。

## 17.5 Metadata Queue

```http
GET  /metadata/queue
GET  /metadata/queue/{id}
POST /metadata/queue/{id}/retry
POST /metadata/queue/{id}/cancel
```

## 17.6 计划

```http
POST /plans
GET  /plans
GET  /plans/{id}
POST /plans/{id}/approve
POST /plans/{id}/reject
POST /plans/{id}/expire
```

v0.1.1 不提供：

```http
POST /plans/{id}/execute
```

## 17.7 Jobs

```http
GET /jobs
GET /jobs/{id}
GET /jobs/{id}/events
POST /jobs/{id}/cancel
```

## 17.8 Agent

```http
POST /agent/query
POST /agent/create-plan-request
```

AI 输出必须经 JSON Schema 和业务校验。

---

# 18. AI Provider 接口

Go 接口参考：

```go
type Provider interface {
    Probe(
        ctx context.Context,
        cfg Config,
    ) (Capabilities, error)

    Complete(
        ctx context.Context,
        req CompletionRequest,
    ) (*CompletionResponse, error)

    CompleteWithTools(
        ctx context.Context,
        req ToolCompletionRequest,
    ) (*ToolCompletionResponse, error)
}

type Capabilities struct {
    Streaming        bool `json:"streaming"`
    ToolCalling      bool `json:"tool_calling"`
    StructuredOutput bool `json:"structured_output"`
    Vision           bool `json:"vision"`
}
```

## 18.1 能力探测

用户点击“测试模型”后：

1. 普通文本；
2. JSON Object；
3. JSON Schema；
4. 单工具调用；
5. 多轮工具调用；
6. 流式输出。

输出：

```json
{
  "connected": true,
  "model": "qwen3",
  "capabilities": {
    "streaming": true,
    "tool_calling": true,
    "structured_output": false
  },
  "mode": "limited_agent"
}
```

不得假设 `/models` 一定可用。

## 18.2 降级策略

```text
无 Tool Calling
→ 只允许自然语言查询，不允许工具模式

无 Structured Output
→ 使用 JSON Object + 本地 Schema 校验
→ 校验失败则拒绝，不猜测修复

Streaming 与 Tools 不兼容
→ 工具请求关闭 Streaming

模型完全不可用
→ AI 功能关闭
→ 核心功能继续可用
```

## 18.3 Base URL

- 用户填写完整 API 根地址；
- 只清除末尾多余 `/`；
- 不擅自添加或删除 `/v1`；
- 测试具体协议端点；
- 支持自定义 Header；
- Secret Header 脱敏；
- TLS 校验默认开启；
- AI 无权修改 Base URL；
- AI Base URL 与 Metadata Provider Endpoint 完全独立。

## 18.4 Provider 内容隔离测试

每次 AI 请求前必须执行 Context Policy：

```go
ValidateAgentContext(context) error
```

若发现以下字段，立即拒绝：

- `provider_item_id`
- `provider_raw_payload`
- `provider_title`
- `provider_overview`
- `provider_cast`
- `provider_artwork`
- `tmdb_*`
- `tvdb_*`
- `bangumi_*`

同时生成 `agent_context_audits` 记录。

---

# 19. Web UI 页面

v0.1.1 页面：

1. 首次配置向导；
2. Dashboard；
3. Library Roots；
4. 扫描任务；
5. 媒体资产列表；
6. 资产详情；
7. Provider 候选确认；
8. 命名预览；
9. Action Plan 列表；
10. Action Plan 详情；
11. Metadata Provider 设置；
12. Provider Queue；
13. AI 设置；
14. 隐私设置；
15. Compliance Gate 状态；
16. 日志与诊断；
17. 关于、署名与许可证。

## 19.1 首次配置向导

步骤：

```text
欢迎
→ 数据目录
→ 媒体目录
→ Metadata Provider
→ Provider 代理，可跳过
→ AI，可跳过
→ Compliance 检查
→ 隐私模式
→ 运行首次扫描
```

若 TMDB 已启用，AI 页面必须显示：

```text
当前合规 Gate 阻止同时启用 TMDB 与 AI。
```

## 19.2 资产列表核心列

```text
原文件名
解析标题
年份
时长
Provider 状态
最高候选
匹配分数区间
匹配状态
计划状态
```

## 19.3 候选确认页

必须展示：

- 原始文件名；
- 清洗后的标题；
- 文件时长；
- Provider；
- 候选海报；
- 候选标题；
- 原始标题；
- 年份；
- 时长；
- Provider ID；
- 排序分；
- 分数区间；
- 每组证据；
- 被丢弃的相关证据；
- 风险警告；
- 确认、拒绝、搜索其他候选。

该页面由确定性 API 提供，不经过 AI。

## 19.4 Compliance 页面

展示：

- Gate ID；
- 状态；
- 原因；
- 当前禁止组合；
- 允许组合；
- 最后更新时间；
- 文档链接；
- 不提供用户侧“忽略风险”按钮。

---

# 20. 可观测性

## 20.1 日志

结构化日志字段：

```text
timestamp
level
component
request_id
job_id
media_asset_id
plan_id
provider
event
duration_ms
error_code
```

禁止记录：

- API Key；
- Authorization Header；
- 代理密码；
- 完整视频路径，除非本地调试模式且用户主动开启；
- AI 请求完整内容，除非用户主动开启调试；
- Provider 原始响应；
- 视频或字幕内容。

## 20.2 错误码

前缀：

```text
CFG_
FS_
SCAN_
PROBE_
PARSE_
META_
TMDB_
MATCH_
PLAN_
AI_
DB_
SEC_
COMPLIANCE_
```

示例：

```text
META_PROVIDER_UNAVAILABLE
META_QUEUE_ATTEMPTS_EXHAUSTED
TMDB_AUTH_INVALID
TMDB_RATE_LIMITED
PROBE_FFPROBE_NOT_FOUND
FS_PATH_OUTSIDE_ROOT
PLAN_INPUT_CHANGED
AI_TOOL_CALL_INVALID
AI_CONTEXT_PROVIDER_CONTENT_DETECTED
COMPLIANCE_PROVIDER_AI_COMBINATION_BLOCKED
DB_BACKUP_FAILED
```

## 20.3 指标

首版本地指标：

- 扫描文件数；
- Probe 成功率；
- Parser 状态分布；
- Provider 请求数；
- 缓存命中率；
- Queue 长度；
- 重试次数；
- Resolver 分数区间分布；
- 人工确认率；
- AI 请求数；
- AI Schema 校验失败数；
- Compliance 拦截次数。

默认不上传。

---

# 21. 测试策略

## 21.1 单元测试

覆盖：

- 标题归一化；
- CJK 标题；
- 年份解析；
- 技术标签清理；
- 目标路径生成；
- 非法字符处理；
- 分阶段评分；
- 相关证据去重；
- 冲突惩罚；
- 配置覆盖；
- Secret 脱敏；
- 路径越界；
- Action Plan 失效；
- Compliance Gate；
- AI Context Policy；
- Provider Queue 退避；
- Provider 请求 fingerprint；
- SQLite 备份。

## 21.2 测试语料

至少 200 个真实或仿真的电影文件名，覆盖：

- 英文；
- 简体中文；
- 繁体中文；
- 日文；
- 韩文；
- 同名不同年份；
- 标题包含数字；
- 年份缺失；
- 多个年份；
- 发行组；
- 版本标签；
- HDR / DV；
- 2160p / 1080p；
- 点、下划线、空格；
- 父目录提供标题；
- 文件名混乱；
- 样片；
- 花絮；
- 重复文件。

语料拆分：

```text
development
heldout
```

Held-out 不得用于规则调参。

## 21.3 集成测试

- 临时目录扫描；
- `ffprobe` 可用与不可用；
- SQLite Migration；
- SQLite WAL 备份恢复；
- Mock Provider；
- Local NFO Provider；
- TMDB Mock；
- 代理成功与失败；
- 429；
- 网络断开；
- Provider Queue；
- 最大重试；
- 任务取消；
- 重复扫描；
- 文件扫描期间变化；
- 大小写路径；
- Unicode 路径；
- AI Mock；
- AI 无 Tool Calling；
- AI 无 Structured Output；
- AI Context 注入 Provider 字段；
- TMDB + AI 同时启用被阻止。

## 21.4 Golden Tests

以下输出采用 Golden File：

- 文件名解析；
- 候选排序；
- 证据分组；
- Action Plan JSON；
- 命名预览；
- API Schema；
- 隐私脱敏；
- AI AgentView；
- Compliance 错误响应。

修改 Golden File 必须在 PR 中解释原因。

## 21.5 安全测试

v0.1.1：

- 目录穿越；
- 软链接逃逸；
- Secret 日志泄露；
- 恶意文件名 Prompt Injection；
- 恶意 NFO；
- Provider 响应注入；
- AI 工具参数越权；
- Compliance Gate 绕过；
- CSRF；
- 未授权局域网访问；
- 诊断包脱敏。

v0.2 追加：

- TOCTOU；
- 硬链接；
- UNC；
- Windows 长路径；
- 大小写碰撞；
- 跨盘中断；
- 进程崩溃恢复。

---

# 22. v0.1.1 验收标准

全部满足才允许发布 Alpha。

## 22.1 功能

- 可添加只读电影目录；
- 可启动扫描；
- 可识别支持的视频扩展名；
- 可调用 `ffprobe`；
- 可解析标题和年份；
- 支持 CJK 电影文件名；
- Metadata Provider 接口存在；
- Mock Provider 可用；
- Local NFO Provider 可用；
- TMDB Adapter 可配置；
- TMDB 支持代理；
- Provider 缓存可用；
- Provider Queue 可用；
- Provider 失败不阻断扫描；
- 可展示至少 5 个候选；
- 可展示分组评分证据；
- 可人工确认；
- 无自动确认；
- 可生成 Jellyfin 风格命名预览；
- 可生成 Action Plan；
- 可配置 AI 或完全关闭 AI；
- AI 可处理本地资产查询与 Dry Run Plan 请求；
- AI 能力探测可用；
- 所有数据在本地保存；
- 不需要项目方账号。

## 22.2 安全与合规

- v0.1.1 无真实文件写入 API；
- 后端无任意 Shell 接口；
- AI 无任意路径工具；
- AI 无 Provider 工具；
- AI Context 不包含 Provider 内容；
- 每次 AI 请求有 Context Audit；
- Secret 不进入日志；
- API 不返回完整 Secret；
- 路径必须位于配置根目录；
- 诊断导出自动脱敏；
- 默认无遥测；
- 默认绑定 `127.0.0.1`；
- 跨站请求有基础防护；
- 设置修改有审计事件；
- `COMPLIANCE-TMDB-AI` 默认 blocked；
- TMDB 与 AI 同时启用会被代码阻止；
- 用户设置不能绕过 Gate；
- TMDB 署名与非背书声明已加入；
- Provider 条款风险文档存在。

## 22.3 质量

- 核心包单元测试通过；
- 数据库 Migration 可重复应用；
- 数据库备份可恢复；
- 重复扫描不产生重复资产；
- 200 条语料建立；
- Held-out 未参与调参；
- 常见模式标题准确率达到目标；
- 评分结果可复现；
- 相关标题信号不重复计分；
- 所有 API 有错误码；
- Docker 启动成功；
- README 可让新用户在 15 分钟内完成首次扫描；
- Windows、Linux 至少各完成一次手工验证；
- 不存在阻断级已知 Bug。

---

# 23. 发布门槛

## 23.1 Alpha Core

允许：

- 开发者使用；
- 数据库结构变化；
- Docker 与源码运行；
- 不承诺旧数据兼容。

要求：

- 不修改媒体文件；
- 核心链路可用；
- Mock / Local NFO / TMDB Adapter 可用；
- AI 可不包含在 Alpha Core 演示中；
- Secret 安全；
- Provider 故障可降级；
- Compliance Gate 生效。

## 23.2 Alpha Assistant

除 Alpha Core 要求外：

- AI 能力探测通过；
- AI Context Policy 通过；
- 无 Provider 内容进入 AI；
- TMDB 启用时 AI 被阻止；
- Local NFO 或无 Provider 模式下 AI 可用；
- 所有工具调用经 Schema 校验；
- AI 失败不影响核心功能。

## 23.3 Beta

要求：

- Migration 稳定；
- 设置导入导出；
- Windows / Linux 安装说明；
- 解析语料超过 500 条；
- Provider Adapter 契约测试；
- Queue 故障恢复；
- API 初步稳定；
- 安全审查完成；
- 品牌和商标复核；
- 第三方许可证清单；
- 合规 Gate 状态公开。

## 23.4 解除 TMDB + AI Gate 的发布要求

必须具备：

- 明确的依据；
- 合并 ADR；
- 更新本文档；
- 更新 Context Policy；
- 更新威胁模型；
- 更新测试；
- 更新用户文档；
- 更新宣传文案；
- 维护者签署决定记录。

没有这些材料，不得解除。

---

# 24. 第一批 GitHub Issues

## Epic A：项目骨架

### A-01 初始化 Monorepo

验收：

- Go Server 可启动；
- React Web 可启动；
- Makefile 可用；
- CI 执行 lint 和 test；
- AGPL-3.0 LICENSE 存在。

### A-02 配置加载

- YAML；
- 环境变量覆盖；
- Schema 校验；
- Secret 不回写；
- Compliance 配置不可由普通用户覆盖。

### A-03 SQLite 与 Migration

- `modernc.org/sqlite`；
- WAL；
- 单 Writer；
- 初始表；
- Migration 版本。

### A-04 SQLite 备份与恢复

- Checkpoint；
- `VACUUM INTO`；
- 恢复测试；
- 轮换策略。

### A-05 Secret Store

- Docker Secret；
- 环境变量；
- Keychain；
- 日志脱敏。

## Epic B：扫描

### B-01 Library Root CRUD
### B-02 文件扫描
### B-03 稳定文件检测
### B-04 重复扫描幂等
### B-05 Job 进度和取消
### B-06 Provider 故障不影响扫描

## Epic C：媒体探测

### C-01 ffprobe 可用性检测
### C-02 解析 ffprobe JSON
### C-03 超时和错误状态
### C-04 Probe 缓存
### C-05 FFmpeg 许可证说明

## Epic D：文件名解析

### D-01 标题归一化
### D-02 年份识别
### D-03 技术标签清洗
### D-04 版本标签识别
### D-05 CJK 比较键
### D-06 Development Corpus
### D-07 Held-out Corpus
### D-08 字段级准确率报告

## Epic E：Metadata Provider

### E-01 Provider Interface
### E-02 Provider Registry
### E-03 Mock Provider
### E-04 Local NFO Provider
### E-05 TMDB Adapter
### E-06 Provider Proxy
### E-07 Provider Cache
### E-08 请求合并
### E-09 Offline Retry Queue
### E-10 Provider 契约测试
### E-11 TMDB 署名页

## Epic F：Resolver

### F-01 硬约束
### F-02 标题证据组
### F-03 相关信号去重
### F-04 辅助证据组
### F-05 冲突惩罚
### F-06 分数区间
### F-07 证据模型
### F-08 人工确认
### F-09 清除与重新匹配
### F-10 Resolver Golden Tests

## Epic G：命名与计划

### G-01 Jellyfin 命名模板
### G-02 通用 ProviderID 字段
### G-03 文件名清洗
### G-04 目标冲突预检查
### G-05 Action Plan
### G-06 Plan 失效检测
### G-07 前后对照导出

## Epic H：AI

### H-01 OpenAI-compatible Provider
### H-02 能力探测
### H-03 AgentView
### H-04 Context Policy
### H-05 Context Audit
### H-06 受限工具 Schema
### H-07 自然语言筛选
### H-08 Dry Run Plan Request
### H-09 AI 完全关闭模式
### H-10 能力降级
### H-11 Provider 内容注入测试

## Epic I：Compliance

### I-01 Compliance Gate Framework
### I-02 `COMPLIANCE-TMDB-AI`
### I-03 运行时互斥
### I-04 Gate 只读 API
### I-05 Compliance UI
### I-06 TMDB 风险文档
### I-07 宣传文案检查
### I-08 Gate 解除流程

## Epic J：Web

### J-01 首次配置向导
### J-02 Dashboard
### J-03 媒体资产列表
### J-04 候选确认页
### J-05 命名预览页
### J-06 Plan 详情
### J-07 Provider 设置
### J-08 Provider Queue
### J-09 AI 设置
### J-10 Compliance 状态
### J-11 日志与诊断

## Epic K：发布

### K-01 Dockerfile
### K-02 Docker Compose 示例
### K-03 README 快速开始
### K-04 SECURITY.md
### K-05 CONTRIBUTING.md
### K-06 Third-party Notices
### K-07 Alpha Core 检查表
### K-08 Alpha Assistant 检查表
### K-09 品牌复核

---

# 25. 两周 Sprint 0

Sprint 目标：

> 完成不依赖 AI 的只读纵向链路，并把 Provider 抽象、缓存、队列和 Compliance Gate 放进基础架构。

## 第 1–2 天

- 初始化仓库；
- Go Server；
- React；
- CI；
- `modernc.org/sqlite`；
- Migration；
- `/health`；
- 配置加载；
- Compliance Gate 骨架。

## 第 3–4 天

- Library Root；
- 文件扫描；
- 媒体资产入库；
- Job 状态；
- 基础资产列表；
- Provider 故障状态模型。

## 第 5–6 天

- `ffprobe`；
- Probe JSON；
- 资产详情；
- 超时和错误；
- Probe 缓存。

## 第 7–8 天

- 文件名归一化；
- 年份解析；
- 技术标签；
- CJK 比较键；
- Development Corpus 100 条；
- Held-out 框架。

## 第 9–10 天

- Metadata Provider Interface；
- Provider Registry；
- Mock Provider；
- Local NFO Provider；
- Provider Cache；
- Provider Queue。

## 第 11–12 天

- TMDB Adapter；
- 连接测试；
- 代理；
- 电影搜索；
- 官方 Endpoint 限制；
- `COMPLIANCE-TMDB-AI` 运行时互斥。

## 第 13–14 天

- 分阶段候选排序；
- 证据去重；
- 人工确认；
- 命名预览；
- Action Plan；
- Dry Run 页面；
- Alpha Core 演示；
- README。

Sprint 0 不因 AI UI 延迟核心链路。

AI 进入 Sprint 1，不是 Sprint 0 Alpha Core 的阻断项。

---

# 26. Definition of Done

一个 Issue 只有满足以下条件才可关闭：

- 实现代码已合并；
- 有对应测试，或明确说明无法自动测试的原因；
- 错误路径已处理；
- 日志无 Secret；
- API 或配置变更已更新文档；
- 数据库变更有 Migration；
- Provider 变更通过契约测试；
- UI 包含加载、空状态和错误状态；
- Compliance 影响已评估；
- 不引入 v0.1.1 范围外功能；
- CI 通过；
- 验收条件逐项确认。

若 Issue 涉及 AI，还必须：

- Context Policy 测试通过；
- 无 Provider 内容；
- 工具权限不扩大；
- Schema 校验覆盖失败路径。

---

# 27. 代码规范

Go：

- `gofmt`；
- `go vet`；
- 静态检查；
- 包职责单一；
- 错误 wrap 并保留错误码；
- 禁止用 `panic` 处理业务错误；
- Context 贯穿网络、数据库和外部进程；
- 所有外部调用有 Timeout；
- 数据库操作有明确事务边界；
- Provider Adapter 不泄露特有类型到 Resolver；
- AI Package 不得 import Provider 实现包；
- Compliance Package 不得被 UI 配置直接修改。

可通过依赖边界测试或静态检查禁止：

```text
internal/ai -> internal/metadata/providers/*
internal/ai -> internal/secrets
```

TypeScript：

- `strict`；
- API 类型由 OpenAPI 或共享 Schema 生成；
- 禁止无理由使用 `any`；
- 异步请求有取消与错误状态；
- Secret 输入框不可回显；
- Compliance 阻断必须显示明确原因；
- UI 不提供绕过按钮。

通用：

- 数据库时间统一 UTC；
- API 输出 ISO 8601；
- ID 使用 UUIDv7 或同类有序 ID；
- JSON 字段使用 snake_case；
- 配置字段保持稳定；
- 未实现能力返回明确错误；
- 不静默降级安全规则。

---

# 28. ADR 初始记录

## ADR-001：后端使用 Go

状态：Accepted

原因：

- 单二进制；
- NAS 友好；
- 文件系统与并发任务适配；
- 跨平台；
- 部署简单。

## ADR-002：SQLite + modernc.org/sqlite

状态：Accepted

原因：

- 本地单用户；
- 无数据库运维；
- cgo-free 交叉编译；
- WAL 支持并发读取；
- 初期规模足够。

约束：

- 单 Writer；
- `VACUUM INTO` 备份；
- 性能问题必须先 Profile，再考虑替换驱动。

## ADR-003：AI 为可选增强

状态：Accepted

后果：

- 无 AI 时核心功能完整；
- 核心匹配和计划必须确定性；
- AI 只调用受限本地工具。

## ADR-004：v0.1.1 禁止真实文件写入

状态：Accepted

原因：

- 优先验证识别、匹配与计划；
- 降低数据损坏风险；
- 执行器独立设计。

## ADR-005：用户自带 Provider 与 AI 凭证

状态：Accepted

后果：

- 项目不托管密钥；
- 不承担第三方 API 成本；
- Secret 管理是核心能力；
- BYOK 不替代第三方条款合规。

## ADR-006：主项目使用 AGPL-3.0

状态：Accepted

原因：

- 保持网络服务修改的开源回馈；
- 保护社区成果；
- 允许自由自托管。

## ADR-007：Metadata Provider Interface-First

状态：Accepted

原因：

- 避免 TMDB 单点绑定；
- 支持本地 NFO；
- 支持网络故障降级；
- 为未来 Provider 留出稳定边界。

## ADR-008：Provider 与 AI 默认隔离

状态：Accepted

约束：

- AI 不读取 Provider 内容；
- AI 不 import Provider Adapter；
- Provider 候选解释由确定性 Resolver 完成；
- Context Audit 强制记录。

## ADR-009：TMDB + AI 组合受 Compliance Gate 控制

状态：Accepted

默认：

```text
blocked
```

解除需要正式依据、ADR、测试和文档。

## ADR-010：候选采用分阶段排序

状态：Accepted

原因：

- 避免相关证据重复计分；
- 保留可解释性；
- 分数只表示排序；
- v0.1.1 不自动确认。

## ADR-011：Provider 代理、缓存与离线队列进入 v0.1.1

状态：Accepted

原因：

- 网络可用性不可假设；
- Provider 不应阻断扫描；
- 重试必须有界；
- 中国大陆用户是重要目标群体。

## ADR-012：项目名称采用 ReelWarden（影卫）

状态：Accepted，待发行前复核

约束：

- Repo、CLI 和 Docker 名称统一为 `reelwarden`；
- 首次公开发行前复核商标、域名和主要包注册表；
- 暂不因命名讨论阻塞开发。

---

# 29. 变更控制

以下变更必须修改本文档并通过 PR：

- 增加或删除 v0.1.1 功能；
- 开启真实文件写入；
- 变更许可证；
- 变更核心技术栈；
- 修改 API 兼容承诺；
- 修改 Secret 存储；
- 扩大 AI 权限；
- 允许 AI 读取 Provider 内容；
- 修改默认隐私策略；
- 修改匹配门槛；
- 开启自动确认；
- 引入新的外部 Provider；
- 修改 Compliance Gate；
- 增加遥测；
- 增加云端账户或中央服务。

PR 必须包含：

```text
变更原因
用户价值
安全影响
合规影响
兼容影响
迁移方案
测试方案
回滚方案
```

---

# 30. 开工指令

严格按以下顺序执行：

```text
1. 创建 reelwarden 仓库
2. 添加 LICENSE、README、SECURITY、CONTRIBUTING
3. 初始化 Go 和 React
4. 实现配置、Compliance Gate 与 SQLite Migration
5. 实现 Library Root 与只读扫描
6. 实现 ffprobe
7. 建立 Development / Held-out 解析语料
8. 实现 Metadata Provider Interface
9. 实现 Mock 和 Local NFO Provider
10. 实现 Provider Cache、Proxy 与 Retry Queue
11. 实现 TMDB Adapter
12. 实现 TMDB + AI 运行时互斥
13. 实现分阶段候选排序与证据去重
14. 实现人工确认
15. 实现命名预览
16. 实现 Action Plan
17. 完成 Alpha Core
18. 实现可选 AI、AgentView 与 Context Audit
19. 完成 Alpha Assistant
20. 才开始设计真实文件执行器
```

任何工作若不能直接推动上述链路，默认放入 Backlog。

---

# 31. 当前成功标准

第一个成功不是“Agent 能聊天”，而是：

> 给定一个真实电影目录，ReelWarden 能稳定索引文件，在 Provider 暂时不可用时继续完成本地工作；当 Provider 可用时给出可解释候选，由用户人工确认，并生成一份可信的 Dry Run 改名计划，同时不改动任何媒体文件。

第二个成功是：

> 在不向 AI 暴露 Provider 内容、不授予文件写权限、且通过 Compliance Gate 的前提下，AI 能可靠地把自然语言转成受限的本地查询和计划请求。

---

# 32. 本地启动目标

源码：

```bash
git clone <repository>
cd reelwarden

cp config.example.yaml config.yaml

make dev
```

Docker：

```bash
docker compose up -d
```

打开：

```text
http://127.0.0.1:8787
```

首次进入：

```text
添加媒体目录
→ 选择 Metadata Provider
→ 配置代理，可跳过
→ 填写 Provider 凭证，可跳过
→ AI 可跳过
→ 检查 Compliance 状态
→ 开始扫描
→ 审核候选
→ 查看 Dry Run Plan
```

---

# 33. 合规与证据登记

项目必须维护以下文件：

```text
docs/compliance/TMDB_AI_GATE.md
docs/compliance/THIRD_PARTY_PROVIDER_MATRIX.md
docs/compliance/FFMPEG_BUILD_LICENSE.md
docs/compliance/ATTRIBUTIONS.md
docs/reviews/REVIEW-001-independent-technical-review.md
```

## 33.1 Provider Matrix 最小字段

```text
Provider
Terms version/date
Authentication
Attribution
Commercial restrictions
AI/ML restrictions
Caching restrictions
Redistribution restrictions
User-agent requirements
Network availability notes
Project decision
Last reviewed date
```

## 33.2 风险表述

本文档不是法律意见。

对条款存在不确定性时，默认策略是：

```text
不假设允许
→ 限制数据流
→ 代码 Gate
→ 寻求书面澄清
→ 形成 ADR
```

---

# 34. 版本历史

## v0.1.0

- 初始执行基线；
- Go + SQLite + React；
- TMDB P0；
- AI 可选；
- v0.1 只读。

## v0.1.1

- Provider Interface-First；
- 合规 Gate；
- Provider 与 AI 隔离；
- TMDB 与 AI 默认互斥；
- 代理、缓存和离线队列进入首版；
- 分阶段评分；
- 取消自动确认；
- `modernc.org/sqlite`；
- CJK held-out 语料；
- ReelWarden 正式命名。

---

**本文档结束。**

变更本文档即变更项目执行基线。未写入本文档或已合并 ADR 的新需求，默认不属于当前开发承诺。
