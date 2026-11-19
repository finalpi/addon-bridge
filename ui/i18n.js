let currentLanguage = "en";
const english = {
  等待在浏览器中加载: "Waiting for browser installation",
  打开扩展管理页: "Open extensions page",
  "已安装时点击扩展卡片上的重新加载。首次安装请选择加载已解压的扩展程序，并使用上方目录。": "For an installed extension, select Reload on its card. For a first installation, select Load unpacked and choose the folder above.",
  "默认使用现有用户资料。下载后可从扩展管理页加载，已安装的扩展可在同一页面重新加载。": "Use your existing profile. Open the browser’s extensions page to load a downloaded extension or reload an installed one.",

  检查更新: "Check updates",
  已安装: "Installed",
  最新: "Latest",
  未安装: "Not installed",
  未检查: "Not checked",
  安装包: "Package",
  编辑: "Edit",
  保存: "Save",
  取消: "Cancel",
  仓库: "Repository",
  等待浏览器退出: "Waiting for browser exit",
  "已注册，等待下次启动": "Registered; available on next start",
  等待后台注册: "Waiting for background registration",
  后台安装: "Install in background",
  "浏览器退出后自动注册，下次正常启动时加载；安装过程不会打开窗口。":
    "Registration runs after the browser exits. The extension loads on its next normal start. Installation does not open a browser window.",
  "默认使用你现有的用户资料。程序在浏览器退出后后台注册扩展，下次正常启动时加载。安装过程不会打开浏览器窗口。":
    "Your existing profile is used by default. Registration runs after the browser exits, and the extension loads on its next normal start. Installation does not open a browser window.",
  扩展管理: "Extensions",
  浏览器: "Browsers",
  同步设置: "Sync settings",
  后台运行中: "Running in the background",
  "关闭此页面后，托盘程序会继续自动同步。":
    "Automatic sync continues from the system tray after this page is closed.",
  你的浏览器工具箱: "Your browser toolkit",
  立即同步: "Sync now",
  "正在连接管理程序…": "Connecting to AddonBridge…",
  "GitHub 扩展": "GitHub extensions",
  添加扩展: "Add extension",
  "从 GitHub Release 获取 Chrome 成品 ZIP，自动跟踪最新稳定版本。":
    "Install Chrome ZIP packages from GitHub Releases and follow the latest stable version.",
  "GitHub 仓库": "GitHub repository",
  安装包匹配规则: "Release asset pattern",
  浏览器用户资料: "Browser profiles",
  "默认使用你现有的用户资料。点击立即同步即可自动安装。安装时程序会短暂操作浏览器窗口；之后由托盘程序自动更新和同步。":
    "Your existing profile is used by default. Select Sync now to install automatically. The installer briefly operates a browser window, then the tray app handles updates and synchronization.",
  自动同步: "Automatic synchronization",
  重启浏览器: "Restart browser",
  "随系统启动 AddonBridge": "Start AddonBridge with the system",
  "检查扩展更新，并同步已开启的插件设置。":
    "Check for extension updates and synchronize settings you have enabled.",
  暂停自动同步: "Pause automatic sync",
  恢复自动同步: "Resume automatic sync",
  自动同步已暂停: "Automatic sync paused",
  "同步间隔（分钟）": "Sync interval (minutes)",
  保存间隔: "Save interval",
  未连接: "Not connected",
  已连接: "Connected",
  "在各台电脑使用同一个 Gist 和共享密码。清单和扩展设置会加密后上传。":
    "The exported file contains complete sync credentials. Import it on another computer for setup. Keep the file private.",
  共享加密密码: "Shared encryption password",
  "具有 Gist 读写权限": "Token with Gist read/write access",
  "至少 12 个字符": "At least 12 characters",
  保存到系统凭据库: "Remember in the system credential store",
  保存凭据: "Save credentials",
  复制同步密码: "Copy sync password",
  "导出完整同步配置（含 Token）": "Export full sync config (includes token)",
  导入完整同步配置: "Import full sync config",
  "已有 Gist ID": "Existing Gist ID",
  "粘贴另一台电脑的 Gist ID": "Paste the Gist ID from your other computer",
  "连接 Gist": "Connect Gist",
  "创建 Gist": "Create Gist",
  "扩展设置同步支持 chrome.storage.local 和 chrome.storage.sync。每个扩展可单独开启，可能包含该扩展的历史记录或登录状态。":
    "Settings sync supports chrome.storage.local and chrome.storage.sync. Enable it per extension. These settings may include extension history or sign-in state.",
  语言: "Language",
  跟随系统: "System language",
  "本机管理 · 加密同步": "Local management · Encrypted sync",
  "添加一个 GitHub 扩展，再到“浏览器”启动 Chrome 或 Brave。":
    "Add a GitHub extension, then configure Chrome or Brave under Browsers.",
  等待同步到已启动的浏览器: "Waiting for synchronization",
  同步扩展设置: "Sync extension settings",
  移除: "Remove",
  个扩展: "extensions",
  等待自动安装: "Waiting for automatic installation",
  等待浏览器连接: "Waiting for the browser",
  等待重新加载: "Waiting for reload",
  复制目录: "Copy folder",
  请选中上方目录并复制: "Select and copy the folder path above.",
  一键安装: "Install automatically",
  "点击立即同步自动完成安装；安装完成后自动更新并同步设置。":
    "Select Sync now to install automatically. Updates and settings synchronization follow automatically.",
  已添加: "Configured",
  已检测到: "Detected",
  未检测到: "Not detected",
  "使用程序创建的独立资料目录。":
    "Use a separate profile created by AddonBridge.",
  "使用现有的登录状态、书签和扩展。":
    "Use your existing sign-ins, bookmarks and extensions.",
  "现有用户资料（默认）": "Existing profile (default)",
  程序专用资料: "Separate managed profile",
  应用资料选择: "Apply profile",
  打开所选资料: "Open selected profile",
  添加浏览器: "Add browser",
  "正在处理，请稍候…": "Working, please wait…",
  尚未同步: "No synchronization yet",
  上次同步: "Last synchronization",
  "凭据已就绪。密码和 Token 不会回显。":
    "Credentials are ready. The token is never displayed. Use Copy sync password when setting up another computer.",
  "尚未设置同步凭据。": "Sync credentials have not been configured.",
  准备就绪: "Ready",
  操作完成: "Operation completed",
  同步已完成: "Synchronization completed",
  凭据已设置: "Credentials saved",
  请先添加浏览器: "Add the browser first.",
  请等待当前操作完成: "Wait for the current operation to finish.",
  "正在处理上一项操作，请稍后再试":
    "Another operation is running. Please try again shortly.",
  "请填写 Token，以及至少 12 个字符的密码":
    "Enter a token and a password containing at least 12 characters.",
  "系统凭据库保存失败，可取消记住后仅在本次使用":
    "The system credential store could not save these credentials. Uncheck Remember to use them for this session.",
  无法删除之前保存的系统凭据: "Could not remove previously saved credentials.",
  请从托盘重新打开管理面板: "Reopen the dashboard from the system tray.",
};
function t(text) {
  return currentLanguage === "zh-CN"
    ? text
    : english[String(text).trim()] || text;
}
const originalText = new WeakMap();
const originalPlaceholder = new WeakMap();
function translatePage() {
  document.documentElement.lang = currentLanguage;
  document.title = `AddonBridge · ${t("扩展管理")}`;
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  while (walker.nextNode()) {
    const node = walker.currentNode;
    if (node.parentElement.closest("#extension-list, #browser-list, #error"))
      continue;
    if (!originalText.has(node)) originalText.set(node, node.textContent);
    const original = originalText.get(node);
    node.textContent = original.replace(original.trim(), t(original.trim()));
  }
  for (const node of document.querySelectorAll("[placeholder]")) {
    if (!originalPlaceholder.has(node))
      originalPlaceholder.set(node, node.placeholder);
    node.placeholder = t(originalPlaceholder.get(node));
  }
  const selected = document.querySelector("[data-tab].selected");
  if (selected)
    document.getElementById("heading").textContent = selected.textContent;
}
