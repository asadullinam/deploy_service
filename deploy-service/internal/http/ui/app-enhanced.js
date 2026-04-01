/**
 * Улучшения интерфейса — вставьте это в НАЧАЛО app.js
 */

// Состояние панели отладки
const debugState = {
  enabled: window.location.search.includes("debug=1") || window.location.hash.includes("debug=1"),
  logs: [],
  maxLogs: 100,
};

// Расширенное логирование
function debugLog(type, message, data = null) {
  const entry = {
    timestamp: new Date().toISOString(),
    type,
    message,
    data,
  };
  debugState.logs.push(entry);
  if (debugState.logs.length > debugState.maxLogs) {
    debugState.logs.shift();
  }

  console.log(`[${type.toUpperCase()}]`, message, data || "");

  if (debugState.enabled) {
    updateDebugPanel();
  }
}

// Создание панели отладки
function createDebugPanel() {
  if (!debugState.enabled) {
    return;
  }

  const debugPanel = document.createElement("div");
  debugPanel.id = "debug-panel";
  debugPanel.style.cssText = `
    position: fixed;
    bottom: 60px;
    right: 24px;
    width: 420px;
    max-height: 400px;
    background: rgba(20, 20, 20, 0.98);
    color: #0f0;
    padding: 16px;
    border-radius: 12px;
    box-shadow: 0 20px 60px rgba(0,0,0,0.7);
    font-family: 'IBM Plex Mono', monospace;
    font-size: 12px;
    z-index: 10000;
    display: none;
    overflow: hidden;
  `;

  debugPanel.innerHTML = `
    <div style="display: flex; justify-content: space-between; margin-bottom: 12px; border-bottom: 1px solid #0f0; padding-bottom: 8px;">
      <strong style="color: #0f0;">Debug Console</strong>
      <div>
        <button onclick="clearDebugLogs()" style="background: #222; color: #0f0; border: 1px solid #0f0; padding: 2px 8px; border-radius: 4px; cursor: pointer; margin-right: 4px;">Clear</button>
        <button onclick="toggleDebugPanel()" style="background: #222; color: #f00; border: 1px solid #f00; padding: 2px 8px; border-radius: 4px; cursor: pointer;">Close</button>
      </div>
    </div>
    <div id="debug-content" style="max-height: 300px; overflow-y: auto; line-height: 1.4;">
      <div style="color: #888;">Log entries will appear here...</div>
    </div>
  `;

  document.body.appendChild(debugPanel);

  // Добавление кнопки переключения
  const toggleBtn = document.createElement("button");
  toggleBtn.id = "debug-toggle";
  toggleBtn.textContent = "{ Debug }";
  toggleBtn.style.cssText = `
    position: fixed;
    bottom: 24px;
    right: 24px;
    background: linear-gradient(135deg, #0a0 0%, #070 100%);
    color: #000;
    border: none;
    padding: 10px 16px;
    border-radius: 24px;
    cursor: pointer;
    font-family: 'IBM Plex Mono', monospace;
    font-weight: 700;
    font-size: 12px;
    z-index: 9999;
    box-shadow: 0 4px 12px rgba(0, 255, 0, 0.3);
    transition: transform 0.2s;
  `;
  toggleBtn.onclick = toggleDebugPanel;
  toggleBtn.onmouseenter = () => (toggleBtn.style.transform = "translateY(-2px)");
  toggleBtn.onmouseleave = () => (toggleBtn.style.transform = "translateY(0)");

  document.body.appendChild(toggleBtn);
}

function toggleDebugPanel() {
  if (!debugState.enabled) {
    return;
  }
  const panel = document.getElementById("debug-panel");
  if (!panel) {
    return;
  }
  const isVisible = panel.style.display !== "none";
  panel.style.display = isVisible ? "none" : "block";
  if (!isVisible) updateDebugPanel();
}

function clearDebugLogs() {
  debugState.logs = [];
  updateDebugPanel();
}

