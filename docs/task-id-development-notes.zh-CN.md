# Boards 任务 ID 与 loong64 插件开发记录

本文记录从修改 Focalboard/Boards 源码开始，到生成 `linux-loong64`
Mattermost Boards 插件包期间的需求演进、实现调整、问题修复和构建验证。

## 背景

使用 Mattermost Boards 管理项目时，原版卡片只有内部 block/card ID。这类 ID
是长字符串，不适合写入代码提交、构建产物、实验日志或性能分析记录。时间久了，
代码变更、Boards 任务和当时的构建环境很难对应起来。

本次改动的目标是：

- 给每张卡片增加稳定、可读的引用 ID。
- 旧卡片也要能迁移到可读编号，不能继续暴露内部字符串。
- 只构建 `linux-loong64` 插件。
- 不破坏 Mattermost 7.10/7.11 Boards 原有使用流程。

## 版本变更记录

### `7.11.0-taskid-boardonly-linux-loong64`

- 用途：保留的可回退版本。
- 功能：只提供 board 内短编号 `#N`。
- 修复：新建卡片、复制卡片、旧卡片迁移都会生成 board 内数字编号。
- 限制：没有跨 board 的全局 ID；不同 board 中可能同时存在 `#1`、`#2`。
- 产物：
  - `focalboard-7.11.0-taskid-boardonly-linux-loong64.tar.gz`
  - `focalboard-7.11.0-taskid-boardonly-linux-loong64.tar.gz.zst`

### `7.11.1-taskid-linux-loong64`

- 功能：在 board 内 `#N` 基础上增加跨 board 的 `globalTaskId`，显示为 `G-N`。
- 功能：`globalTaskId` 同时写入 `fields.globalTaskId` 和 `fields.properties.__globalTaskId`。
- 功能：卡片详情属性区显示只读 `Global ID`。
- 修复：插件 manifest 版本从 `7.11.0` 升到 `7.11.1`，避免同版本上传时 Mattermost 无法正常升级重启插件。
- 问题：最初为了避免全库扫描，缺失全局计数器时从毫秒时间戳起步，可能生成 `G-1780519393936` 这种不可读编号。
- 问题：全局迁移和 board 打开路径仍有性能压力。

### `7.11.2-taskid-linux-loong64`

- 修复：停止使用毫秒时间戳作为全局 ID 起点。
- 修复：如果系统设置中已存在时间戳形态计数器，会扫描已有正常 `G-N` 并重置为正常最大值。
- 修复：历史上已生成的 `G-1780...` 这类时间戳 ID 会被视为无效旧值，在对应 board 迁移时重新分配正常 `G-N`。
- 保持：新卡、复制卡仍由服务端生成 `#N` 和 `G-N`，客户端传入值会被覆盖。

### `7.11.3-taskid-linux-loong64`

- 修复：软删除卡片不再占用后续编号。
- 修复：如果删除的是当前最高 `#N` 或 `G-N`，内存缓存和全局系统计数器会回退，下一张新卡可以复用该最高号。
- 修复：恢复已删除卡片时，如果原编号已被新卡复用，恢复卡片会自动获得新的不冲突编号。
- 行为变化：编号唯一范围明确为“当前未删除卡片”；历史日志中如果只记录短 ID，删除复用后可能出现旧记录和新卡共用同一短 ID 的情况。

### `7.11.4-taskid-linux-loong64`

- 功能：board 顶部三点菜单增加 `Deleted cards` 入口。
- 功能：弹窗列出当前 board 已删除卡片，显示 board 内 `#N`、全局 `G-N`、标题和删除时间。
- 功能：弹窗中可点击 `Restore` 恢复卡片。
- 修复：前端 Redux store 新增 `deletedCards`，避免已删除卡片混入正常看板/表格，同时能在恢复弹窗中展示。
- 修复：恢复操作调用已有 `undeleteBlock` API，并把返回的卡片写回前端状态。
- 验证：`webapp npm run check`、主 webapp `npm run pack`、插件 webapp `npm run build` 均通过。
- 已知：当前 loong64 环境无法运行 Jest，原因是 `@swc/core` 没有 loong64 Linux native binding；已同步更新相关 snapshot 文本。

## 需求演进

### 第一阶段：board 内唯一 ID

最初需求是给每张卡片增加唯一 ID，方便在代码中引用和跟踪。实现选择为
board 内编号：

- 字段：`fields.taskId`
- 格式：`#1`, `#2`, `#3`
- 唯一范围：同一个 board 内
- 用途：日常看板沟通、单个项目 board 内的短引用

旧卡片迁移规则：

