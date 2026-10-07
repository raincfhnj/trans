# 故障复盘：SendInput 的结构体布局不是 Windows 的联合体，粘贴全部被拒

## 基本信息

| 字段 | 内容 |
|------|------|
| 日期 | 2026-10-06 |
| 发现人 | 用户实际使用（把提示词投递到 OpenCode 窗口时连续失败 5 次） |
| 严重程度 | P0-阻断（投递功能的核心路径，alt+enter 一旦走到粘贴步骤必然失败） |
| 影响范围 | `wintarget` 投递路径的粘贴环节：所有由面板/托盘触发、走到 `sendKeys` 的提示词投递；自 42426e9（2026-10-01）起存在 |
| 关联 Issue/PR | 无 |
| 关联提交 | 故障存在于 b05cd0a（修复尚未提交） |

## 1. 问题描述

### 1.1 问题场景

面板（`trans-window open` → `trans-window popup --send`）把提示词写上剪贴板、把目标窗口调到前台之后，按下粘贴组合键 `ctrl+shift+v` 把提示词交进去。这一次的目标窗口是 "OpenCode"（0xe90a88）。

### 1.2 具体表现

- 面板报错：`the prompt is on the clipboard but could not be pasted: SendInput was refused`
- 提示词确实已经在剪贴板上（按提示手动 Ctrl+V 可以贴进去），但**程序自动投递一次都没成功过**；
- 连续重试 5 次全部同样失败，日志里除了一句 "was refused" 没有任何别的线索。

### 1.3 错误信息

**日志信息**（`%LOCALAPPDATA%\trans\trans-window.log`）：

```
2026-10-06T20:42:29+08:00 the paste chord ctrl+shift+v was refused: SendInput was refused
2026-10-06T20:43:02+08:00 the paste chord ctrl+shift+v was refused: SendInput was refused
2026-10-06T20:43:13+08:00 the paste chord ctrl+shift+v was refused: SendInput was refused
2026-10-06T20:43:21+08:00 the paste chord ctrl+shift+v was refused: SendInput was refused
2026-10-06T20:43:28+08:00 the paste chord ctrl+shift+v was refused: SendInput was refused
```

**面板提示**（用户看到的）：

```
the prompt is on the clipboard but could not be pasted: SendInput was refused
```

## 2. 临时解决方案（可选）

### 2.1 方案描述

报错文案本来就说清楚了：提示词还在剪贴板上，**手动 Ctrl+V 粘贴后自己回车**即可，功能上不丢数据。

### 2.2 止血效果

能用，但每次投递都要手动粘贴，`alt+enter` 的意义没有了。

### 2.3 临时方案的局限

绕过了故障，没有解决"程序投递必然失败"；而且日志只写 "was refused"，下一次同类故障照样无从下手。

## 3. 根本原因分析

### 3.1 问题分析过程

1. **观察到的表象**：面板报 `SendInput was refused`，但剪贴板、前台窗口、焦点这些前序步骤都成功了（否则会是别的报错），失败点落在"按键注入"这一步。
2. **先查日志**：`trans-window.log` 里 `the paste chord ... was refused` 来自 `internal/wintarget/target_windows.go:81` 的 `note(...)`，被 `internal/wintarget/policy.go:195` 的 `pasteFailed()` 包装成用户看到的那句话。注意 `note()` 写的是 `trans-window.log` 而不是 `panel.log`，这一点一开始差点看漏。
3. **第一个方向（被误导）**：`policy.go:189-193` 的注释把这种情况归因为 UIPI（目标窗口完整性级别更高时 Windows 静默丢弃输入）。于是去核对本机所有相关进程的完整性级别（`GetTokenInformation(TokenIntegrityLevel)`）：`trans-windowd`、`WindowsTerminal`、`opencode`、本 shell 全是 `S-1-16-12288`（High，本机 UAC 关闭），**同级，UIPI 不成立**，排除。
4. **第二个方向（看错误是怎么产生的）**：`internal/win32/input_windows.go:248-256`：
   ```go
   result, _, _ := procSendInput.Call(...)
   seen := int(result)
   if seen == 0 {
       return 0, lastError("SendInput")
   }
   ```
   `lastError()` 读的是 `syscall.GetLastError()`。写最小复现实验后发现两件事：
   - `procSendInput.Call` 的**第三个返回值**是 `The parameter is incorrect.`（ERROR_INVALID_PARAMETER）；
   - 紧接着的 `syscall.GetLastError()` 却是 `nil`。
