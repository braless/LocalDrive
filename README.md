# LocalDrive v2.4.0

[![Release](https://github.com/braless/LocalDrive/actions/workflows/release.yml/badge.svg)](https://github.com/braless/LocalDrive/actions/workflows/release.yml)
[![CI](https://github.com/braless/LocalDrive/actions/workflows/ci.yml/badge.svg)](https://github.com/braless/LocalDrive/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

一个**零依赖的文件夹发布服务**：把本机任意多个目录通过 HTTP 共享出去，并把「发布某个文件 → 拿到一条可分享链接」压缩成一次右键操作。

纯 Go 标准库实现，编译产物是**单个可执行文件**，无运行时依赖（不需要 .NET / Python / VC 运行库 / glibc），**无需管理员或 root 权限**即可运行。

| 平台 | 产物 | 形态 | 系统集成 |
| --- | --- | --- | --- |
| Windows | `LocalDrive.exe`（8,126,464 字节） | GUI 子系统，托盘常驻，无控制台窗口 | 资源管理器右键菜单 + Win11 一级菜单 + 托盘菜单 |
| Linux amd64 | `LocalDrive-linux`（7,565,472 字节） | 控制台前台进程 | 无（依赖信号退出，可自行封装 systemd） |
| Linux arm64 | `LocalDrive-linux-arm64`（6,946,976 字节） | 控制台前台进程 | 同上（树莓派 / ARM 服务器 / 群晖 ARM 机型） |

> **v2.4 变更**
> - 产品更名为 **LocalDrive**：可执行文件、程序数据目录、日志、注册表项、右键菜单、Win11 稀疏包、模块名全部统一
> - 新增 `[site]` 配置段：**网页左上角名称**、**浏览器标签标题**、**底部介绍文字**、**自定义标签图标（favicon）**
> - 新增 `[network] publicURL`：自定义分享链接的地址前缀（域名 / 反向代理），优先于本机 IP
> - 图标支持五种写法：图片文件路径、emoji / 短文本、内联 `<svg>`、`http(s)://` 地址、`data:` URI
> - 设置页新增「站点外观」区块，图标支持**实时预览**与**选择本地图片**（≤ 48 KB 直接内联进 config.ini）
> - 同时产出 **Windows amd64 / Linux amd64 / Linux arm64** 三个平台的二进制

> **v2.3 变更**
> - `config.ini` 移到**程序目录**，与可执行文件同级；Windows 下右键 exe 即可「编辑配置」
> - 支持配置**多个展示目录**：只配一个时直接展示该目录内容；配多个时先展示目录列表，再进入各自的下一级
> - 发布目录改为**临时存放点**，按 `[publish] ttlMinutes`（默认 60 分钟）**自动清理**
> - 预览弹窗两侧新增**上一个 / 下一个**按钮（支持 `←` `→`），顶部「新窗口打开」改为**全屏**
> - 图片预览支持**滚轮缩放**、**按住拖动**平移、双击在 100% / 200% 间切换
> - **新增 Linux 平台支持**（`main_linux.go` + `stub_linux.go`，通过 build tag 条件编译）

---

## 一、平台功能对照

| 能力 | Windows | Linux | 说明 |
| --- | :---: | :---: | --- |
| HTTP 文件服务 / 目录列举 | ✅ | ✅ | 端口占用自动顺延 |
| 多展示目录（`@N` 虚拟根） | ✅ | ✅ | 逻辑完全共用 |
| 网页列表 / 分页 / 排序 / 筛选 | ✅ | ✅ | 前端代码完全共用 |
| 图片·视频·音频·PDF·文本预览 | ✅ | ✅ | 浏览器端能力，与服务端 OS 无关 |
| 预览缩放 / 拖动 / 全屏 / 上下切换 | ✅ | ✅ | 同上 |
| 设置页（可视化改配置） | ✅ | ✅ | 仅允许 `127.0.0.1` 访问 |
| 站点外观自定义（名称 / 标题 / 页脚） | ✅ | ✅ | `[site]` 段，模板注入 |
| 自定义标签图标（favicon） | ✅ | ✅ | 路径 / emoji / SVG / URL / data URI |
| 自定义分享链接前缀（`publicURL`） | ✅ | ✅ | 反向代理、域名访问 |
| 发布文件 + 生成链接 | ✅ | ✅ | 逻辑共用 |
| 链接写入剪贴板 | ✅ 原生 Win32 | ⚠️ 需外部工具 | Linux 依赖 `xclip` / `xsel` / `wl-copy` |
| 发布目录 TTL 自动清理 | ✅ | ✅ | 共用同一套清理器 |
| 单实例控制 | ✅ 互斥体 | ✅ `flock` | |
| 系统托盘图标 | ✅ | ❌ | Linux 版无 GUI |
| 资源管理器右键「发布」 | ✅ | ❌ | 需桌面环境集成，未实现 |
| Win11 一级右键菜单 | ✅ | ❌ | 稀疏包 + COM 服务器，Windows 专有 |
| 气泡通知 | ✅ | ⚠️ 打印到 stdout | |
| 打开文件 / URL | ✅ | ⚠️ 需 `xdg-open` | 无桌面环境下静默失败 |
| 图形化选择目录 | ✅ | ⚠️ 需 `zenity`/`kdialog` | Linux 版当前未调用该能力 |
| 自动打开浏览器 | ✅ | ⚠️ 需 `xdg-open` | `autoOpen = true` 时生效 |

---

## 二、快速开始

### 获取预编译版本

到 [Releases](../../releases) 页面按平台下载，无需自行编译：

| 附件 | 适用平台 |
| --- | --- |
| `LocalDrive-windows-amd64.exe` | Windows 10 / 11 x64 |
| `LocalDrive-linux-amd64` | Linux x86-64 |
| `LocalDrive-linux-arm64` | Linux ARM64（树莓派、ARM 服务器） |

每个版本附带 `checksums.txt`。Linux 用 `sha256sum -c checksums.txt` 核对，Windows 用 `certutil -hashfile LocalDrive-windows-amd64.exe SHA256`。

### Windows

1. 把 `LocalDrive.exe` 放到任意目录（**建议独立目录**，因为发布临时目录默认建在它旁边）。
2. **双击运行**。程序不显示窗口，只在任务栏右下角托盘区出现一个图标（Win11 默认折叠进「隐藏的图标」）。
3. 首次运行会自动安装右键菜单、生成 `config.ini`，并弹出气泡提示访问地址。
4. 编辑 `config.ini` 的 `[paths] roots` 填入要展示的目录，保存后托盘右键 →「重新加载配置」。
5. 在资源管理器里对任意文件 / 文件夹点右键 →「发布」→ 链接已复制到剪贴板，直接粘贴即可。

> 想先看一份带注释的完整配置，可参考仓库里的 `config.example.ini`。`config.ini` 含本机路径与 IP，已列入 `.gitignore`，不会被提交。

### Linux

```bash
chmod +x LocalDrive-linux
./LocalDrive-linux
```

首次运行会在**可执行文件同目录**生成 `config.ini`，随后前台阻塞运行，`Ctrl+C` 退出。

```bash
# 后台常驻（简单方式）
nohup ./LocalDrive-linux >/dev/null 2>&1 &

# 发布文件（需已安装 xclip / xsel / wl-copy 之一）
./LocalDrive-linux -publish /path/to/file.pdf
```

> 前台运行时程序会把访问地址打印到终端。由于 Linux 版**不把日志回显到 stdout**，排障请查看日志文件（位置见第五节）。

### Linux：以 systemd 服务常驻（推荐）

```ini
# /etc/systemd/system/localdrive.service
[Unit]
Description=LocalDrive file publishing service
After=network-online.target

[Service]
Type=simple
ExecStart=/opt/localdrive/LocalDrive-linux
WorkingDirectory=/opt/localdrive
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now localdrive
```

---

## 三、已实现功能

### 3.1 HTTP 文件服务

- 监听 `0.0.0.0`，默认端口 `8080`；**被占用时自动向后顺延**（最多尝试 20 个端口，实测常落在 `8081`）。
- 两类文件入口：
  - `/raw/<相对路径>` —— 浏览器内直接查看（图片 / PDF / 文本可直接渲染）
  - `/dl/<相对路径>` —— 强制下载，`Content-Disposition` 按 RFC 2231 编码，中文文件名不乱码
- 支持 HTTP **`Range` 请求**：断点续传、视频拖动进度条均可用（实测返回 `206`）。
- `Cache-Control: no-cache`，避免分享链接拿到旧内容。

### 3.2 多展示目录（`@N` 虚拟根）

配置项 `[paths] roots` 支持多个目录（`|` 或换行分隔）：

- **只配 1 个** → 网页直接列出该目录内容，URL **不带前缀**，保持简洁。
- **配 ≥2 个** → 顶层先列出各目录（名称 + 完整路径），点进去后各自独立浏览；路径以 `@0`、`@1` 标识第几个目录。
- 若发布目录不在任何展示目录内、且里面有文件，会**自动追加为一个顶层根**，保证发布链接永远可访问。
- 目录不存在时会在列表中标记 `missing`，而不是整体报错。
- 非法索引（`@2`、`@99`、`@x`、`@-1`）一律返回 `400`。

### 3.3 发布（Publish）

选中一个或多个文件、或一个文件夹，触发发布：

1. **文件已在发布目录内** → 跳过复制，只刷新 mtime（延长保留期），直接生成链接。
2. **文件在发布目录外** → 复制到发布目录后生成链接。
   - 目标已存在同名但大小不同的文件时，自动改名为 `文件名 (1).ext`，**不覆盖**。
   - 文件夹按整棵树复制，保留子目录结构。
3. 链接写入**系统剪贴板**（多选时为多行），并提示「多久后自动清理」。
4. 若后台服务未运行，会尝试**自动拉起**（Windows 走 `spawnDetached`，Linux 走 `nohup`），最多等待 10 秒。

链接形态：

```
http://192.168.1.7:8081/raw/@0/%E5%9B%BE%E7%89%87.png      ← 多目录模式，第 0 个展示目录
http://192.168.1.7:8081/raw/%E6%96%87%E6%A1%A3.pdf          ← 单目录模式，无前缀
```

### 3.4 发布目录与自动清理

- 默认发布目录为 **`<程序目录>/published`**（Windows 亦可写作 `published`），可在 `publishDir` 中改为任意路径。
- 清理器：**启动后 30 秒首跑**，之后每 **5 分钟**扫描一次，删除 mtime 早于 `now - ttlMinutes` 的文件，再自底向上删除空目录。
- `ttlMinutes = 0` 表示**关闭**自动清理。
- **保留期计时起点是「文件被复制进发布目录的那一刻」**（`copyFile` 会把目标 mtime 设为当前时间，不继承源文件时间），所以刚发布的文件不会被立刻清掉。
- **两道安全护栏**防止误删程序自身：
  1. `ResolvePublishDir()` 发现配置指向包含可执行文件的目录时，自动回退到 `<程序目录>/published`；
  2. `cleanPublishDir()` 内再做一次 `within(dir, exePath())` 检查，命中则跳过清理并记录警告。

### 3.5 网页界面

#### 站点外观（`[site]` 段）

页面不是纯静态的：`serveIndex` / `serveSettings` 在输出前会把模板里的占位符替换成当前配置，因此改完 `config.ini` 刷新页面即生效（无需重启进程）。

| 配置项 | 作用位置 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `[site] name` | 左上角图标右侧 | `文件服务` | 留空回退为默认值 |
| `[site] title` | `<title>` / 浏览器标签 | 与 `name` 相同 | 留空表示**跟随 name**；改 name 时标题自动跟着变 |
| `[site] footer` | 页面底部第二行 | `由 LocalDrive 提供` | 留空则**整行不显示**（CSS `:empty` 隐藏） |
| `[site] favicon` | 标签图标 + 左上角图标 | 内置蓝色方块 | 五种写法，见下 |

`favicon` 的解析顺序（`resolveFavicon()`）：

1. `data:` URI —— 直接解码输出；
2. `http://` / `https://` —— 302 重定向到该地址；
3. 以 `<svg` 或 `<?xml` 开头 —— 当作内联 SVG 原样输出；
4. 文件路径 —— 绝对路径直接读，相对路径按**程序目录**解析；按扩展名给出 MIME（png / ico / jpg / jpeg / gif / webp / bmp / svg），识别不出时用 `http.DetectContentType`；
5. 其余 —— 当作 emoji 或 1~4 个字符，生成纯色圆角图标（文本经 XML 转义，`<` `&` 安全）。

图标 URL 带 `?v=<crc32>` 版本参数，改配置后浏览器会重新拉取，不会沿用旧缓存。

设置页的「浏览器图标」一行提供实时预览：输入框变化后 280 ms 防抖请求 `/api/favicon?v=<候选值>`，由服务端用同一套 `resolveFavicon()` 渲染，所见即所得。点「选择图片」时前端用 `FileReader` 转成 data URI 填入输入框，**≤ 48 KB 的图片直接内联进 config.ini**，不产生额外文件；超过则提示改用文件路径。

#### `/` 文件列表

- **面包屑导航**：单目录模式从根开始；多目录模式顶层显示「所有目录」。
- **分页**：底部分页条显示「共 N 项 / 第 X / Y 页」，可翻首末页；每页条数可在页面上切换（10 / 20 / 50 / 100 / 200 / 500 / 1000，默认取 `pageSize`，选择记入 `localStorage`）。
- **筛选**：关键词即时过滤，`/` 聚焦搜索框，`Esc` 清空。
- **排序**：点表头按名称 / 大小 / 修改时间排序；名称为**自然排序**（`file2` 排在 `file10` 之前）。
- **主题**：跟随系统深浅色。
- **复制链接**：每行可直接复制该文件的 HTTP 地址；目录则复制其真实路径。
- **深链**：`?preview=<相对路径>` 打开即自动弹出预览窗，例如 `http://host:8081/?preview=@0/照片/01.jpg`。

#### 弹窗预览

点击文件名在**当前页面弹出预览层**，背景虚化，页面不跳转。`Esc`、点遮罩或右上角 ✕ 关闭。

| 类型 | 扩展名 | 预览方式 |
| --- | --- | --- |
| 图片 | png jpg jpeg gif bmp webp svg ico avif apng jfif | 等比显示；**滚轮缩放**（10% ~ 2000%，以光标为中心）、**按住拖动**平移、**双击**在 100% / 200% 间切换 |
| 视频 | mp4 webm ogv m4v mov | `<video>` 原生控制条，支持拖动进度 |
| 视频（视解码器） | mkv avi wmv flv ts rmvb | 尝试播放，失败时提示下载 |
| 音频 | mp3 wav ogg oga flac aac m4a opus weba | `<audio>` 原生控制条 |
| PDF | pdf | 内嵌 `<iframe>` |
| 文本 | txt md log json xml yml yaml ini conf csv 及主流源码后缀等 | 等宽字体展示；**超过 600 KB 只显示前 600 KB** |

- **上一个 / 下一个**：预览层左右两侧圆形按钮，在当前目录**可预览的文件**之间循环切换，也支持键盘 `←` `→`；标题右侧显示 `当前序号 / 总数`。
- **全屏**：标题栏 `⛶` 按钮进入 / 退出全屏（`requestFullscreen`），全屏时图标切换为 `⤡`。
- 标题栏另有「下载」按钮；不支持预览的类型会直接提示并给出下载入口。

#### `/settings` 设置页

可视化修改，按区块分组：

| 区块 | 字段 |
| --- | --- |
| 站点外观 | 左上角名称、浏览器标题、底部介绍、浏览器图标（带预览 + 选择图片） |
| 服务 | 监听端口、启动后自动打开浏览器 |
| 展示目录 | 目录列表（多行，一行一个）、发布目录、发布保留时长、对外 IP、对外地址 |
| 列表 | 每页条数、是否显示隐藏文件、列举上限 |
| 隐藏规则 | 隐藏文件类型、隐藏文件规则 |

- **仅接受来自 `127.0.0.1` 的请求**。
- 修改端口后服务立即重启并自动跳转到新地址；其余字段保存后页面自动刷新（外观立即生效）。
- 页面底部列出当前访问地址、展示目录、发布目录、配置文件路径。

### 3.6 系统集成

#### Windows

| 位置 | 菜单项 | 实现方式 |
| --- | --- | --- |
| 任意文件 / 文件夹 | **发布到 LocalDrive** | `HKCU\Software\Classes\*\shell\...`，传统菜单 |
| 任意文件 / 文件夹 | **发布** | Win11 **一级菜单**：稀疏包 + 进程外 COM 服务器（`IExplorerCommand`） |
| `LocalDrive.exe` 本身 | **编辑配置 (LocalDrive)** | `HKCU\...\Applications\LocalDrive.exe\shell\...` |

托盘右键菜单：打开文件列表 / 复制访问地址 / 打开设置页 / 指定端口（预设 5 个 + 自定义）/ 目录配置（逐个打开、添加、移除、打开发布目录、更改发布目录、立即清理）/ 本机 IP / 编辑配置文件 / 重新加载配置 / 安装或移除两种右键菜单 / 查看日志 / 关于 / 退出。

所有注册表写入均在 `HKCU`，**不需要管理员权限**。

#### Linux

- 无托盘、无右键菜单集成。
- `SIGINT` / `SIGTERM` 触发优雅退出：停止清理 goroutine、删除 `state.json`、关闭日志。
- 单实例通过 `/tmp/localdrive.lock` 上的 `flock` 实现。

### 3.7 配置与日志位置

| 项目 | Windows | Linux |
| --- | --- | --- |
| 配置文件 | `<程序目录>\config.ini`（不可写时退回 `%LOCALAPPDATA%\LocalDrive\`） | `<程序目录>/config.ini`（不可写时退回 `/tmp/LocalDrive/`） |
| 发布临时目录 | `<程序目录>\published`（可配置） | `<程序目录>/published`（可配置） |
| 运行日志 | `%LOCALAPPDATA%\LocalDrive\LocalDrive.log` | **`/tmp/LocalDrive/LocalDrive.log`** |
| 运行状态 | `%LOCALAPPDATA%\LocalDrive\state.json` | **`/tmp/LocalDrive/state.json`** |
| 程序图标 | `%LOCALAPPDATA%\LocalDrive\LocalDrive.ico`（代码生成） | 无 |
| Win11 菜单包 | `%LOCALAPPDATA%\LocalDrive\Win11Menu\` | 无 |

> 日志超过 2 MB 自动轮转为 `.old`（**只保留一份历史**）。
> 从旧版本升级时，会把 `%LOCALAPPDATA%\LocalDrive\config.ini`（或更早的 `config.json`）迁移到程序目录，原文件改名为 `.bak`。

### 3.8 安全机制

- **路径穿越防护**：`resolve()` 做 `filepath.Join` + `within()` 双重校验，并 `EvalSymlinks` 后再次校验，防止软链接逃逸。已覆盖 `../`、URL 编码变体、`....//`、双重编码、以及多目录 `@N` 前缀变体。
- **配置接口仅限本机**：`/api/config` 检查 `RemoteAddr` 是否回环地址。
- **隐藏规则**：`[hide] types` 按扩展名、`rules` 按通配符（`*` `?`，不区分大小写）过滤；命中**目录**时整棵子树跳过。
- **发布目录护栏**：见 3.4。
- **无身份认证**（见第六节风险说明）。

---

## 四、命令行参数

| 参数 | Windows | Linux | 说明 |
| --- | :---: | :---: | --- |
| 无参数 | ✅ | ✅ | 正常启动（Windows 托盘常驻 / Linux 前台运行） |
| `-publish <路径...>` | ✅ | ✅ | 发布文件并复制链接 |
| `-edit-config` / `--config` | ✅ | ✅ | 打开 `config.ini` |
| `-console` | ✅ | ❌ | 附加到终端输出实时日志（仅 Windows 有 GUI 子系统） |
| `-install-menu` / `-uninstall-menu` | ✅ | ❌ | 安装 / 移除传统右键菜单 |
| `-install-win11` / `-uninstall-win11` | ✅ | ❌ | 安装 / 移除 Win11 一级菜单稀疏包 |
| `-comserver` | ✅ | ❌ | 作为进程外 COM 服务器运行（由 shell 激活，勿手动调用） |
| `-icon` | ✅ | ❌ | 打印图标文件路径 |

> **注意**：Linux 版**没有实现** `-install-menu`、`-install-win11`、`-comserver`、`-icon`。传入这些参数**不会报错，而是被静默忽略并进入正常启动流程**，容易造成误解。

---

## 五、config.ini 全量说明

```ini
[site]
; 网页左上角显示的名称
name = 文件服务

; 浏览器标签页标题；留空 = 与上面的 name 相同
title =

; 页面底部的介绍文字；留空 = 不显示该行
footer = 由 LocalDrive 提供

; 浏览器标签图标。可填：
;   1) 图片文件路径（相对路径按程序目录解析，支持 png/ico/jpg/gif/webp/svg/bmp）
;   2) emoji 或 1-4 个字符（自动生成纯色图标）
;   3) 内联 <svg>...</svg> 片段
;   4) http(s):// 图片地址
;   5) data:image/...;base64,...
; 留空 = 内置图标
favicon =

[server]
port = 8080              ; 监听端口；被占用时自动向后顺延（最多试 20 个）
autoOpen = false         ; 启动后是否自动打开浏览器

[paths]
; 网页上要展示的目录，可写多个，用 | 分隔，也可以一行写一个
;   只配一个目录 → 网页直接展示该目录下的文件
;   配多个目录   → 网页先展示目录列表，再进入各自的下一级
; 留空 = 不展示任何目录（只保留下面「发布」的临时目录）
roots = D:\共享资料 | E:\照片
publishDir =             ; 「发布」文件的临时存放目录，留空 = <程序目录>/published

[publish]
ttlMinutes = 60          ; 发布文件保留时长（分钟），到期自动删除；0 = 不清理

[network]
urlHost = 192.168.1.7    ; 生成链接时使用的本机 IP，留空 = 自动选择（优先 192.168.x）
publicURL =              ; 分享链接前缀，优先于 urlHost。例：https://files.example.com

[listing]
showHidden = false       ; 是否列举以 . 开头的隐藏文件
pageSize = 100           ; 网页列表每页条数（10 ~ 5000）
maxFiles = 20000         ; /api/all 全量列举接口的数量上限

[hide]
; 按扩展名隐藏（不区分大小写，逗号分隔，不用写点号）
types = tmp, temp, log, bak, swp, swo, lock, pyc

; 按文件名 / 文件夹名通配符隐藏（支持 * 和 ?，逗号分隔）
rules = ~$*, .git, .svn, node_modules, Thumbs.db, desktop.ini
```

补充说明：

- 路径用 `|` 分隔；路径里的逗号**不会**被当作分隔符（Windows 路径可能含逗号）。
- 旧版单目录写法 `root = D:\xxx` 仍然兼容：没有 `roots` 时会自动当作一个目录。
- 同一个 key 出现多次时按行累加，因此可以「一个目录一行」地写 `roots`。
- `publicURL` 不写协议时自动补 `http://`，尾部斜杠会被去掉；填了它就**不再使用** `urlHost` 和实际端口拼地址，适合放在 nginx / frp / Cloudflare 后面。
- `[site] title` 与 `name` 相同时，保存配置会把它写成空值，表示「跟随 name」。
- **通过设置页保存会重写整个文件**，你手写的额外注释与未知字段会丢失（程序只保留自己认识的键）。

---

## 六、构建

需要 Go 1.21+（已在 Go 1.27 验证）。

```bash
# Windows（关键：-H windowsgui，否则会弹 cmd 黑窗口）
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -trimpath -ldflags "-s -w -H windowsgui" -o LocalDrive.exe .

# Linux amd64（CGO_ENABLED=0 → 纯静态，不依赖 glibc）
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags "-s -w" -o LocalDrive-linux .

# Linux arm64（树莓派 / ARM 服务器）
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
  go build -trimpath -ldflags "-s -w" -o LocalDrive-linux-arm64 .
```

也可以直接双击 `build.bat`（仅 Windows）或 `build-all.bat`（三平台一次出齐）。

> **注意**：默认发布目录 `<程序目录>/published` 位于模块根目录下。如果往里发布过 `.go` 源文件，`go build ./...` / `go vet ./...` 会把它当作子包去编译而报错——改用 `go build .` / `go vet .`，或把 `publishDir` 配到模块目录之外。

### 6.1 自动发布（GitHub Actions）

仓库内置两个工作流：

| 工作流 | 触发条件 | 作用 |
| --- | --- | --- |
| `.github/workflows/ci.yml` | 推送到 `main` / `master`、PR、手动 | `gofmt` 校验、`go vet`、三平台编译（不产出文件） |
| `.github/workflows/release.yml` | 推送 `v*` 标签、手动 | 三平台编译 → 上传产物 → 创建 Release 并附校验和 |

发布一个新版本只需打标签：

```bash
git tag v2.4.0
git push origin v2.4.0
```

工作流会在 Ubuntu runner 上交叉编译（`CGO_ENABLED=0`，无需目标平台），产出三个附件：

| 附件 | 平台 |
| --- | --- |
| `LocalDrive-windows-amd64.exe` | Windows 10/11 x64，GUI 子系统（无控制台窗口） |
| `LocalDrive-linux-amd64` | Linux x86-64 |
| `LocalDrive-linux-arm64` | Linux AArch64（树莓派、ARM 服务器、群晖 ARM 机型） |
| `checksums.txt` | 上述文件的 SHA-256 |

Release 说明由 `generate_release_notes` 依据提交记录自动生成。也可在 Actions 页面手动触发 `Release` 工作流（`workflow_dispatch`），但**手动触发不会创建 Release**——`release` job 有 `if: startsWith(github.ref, 'refs/tags/v')` 守卫，只有标签推送才会走到发布环节。

---

## 七、已知问题与限制

按影响程度排序。**标 ⚠️ 的是建议优先关注项。**

### 7.1 两平台共有

| # | 问题 | 说明与影响 |
| --- | --- | --- |
| 1 | ⚠️ **无任何身份认证** | 服务监听 `0.0.0.0`，同网段任何人可读取全部展示目录。**切勿映射到公网**。仅本机使用时也仍对局域网开放，需在防火墙层面限制端口。 |
| 2 | ⚠️ **反向代理会让「仅本机」限制失效** | 设置页与 `/api/config` 依赖 `RemoteAddr` 是否为回环地址判断。若前面挂 nginx / frp 转发，`RemoteAddr` 会变成 `127.0.0.1`，等于把配置写接口暴露给外网。**不要在反代后暴露设置页**。 |
| 3 | ⚠️ **服务端不分页** | `pageSize` 只在前端切片，服务端 `os.ReadDir` 一次性读取整个目录并全部返回。目录内有数万文件时，首次加载会明显变慢、内存占用上升。 |
| 4 | **保存配置会重写整个文件** | 设置页保存时按模板重新生成 `config.ini`，用户手写的额外注释、未知键、键顺序都会丢失。 |
| 5 | **TTL 清理有最长 5 分钟延迟** | 清理器每 5 分钟才跑一次，文件可能比预期多存活最多 5 分钟；首次清理在启动后 30 秒。修改 `ttlMinutes` 后想立刻生效，重启程序比等 ticker 更快。 |
| 6 | **清理按 mtime 判断** | 手动 `touch` 文件、或系统时间被调整（NTP 跳变、虚拟机休眠恢复）会让过期判断偏移。 |
| 7 | **软链接不能指向根目录之外** | `resolve()` 会 `EvalSymlinks` 并拒绝逃逸，因此「根目录内放一个指向外部的软链接」无法访问——这是安全取舍，不是 bug，但会让人意外。 |
| 8 | **隐藏规则只影响列表展示** | 知道文件名仍可通过 `/raw/`、`/dl/` 直接访问。隐藏 ≠ 禁止访问。 |
| 9 | **`/api/all` 是遗留接口** | 前端已不再使用（「全部文件」页签已移除），但接口仍然存在，会遍历全部展示目录。属于额外的信息暴露面与死代码。 |
| 10 | **日志只保留一份历史** | 超过 2 MB 轮转为 `LocalDrive.log.old`，再超就覆盖，没有按天归档。 |
| 11 | **端口 < 1024 需要特权** | Windows 需管理员、Linux 需 root 或 `CAP_NET_BIND_SERVICE`。 |
| 12 | **多根模式会破坏以 `@` 开头的旧链接** | 单根时文件名可以叫 `@1.txt`，链接为 `/raw/@1.txt`；改成多根后首段会被当作根索引解析，`strconv.Atoi` 失败 → 返回 `400`。同理，名为 `@数字` 的**目录**也无法再通过无前缀的旧路径进入。重新从页面点进去拿新链接即可。 |
| 13 | **重叠的展示目录会产生重复 URL** | `ResolveRoots()` 只对完全相同的路径去重，不检查嵌套关系。若同时配置 `D:\data` 与 `D:\data\sub`，同一文件可经 `@0/sub/x` 和 `@1/x` 两个地址访问。 |
| 14 | **`publicURL` 填错会导致链接不可用** | 填了 `publicURL` 后，`/api/info` 的 `baseURL` 与「发布」生成的链接都用它拼接，**不再校验可达性**。若域名解析不到或反代路径前缀不匹配，链接在外部打开是 404，而本机 `127.0.0.1` 仍正常——容易误判为服务故障。 |
| 15 | **站点文本不支持 HTML** | `[site]` 的名称 / 标题 / 页脚都经 `html.EscapeString` 转义后注入，写 `<b>` 会原样显示为文本。这是刻意的防注入取舍。 |
| 16 | **favicon 每次请求都重新读盘** | `/favicon.ico` 无内存缓存，命中即 `os.ReadFile`。图标通常只有几十 KB，浏览器也只在标签页首次加载时请求，实际影响可忽略；但若把图标指向网络路径（UNC / NFS），每次刷新都会产生一次 IO。 |
| 17 | **内联 data URI 图标会让 config.ini 变长** | 设置页「选择图片」会把 ≤ 48 KB 的图片转成 base64 写入 `[site] favicon`，配置文件随之增大约 1/3。这是「不额外产生文件」的代价，图标较大时建议改用文件路径。 |
| 18 | **图标文件路径按程序目录解析，不跟随 config.ini** | 相对路径统一以 `exeDir()` 为基准。若程序目录不可写、`config.ini` 已退回到 `%LOCALAPPDATA%`，把图标放在 config.ini 旁边是找不到的——此时请填绝对路径。 |

### 7.2 Windows 专有

| # | 问题 | 说明与影响 |
| --- | --- | --- |
| 19 | ⚠️ **8080 常被系统保留占用** | Windows 的 `http.sys` 会预留端口段（实测本机 8080 被 PID 4 占用），程序自动顺延到 8081。用 `netsh int ipv4 show excludedportrange protocol=tcp` 可查看保留段。 |
| 20 | ⚠️ **GUI 子系统无控制台输出** | 双击启动后看不到任何报错。排查需用 `-console` 从终端启动，或查看 `%LOCALAPPDATA%\LocalDrive\LocalDrive.log`。 |
| 21 | **Win11 稀疏包依赖开发者模式** | 一级菜单需要免签名注册稀疏包。若系统未开启开发者模式、或企业组策略禁止，注册会失败——**不影响其它功能**，可在托盘菜单重试或退回传统菜单。 |
| 22 | **右键菜单仅当前用户** | 注册在 `HKCU`，换用户登录需重新安装。 |
| 23 | **程序目录不可写时配置会挪位** | 放在 `Program Files` 等受保护目录时，`config.ini` 会退回 `%LOCALAPPDATA%\LocalDrive\`，用户可能找不到。 |
| 24 | **未启用长路径支持** | 未在 manifest 中声明 `longPathAware`，超过 260 字符的路径可能失败。 |
| 25 | **可能被杀软误报** | 未签名 exe + 写注册表 + 操作剪贴板 + 常驻托盘，是启发式扫描的典型特征。 |
| 26 | **单实例作用域为当前登录会话** | 互斥体是 `Local\` 语义，不同登录会话可各起一个实例。 |
| 27 | **托盘图标默认被折叠** | Win11 会把新图标放进「隐藏的图标」，用户常以为程序没启动。 |
| 28 | **发布子进程依赖状态文件** | `-publish` 通过 `state.json` + 端口探测找主程序；若主程序被防火墙拦截或启动很慢，可能等待 10 秒后超时。 |
| 29 | **exe 图标与网页图标是两套** | 托盘 / 资源管理器显示的图标由 `icon.go` 代码绘制并写入 `%LOCALAPPDATA%`，与 `[site] favicon` 无关。改 favicon 不会改变 exe 的图标。 |

### 7.3 Linux 专有

| # | 问题 | 说明与影响 |
| --- | --- | --- |
| 30 | ⚠️ **日志与状态落在 `/tmp`** | Linux 下 `LOCALAPPDATA` / `APPDATA` 均为空，`appDir()` 退回 `os.TempDir()`，即 `/tmp/LocalDrive/`。**重启后日志和 `state.json` 会丢失**，且多用户共享同一目录。建议后续改为 `$XDG_STATE_HOME` 或 `~/.local/state/LocalDrive`。 |
| 31 | ⚠️ **无守护进程管理** | 前台运行，关闭终端即退出。生产环境必须自行封装 systemd / supervisord，或用 `nohup`。 |
| 32 | ⚠️ **剪贴板依赖外部命令** | 需要 `xclip` / `xsel`（X11）或 `wl-copy`（Wayland）。纯 SSH / 无桌面环境下写入失败，但**链接仍会打印到日志与 stdout**，不会丢失。 |
| 33 | **无托盘与右键菜单集成** | 无法像 Windows 那样「选中文件 → 右键发布」，只能命令行调用 `-publish`。 |
| 34 | **`openPath` / `openURL` 静默失败** | 依赖 `xdg-open`；headless 环境下调用无效且不报错。`autoOpen = true` 在服务器上无意义。 |
| 35 | **日志不回显到 stdout** | `logToStd` 恒为 `false`（该开关只由 Windows 的 `-console` 置位）。终端里只看得到启动地址，其余诊断信息必须读文件——而文件又在 `/tmp`（见 #30）。 |
| 36 | **`\|` 分隔符与 Linux 文件名冲突** | Linux 允许文件名 / 目录名包含 `\|`。若某个展示目录的**路径本身**含 `\|`，`roots` 会被错误拆分。Windows 文件名不允许 `\|`，无此问题。 |
| 37 | **单实例锁可能误判** | 锁文件 `/tmp/localdrive.lock` 权限 0666 受 umask 影响。多用户同机时，后一个用户可能因打不开锁文件而被判定为「已在运行」并直接退出。 |
| 38 | **不支持的参数被静默忽略** | `-install-menu`、`-install-win11`、`-comserver`、`-icon` 传入后不报错，直接进入正常启动流程。 |
| 39 | **仅 Linux 参与编译** | 平台文件命名为 `*_linux.go`，文件名后缀已把编译范围限定为 `GOOS=linux`。macOS / BSD 等其他 Unix 平台**不会**包含这些文件，编译会因符号缺失而失败。 |
| 40 | **需匹配目标架构** | 交叉编译产物只对应其 `GOARCH`；在 ARM 设备上运行 amd64 版本会报 `Exec format error`。本版已同时提供 `LocalDrive-linux`（amd64）与 `LocalDrive-linux-arm64`。 |

### 7.4 快速规避建议

| 场景 | 建议 |
| --- | --- |
| 只想本机用 | 防火墙阻止该端口入站；或把 `roots` 指向非敏感目录 |
| 需要公网访问 | **不要直接暴露**。加反代 + 认证（Basic Auth / OAuth），并**屏蔽 `/settings` 与 `/api/config`** |
| 目录特别大 | 拆分为多个子目录作为多个 `roots`；或调小 `pageSize` 减少前端渲染压力（注意服务端仍会全量返回） |
| Linux 长期运行 | 用 systemd 单元 + `Restart=on-failure`；并定期清理 `/tmp/LocalDrive` |
| 不想自动清理发布文件 | `ttlMinutes = 0` |
| 想让发布文件立刻消失 | 重启程序触发首跑（30 秒后执行），而不是等 5 分钟 ticker |
| 想让页面看起来像自己的服务 | 改 `[site]` 的 `name` / `title` / `footer` / `favicon`，无需重启，刷新即可 |
| 挂在域名或反代后面 | 填 `[network] publicURL`，让生成的分享链接指向真实对外地址 |
| 不想让图标塞进 config.ini | `favicon` 填文件名（把图片放程序目录），或填绝对路径 |

---

## 八、源码结构

| 文件 | 作用 | 平台 |
| --- | --- | --- |
| `main.go` | 入口：配置加载、模式分发（托盘 / 发布 / 编辑配置 / 安装菜单 / COM）、单实例、启动流程 | Windows |
| `main_linux.go` | 入口：配置加载、模式分发（发布 / 编辑配置）、单实例、启动流程、信号等待 | Linux |
| `stub_linux.go` | Linux 平台桩：剪贴板（xclip/xsel/wl-copy）、`xdg-open`、`flock` 单实例、`nohup` 拉起、空托盘、右键菜单空实现 | Linux |
| `config.go` | `config.ini` 读写（含注释生成、旧配置迁移）、多目录解析、发布目录护栏、本机 IP 选择 | 共用 |
| `server.go` | HTTP 服务：路由、多目录列举、隐藏规则、文件服务、路径安全、运行时换端口、TTL 清理器 | 共用 |
| `page.go` | 内置前端（列表页 + 设置页，HTML/CSS/JS 全内联，无 CDN；含分页、预览、缩放平移、全屏、站点外观注入） | 共用 |
| `favicon.go` | 站点图标解析（路径 / emoji / 内联 SVG / URL / data URI）与 `/favicon.ico`、`/api/favicon` 预览接口 | 共用 |
| `publish.go` | 发布核心：复制 / 去重、生成链接（含 `@N` 前缀）、写剪贴板、通知、服务保活 | 共用 |
| `log.go` | 文件日志（2 MB 轮转） | 共用 |
| `icon.go` | 图标绘制（代码生成，4 倍超采样抗锯齿，无外部资源） | 共用 |
| `tray_windows.go` | 托盘图标、右键菜单、气泡通知、消息循环、进程间通信 | Windows |
| `win32_windows.go` | Win32 API 封装：窗口、菜单、剪贴板、文件夹选择框、图标、进程 | Windows |
| `shellmenu_windows.go` | 资源管理器右键菜单 + 右键 exe「编辑配置」注册（直接调 advapi32） | Windows |
| `comserver_windows.go` | 进程外 COM 服务器，实现 `IExplorerCommand`（Win11 一级菜单） | Windows |
| `shellpkg_windows.go` | Win11 稀疏包清单生成与注册 / 注销 | Windows |
| `build.bat` | Windows 一键编译脚本 | Windows |

---

## 附：技术要点备忘

- **多根虚拟路径**：路径首段 `@N` 由 `splitRoot()` 解析；`strconv.Atoi` 严格校验，非法值返回 `400`。无前缀路径按第 0 个根处理（兼容旧链接），但首段以 `@` 开头时必定按根索引解析，因此 `@` 开头的文件名在多根模式下需要带前缀访问（见 #12）。
- **`EffectiveRoots()`**：配置目录 +（发布目录不在任何配置目录内、且非空时）自动追加，保证分享链接永远可解析。
- **保留期起点 = 目标文件 mtime**：`copyFile()` 用 `os.Chtimes(dst, now, now)` 而非继承源文件时间。
- **配置路径缓存**：`configPath()` 用 `sync.Once` 缓存，避免每次调用都探测可写性。
- **INI 多行累加**：`parseINI()` 对同名 key 用 `\n` 拼接；`firstLine()` 取标量、`iniList()` 按逗号/竖线/换行拆、`iniPathList()` 只按竖线/换行拆（因 Windows 路径可含逗号）。
- **右键 exe 菜单**：注册在 `HKCU\Software\Classes\Applications\<exe 文件名>\shell\<动词>`，只对该 exe 生效，无需管理员。
- **Win11 一级菜单**：稀疏包（`uap10:AllowExternalContent`）+ `com:ExeServer` 进程外 COM（`LocalDrive.exe -comserver`）实现 `IExplorerCommand`；COM 接口用「手搓 vtable + `syscall.NewCallback`」实现，不依赖 cgo。
