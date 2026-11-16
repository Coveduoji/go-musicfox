# go-musicfox 项目架构文档

## 沟通语言准则

### 重要准则：与用户交流必须使用中文

**所有贡献者、开发者以及 AI 助手在与用户交流时，必须使用中文。**

#### 适用场景
- 代码审查意见和反馈
- Issue 回复和讨论
- Pull Request 描述和评论
- 文档编写（README、CHANGELOG 等除外，代码注释和提交信息仍使用英文）
- AI 助手与用户的对话

#### 例外情况
- **代码注释**：使用英文（保持代码可读性和国际化标准）
- **Git Commit Message**：使用英文（遵循 Conventional Commits 规范）
- **代码变量/函数命名**：使用英文（遵循 Go 语言惯例）
- **API 文档和错误消息**：根据实际情况使用中文或英文

#### 示例

```markdown
✓ 正确示例（中文交流）：
"这个 PR 修复了歌词封面图圆角功能的性能问题，优化后 CPU 占用从 10% 降至 2%。"

✗ 错误示例（英文交流）：
"This PR fixes the performance issue with lyric cover cornerRadius feature..."
```

## 编码行为准则

本准则旨在减少 AI 编码常见的错误。**权衡原则**：这些准则偏向谨慎而非速度，对于简单任务可自行判断。

