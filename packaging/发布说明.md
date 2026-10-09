# Local Dream Windows 发布包

此目录可直接在 **Windows x64** 上运行，目标机器不需要安装 Go、Node.js 或 npm。

## 使用方法

1. 解压 ZIP 到一个可写目录（不要在压缩包预览器中直接运行）。
2. 确保本机已有可工作的 ComfyUI、所需 H3 工作流节点和模型；本包不携带 ComfyUI、Python、模型或自定义节点。
3. 双击 `start-localdream.cmd`，然后在浏览器打开 `http://127.0.0.1:8790`。

程序会优先连接已在 `127.0.0.1:8188` 运行的 ComfyUI。若该地址没有运行中的 ComfyUI，会尝试识别 ComfyUI Desktop 安装并启动其本地后端。

## 自定义 ComfyUI 位置

将 `config.example.cmd` 复制并改名为 `config.local.cmd`，按实际安装位置取消需要的 `set` 行注释并修改路径。`start-localdream.cmd` 会在启动前读取它。

常用变量：

- `COMFYUI_URL`：已有 ComfyUI API 的地址，默认 `http://127.0.0.1:8188`。
- `COMFYUI_DIR`：ComfyUI 根目录；无法自动识别 Desktop 安装时需要设置。
- `COMFYUI_INPUT`：ComfyUI 的 input 目录；素材会复制到这里。
- `LOCALDREAM_ADDR`：Local Dream 的监听地址，默认 `127.0.0.1:8790`。

## 数据与隐私

此发布包**不包含任何现有数据库**，也不包含上传素材、生成记录、Provider 配置/API Key 或 ComfyUI 日志。首次启动后，程序会自动创建空的 `data/` 目录、空 SQLite 数据库和运行日志；这些都是该解压副本自己的本地数据。

不要把 `data/` 目录与他人共享，尤其是其中的 `provider.json` 和 SQLite 文件。
