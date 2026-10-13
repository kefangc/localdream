# Local Dream MCP 接口

## 接入

- Endpoint：`http://127.0.0.1:8790/mcp`（跟随 `LOCALDREAM_ADDR` 的端口）。
- Transport：Streamable HTTP；官方 Go SDK `github.com/modelcontextprotocol/go-sdk`，版本固定于 `go.mod`。
- 与前端同一 Go 进程，复用 SQLite、任务编译、提交、队列跟踪、结果代理与取消服务。
- 外部 AI 的模型负责选择和调用工具。本服务不会在 MCP 调用中请求 `/responses` 或其他语言模型。
- 客户端填写 HTTP MCP 地址即可；各客户端具体配置字段以其文档为准。

服务使用无会话 Streamable HTTP 与 JSON 响应。没有 MCP 会话 ID 或独立 GET SSE 流；不支持的 GET/DELETE 返回 405。视频是异步业务任务，状态通过 `get_generation` 查询，与 HTTP 连接寿命无关。

支持版本由固定 SDK 实现，并通过 MCP 发现/初始化流程获取。兼容旧版客户端的 `initialize` 流程；新版客户端可使用 SDK 的能力发现。不要把 MCP 会话状态当作视频任务状态。

## 访问范围

默认只接受 loopback 客户端和 localhost/loopback Host；校验 Origin，拒绝跨站浏览器请求。
设置 `LOCALDREAM_MCP_TOKEN` 后，本机及远程 MCP 请求均需 `Authorization: Bearer <token>`。
远程使用还需显式监听可达地址，建议在可信网络或 HTTPS 代理后部署。反向代理应向上游传递 loopback Host，以兼容 SDK 的 localhost 防护，并验证外部 Origin；代理访问必须配置 token。

此 token 是本服务的预共享访问凭据，不是 OAuth 登录。持有者拥有整个本地工作室的 MCP 访问权。
`project_id` 是显式业务范围，防止误用素材，不是多用户权限系统。空字符串仅表示全局工作室，不表示所有项目。
现有 `/api/*` 与 `/media/*` 不会因为 MCP token 自动获得认证；不要据此把整个原有服务公开到不可信网络。

## 工具

所有工具提供 inputSchema、outputSchema 和只读/副作用/幂等 annotations。成功返回：

```json
{"ok": true, "data": {}}
```

业务错误返回 `isError: true`，并提供结构化结果：

```json
{"ok": false, "error": {"code": "request_id_conflict", "message": "...", "retryable": false}}
```

JSON Schema 校验错误由 SDK 返回 `isError: true` 和可读说明，可能没有同样的业务错误 envelope。
未知工具等协议错误遵循 MCP/JSON-RPC。客户端需先检查协议错误与 `isError`，再读取 `structuredContent`。

| 工具 | 输入重点 | 行为 |
| --- | --- | --- |
| `get_capabilities` | `{}` | 应用约束、提示词必需段落、模板与 prompt 节点探测；参考槽位成功探测时取编译器上限交集 |
| `list_projects` | 可选 `offset/limit` | 返回未删除项目；归档项目不接受新任务；全局工作室用空 project_id |
| `list_assets` | `project_id`，可选 `type/query/offset/limit` | 仅返回指定项目素材；默认 30 项、最大 100 项 |
| `read_prompt_guide` | `resource` | 读取 `SKILL.md`、`references/base-en.txt` 或 `references/ref-en.txt` |
| `prepare_generation` | 完整参数、`project_id/request_id` | 校验并持久化不可变草稿；不复制素材、不编译工作流、不启动 GPU |
| `get_generation_draft` | `project_id/draft_id` | 重连或重启后读取草稿及是否已经提交 |
| `submit_generation` | `project_id/draft_id/revision` | 用户授权范围内提交任务；同一草稿永远对应同一任务 |
| `get_generation` | `project_id/generation_id` | 真实任务状态、参数、进度是否已知、产物路径及建议轮询间隔 |
| `cancel_generation` | `project_id/generation_id` | 复用现有取消逻辑；已取消可重复调用；已完成/失败不能取消 |

工具读取素材元数据，并不分析图像或视频内容。现有素材上传仍使用产品界面或 HTTP API。

## 建议调用顺序

1. 读取 `get_capabilities`，必要时 `list_projects/list_assets`。
2. 读取提示词指南：先 `SKILL.md`，普通模式读 `base-en.txt`，多参考读 `ref-en.txt`。
3. 用客户端生成的唯一 `request_id` 准备草稿。
4. 用户已授权生成时直接提交；用户只要求方案时停留在草稿。
5. 保存 `draft_id/generation_id`，按 `poll_after_seconds` 查询。`queued/running` 不是完成。
6. 只有 `completed` 且 `output_url` 非空才交付视频。路径相对于 Local Dream 地址，而非 ComfyUI 地址。

以下为 `prepare_generation` 的工具参数示例；具体 JSON-RPC 封装由 MCP 客户端完成：

