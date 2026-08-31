# Local Dream

本地即梦式 ComfyUI 创作台初版。

## 启动

```powershell
cd E:\tools\ComfyUI\app\frontend
npm install
npm run build

cd ..
go run .
```

另开一个终端启动适配层：

```powershell
cd E:\tools\ComfyUI\app
$env:COMFYUI_URL='http://127.0.0.1:8188'
python adapter\adapter.py
```

打开 `http://127.0.0.1:8790`。前端开发模式可用 `npm run dev`，地址为 `http://127.0.0.1:5174`。

当前 H3 模板副本位于 `templates/h3/`。适配层已经把 ComfyUI 连接隔离出来；下一步可在 `adapter.py` 中将模板 API JSON 编译为 ComfyUI `/prompt` 所需的节点图。
