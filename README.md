# Local Dream

基于本地 ComfyUI 的 H3 视频创作台，使用 Go 后端、React 前端和 SQLite 保存项目、素材与任务。支持文生视频、图生视频、首尾帧、多参考视频及分段长视频；同一服务提供 HTTP API 和 Streamable HTTP MCP。

## 项目结构

```text
main.go                 启动入口，调用 internal/app.Run
internal/
  app/                  服务启动、HTTP/MCP、任务生命周期、存储与工作流编译
  comfy/                ComfyUI 节点信息 HTTP 客户端
  workflow/             运行时参考槽位解析及素材能力数据类型
  responses/            可选内置 Agent 使用的 Responses 客户端
  skills/               MCP/Agent 内嵌的 H3 提示词指南
frontend/               React、TypeScript、Vite 源码
templates/h3/          H3 工作流模板及模式清单
scripts/                MCP 检查、参考 FPS 核验、Windows 打包
packaging/              发布包启动脚本、示例配置与 README
docs/mcp.md             MCP 接入、参数和任务语义
```

后端测试与实现放在同一包目录。`internal/app` 保留现有服务与生成逻辑的组合；外部协议客户端和槽位解析独立成包。项目根目录也是运行时资源根目录，`go run .` 和 `go build .` 的入口保持可用。

`data/`、`frontend/dist/`、依赖、二进制、发布包、预览截图和本机配置由程序或工具生成，不纳入 Git。项目仓库只维护 README 和 MCP 文档；内嵌 H3 指南属于运行时资源。

## 本地启动

需要 Go 1.26、Node.js/npm，以及安装了 H3 模型和所需节点的 ComfyUI。当前目录是包含本 README 和 `go.mod` 的仓库根目录。

```powershell
cd frontend
npm ci
npm run build
cd ..
go run .
```

打开 `http://127.0.0.1:8790`。服务优先复用 `http://127.0.0.1:8188` 的 ComfyUI；后端不可用时默认尝试识别 ComfyUI Desktop 安装并启动本地后端。

前端开发时，在 `frontend` 下运行 `npm run dev`，打开 `http://127.0.0.1:5174`。API 请求代理到 Go 服务的 8790 端口。

## 配置

通过环境变量配置；Windows 发布包也可将 `config.example.cmd` 复制为 `config.local.cmd`，修改后由启动脚本加载。

| 变量 | 用途与默认值 |
| --- | --- |
| `LOCALDREAM_ADDR` | Go 服务监听地址，默认 `127.0.0.1:8790` |
| `COMFYUI_URL` | ComfyUI API 地址，默认 `http://127.0.0.1:8188` |
| `COMFYUI_DIR` | 自动启动时的 ComfyUI 根目录；可自动识别 Desktop 安装 |
| `COMFYUI_PYTHON` | ComfyUI Python 路径，默认使用安装目录下的虚拟环境 |
| `COMFYUI_INPUT` | 参考素材复制目录；默认 `D:\Comfy-Desktop\ComfyUI-Shared\input` |
| `COMFYUI_OUTPUT` | 自动启动 ComfyUI 时的输出目录，默认与 input 目录同级的 output |
| `COMFYUI_MODEL_PATHS_CONFIG` | 自动启动时传入的额外模型路径配置 |
| `LOCALDREAM_COMFY_AUTOSTART` | 设为 `0` 禁用自动启动 ComfyUI |
| `H3_TEMPLATE_DIR` | 自定义模板目录，默认 `templates/h3` |
| `LOCALDREAM_MCP_TOKEN` | 可选 MCP Bearer token；配置后本机 MCP 请求同样需要认证 |

运行状态保存在 `data/`：SQLite 保存任务与元数据，`assets/` 保存素材，`provider.json` 保存可选 Agent 的连接配置，`comfy-backend.log` 保存自动启动的后端日志。Provider API Key 和本机数据不应提交。

## MCP 与生成参数

Endpoint 为 `http://127.0.0.1:8790/mcp`。MCP 工具负责查询能力、项目与素材，读取 H3 指南，准备持久化草稿，提交、查询和取消任务；MCP 调用本身不会调用另一语言模型。完整工具协议、接入示例及访问限制见 [MCP 接口说明](docs/mcp.md)。

普通模板输出 24 FPS，并将请求时长对齐到 H3 的 `17n + 5` 帧。`duration` 传原始需求，不提前补帧；实际成片可能略长。参考帧率 `reference_video_fps` 默认 7，支持 1–60 的小数，`0` 使用源帧率；需要保留原生 H3 参考动作时间时应显式设为 `24`。输出分辨率支持 `480p`、`720p` 和 `0.1MP`–`1.9MP` 自定义值。

长视频使用单独的 Motion Context 模板，按片段持久化计划与任务，处理续接上下文、有效时长、裁剪和重生成。该流程通过产品 HTTP API 和界面使用，当前 MCP 不暴露长视频编排。

默认监听本机。需要远程使用时应通过可信网络或 HTTPS 代理提供访问控制：现有产品 HTTP API 没有登录保护，MCP token 只保护 MCP 接口，不能保护整个站点。不要直接暴露 ComfyUI 8188 或 Vite 5174。

## 验证

```powershell
go test ./... -count=1
go vet ./...

cd frontend
npm run typecheck
npm run format:check
npm run build
cd ..

.\scripts\check-mcp.ps1
```

普通自动化测试使用临时数据库和模拟服务；MCP 检查脚本仅初始化协议、发现工具与查询能力。

可选的真实 ComfyUI 文生编译检查只读取节点信息，不提交推理任务：

```powershell
$env:COMFYUI_INTEGRATION = '1'
go test ./internal/app -run '^TestCompileTextToVideoAgainstComfyUI$' -v
Remove-Item Env:COMFYUI_INTEGRATION
```

## Windows 发布包

```powershell
.\scripts\package-windows.ps1
```

脚本构建前端和 Go 可执行文件，将启动脚本、示例配置、发布 README 与工作流模板打包到 `release/`。不包含本机数据库、素材、Provider 配置或日志。目标机器无需安装 Go 或 Node.js，但仍需要安装 ComfyUI、模型和工作流节点。发布包使用方法见 [packaging/README.md](packaging/README.md)。
