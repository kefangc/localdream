# 测试、验收与交付规范

## 测试分层

| 层次 | 验证内容 | 隔离方式 |
| --- | --- | --- |
| 领域单元 | min/max、总数、模式、错误码、取消策略 | 小结构体、fake provider |
| 外部协议 | tuple、缺失值、非法 JSON、技术槽位映射 | 合成 JSON + 已脱敏真实 schema fixture |
| HTTP Client | 方法、路径、状态码、读取上限、错误链、context | httptest 或小型自定义 RoundTripper |
| 产品 API | DTO、参数检查、错误到状态码映射、无副作用拒绝 | httptest + fake 服务/仓储 |
| 存储和 worker | 事务、状态迁移、并发取消、重启核对 | t.TempDir SQLite、可控 fake executor |
| 前端 | 本次选择计数、模式切换、离线提示、错误恢复 | 按需组件测试 + 浏览器实际操作 |
| 联机和生成 | 当前安装节点兼容、真实输出、音频绑定 | 显式启用，使用授权素材和有限 GPU 预算 |

普通 go test 不依赖 8188、用户 data/assets 或硬编码 asset_1。先检查既有联机测试是否有文件复制/提交副作用，再决定运行。

## 高价值用例

- max=0 是明确禁止，不是缺失；missing/null/非法类型不能静默变成“无限制”。
- 检查有效最小/最大边界、恰好等于限制、超过限制、空选择、负数。
- Catalog 不误吞调用者取消；探测超时与父请求超时分别测试。
- 失败测试不只判断 err != nil，检查错误类别/字段或稳定上下文，防止拼错 fixture 造成假阳性。
- 公开方法错误包装如需保留底层类型，使用 errors.As/Is 测试；strings.Contains 只能证明文本，不证明 %w 错误链。
- tuple 空数组/单元素不会 panic；未知 schema 和缺失 optional 槽位按支持策略处理。
- fake provider 返回更大技术上限时，不放开模式禁止的素材或未实现的音频绑定。
- 超限请求不复制文件、不入队、不发 POST /prompt；用 fake 调用次数验证。
- 测试调用者改变传给 NewCatalog 的 map 后是否会影响目录；明确不可变配置或复制所有权。
- worker 用 channel/可控屏障协调，不用任意 sleep 猜测顺序。

## TDD 的适用边界

新业务规则和故障修复优先 Red-Green-Refactor。常规库调用允许实现后补测，回归测试可直接 PASS。无需为了流程主动破坏正确代码。

不要追求 100% 覆盖率作为交付门槛；关注会导致错误生成、数据丢失、重复提交、取消无效和信息泄露的路径。私有辅助函数可以测试，但保留公开入口和真实流程测试。

## 常用验证命令

在 PowerShell 中，从明确目录运行：

```powershell
Set-Location E:\tools\ComfyUI\app
go test ./... -count=1
go vet ./...
```

代码格式化仅指定本轮改动文件，例如：

```powershell
gofmt -w internal\workflow\catalog.go internal\workflow\catalog_test.go
```

不要依赖 PowerShell 为 gofmt 展开 `*.go`，也不要格式化无关工作区。

前端发生变更后：

```powershell
Set-Location E:\tools\ComfyUI\app\frontend
npm run typecheck
npm run format:check
npm run build
```

这些命令不能替代打开页面实际操作。当前本地端口和依赖是否可用应现场确认。

worker 阶段可尝试 `go test -race ./...`；Windows 工具链缺失造成无法运行时记录限制，不能把未运行当通过。联机测试使用显式开关且在 finally 恢复原环境变量，不让后续普通测试意外访问真实执行端。

## Git 与迁移

- 每个可验证切片一个提交；工作中的 Red 可以暂存，但提交检查点需说明测试状态。
- 提交前分别检查 git diff 和 git diff --cached；AM 表示暂存区未必包含最新内容。
- 只 stage 本轮文件，不 git add . 收进媒体、密钥、二进制或其他工作。
- 先审查旧实现、固定必要行为，再接入新模块；确认消费者迁移后删除重复逻辑。
- 不重置用户工作、不自动恢复旧音频回退 stash、不为修拼写强制重写历史。

## 完成定义

一个功能标记完成必须同时满足：

1. 对应用户行为可用，不只是函数存在。
2. 用户能解释输入输出、错误策略和副作用。
3. 有针对性自动测试，旧行为无意外回退。
4. API/前端/生成相关变化完成相应集成验证。
5. 日志和错误信息足够排查问题，不输出 API Key 或大段上游隐私内容。
6. 文档注明验证环境、范围和仍未验证项。

## 真实使用记录模板

```text
日期 / commit / ComfyUI 版本：
模板 / 模式 / 模型 / seed：
素材类型与绑定角色（不提交私有媒体）：
请求参数 / job_id / prompt_id：
预期行为：
实际结果与输出位置：
是否人工播放检查：
音频是否单独验证：
故障和未验收事项：
```

错误排查顺序：首条编译错误 -> 类型/作用域 -> 测试数据合法性 -> 正确失败原因 -> HTTP -> 图编译 -> 实际模型执行。不要一看到测试绿就跳过后两层。