- 先保留已有合法 `#N`。
- 重复的 `#N` 只保留按 `createAt` 和 block ID 排序最早的一张。
- 缺失、非法值、内部 card ID 字符串都视为需要迁移。
- 新编号从当前最大 N 之后继续分配，不填补空洞，避免破坏已有引用。

### 第二阶段：真实创建路径修复

第一次部署后发现新卡片仍显示长字符串。原因是 Boards 前端新建卡片实际走的是：

```text
POST /boards/{boardID}/blocks
```

也就是直接插入 `card` block，而不是最初重点修改的 `/cards` 创建接口。

修复点：

- 在 `InsertBlockAndNotify` 和 `InsertBlocksAndNotify` 中识别 `TypeCard`。
- 服务端强制覆盖客户端传入的 `taskId`。
- 新卡、复制卡都由服务端生成 `#N`。

### 第三阶段：旧数据全量读取路径修复

第二次部署后，新卡已经是数字 ID，但旧卡仍显示字符串。原因是打开 board 时前端
请求全量 blocks：

```text
GET /boards/{boardID}/blocks?all=...
```

对应 app 层 `GetBlocksForBoard`，此前没有触发旧卡迁移。

修复点：

- 在 `GetBlocksForBoard` 返回 blocks 前执行旧卡 `taskId` 补齐。
- 打开旧 board 时会自动把旧字符串迁移成 `#N`。

### 第四阶段：跨 board 全局 ID

后续需求是除了 board 内 `#N`，还需要一个在所有 board 中唯一的整体 ID，用于跨
项目、跨 board、代码提交和构建产物追踪。

这个需求是合理的，但不应该替代 board 内 ID。最终设计为双 ID：

- `taskId`: board 内唯一，格式 `#N`
- `globalTaskId`: 全局唯一，格式 `G-N`

`globalTaskId` 同时写入：

- `fields.globalTaskId`
- `fields.properties.__globalTaskId`

前端在卡片详情属性区以只读属性形式显示 `Global ID`。不自动修改用户的 board
property template，避免污染用户自定义列配置。

### 第五阶段：性能修复

引入旧数据迁移和全局 ID 后，删除/新建卡片出现约 0.5-1 秒延迟。原因是迁移逻辑
在打开 board、新建、复制等路径中反复扫描 blocks，尤其全局 ID 会扫描所有 board
的卡片。

性能修复：

- 每个 board 的旧 `taskId` 迁移只在当前进程内执行一次。
- 全局 `globalTaskId` 不再通过扫描所有 board 恢复最大值。
- 全局编号改用 `system_settings` 中的持久计数器 `focalboard_card_global_task_id_max`。
- 如果计数器不存在，首次发号时扫描一次已有卡片，按最高的正常 `G-N` 初始化。
- 上一版错误生成的毫秒时间戳形态 ID，例如 `G-1780519393936`，视为无效旧值，
  在对应 board 迁移时重新分配为正常序列号。
- 旧 `globalTaskId` 只在当前打开或写入的 board 内懒迁移，并在进程内按 board 缓存。
- 打开整板时复用已经取回的 board blocks 做迁移，避免打开前额外查询一次当前 board 卡片。
- 新建/复制卡片直接从持久计数器递增，不再重复全量扫描。
- 软删除卡片不再参与后续最大编号计算；如果删除的是当前最高号，缓存和全局计数器会回退。
- 恢复已删除卡片时，如果原编号已被新卡片复用，恢复的卡片会自动获得新的不冲突编号。
- board 内编号仍使用 board 级锁。
- 全局编号使用全局锁。
- 缓存 map 增加独立锁，避免不同 board 并发写缓存导致 Go map race。

### 第六阶段：已删除卡片 UI 恢复

在 `7.11.3` 中，为了满足“删除最高编号后下一张新卡接替该编号”的需求，服务端把
软删除卡片排除在编号占用范围之外，并补了恢复冲突时自动重分配 ID 的逻辑。
随后产生了新的使用问题：既然后端支持恢复，那么 UI 中也需要一个可见入口，否则用户
不知道如何恢复已删除卡片。

需求：

- 能在 board UI 中看到当前 board 已删除卡片。
- 能从 UI 直接恢复卡片，不需要手写 API 或操作数据库。
- 恢复后仍遵守 `7.11.3` 的编号规则：如果原 `#N` / `G-N` 已被新卡复用，恢复卡片
  自动获得新的不冲突编号。
- 已删除卡片不能重新混入正常看板、表格、画廊、日历视图。

实现：

- 复用后端已有接口 `POST /boards/{boardID}/blocks/{blockID}/undelete`。
- 复用前端已有 `octoClient.undeleteBlock`，在 `mutator` 中新增显式
  `undeleteBlock` 方法，把恢复后的 block 写回 Redux。
