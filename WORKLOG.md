# WORKLOG

## 现状

**分支：** `experiment/go-core-phase2b`
**阶段：** Go-only resume baseline 已加固，等待最终 review

Go CLI spike (`core-go/`) 已实现：
- 音频切割（ffmpeg，含减半重试）
- MiMo ASR 转写（含重试、跳过已完成）
- Transcript 合并
- notes_input.md 生成（notes/minutes 模式）
- DeepSeek 会议纪要生成（minutes 模式）
- Obsidian 模板渲染（minutes 模式）
- `.env` 加载（支持 `--env-file` / `--project-root`）
- Flag 解析支持任意位置
- **Resume 命令** — 完整状态机：load+validate manifest → 独占锁 → 磁盘 reconcile → 最早未完成阶段幂等执行
- **Manifest 支持** — `task_manifest.json`（schema_version=1），嵌套 chunk 逐条状态和 artifact 状态
- **Prompt/Template 标志** — `--prompt-path` 和 `--obsidian-template-path`
- **原子写入** — 所有生成文件通过 temp+rename 写入
- **独占锁** — `.resume.lock` 使用平台原生文件锁，进程退出后由操作系统自动释放
- 150 个 Go 单元测试全通过（含 race detector）
- Python 114 个测试无回归

## 当前任务

- [x] 初始 spike 实现（transcript 模式）
- [x] Review fixes: flag 解析、`--env-file`、测试补充
- [x] Hardening: ffmpeg 跨平台提示、snapshot test、env/slug/stdout contract 测试
- [x] notes/minutes 模式扩展
- [x] Review fixes: gofmt、template error handling、write error check、WORKLOG 修正
- [x] Phase 1: resume 命令、manifest 支持、prompt/template 标志
- [x] Review fixes: gofmt、chunk 严格三位编号校验、README 更新
- [x] Resume state machine 重设计（state.go、artifact.go、tasklock.go、reconcile.go、manifest 重写、resume 重写）
- [x] Codex review：修复 hashless transcript 信任与 re-split 静默清理失败
- [ ] 等待 Go-only baseline 最终 review
- [ ] 可选：端到端手动验证（需要真实 API key + 音频文件）

---

## 历史

### 2026-07-09

**Session 1 — 初始 spike 实现**
- 创建 `experiment/go-core-spike` 分支
- 实现 `core-go/` 目录：config, taskdir, splitter, mimo, merger, main
- 24 个 Go 测试通过，Python 111 测试无回归
- 零外部 Go 依赖

**Session 2 — Review fixes**
- 修复 flag 解析：`reorderArgs()` 支持 flag 在 audio path 前后
- 添加 `--env-file` 和 `--project-root` 参数
- 补充 7 个 flag 解析测试（总计 31 个）
- README 文档化 `.env` 查找行为和 Tauri 集成限制

**Session 3 — Hardening**
- ffmpeg missing error 改为跨平台提示（macOS/Ubuntu/Windows）
- 新增 transcript output snapshot test
- 新增 .env parsing 测试：双引号、单引号、注释、空行、负值、零值
- 新增 SafeSlug 测试：中文、特殊字符、空格
- 新增 stdout contract test
- 新增 CLI error case 测试
- 总测试数：31 → 50

**Session 4 — notes/minutes 模式扩展**
- 创建 `experiment/go-core-minutes` 分支（基于 `experiment/go-core-spike`）
- 扩展 config.go：DeepSeek 设置（API key、endpoint、model、timeout、prompt/template 路径）
- 实现 notes_input.go：prompt + transcript 拼接，格式对齐 Python
- 实现 deepseek.go：DeepSeek API 客户端（401/402/429/4xx/5xx/timeout/empty 全覆盖）
- 实现 obsidian.go：模板占位符替换（title、generated_at、generated_minutes、transcript、task_name）
- 更新 main.go：支持 transcript/notes/minutes 三种输出模式
- 更新 README：添加 notes/minutes 模式说明、环境变量表格
- 新增 28 个测试（总计 78 个）

**Session 5 — minutes review fixes**
- 应用 `gofmt`（cli_test.go、config.go、mimo.go）
- 移除 Obsidian template 失败时的 raw minutes fallback，改为返回错误
- 修复 `os.ReadFile` 和 `os.WriteFile` 错误检查
- 修正 WORKLOG.md 分支名（experiment/go-core-minutes → experiment/go-core-spike）

