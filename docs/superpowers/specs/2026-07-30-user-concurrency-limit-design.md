# 用户级并发限制设计

日期：2026-07-30。已由用户批准。

## 背景

现有并发限制只有渠道级（`channel.max_concurrency`，`controller/relay.go` 调
`service.TryAcquireChannelConcurrency`）。单个重度用户可占满整条渠道的并发，
需要用户级的在途请求数上限。

## 配置与语义

- 全局默认：option `UserMaxConcurrency`（int），`0` = 功能关闭（默认）。
  运营侧「速率限制」设置卡配置。
- 每用户覆盖：`users.max_concurrency`（`*int`，AutoMigrate ADD COLUMN）。
  - `nil` / `0`：跟随全局默认
  - `-1`：该用户不限并发
  - `>0`：覆盖值
- 管理员经用户编辑抽屉修改（`PUT /api/user/`，同 `ratio` 字段模式）。
- `UserBase` 增加 `MaxConcurrency *int`，必须写入 `writeUserCache` 的
  Lua HSET（空串哨兵 ↔ nil 指针），并带缓存往返测试。

## 计数器与中间件

- 泛化 `service/channel_concurrency.go` 为按 string key 的 acquire/release
  （Lua 脚本已是 KEYS[1] 泛型；内存 fallback 改 `map[string]`）。
  渠道 key `channel_concurrency:<id>`，用户 key `user_concurrency:<userId>`。
- 新中间件 `middleware.UserConcurrencyLimit()`：TokenAuth 后取 userId 与
  生效上限；acquire 失败 → `abortWithOpenAiMessage(429, ...)`；成功 →
  defer release 后 `c.Next()`。流式/WS 在 handler 返回前占用槽位。
- fail-open：Redis 出错放行（与渠道一致）；Lua 300s TTL 兜底防泄漏。

## 挂载点

- `/v1` 组与 `/v1beta` Gemini 原生组：group-level `Use`，紧跟
  `ModelRequestRateLimit()`。
- MJ / Suno 任务提交路由：per-route 挂载；fetch/查询/图片下载不计入。
- `/pg` playground 不挂。

## 前端

- 系统设置 → 速率限制卡：「用户最大并发数」数字输入（0 = 不限制）。
- 用户编辑抽屉：「最大并发数」输入（空 = 跟随全局，-1 = 不限制）。
- i18n：en 为 key，补 zh；其余语言 fallback。

## 测试

- service：上限拒绝、release 后可再入、不减到负、miniredis Lua 路径。
- model：MaxConcurrency 缓存往返（nil / -1 / 5）。
- middleware：全局 0 不拦；超限 429；`-1` 绕过；结束释放。