- Redux `cards` store 新增 `deletedCards`，专门保存软删除卡片。
- `updateCards` / `loadBoardData` / `initialReadOnlyLoad` 遇到 `deleteAt != 0`
  的 card 时写入 `deletedCards`，并从正常 `cards` / `templates` 中移除。
- 正常 selector 仍只返回未删除卡片，避免影响 board 原功能。
- board 顶部三点菜单增加 `Deleted cards ({count})`。
- 新增 `DeletedCardsDialog`，显示 `#N`、`G-N`、标题、删除时间和 `Restore` 按钮。
- 恢复失败时显示错误 flash message，成功时显示正常提示。

验证与构建过程：

- `webapp npm run check` 通过。
- 主 webapp `npm run pack` 通过，只出现原有 bundle size warning。
- 插件 webapp `npm run build` 通过，只出现原有 bundle size warning。
- 插件 webapp `npm run lint` 通过。
- `mattermost-plugin/webapp npm run check-types` 在当前仓库/loong64 环境失败，
  错误来自既有 Mattermost webapp 类型定义和 React 类型重复，不指向本次新增文件。
- Jest 在当前 loong64 环境失败，原因是 `@swc/core` 没有 Linux loong64 native
  binding；因此无法在本机直接跑 `viewHeaderActionsMenu.test.tsx`，已手动同步
  snapshot 中新增的 `Deleted cards (1)` 菜单项。

构建问题与处理：

- 运行 `mattermost-plugin/webapp npm run check-types` 后，`tsc` 会在
  `mattermost-plugin/webapp/dist` 中生成大量 `.js` / `.map` 和测试编译产物。
- 如果不清理直接 `make bundle`，这些临时产物会被打进插件包。
- 处理方式是清理 `webapp/pack`、`webapp/dist`、`mattermost-plugin/webapp/dist`、
  `mattermost-plugin/dist`，然后按正确顺序重新执行：
  `webapp npm run pack`、`mattermost-plugin/webapp npm run build`、`make bundle`。
- 最终校验确认 `focalboard-7.11.4.tar.gz` 中没有 `webapp/dist/webapp`、
  `webapp/dist/mattermost-plugin` 或 `*.test.js` 编译产物。

## 主要源码修改

服务端模型：

- `server/model/card.go`
  - 增加 `TaskID`
  - 增加 `GlobalTaskID`
  - `Card2Block` / `Block2Card` 双向转换字段

服务端 app 层：

- `server/app/cards.go`
  - board 内 `taskId` 生成、解析、迁移
  - 全局 `globalTaskId` 生成、解析、迁移
  - 重复编号去重
  - 迁移缓存和最大编号缓存
  - `properties.__globalTaskId` 同步

- `server/app/blocks.go`
  - `GetBlocks`
  - `GetBlocksForBoard`
  - `InsertBlockAndNotify`
  - `InsertBlocksAndNotify`
  - `DuplicateBlock`
  - 这些真实路径都会处理 card ID

- `server/app/app.go`
  - board 级 task ID 锁
  - 全局 ID 锁
  - 迁移状态和最大编号缓存

前端：

- `webapp/src/blocks/card.ts`
  - 增加 `globalTaskId` 类型

- `webapp/src/components/cardDetail/cardDetailProperties.tsx`
  - 卡片详情属性区显示只读 `Global ID`

- `webapp/src/store/cards.ts`
  - limited card 状态保留 `globalTaskId`
  - 增加 `deletedCards`，保存已删除卡片供恢复弹窗使用

- `webapp/src/styles/main.scss`
  - `Global ID` 只读显示样式

- `webapp/src/mutator.ts`
  - 增加 `undeleteBlock`，调用恢复 API 并更新前端状态

- `webapp/src/components/viewHeader/viewHeaderActionsMenu.tsx`
  - board 顶部三点菜单增加 `Deleted cards ({count})`

- `webapp/src/components/viewHeader/deletedCardsDialog.tsx`
  - 已删除卡片列表与恢复按钮

- `webapp/src/components/viewHeader/deletedCardsDialog.scss`
  - 已删除卡片弹窗样式

插件/loong64：

- `mattermost-plugin/plugin.json`
  - 只保留 `linux-loong64`

- `mattermost-plugin/Makefile`
  - 增加 loong64 server build

- `server/services/store/sqlstore/sqlite_migration*.go`
  - loong64 下绕开不支持的 sqlite migration driver import

- `mattermost-plugin/go.mod`
  - 升级 `modernc.org/sqlite` / `modernc.org/libc`，解决 loong64 编译问题

## 遇到的问题

### 前端创建路径不是 `/cards`

现象：部署后新卡片显示内部字符串。

原因：Boards UI 直接插入 card block。

修复：在 block insert 路径生成 ID。

### 旧数据全量加载没有迁移