```json
{
  "project_id": "",
  "request_id": "video-rain-20261009-001",
  "mode": "text_to_video",
  "prompt": "integrated_multimodal_description: [Shot 1] A locked-off wide shot of rain falling on a quiet street at dusk.\n\noverall_soundscape: Steady rain and distant traffic.\n\nnon_diegetic_music: N/A",
  "duration": 7,
  "aspect_ratio": "16:9",
  "resolution": "0.75MP",
  "steps": 50,
  "reference_video_fps": 7,
  "asset_ids": []
}
```

随后将返回草稿的准确 ID 和 revision 交给 `submit_generation`：

```json
{"project_id": "", "draft_id": "<returned draft_id>", "revision": 1}
```

`request_id` 为 8–128 位字母、数字、连字符或下划线。相同项目中，同一 ID 加相同规范化参数返回原草稿，参数变化报冲突。草稿不可编辑；新方案使用新 ID。草稿保存默认参考 FPS=7、默认 steps=50 等最终参数。

一份草稿预留唯一 `generation_id`。提交超时先查询草稿/任务，再用相同草稿重试，不能通过新草稿“重试网络请求”。已失败/取消的任务再次提交原草稿仍返回原任务；确实要重新生成时创建新草稿。已删除的任务不会通过旧草稿重新生成。

## 时长、参考帧率与 MP

- `duration` 传用户原始需求，例如 7 秒传 `7`，不要让模型预先补成 `7.29`。普通模板本身将 `round(duration * 24)` 向上对齐到 `17n + 5` 帧：7 秒为 168 帧，对齐为 175 帧，输出约 `175 / 24 = 7.2917` 秒。这是普通工作流的帧数对齐，不是长视频续接补偿；长视频的上下文补帧与裁剪另有逻辑。这里不做精确时长裁剪，自定义工作流也可能有不同对齐规则。
- `reference_video_fps` 已包含在 `prepare_generation` 的工具 Schema 中。省略或 `null` 默认 7；`0` 使用源帧率；其他允许 1–60（含小数，如 `12.5`）。控制参考视频解码/采样，不改变输出 24 FPS。纯文生模式不使用参考视频，该参数不会产生效果。草稿及任务结果均回显参考 FPS。
- 内置原生 H3 节点按 24 FPS 解释参考帧序列。需要保留动作时间尺度时，显式传 `24`；直接低帧率抽帧可能缩短模型理解的参考时长。该参数是每秒帧数，不是参考总帧数上限。
- `resolution` 支持 `480p`（0.4 MP）、`720p`（0.9 MP），也支持 `0.1MP`–`1.9MP` 范围内的自定义十进制字符串，例如 `0.75MP`、`0.98MP`、`1.2MP`。MP 是百万像素，不是长边像素或固定宽高；实际宽高由画面比例及工作流对齐规则决定。
- 自定义 MP 保留小数精度，不再强制保留一位小数；`0.750MP` 与 `0.75MP` 规范化为同一参数。`get_capabilities.resolutions` 列出预设，自定义范围见 `custom_resolution_mp_range` 和 `custom_resolution_format`。现有 HTTP 创建接口、内置 Agent 与 MCP 共用分辨率校验。

## 能力边界

- 本版 MCP 暴露普通 H3 四种模式：文生、图生、首尾帧、多参考，支持自定义 MP。长视频编排、LoRA 选择和上传尚未开放。
- 参数与素材绑定校验不等于完整 GPU 预检。`get_capabilities` 检查本地模板、prompt 节点与部分参考槽位；`prompt_node_available` 不表示全部模型和节点都可执行。
- ComfyUI 不可达时明确返回 unknown 与 fallback 警告，不伪造可用状态。
- GPU 图编译/模型校验仍在提交后的现有后台 worker 执行。错误通过任务状态返回。
- 参考 FPS 省略为 7，0 取源帧率，其他为 1–60；输出 24 FPS。参考视频音轨不会自动重用。
- draft 保存在增量新增的 `generation_drafts` 表；不会重写已有任务或素材记录。

## 验证

```powershell
go test ./...
go vet ./...
.\scripts\check-mcp.ps1
```

`check-mcp.ps1` 仅做协议初始化、工具发现与能力查询；不保存草稿、不创建视频任务。
可传 `-Endpoint http://127.0.0.1:8790/mcp`。设置了 token 时，脚本从 `LOCALDREAM_MCP_TOKEN` 环境变量读取，不输出凭据。

自动化测试通过官方 MCP 客户端连接临时 HTTP 服务，使用临时数据库和模拟 ComfyUI。
覆盖草稿持久化/去重、并发提交去重、重启恢复、参数 Schema、项目范围、取消幂等、访问限制，以及现有编译/提交/结果查询的闭环；不消耗真实 GPU。

MCP 的 HTTP 边界、工具、能力探测和测试位于 `internal/app/mcp_*.go`；外部 ComfyUI HTTP 客户端位于 `internal/comfy/`，参考槽位解析位于 `internal/workflow/`。`read_prompt_guide` 的资源来自 `internal/skills/` 的嵌入文件，不能作为普通文档排除。

官方参考：[Go SDK](https://github.com/modelcontextprotocol/go-sdk)、[Streamable HTTP](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports)、[Tools](https://modelcontextprotocol.io/specification/2025-11-25/server/tools)。
