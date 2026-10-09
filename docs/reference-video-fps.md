# 参考视频 FPS

生成请求增加 `reference_video_fps`，普通 `/api/generations` 和长视频
`/api/long-videos` 的各个 `segments` 均支持。前端在「多参考视频」的高级参数中
提供输入框；每段长视频也可独立设置。

- 新请求省略字段（或传 `null`）：默认 **7fps**。
- **1–60fps**：按该帧率载入参考视频，支持小数，例如 `7.5`。
- **0**：使用参考文件的源帧率。
- 负值、介于 0 与 1 的值、超过 60 的值及非有限数字均拒绝。
  HTTP API 返回 `400`，不创建生成任务。

例如（替换提示词和素材 ID 后再提交）：

```json
{
  "mode": "multi_reference_video",
  "strategy": "normal",
  "prompt": "Use the reference character and preserve the reference motion.",
  "aspect_ratio": "16:9",
  "resolution": "0.7MP",
  "duration": 5,
  "steps": 24,
  "lora": "",
  "lora_strength": 0,
  "reference_video_fps": 7,
  "asset_ids": ["<image_asset_id>", "<video_asset_id>"],
  "use_agent": false
}
```

该参数直接写入每个实际参考视频 `VHS_LoadVideo` 节点的 `force_rate`。
多个视频共用当前任务的设置。`custom_width`、`custom_height`、帧数上限和跳帧
参数仍沿用原设置。它控制参考载入，不会改变成片输出的 **24fps**，也不会改变
H3 的内部时间语义、时长网格、LoRA 或采样步数。

## 历史兼容

新任务把解析后的 FPS 保存到 SQLite 的任务 JSON；查询历史和复用参数可取得
该字段，任务恢复、长视频重试、分段重生成与派生计划均保留它。

旧记录缺少该字段，代表原来的 `force_rate: 0`。返回历史、任务恢复或打开旧
长视频计划时使用 **0（源帧率）**，不会把过去的任务描述成 7fps，也不重写既有
生成文件。前端打开旧浏览器草稿同样沿用源帧率。新建任务和新建片段默认 7fps。

## H3 对短参考的限制

本机 `MiniMaxH3ReferenceToVideo` 接收 IMAGE 帧张量，不接收 VHS 的 FPS 元数据。
它按 24fps 解释帧序列，并将参考帧数向下截为 `17k+5`；不足 5 帧时报错。
因此低 FPS 可能只留下 5 帧，丢失后半段动作。提高参考 FPS 可以增加保留帧数，
但不能据此宣称参考时序与原片完全等价。输出仍需要逐段视觉检查。

## 不提交生成任务的验证

```powershell
go test ./...
go vet ./...
$env:COMFYUI_INTEGRATION='1'
$env:REFERENCE_FPS_AUDIT_PATH='<绝对路径>\reference-fps-compiled-graphs.json'
go test -run TestCompileReferenceVideoFPSAgainstComfyUI -v
```

集成测试读取本机 `/object_info`，编译普通和长视频模板，核对所有实际参考
载入节点的 `force_rate`、CreateVideo 的 24fps、24 采样步数和无 LoRA；不调用
`POST /prompt`。测试使用临时素材与 input 目录，不会覆盖用户素材。

`scripts/verify_reference_video_fps.py` 用 ComfyUI 自己的 Python 和已安装的
`VHS_LoadVideo`，根据上述编译图直接载入真实短片段，记录帧张量形状、实际
载入 FPS 与帧数。运行在 CPU，不启动服务、不载入模型、不采样。报告中的 H3
保留帧数是根据已安装节点代码计算的值，未执行 H3 VAE 编码。

```powershell
& '<ComfyUI Python 绝对路径>' scripts\verify_reference_video_fps.py `
  --comfy-root '<ComfyUI 根目录>' `
  --sources '<对应原片片段目录>' `
  --graphs '<编译图 JSON 绝对路径>' `
  --output '<验证报告 JSON 绝对路径>'
```
