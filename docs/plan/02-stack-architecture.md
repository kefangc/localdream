# 技术栈与架构决策

## 技术选择

| 层面 | 选择 | 当前状态 / 引入时机 |
| --- | --- | --- |
| 产品后端 | Go；go.mod 当前为 Go 1.26 | 已使用，不因重构换语言 |
| HTTP | net/http、http.Client、httptest | 已使用；现阶段不为路由简单性引入 Gin/chi |
| JSON/配置 | encoding/json、环境变量、类型化 manifest | 已使用，补 schema 校验和配置加载 |
| 前端 | React + TypeScript + Vite + lucide-react，现有 CSS | 保留；不迁 Next.js，不重做页面 |
| 推理执行 | 外部 ComfyUI HTTP API；H3 已选模板 | 保留 Go 直连，不重新加入独立 Python bridge |
| 数据库 | SQLite + database/sql；优先评估 modernc.org/sqlite | M2 引入；先验证 Go/Windows 兼容和构建成本，不预设已安装 |
| SQL | 显式 SQL + 版本化 migration | 先学事务和数据模型，不先上 ORM；重复明显后评估 sqlc |
| 文件存储 | 本地受控 data 目录；数据库保存元数据 | 大媒体不存数据库；权重不纳入仓库/安装包 |
| 任务执行 | SQLite 任务状态 + 单 worker + 有界接收 | M2；内存 channel 是唤醒机制，不是唯一任务真相 |
| 日志与关闭 | log/slog、signal.NotifyContext、http.Server.Shutdown | 随任务可靠性接入 |
| 媒体处理 | 必要时使用 FFmpeg/ffprobe | 调用前检查现有路径，不默认安装；参数数组调用并绑定 context |
| 测试 | testing、httptest、临时目录；可选 go-cmp | 已使用；复杂 diff 才加库，前端测试按需要评估 Vitest/Testing Library |
| 运行交付 | Go 二进制 + Vite dist + 模板/配置 | M3；桌面壳和自动更新不是 M1 前提 |

当前 frontend/package.json 多个版本声明为 latest，存在 package-lock.json。发布前核对锁文件、使用 npm ci，并把直接依赖改为经过验证的版本范围；不要在教学中顺带批量升级。

暂不采用：微服务、Kubernetes、Redis、消息中间件、PostgreSQL、通用 DI 框架、完整插件平台。出现明确的多机调度/多用户协作需求后再讨论。

## 总体结构

```text
React 页面
  -> 产品 HTTP API
    -> 应用服务：查询能力 / 创建任务 / 取消任务
      -> 领域规则与绑定计划
      -> 存储接口、ComfyUI 接口
        -> SQLite / 本地文件 / ComfyUI
```

M1 可保留 main.go 为组装入口。边迁移边删除旧重复逻辑，不一次创建所有空目录。

```text
cmd/localdream/       最终入口，依赖组装和启动
internal/httpapi/     请求解析、DTO、状态码、路由
internal/workflow/    模式、能力决策、模板编译与绑定规则
internal/comfy/       ComfyUI HTTP 与专属协议适配
internal/asset/       上传、素材元数据和生命周期
internal/generation/ 创建/取消任务、状态机、worker
internal/agent/      可选意图解释，不能绕过验证
internal/storage/    SQLite 仓储与 migration
internal/config/     稳定后再提取的配置加载
```

这是目标职责图，不是立即建目录清单。当前 workflow/detector.go 仍包含 ComfyUI 协议细节，属于过渡布局；不能宣称整个 workflow 包已是纯领域层。M1 闭环后再评估迁至 comfy，保留现有测试，不为包名先停工。

接口由需要它的调用方定义；保持小接口，不为每个结构体配一个接口。Catalog 通过 RuntimeSlotsProvider 工作，workflow 不反向 import comfy。不要建立通用 common/utils 大包。

## 能力不是一个 max 数字

必须区分：

1. Runtime slots：当前节点技术上限，区分图片、视频、视频配套音轨、独立音频。
2. Template bindings：当前模板如何将用户素材接到节点。
3. Mode policy：产品模式语义，如图生恰好一张、首尾帧两个明确角色。
4. Implemented support：编译器已实现并验证哪些绑定。

有效能力来自这些约束的组合，不允许 runtime max 自动放开尚未实现的功能。

fallback 默认值与产品硬限制分开建模：前者是离线估计，后者即使在线也必须遵守。不要把所有 fallback max 当硬限制取最小，也不要将整个 runtime range 直接覆盖所有模式语义。遇到相互冲突的最小/最大值应返回不可用原因，而不是生成不可能满足的能力。

当前解析器只适配指定的 autogrow 输入名称，不是任意 ComfyUI 节点通用发现器。缺失字段可能是不支持，也可能是协议错误，需要按模板契约判断，不能统一当成 0 或无限。

## Catalog 与降级策略

- 已确认静态模式没有 RuntimeNode：使用声明式配置，不算探测失败。
- 在线且 schema 合法：合并 runtime 与模式/绑定限制。
- 网络不可用或探测失败：界面可使用有效 fallback，并明确 source、fallback、warning；不代表执行端已就绪。
- 未知模式、fallback 无效或绑定不支持：硬错误或明确不可用。
- 调用者 ctx 已取消或 deadline 已到：停止，不将取消转成成功。
- 探测子请求超时但父请求仍有效：可依明确策略降级；不要只凭 errors.Is(err, DeadlineExceeded) 混淆两者。
- schema 不兼容可以给界面离线信息，但不能自动批准生产提交；入队前做执行兼容性检查。
- 没有 runtime 数据时返回明确缺失状态，例如 runtime_slots 为 null/省略，避免零值被解释为实时 max=0。

warnings 给用户简明原因；详细上游 body、路径和内部错误记录日志，不直接回传所有原始内容。

## 素材与提交

素材库容量不同于本次任务引用数量。用 asset_id + role + position 表达绑定，不继续用 prompt.includes(filename) 作为唯一依据。

```text
解析请求 -> 查素材元数据 -> 解析能力和绑定计划
 -> 验证类型/数量/角色/兼容性 -> 保存任务 -> worker 准备文件并编译 -> 提交
```

首尾帧使用 first_frame/last_frame；独立音频与视频音轨分开。视频配套音轨必须有用户选择或明确默认策略，不一律自动参与条件生成。

为排队中的任务记录能力、模板和参数快照；执行前若环境变化则检查兼容性，不能无提示地换规则。

## 安全与部署边界

仅监听本机；原 README 中无认证公网监听方法不是新计划的部署方案。远程访问是独立扩展，需要认证、HTTPS、来源控制与授权。

上传限制使用 MaxBytesReader，并检测实际媒体、受控路径、唯一 ID、失败清理。静态 /assets/ 保留给 Vite，媒体使用 /media/。作品由产品后端受控访问，不将 localhost:8188 当成跨机器可访问的最终作品 URL。

Provider API Key 不进 Git/测试样例/日志。外部命令不用字符串拼 shell。删除素材前检查任务引用；数据迁移、清理和模型安装单独授权。
