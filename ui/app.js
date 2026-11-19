const $ = (id) => document.getElementById(id);
const fragment = location.hash.slice(1);
if (fragment) {
  sessionStorage.setItem("access", fragment);
  history.replaceState(null, "", "/");
}
const access = sessionStorage.getItem("access") || "";
let state;
let rendering = "";
async function request(path, body) {
  const response = await fetch(path, {
    method: body ? "POST" : "GET",
    headers: {
      Authorization: `Bearer ${access}`,
      "Content-Type": "application/json",
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!response.ok) throw new Error(await response.text());
  return response.json();
}
function showError(error) {
  $("error").textContent = t(error.message);
  $("error").hidden = false;
}
async function act(action, values = {}) {
  try {
    await request("/api/action", { action, ...values });
    $("error").hidden = true;
    await refresh();
    return true;
  } catch (error) {
    showError(error);
    return false;
  }
}
function element(tag, text, className) {
  const node = document.createElement(tag);
  if (text) node.textContent = t(text);
  if (className) node.className = className;
  return node;
}
function button(label, fn, className) {
  const node = element("button", label, className);
  node.type = "button";
  node.disabled = state.busy;
  node.onclick = fn;
  return node;
}
function assetPattern(extension) {
  return !extension.asset ||
    extension.asset === "*.zip" ||
    extension.asset === "*chrome*.zip"
    ? "*chrome*.zip"
    : extension.asset;
}
function versionFromRevision(revision) {
  return revision ? revision.split("/")[0] : "—";
}
function renderExtensions() {
  const list = $("extension-list");
  list.replaceChildren();
  const extensions = Object.values(state.extensions).sort((a, b) =>
    a.repo.localeCompare(b.repo),
  );
  $("extension-count").textContent = `${extensions.length} ${t("个扩展")}`;
  if (!extensions.length) {
    list.append(
      element(
        "div",
        "添加一个 GitHub 扩展，再到“浏览器”启动 Chrome 或 Brave。",
        "empty",
      ),
    );
  }
  for (const extension of extensions) {
    const row = element("div", null, "extension-row");
    row.append(
      element(
        "div",
        extension.repo.split("/")[1][0].toUpperCase(),
        "extension-icon",
      ),
    );
    const details = element("div", null, "extension-details");
    details.append(element("strong", extension.repo));
    const release = (state.releases || {})[extension.repo] || {};
    const installedVersions = [];
    for (const [key, value] of Object.entries(state.installed || {})) {
      if (key.endsWith("/" + extension.repo))
        installedVersions.push(
          `${key.split("/")[0]} ${versionFromRevision(value.revision)}`,
        );
    }
    for (const item of Object.values(state.existing || {})) {
      if (item.repo === extension.repo && item.registeredRevision)
        installedVersions.push(
          `${item.browser} ${versionFromRevision(item.registeredRevision)}`,
        );
    }
    const versionLine = `${t("已安装")}: ${installedVersions.length ? installedVersions.join(" · ") : t("未安装")}  |  ${t("最新")}: ${release.latestVersion || t("未检查")}`;
    details.append(element("p", versionLine));
    details.append(
      element(
        "p",
        `${t("安装包")}: ${assetPattern(extension)}${release.latestAsset ? ` · ${release.latestAsset}` : ""}`,
      ),
    );
    if (release.error)
      details.append(element("p", release.error, "inline-error"));
    row.append(details);
    const actions = element("div", null, "extension-actions");
    const settingsLabel = element("label", null, "check");
    const settingsInput = document.createElement("input");
    settingsInput.type = "checkbox";
    settingsInput.checked = extension.settings;
    settingsInput.disabled = state.busy;
    settingsInput.onchange = () =>
      act("settings", {
        repo: extension.repo,
        value: settingsInput.checked ? "on" : "off",
      });
    settingsLabel.append(
      settingsInput,
      document.createTextNode(t("同步扩展设置")),
    );
    actions.append(settingsLabel);
    actions.append(button("检查更新", () => act("check"), "small-action"));
    actions.append(
      button("编辑", () => showEditForm(extension, row), "small-action"),
    );
    actions.append(
      button(
        "移除",
        () => {
          if (
            confirm(
              currentLanguage === "zh-CN"
                ? `移除 ${extension.repo}？已接入的扩展下次连接时会卸载。`
                : `Remove ${extension.repo}? Connected copies will uninstall on their next connection.`,
            )
          )
            act("remove", { repo: extension.repo });
        },
        "delete",
      ),
    );
    row.append(actions);
    list.append(row);
    renderExistingInstallations(list, extension);
  }
}
function showEditForm(extension, row) {
  const existing = Array.from(
    document.querySelectorAll(".extension-edit"),
  ).find((item) => item.dataset.repo === extension.repo);
  if (existing) {
    existing.remove();
    return;
  }
  const form = element("form", null, "card extension-edit");
  form.dataset.repo = extension.repo;
  const repoLabel = element("label", t("仓库"));
  const repoInput = document.createElement("input");
  repoInput.name = "repo";
  repoInput.required = true;
  repoInput.value = extension.repo;
  repoLabel.append(repoInput);
  const assetLabel = element("label", t("安装包匹配规则"));
  const assetInput = document.createElement("input");
  assetInput.name = "asset";
  assetInput.required = true;
  assetInput.value = assetPattern(extension);
  assetLabel.append(assetInput);
  const actions = document.createElement("div");
  const save = document.createElement("button");
  save.type = "submit";
  save.className = "primary";
  save.textContent = t("保存");
  const cancel = document.createElement("button");
  cancel.type = "button";
  cancel.className = "cancel-edit";
  cancel.textContent = t("取消");
  actions.append(save, cancel);
  form.append(repoLabel, assetLabel, actions);
  form.onsubmit = (event) => {
    event.preventDefault();
    const data = new FormData(form);
    act("edit", {
      repo: extension.repo,
      newRepo: data.get("repo").trim(),
      asset: data.get("asset").trim(),
    });
  };
  cancel.onclick = () => form.remove();
  row.after(form);
}
function renderExistingInstallations(list, extension) {
  const existing = Object.values(state.existing || {}).filter(
    (item) =>
      item.repo === extension.repo &&
      state.browsers.some(
        (b) =>
          b.name === item.browser &&
          b.mode !== "managed" &&
          b.profile === item.profile,
      ),
  );
  for (const item of existing) {
    const block = element("div", null, "existing-install");
    const seen = Boolean(item.activeRevision);
    const current = item.activeRevision === item.revision;
    const recent = seen && Date.now() - new Date(item.seen).getTime() < 180000;
    const status = !seen ? "等待在浏览器中加载"
      : !current ? "等待重新加载"
      : recent ? "已连接" : "等待浏览器连接";
    block.append(element("strong", `${item.browser} · ${item.profile} · ${t(status)}`));
    const path = element("input");
    path.value = item.path;
    path.readOnly = true;
    path.setAttribute("aria-label", "扩展加载目录");
    block.append(path);
    const controls = element("div", null, "install-controls");
    controls.append(
      button("复制目录", async () => {
        try {
          await navigator.clipboard.writeText(item.path);
        } catch (error) {
          path.select();
          showError(new Error("请选中上方目录并复制"));
        }
      }),
    );
    controls.append(button("打开扩展管理页", () => act("install-page", { browser: item.browser })));
    block.append(controls);
    block.append(element("p", "已安装时点击扩展卡片上的重新加载。首次安装请选择加载已解压的扩展程序，并使用上方目录。", "hint"));
    list.append(block);
  }
}
function renderBrowsers() {
  const list = $("browser-list");
  list.replaceChildren();
  const names = {
    brave: "Brave",
    chrome: "Google Chrome",
    edge: "Microsoft Edge",
    chromium: "Chromium",
  };
  for (const browser of state.browsers) {
    const card = element("div", null, "card browser-card");
    const title = element("div", null, "section-title");
    title.append(
      element("h3", names[browser.name]),
      element(
        "span",
        browser.configured
          ? "已添加"
          : browser.available
            ? "已检测到"
            : "未检测到",
        "badge",
      ),
    );
    card.append(title);
    card.append(
      element(
        "p",
        browser.mode === "managed"
          ? "使用程序创建的独立资料目录。"
          : "使用现有的登录状态、书签和扩展。",
      ),
    );
    if (browser.configured) {
      const mode = element("select");
      mode.setAttribute("aria-label", names[browser.name] + " 资料模式");
      for (const [value, label] of [
        ["existing", "现有用户资料（默认）"],
        ["managed", "程序专用资料"],
      ]) {
        const option = element("option", label);
        option.value = value;
        mode.append(option);
      }
      mode.value = browser.mode;
      const profile = element("select");
      profile.setAttribute("aria-label", names[browser.name] + " 用户资料");
      for (const item of browser.profiles) {
        const option = element("option", `${item.name} · ${item.directory}`);
        option.value = item.directory;
        profile.append(option);
      }
      profile.value = browser.profile;
      profile.disabled = mode.value === "managed";
      mode.onchange = () => {
        profile.disabled = mode.value === "managed";
      };
      card.append(mode, profile);
      card.append(
        button("应用资料选择", () =>
          act("profile", {
            browser: browser.name,
            mode: mode.value,
            profile: profile.value,
          }),
        ),
      );
    }
    const control = button(
      browser.configured ? "打开所选资料" : "添加浏览器",
      () =>
        act(browser.configured ? "start" : "browser", {
          browser: browser.name,
        }),
    );
    control.disabled =
      state.busy || (!browser.available && !browser.configured);
    card.append(control);
    if (browser.configured && browser.mode !== "managed") {
      card.append(
        button("重启浏览器", () =>
          act("restart-browser", { browser: browser.name }),
        ),
      );
    }
    list.append(card);
  }
}
async function refresh() {
  try {
    state = await request("/api/state");
    currentLanguage = state.language;
    translatePage();
    $("language").value = state.languagePreference;
    $("status").textContent = state.busy
      ? t("正在处理，请稍候…")
      : t(state.message);
    $("last-sync").textContent = state.lastSync.startsWith("0001")
      ? t("尚未同步")
      : `${t("上次同步")} ${new Date(state.lastSync).toLocaleTimeString()}`;
    $("daemon").textContent = state.paused
      ? t("自动同步已暂停")
      : t("后台运行中");
    $("pause").textContent = state.paused
      ? t("恢复自动同步")
      : t("暂停自动同步");
    $("startup-toggle").checked = state.startOnLogin;
    $("gist-state").textContent = state.gist ? t("已连接") : t("未连接");
    $("credential-state").textContent = state.credentials
      ? t("凭据已就绪。密码和 Token 不会回显。")
      : t("尚未设置同步凭据。");
    if (document.activeElement !== $("gist-id"))
      $("gist-id").value = state.gist;
    if (document.activeElement !== $("interval"))
      $("interval").value = state.interval;
    if (state.failed) showError(new Error(state.message));
    for (const node of document.querySelectorAll("form button,#sync-now"))
      node.disabled = state.busy;
    $("gist-create").disabled = state.busy || Boolean(state.gist);
    const signature = JSON.stringify([
      state.extensions,
      state.installed,
      state.existing,
      state.releases,
      state.browsers,
      state.busy,
      state.language,
    ]);
    if (signature !== rendering) {
      rendering = signature;
      renderExtensions();
      renderBrowsers();
    }
  } catch (error) {
    showError(error);
  }
}
document.querySelectorAll("[data-tab]").forEach(
  (node) =>
    (node.onclick = () => {
      document
        .querySelectorAll(".tab")
        .forEach((tab) => (tab.hidden = tab.id !== node.dataset.tab));
      document
        .querySelectorAll("[data-tab]")
        .forEach((tab) => tab.classList.toggle("selected", tab === node));
      $("heading").textContent = node.textContent;
    }),
);
$("sync-now").onclick = () => act("sync");
$("check-updates").onclick = () => act("check");
$("pause").onclick = () => act("pause");
$("startup-toggle").onchange = () =>
  act("startup", { value: $("startup-toggle").checked ? "on" : "off" });
$("add-form").onsubmit = async (event) => {
  event.preventDefault();
  if (
    await act("add", {
      repo: $("repo").value.trim(),
      asset: $("asset").value.trim() || "*chrome*.zip",
    })
  )
    $("repo").value = "";
};
$("interval-form").onsubmit = (event) => {
  event.preventDefault();
  act("interval", { value: $("interval").value });
};
$("credentials-form").onsubmit = async (event) => {
  event.preventDefault();
  if (
    await act("credentials", {
      token: $("token").value,
      password: $("password").value,
      remember: $("remember").checked,
    })
  ) {
    $("token").value = "";
    $("password").value = "";
  }
};
$("gist-form").onsubmit = (event) => {
  event.preventDefault();
  act("gist", { value: $("gist-id").value.trim() });
};
$("gist-create").onclick = () => act("gist", { value: "create" });
$("language").onchange = () => act("language", { value: $("language").value });
$("export-sync").onclick = async () => {
  try {
    const configuration = await request("/api/action", { action: "export-sync" });
    const blob = new Blob([JSON.stringify(configuration, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "addonbridge-sync.json";
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  } catch (error) {
    showError(error);
  }
};
$("import-sync").onchange = async () => {
  const file = $("import-sync").files[0];
  if (!file) return;
  try {
    const configuration = JSON.parse(await file.text());
    if (!configuration.githubToken || !configuration.password || !configuration.gist) throw new Error("配置文件缺少 Gist、Token 或密码");
    if (!confirm("导入将替换当前的 Gist、GitHub Token、同步密码和浏览器清单，是否继续？")) return;
    await act("import-sync", { value: JSON.stringify(configuration) });
  } catch (error) {
    showError(new Error("同步配置文件无效或无法读取"));
  } finally {
    $("import-sync").value = "";
  }
};
refresh();
setInterval(() => {
  if (!document.hidden) refresh();
}, 2000);
