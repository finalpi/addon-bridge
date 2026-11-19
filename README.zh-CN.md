# AddonBridge · 扩展桥

[English](README.md)

### 功能

- 管理通过 GitHub Release ZIP 分发的 Manifest V3 扩展。
- 默认接入已有的 Chrome、Brave、Edge 或 Chromium 用户资料。
- 下载扩展到固定目录，可从面板直接打开浏览器的扩展管理页。
- 自动检查更新，保持扩展 ID 稳定。
- 通过加密 Gist 同步扩展清单、更新间隔及选中的扩展设置。
- 提供系统托盘和本机管理面板。
- 根据系统语言显示界面，也可手动选择中文或英文。

### 开始使用

1. 下载对应操作系统和架构的压缩包。
2. Windows 打开 `AddonBridge.exe`，macOS 打开 `AddonBridge.app`，Linux 执行 `./addonbridge`。
3. 在“浏览器”中添加浏览器，选择日常使用的现有用户资料。
4. 在“扩展管理”中填写 `OWNER/REPOSITORY` 格式的仓库地址，以及安装包文件名或匹配规则。规则需要准确匹配一个已编译的扩展 ZIP。
5. 点击“立即同步”准备安装包。在扩展卡片中复制目录，再点“打开扩展管理页”。开启开发者模式后，选择“加载已解压的扩展程序”并选中该目录。如果浏览器已从同一目录安装过扩展，点扩展卡片上的“重新加载”即可。

默认安装包规则为 `*chrome*.zip`，可匹配 `name_version_chrome.zip` 这类文件。项目命名方式不同，可以在扩展卡片中编辑仓库和匹配规则。卡片会显示已安装版本和最新版本。“检查更新”只查询 GitHub，不安装；“立即同步”才会下载并应用更新。

管理面板在已安装的浏览器中打开。关闭面板后，托盘程序继续运行。托盘菜单提供立即同步、暂停或恢复和退出操作。也可在界面中主动选择独立的程序专用资料模式。

可在“同步设置”或托盘菜单开启“随系统启动 AddonBridge”。登录系统后，程序会在托盘启动。

Linux 桌面需要支持 StatusNotifierItem。GNOME 可能需要安装 AppIndicator 扩展。

### 加密同步

在“同步设置”中填写具有 Gist 读写权限的 GitHub Token，以及至少 12 个字符的共享密码。在第一台电脑创建私密 Gist，其他电脑使用相同的 Gist ID 和密码连接。

凭据可保存到系统凭据库，或仅在本次运行期间保存在内存中。配置其他电脑时可使用“复制同步密码”。每台设备应分别初始化，以生成独立的设备 ID。

每台设备写入自己的加密快照。数据使用 AES-256-GCM 加密，通过 PBKDF2-SHA256 派生密钥，每次生成随机盐和 nonce。记录按逻辑计数器和设备标识合并，删除标记用于防止旧设备恢复已移除的数据。同一键发生并发修改时，整个值按确定规则选择。

扩展设置同步需要逐个开启，覆盖 `chrome.storage.local` 和 `chrome.storage.sync`。其中可能包含扩展历史记录或登录状态。IndexedDB、localStorage 和浏览器自身设置暂未纳入同步。本机状态文件保存设置明文，云端快照经过加密，Gist 历史保留旧的加密版本。

### 安装与运行

已有资料中的扩展由浏览器安装。后台连接脚本收到更新后会请求重新加载。如果脚本未能连接，可在扩展管理页手动点“重新加载”。

程序会为管理的扩展加入后台连接脚本，以及同步所需的 `alarms`、`storage` 和本机地址访问权限。原有后台脚本在原目录中通过包装脚本加载。连接脚本每分钟检查待应用的变化，扩展版本和 Gist 默认每小时检查一次，可在界面中调整。

Chromium 在安装或重新加载解压扩展时触发 `runtime.onInstalled`。扩展可以通过这个事件注册图片右键菜单等功能。

管理服务只监听 `127.0.0.1`。面板接口验证随机访问令牌、Host 和 Origin，扩展连接令牌限定到对应安装。云端快照不包含 Token 和本机浏览器路径。

Windows 上的 Chrome 和 Brave 已完成运行测试。macOS、Linux 已完成交叉编译，实际运行仍待验证，Edge 也尚未实测。具体环境与验证范围见 `validation.json`。

### 命令行

将大写占位符替换为实际仓库和安装包匹配规则。Windows 下请将命令中的 `addonbridge` 换成 `addonbridge-cli.exe`。

```sh
addonbridge init
addonbridge browser add chrome
addonbridge profile chrome existing Default
addonbridge add OWNER/REPOSITORY "ASSET_PATTERN"
addonbridge settings OWNER/REPOSITORY on
addonbridge gui
addonbridge sync
addonbridge status
```

新安装的数据默认位于操作系统用户配置目录下的 `AddonBridge`。已有安装会自动沿用原数据位置和加密记录。可以通过 `ADDONBRIDGE_HOME` 指定自定义目录。

命令行同步可使用 `ADDONBRIDGE_GITHUB_TOKEN` 和 `ADDONBRIDGE_PASSWORD`。现有资料的连接脚本需要托盘程序运行。桌面操作与命令行操作通过进程锁协调。

### 构建与测试

需要 Go 1.25 或更新版本，发布打包脚本使用 Python 3。

```sh
go test ./...
go vet ./...
go build -trimpath -ldflags="-s -w" -o addonbridge .
python build_release.py
```

构建脚本生成 Windows、macOS、Linux 的 amd64 和 arm64 包，包含桌面程序、命令行程序、源码包和 SHA-256 校验文件。

设置 `ADDONBRIDGE_INTEGRATION=1` 可在临时资料中测试已安装的浏览器。真实 Release 测试还需要设置 `ADDONBRIDGE_RELEASE_TEST=1`、`ADDONBRIDGE_TEST_REPOSITORY` 和 `ADDONBRIDGE_TEST_ASSET`。交叉编译通过仅代表构建成功。