现象：新卡是数字 ID，旧卡仍是字符串。

原因：打开 board 时走 `GetBlocksForBoard`。

修复：`GetBlocksForBoard` 前触发迁移。

### `CreateCard` 与 block insert 双重加锁

问题：最初 `CreateCard` 自己分配 `taskId`，后续 block insert 也分配，存在逻辑分叉
和潜在死锁。

修复：统一由 block insert 路径分配 card ID。

### 全局 ID 引入性能问题

问题：全局扫描所有卡片成本高。

修复：全局 ID 使用 `system_settings` 持久计数器；打开 board 只迁移当前 board
中的旧卡片，不再扫描所有 board。

### loong64 sqlite 依赖编译问题

问题：旧 `modernc.org/libc` 在 loong64 下缺少若干 build target。

修复：

- 插件 go.mod 升级 modernc 依赖。
- Focalboard sqlite migration driver 在 `linux/loong64` 下使用 build tag 隔离。

### npm install/cwebp-bin 问题

问题：loongarch64 下 `cwebp-bin` 的 config.guess 不识别架构。

处理：

- 使用 `npm install --ignore-scripts` 安装依赖。
- 再分别执行 webapp 和插件 webapp 的 webpack build。

## 当前行为

新建卡片：

- 服务端覆盖客户端传入的 `taskId` / `globalTaskId`
- 生成 board 内 `#N`
- 生成全局 `G-N`
- 写入 `properties.__globalTaskId`

复制卡片：

- 生成新的 `#N`
- 生成新的 `G-N`
- 不沿用源卡片编号

删除/恢复卡片：

- 删除卡片后，该卡片不再占用 board 内 `#N` 和全局 `G-N`
- 如果删除的是当前最高编号，下一张新卡片会复用这个最高编号
- 如果被删除的卡片后来恢复，而原编号已被其他活动卡片复用，恢复卡片会重新分配新编号
- UI 在 board 顶部三点菜单中增加 `Deleted cards` 入口，列出当前 board 已删除卡片并支持恢复

打开旧 board：

- 首次加载时迁移旧 `taskId`
- 首次加载当前 board 时迁移旧 `globalTaskId`
- 迁移后按 board 使用进程内缓存，避免重复扫描

显示：

- board 内 ID 显示在卡片标题附近
- 全局 ID 显示在卡片详情的属性区，名称为 `Global ID`

## 验证命令

关键服务端测试：

```bash
cd server
GOCACHE=/tmp/focalboard-gocache GOMODCACHE=/tmp/focalboard-gomodcache go test ./app -run '^(TestCardTaskIDLifecycleUsesUniqueIDs|TestCreateCard|TestEnsureCardTaskIDs)$' -count=1
GOCACHE=/tmp/focalboard-gocache GOMODCACHE=/tmp/focalboard-gomodcache go test ./model -run '^(TestBlock2Card|TestCard2BlockIncludesTaskID)$' -count=1
```

前端检查：

```bash
cd webapp
npm run check
```

构建：

```bash
cd webapp
npm run pack

cd ../mattermost-plugin/webapp
npm run build

cd ../server
GOCACHE=/tmp/focalboard-gocache GOMODCACHE=/tmp/focalboard-gomodcache \
CGO_ENABLED=0 GOOS=linux GOARCH=loong64 \
go build -trimpath -o dist/plugin-linux-loong64

cd ..
make bundle
```

包校验：

```bash
tar -xOzf mattermost-plugin/dist/focalboard-7.11.4.tar.gz focalboard/plugin.json
tar -tzf mattermost-plugin/dist/focalboard-7.11.4.tar.gz | grep plugin-linux
file mattermost-plugin/server/dist/plugin-linux-loong64
```

## 已知边界

- `taskId` 的严格唯一范围是单 board 的未删除卡片。
- `globalTaskId` 的严格唯一范围是单插件进程串行写入场景下的未删除卡片。
- 删除后复用编号会让历史日志中的旧编号和新卡片复用同一个短 ID；需要长期审计时应同时记录卡片标题、时间和代码提交。
- 如果同一个数据库有多个插件进程同时写入，严格跨进程唯一需要数据库事务序列或唯一约束。
- 当前 ID 存在 block fields JSON 中，不是独立索引列。
- 全局编号计数器持久化在 `system_settings`；插件重启后不会为了恢复最大值扫描全库。
- 如果系统设置里没有旧计数器，首次发号会扫描一次已有卡片并按最高正常 `G-N`
  初始化；不会再从当前毫秒时间戳起步。

## 产物

最终插件包生成在工作区根目录：

```text
focalboard-7.11.4-taskid-linux-loong64.tar.gz
focalboard-7.11.4-taskid-linux-loong64.tar.gz.zst
```
