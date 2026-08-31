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

当前 H3 模板副本位于 `templates/h3/`。Go 会把画布工作流编译为 ComfyUI `/prompt` 所需的 API 节点图，再轮询 `/history/{prompt_id}` 获取完成状态与输出视频 URL。

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
