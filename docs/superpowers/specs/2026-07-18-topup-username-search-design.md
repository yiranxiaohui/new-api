# 充值订单历史支持按用户名搜索 — 设计文档

- 日期：2026-07-18
- 状态：已批准，待实现
- 范围：管理员端充值订单历史（TopUp）新增「按用户名搜索」，并在结果卡片展示用户名

## 背景与现状

充值订单对应模型 `TopUp`（`model/topup.go:14`）。订单历史 UI 是 `wallet` feature 里的弹窗 `BillingHistoryDialog`（`web/default/src/features/wallet/components/dialogs/billing-history-dialog.tsx`），管理员与普通用户共用同一弹窗，靠 `useIsAdmin()` 区分数据范围：

- 管理员 → `GET /api/user/topup`（全平台）→ `controller.GetAllTopUps`（`controller/topup.go:466`）→ `model.SearchAllTopUps`（`model/topup.go:309`）
- 普通用户 → `GET /api/user/topup/self`（仅自己，含 30 天时间窗口）→ `controller.GetUserTopUps` → `model.SearchUserTopUps`

当前搜索（两端一致）**只对 `trade_no`（订单号）做 LIKE**：

```go
query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
```

关键约束：

- `TopUp` 表**没有 username 列**，只有 `user_id`（`TopUp.UserId`，`model/topup.go:16`）。
- `TopUp.Username` 是 `gorm:"-"` 虚拟字段（`model/topup.go:25`），运行时由 `fillTopUpUsernames`（`model/topup.go:29`）从 `User.Username`（`model/user.go:81`）按 `user_id` 批量回填。`SearchAllTopUps` 末尾已调用该回填。
- 关键词清洗复用 `sanitizeLikePattern`（`model/token.go:88`）：转义 `_`、`!`，去 `%` 后长度需 ≥2，`ESCAPE '!'` 跨库兼容。**注意：`sanitizeLikePattern` 不会自动包裹 `%`——无 `%` 的关键词是精确匹配。** 因此本功能新增 `fuzzyLikePattern`（`model/token.go`）：剥离用户自带 `%`、转义 `_`/`!`、强制 `%keyword%` 包裹、模糊关键词长度需 ≥2，用于管理员这类「总是模糊匹配」的场景。
- COUNT 安全上限 `searchTopUpCountHardLimit = 10000`（`model/topup.go:266`）。

## 目标

管理员在充值历史搜索框输入一个关键词，能**同时**按订单号或用户名模糊匹配，并在结果卡片上看到用户名。

## 需求确认（已与用户敲定）

1. **仅管理员**：只改管理员链路（`SearchAllTopUps` / `GetAllTopUps` / 管理员视角 UI）。`SearchUserTopUps`（用户端）**不改动**。
2. **统一关键词**：单个搜索框，关键词同时模糊匹配 `trade_no` OR `username`（任一命中即返回）。
3. **卡片展示用户名**：管理员视角下，结果卡片在 User ID 徽章旁额外显示用户名徽章。
4. **模糊匹配**：管理员端订单号与用户名都用 `%keyword%` 包裹后模糊匹配，共用同一个 `pattern`。**这将管理员端订单号搜索从原先的「精确匹配」改为「模糊匹配」**（因 `sanitizeLikePattern` 不自动加 `%`、前端也不加 `%`，旧行为实为精确匹配）；此改动经用户确认为期望行为。用户端 `SearchUserTopUps` 语义不变。
5. **placeholder 统一文案**：管理员与普通用户都显示「按订单号或用户名搜索…」，实现上不按角色分支。

## 非目标（YAGNI）

- 不改普通用户端 `SearchUserTopUps` 及其 30 天时间窗口。
- 不引入订单号/用户名的字段切换 UI。
- 不改动搜索交互现状（当前每次击键即发请求、无防抖），保持原样，不扩大改动面。
- 不为 `TopUp` 表新增 username 列（继续用虚拟字段 + 运行时回填）。

