# 长视频重生成规则

- 重生成最后一段：原计划内直接清空最后一段的 `job_id`，保留前面已完成片段、原视频和所有 latent，调度器重新提交最后一段。
- 重生成中间段 `x`：创建新的长视频计划 fork。新计划复制原计划的标题、项目、画幅、设置和所有分段请求；保留 `1..x-1` 的 job 关联与结果，把 `x..n` 的 job 置空。原计划不修改，原视频和原 latent 不删除。
- fork 会把原计划的 `h3_context/localdream/<source_sequence>` 下所有 `clip_*.safetensors` 复制到新的 sequence 目录。第 `x` 段使用新目录中 `clip_{x-1}`，后续新生成继续写新目录。
- 如果旧计划来自早期“只保留最新 latent”的版本而缺少 `clip_{x-1}`，接口会明确返回缺少前置 latent，而不会伪造续接或误用未来段。新生成的计划会完整保存 latent，因此之后可以正常 fork。
- 原计划和 fork 的任务记录共享前缀 job 记录，但后续新片段拥有新的 job 和新的 continuity sequence ID。