> 本准则改编自 [andrej-karpathy-skills/CLAUDE.md](https://github.com/forrestchang/andrej-karpathy-skills/blob/main/CLAUDE.md)

### 1. 三思而后行

**不要假设。不要隐藏困惑。明确权衡。**

在实现之前：
- 明确陈述你的假设。如果不确定，先问。
- 如果存在多种解释方案，全部提出——不要默默选择一个。
- 如果存在更简单的方案，指出它。有理由时可以反驳。
- 如果有不清楚的地方，停下来。说出哪里不清楚，然后问。

### 2. 简单性优先

**最小代码解决问题。不要 speculative。**

- 不添加需求之外的功能。
- 一次性使用的代码不抽象。
- 不添加未被请求的"灵活性"或"可配置性"。
- 不处理不可能发生的错误场景。
- 如果写了 200 行而可以用 50 行完成，重写。

问自己："高级工程师会觉得这过于复杂吗？"如果是，简化。

### 3. 精准修改

**只触碰必须改的。只清理自己造成的混乱。**

编辑现有代码时：
- 不"改进"相邻代码、注释或格式。
- 不重构没坏的东西。
- 匹配现有风格，即使你可能用不同方式实现。
- 如果发现无关的死代码，指出它——不要删除它。

当你的修改产生孤儿（不再使用的代码）时：
- 移除因你的修改而不再使用的 import/变量/函数。
- 不要移除预先存在的死代码，除非被要求。

**检验标准**：每一行修改都应该能直接追溯到用户的请求。

### 4. 目标驱动执行

**定义成功标准。循环验证直到完成。**

将任务转化为可验证的目标：

| 模糊表述 | 可验证目标 |
|---------|-----------|
| "添加验证" | "为无效输入编写测试，然后让测试通过" |
| "修复 bug" | "编写能复现问题的测试，然后让测试通过" |
| "重构 X" | "确保重构前后测试都通过" |

对于多步骤任务，简要陈述计划：

```
1. [步骤] → 验证: [检查点]
2. [步骤] → 验证: [检查点]
3. [步骤] → 验证: [检查点]
```

明确的目标让你能独立循环验证。模糊的目标（"让它能工作"）需要不断确认。

---

**这些准则生效的标志**：diff 中不必要的修改更少，因过度复杂而返工更少，澄清问题在错误之前而非错误之后提出。

## 项目概述

go-musicfox 是基于 Go 和 bubbletea 的网易云音乐 TUI 客户端，支持 macOS/Linux/Windows。

**技术栈**：
- **UI 框架**：bubbletea + foxful-cli（部分定制）
- **音频处理**：beep、go-mp3、go-flac
- **存储**：BoltDB
- **配置**：TOML + mapstructure
- **API**：netease-music SDK

**项目结构**：`cmd/` 入口 | `internal/` 核心业务（22个包） | `utils/` 工具 | `configs/` 嵌入式配置

## 核心架构

### 应用入口与初始化

**入口**：`cmd/musicfox.go` → `runtime.Run()` → 加载配置 → 数据迁移 → 启动 TUI

### UI 协调器

**文件**：`internal/ui/netease.go`

核心结构包含：login、search、player、lyricService、trackManager 等组件。

### 核心接口

**Menu 接口**（`internal/ui/menu.go`）：
```go
type Menu interface {
    model.Menu
    IsPlayable() bool
    IsLocatable() bool
}
type SongsMenu interface { Songs() []structs.Song }
type PlaylistsMenu interface { Playlists() []structs.Playlist }
```

**Player 接口**（`internal/player/player.go`）：
```go
type Player interface {
    Play(music URLMusic)
    Pause()/Resume()/Stop()/Toggle()
    Seek(duration time.Duration)
    PassedTime()/PlayedTime() time.Duration
    Volume()/SetVolume()/UpVolume()/DownVolume()
    State() types.State
    Close()
}
```

### 播放引擎

| 引擎 | 平台 | 特点 |
|------|------|------|
| Beep（默认） | 跨平台 | MP3/FLAC/OGG/WAV |
| DLNA | 跨平台 | 设备投送 |
| MPV | 跨平台 | IPC 控制 |
| MPD | Linux | 远程服务器 |
| AVFoundation | macOS | 原生集成 |
| MediaPlayer | Windows | WinRT API |

### 事件处理

**文件**：`internal/ui/event_handler.go`

支持 40+ 键盘操作、鼠标事件（单击/双击/滚轮/右键）、可配置快捷键。

### 其他模块

- **歌词**：LRC/YRC 格式，支持 smooth/wave/glow 渲染模式
- **v4.8.5 自适应播放区**：`internal/ui/now_playing_renderer.go` 复用此版本的封面缓存、Kitty 协议和歌词渲染，使用原有 Bubble Tea 0.25 / Lip Gloss 0.8，不升级依赖。`[main.lyric].layout = "adaptive"` 默认显示小尺寸原图及主区域歌名/歌手；宽度至少 76 列时左右排列，否则封面在上、歌名与歌词在下。宽屏封面高度与右侧歌名/歌词块一致、顶部对齐，按实际 cell 像素比例计算所占列数并受横向可用宽度限制；窄屏最多 10 行；高度不足时隐藏封面、减少歌词行数。歌词栏宽度只取决于窗口尺寸（最多 48 列），左边缘复用 `Main.MenuStartColumn()`，剩余宽度参与布局计算，不再使用独立居中或视觉偏移，避免异步歌词加载和开始播放后两栏回退；宽窗口间隔 4～6 列，播放区在可用空白内上移最多 4 行，文字与原图使用同一偏移。不支持图片的终端隐藏封面，不输出字符像素块。图片异步复用现有缓存并经 `kitty.ResizeImageSquare` 缩至 320px；按歌曲缓存编码，普通缩放只移动摆放；`kitty/cell_size_unix.go` 经现有 x/sys ioctl 读取单元格像素尺寸（不可用时由 Kitty 计算），`PlaceImage` 只传行数、自动按源图比例确定宽度，避免强制列/行比例拉伸，切页 `Hide` 清理摆放并取消请求，清屏后的 `Invalidate` 恢复原图。失败后封面下载冷却 30 秒，终端写失败冷却 1 秒。`classic` 恢复原歌词/Kitty 布局。
- **自适应菜单列数**：`[theme].menuColumns` 默认 0，保持原 `doubleColumn` / `centerEverything` 菜单行为；1～3 启用最大列数配置，110 列及以上最多三列、75～109 最多两列、其余一列。foxful-cli 本地扩展 `model/menu_grid.go` 按对称边距布局，等宽列均匀分布、各列文字统一左对齐，索引按本页位数补齐；`Main.MenuStartColumn` 为首列边缘，封面跟随它。`MenuColumnCount` 返回实际列数，`MenuIndexAt` 复用渲染边界（含截断后的实际宽度），`event_handler.go` 在显式 menuColumns 模式下委托它，不再猜双列宽度。页面容量向上补齐为列数倍数，四方向导航、g/G、翻页和缩放保持列/选中项；框架补丁仍统一在 `patches/foxful-cli-fixed-footer.patch` 中。
- **固定底栏**：`internal/ui/playback_bar_renderer.go` 通过 foxful-cli `BottomComponent` 固定于窗口底部，只显示喜欢、进度、模式、上一首、播放/暂停、下一首、队列、频谱开关和音量减/加，不显示封面、歌名或歌手。控件按左侧喜欢/模式、中间上一首/胶囊播放/下一首、右侧队列/频谱/音量分组，`playback_icons.go` 在 Kitty 渲染透明红白图标（用户 SVG 风格播放/暂停、Lucide 0.468.0 辅助图标），GNOME 回退字符控件。`assets/playback` 保存 SVG、预生成 PNG 和许可，`hack/render_playback_icons.py` 使用可选 CairoSVG 再生成，无新 Go 依赖。图标按 action/variant 缓存编码与独立 ID；重复帧不上传，状态变更只更新相关摆放；Hide/Invalidate 处理页面切换和清屏，写失败冷却并回退字符。喜欢鼠标操作按缓存状态切换，`operate.go` 复用原 NewOperation/PlaylistTracksService，成功后同步 `likelist.SetLikeSong`，失败不改缓存；同步 UI handler 内不得调用 App.Rerender（Program.Send 会自阻塞），由原事件返回的 Tick 重绘（快捷键仍分别对应喜欢/取消）。控件图标仅红/白，模式文字只在宽屏显示；音量条支持直接点击和拖动，保留加/减。`[player].mouseVolumeEnabled` 默认 true；false 时忽略普通及 Ctrl + 上下滚轮的音量操作，不影响其他鼠标控件；`mouseVolumeStep` 仍仅控制 Ctrl + 滚轮步长。`[main.visualizer].enable` 默认为 true，重启生效；`Config.SpectrumEnabled` 同时限制 adaptive 布局，音符按钮即时切换会话频谱，不修改配置默认值或歌词；可选 SpectrumController 使 Beep 在自身锁内控制原子 active，采样 tap 关闭时仅检查标记不复制 PCM。分析器首次激活才启动 FFT worker；关闭后停止采样和 10Hz timer，ticker 以 atomic enabled 和控制 channel 切换，原音频计时恢复驱动刷新，足够大的窗口始终预留相同频谱空间，开关不改变主区、横线或控件坐标。至少 32 列且 20 行时预留三行频谱区，最后一行的中线固定为原关闭状态分割线；Kitty 的 `spectrum_image.go` 将白色细柱、单像素灰线和下方透明渐淡倒影统一绘制，开/关均复用同一图像基线；关闭只提交无柱体图像（不重复刷新）。保持固定图像 ID，每次内容改变重新传输并摆放；相同量化帧跳过，Hide/Invalidate 处理切页和清屏，输出失败冷却并回退字符。PNG 使用标准库 BestSpeed 和 EncoderBufferPool，传输复用 `kitty.TransmitImageWithEncoder`，不重传封面/控件；GNOME 字符回退以最后一行中线为基准，不依赖字体下划线。`internal/player/spectrum.go` 通过可选 `SpectrumProvider` 接口暴露 24 个对数频段，插值后满宽显示。Beep 在音量处理后非阻塞复制 PCM，Gonum FFT 在独立 worker 以最多 20Hz 分析双声道 2048 点 Hann 窗，重用缓冲，忙时丢帧。epoch 隔离暂停、切歌和 seek 前的采样，暂停/停止清空。`ticker_by_player.go` 仅在自适应布局且引擎支持 PCM 时以 10Hz 刷新播放界面，取代原时间事件的重复重绘；原音频计时、歌词更新、全局 frameRate 不变。其他引擎、静音或过期采样显示静态分隔线；小窗口隐藏频谱图，频谱图仅在量化内容变化时最多 10Hz 传输，不增加封面与控件传输。底栏基础三行，可容纳频谱时固定加两行，不随开关变化，小于 76 列再加一行放队列/频谱/音量；点击命中使用裁剪后实际渲染区间与屏幕坐标，按键与鼠标复用播放器能力。`FlexibleComponent` 接收菜单与底栏间实际剩余空间，`HidingComponent`/`InvalidatingComponent` 在切页/清屏时维护原图生命周期。扩展补丁为 `patches/foxful-cli-fixed-footer.patch`；`make vendor` 运行 tidy/vendor、固定版本 modvendor 复制原生头文件，再自动应用补丁；手动重新生成 vendor 后执行 `make vendor-patch`，补丁冲突不能跳过。
- **播放列表**：列表循环/顺序/单曲循环/随机/无限随机/智能心动模式
- **远程控制**：MPRIS(linux)、Now Playing(macOS)、System Media(Windows)
- **存储**：BoltDB，存储用户信息、播放状态、播放列表快照

## 开发指南

### 添加新菜单

1. 创建 `internal/ui/menu_new_feature.go`，嵌入 `baseMenu`
2. 实现 `Menu` 接口
3. 注册到导航系统

### 添加新播放器引擎

1. 实现 `internal/player.Player` 接口
2. 在 `player.go:NewPlayerFromConfig()` 添加 case
3. 添加配置支持

### 修改快捷键

1. 在 `internal/keybindings/keybindings.go` 定义 `OperateType`
2. 在 `event_handler.go` 添加键映射
3. 在配置文件中添加自定义绑定

### 添加新渲染器

1. 实现 `Update()` 和 `View()` 方法
2. 在 `netease.go:Components()` 注册

## 文档维护准则

### 重要准则：修改代码后需维护 AGENTS.md

**所有贡献者在修改代码后，必须检查并更新 AGENTS.md 文档，防止文档腐化。**

#### 何时需要更新文档
- 添加、删除或重命名核心文件或目录
- 新增功能模块或组件
- 修改项目结构或架构
- 更改 API 接口或配置格式
- 添加新的播放器引擎、菜单类型、渲染模式
- 修改关键路径或重要流程
- **快捷键变更**：新增、删除或修改快捷键后，必须同步更新 README 中的快捷键说明部分（help 菜单是动态从 keybindings 配置生成的，无需手动修改）

#### 更新检查清单
- [ ] 目录结构是否准确反映当前项目结构
- [ ] 核心文件路径是否正确
- [ ] 接口定义是否与代码一致
- [ ] 新增功能的说明是否完整
- [ ] 开发指南是否需要补充
- [ ] 关键文件路径表格是否需要更新

#### 更新优先级
| 变更类型 | 优先级 | 说明 |
|---------|--------|------|
| 架构变更 | **高** | 必须立即更新 |
| 新增核心模块 | **高** | 必须添加说明 |
| 文件路径变更 | **中** | 及时更新路径表 |
| 细微修复 | **低** | 可批量更新 |

#### 维护建议
- 保持文档与代码同步，避免技术债务积累
- 使用一致的术语和格式
- 添加代码示例时确保与实际代码匹配
- 定期（如每月）审查文档完整性
- PR 审查时应包含文档检查

#### 文档腐化警告信号
- 文件路径与实际不符
- 过时的 API 接口描述
- 已删除功能仍出现在文档中
- 章节结构混乱或重复
- 与 README/CHANGELOG 存在矛盾

违反此准则可能导致：新贡献者无法理解项目结构、开发效率降低、文档失去参考价值、维护成本增加。

## Git 提交规范

### 重要准则：Git Commit Message 必须遵循 Conventional Commits 规范

**所有贡献者在提交代码时，必须遵循 [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) 规范。**

#### 提交格式

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

#### Type 类型

| Type | 说明 |
|------|------|
| `feat` | 新功能开发 |
| `fix` | Bug 修复 |
| `docs` | 文档更新 |
| `style` | 代码格式调整（不影响功能） |
| `refactor` | 代码重构（既不修复bug也不添加功能） |
| `perf` | 性能优化 |
| `test` | 测试相关 |
| `chore` | 构建工具、辅助工具、配置变更 |
| `revert` | 回滚提交 |

#### 示例

```
feat(player): 添加 MPV 播放引擎支持

- 支持多种音频格式
- 实现播放进度控制
- 优化内存使用

Closes #123
```

#### 规范要求

1. **必须使用英文**：提交信息、描述均使用英文
2. **动词开头**：描述部分以动词开头，使用现在时态
3. **长度限制**：标题不超过 50 字符
4. **Body 可选**：复杂变更可添加详细说明，每行不超过 72 字符
5. **引用 Issues**：在 footer 中使用 `Closes #xxx` 关联 Issue