## 方案（已选：方案 A 子查询）

在 `SearchAllTopUps` 中扩展 WHERE：订单号 LIKE **OR** user_id 属于「用户名 LIKE 命中的用户集合」。用 GORM 子查询表达式实现，不改变 `Model(&TopUp{})`。

### 后端改动

文件：`model/topup.go`，函数 `SearchAllTopUps`（`model/topup.go:309`）。

`keyword != ""` 分支由：

```go
pattern, perr := sanitizeLikePattern(keyword)
if perr != nil {
    tx.Rollback()
    return nil, 0, perr
}
query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
```

改为：

```go
pattern, perr := fuzzyLikePattern(keyword)
if perr != nil {
    tx.Rollback()
    return nil, 0, perr
}
usernameSub := tx.Model(&User{}).
    Select("id").
    Where("username LIKE ? ESCAPE '!'", pattern)
query = query.Where(
    tx.Where("trade_no LIKE ? ESCAPE '!'", pattern).
        Or("user_id IN (?)", usernameSub),
)
```

并在 `model/token.go` 的 `sanitizeLikePattern` 旁新增 `fuzzyLikePattern`：

```go
// fuzzyLikePattern 构造「包含匹配」的 LIKE 模式：无论用户是否输入 % 通配符，
// 都对关键词做 %keyword% 的模糊匹配。先剥离用户自带 % 再转义 _/!，
// 去空白后关键词长度需 >= 2。
func fuzzyLikePattern(input string) (string, error) {
    trimmed := strings.TrimSpace(input)
    trimmed = strings.ReplaceAll(trimmed, "%", "")
    if len(trimmed) < 2 {
        return "", errors.New("使用模糊搜索时，关键词长度至少为 2 个字符")
    }
    escaped := strings.ReplaceAll(trimmed, "!", "!!")
    escaped = strings.ReplaceAll(escaped, "_", "!_")
    return "%" + escaped + "%", nil
}
```

要点：

- 用 `tx.Where(...).Or(...)` 包一层，保证 `trade_no LIKE ... OR user_id IN (...)` 作为一个整体分组。
- 订单号与用户名共用同一个 `pattern`（同一次 `fuzzyLikePattern` 的 `%keyword%`）。
- `fuzzyLikePattern` 强制 `%` 包裹，因此关键词是「包含匹配」而非精确匹配；`_`/`!` 被转义为字面量，避免 `_` 被当作单字符通配符。
- `Model(&TopUp{})` 不变 → `Count`、`Find`、末尾 `fillTopUpUsernames(topups)` 全部照旧。
- 跨库安全：纯 GORM 子查询，无硬编码表名/方言函数；`username`/`user_id`/`id` 均非保留字。`ESCAPE '!'` 沿用现有跨库写法。
- `searchTopUpCountHardLimit` COUNT 上限不变。
- `controller/topup.go` **无需改动**（`GetAllTopUps` 已透传 `keyword`）。

### 前端改动

1. **类型**（`web/default/src/features/wallet/types.ts`，`TopupRecord`，约 255–274 行）新增：

```ts
username?: string // 管理员视角由后端 fillTopUpUsernames 回填；用户端为空
```

用可选：用户端 `/topup/self` 不回填，且与 fork 兼容。

2. **卡片展示**（`billing-history-dialog.tsx`，User ID 徽章旁，约 208–215 行）：保留原 User ID 徽章不变，管理员视角下若 `record.username` 非空则额外显示用户名徽章：

```tsx
{isAdmin && record.username && (
  <StatusBadge
    label={`${t('User')}: ${record.username}`}
    variant='neutral'
    size='sm'
    copyText={record.username}
  />
)}
```

3. **placeholder**（`billing-history-dialog.tsx`，约 116 行）：`t('Search by order number...')` → `t('Search by order number or username...')`（统一文案，不按角色分支）。

