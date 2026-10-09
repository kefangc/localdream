# 当前迭代：把新能力带到真实 API

基线日期：2026-09-10。下列勾选只描述当天检查结果。

## 状态

- [x] workflow 能力类型、数量校验和协议解析已有实现与测试。
- [x] ComfyUI HTTP Client 和 RuntimeAssetSlots 组合测试通过。
- [x] Catalog 在线成功及未知模式测试存在。
- [ ] Catalog fallback 测试通过：当前仍失败，Provider 错误被直接返回。
- [ ] 类型化 manifest 最小加载。
- [ ] capabilities 产品 API。
- [ ] 页面使用真实能力数据。
- [ ] 生成提交前使用相同能力和绑定规则。
- [ ] 真实小样本生成验证。

现有提交检查点：e7cf931（workflow 基础），5c62c98（Client）。当前还有 Client 未提交改动和 Catalog 暂存/未暂存改动。继续前先看差异，不重新建项目。

## 本轮范围

只完成“有效能力查询”；不迁移 SQLite、不重写前端、不实现全部参数发现。H3 多参考模式先贯通，其他模式维持旧行为并明确状态。

### 任务 A：用自己的实现完成 Catalog 降级

由用户先回答再写代码：

1. 哪些失败允许离线展示？哪些必须返回 error？
2. 调用者已取消和客户端探测超时如何区分？
3. fallback 的能力合法性由谁验证？零值是否真的表示有效配置？
4. 未配置 Provider 是离线运行配置还是组装错误？本版选择一种明确策略并测试。

验收用例：

- Provider 普通失败 + 有效 fallback：返回降级能力和简明 warning。
- 调用者 context 预先取消：不调用 Provider，不返回成功能力。
- Provider 调用后父 context 已结束：保留取消/到期错误。
- 未知模式：不调用 Provider，返回可识别错误。
- 无 runtime 数据：不将零槽位冒充实时探测结果。
- 保留已有在线 synthetic 11/4/6/7 测试，但注明它仅验证已支持的多参考映射。

先实现这些，不再逐个检查写一大轮重复答案。助手审查真实 diff 和测试输出。

### 任务 B：最小 manifest 载入

只增加多参考模式需要的模式标识、runtime 节点映射、fallback 数量、总数语义及绑定支持声明。

不得把当前旧 manifest 的全局 constraints 直接套给所有模式；旧结构没有独立音频兜底字段，需要明确新增并验证。声明 fallback 表示产品选择而非本机实时探测事实。

测试从 t.TempDir 或 fixture 读取，覆盖未知模式、缺失/非法上下限和不支持的绑定声明。Catalog 构造后配置应只读；不热更新 map。

### 任务 C：能力 API

先由用户定义一个 DTO 和 handler；建议接口：

```http
GET /api/workflows/capabilities?mode=multi_reference_video
```

响应至少表达：mode、有效素材约束、source、fallback、warnings。技术 runtime slots 不必全部暴露前端；明确该接口返回的是能力，不是执行端健康保证。

临时组装可放在现有 main.go，handler 提取至 internal/httpapi。只改相关路由，不重写全部 Server。前端依赖 DTO，不直接解读 ComfyUI JSON。

测试：成功返回 JSON；缺少/未知模式有明确错误；错误体保留旧客户端可显示的 error 文本并逐步增加 code；warnings 无内容时是 [] 而非 null。

### 任务 D：页面与提交下一轮接入

页面按本次引用显示数量与降级来源；选中状态独立于提示词文本。先给现有 UI 接能力，不重做视觉。

后端必须重新查素材并验证；不能信任客户端上报 count 或 max。非法请求在复制文件和创建任务前拒绝。实际生成还须验证模板/绑定/执行端；不因为查询用了 fallback 就宣称可生成。

## 本轮独立能力检查

- 能不看完整示例写出一个 HTTP handler 和成功/失败测试。
- 能解释 RuntimeSlotsProvider 为什么由消费者定义。
- 能说明为什么普通网络错误可以降级但请求取消不能继续。
- 能定位一次失败测试的真正原因，而不是只修改到 PASS。

## 交付与下一步

先在当前分支完成 Catalog 和 manifest 查询所需切片，绿色后检查并分开提交。随后交付 API 与页面闭环；M1 的完整验收见 [03-milestones.md](03-milestones.md)。

每次交接只报告实际已完成、验证证据、一个主要阻塞项、下一项最小任务。不再重复整套路线，也不按聊天轮次报百分比。
