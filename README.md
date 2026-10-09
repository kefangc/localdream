# Local Dream

本地即梦式 ComfyUI 创作台初版。前端、产品 API、H3 工作流编译、素材绑定和 ComfyUI 任务跟踪均由 Go 服务完成；不需要 Python 适配层。

## 启动

```powershell
cd E:\tools\ComfyUI\app\frontend
npm install
npm run build

cd ..
go run .
```

打开 `http://127.0.0.1:8790`。Go 服务默认直接连接 `http://127.0.0.1:8188` 的 ComfyUI，并读取下列环境变量：

```powershell
$env:COMFYUI_URL='http://127.0.0.1:8188' # 可选，ComfyUI API 地址
$env:COMFYUI_INPUT='D:\Comfy-Desktop\ComfyUI-Shared\input' # 可选，ComfyUI input 目录
$env:LOCALDREAM_ADDR='127.0.0.1:8790' # 可选，Local Dream 监听地址
```

前端开发模式可用 `npm run dev`，地址为 `http://127.0.0.1:5174`。

## 公网部署

生产前端由 Go 在同一端口提供，不要将 Vite 开发服务器 `5174` 暴露到公网。默认监听地址 `127.0.0.1:8790` 仅允许本机访问；公网发布时，使用下面的环境变量启动：

```powershell
$env:LOCALDREAM_ADDR='0.0.0.0:8790'
go run .
```

同时在 Windows 防火墙中只开放 TCP 8790，并在路由器中将外网 TCP 8790 转发到本机的局域网 IP。不要公开 `8188`（ComfyUI）或 `5174`（Vite 开发服务器）。

此模式不包含登录保护；若计划长期对公网开放，应在反向代理上配置域名、HTTPS 和访问控制。

当前 H3 模板副本位于 `templates/h3/`。Go 会把画布工作流编译为 ComfyUI `/prompt` 所需的 API 节点图，再轮询 `/history/{prompt_id}` 获取完成状态与输出视频 URL。

参考视频可通过 `reference_video_fps` 控制载入帧率，新请求默认 7fps，允许 1–60fps（支持小数），0 使用源帧率；输出仍为 24fps。历史兼容、短参考限制和验证方式见 [参考视频 FPS](docs/reference-video-fps.md)。

## 验证

```powershell
go test ./...
go vet ./...

cd frontend
npm run format:check
npm run typecheck
npm run build
```

如需在不提交生成任务的情况下校验本机 ComfyUI 的对象信息与文生视频工作流编译：

```powershell
$env:COMFYUI_INTEGRATION='1'
go test -run TestCompileTextToVideoAgainstComfyUI -v
```

## v1 长视频更新

手动分段、有效时长补偿、连续续接、配置与后端职责拆分见 [长视频使用与实现说明](docs/long-video-v1.md)。v2 未改动。真实 GPU 生成验证状态请以该说明为准。