**不改动**：`api.ts`、`use-billing-history.ts`（`keyword` 已透传，逻辑无需动）。

### i18n

前端文案键（`web/default/src/i18n/locales/*.json`，扁平 JSON，英文源串作 key）：

- 复用已存在的 `"User"` 键（`en.json:4919` 已有），无需新增。
- 新增 `"Search by order number or username..."`：`en.json`（基准）、`zh.json`（回退，译「按订单号或用户名搜索…」）必须有；`fr/ru/ja/vi` 按 fork 既有做法加英文兜底。
- 旧键 `"Search by order number..."`（`en.json:3984`）：若全局无其它引用则一并移除；若仍被别处使用则保留。实现时用 grep 确认。

## 测试

### 后端（新增回归测试）

已存在 `model/topup_username_test.go`（fork 本地针对 `fillTopUpUsernames` 的契约测试），本次测试**加入该文件**，复用其建库范式：

- fixture：`truncateTables(t)`（`model/task_cas_test.go:64`，已清理 `users`/`top_ups`）。
- 建 User 需 `Username/Password/Role/Status/AffCode`（`AffCode` 有 UNIQUE 索引，各用户取不同值）。
- 建 TopUp 范式见现有测试。

新增 `TestSearchAllTopUps_ByUsername`（子测试驱动，`testify/require` 做 setup、`assert` 做校验），覆盖核心契约：

1. 关键词模糊匹配某用户名片段 → 返回该用户的订单（且不含其他用户的订单）。
2. 关键词完整/部分匹配订单号 → 命中（订单号现在是 `%keyword%` 模糊匹配，部分片段也命中）。
3. 关键词同时匹配多个用户名（如 `pay` 命中 `alice_pay`/`bob_pay`）→ 全部返回、无重复。
4. 关键词中的 `_` 被转义为字面量，不作单字符通配符（如 `b_b` 不匹配 `bob_pay`）。
5. 关键词两者都不匹配 → 空结果、total 为 0。
6. 单字符关键词被拒绝（`fuzzyLikePattern` 的 ≥2 长度守卫，防止 `%a%` 全表扫描）→ 返回 error。

保护对象是「管理员按用户名/订单号模糊搜索订单」这一新契约本身，以及 `_` 转义、最小长度守卫这两个安全不变量，符合项目「保护真实行为/回归路径」的测试标准；不引入随机/压力/日志断言类无效测试。

### 前端

- `bun run typecheck` 确保 `TopupRecord.username` 与卡片使用一致。
- 纯展示改动，不新增前端单测。

## 验收标准

1. 管理员在充值历史搜索框输入某用户名片段 → 列表返回匹配到的所有用户的充值订单。
2. 输入订单号片段（部分即可）→ 管理员端命中该订单（现为模糊匹配）。
3. 管理员视角结果卡片显示用户名徽章（有用户名时）。
4. 普通用户端 `/topup/self` 行为与 30 天时间窗口不受影响（`SearchUserTopUps` 未改动，仍为原精确/用户自控 `%` 语义）。
5. SQLite / MySQL / PostgreSQL 三库均可用（纯 GORM 子查询保证）。

## 影响文件清单

- `model/token.go` — 新增 `fuzzyLikePattern`（`%keyword%` 包裹 + `_`/`!` 转义 + 最小长度守卫）
- `model/topup.go` — `SearchAllTopUps` WHERE 扩展，改用 `fuzzyLikePattern`
- `model/topup_username_test.go` — 新增 `TestSearchAllTopUps_ByUsername`
- `web/default/src/features/wallet/types.ts` — `TopupRecord.username`
- `web/default/src/features/wallet/components/dialogs/billing-history-dialog.tsx` — 用户名徽章 + placeholder
- `web/default/src/i18n/locales/{en,zh,fr,ru,ja,vi}.json` — 新 placeholder 键