5. **为什么 `GetLastError()` 是空**：Go 的 syscall 封装在进函数前会清掉线程的 last-error 字（实验证明：`SetLastError(1234)` 之后调 `syscall.GetLastError()` 仍为 `nil`；`FindFirstFileW` 找不到路径时，`Call` 的第三返回值是 `The system cannot find the path specified.`，而 `GetLastError()` 仍是 `nil`）。所以**这个项目里 `lastError()` 从诞生起恒等于 "was refused"**，真实原因被丢光了——这就是"没有任何线索"的原因。
6. **继续追 ERROR_INVALID_PARAMETER**：SendInput 的文档写明 `cbSize` 必须等于 `sizeof(INPUT)`。写复现程序打印本项目结构体的真实大小：
   ```
   sizeof(input)=72  mouse=32 kbd=24 hw=8 union=64 align=8
   cbSize=72 -> r1=0  callErr=The parameter is incorrect.
   cbSize=40 -> r1=4  (Windows 接受)
   cbSize=28 -> r1=0  callErr=The parameter is incorrect.
   ```
   64 位 Windows 的 `INPUT` 是 **40 字节**，本项目传出的是 **72 字节**。
7. **为什么是 72**：`internal/win32/procs_windows.go` 里
   ```go
   type inputUnion struct {
       mouse    mouseInput   // 32
       keyboard keyInput     // 24
       hardware hardwareInput // 8
   }
   ```
   这**不是联合体，是顺序结构体**。Go 没有 union，三个字段并列排布 → 64 字节，再加 tag 与对齐 → 72。Windows 的 `INPUT` 是 `DWORD type` + 一个按**最大成员**（MOUSEINPUT）取宽度的联合体 → 40。字段注释写着"union is sized for a mouse event on purpose"，说明作者的意图是对的，但 Go 的内存模型把这份意图表达成了另一件事。
8. **顺藤摸出第二个布局错误**：`keyInput` 的第一个字段是 `kind uint32`，而 SDK `winuser.h:6127-6143` 里 `KEYBDINPUT` 的首字段是 `WORD wVk`——联合体里**没有**这个 tag（tag 是 `INPUT.type`）。也就是说即使把 `cbSize` 改成 40，`wVk` 也会读到那个多出来的 `kind`，按键码整体错位 4 字节，粘贴照样不会发生（只是会变成"注入了错误的键"）。
9. **为什么测试没拦住**：`internal/win32/input_windows_test.go:17-27` 只断言了
   ```go
   size%unsafe.Alignof(uintptr(0)) != 0   // 72 % 8 == 0 ✓
   size < unsafe.Sizeof(mouseInput{})+4   // 72 >= 36 ✓
   ```
   "够大且对齐"对 72 字节完全成立。**测试只检查了自己关心的性质，没有检查 Windows 关心的那个数字。**
10. **什么时候引入**：`42426e9 refactor: take the workspace as it stands`（2026-10-01）把原来的 `keybd_event` 换成 `SendInput` 并新增 `procs_windows.go`；`keybd_event` 只收参数、不收结构体，所以在此之前没有这个问题。该提交是"整个工作区一次性入库"，新结构体没有单独的验证环节。

### 3.2 直接原因

`internal/win32/procs_windows.go:34-67`：`inputUnion` 用 Go 的顺序字段模拟 C 的联合体，`input` 因此是 72 字节而非 40 字节；`keyInput` 又多了一个 `kind` 前导字段，字段整体后移 4 字节。`SendInput` 校验 `cbSize == sizeof(INPUT)` 失败，一个事件都不收。

**相关代码位置**：`internal/win32/procs_windows.go:34-67`（修复前）、`internal/win32/input_windows.go:248-256`（错误被吞掉的位置）

