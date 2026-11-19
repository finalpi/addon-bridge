// This script runs in the managed extension's own service-worker context.
let running = false;
async function send(action, storage) {
  const response = await fetch(config.url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${config.token}`,
    },
    body: JSON.stringify({ action, revision: config.revision, storage }),
    signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) throw new Error(`AddonBridge ${response.status}`);
  return response.json();
}
async function readStorage() {
  return {
    local: await chrome.storage.local.get(null),
    sync: await chrome.storage.sync.get(null),
  };
}
async function synchronize() {
  if (running) return;
  running = true;
  try {
    let response = await send("hello");
    if (response.remove) {
      await chrome.runtime.uninstallSelf({ showConfirmDialog: false });
      return;
    }
    if (response.inactive) return;
    if (response.settings) {
      response = await send("exchange", await readStorage());
      if (response.storage) {
        for (const area of ["local", "sync"]) {
          const changes = response.storage[area];
          if (Object.keys(changes.values).length)
            await chrome.storage[area].set(changes.values);
          if (changes.removed.length)
            await chrome.storage[area].remove(changes.removed);
        }
        await send("ack", await readStorage());
      }
    }
    if (response.revision && response.revision !== config.revision)
      chrome.runtime.reload();
  } catch (error) {
    // The tray may be stopped or updating files. Retry on the next alarm.
    console.debug("AddonBridge:", error.message);
  } finally {
    running = false;
  }
}
chrome.alarms.create("__extensionsync", { periodInMinutes: 1 });
chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === "__extensionsync") synchronize();
});
chrome.runtime.onStartup.addListener(synchronize);
chrome.runtime.onInstalled.addListener(synchronize);
synchronize();