**Session 6 — Phase 1: resume/manifest 实现**
- 实现 `resume.go`：`resume <task_dir>` 命令，支持跳过已完成的转录
- 实现 `manifest.go`：任务状态跟踪（running/completed/failed）
- 添加 `--prompt-path` 和 `--obsidian-template-path` 标志
- 更新 main.go：添加 resume 命令和新标志
- 新增 resume_test.go：chunk 发现、格式验证、编号连续性测试
- 新增 manifest_test.go：manifest 创建、加载、状态更新测试
- 更新 config_test.go：路径解析测试
- 总测试数：78 → 84

**Session 7 — Phase 1 review fixes**
- 应用 `gofmt`（manifest.go、manifest_test.go、resume.go、resume_test.go）
- 修正 chunk 文件名校验：只接受严格三位编号（part_001.wav），拒绝 part_1.wav
- 更新 README.md：移除 "No resume"，添加 resume 功能说明
- 更新 WORKLOG.md：补充 resume/manifest 实现记录

**Session 8 — Resume state machine redesign**
- 参考 `docs/go_core_resume_state_machine_design.md`，重新设计 resume 架构

- **新增文件：**
  - `state.go`：TaskState 枚举（CREATED/VALIDATING_AUDIO/SPLITTING/TRANSCRIBING/MERGING/NOTES_GENERATING/MINUTES_GENERATING/VALIDATING_OUTPUTS/COMPLETED/FAILED）、ChunkState 枚举（PENDING/IN_PROGRESS/COMPLETED/FAILED）、ArtifactState 枚举、`ResumeStage()` 阶段选择逻辑
  - `artifact.go`：`WriteFileAtomically()` — temp file + fsync + atomic rename，所有生成文件使用此方式写入
  - `tasklock.go`：`AcquireTaskLock()`/`Release()` — `.resume.lock` 使用 Unix `flock` / Windows `LockFileEx` 原生独占锁
  - `reconcile.go`：`ReconcileChunks()` — 校验 chunk 音频和转写文件，重置 IN_PROGRESS/缺失/空文件为 PENDING；`ReconcileArtifacts()` — 校验产物并级联失效下游；`BuildChunkItemsFromDisk()` — legacy manifest 迁移用

- **重写文件：**
  - `manifest.go`：新 schema（schema_version=1），嵌套 Source/Chunks.Items/Artifacts/LastError，新增 `ChunkByID()`/`CountCompletedChunks()`/`ValidateManifest()`/`SetTaskState()`/`RecordTaskError()`/`MarkTaskCompleted()`
  - `resume.go`：完整重写 — 加载+校验 manifest → 获取锁 → 磁盘 reconcile → `ResumeStage()` 选最早未完成阶段 → 幂等执行各阶段（splitting/transcribing/merging/notes/minutes/validating）。支持 legacy manifest（schema_version=0）自动迁移
  - `main.go`：`runWorkflow` 使用新 manifest schema，splitting 后填充 Chunks.Items，transcription 时逐 chunk 更新状态，minutes 用 atomic write

- **测试覆盖（设计文档 8 项验收标准）：**
  1. 进程中断（chunk 5/10）→ 只重转缺失 chunk
  2. merge 失败 → 只重跑 merge
  3. DeepSeek 失败 → 只重跑 minutes
  4. 删除转写文件 → 级联失效下游，只重转该 chunk
  5. 已完成任务 → 无 API 调用
  6. manifest 计数器从磁盘 reconcile（含归零）
  7. 损坏 manifest → 拒绝不安全恢复，保留文件
  8. 并发 resume → 被锁拒绝

- 其他测试：锁竞争与释放、legacy manifest 迁移、ResumeStage 边界、ReconcileArtifacts 级联失效、WriteFileAtomically、BuildChunkItemsFromDisk
- 总测试数：150 个 Go 测试全通过（含 race detector），Python 114 个无回归

**Session 9 — Go-only baseline final hardening**
- hashless chunk transcript 不再被 reconcile 自动信任；正常转写在标记完成前持久化 SHA-256
- re-split、chunk 清理和重编号失败会中止任务，不再静默保留旧文件
- `ValidateManifest()` 增加 integrity version、重复 chunk ID、状态、计数、格式与哈希校验
- manifest error 记录保留 API 超时、429 与服务端错误的 retryable 属性
- 新增 Go Core GitHub Actions：test、vet、race 和 macOS/Linux/Windows 编译
