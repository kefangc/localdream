# Local Dream Agent 模式实现规划

日期：2026-09-26。目标：现用 `E:/tools/ComfyUI/app` Go 后端与配套 React 前端。

本文是调研后的实现设计，不代表功能已经实现。依据为当日工作区代码、侧边浏览器中的即梦与 Updream 页面，以及 OpenAI 官方文档。没有启动真实推理、修改业务代码、安装第三方 skill 或修改生产数据库。`localdream-v2` 不在实施范围。

## 1. 产品目标与关键决定

将 Agent 建成项目内的创作执行助手：理解文本和参考素材，判断是否需要技能，形成可编辑方案，调用本地生成能力，等待任务结果，并根据用户反馈修改作品。

第一版应贯通：**一句需求 + 引用素材 → 自动选择技能 → 形成生成草稿 → 按授权提交 → 展示生成状态 → 返回真实视频 → 对话修改**。

核心决定：

1. 模型协议固定使用 `POST /v1/responses`；模型名和服务地址可配置，不把语言模型与视频生成模型混为一谈。
2. 使用单个 Go Agent 执行循环与类型化工具。当前阶段不引入多 Agent 框架、Python 服务、Redis 或独立工作流引擎。
3. 视频生成继续由现有 ComfyUI 编译器、任务生命周期和长视频调度器负责。Agent 不生成任意 ComfyUI 图，不直接写数据库或执行 shell。
4. 技能采用 `SKILL.md + references/ + assets/` 目录形式，内置完整 `h3-prompt-writing`。用户可安装、创建、启停；Agent 可自动选择，并在配置允许时从受信目录自动安装。
5. Skill 是创作方法和领域知识；Tool 是后端可以执行的能力。安装一个分镜技能不会凭空获得图片生成、配音或视频合成能力。
6. 会话、运行、草稿、工具操作、用户问题及媒体任务分开持久化。断开浏览器连接不取消后台创作。
7. 前端展示来自后端的阶段与产物卡片。任务 ID、实际模型、参数和完成状态必须来自工具结果，不能从助手自然语言中提取。
8. 所有模式共用相同能力校验，明确区分用户要求、模型建议和最终执行参数。模型或画幅等硬约束不支持时，不静默替换。

## 2. 竞品观察：可借鉴的产品行为

### 2.1 即梦：创作入口、技能工作流与素材上下文