**关键代码片段**：

```go
// 修改前：三个字段并列 = 64 字节的联合体（实为顺序结构）
type input struct {
	kind  uint32
	union inputUnion
}
type inputUnion struct {
	mouse    mouseInput   // 32
	keyboard keyInput     // 24 —— 首字段是 kind，多了一个字
	hardware hardwareInput // 8
}
type keyInput struct {
	kind  uint32 // KEYBDINPUT 没有这个字段，wVk 应该在这里
	key   uint16
	...
}
```

### 3.3 根本原因

- **设计层面**：与 C 结构体对接时，直接按"字段长得像"来写，没有把 **union / 布局 / 尺寸**当作契约。注释里写了意图（"union is sized for a mouse event"），但没有任何东西验证意图与内存布局一致。
- **开发层面**：`keybd_event → SendInput` 是一次底层 API 换代，属于"必须实机验证"的改动，却只补了一个自己出题自己答的尺寸测试；42426e9 把整个工作区一次性入库，缺少可回溯的验证步骤。
- **流程层面**：诊断出口 `lastError()` 从未被验证过——它一直恒等于 "was refused"，等于没有诊断。**故障的可诊断性缺陷把 P0 拖成了"只能靠猜"。**

### 3.4 为什么没有提前发现

- 代码审查阶段：结构体看着"字段都对得上"，union 的问题藏在内存模型里，人眼不容易看出来；`lastError()` 也没有被追问过"它到底会返回什么"。
- 测试阶段：尺寸测试只断言"够大 + 对齐"，72 通过；**没有任何一次真实调用 SendInput 的用例**，所以"Windows 收不收"这件事从来没被问过。
- 监控告警：`note()` 写了日志，但内容是被污染的 "was refused"，有日志等于没日志。
- 复现频率：10-01 之后真正走到粘贴这一步的投递很少（更早的失败都停在剪贴板快照阶段），所以 10-06 20:42 才第一次暴露。

## 4. 解决方案

### 4.1 根本解决方案

**修改文件 1：`internal/win32/procs_windows.go`**

按 SDK `winuser.h` 逐字段重写，联合体用"一个事件 + 补齐到最大成员宽度"表达：

```go
// 修改后
type input struct {
	kind  uint32
	union inputUnion
}

// 键盘事件放在联合体开头，剩下的宽度是给"最大成员"留的：
// SendInput 在读事件之前先量 INPUT 的尺寸，所以只发键盘也要量到 MOUSEINPUT 的宽度。
type inputUnion struct {
	keyboard keyInput
	_        [inputUnionBytes - unsafe.Sizeof(keyInput{})]byte
}

const inputUnionBytes = unsafe.Sizeof(mouseInput{})

type keyInput struct { // == KEYBDINPUT：首字段是 WORD wVk，没有 tag
	key   uint16
	scan  uint16
	flags uint32
	Time  uint32
	Extra uintptr
}
```

**修改文件 2：`internal/win32/input_windows.go`**

`keyDown/keyUp` 不再写 `kind`；`sendKeys` 把 `Call` 的第三返回值交给 `lastError`。

**修改文件 3：`internal/win32/window_windows.go`（诊断缺陷，一并修）**

```go
// 修改前：读的是 Go 已经清掉的线程 last-error，恒为空
func lastError(proc string) error {
	err := syscall.GetLastError()
	...
}

// 修改后：错误必须由调用自己带回来
func callWhy(proc *syscall.LazyProc, args ...uintptr) (uintptr, error) {
	result, _, why := proc.Call(args...)
	return result, why
}

func lastError(proc string, from error) error {
	if from == nil || errors.Is(from, syscall.Errno(0)) {
		return fmt.Errorf("%s was refused", proc)
	}
	return fmt.Errorf("%s failed: %w", proc, from)
}
```

16 处调用点（`input_windows.go` 4 处、`clipboard_windows.go` 6 处、`console_windows.go` 6 处）全部改成 `callWhy(...)` + `lastError(名字, err)`。