function updateDebugPanel() {
  const content = document.getElementById("debug-content");
  if (!content) return;

  if (debugState.logs.length === 0) {
    content.innerHTML = '<div style="color: #888;">No log entries</div>';
    return;
  }

  content.innerHTML = debugState.logs
    .slice(-50)
    .reverse()
    .map((log) => {
      const color =
        {
          api: "#0af",
          error: "#f00",
          success: "#0f0",
          info: "#ff0",
          warn: "#fa0",
        }[log.type] || "#888";

      const time = new Date(log.timestamp).toLocaleTimeString();
      const dataStr = log.data
        ? `<br><span style="color: #666;">${JSON.stringify(log.data, null, 2)}</span>`
        : "";

      return `
      <div style="margin-bottom: 8px; padding: 6px; background: rgba(0, 255, 0, 0.05); border-left: 2px solid ${color};">
        <span style="color: #666;">[${time}]</span>
        <span style="color:${color};">[${log.type.toUpperCase()}]</span>
        ${log.message}${dataStr}
      </div>
    `;
    })
    .join("");

  content.scrollTop = 0;
}

// Расширенная обертка над API с логированием
const originalFetch = window.fetch;
window.fetch = async function (...args) {
  const url = args[0];
  const options = args[1] || {};

  debugLog(
    "api",
    `-> ${options.method || "GET"} ${url}`,
    options.body ? JSON.parse(options.body) : null,
  );

  try {
    const response = await originalFetch(...args);
    const clonedResponse = response.clone();

    try {
      const data = await clonedResponse.json();
      if (response.ok) {
        debugLog("success", `<- ${response.status} ${url}`, data);
      } else {
        debugLog("error", `<- ${response.status} ${url}`, data);
      }
    } catch (e) {
      debugLog("info", `<- ${response.status} ${url}`, "non-JSON response");
    }

    return response;
  } catch (error) {
    debugLog("error", `ERROR: ${url}`, error.message);
    throw error;
  }
};

// Расширенные состояния загрузки для кнопок
function setButtonLoading(button, loading = true) {
  if (!button) return;

  if (loading) {
    button.dataset.originalText = button.textContent;
    button.textContent = "Загрузка...";
    button.disabled = true;
    button.style.opacity = "0.7";
  } else {
    button.textContent =
      button.dataset.originalText || button.textContent.replace("Загрузка...", "");
    button.disabled = false;
    button.style.opacity = "1";
  }
}

// Расширенные всплывающие уведомления с типами
function showEnhancedToast(message, type = "info", duration = 3000) {
  debugLog(type, message);

  const toast = document.getElementById("toast");
  if (!toast) return;

  const icons = {
    success: "OK",
    error: "ERR",
    warning: "WARN",
    info: "INFO",
  };

  const colors = {
    success: "rgba(47, 107, 88, 0.98)",
    error: "rgba(163, 59, 59, 0.98)",
    warning: "rgba(139, 98, 54, 0.98)",
    info: "rgba(32, 79, 88, 0.98)",
  };

  toast.textContent = `${icons[type] || ""} ${message}`;
  toast.style.background = colors[type] || colors.info;
  toast.classList.remove("hidden");

  clearTimeout(window.toastTimer);
  window.toastTimer = setTimeout(() => {
    toast.classList.add("hidden");
  }, duration);
}

// Добавление инспектора состояния
function inspectState() {
  console.group("Application State");
  console.log("Current Page:", state.currentPage);
  console.log("Selected Project:", state.selectedProject);
  console.log("All Projects:", state.projects);
  console.log("Billing:", state.billing);
  console.log("Token:", state.token ? "Present" : "Missing");
  console.groupEnd();

  debugLog("info", "State inspected (check console)");
}

// Горячие клавиши для режима отладки
document.addEventListener("keydown", (e) => {
  // Ctrl/Cmd + Shift + D — переключить панель отладки
  if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === "D") {
    e.preventDefault();
    toggleDebugPanel();
  }
  // Ctrl/Cmd + Shift + I — показать состояние
  if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === "I") {
    e.preventDefault();
    inspectState();
  }
});

// Инициализация панели отладки после готовности DOM
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", createDebugPanel);
} else {
  createDebugPanel();
}

// Экспорт для глобального доступа
window.debugLog = debugLog;
window.inspectState = inspectState;
window.setButtonLoading = setButtonLoading;
window.showEnhancedToast = showEnhancedToast;

if (debugState.enabled) {
  console.log("%cDebug tools loaded", "color: #0f0; font-size: 16px; font-weight: bold");
  console.log("%c- Press Ctrl+Shift+D to toggle debug panel", "color: #0af");
  console.log("%c- Press Ctrl+Shift+I to inspect state", "color: #0af");
  console.log("%c- Debug panel shows all API calls and errors", "color: #0af");
}