观察入口：[即梦 Agent 页面](https://jimeng.jianying.com/ai-tool/home?createTab=chat&type=agentic)。

当日实际页面可见：

- Agent 模式入口，输入想法、剧本或上传参考；支持 `/` 调用技能及添加主体。
- 对话、画布、资产、主体是并列的产品对象，并有历史对话入口。
- 技能菜单有名称和用途描述；包括视频反解、创作分镜、全流程广告片导演、剧情短片、电商套图、海报设计、品牌设计。
- “创作分镜”的介绍包含长叙事分段；“剧情短片”的介绍覆盖大纲、剧本、素材、需求确认、分镜、生成和剪辑等阶段。
- 管理技能可以进入新建表单，字段为名称、描述、内容；还有“用 Agent 创建技能”入口。部分官方技能正文不公开。
- “自动”按钮打开的是生成偏好面板，包含图片/视频、智能比例及模型设置；不能仅凭这个按钮推断它的内部模型路由算法。

对本项目的启示：Agent 面向“作品目标”，技能可以约束整个创作过程；素材、分镜、视频与结果都要成为可以引用和修改的对象。`/skill` 是人工指定的入口，自动选择应作为默认方式。

### 2.2 Updream：对话与画布产物联动

观察入口：[Updream 画布](https://www.updream.cn/canvas?project=348820021226917888)。这里是当前浏览器项目的已有对话，引用链接需要相应会话权限。

当日实际页面可见：

- 项目画布旁有 AI 助手、多轮历史、步骤完成摘要，以及生成结果入口。
- 同一条需求附带图片、音频和视频；助手通过问题收集模型选择，再给出异步生成状态和画布节点标识。
- 有“去画布查看进度”“修改生成参数”等后续动作。
- 输入区提示 `/` 调用技能、`@` 引用素材；有技能插件、技能社区入口和当前为“询问”的确认模式。
- 页面提示生成不阻塞、可以开启下一轮对话；现有消息支持编辑后从对应位置重新开始。

观察到的限制：技能入口点击后未取得可读的目录内容，不能据此断言其安装协议；步骤摘要没有提供可验证的内部工具参数。现有对话里用户选择的模型和助手最终声称使用的模型不一致，且要求的分辨率被降低。这能证明页面存在叙述不一致，不能证明它实际调用了哪个服务。

对本项目的启示：需要结构化问题、持久化草稿、异步媒体任务、继续对话和产物操作。模型与参数卡片应由执行记录生成，避免用户选择与实际执行脱节。

### 2.3 借鉴范围

这次观察没有向平台发送新创作消息、上传素材或消耗生成积分；没有获得它们的私有后端实现。以下是基于公开交互和本地代码提出的工程方案，不把竞品页面描述当作已验证的生成质量或系统架构。

首期沿用 Local Dream 的项目和作品列表，加 Agent 侧栏与结构化产物卡片。无限画布可以后续接入，不作为后台 Agent 闭环的前置条件。

## 3. 当前代码基线与必须处理的接入点

| 当前文件/能力 | 当日代码事实 | 新 Agent 的处理 |
| --- | --- | --- |
| [provider_http.go](../provider_http.go) | `agentHandler` 一次请求 `/chat/completions`，素材用文本格式传入，返回 JSON | 新增独立 Responses Client 和执行循环，旧 interpret 做受限兼容入口 |
| [frontend/src/App.tsx](../frontend/src/App.tsx) | `interpret()` 把 subject/motion/camera 等字段合并回填输入框 | 新增对话与草稿视图，不继续把完整 H3 结构拆回五个字段 |
| [domain.go](../domain.go) | `UseAgent` 是布尔值；没有会话、运行、工具记录 | Agent 使用独立 API 与对象；生成请求只接收已解析且校验过的参数 |
| [generations_http.go](../generations_http.go) | POST 内混合参数校验、素材快照、建任务和启动执行 | 抽出共用应用服务，普通表单和 Agent 同样调用 |
| [generation_submit.go](../generation_submit.go) | 已有 seed、client_id、先落库后提交、原子认领和提交异常后的核对 | 保留这些语义，外层再加 Agent 操作与 generation 的事务关联 |
| [generation_lifecycle.go](../generation_lifecycle.go) | 读取 queue/history，恢复任务，保存完成状态 | 作为媒体状态事实来源，给 Agent 提供完成事件和重启补查 |
| [comfy.go](../comfy.go) | `compilePrompt` 会访问 ComfyUI、复制素材；引用按类型和顺序绑定 | 不能直接当“无副作用预览”；抽出纯参数/引用校验，显式保存绑定清单 |
| [internal/workflow/catalog.go](../internal/workflow/catalog.go) | 有能力探测/fallback 类型；当前业务入口未接 `NewCatalog/Resolve` | 接到统一能力服务，模板绑定上限与运行时槽位取交集 |
| [long_video_service.go](../long_video_service.go) | 有分段调度、时长计算、重生成/分叉 | 包装为序列工具，继续使用 revision 和已有依赖顺序 |
| [long_video_model.go](../long_video_model.go) | LongVideo 是持久化片段序列；并非已合并的媒体文件 | 交付片段序列时明确标识；合并导出另列能力 |
| [store.go](../store.go) | SQLite，现有对象多为 id + JSON；单数据库连接 | 新增 Agent 专用表与增量迁移，不全库重构，不在事务内做网络请求 |
| [frontend/src/lib/api.ts](../frontend/src/lib/api.ts) | 通用请求有 60 秒超时，等待完整 JSON | 发送消息改为快速 202；事件流使用单独客户端 |

还需在相关改动中修正：

- 目前 `UseAgent=true` 只绕过了空提示词检查，并不会自动触发 Agent。新生成入口必须拒绝空提示词，也不能把该布尔值当作有效授权。
- 时长约束存在多处来源：普通接口允许 1–15 秒，manifest 为 5–15，H3 skill 推荐 4–15，长视频又有有效秒数/输入秒数和建议上限。统一返回“实现能力、模板约束、创作建议”，不拍脑袋硬编码一个新范围。
- 当前多参考编译器上限为 9 图/3 视频/1 独立音频。即使运行时节点有更多槽位，Agent 也不能超出编译器实际支持。
- 当前视频引用连接视觉帧，`attachAssets` 会清除 `ref_video_audios.*`，没有把参考视频自带音轨接回。不能承诺自动复用视频音频；需显式抽取/绑定或报告不支持。
- 当前 template 中 `image_to_video` 节点 133 有 `first_frame` 链路，首尾帧节点 259 有两个关键帧输入；Ref2VA 节点 316 的参考图槽位不是同等意义的硬首帧。应按实际模式绑定，不能只靠提示词说“这是第一帧”。
- 当前没有 L2VA 单独产品模式；skill 支持 L2VA 不等于后端已可执行。

以上只做了代码核对，尚未通过本次真实生成验证。

## 4. 用户流程：Agent 应完成哪些事情

| 用户请求 | Agent 行为 | 预期产物 |
| --- | --- | --- |
| “做一段雨夜街景视频” | 读取能力，合理补足创作细节，加载 H3 技能，按授权生成 | 草稿、任务卡、完成视频 |
| “用这张角色图，参考这个视频动作” | 识别素材角色，分析必要内容，固定引用标签，选择 Ref2VA | 引用清单、提示词、生成结果 |
| “先给我三个故事方向，不要生成” | 只做创意和方案，可不使用任何 skill | 文案/方案卡；零媒体任务 |
| “做一个 30 秒广告短片” | 故事规划、分段、角色一致性、每段 H3 编译、分段生成 | 分镜草稿、LongVideo；导出能力具备后才给完整 MP4 |
| “第二段慢一点，前面保留” | 解析所指作品、读取段落，修改草稿；解释 Motion Context 后续依赖 | 新版本/分叉序列及受影响段落 |
| “刚才的任务到哪了” | 直接读取任务状态；无需 H3 skill | 真实状态、已有进度证据 |
| “安装这个 skill，然后用它写分镜” | 安装/校验、展示可用性、加载内容、按现有工具执行 | 安装记录、技能版本、分镜产物 |

对话继续时默认绑定当前项目、用户显式选中的素材和作品。模糊的“上一个”“第二段”在有唯一指代时自动解析；有多个候选时才提出短问题。

## 5. 后端结构：一套执行循环，复用既有业务

```text
React：消息 / @引用 / 技能 / 草稿 / 参数问题 / 结果卡
  │ POST message → 202(run_id)；GET events → SSE
Agent HTTP
  │
Agent Service + Runner + SQLite
  ├─ ContextBuilder：项目上下文、消息、草稿、素材、技能目录
  ├─ ResponsesClient：POST /v1/responses、SSE 解析
  ├─ SkillRegistry：发现、按需读取、安装版本管理
  ├─ ToolRegistry：Schema、参数校验、执行权限、执行结果
  └─ BusinessAdapter
       ├─ Asset / Project 查询及素材解析
       ├─ Capability / GenerationDraft 校验
       ├─ GenerationService → compilePrompt → ComfyUI
       └─ SequenceService → 现有 LongVideo dispatcher
```

建议目录（均为拟新增，非现有功能）：

```text
app/
  agent_http.go                 # 接收消息、运行查询、问题回答和 SSE
  agent_business_adapter.go     # 实现 internal/agent 定义的业务接口
  generation_service.go        # 从 handler 抽出的共享建任务服务
  sequence_service.go          # 从 long_video_http 抽出的共享序列操作
  internal/agent/
    model.go, service.go, runner.go, context.go
    tools.go, policy.go, store.go, events.go
  internal/responses/
    client.go, types.go, stream.go
  internal/skills/
    registry.go, loader.go, install.go, builtin.go
    builtin/h3-prompt-writing/
      SKILL.md
      references/base-en.txt
      references/ref-en.txt
  data/skills/installed/<skill-id>/<version>/
  data/skills/staging/<installation-id>/
  frontend/src/components/agent/
    AgentPanel.tsx, MessageList.tsx, DraftCard.tsx
    QuestionCard.tsx, RunSteps.tsx, SkillManager.tsx
  frontend/src/lib/agent-api.ts
```

依赖方向：`internal/agent` 定义它需要的接口，`package main` 的适配器实现接口；不要让 agent 包反向导入 main，也不要为内部工具请求自己的 HTTP 服务。

初期按工具分为 `GetContext`、`ListCapabilities`、`PrepareDraft`、`SubmitDraft`、`GetOperation` 等小接口即可。基础抽取围绕本期调用点，不迁移整个 Server。

## 6. `/v1/responses` 接入契约

### 6.1 地址、模型与兼容性

保留现有本机 Provider 配置，增加 `api_type=responses`、能力探测结果及探测时间。将 `base_url` 明确定义为 API 根（例如 `https://api.openai.com/v1`），统一规范化尾斜线；请求拼接 `/responses`。若输入完整 `/v1/responses` 地址，在保存配置时规范化一次，避免 `/v1/v1/responses` 或 `/responses/responses`。

语言模型仍由用户配置，不默认换模型或提高推理档位。能力探测分别记录：基础文本、function calling、流式、图像输入、结构化输出、状态延续。`GET /models` 成功只能证明列表接口可用，不能证明工具调用可用。

关键门槛为 **Responses + 自定义函数调用**。若服务只返回文本，明确告知不能进入完整执行模式，不静默切回 Chat Completions。图像分析、严格 Schema 等属于独立能力；缺少时按能力收缩并标明限制。

### 6.2 请求形态

以下使用占位模型名，是最小协议示意；实际请求还包含系统边界、当前上下文和本轮可用工具。

```json
{
  "model": "<configured-model>",
  "instructions": "你是 Local Dream 创作助手。使用可用工具完成用户目标；执行事实以工具结果为准。",
  "input": [
    {"role": "user", "content": [{"type": "input_text", "text": "查询当前项目可用的生成方式"}]}
  ],
  "tools": [{
    "type": "function",
    "name": "get_capabilities",
    "description": "读取当前项目可执行的生成模式、参数约束和引用能力。",
    "strict": true,
    "parameters": {"type": "object", "properties": {}, "required": [], "additionalProperties": false}
  }],
  "tool_choice": "auto",
  "parallel_tool_calls": false,
  "stream": true,
  "store": false,
  "max_output_tokens": 6000
}
```

自定义函数定义是扁平 `type/name/parameters`，不能沿用 Chat Completions 的嵌套结构。工具请求从 `output` 的 `function_call` 项读取；结果用相同 `call_id` 的 `function_call_output` 送回。默认顺序调用先把副作用、依赖和恢复做好，后续仅对独立只读工具开放受控并发。严格 Schema 和业务参数校验同时保留。参见 [OpenAI Function calling](https://developers.openai.com/api/docs/guides/function-calling)。

`temperature`、推理配置、`include` 等按实际 provider/model 能力发送；不要复制现有固定 `temperature: 0.4`。如需最终结构化输出，使用 Responses 的 `text.format`，不发送旧 `response_format`。本方案的执行参数优先通过工具参数承载，用户说明可以保持自然语言。

### 6.3 执行循环

1. 保存用户消息并创建 Run，分配固定 `run_id`，返回 202。
2. Runner 读取服务器控制的项目范围、上下文和技能目录，调用 Responses。
3. 文本增量转成界面事件；聚合每个 output item，完整响应落库。
4. 对完成的函数参数做 JSON/Schema/项目归属/能力/执行授权校验。未完整收到响应时不执行媒体提交等副作用。
5. 工具完成后保存结果，并将对应 `function_call_output` 加入下一次请求；执行失败也返回结构化错误，让 Agent 有机会修正。
6. 需要用户回答或等待媒体完成时持久化暂停点，释放 Runner；不保持一个请求睡等几个小时。
7. 用户回答、媒体完成或重启恢复后，在同一 Run 恢复循环。
8. 达到目标才结束 Run。单次模型响应结束不代表整个创作完成。

工具结果示例：

```json
{
  "type": "function_call_output",
  "call_id": "call_example",
  "output": "{\"ok\":true,\"generation_id\":\"job_example\",\"status\":\"queued\",\"actual_mode\":\"multi_reference_video\"}"
}
```

服务端保存的 `generation_id` 才能生成任务卡；自然语言“已提交”不会触发任何成功状态。

### 6.4 会话状态与流式

默认本地保存协议 item，使用 `store:false` 和显式历史重放。保留可重放的消息、函数调用、结果，以及所选模型所需的 reasoning/phase 等 item 字段；不能只拼接 assistant 文本。`previous_response_id` 可作为已验证 provider 的优化，但不是本地恢复的唯一依据。不同 provider 或模型切换时创建新的协议上下文，用用户消息和业务摘要继续，不能跨服务重用 response ID。[OpenAI Conversation state](https://developers.openai.com/api/docs/guides/conversation-state)

Responses Client 处理 `response.created`、文本 delta、output item、函数参数增量与完成、`response.completed/failed/incomplete` 和错误。SSE 按事件边界拼接，处理多行 data、UTF-8 分片、空行及较长参数；EOF 不能当成功。采用独立 LLM HTTP Client，区分连接超时、首事件/空闲超时、每次调用 deadline 与整个 Run 的执行预算。[OpenAI Streaming](https://developers.openai.com/api/docs/guides/streaming-responses)

前端事件流断开只停止订阅。显式停止 Run 才取消当前模型请求与后续执行；已提交的视频另行决定取消范围。provider 密钥继续仅由后端读取，不写入消息、工具结果、事件或普通日志。

## 7. 技能系统：内置、自动发现、安装和按需加载

### 7.1 选用应用管理的技能运行方式

OpenAI 当前文档有与 shell 环境关联的原生 Skills 能力，不能说 Responses 不支持 skill；但支持 `/v1/responses` 的第三方服务未必支持原生技能上传和 shell 环境。因此本项目首先实现应用自己的技能目录与读取工具，只依赖普通 function calling，不依赖云端 shell。[OpenAI Skills](https://developers.openai.com/api/docs/guides/tools-skills)

文件包遵循 [Agent Skills 规范](https://agentskills.io/specification)：入口包含 YAML `name`、`description` 和正文，引用材料按需读取。产品专用版本、来源、启用范围和能力声明存入安装记录或独立应用清单，不假定它们都是标准字段。

三层加载：

1. 发现层：只把启用技能的 ID、名称、description、能力要求加入目录。
2. 激活层：模型调用 `load_skill(skill_id)` 读取该版本 SKILL.md。
3. 参考层：模型调用 `read_skill_resource(skill_id, resource_path)`，只读包内所需文件。

初期技能少，全量短目录足够；以后再增加 `search_skills` 的相关度检索，不先建向量数据库。一次 Run 固定 registry 快照和已加载内容哈希，更新技能只影响后续 Run。

### 7.2 自动选择与显式指定

默认 `skill_policy=auto`，允许零个、一个或多个技能；技能不是每条消息的必选项。

| 需求 | 预期选择 |
| --- | --- |
| 查询生成进度、解释参数 | 不加载 H3，直接调用查询工具/回答 |
| H3 文生/图生/首尾帧/多参考提示词或生成 | 加载 `h3-prompt-writing` 与相应参考文件 |
| 长故事分镜 | 先加载故事/分镜规划技能，再为各段加载 H3 技能 |
| 根据视频提取镜头节奏 | 加载视频分析技能，同时调用实际可用的媒体分析工具 |
| 用户输入 `/h3-prompt-writing` | 显式加载该技能，不依赖关键词判断 |
| 某 skill 要求尚不存在的图片生成工具 | 可用于文本规划，但不能声称已生成参考图 |

UI 支持 `auto / explicit / none`。明确指定的技能不可用时返回原因，不悄悄换技能；`none` 模式可以手动生成，但不标为已使用 H3 专业改写。技能未匹配时不多问用户，允许 Agent 直接完成普通任务。

发现与选用由模型语义判断；执行前后端仍校验该技能已启用、版本存在、资源路径合法、必要工具存在。显式指定优先于自动匹配；用户当前硬约束高于技能建议；技能正文不能修改后端工具权限。

输出 `skill.loaded` 事件，展示“使用 H3 提示词技能：多参考模式”等简短原因。保存可观察的选择结果，不保存或展示模型隐藏推理。

### 7.3 内置 `h3-prompt-writing`

源文件已核对：[本机 H3 Skill](C:/Users/21253/.codex/skills/h3-prompt-writing/SKILL.md)。实施时将 SKILL.md 与两个 references 完整复制为应用内置资源，使用 `go:embed` 随程序发布，避免依赖用户的 Codex 安装路径。保留来源、原文和内容哈希；不要只有名字却没有参考材料。

技能主要输出契约：

| 模式 | 当前产品对应 | 内容约束 |
| --- | --- | --- |
| T2VA | text_to_video | 三字段：integrated_multimodal_description / overall_soundscape / non_diegetic_music |
| I2VA | image_to_video | 三字段前加首帧对齐说明；图像通过 first_frame 连接 |
| FL2VA | first_last_frame | 首尾帧对齐说明，说明从开头到结尾的连续过程 |
| L2VA | 当前未暴露 | 可以理解/编写该类草稿，执行返回能力缺失，不假装支持 |
| Ref2VA | multi_reference_video | 六字段：subject_definitions / summary / retention_analysis / detailed_description / overall_soundscape / non_diegetic_music |

正文使用英文，保留台词、歌词、画内文字的原始语言。标签、说话人、时间与实际引用素材一致；保留用户明确动作、反应、镜头顺序和音轨需求。

H3 工具输出保存结构化 `prompt_sections` 和渲染后的完整 `prompt_text`；最终编译使用 `prompt_text`。旧五段式 `Intent.prompt` 不能作为 H3 中间表示。

参考指南的读取按任务发生：基础模式加载 base-en，Ref2VA 加载 ref-en，涉及共通时间/说话规则时补读 base-en。长片先规划段落，再为各段编写本地时间轴提示词；不能把 30 秒全片提示词直接塞给每个短片。4–15 秒是本技能的写作建议范围，实际单段时长仍须与能力和帧网格一致；不支持的范围采用拆段或显式模式适配。

### 7.4 用户安装与 Agent 自行安装

需要同时支持两种“自行安装”：用户导入自己找到的 skill，以及 Agent 在允许的来源中按任务补充技能。它们都安装在应用的 `data/skills`，不会修改 Codex 的全局技能。

第一阶段安装入口支持本地目录/ZIP 导入；随后增加固定 Git 仓库子目录与指定 commit/tag。可在 UI 手工创建只有 SKILL.md 的技能，再导出为标准目录包。

安装事务流程：获取到 staging → 校验路径和大小 → 解析入口/引用 → 记录来源与 hash → 检查工具需求 → 原子移入版本目录 → 更新 SQLite 安装记录。失败清理本次 staging；不覆盖运行中版本。解压禁止越界路径、符号链接和 Windows 重解析点逃逸；读取资源也必须检查最终路径在包根内。

建议安装策略：

- `manual`：用户手动安装，Agent 可建议具体技能。
- `trusted_auto`：Agent 可以 `search_skill_catalog`、`inspect_skill_package`、`install_skill`；仅对用户事先配置的来源、固定版本、纯指令/参考文件包自动安装。
- 未配置来源时，使用内置技能完成能完成的部分，不自动全网下载不明代码。

初版对 `scripts/` 可保存并标记“不可执行”，需要脚本执行的技能显示“部分兼容/不可用于该流程”；不能把“装成功”展示为“所有能力可用”。后续如确需脚本，另外设计限权执行环境，不能借安装功能给 Agent 开放本机终端。应用检查 `allowed-tools` 只是兼容性声明，不能凭技能文件给自己新增权限。

自动安装属于可观察的运行步骤，结果含 skill_id/version/source/hash；一旦安装成功，本 Run 显式加入该版本，而不是偷偷替换已激活的技能。卸载实际是禁用/解除安装选择；保留仍被历史 Run 引用的内容快照。

额外推荐的应用内技能按实际业务逐项编写：故事分镜、长视频段落规划、参考视频分析。它们与 H3 技能分工，不把所有知识塞进一个超长系统提示词。

## 8. 素材理解与引用绑定

### 8.1 先区分“拥有素材”与“模型看过素材”

现有 interpret 只把素材元数据转为文本，`/media/...` 地址也不能直接被远端模型访问。新流程分层处理：

- 素材元数据：类型、时长、尺寸、名称、项目归属，不伪造视觉内容。
- 图片：在 provider 支持图像时发送受控尺寸的图片内容；先以 data URL 适配，避免要求开放本机端口。
- 视频：先用本地 ffprobe 获取时长等信息，按时间点抽取有时间戳的帧供视觉模型理解；镜头/动作分析记录覆盖区间与采样限制。
- 音频：元信息与音量等本地可测信息不等于听懂内容；转录/音色分析使用另行验证的能力。保持语言模型编排接口为 Responses，不假定所有兼容模型能直接接收原生音视频。
- 用户文档：后续新增文档解析与引用，不塞入现有只处理图/视频/音频的资产上传分支。

只把用户本次选中或明确授权使用的素材内容发送到其配置的模型服务。首次配置素材分析时清晰显示数据去向；已启用后不逐帧重复确认。项目中其他素材默认只列元数据，避免将整个素材库发送给模型。

媒体预处理属于后端工具实现，执行固定程序和参数模板；不存在“让模型提供 shell 命令再运行”。如果 ffmpeg/ffprobe 未配置，工具返回明确 capability_missing；不能把视频文件名当作视频分析结果。

分析结果以 `asset_hash + analysis_kind + model + recipe_version` 缓存，保存证据时间戳。未知角色外观、不可识别台词和未覆盖镜头必须留空或标为未知。需要精确动作时增加采样或请用户说明，而不是补写为事实。

### 8.2 持久化语义角色和实际编译顺序

新增 `AssetBinding`，至少包含：

```json
{
  "asset_id": "asset_example",
  "kind": "image",
  "role": "character_reference",
  "position": 0,
  "label": "<Picture 1>",
  "source_track": "visual",
  "use_description": "提供角色外观",
  "content_hash": "<asset-hash>"
}
```

常用 role 为 first_frame、last_frame、character_reference、style_reference、motion_reference、voice_reference 等。role 表达用户意图，不承诺模型能严格锁定效果；binding resolver 将其映射到当前模式真正支持的端口。

具体规则：

1. API 和工具传 `asset_id`，服务端重新查项目与文件，忽略模型给出的任意路径/URL。
2. 每一类引用由服务端固定顺序；`<Picture 1>` 映射实际第一个图像槽位，不能按文件名临时排序。
3. `<Subject N>` 是提示词中的主体概念，可关联一个或多个素材，不是 ComfyUI 插槽 ID。
4. `<Video N>` 与 `<Audio N>` 独立编号；视频有声音不代表自动成为 Audio 引用。
5. 模式不支持的角色组合返回 `unsupported_binding`。例如同时要求硬首尾帧与当前 Ref2VA 的额外参考，不能只改文字就声称全部支持。
6. 提交前冻结完整绑定清单、素材快照和能力版本，编译后保存实际映射；后续编辑产生新版本。

首期可以由绑定清单确定排序后转换为旧 `asset_ids`，继续调用现有编译器。需要视频音轨或新关键帧组合时，再单独扩展编译器及其测试。

## 9. 业务草稿与工具设计

### 9.1 将创作意图变成可执行草稿

核心对象为 `GenerationDraft`，包括 `id/conversation_id/project_id/revision`、用户原需求、生成目标、约束、规范化参数、引用绑定、H3 结构、技能版本、能力版本和校验报告。

保存三组信息：

- `requested`：用户明确提出的模型、时长、画幅、台词、动作和素材角色。
- `proposed`：Agent 建议及其合理默认值。
- `effective`：经过后端验证、最终可执行的参数和真实模型/模板标识。

每个硬约束记录来源，用户显式选择和要求优先。普通氛围、镜头措辞等低风险细节可自行完善；模型更换、改变角色、删除台词、改变输出数量等有实质影响的调整必须在授权范围内。

`prepare_generation` 返回规范化草稿、需要回答的问题和错误，不做 ComfyUI POST，也不复制素材。它不能简单调用有副作用的 `compilePrompt`。提交时重新验证可变能力/素材是否仍匹配，若有变化要求重新准备该 revision。

执行凭据绑定 `draft_id + revision + content_hash + scope`。用户确认版本 3 后，Agent 修改了参数得到版本 4，旧确认不能用来执行版本 4。

### 9.2 首期工具清单

项目范围从 Run 注入，模型不任意指定或更换 project_id。资源工具内部统一检查归属，即使知道其他项目的 ID 也不能越界读取。

| 工具 | 主要输入/输出 | 效果与所属阶段 |
| --- | --- | --- |
| `get_project_context` | 当前项目、选中作品、创作偏好摘要 | 只读，首期 |
| `list_assets` | 类型/查询条件 → 范围内的素材摘要 | 只读，首期 |
| `inspect_asset` | asset_id、分析目的 → 有依据的媒体描述 | 首期图片；视频/音频按能力扩展 |
| `get_capabilities` | 模式筛选 → 可执行模式、限制、降级来源 | 只读，首期 |
| `load_skill` / `read_skill_resource` | skill_id/包内路径 → 对应版本内容 | 只读，首期 |
| `prepare_generation` | 创作参数、提示词结构、绑定 → draft_id/revision/校验结果 | 只保存草稿，首期 |
| `submit_generation` | draft_id/revision → generation_id、有效参数 | 创建媒体任务，首期 |
| `get_artifact_status` | generation_id/sequence_id → 实际状态与输出 | 只读，首期 |
| `watch_artifacts` | 产物 ID → 已注册等待关系 | 持久化等待，首期 |
| `ask_user` | 问题、选项、目标 draft/revision → pending_action_id | 暂停并等回答，首期 |
| `prepare_sequence` / `submit_sequence` | 多段故事/参数 → sequence 草稿/LongVideo | 长视频阶段 |
| `revise_artifact` | 目标、revision、修改说明 → 新草稿/分叉影响范围 | 长视频阶段；确认后复用提交工具 |
| `search_skill_catalog` / `inspect_skill_package` / `install_skill` | 配置来源中的 ID/版本 → 安装结果 | 自行安装阶段 |
| `export_sequence` | 序列、输出设置 → 导出任务/最终 MP4 | 后续导出能力 |

工具名代表模型调用契约，底层复用相同的服务方法。工具数量不等于独立服务数量，不为每个工具创建一个进程。

取消/重试可以在 UI 直接操作首期已有接口，后续作为 Agent 工具开放时也必须经过同样的范围和幂等检查。首期不开放任意 HTTP、SQL、文件写入、工作流导入或全局 ComfyUI interrupt。

### 9.3 执行模式

将两个维度分开：`skill_policy` 决定是否使用技能，`execution_policy` 决定生成动作如何授权。

| 执行模式 | 行为 |
| --- | --- |
| plan_only | 可分析、编写、修改草稿；禁止创建媒体任务 |
| ask | 默认给出具体草稿；只有授权不充分或方案发生实质变化时请求执行确认 |
| auto | 用户在本次创作范围内允许执行；满足服务端限制的草稿直接提交 |

用户明确说“用这些参数直接生成一条”即可形成该条任务的执行授权，无需再问相同问题；“帮我看看”“先改提示词”不等于生成授权。单靠模型返回 `approved=true` 不生效，授权来自 UI 操作或持久化用户指令与后端策略。

自动模式限制本轮媒体任务数量、总时长、自动重生成次数及 LLM 调用预算。首期默认不因为质量主观评估自动无限重抽；明确的格式修复可做有限重试。每个自动后续操作仍检查当前用户意图和项目 revision。

## 10. Run、媒体任务、等待与多轮会话

### 10.1 三类状态不可混用

```text
Conversation：多轮消息与创作上下文
  └─ Run：一次用户目标的执行
       ├─ model/tool step
       ├─ Draft：可编辑方案
       └─ Artifact：Generation / LongVideo / Export
```

Run 状态建议：`queued → running → waiting_user / waiting_external → running → completed`，另有 `failed/cancelled/interrupted`。

媒体任务继续沿用自己的 queued/running/completed/failed/cancelled 和 submission phase。`response.completed` 只代表一次模型调用结束；`submit_generation` 成功只代表已建任务；Run 是否等待成片由本次目标决定。用户要成片时，Run 持续到媒体任务完成或明确失败；用户只要求“提交后告诉我编号”时可完成 Run，同时保留任务卡实时更新。

### 10.2 不用模型循环轮询进度

`submit_generation` 返回真实任务 ID；`watch_artifacts` 立即返回“等待关系已保存”，Runner 将状态设为 waiting_external。现有 watcher 的完成/失败落库后发通知；轻量定时核对作为丢事件和重启后的补偿。

通知以 `artifact_id + terminal_version` 去重，将等待 Run 排入恢复队列。执行恢复前重新读取当前状态，不用旧事件内容覆盖数据库。所有工具 call 都已有结果后再挂起，避免协议历史留着缺失结果的函数调用。

视频生成期间可继续聊天：等待外部任务的 Run 不占语言模型执行锁。每个 conversation 同时只有一个模型执行片段；新的用户指令先更新会话 revision，再串行调度。旧 Run 恢复时读取最新 revision：若目标已被修改或取消，只更新产物卡，不按旧计划继续自动提交。

### 10.3 停止、恢复和提交不确定性

- 关闭页面：只断开事件订阅，Run/媒体任务不受影响。
- “停止 Agent”：取消当前模型调用和未执行步骤，已提交视频继续或按用户选择取消；响应明确列出仍在运行的产物。
- “停止本轮全部”：逐个取消该 Run 所属任务，沿用现有 queue/client_id 核对，不能中断其他来源任务。
- 进程重启：恢复 waiting_user 原问题，恢复 waiting_external 等待关系，检查 running 的中断点；先核对已落库操作，后继续模型调用。
- Responses 网络错误：未完成输出不执行副作用；重试可能产生新 call_id，业务幂等不能依赖 call_id。
- ComfyUI POST 超时：继续已有 queue/history/client_id 核对语义。结果未知时禁止 Agent 自行新建任务“试一次”。

### 10.4 两层幂等与原子关联

第一层针对工具回放：对 `(run_id, provider_call_id)` 唯一，已完成的调用返回旧结果。

第二层针对业务意图：服务端生成 `operation_key`，绑定 `action + draft_id + revision + variant_index`。同一个已确认草稿的相同提交，不论模型换了 call_id、网络重试还是刷新页面，都返回同一个 generation_id。用户明确“再生成一个”才分配新的 variant_index。

**operation 与新 Generation/LongVideo 的关联必须在同一 SQLite 事务提交，然后再启动 worker。** 若只在建任务后单独保存工具结果，进程可能在二者之间崩溃，重启会产生重复任务。新增服务要支持事务内创建而不启动执行，外层提交后再调度；这沿用现有长视频 child generation 原子关联的思路。

对于派生副作用（取消、安装、导出等），同样记录 operation 状态并提供 reconcile；不能承诺跨数据库与外部服务的绝对 exactly-once。目标是本地操作可恢复、未知外部结果不盲目重试。

## 11. 最小持久化模型与迁移

保留现有 projects/assets/generations/long_videos 的表与 ID。Agent 表使用独立迁移记录，例如 `agent_schema_migrations`，启动时执行事务迁移；不直接改现有 `schema_version=1` 的语义而导致旧检查拒绝启动。

稳定的查询字段用列，模型协议和不同草稿的内容用 JSON。建议分阶段落地：

| 表 | 首期关键列 | 用途 |
| --- | --- | --- |
| agent_conversations | id, project_id, title, revision, summary_json, created_at, updated_at | 会话与项目上下文 |
| agent_messages | id, conversation_id, run_id, role, content_json, created_at | 用户可见消息及引用 |
| agent_runs | id, conversation_id, source_message_id, status, phase, execution_policy, skill_policy, checkpoint_json, usage_json, error_json, timestamps | 运行与恢复点 |
| agent_items | id, run_id, model_step, item_index, response_id, type, payload_json | Responses 协议 item；模型返回和本地工具结果 |
| agent_drafts | id, conversation_id, run_id, kind, revision, content_hash, payload_json, timestamps | 生成/分镜方案 |
| agent_operations | id, operation_key, run_id, tool_name, provider_call_id, status, resource_type, resource_id, args_json, result_json, timestamps | 工具执行、幂等、产物关联 |
| agent_pending_actions | id, run_id, kind, draft_id, draft_revision, status, payload_json, answer_json, timestamps | 提问/执行授权，刷新与重启后仍可回答 |
| agent_events | id, conversation_id, run_id, type, payload_json, created_at | 单调递增事件 ID，用于 SSE 重放 |
| skill_installations | id, name, version, source, hash, enabled, capabilities_json, timestamps | 技能安装阶段引入 |
| asset_analyses | id, asset_id, content_hash, kind, model, recipe_version, result_json | 多媒体分析阶段引入 |

首期 waiting_artifacts 和 skill 快照可放 `agent_runs.checkpoint_json`，无需立刻拆很多关联表。索引至少包括 conversation 消息/事件顺序、run 状态、operation_key 唯一、run/call_id 唯一、draft_id/revision 与分析缓存键。

当前 SQLite `SetMaxOpenConns(1)`：事务内只用同一个 tx，不能再调用隐式使用 db 的 helper，否则可能等待自己释放连接。任何模型请求、素材处理或 ComfyUI 调用都不在 SQL 事务或 `s.mu` 持锁期间进行。

消息编辑采用新版本/从某消息分叉，不删除已经执行的事实。旧任务与文件保留；重放历史对话不重放媒体提交。会话摘要只压缩已完成的历史，必须保留用户硬约束、引用 ID、最新草稿、待回答问题和任务事实；未完成的 call/output 配对不拆开压缩。

## 12. 对前端的 HTTP 与事件接口

### 12.1 首期接口

| 方法与路径 | 行为 |
| --- | --- |
| `POST /api/agent/conversations` | 创建项目内会话，返回 201 |
| `GET /api/agent/conversations?project_id=...` | 会话列表 |
| `GET /api/agent/conversations/{id}` | 消息、草稿、运行摘要、待答问题及事件游标快照 |
| `POST /api/agent/conversations/{id}/messages` | 保存消息并调度 Run，202 返回 message_id/run_id |
| `GET /api/agent/conversations/{id}/events?after=...` | SSE 补发历史事件并订阅新事件 |
| `GET /api/agent/runs/{id}` | 当前运行状态与关联产物 |
| `POST /api/agent/runs/{id}/answers` | 回答指定 pending_action，原子恢复；重复回答幂等 |
| `POST /api/agent/runs/{id}/cancel` | 停止 Run，参数明确是否取消该 Run 媒体任务 |
| `PATCH /api/agent/drafts/{id}` | 带 expected_revision 修改草稿，冲突返回 409 |
| `POST /api/agent/drafts/{id}/execute` | UI 执行草稿，绑定 revision/hash，调用同一个提交服务 |
| `GET /api/agent/skills` | 启用技能、来源、可用性 |
| `POST /api/agent/skills/install` | 用户导入/安装；与 Agent install 工具共用安装服务 |
| `PATCH /api/agent/skills/{id}` | 启停、选用版本；不改运行中快照 |

后续按实际需求增加 archive、fork、技能导出等，不要求第一阶段实现完整平台管理后台。

发送消息示例：

```json
{
  "client_message_id": "<browser-generated-id>",
  "text": "用这个角色图参考视频动作，制作一段8秒短片",
  "asset_ids": ["asset_character", "asset_motion"],
  "artifact_refs": [],
  "skill_policy": "auto",
  "skill_ids": [],
  "execution_policy": "ask",
  "expected_conversation_revision": 4
}
```

会话决定 project_id，重复 client_message_id 返回同一消息/Run。公开错误统一为 `{error, code, details?, retryable?}`；模型内部报错和密钥不直接透传 UI。

### 12.2 事件契约

```text
id: 107
event: artifact.updated
data: {"run_id":"run_example","generation_id":"job_example","status":"running","progress_known":false}
```

事件包括 message.delta/message.completed、run.updated、skill.loaded、tool.started/tool.completed、draft.updated、question.required、artifact.created/artifact.updated、run.completed/run.failed。

事件先落库再广播；持久化增量可按小时间窗合并，最终 message.completed 含完整文本，以免丢分片或重复展示。快照带 `last_event_id`，客户端按 ID 去重，断线后重放；游标过期要求重新取快照。

`message.delta` 只用于说明文字，不能驱动任务创建。工具名称由后端映射为“检查素材”“生成分镜”“等待视频”等用户可读描述，原始协议放可选诊断视图。

SSE 用单独 fetch stream/EventSource 客户端，避免现有 60 秒 JSON helper。检查反向代理缓冲与心跳；若前端跨源部署，更新 CORS 的 PATCH、Last-Event-ID 等需求，同源部署保持简单。

## 13. 长视频与“只修改第二段”的真实语义

复用 `prepareSequence`、`calculateClipTiming`、dispatcher、`regenerateOrFork`，并把它们从 HTTP 层抽成可调用服务。

Agent 先生成故事大纲与镜头清单，规范化各段预计有效时长，再由 Go 计算输入帧、上下文重叠和裁剪结果，返回实际可交付时长。H3 单段提示词中的时间轴从该段本地 0 秒开始；输入包含承接上下文时必须描述正确的接续状态，裁剪后成片时长与输入时长分别保存。

每段保存人物/场景连续性、必要对白、进入/离开状态、引用清单和依赖段落。Agent 不能仅凭“第2段”修改一个扁平大字符串。

当前实现的局部重生成：

- 最后一段重生成会替换该段的任务关联，并保留旧 Generation 作为 superseded。
- 中间段重生成会创建新序列，保留此前片段，从修改段起重新生成后续依赖段。
- 因此“第二段改一下，第三段完全不动”在 Motion Context 链路上并非天然可满足。准备草稿时展示受影响范围；用户需要独立镜头时再选择独立剪辑结构，不能隐瞒后续重算。

当前没有合并导出工具。完整成片导出阶段需增加 export job，固定 ffmpeg 调用，检查视频尺寸/帧率/音轨，处理已裁掉的 overlap，验证输出文件存在且可探测播放。`LongVideo.status=completed` 与 `export.status=completed` 分开；只有导出任务完成才能宣称已获得统一 MP4。

图片生成、配音、字幕、音乐和镜头级编辑都是可扩展工具，但要先实现 provider/本地后端能力，再向技能目录声明，不提前展示可执行按钮。

## 14. 实施顺序与每阶段交付

阶段是可以单独验收的业务切片，按依赖推进；不按聊天轮数或文件数量报告进度。

| 阶段 | 改动重点 | 用户得到的结果 | 必要验收 |
| --- | --- | --- | --- |
| A：接入基础 | 抽出创建 generation/sequence 的最小服务；统一只读能力与草稿校验；Responses Client；内置 H3 包 | 已有生成方式保持可用，Agent 可按 Responses 完成工具调用 | 模拟 provider 工具闭环、现有生成测试、地址规范化与错误状态 |
| B：单视频完整 Agent | 会话/Run/草稿/操作/事件；自动选 skill；项目素材；图片理解；授权提交；真实结果卡 | 对话生成单条视频并继续修改，刷新可恢复 | 文本/图片场景、零技能查询、实际任务关联、断线不取消、重复提交幂等 |
| C：多媒体与技能安装 | 视频抽帧、音轨能力说明；用户导入/创建/启停；受信来源自动安装 | Agent 能使用用户技能，按素材内容规划任务 | 无效包不生效、版本固定、缺工具显示兼容性、引用角色正确 |
| D：长视频创作与修改 | 故事/分镜技能、序列草稿、时长计算、段落操作、分叉 | 自然语言制作长片段序列、从指定段修改 | 实际时长/重叠帧一致，中段修改保留原序列，后续依赖范围正确 |
| E：完整成片与更多制作工具 | 合并导出、可选图像/配音工具、素材自动组织 | 单个可下载成片，可逐步支持更完整广告/短片流程 | 输出媒体真实可用，工具能力和界面声明一致 |

对“Agent 可自行安装”的最低完整要求是 C 阶段，包括受信目录自动安装；只有用户手动导入还不算此需求完成。B 阶段是第一个可用闭环，但不是整份需求全部交付。

可先交付 B 的简单对话 UI，无需等无限画布、技能社区或所有多媒体分析到位。默认使用现有 ComfyUI/H3，不增加第二套视频服务。接入前先核对用户配置的 Responses provider 是否满足工具调用与所需视觉能力；这需要实施阶段的授权 API 请求，本次规划没有调用。

### 14.1 第一块具体实现切片

建议从 **共享建任务服务 + Responses 工具闭环 + H3 内置技能** 开始，而不是先重做页面。

1. 为现有 `generationsHandler` 抽取规范化/校验与任务准备函数，明确区分“验证”“事务内建记录”“事务后启动 worker”。不改变既有模板编译、seed、client_id 和状态恢复语义。
2. 加最小 Responses Client，先支持文本、函数调用、流式完成/失败，并用模拟 HTTP provider 测工具回传。
3. 内置 H3 目录与只读加载工具，默认目录中仅有已启用技能元数据。
4. 加 `prepare_generation`，让模型依据素材和实际能力输出一个可执行草稿；H3 草稿保留完整章节和绑定。
5. 加带 operation_key 的提交适配器，事务保存操作与任务关联后才启动现有执行器。
6. 接一条最小会话 API，使“文本需求 → 自动加载 H3 → 草稿 → 已授权提交”贯通；再补足持久化等待、SSE UI 和重启恢复，完成 B。

A 阶段的模拟 provider 只能证明协议和工具循环；真实媒体完成需要实施阶段单独跑一次获授权的小样本，不能把测试构造的 generation_id 当真实视频。

### 14.2 现有功能兼容

- 保留普通模式表单及直接 `/api/generations` 路径；两入口调用同一服务和校验逻辑。
- Agent 使用能力/策略选路后，如果已明确选择普通模式，提交传明确 `strategy=normal`；需要长视频时走 sequence 服务，避免被当前自动时长阈值再改变路由。
- 旧 `/api/agent/interpret` 暂时作为 `plan_only` 适配层，单次返回旧 Intent 给原按钮；新 Agent 页面使用新 API。兼容层不能承担完整 H3 草稿的执行，避免格式信息丢失。
- `use_agent` 从执行语义中退役，旧客户端字段可以暂时接受但不赋予任何特殊权限；仍校验完整 prompt。
- Provider GET 继续隐藏密钥；配置测试明确区分“模型列表可用”“Responses 可用”“工具调用可用”“图片理解可用”。
- 新 Agent 表与 data/skills 追加建立；现有资产、项目、Generation ID、输出 URL 和用户改动保留。
- 开发过程使用临时数据库/隔离端口验证。正式服务重启与真实生成放到用户授权的部署/测试步骤。

## 15. 最少但足够的验证

### 15.1 自动化验证

| 场景 | 必须证明的结果 |
| --- | --- |
| 查询进度 | 可以零技能完成，不误触生成 |
| 自动/显式 H3 选择 | 自动匹配合适参考文件，显式选择不会被忽略 |
| 不用 skill | 不隐式注入 H3，不把普通回答标为技能结果 |
| 三字段/六字段与引用 | 字段齐全、标签存在、顺序稳定，原台词与硬约束保留 |
| 只要方案 | 工具层拒绝媒体提交，即使模型误调用 |
| 不支持的模型/2K/L2VA/引用组合 | 给出明确差异和选项，不静默替换 |
| Provider 流中断/failed/incomplete | 不标完成、不执行半截参数、不无条件重提 |
| 模型重复 call_id 或换 call_id 再提交 | 同一草稿 revision/variant 只有一条媒体任务 |
| 落库与启动之间退出 | 重启恢复同一 operation 和任务，没有重复创建 |
| 已提交 ComfyUI 但 POST 回执丢失 | 核对既有 queue/history/client_id，不重新提交 |
| 等待期间刷新/重启 | 待答问题、草稿、工具结果与任务关联仍在 |
| 修改旧草稿/重复点击答案 | revision 冲突明确，旧授权不可执行新参数 |
| 多项目引用 | 不能通过模型构造 ID 读取或操作别的项目资源 |
| 技能包越界/不支持脚本 | 拒绝越界，缺执行器不假装技能全面可用 |
| 更新技能后恢复旧 Run | 仍读取原版本内容 |
| 改中间段 | 保留原序列，新序列只复用允许复用的前缀 |

执行原项目 Go 测试以及受影响的前端 typecheck/build。只在新改动、失败或未解决的具体问题出现时扩大测试；不写仅复述实现的测试。

### 15.2 真实闭环验收

实施完成后，选择用户授权的一组轻量参数：

1. 文本输入，自动选择 H3，创建并完成一条真实视频，输出能读取，记录真实 prompt_id 与 generation_id。
2. 用户选一张图，使用图生视频模式，核对提交图中的实际 first_frame 链路与最终结果。
3. 在同一会话修改作品，得到新 generation 而不是覆盖原记录。
4. 长视频阶段再做两段接续和一次指定段修改，核对实际时长和原序列保留。
5. 安装一个纯指令测试 skill，自动匹配一次，再显式关闭，确认关闭后不加载。

视频视觉/声音效果验收要在用户同意后进行；纯代码/Schema 检查不能证明角色稳定、动作正确、声音相同或成片可用。所有验证记录分开写：实现完成、构建/自动测试、模拟集成、真实模型调用、真实视频生成、视觉/音频验收。

## 16. 运行记录、预算与排错

每次 Run 记录模型/provider 标识、使用量（provider 返回时）、模型调用数、工具耗时、加载技能版本、草稿版本、关联任务、失败阶段及可恢复性。普通日志只用 ID 和错误码；大媒体 base64、API key、完整私人素材内容不写日志。

建议初期可配置上限：每次持续执行最多 12 次模型调用，每次函数参数修复最多 2 次；上下文预算按所选模型设置，不猜默认上下文长度。达到预算保存当前草稿并暂停/返回可恢复错误，而不是丢弃结果。后台等待视频期间不累计模型调用，也不靠持续 LLM 请求保活。

模型请求超时、工具参数错误、素材缺失、能力不足、GPU 生成失败和提交结果未知分别给出不同 code。`retryable=true` 不等于可以重生成视频；重试策略必须由对应工具的恢复语义决定。

优先排查四个 ID：conversation_id → run_id → operation_id → generation_id/prompt_id。费用若没有 provider usage/定价依据，不输出虚构金额；显示已知 tokens、调用次数和媒体任务数量即可。

## 17. 暂不纳入首轮的事项

- 自主访问用户电脑、任意 shell/HTTP 调用、任意 ComfyUI 节点生成。
- 完整技能市场、账号体系、多租户计费和分布式任务调度。
- 为简单技能选择先建向量库，或为阶段规划先搭多 Agent 团队。
- 将所有模型提供商的原生 Skills、Conversations、后台任务协议统一封装成大平台。
- 没有真实视觉/音频工具时宣称自动审片；没有合成器时宣称一键成片。

这些不影响本次核心目标。需要扩展时增加明确能力及工具，再让现有 Agent 和技能选择机制使用它。

## 18. 关键决策清单与资料

| 决策 | 本方案结论 |
| --- | --- |
| 目标后端 | 当前 app，用户已确认 |
| LLM 接口 | `/v1/responses`，不静默 fallback 到 Chat Completions |
| 执行结构 | Go 内置单 Runner + 类型化工具 + SQLite |
| 内置技能 | 完整 `h3-prompt-writing` 和两个 references，嵌入发布 |
| 技能自动选择 | 默认开启，可选零/一/多技能，支持 `/skill` 和禁用 |
| 技能安装 | 用户导入/创建 + 配置受信来源后的 Agent 自动安装 |
| 主业务 | 理解需求、素材分析、创作规划、生成、等待、继续修改 |
| 长视频 | 复用现有序列/continuity；导出独立实现 |
| UI | 项目内 Agent 侧栏、步骤、问题、可编辑草稿、结果卡 |
| 完成标准 | 获得真实产物并可继续修改；测试与实机效果分开报告 |

参考资料：

- [OpenAI Function calling](https://developers.openai.com/api/docs/guides/function-calling)：工具声明、call_id 与工具结果。
- [OpenAI Conversation state](https://developers.openai.com/api/docs/guides/conversation-state)：会话与协议 item 延续。
- [OpenAI Streaming](https://developers.openai.com/api/docs/guides/streaming-responses)：Responses 流式事件。
- [OpenAI Skills](https://developers.openai.com/api/docs/guides/tools-skills)：原生技能与 shell 环境的集成边界。
- [Agent Skills 规范](https://agentskills.io/specification)：目录与 SKILL.md 格式。
- 即梦与 Updream 的产品观察链接见第 2 节；本地代码链接见第 3 节。

规划完整性检查：覆盖用户要求的 Responses 接口、内置 H3、用户安装、Agent 自行安装、自动技能选择，以及超出提示词优化的生成与作品修改闭环。外部 provider 的真实兼容性、实际生成质量和本次新增设计的运行效果仍需在实施阶段验证。
