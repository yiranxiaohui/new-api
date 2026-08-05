# New API Video 渠道类型设计

日期：2026-08-05
状态：已确认

## 背景

部分视频中转上游（例如 `code.shoestravel.xin`，提供 Veo 3.1 Quality/Fast/Lite 等模型）只暴露两个接口：

- `POST /v1/video/generations` — 创建任务
- `GET /v1/videos/{task_id}` — 查询任务

该协议与本项目自身的视频任务入口一致（「new-api 风格视频协议」），但现有任务适配器没有任何一个以 `POST /v1/video/generations` 作为上游提交地址：Sora/OpenAI 适配器提交走 `POST /v1/videos`，其余适配器均为各自私有路径。因此这类上游目前无法作为渠道接入。

## 目标

新增渠道类型 **New API Video**（ID 61），复用 Sora 任务适配器，仅按渠道类型分支差异点，使该协议的上游可以作为视频任务渠道接入。

非目标：不支持 remix；不做 seconds/size 计费倍率；不新建独立适配器包。

## 设计

### 1. 常量与路由

- `constant/channel.go`
  - `ChannelTypeNewAPIVideo = 61`（紧跟 `ChannelTypeNewAPI = 60`，位于 `ChannelTypeDummy` 之前）。
  - `ChannelTypeNames[61] = "New API Video"`。
  - `ChannelBaseURLs` 追加索引 61 的空串占位（无默认 base URL，必须由渠道配置提供）。
- `relay/relay_adaptor.go` `GetTaskAdaptor`：`ChannelTypeNewAPIVideo` 分支返回 `&tasksora.TaskAdaptor{}`（与 Sora/OpenAI 同一个 case 或新 case 均可，跟随现有 switch 风格）。
- `common/endpoint_type.go` `GetEndpointTypesByChannelType`：61 与 Sora 一致，返回 `[]{EndpointTypeOpenAIVideo}`。

### 2. Sora 适配器分支（`relay/channel/task/sora/adaptor.go`）

适配器已有 `ChannelType` 字段（`Init` 时从 RelayInfo 填充），按 `a.ChannelType == constant.ChannelTypeNewAPIVideo` 分支：

- **提交 URL**（`BuildRequestURL`）：返回 `{base}/v1/video/generations`。remix action 在类型 61 下直接返回错误（该协议无 remix 端点）；拒绝点放在 `ValidateRequestAndSetAction` 的 remix 分支，返回 400 本地错误。
- **计费**（`EstimateBilling`）：类型 61 返回 `nil`——不做 seconds/size 倍率，价格完全由管理端按模型配置的固定按次价决定。
- **查询**（`FetchTask`）：`GET {base}/v1/videos/{taskID}`，与现状相同，无需改动。
- **结果解析**（`ParseTaskResult`）：状态映射复用现有逻辑（queued/pending → 排队，processing/in_progress → 进行中，completed → 成功，failed/cancelled → 失败）。区别：completed 时需要从响应中提取视频 URL 填入 `TaskInfo.Url`（Sora 分支刻意留空，因为它走上游 content 端点；类型 61 的上游没有 content 端点）。
  - 由于 `ParseTaskResult(respBody []byte)` 签名中拿不到 channel type，采用无害的通用做法：在 responseTask 中新增兼容字段并按序尝试提取 `url`、`video_url`、`videos[0].url`、`data.url` 等常见字段；提取到则填 `TaskInfo.Url`，提取不到保持留空。对现有 Sora 渠道无影响（OpenAI 官方响应没有这些字段，行为不变）。
  - `TaskInfo.Url` 非空时框架会将其落入任务 `PrivateData.ResultURL`（与其他非 Sora 适配器一致）。

### 3. 成片下载（`controller/video_proxy.go`）

不把 61 加入 `ChannelTypeOpenAI, ChannelTypeSora` 分支。类型 61 落入 `default` 分支：直接用任务存储的 `ResultURL` 经现有 SSRF 防护代理下载。客户端仍通过本站 `GET /v1/videos/{task_id}/content` 获取成片，行为与 Kling/Vidu 等渠道一致。

### 4. 前端

- `web/src/features/channels/constants.ts`：`CHANNEL_TYPES` 加 `61: 'New API Video'`；`CHANNEL_TYPE_DISPLAY_ORDER` 在 55（Sora）附近插入 61。
- `web/src/features/channels/lib/channel-utils.ts`：图标映射加 `61: 'OpenAI'`。

### 5. 测试

表驱动 + testify（require/assert），与 `relay/common/relay_utils_test.go` 风格一致：

- `BuildRequestURL`：类型 61 → `/v1/video/generations`；Sora/OpenAI 类型 → `/v1/videos`（回归）。
- `EstimateBilling`：类型 61 返回 `nil`（即使请求带 seconds/size）。
- `ParseTaskResult`：completed 响应各 URL 字段变体的提取；无 URL 字段时 Url 留空；Sora 官方响应形状行为不变（回归）。
- remix 拒绝：类型 61 的 remix action 返回 400。

## 计费链路确认

类型 61 走纯按次计费：无 OtherRatios 乘数，用户请求中不存在任何进入计费的数量字段，符合 AGENTS.md 计费安全不变量（无需新增边界钳制；不触碰 quota 转换路径）。

## 兼容性

- 渠道 ID 61 为新增，不影响既有渠道数据。
- responseTask 新增的兼容解析字段均为 `omitempty` 可选字段，对现有 Sora 渠道解析无行为变化。
- 数据库无 schema 变更。