**方案说明**：三个问题（尺寸、字段错位、错误被吞）必须一起修——只改尺寸，粘贴会变成"注入错误的键"；只修错误信息，粘贴仍然失败。没有选择"换回 keybd_event"：SendInput 是当前唯一正确的注入 API（keybd_event 已过时），而且它的结构体契约正是这套代码应该写对的东西。

**修改文件 4：`internal/win32/input_windows_test.go`**

- 尺寸测试改成**钉死 Windows 的数字**：`INPUT` 40/28、`KEYBDINPUT` 24/16、`MOUSEINPUT` 32/24（与 `winuser.h` 一致），并额外钉死 6 个字段偏移（`wVk@0、wScan@2、dwFlags@4、time@8、dwExtraInfo@16/12`、联合体起点）——偏移断言正是抓"多一个 `kind` 字段"这类错位的那一条；
- 新增 `TestWindowsTakesWhatSendInputIsGiven`：真实调用 `sendKeys` 发 6 个 VK0 keyup（虚拟键 0 不是任何键，不产生击键副作用），要求 Windows 全部收下；无交互桌面（`Foreground() == 0`）时按仓库既有约定 skip。**这是唯一一条会直接问"Windows 收不收"的用例，也是当初缺失的那一条。**

### 4.2 影响范围评估

- 投递路径：粘贴从"必然失败"恢复为正常；`Enter()` 复用同一条 `sendKeys`，回车键一并受益；
- 剪贴板 / 控制台 / 热键：只有错误文案变化（从恒定的 "was refused" 变成 Windows 的原话），行为不变；
- 兼容性：只在 Windows 生效（文件均带 `//go:build windows`），32 位布局同样按 28/16/24 计算，测试按指针宽度断言；
- 回归验证：`make qa`（fmt-check / vet / lint / race / govulncheck）全绿；复现程序 `cbSize=40` → Windows 收下事件。

## 5. 预防措施

### 5.1 代码层面

- [x] 与 C 结构体对接的 Go 结构体，字段注释必须写明来自哪个 SDK 头文件（如 `winuser.h` 的 `KEYBDINPUT`），**union 必须显式按最大成员宽度布局，不允许三个字段并列**；
- [ ] 评审清单加一条：FFI 结构体问一句"Windows 侧这是 union / 位域 / 变长结构吗？Go 这边是怎么表达的？"；
- [x] 凡是"要把错误说给人听"的 Win32 调用，一律走 `callWhy` + `lastError(名字, err)`；`syscall.GetLastError()` 在 `Proc.Call` 之后恒为空，不得再用。

### 5.2 测试层面

- [x] 结构体尺寸/偏移断言**必须写 Windows 的数字**，不能写"够大""对齐"这种自身性质（反例：旧的 INPUT 测试；正例：`guiThreadInfo` 的测试）；
- [x] 补一条真实调用 API 的用例（`TestWindowsTakesWhatSendInputIsGiven`），按"缺什么能力就 skip 什么"的既有约定处理无桌面会话；
- [ ] 凡新增/替换底层注入类 API（SendInput、PostMessage 等），必须先在真机上跑通一次再合入，不允许只靠单元测试。

### 5.3 监控层面

- [x] 排查口径：`%LOCALAPPDATA%\trans\trans-window.log` 出现 `the paste chord ... refused` 即粘贴环节失败；现在它会带上 Windows 的原话（例如 `SendInput failed: The parameter is incorrect.`），可直接区分 UIPI、参数错误、无桌面三种情形。

### 5.4 流程/规范层面

- [ ] 像 42426e9 这样"整个工作区一次性入库"的提交，至少把**新增的跨语言结构体/系统调用契约**拆出来单独验证并留下可回溯记录；
- [ ] `docs/` 里的故障复盘已成惯例：修复完成后按本模板补一份，让"可诊断性缺陷"本身也被记账。

## 6. 经验总结（一句话）

> Go 里没有 union——按 C 结构体写 Go 结构体时，尺寸和偏移必须拿 SDK 头文件的数字逐个钉死并真实调用一次 API；而错误信息必须来自那次调用本身，`syscall.GetLastError()` 在 Go 里读到的永远是空，说不清原因的报错等于没有报错。
