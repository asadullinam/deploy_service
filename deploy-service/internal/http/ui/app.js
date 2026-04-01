"use strict";

const state = {
  token: "",
  mode: "login",
  theme: "light",
  projectGitHubTokenConfigured: false,
  currentPage: "dashboard",
  currentProjectTab: "overview",
  projectsBulkEditMode: false,
  searchQuery: "",
  projectStatusFilter: "",
  projects: [],
  selectedProjectId: "",
  selectedProject: null,
  billing: null,
  telegramSettings: null,
  cost: null,
  stages: [],
  selectedStageId: "",
  runtimeStatus: null,
  releases: [],
  projectLogs: null,
  logPodsCatalog: [],
  logContainersCatalog: [],
  logsFullscreen: false,
  logsSearch: "",
  logsLevel: "",
  logsSince: "15m",
  logsPod: "",
  logsContainer: "",
  bulkDeleteSelection: new Set(),
  bulkDeleteReport: null,
  bulkDeleteInProgress: false,
  applyingProjectSettings: false,
  deploymentSettingsDirty: false,
  projectSetupProjectId: "",
  workspaceDataUpdatedAt: 0,
};

const projectStatusLabels = {
  creating: "Создается",
  failed: "Ошибка",
  active: "Активен",
  suspended: "Приостановлен",
  deleting: "Удаляется",
  deleted: "Удален",
};

const releaseStatusLabels = {
  pending: "Ожидает",
  building: "Собирается",
  deploying: "Разворачивается",
  success: "Успешно",
  failed: "Ошибка",
};

const HETZNER_TARIFF = {
  cpuCoreHourRub: 0.72,
  memoryGBHourRub: 0.09,
  storageGBMonthRub: 45.0,
  egressGBRub: 1.08,
  dedicatedLoadBalancerHourRub: 0.75,
  referenceHoursPerMonth: 730,
  includedTrafficTB: 20,
  persistentStorageGB: 5,
};

const pageMeta = {
  dashboard: {
    eyebrow: "",
    title: "",
    subtitle: "",
  },
  about: {
    eyebrow: "",
    title: "О нас",
    subtitle: "",
  },
  projects: {
    eyebrow: "Каталог",
    title: "Проекты",
    subtitle: "Список проектов, поиск, фильтрация по статусу и создание новой среды.",
  },
  project: {
    eyebrow: "Проект",
    title: "Проект",
    subtitle: "Контуры, деплой, состояние сервиса, релизы и доступы выбранной среды.",
  },
  billing: {
    eyebrow: "Аккаунт",
    title: "Биллинг",
    subtitle: "Баланс и транзакции.",
  },
  api: {
    eyebrow: "Справка",
    title: "API",
    subtitle: "Swagger, OpenAPI и самые полезные эндпоинты для ручной проверки.",
  },
};

const els = {
  authView: document.getElementById("authView"),
  appView: document.getElementById("appView"),
  loginTab: document.getElementById("loginTab"),
  registerTab: document.getElementById("registerTab"),
  authToggleSlider: document.querySelector(".auth-toggle-slider"),
  authForm: document.getElementById("authForm"),
  authSubmit: document.getElementById("authSubmit"),
  authError: document.getElementById("authError"),
  authTelegramUsername: document.getElementById("authTelegramUsername"),
  themeToggleButtons: Array.from(document.querySelectorAll("[data-theme-toggle]")),
  themeToggleFab: document.querySelector(".theme-toggle-fab"),
  openProjectGitHubTokenModalButton: document.getElementById("openProjectGitHubTokenModalButton"),
  projectGitHubTokenModal: document.getElementById("projectGitHubTokenModal"),
  projectGitHubTokenModalClose: document.getElementById("projectGitHubTokenModalClose"),
  projectGitHubTokenInput: document.getElementById("projectGitHubTokenInput"),
  toggleProjectGitHubTokenButton: document.getElementById("toggleProjectGitHubTokenButton"),
  saveProjectGitHubTokenButton: document.getElementById("saveProjectGitHubTokenButton"),
  deleteProjectGitHubTokenButton: document.getElementById("deleteProjectGitHubTokenButton"),
  projectGitHubTokenStatus: document.getElementById("projectGitHubTokenStatus"),
  projectSetupModal: document.getElementById("projectSetupModal"),
  projectSetupModalClose: document.getElementById("projectSetupModalClose"),
  projectSetupForm: document.getElementById("projectSetupForm"),
  projectSetupRepositoryUrl: document.getElementById("projectSetupRepositoryUrl"),
  projectSetupBaseBranch: document.getElementById("projectSetupBaseBranch"),
  projectSetupStatusText: document.getElementById("projectSetupStatusText"),
  projectSetupLaterButton: document.getElementById("projectSetupLaterButton"),
  projectSetupSaveButton: document.getElementById("projectSetupSaveButton"),
  logoutButton: document.getElementById("logoutButton"),
  projectTabsSlider: document.querySelector(".project-tabs-slider"),
  navDashboard: document.getElementById("navDashboard"),
  navProjects: document.getElementById("navProjects"),
  navBilling: document.getElementById("navBilling"),
  navFlow: document.getElementById("navFlow"),
  navAbout: document.getElementById("navAbout"),
  projectsEditModeButton: document.getElementById("projectsEditModeButton"),
  projectsBrowseBlock: document.getElementById("projectsBrowseBlock"),
  projectsBulkEditBlock: document.getElementById("projectsBulkEditBlock"),
  projectSearch: document.getElementById("projectSearch"),
  projectStatusFilterBar: document.getElementById("projectStatusFilterBar"),
  projectStatusFilterLabel: document.getElementById("projectStatusFilterLabel"),
  projectStatusFilterClear: document.getElementById("projectStatusFilterClear"),
  repositoryUrl: document.getElementById("repositoryUrl"),
  createProjectForm: document.getElementById("createProjectForm"),
  refreshProjectsButton: document.getElementById("refreshProjectsButton"),
  projectList: document.getElementById("projectList"),
  projectListEmpty: document.getElementById("projectListEmpty"),
  workspaceHeader: document.getElementById("workspaceHeader"),
  workspaceEyebrow: document.getElementById("workspaceEyebrow"),
  workspaceTitle: document.getElementById("workspaceTitle"),
  workspaceSubtitle: document.getElementById("workspaceSubtitle"),
  projectHeaderMeta: document.getElementById("projectHeaderMeta"),
  projectHeaderControls: document.getElementById("projectHeaderControls"),
  headerSelectProjectButton: document.getElementById("headerSelectProjectButton"),
  dashboardPage: document.getElementById("dashboardPage"),
  projectsPage: document.getElementById("projectsPage"),
  projectPage: document.getElementById("projectPage"),
  billingPage: document.getElementById("billingPage"),
  flowPage: document.getElementById("flowPage"),
  aboutPage: document.getElementById("aboutPage"),
  bulkDeleteSelectAllButton: document.getElementById("bulkDeleteSelectAllButton"),
  bulkDeleteClearButton: document.getElementById("bulkDeleteClearButton"),
  bulkDeleteDeleteButton: document.getElementById("bulkDeleteDeleteButton"),
  bulkDeleteSelectionCount: document.getElementById("bulkDeleteSelectionCount"),
  bulkDeleteList: document.getElementById("bulkDeleteList"),
  bulkDeleteEmpty: document.getElementById("bulkDeleteEmpty"),
  bulkDeleteReport: document.getElementById("bulkDeleteReport"),
  dashboardCreateProjectButton: document.getElementById("dashboardCreateProjectButton"),
  dashboardBillingPageButton: document.getElementById("dashboardBillingPageButton"),
  dashboardOpenBillingButton: document.getElementById("dashboardOpenBillingButton"),
  dashboardTopUpButton: document.getElementById("dashboardTopUpButton"),
  statCardAllProjects: document.getElementById("statCardAllProjects"),
  statCardActiveProjects: document.getElementById("statCardActiveProjects"),
  statCardSuspendedProjects: document.getElementById("statCardSuspendedProjects"),
  statProjects: document.getElementById("statProjects"),
  statActiveProjects: document.getElementById("statActiveProjects"),
  statSuspendedProjects: document.getElementById("statSuspendedProjects"),
  dashboardSpotlight: document.getElementById("dashboardSpotlight"),
  summaryStatus: document.getElementById("summaryStatus"),
  suspendButton: document.getElementById("suspendButton"),
  resumeButton: document.getElementById("resumeButton"),
  deleteButton: document.getElementById("deleteButton"),
  tabOverview: document.getElementById("tabOverview"),
  tabStages: document.getElementById("tabStages"),
  tabDeploy: document.getElementById("tabDeploy"),
  tabRuntime: document.getElementById("tabRuntime"),
  tabReleases: document.getElementById("tabReleases"),
  tabLogs: document.getElementById("tabLogs"),
  tabAccess: document.getElementById("tabAccess"),
  tabUrls: document.getElementById("tabUrls"),
  projectTabOverview: document.getElementById("projectTabOverview"),
  projectTabStages: document.getElementById("projectTabStages"),
  projectTabDeploy: document.getElementById("projectTabDeploy"),
  projectTabRuntime: document.getElementById("projectTabRuntime"),
  projectTabReleases: document.getElementById("projectTabReleases"),
  projectTabLogs: document.getElementById("projectTabLogs"),
  projectTabAccess: document.getElementById("projectTabAccess"),
  projectTabUrls: document.getElementById("projectTabUrls"),
  stageIndicatorChip: document.getElementById("stageIndicatorChip"),
  urlsContent: document.getElementById("urlsContent"),
  refreshUrlsButton: document.getElementById("refreshUrlsButton"),
  projectUrlValue: document.getElementById("projectUrlValue"),
  overviewServiceUrls: document.getElementById("overviewServiceUrls"),
  projectGrafanaValue: document.getElementById("projectGrafanaValue"),
  projectCreatedValue: document.getElementById("projectCreatedValue"),
  projectUpdatedValue: document.getElementById("projectUpdatedValue"),
  runtimeSummaryCard: document.getElementById("runtimeSummaryCard"),
  runtimeDetailCard: document.getElementById("runtimeDetailCard"),
  runtimePods: document.getElementById("runtimePods"),
  runtimePodsEmpty: document.getElementById("runtimePodsEmpty"),
  overviewRefreshRuntimeButton: document.getElementById("overviewRefreshRuntimeButton"),
  runtimeRefreshButton: document.getElementById("runtimeRefreshButton"),
  githubForm: document.getElementById("githubForm"),
  toggleGitHubTokenButton: document.getElementById("toggleGitHubTokenButton"),
  detectGitHubButton: document.getElementById("detectGitHubButton"),
  createPrButton: document.getElementById("createPrButton"),
  githubMessage: document.getElementById("githubMessage"),
  loadCostButton: document.getElementById("loadCostButton"),
  costTotalValue: document.getElementById("costTotalValue"),
  costBreakdownValue: document.getElementById("costBreakdownValue"),
  deployPlanSummary: document.getElementById("deployPlanSummary"),
  releaseList: document.getElementById("releaseList"),
  releaseEmpty: document.getElementById("releaseEmpty"),
  refreshReleasesButton: document.getElementById("refreshReleasesButton"),
  releasesStageFilter: document.getElementById("releasesStageFilter"),
  deployStageSlug: document.getElementById("deployStageSlug"),
  createStageForm: document.getElementById("createStageForm"),
  stageNameInput: document.getElementById("stageNameInput"),
  createStageButton: document.getElementById("createStageButton"),
  refreshStagesButton: document.getElementById("refreshStagesButton"),
  stageList: document.getElementById("stageList"),
  stageListEmpty: document.getElementById("stageListEmpty"),
  runtimeStageSelect: document.getElementById("runtimeStageSelect"),
  logsStageSelect: document.getElementById("logsStageSelect"),
  logsSinceSelect: document.getElementById("logsSinceSelect"),
  logsLevelSelect: document.getElementById("logsLevelSelect"),
  logsSearchInput: document.getElementById("logsSearchInput"),
  logsPodInput: document.getElementById("logsPodInput"),
  logsContainerInput: document.getElementById("logsContainerInput"),
  logsRefreshButton: document.getElementById("logsRefreshButton"),
  logsOpenGrafanaButton: document.getElementById("logsOpenGrafanaButton"),
  logsFullscreenButton: document.getElementById("logsFullscreenButton"),
  logsPanel: document.getElementById("logsPanel"),
  logsSummary: document.getElementById("logsSummary"),
  logsViewerShell: document.getElementById("logsViewerShell"),
  logsViewer: document.getElementById("logsViewer"),
  logsEmpty: document.getElementById("logsEmpty"),
  projectGuardBanner: document.getElementById("projectGuardBanner"),
  projectGuardTitle: document.getElementById("projectGuardTitle"),
  projectGuardBody: document.getElementById("projectGuardBody"),
  projectGuardActionButton: document.getElementById("projectGuardActionButton"),
  downloadKubeconfigButton: document.getElementById("downloadKubeconfigButton"),
  rotateKubeconfigButton: document.getElementById("rotateKubeconfigButton"),
  copyKubeconfigBase64Button: document.getElementById("copyKubeconfigBase64Button"),
  copyVisibleKubeconfigBase64Button: document.getElementById("copyVisibleKubeconfigBase64Button"),
  kubeconfigBase64Block: document.getElementById("kubeconfigBase64Block"),
  kubeconfigBase64Value: document.getElementById("kubeconfigBase64Value"),
  kubeconfigMessage: document.getElementById("kubeconfigMessage"),
  accessProjectId: document.getElementById("accessProjectId"),
  accessProjectUrl: document.getElementById("accessProjectUrl"),
  accessProjectGrafana: document.getElementById("accessProjectGrafana"),
  billingBalanceValue: document.getElementById("billingBalanceValue"),
  billingSpentValue: document.getElementById("billingSpentValue"),
  billingAvailableValue: document.getElementById("billingAvailableValue"),
  billingTopUpButton: document.getElementById("billingTopUpButton"),
  topUpButton: document.getElementById("topUpButton"),
  billingPageBalance: document.getElementById("billingPageBalance"),
  billingPageSpent: document.getElementById("billingPageSpent"),
  billingPageAvailable: document.getElementById("billingPageAvailable"),
  telegramSettingsForm: document.getElementById("telegramSettingsForm"),
  telegramUsernameInput: document.getElementById("telegramUsernameInput"),
  telegramNotificationsEnabled: document.getElementById("telegramNotificationsEnabled"),
  telegramSaveButton: document.getElementById("telegramSaveButton"),
  telegramDisconnectButton: document.getElementById("telegramDisconnectButton"),
  telegramBotLink: document.getElementById("telegramBotLink"),
  telegramCopyCodeButton: document.getElementById("telegramCopyCodeButton"),
  telegramStatusCard: document.getElementById("telegramStatusCard"),
  telegramStatusNote: document.getElementById("telegramStatusNote"),
  toast: document.getElementById("toast"),
  confirmModal: document.getElementById("confirmModal"),
  confirmModalTitle: document.getElementById("confirmModalTitle"),
  confirmModalBody: document.getElementById("confirmModalBody"),
  confirmModalClose: document.getElementById("confirmModalClose"),
  confirmModalCancel: document.getElementById("confirmModalCancel"),
  confirmModalSubmit: document.getElementById("confirmModalSubmit"),
};

let pendingConfirmResolver = null;
let lastFocusedElement = null;
let runtimeStatusInFlight = null;
let projectLogsInFlight = null;
let workspaceDataInFlight = null;
const workspaceDataTTL = 15000;
const tabPrefetchDelays = {
  runtime: 150,
  releases: 250,
  access: 350,
  logs: 700,
};
const THEME_STORAGE_KEY = "deploy-service-ui-theme";

function on(element, eventName, handler) {
  if (!element) {
    console.warn("[UI] Event handler не зарегистрирован: элемент не найден");
    return;
  }
  element.addEventListener(eventName, handler);
}

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  headers.set("Accept", "application/json");
  if (options.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (state.token) {
    headers.set("Authorization", `Bearer ${state.token}`);
  }

  console.log(`[API] ${options.method || "GET"} ${path}`);

  // Добавляем таймаут для долгих операций
  const controller = new AbortController();
  const timeout = options.timeout || 90000; // Default 90 seconds for project creation
  const timeoutId = setTimeout(() => {
    controller.abort();
    console.warn(`[API] Request timeout (${timeout}ms): ${path}`);
  }, timeout);

  try {
    const response = await fetch(path, {
      ...options,
      headers,
      signal: controller.signal,
    });

    clearTimeout(timeoutId);

    const contentType = response.headers.get("content-type") || "";
    let payload = null;
    if (contentType.includes("application/json")) {
      payload = await response.json();
    } else {
      payload = await response.text();
    }

    if (!response.ok) {
      console.error(`[API] ${response.status} ${path}`, payload);
      if (
        response.status === 401 ||
        (payload && typeof payload === "object" && payload.error === "user not found")
      ) {
        logout(false);
      }
      const message =
        payload && typeof payload === "object" && payload.error
          ? payload.error
          : `${response.status} ${response.statusText}`;
      throw new Error(message);
    }

    console.log(`[API] OK ${path}`, payload);
    return payload;
  } catch (error) {
    clearTimeout(timeoutId);
    if (error.name === "AbortError") {
      throw new Error(`Запрос превысил лимит времени (${timeout / 1000}s). Попробуйте еще раз.`);
    }
    throw error;
  }
}

function setToken(token) {
  state.token = token;
  try {
    sessionStorage.setItem("deploy_token", token);
  } catch (_) {}
}

function normalizeTheme(value) {
  return String(value || "").toLowerCase() === "dark" ? "dark" : "light";
}

function readStoredTheme() {
  try {
    return normalizeTheme(window.localStorage.getItem(THEME_STORAGE_KEY));
  } catch (error) {
    return "light";
  }
}

function persistTheme(theme) {
  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
  } catch (error) {
    // localStorage может быть недоступен в строгих режимах браузера
  }
}

const ICON_MOON =
  '<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>';
const ICON_SUN =
  '<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>';

function renderThemeToggleButtons() {
  const isDark = state.theme === "dark";
  const icon = isDark ? ICON_SUN : ICON_MOON;
  const label = isDark ? "Светлая тема" : "Темная тема";
  (els.themeToggleButtons || []).forEach((button) => {
    button.innerHTML = icon;
    button.setAttribute("aria-label", label);
    button.setAttribute("title", label);
  });
}

function applyTheme(theme) {
  const normalized = normalizeTheme(theme);
  state.theme = normalized;
  document.documentElement.setAttribute("data-theme", normalized);
  renderThemeToggleButtons();
}

function toggleTheme() {
  const nextTheme = state.theme === "dark" ? "light" : "dark";
  applyTheme(nextTheme);
  persistTheme(nextTheme);
}

function setProjectSetupModalBusy(isBusy) {
  if (els.projectSetupSaveButton) {
    els.projectSetupSaveButton.disabled = isBusy;
    els.projectSetupSaveButton.textContent = isBusy ? "Сохранение..." : "Сохранить настройки";
  }
  if (els.projectSetupLaterButton) {
    els.projectSetupLaterButton.disabled = isBusy;
  }
  if (els.projectSetupModalClose) {
    els.projectSetupModalClose.disabled = isBusy;
  }
}

function openProjectSetupModal(project) {
  if (!els.projectSetupModal) {
    return;
  }
  const projectID = String(project?.id || state.selectedProjectId || "").trim();
  if (!projectID) {
    return;
  }

  state.projectSetupProjectId = projectID;
  if (els.projectSetupRepositoryUrl) {
    els.projectSetupRepositoryUrl.value = buildRepositoryURL(
      project?.repositoryOwner || "",
      project?.repositoryName || "",
    );
  }
  if (els.projectSetupBaseBranch) {
    els.projectSetupBaseBranch.value = (project?.baseBranch || "").trim() || "main";
  }
  if (els.projectSetupStatusText) {
    els.projectSetupStatusText.textContent =
      "После сохранения поля «Репозиторий» и «Ветка» не будут переспрашиваться. GitHub токен настраивается отдельной кнопкой на вкладке «Деплой».";
  }
  setProjectSetupModalBusy(false);

  els.projectSetupModal.classList.remove("hidden");
  els.projectSetupModal.setAttribute("aria-hidden", "false");
  requestAnimationFrame(() => {
    if (els.projectSetupRepositoryUrl) {
      els.projectSetupRepositoryUrl.focus();
    }
  });
}

function closeProjectSetupModal() {
  if (!els.projectSetupModal) {
    return;
  }
  if (document.activeElement instanceof HTMLElement) {
    document.activeElement.blur();
  }
  els.projectSetupModal.classList.add("hidden");
  els.projectSetupModal.setAttribute("aria-hidden", "true");
  state.projectSetupProjectId = "";
}

function openProjectGitHubTokenModal() {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId || !els.projectGitHubTokenModal) {
    return;
  }
  if (els.projectGitHubTokenInput) {
    els.projectGitHubTokenInput.value = "";
    els.projectGitHubTokenInput.type = "password";
  }
  if (els.toggleProjectGitHubTokenButton) {
    els.toggleProjectGitHubTokenButton.setAttribute("aria-label", "Показать токен");
    els.toggleProjectGitHubTokenButton.setAttribute("title", "Показать токен");
  }
  els.projectGitHubTokenModal.classList.remove("hidden");
  els.projectGitHubTokenModal.setAttribute("aria-hidden", "false");
  requestAnimationFrame(() => {
    if (els.projectGitHubTokenInput) {
      els.projectGitHubTokenInput.focus();
    }
  });
}

function closeProjectGitHubTokenModal() {
  if (!els.projectGitHubTokenModal) {
    return;
  }
  if (document.activeElement instanceof HTMLElement) {
    document.activeElement.blur();
  }
  els.projectGitHubTokenModal.classList.add("hidden");
  els.projectGitHubTokenModal.setAttribute("aria-hidden", "true");
}

function clearAuthError() {
  els.authError.textContent = "";
  els.authError.classList.add("hidden");
}

function showAuthError(message) {
  els.authError.textContent = message;
  els.authError.classList.remove("hidden");
}

function showToast(message, tone = "info") {
  els.toast.textContent = message;
  els.toast.classList.remove("hidden");
  clearTimeout(showToast.timer);
  const normalizedTone = String(tone || "info").toLowerCase();
  const messageLength = String(message || "").length;
  const timeoutMs =
    normalizedTone === "error"
      ? 9000
      : normalizedTone === "success"
        ? 3600
        : messageLength > 120
          ? 8000
          : 5200;
  showToast.timer = setTimeout(() => {
    els.toast.classList.add("hidden");
  }, timeoutMs);
}

function closeConfirmModal(result) {
  if (!els.confirmModal || !pendingConfirmResolver) {
    if (els.confirmModal) {
      if (document.activeElement instanceof HTMLElement) {
        document.activeElement.blur();
      }
      els.confirmModal.classList.add("hidden");
      els.confirmModal.setAttribute("aria-hidden", "true");
    }
    return;
  }

  const resolve = pendingConfirmResolver;
  pendingConfirmResolver = null;
  if (document.activeElement instanceof HTMLElement) {
    document.activeElement.blur();
  }
  els.confirmModal.classList.add("hidden");
  els.confirmModal.setAttribute("aria-hidden", "true");
  if (lastFocusedElement instanceof HTMLElement) {
    lastFocusedElement.focus();
  }
  resolve(result);
}

function confirmAction({
  title = "Подтвердите действие",
  description = "Проверьте, что действие действительно нужно выполнить.",
  confirmLabel = "Подтвердить",
  tone = "primary",
}) {
  if (!els.confirmModal) {
    return Promise.resolve(window.confirm(description));
  }

  if (pendingConfirmResolver) {
    closeConfirmModal(false);
  }

  lastFocusedElement =
    document.activeElement instanceof HTMLElement ? document.activeElement : null;
  els.confirmModalTitle.textContent = title;
  els.confirmModalBody.textContent = description;
  els.confirmModalSubmit.textContent = confirmLabel;
  els.confirmModalSubmit.classList.remove("primary-button", "danger-button", "ghost-button");
  els.confirmModalSubmit.classList.add(tone === "danger" ? "danger-button" : "primary-button");
  els.confirmModal.classList.remove("hidden");
  els.confirmModal.setAttribute("aria-hidden", "false");

  return new Promise((resolve) => {
    pendingConfirmResolver = resolve;
    requestAnimationFrame(() => {
      els.confirmModalSubmit.focus();
    });
  });
}

function formatDate(value) {
  if (!value) {
    return "—";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("ru-RU", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function formatMoney(value, digits = 2) {
  return `${Number(value || 0).toFixed(digits)} ₽`;
}

function statusClass(status) {
  return String(status || "neutral").toLowerCase();
}

function displayProjectStatus(status) {
  return projectStatusLabels[status] || status || "—";
}

function displayReleaseStatus(status) {
  return releaseStatusLabels[status] || status || "—";
}

function stageById(stageID) {
  return (state.stages || []).find((stage) => stage.id === stageID) || null;
}

function selectedStage() {
  return stageById(state.selectedStageId);
}

function stageDisplayName(stage) {
  if (!stage) {
    return "production";
  }
  if (stage.name && stage.slug && stage.name.toLowerCase() !== stage.slug.toLowerCase()) {
    return `${stage.name} (${stage.slug})`;
  }
  return stage.name || stage.slug || "stage";
}

function normalizeSelectedStage() {
  const available = (state.stages || []).filter((stage) => stage.status !== "deleted");
  if (available.length === 0) {
    state.selectedStageId = "";
    return;
  }

  const stillExists = available.some((stage) => stage.id === state.selectedStageId);
  if (stillExists) {
    return;
  }

  const production = available.find((stage) => stage.slug === "production");
  state.selectedStageId = (production || available[0]).id;
}

function getProjectGuardState(project = state.selectedProject, billing = state.billing) {
  if (!project || project.status !== "suspended") {
    return null;
  }

  const deletionDueAt = project.deletionDueAt ? formatDate(project.deletionDueAt) : "";
  if (billing && !billing.exemptFromGuard && Number(billing.availableRub || 0) <= 0) {
    return {
      kind: "insufficient-balance",
      title: deletionDueAt
        ? "Проект автоматически приостановлен из-за недостатка средств"
        : "Проект автоматически приостановлен из-за нулевого остатка",
      body: deletionDueAt
        ? `Доступно только ${formatMoney(
            billing.availableRub,
          )}. Если не пополнить баланс до ${deletionDueAt}, проект будет удален окончательно.`
        : `Доступно только ${formatMoney(
            billing.availableRub,
          )}. Сначала пополни баланс, после этого можно безопасно возобновить среду.`,
      actionLabel: "Открыть биллинг",
    };
  }

  return {
    kind: "resume-available",
    title: "Проект находится в paused-состоянии",
    body: "Баланс уже позволяет продолжить работу. Используй безопасное возобновление, чтобы вернуть среду в активное состояние.",
    actionLabel: "Возобновить сейчас",
  };
}

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function escapeAttr(value) {
  return escapeHtml(value);
}

function compactUrlLabel(value) {
  if (!value) {
    return "";
  }
  try {
    const parsed = new URL(normalizeExternalURL(value));
    const path = parsed.pathname && parsed.pathname !== "/" ? parsed.pathname : "";
    return `${parsed.hostname}${parsed.port ? `:${parsed.port}` : ""}${path}`;
  } catch {
    return value;
  }
}

function normalizeExternalURL(value) {
  const raw = String(value || "").trim();
  if (!raw) {
    return "";
  }
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(raw)) {
    return raw;
  }
  return `https://${raw.replace(/^\/+/, "")}`;
}

function renderLinkedValue(value, emptyLabel, options = {}) {
  const kind = options.kind || "url";
  if (!value) {
    return `<span class="resource-empty">${escapeHtml(emptyLabel)}</span>`;
  }
  if (kind === "id") {
    return `<span class="value-code">${escapeHtml(value)}</span>`;
  }

  const normalizedValue = normalizeExternalURL(value);
  const label = options.compact ? compactUrlLabel(normalizedValue) : normalizedValue;
  return `<a class="resource-link" href="${escapeAttr(normalizedValue)}" target="_blank" rel="noreferrer">${escapeHtml(label)}</a>`;
}

function authPayloadFromForm() {
  return {
    email: document.getElementById("authEmail").value.trim(),
    password: document.getElementById("authPassword").value,
    telegramUsername: document.getElementById("authTelegramUsername")?.value.trim() || "",
  };
}

function githubTokenValue() {
  return document.getElementById("githubToken").value.trim();
}

function setMode(mode) {
  state.mode = mode;
  els.loginTab.classList.toggle("active", mode === "login");
  els.registerTab.classList.toggle("active", mode === "register");
  if (els.authToggleSlider) {
    els.authToggleSlider.classList.toggle("register", mode === "register");
  }
  els.authSubmit.textContent = mode === "login" ? "Войти" : "Зарегистрироваться";
  const telegramField = els.authTelegramUsername?.closest(".field");
  const telegramHint = document.querySelector(".auth-field-hint");
  if (telegramField) {
    telegramField.classList.toggle("hidden", mode !== "register");
  }
  if (telegramHint) {
    telegramHint.classList.toggle("hidden", mode !== "register");
  }
  clearAuthError();
}

function renderShell() {
  const authed = Boolean(state.token);
  els.authView.classList.toggle("hidden", authed);
  els.appView.classList.toggle("hidden", !authed);
}

function renderWorkspaceHeader() {
  const hideHeader = ["dashboard", "about", "flow", "projects", "billing"].includes(
    state.currentPage,
  );
  if (els.workspaceHeader) {
    els.workspaceHeader.classList.toggle("hidden", hideHeader);
  }
  if (hideHeader) return;

  const meta = pageMeta[state.currentPage] || pageMeta.dashboard;
  if (state.currentPage === "project" && state.selectedProject) {
    els.workspaceEyebrow.textContent = "Проект";
    els.workspaceTitle.textContent = state.selectedProject.name;
    els.workspaceSubtitle.textContent = "";
  } else if (state.currentPage === "projects" && state.projectsBulkEditMode) {
    els.workspaceEyebrow.textContent = "Проекты";
    els.workspaceTitle.textContent = "Удаление проектов";
    els.workspaceSubtitle.textContent = "Выбери несколько проектов и удали их одним действием.";
  } else {
    els.workspaceEyebrow.textContent = meta.eyebrow;
    els.workspaceTitle.textContent = meta.title;
    els.workspaceSubtitle.textContent = meta.subtitle;
  }
  const showProjectHeader = state.currentPage === "project";
  if (els.projectHeaderMeta) {
    els.projectHeaderMeta.classList.toggle("hidden", !showProjectHeader);
  }
  if (els.projectHeaderControls) {
    els.projectHeaderControls.classList.toggle("hidden", !showProjectHeader);
  }
  if (els.summaryStatus) {
    els.summaryStatus.classList.toggle("hidden", !showProjectHeader);
  }
  els.headerSelectProjectButton.classList.toggle(
    "hidden",
    !state.selectedProjectId || state.currentPage === "project",
  );
}

function renderNavigation() {
  els.navDashboard.classList.toggle("active", state.currentPage === "dashboard");
  els.navProjects.classList.toggle("active", state.currentPage === "projects");
  els.navBilling.classList.toggle("active", state.currentPage === "billing");
  els.navFlow.classList.toggle("active", state.currentPage === "flow");
  els.navAbout.classList.toggle("active", state.currentPage === "about");

  els.dashboardPage.classList.toggle("hidden", state.currentPage !== "dashboard");
  els.projectsPage.classList.toggle("hidden", state.currentPage !== "projects");
  els.projectPage.classList.toggle("hidden", state.currentPage !== "project");
  els.billingPage.classList.toggle("hidden", state.currentPage !== "billing");
  els.flowPage.classList.toggle("hidden", state.currentPage !== "flow");
  els.aboutPage.classList.toggle("hidden", state.currentPage !== "about");

  const tabs = {
    overview: [els.tabOverview, els.projectTabOverview],
    stages: [els.tabStages, els.projectTabStages],
    deploy: [els.tabDeploy, els.projectTabDeploy],
    runtime: [els.tabRuntime, els.projectTabRuntime],
    releases: [els.tabReleases, els.projectTabReleases],
    logs: [els.tabLogs, els.projectTabLogs],
    access: [els.tabAccess, els.projectTabAccess],
    urls: [els.tabUrls, els.projectTabUrls],
  };

  Object.entries(tabs).forEach(([name, [button, page]]) => {
    button.classList.toggle("active", state.currentProjectTab === name);
    page.classList.toggle("hidden", state.currentProjectTab !== name);
  });

  if (els.projectTabsSlider) {
    const activeButton = tabs[state.currentProjectTab]?.[0];
    if (activeButton) {
      els.projectTabsSlider.style.left = `${activeButton.offsetLeft}px`;
      els.projectTabsSlider.style.width = `${activeButton.offsetWidth}px`;
    }
  }
}

function renderProjectsBulkEditMode() {
  if (!els.projectsBrowseBlock || !els.projectsBulkEditBlock || !els.projectsEditModeButton) {
    return;
  }
  const enabled = Boolean(state.projectsBulkEditMode);
  els.projectsBrowseBlock.classList.toggle("hidden", enabled);
  els.projectsBulkEditBlock.classList.toggle("hidden", !enabled);
  els.projectsEditModeButton.textContent = enabled
    ? "Выйти из режима редактирования"
    : "Редактировать";
}

function setProjectsBulkEditMode(enabled) {
  state.projectsBulkEditMode = Boolean(enabled);
  renderProjectsBulkEditMode();
  renderWorkspaceHeader();
}

function statusFilterLabel(status) {
  switch (status) {
    case "active":
      return "Активные";
    case "suspended":
      return "Приостановленные";
    default:
      return "Все проекты";
  }
}

function setProjectStatusFilter(status) {
  if (!["", "active", "suspended"].includes(status)) {
    return;
  }
  state.projectStatusFilter = status;
  renderProjects();
}

function renderProjectStatusFilter() {
  if (!els.projectStatusFilterBar || !els.projectStatusFilterLabel) {
    return;
  }
  const hasFilter = Boolean(state.projectStatusFilter);
  els.projectStatusFilterBar.classList.toggle("hidden", !hasFilter);
  els.projectStatusFilterLabel.textContent = hasFilter
    ? `Фильтр: ${statusFilterLabel(state.projectStatusFilter)}`
    : "";
  if (els.projectStatusFilterClear) {
    els.projectStatusFilterClear.disabled = !hasFilter;
  }
}

function renderProjects() {
  els.projectList.innerHTML = "";
  const query = state.searchQuery.trim().toLowerCase();
  const statusFilter = state.projectStatusFilter;
  const visibleProjects = state.projects.filter((project) => {
    if (project.status === "deleted") {
      return false;
    }
    if (statusFilter && project.status !== statusFilter) {
      return false;
    }
    if (!query) {
      return true;
    }
    return `${project.name} ${project.id}`.toLowerCase().includes(query);
  });

  renderProjectStatusFilter();
  renderProjectsBulkEditMode();

  els.projectListEmpty.classList.toggle("hidden", visibleProjects.length > 0);
  if (visibleProjects.length === 0) {
    if (statusFilter) {
      els.projectListEmpty.textContent = `По фильтру «${statusFilterLabel(statusFilter)}» проектов не найдено.`;
    } else {
      els.projectListEmpty.textContent = "Пока нет проектов. Можно создать первый прямо здесь.";
    }
  }

  visibleProjects.forEach((project) => {
    const card = document.createElement("button");
    card.type = "button";
    card.className = `project-card ${project.id === state.selectedProjectId ? "active" : ""}`;
    card.innerHTML = `
      <div class="project-card-head">
        <h4>${escapeHtml(project.name)}</h4>
        <span class="status-pill ${statusClass(project.status)}">${escapeHtml(displayProjectStatus(project.status))}</span>
      </div>
      <p class="project-card-id">${escapeHtml(project.id)}</p>
      <div class="project-card-meta">
        ${project.publicUrl ? `<span class="project-card-url">${escapeHtml(compactUrlLabel(project.publicUrl))}</span>` : ""}
        ${project.deletionDueAt ? `<span class="project-card-updated">Автоудаление: ${escapeHtml(formatDate(project.deletionDueAt))}</span>` : ""}
        <span class="project-card-updated">Обновлен: ${escapeHtml(formatDate(project.updatedAt))}</span>
      </div>
    `;
    card.addEventListener("click", () => {
      openProjectWhenReady(project.id, state.currentProjectTab).catch((error) => {
        showToast(error.message);
      });
    });
    els.projectList.appendChild(card);
  });
}

function bulkDeletableProjects() {
  return state.projects.filter((project) => project.status !== "deleted");
}

function normalizeBulkDeleteSelection() {
  const allowedIds = new Set(bulkDeletableProjects().map((project) => project.id));
  const normalized = new Set(
    Array.from(state.bulkDeleteSelection).filter((projectID) => allowedIds.has(projectID)),
  );
  state.bulkDeleteSelection = normalized;
}

function renderBulkDeletePage() {
  if (
    !els.bulkDeleteList ||
    !els.bulkDeleteEmpty ||
    !els.bulkDeleteSelectionCount ||
    !els.bulkDeleteSelectAllButton ||
    !els.bulkDeleteClearButton ||
    !els.bulkDeleteDeleteButton ||
    !els.bulkDeleteReport
  ) {
    return;
  }

  normalizeBulkDeleteSelection();
  const projects = bulkDeletableProjects();
  const selectedCount = state.bulkDeleteSelection.size;
  const hasProjects = projects.length > 0;

  els.bulkDeleteSelectionCount.textContent = String(selectedCount);
  const countRow = document.getElementById("bulkDeleteSelectionCountRow");
  if (countRow) countRow.classList.toggle("hidden", selectedCount === 0);
  els.bulkDeleteSelectAllButton.disabled =
    !hasProjects || state.bulkDeleteInProgress || selectedCount === projects.length;
  els.bulkDeleteClearButton.disabled = selectedCount === 0 || state.bulkDeleteInProgress;
  els.bulkDeleteDeleteButton.disabled = selectedCount === 0 || state.bulkDeleteInProgress;
  els.bulkDeleteDeleteButton.textContent = state.bulkDeleteInProgress
    ? "Удаление..."
    : "Удалить выбранные";

  els.bulkDeleteList.innerHTML = "";
  els.bulkDeleteEmpty.classList.toggle("hidden", hasProjects);

  projects.forEach((project) => {
    const row = document.createElement("label");
    row.className = "bulk-delete-item";
    row.innerHTML = `
      <span class="bulk-delete-checkbox-wrap">
        <input
          class="bulk-delete-checkbox"
          type="checkbox"
          data-project-id="${escapeAttr(project.id)}"
          ${state.bulkDeleteSelection.has(project.id) ? "checked" : ""}
          ${state.bulkDeleteInProgress ? "disabled" : ""}
        />
      </span>
      <span class="bulk-delete-main">
        <strong>${escapeHtml(project.name)}</strong>
        <span class="bulk-delete-meta">${escapeHtml(project.id)}</span>
      </span>
      <span class="status-pill ${statusClass(project.status)}">${escapeHtml(displayProjectStatus(project.status))}</span>
    `;
    els.bulkDeleteList.appendChild(row);
  });

  els.bulkDeleteList.querySelectorAll(".bulk-delete-checkbox").forEach((checkbox) => {
    checkbox.addEventListener("change", (event) => {
      const input = event.target;
      const projectID = input.getAttribute("data-project-id") || "";
      if (!projectID) {
        return;
      }
      if (input.checked) {
        state.bulkDeleteSelection.add(projectID);
      } else {
        state.bulkDeleteSelection.delete(projectID);
      }
      renderBulkDeletePage();
    });
  });

  if (!state.bulkDeleteReport) {
    els.bulkDeleteReport.classList.add("hidden");
    els.bulkDeleteReport.innerHTML = "";
    return;
  }

  const failures = Array.isArray(state.bulkDeleteReport.failures)
    ? state.bulkDeleteReport.failures
    : [];
  const summaryTone = failures.length === 0 ? "success" : "warning";
  const failureList =
    failures.length === 0
      ? `<p class="muted">Ошибок удаления нет.</p>`
      : `<ul class="bulk-delete-failures">${failures
          .map(
            (item) =>
              `<li><strong>${escapeHtml(item.id)}</strong>: ${escapeHtml(item.error || "неизвестная ошибка")}</li>`,
          )
          .join("")}</ul>`;

  els.bulkDeleteReport.classList.remove("hidden");
  els.bulkDeleteReport.innerHTML = `
    <div class="bulk-delete-report-summary ${summaryTone}">
      Удалено успешно: <strong>${escapeHtml(String(state.bulkDeleteReport.successCount || 0))}</strong> из
      <strong>${escapeHtml(String(state.bulkDeleteReport.requestedCount || 0))}</strong>
    </div>
    ${failureList}
  `;
}

function renderDashboard() {
  const listedProjects = state.projects.filter((project) => project.status !== "deleted");
  const activeProjects = listedProjects.filter((project) => project.status === "active").length;
  const suspendedProjects = listedProjects.filter(
    (project) => project.status === "suspended",
  ).length;

  els.statProjects.textContent = String(listedProjects.length);
  els.statActiveProjects.textContent = String(activeProjects);
  els.statSuspendedProjects.textContent = String(suspendedProjects);
  if (els.statCardAllProjects) {
    els.statCardAllProjects.classList.toggle("active-filter", state.projectStatusFilter === "");
  }
  if (els.statCardActiveProjects) {
    els.statCardActiveProjects.classList.toggle(
      "active-filter",
      state.projectStatusFilter === "active",
    );
  }
  if (els.statCardSuspendedProjects) {
    els.statCardSuspendedProjects.classList.toggle(
      "active-filter",
      state.projectStatusFilter === "suspended",
    );
  }

  const project = state.selectedProject;
  if (!project) {
    els.dashboardSpotlight.className = "spotlight-card empty-state";
    els.dashboardSpotlight.innerHTML = `
      <h4>Проект еще не выбран</h4>
      <p>Выбери проект на странице «Проекты», и здесь появится его сводка, состояние и быстрые действия.</p>
    `;
    return;
  }
  const effectivePublicURL = project.publicUrl || state.runtimeStatus?.publicUrl || "";

  els.dashboardSpotlight.className = "spotlight-card";
  els.dashboardSpotlight.innerHTML = `
    <div class="spotlight-head">
      <div>
        <h4>${escapeHtml(project.name)}</h4>
        <p>${escapeHtml(project.id)}</p>
      </div>
      <span class="status-pill ${statusClass(project.status)}">${escapeHtml(displayProjectStatus(project.status))}</span>
    </div>
    <div class="spotlight-grid">
      <div>
        <span>Публичный адрес</span>
        <strong>${renderLinkedValue(effectivePublicURL, "еще не назначен", { compact: true })}</strong>
      </div>
      <div>
        <span>Состояние</span>
        <strong>${escapeHtml(runtimeHeadline())}</strong>
      </div>
      <div>
        <span>Стоимость</span>
        <strong>${escapeHtml(formatMoney(state.cost?.total))}</strong>
      </div>
      <div>
        <span>Релизы</span>
        <strong>${escapeHtml(String(state.releases.length))}</strong>
      </div>
    </div>
    <div class="action-row">
      <button class="primary-button" type="button" data-open-project="${escapeAttr(project.id)}">Открыть проект</button>
      <button class="ghost-button" type="button" data-open-runtime="${escapeAttr(project.id)}">Проверить состояние</button>
    </div>
  `;

  const openProject = els.dashboardSpotlight.querySelector("[data-open-project]");
  const openRuntime = els.dashboardSpotlight.querySelector("[data-open-runtime]");
  if (openProject) {
    openProject.addEventListener("click", () => {
      openProjectWhenReady(project.id, "overview").catch((error) => {
        showToast(error.message);
      });
    });
  }
  if (openRuntime) {
    openRuntime.addEventListener("click", async () => {
      const opened = await openProjectWhenReady(project.id, "runtime");
      if (opened) {
        await loadRuntimeStatus();
      }
    });
  }
}

function renderProjectWorkspace() {
  const project = state.selectedProject;
  if (!project) {
    els.summaryStatus.textContent = "Нет выбранного проекта";
    els.summaryStatus.className = "status-pill neutral";
    renderOverviewServiceURLs("");
    els.projectGrafanaValue.innerHTML = renderLinkedValue("", "—");
    els.accessProjectId.textContent = "—";
    els.suspendButton.disabled = true;
    els.resumeButton.disabled = true;
    els.suspendButton.classList.remove("hidden");
    els.resumeButton.classList.remove("hidden");
    els.deleteButton.disabled = true;
    els.downloadKubeconfigButton.disabled = true;
    els.rotateKubeconfigButton.disabled = true;
    if (els.copyKubeconfigBase64Button) {
      els.copyKubeconfigBase64Button.disabled = true;
    }
    els.projectGuardBanner.classList.add("hidden");
    state.stages = [];
    state.selectedStageId = "";
    renderStageSelectors();
    renderStages();
    renderRuntimeCards();
    renderCost();
    renderReleases([]);
    renderProjectLogs();
    applyProjectDeploymentSettings(null);
    renderProjectGitHubTokenSection();
    return;
  }

  const effectivePublicURL = project.publicUrl || state.runtimeStatus?.publicUrl || "";
  els.summaryStatus.textContent = displayProjectStatus(project.status);
  els.summaryStatus.className = `status-pill ${statusClass(project.status)}`;
  renderOverviewServiceURLs(effectivePublicURL);
  els.projectGrafanaValue.innerHTML = renderLinkedValue(project.grafanaUrl, "—", {
    compact: true,
  });
  els.accessProjectId.textContent = project.id;

  // Обновляем состояния кнопок и добавляем понятные подсказки
  els.suspendButton.disabled = project.status !== "active";
  els.resumeButton.disabled = project.status !== "suspended";
  els.suspendButton.classList.toggle("hidden", project.status !== "active");
  els.resumeButton.classList.toggle("hidden", project.status !== "suspended");
  els.suspendButton.removeAttribute("data-disabled-reason");
  els.resumeButton.removeAttribute("data-disabled-reason");

  els.deleteButton.disabled = false;
  els.downloadKubeconfigButton.disabled = false;
  els.rotateKubeconfigButton.disabled = false;
  if (els.copyKubeconfigBase64Button) {
    els.copyKubeconfigBase64Button.disabled = false;
  }
  renderProjectGuard(project);
  applyProjectDeploymentSettings(project);
  renderStageSelectors();
  renderStages();
  renderRuntimeCards();
  renderCost();
  renderReleases(state.releases);
  renderProjectLogs();
  renderProjectGitHubTokenSection();
}

function renderProjectGuard(project = state.selectedProject) {
  const guard = getProjectGuardState(project, state.billing);
  if (!guard) {
    els.projectGuardBanner.classList.add("hidden");
    return;
  }

  els.projectGuardBanner.classList.remove("hidden");
  els.projectGuardBanner.classList.toggle("warning", guard.kind !== "resume-available");
  els.projectGuardBanner.classList.toggle("success", guard.kind === "resume-available");
  els.projectGuardTitle.textContent = guard.title;
  els.projectGuardBody.textContent = guard.body;
  els.projectGuardActionButton.textContent = guard.actionLabel;
}

function renderBilling() {
  const billing = state.billing;
  const balance = formatMoney(billing?.balanceRub);
  const spent = formatMoney(billing?.spentThisMonth);
  const available = formatMoney(billing?.availableRub);
  els.billingBalanceValue.textContent = balance;
  els.billingSpentValue.textContent = spent;
  els.billingAvailableValue.textContent = available;

  els.billingPageBalance.textContent = balance;
  els.billingPageSpent.textContent = spent;
  els.billingPageAvailable.textContent = available;
  renderTelegramSettings();
}

function telegramStartCommand() {
  const code = state.telegramSettings?.linkCode;
  if (!code) {
    return "";
  }
  return `/start ${code}`;
}

function renderTelegramSettings() {
  const settings = state.telegramSettings || null;
  if (els.telegramUsernameInput) {
    els.telegramUsernameInput.value = settings?.username ? `@${settings.username}` : "";
  }
  if (els.telegramNotificationsEnabled) {
    els.telegramNotificationsEnabled.checked = settings?.notificationsEnabled ?? true;
  }

  if (!els.telegramStatusCard || !els.telegramStatusNote || !els.telegramBotLink) {
    return;
  }

  const linked = Boolean(settings?.linked);
  const hasUsername = Boolean(settings?.username);
  const startCommand = telegramStartCommand();
  const botHref = settings?.deepLinkUrl || "#";

  els.telegramBotLink.href = botHref;
  els.telegramBotLink.classList.toggle("disabled-link", !settings?.deepLinkUrl);
  els.telegramBotLink.setAttribute("aria-disabled", settings?.deepLinkUrl ? "false" : "true");
  els.telegramCopyCodeButton.disabled = !startCommand;

  if (!hasUsername) {
    els.telegramStatusCard.innerHTML = `
      <div class="check-row">
        <strong>1.</strong>
        <span>Укажи Telegram username, чтобы подготовить связку с ботом.</span>
      </div>
      <div class="check-row">
        <strong>2.</strong>
        <span>После сохранения появится deep-link и команда привязки.</span>
      </div>
      <div class="check-row">
        <strong>3.</strong>
        <span>Когда бот получит <code>/start</code>, уведомления станут активными.</span>
      </div>`;
    els.telegramStatusNote.textContent =
      "Telegram пока не настроен. Можно пропустить этот шаг и вернуться позже.";
    return;
  }

  if (linked) {
    els.telegramStatusCard.innerHTML = `
      <div class="check-row">
        <strong>OK</strong>
        <span>Бот уже связан с аккаунтом ${escapeHtml(settings.username)}.</span>
      </div>
      <div class="check-row">
        <strong>&gt;</strong>
        <span>Уведомления будут приходить по биллингу, ошибкам подготовки среды и нагрузке на ресурсы.</span>
      </div>
      <div class="check-row">
        <strong>&gt;</strong>
        <span>Если нужно временно замолчать чат, отправь боту <code>/stop</code> или сними галочку в интерфейсе.</span>
      </div>`;
    els.telegramStatusNote.textContent = settings.connectedAt
      ? `Подключено: ${formatDate(settings.connectedAt)}.`
      : "Бот подключен.";
    return;
  }

  els.telegramStatusCard.innerHTML = `
    <div class="check-row">
      <strong>1.</strong>
      <span>Username <code>@${escapeHtml(settings.username)}</code> сохранен.</span>
    </div>
    <div class="check-row">
      <strong>2.</strong>
      <span>Открой бота и отправь команду <code>${escapeHtml(startCommand || "/start")}</code>.</span>
    </div>
    <div class="check-row">
      <strong>3.</strong>
      <span>После этого платформа начнет присылать warning и critical события.</span>
    </div>`;
  els.telegramStatusNote.textContent = settings.linkExpiresAt
    ? `Код привязки действует до ${formatDate(settings.linkExpiresAt)}.`
    : "Ожидаем подтверждение от Telegram-бота.";
}

async function loadTelegramSettings() {
  try {
    state.telegramSettings = await api("/me/telegram");
  } catch (error) {
    console.warn("failed to load telegram settings", error);
    state.telegramSettings = null;
  }
  renderTelegramSettings();
}

function runtimeHeadline() {
  if (!state.runtimeStatus) {
    return "еще не проверяли";
  }
  if (!state.runtimeStatus.namespaceExists) {
    return "namespace не найден";
  }
  if (!state.runtimeStatus.deploymentExists) {
    return "deployment еще не применен";
  }
  if (Number(state.runtimeStatus.readyReplicas || 0) > 0) {
    if (
      Object.prototype.hasOwnProperty.call(state.runtimeStatus, "httpRouteExists") &&
      !state.runtimeStatus.httpRouteExists
    ) {
      return `готовых реплик: ${state.runtimeStatus.readyReplicas}, но HTTPRoute не найден`;
    }
    return `готовых реплик: ${state.runtimeStatus.readyReplicas}`;
  }
  return "deployment есть, но готовых реплик пока нет";
}

function runtimeSummaryText(runtime) {
  const message = String(runtime?.message || "").trim();
  if (!message) {
    return runtimeHeadline();
  }
  if (message === "mock deployment is healthy") {
    return "Сервис доступен, среда работает штатно.";
  }
  if (message === "application is deployed and has ready replicas") {
    return "Приложение развернуто, готовые реплики доступны.";
  }
  if (message === "namespace exists, but application manifests have not been applied yet") {
    return "Namespace уже создан, но манифесты приложения еще не были применены.";
  }
  if (message === "stage namespace exists, application manifests not yet applied") {
    return "Контур уже создан, но манифесты приложения в него еще не применены.";
  }
  return message;
}

function releaseSummaryText(release) {
  const message = String(release?.commitMessage || "").trim();
  if (
    release?.status === "success" &&
    message.startsWith("Ожидает запуска деплоя из PR:") &&
    state.runtimeStatus &&
    Number(state.runtimeStatus.readyReplicas || 0) > 0
  ) {
    return runtimeSummaryText(state.runtimeStatus);
  }
  return message;
}

function renderRuntimeCards() {
  const runtime = state.runtimeStatus;
  if (!runtime) {
    const empty = `
      <h4>Данные не загружены</h4>
      <p>Нажми «Получить статус», чтобы увидеть namespace, deployment, service, HTTPRoute и поды.</p>
    `;
    els.runtimeSummaryCard.className = "runtime-summary-card empty-state";
    els.runtimeSummaryCard.innerHTML = empty;
    els.runtimeDetailCard.className = "runtime-detail-card empty-state";
    els.runtimeDetailCard.innerHTML = empty;
    els.runtimePods.innerHTML = "";
    els.runtimePodsEmpty.classList.remove("hidden");
    return;
  }

  const headline = runtimeSummaryText(runtime);
  const checkedAt = formatDate(runtime.lastCheckedAt);

  els.runtimeSummaryCard.className = "runtime-summary-card";
  els.runtimeSummaryCard.innerHTML = `
    <div class="runtime-head">
      <div class="runtime-copy">
        <h4>${escapeHtml(headline)}</h4>
        <p>${escapeHtml(runtime.namespace || "—")}</p>
      </div>
      <span class="status-pill ${Number(runtime.readyReplicas || 0) > 0 ? "success" : "warning"}">${escapeHtml(Number(runtime.readyReplicas || 0) > 0 ? "Готов" : "Требует внимания")}</span>
    </div>
    <div class="runtime-metrics">
      <div><span>Namespace</span><strong>${runtime.namespaceExists ? "есть" : "нет"}</strong></div>
      <div><span>Deployment</span><strong>${runtime.deploymentExists ? "есть" : "нет"}</strong></div>
      <div><span>Service</span><strong>${runtime.serviceExists ? "есть" : "нет"}</strong></div>
      <div><span>HTTPRoute</span><strong>${runtime.httpRouteExists ? "есть" : "нет"}</strong></div>
      <div><span>Готовые реплики</span><strong>${escapeHtml(String(runtime.readyReplicas || 0))}</strong></div>
    </div>
    <p class="muted">Последняя проверка: ${escapeHtml(checkedAt)}</p>
  `;

  els.runtimeDetailCard.className = "runtime-detail-card";
  els.runtimeDetailCard.innerHTML = `
    <div class="runtime-detail-main">
      <div class="runtime-metrics wide">
        <div><span>Namespace</span><strong>${escapeHtml(runtime.namespace || "—")}</strong></div>
        <div><span>HTTPRoute</span><strong>${runtime.httpRouteExists ? "есть" : "нет"}</strong></div>
        <div><span>Ожидаемых реплик</span><strong>${escapeHtml(String(runtime.desiredReplicas || 0))}</strong></div>
        <div><span>Готовых реплик</span><strong>${escapeHtml(String(runtime.readyReplicas || 0))}</strong></div>
        <div><span>Доступных реплик</span><strong>${escapeHtml(String(runtime.availableReplicas || 0))}</strong></div>
      </div>
    </div>
    <p class="muted runtime-detail-note">${escapeHtml(headline)}</p>
  `;

  els.runtimePods.innerHTML = "";
  const pods = Array.isArray(runtime.pods) ? runtime.pods : [];
  els.runtimePodsEmpty.classList.toggle("hidden", pods.length > 0);
  pods.forEach((pod) => {
    const item = document.createElement("article");
    item.className = "pod-card";
    item.innerHTML = `
      <div class="pod-card-head">
        <div>
          <h4>${escapeHtml(pod.name || "pod")}</h4>
          <p>${escapeHtml(pod.phase || "неизвестно")}</p>
        </div>
        <span class="status-pill ${pod.ready ? "success" : "warning"}">${escapeHtml(pod.ready ? "Готов" : "Не готов")}</span>
      </div>
      <div class="project-card-meta">
        <span>Рестарты: ${escapeHtml(String(pod.restarts || 0))}</span>
      </div>
    `;
    els.runtimePods.appendChild(item);
  });
}

function renderCost() {
  if (!state.cost) {
    els.costTotalValue.textContent = "0.00 ₽";
    els.costBreakdownValue.textContent = "Выберите проект, чтобы загрузить расчет.";
    return;
  }
  els.costTotalValue.textContent = formatMoney(state.cost.total);
  els.costBreakdownValue.innerHTML = `
    <div class="cost-breakdown-intro">
      Итоговая сумма складывается из четырех компонентов использования ресурсов.
    </div>
    <div class="cost-metrics">
      <article class="cost-metric">
        <span class="cost-metric-label">Процессор</span>
        <strong>${escapeHtml(Number(state.cost.processorCoreHours || 0).toFixed(2))}</strong>
        <span class="cost-metric-note">CPU-часов</span>
      </article>
      <article class="cost-metric">
        <span class="cost-metric-label">Память</span>
        <strong>${escapeHtml(Number(state.cost.memoryGigabyteHours || 0).toFixed(2))}</strong>
        <span class="cost-metric-note">GB-часов RAM</span>
      </article>
      <article class="cost-metric">
        <span class="cost-metric-label">Хранилище</span>
        <strong>${escapeHtml(Number(state.cost.persistentStorageGigabytes || 0).toFixed(2))}</strong>
        <span class="cost-metric-note">GB постоянного диска</span>
      </article>
      <article class="cost-metric">
        <span class="cost-metric-label">Трафик</span>
        <strong>${escapeHtml(Number(state.cost.outgoingTrafficGigabytes || 0).toFixed(2))}</strong>
        <span class="cost-metric-note">GB исходящего трафика</span>
      </article>
    </div>
  `;
}

function deploymentAccessLabel(serviceType) {
  return serviceType === "ClusterIP" ? "Внутренний доступ" : "Публичный адрес";
}

function deploymentProfileMeta(profile) {
  switch (String(profile || "").toLowerCase()) {
    case "starter":
      return {
        title: "Starter",
        summary: "Легкий профиль для простых сервисов и небольших нагрузок.",
        requests: "50m CPU / 64Mi RAM",
        limits: "300m CPU / 256Mi RAM",
        cpuCores: 0.05,
        memoryGB: 64 / 1024,
      };
    case "performance":
      return {
        title: "Performance",
        summary: "Усиленный профиль для высоких нагрузок и требовательных сервисов.",
        requests: "250m CPU / 256Mi RAM",
        limits: "1000m CPU / 1024Mi RAM",
        cpuCores: 0.25,
        memoryGB: 256 / 1024,
      };
    default:
      return {
        title: "Balanced",
        summary: "Сбалансированный профиль для большинства рабочих сервисов.",
        requests: "100m CPU / 128Mi RAM",
        limits: "500m CPU / 512Mi RAM",
        cpuCores: 0.1,
        memoryGB: 128 / 1024,
      };
  }
}

function calculateDeployEstimate(replicaCount, profile, dedicatedLoadBalancer) {
  const replicas = Math.max(1, Number(replicaCount || 1));
  const cpuMonthly =
    profile.cpuCores *
    replicas *
    HETZNER_TARIFF.referenceHoursPerMonth *
    HETZNER_TARIFF.cpuCoreHourRub;
  const memoryMonthly =
    profile.memoryGB *
    replicas *
    HETZNER_TARIFF.referenceHoursPerMonth *
    HETZNER_TARIFF.memoryGBHourRub;
  const storageMonthly = HETZNER_TARIFF.persistentStorageGB * HETZNER_TARIFF.storageGBMonthRub;
  const dedicatedLoadBalancerMonthly = dedicatedLoadBalancer
    ? HETZNER_TARIFF.dedicatedLoadBalancerHourRub * HETZNER_TARIFF.referenceHoursPerMonth
    : 0;
  const monthly = cpuMonthly + memoryMonthly + storageMonthly + dedicatedLoadBalancerMonthly;
  return {
    cpuMonthly,
    memoryMonthly,
    storageMonthly,
    dedicatedLoadBalancerMonthly,
    monthly,
    daily: monthly / 30,
    weekly: (monthly / 30) * 7,
    hourly: monthly / HETZNER_TARIFF.referenceHoursPerMonth,
  };
}

function renderDeployPlanSummary() {
  if (!els.deployPlanSummary) {
    return;
  }
  const serviceType = document.getElementById("serviceType")?.value || "LoadBalancer";
  const dedicatedLoadBalancer = serviceType === "LoadBalancer" && dedicatedLoadBalancerEnabled();
  const replicaCount = document.getElementById("replicaCount")?.value || "1";
  const profile = deploymentProfileMeta(document.getElementById("resourceProfile")?.value);
  const estimate = calculateDeployEstimate(replicaCount, profile, dedicatedLoadBalancer);
  const replicaLabel = `${replicaCount} ${Number(replicaCount) === 1 ? "копия" : Number(replicaCount) < 5 ? "копии" : "копий"}`;
  const accessSummary =
    serviceType === "ClusterIP"
      ? "Сервис будет доступен только внутри инфраструктуры."
      : dedicatedLoadBalancer
        ? "Сервис получит собственный выделенный внешний балансер."
        : "Сервис будет опубликован через общий ingress и доступен по HTTPS.";
  const accessLabel =
    serviceType === "ClusterIP"
      ? "Внутренний доступ"
      : dedicatedLoadBalancer
        ? "Выделенный балансер"
        : "Общий ingress";

  els.deployPlanSummary.innerHTML = `
    <div class="deploy-plan-head">
      <strong>Итоговая конфигурация деплоя</strong>
      <span class="status-pill neutral">${escapeHtml(profile.title)}</span>
    </div>
    <p class="deploy-plan-copy">${escapeHtml(profile.summary)} ${escapeHtml(accessSummary)}</p>
    <div class="deploy-plan-grid">
      <article class="deploy-plan-card">
        <span class="deploy-plan-label">Доступ</span>
        <strong class="deploy-plan-value">${escapeHtml(accessLabel)}</strong>
      </article>
      <article class="deploy-plan-card">
        <span class="deploy-plan-label">Масштаб</span>
        <strong class="deploy-plan-value">${escapeHtml(replicaLabel)}</strong>
      </article>
      <article class="deploy-plan-card">
        <span class="deploy-plan-label">Ресурсы</span>
        <strong class="deploy-plan-value">${escapeHtml(profile.requests)}</strong>
      </article>
    </div>
    <div class="deploy-price-grid">
      <article class="deploy-price-card">
        <span class="deploy-plan-label">В день</span>
        <strong class="deploy-price-value">${escapeHtml(formatMoney(estimate.daily, 3))}</strong>
      </article>
      <article class="deploy-price-card">
        <span class="deploy-plan-label">В неделю</span>
        <strong class="deploy-price-value">${escapeHtml(formatMoney(estimate.weekly))}</strong>
      </article>
      <article class="deploy-price-card">
        <span class="deploy-plan-label">Оценка в месяц</span>
        <strong class="deploy-price-value">${escapeHtml(formatMoney(estimate.monthly))}</strong>
      </article>
    </div>
    <p class="deploy-plan-copy">
      CPU: ${escapeHtml(formatMoney(estimate.cpuMonthly))}/мес, память:
      ${escapeHtml(formatMoney(estimate.memoryMonthly))}/мес, диск
      ${escapeHtml(HETZNER_TARIFF.persistentStorageGB.toFixed(0))} GB:
      ${escapeHtml(formatMoney(estimate.storageMonthly))}/мес${
        dedicatedLoadBalancer
          ? `, выделенный LB: ${escapeHtml(formatMoney(estimate.dedicatedLoadBalancerMonthly))}/мес`
          : ""
      }.
    </p>
    <p class="deploy-plan-copy">
      Оценка учитывает вычислительные ресурсы, память и базовый объем диска. Итоговая сумма может
      меняться вместе с реальным трафиком и длительностью работы среды.
    </p>
    <p class="deploy-plan-copy">Лимиты: ${escapeHtml(profile.limits)}.</p>
  `;
}

function renderReleases(releases) {
  els.releaseList.innerHTML = "";
  els.releaseEmpty.classList.toggle("hidden", Array.isArray(releases) && releases.length > 0);

  (releases || []).forEach((release) => {
    const card = document.createElement("article");
    card.className = "release-card";
    const summary = releaseSummaryText(release);

    const rollbackButton =
      release.status === "success"
        ? `<button class="ghost-button rollback-button" data-release-id="${escapeAttr(release.id)}" type="button">Откатить</button>`
        : "";

    const releaseStage = stageById(release.stageId);
    const releaseStageLabel = releaseStage
      ? stageDisplayName(releaseStage)
      : release.stageId || "без контура";
    const meta = [
      release.stageId !== state.selectedStageId ? `Контур: ${escapeHtml(releaseStageLabel)}` : "",
      release.commitSha ? escapeHtml(release.commitSha) : "",
      release.imageTag ? escapeHtml(release.imageTag) : "",
      escapeHtml(formatDate(release.updatedAt || release.createdAt)),
    ].filter(Boolean);

    card.innerHTML = `
      <div class="panel-head compact">
        <div>
          <h4>${escapeHtml(release.id)}</h4>
          <div class="project-card-meta">
            ${meta.map((item) => `<span>${item}</span>`).join("")}
          </div>
        </div>
        <span class="status-pill ${statusClass(release.status)}">${escapeHtml(displayReleaseStatus(release.status))}</span>
      </div>
      ${summary ? `<p class="muted">${escapeHtml(summary)}</p>` : ""}
      <div class="action-row">${rollbackButton}</div>
    `;

    els.releaseList.appendChild(card);
  });

  els.releaseList.querySelectorAll(".rollback-button").forEach((button) => {
    button.addEventListener("click", async () => {
      const releaseId = button.getAttribute("data-release-id");
      if (!releaseId || !state.selectedProjectId) {
        return;
      }
      try {
        button.disabled = true;
        await api(`/projects/${state.selectedProjectId}/releases/${releaseId}/rollback`, {
          method: "POST",
        });
        showToast("Откат запущен.");
        await loadReleases();
      } catch (error) {
        showToast(error.message);
      } finally {
        button.disabled = false;
      }
    });
  });
}

function renderProjectLogs() {
  if (!els.logsViewer || !els.logsSummary || !els.logsEmpty) {
    return;
  }

  if (els.logsSearchInput) els.logsSearchInput.value = state.logsSearch || "";
  if (els.logsLevelSelect) els.logsLevelSelect.value = state.logsLevel || "";
  if (els.logsSinceSelect) els.logsSinceSelect.value = state.logsSince || "15m";
  if (els.logsPodInput) els.logsPodInput.value = state.logsPod || "";
  if (els.logsContainerInput) els.logsContainerInput.value = state.logsContainer || "";
  if (els.logsOpenGrafanaButton) {
    els.logsOpenGrafanaButton.disabled = !String(state.selectedProject?.grafanaUrl || "").trim();
  }
  if (els.logsPanel) {
    els.logsPanel.classList.toggle("logs-panel-fullscreen", Boolean(state.logsFullscreen));
  }
  if (els.logsFullscreenButton) {
    els.logsFullscreenButton.textContent = state.logsFullscreen ? "Свернуть" : "Развернуть";
  }
  renderLogSuggestions();

  const response = state.projectLogs;
  if (!response) {
    els.logsSummary.textContent = "Логи еще не загружены.";
    els.logsViewer.innerHTML = "";
    els.logsEmpty.classList.remove("hidden");
    return;
  }

  const entries = Array.isArray(response.entries) ? response.entries : [];
  const stageLabel = response.stageSlug || "весь проект";
  els.logsSummary.textContent = `Найдено строк: ${entries.length}. Источник: ${stageLabel}.`;
  els.logsEmpty.classList.toggle("hidden", entries.length > 0);
  if (entries.length === 0) {
    els.logsViewer.innerHTML = "";
    return;
  }

  els.logsViewer.innerHTML = entries
    .map((entry) => {
      const level = String(entry.level || "").toLowerCase();
      const source = [
        entry.stage || response.stageSlug || "project",
        entry.pod || "pod",
        entry.container || "container",
      ]
        .filter(Boolean)
        .join(" / ");
      return `
        <div class="log-line ${escapeAttr(level)}">
          <span class="log-line-meta">${escapeHtml(formatDate(entry.timestamp))}</span>
          <span class="log-line-source">${escapeHtml(source)}</span>
          <span class="log-line-message">${escapeHtml(entry.message || "")}</span>
        </div>
      `;
    })
    .join("");
}

function uniqueSorted(values) {
  return Array.from(new Set((Array.isArray(values) ? values : []).filter(Boolean))).sort((a, b) =>
    String(a).localeCompare(String(b), "ru", { sensitivity: "base" }),
  );
}

function normalizeLogPodLabel(value) {
  const trimmed = String(value || "").trim();
  const parts = trimmed.split("-x-");
  if (parts.length >= 3 && parts[0]) {
    return parts[0];
  }
  return trimmed;
}

function renderLogFilterOptions(element, values, emptyLabel, formatLabel = (value) => value) {
  if (!element) {
    return;
  }
  const currentValue = String(element.value || "").trim();
  const seenLabels = new Set();
  const options = [];

  uniqueSorted([...values, currentValue]).forEach((value) => {
    const rawValue = String(value || "").trim();
    if (!rawValue) {
      return;
    }
    const label = String(formatLabel(rawValue) || rawValue).trim();
    if (!label || seenLabels.has(label)) {
      return;
    }
    seenLabels.add(label);
    options.push({ rawValue, label });
  });

  element.innerHTML = [
    `<option value="">${escapeHtml(emptyLabel)}</option>`,
    ...options.map(
      ({ rawValue, label }) =>
        `<option value="${escapeAttr(rawValue)}">${escapeHtml(label)}</option>`,
    ),
  ].join("");
  element.value = currentValue;
}

function updateLogCatalogs(response) {
  const entries = Array.isArray(response?.entries) ? response.entries : [];
  const runtimePods = Array.isArray(state.runtimeStatus?.pods) ? state.runtimeStatus.pods : [];
  const responsePods = entries.map((entry) => String(entry?.pod || "").trim());
  const responseContainers = entries.map((entry) => String(entry?.container || "").trim());
  const runtimePodNames = runtimePods.map((pod) => String(pod?.name || "").trim());
  const runtimeContainers = runtimePods.flatMap((pod) =>
    Array.isArray(pod?.containers) ? pod.containers.map((name) => String(name || "").trim()) : [],
  );

  state.logPodsCatalog = uniqueSorted([
    ...state.logPodsCatalog,
    ...responsePods,
    ...runtimePodNames,
  ]);
  state.logContainersCatalog = uniqueSorted([
    ...state.logContainersCatalog,
    ...responseContainers,
    ...runtimeContainers,
  ]);
}

function renderLogSuggestions() {
  renderLogFilterOptions(els.logsPodInput, state.logPodsCatalog, "Все pod'ы", normalizeLogPodLabel);
  renderLogFilterOptions(els.logsContainerInput, state.logContainersCatalog, "Все container'ы");
}

function syncLogFiltersFromInputs() {
  state.logsSearch = (els.logsSearchInput?.value || "").trim();
  state.logsLevel = (els.logsLevelSelect?.value || "").trim();
  state.logsSince = (els.logsSinceSelect?.value || "15m").trim() || "15m";
  state.logsPod = (els.logsPodInput?.value || "").trim();
  state.logsContainer = (els.logsContainerInput?.value || "").trim();
}

function setLogsFullscreen(enabled) {
  state.logsFullscreen = Boolean(enabled);
  document.body.classList.toggle("logs-fullscreen-open", state.logsFullscreen);
  renderProjectLogs();
  if (state.logsFullscreen) {
    els.logsViewer?.scrollIntoView({ block: "nearest" });
  }
}

function renderStageSelectors() {
  const stages = (state.stages || []).filter((stage) => stage.status !== "deleted");
  normalizeSelectedStage();
  const selected = selectedStage();

  if (els.deployStageSlug) {
    if (stages.length === 0) {
      els.deployStageSlug.innerHTML = `<option value="production">production</option>`;
    } else {
      els.deployStageSlug.innerHTML = stages
        .map((stage) => {
          const isSelected = selected && stage.id === selected.id;
          const label = stageDisplayName(stage);
          return `<option value="${escapeAttr(stage.slug)}" ${isSelected ? "selected" : ""}>${escapeHtml(label)}</option>`;
        })
        .join("");
    }
  }

  if (els.releasesStageFilter) {
    if (stages.length === 0) {
      els.releasesStageFilter.innerHTML = `<option value="">production</option>`;
      els.releasesStageFilter.value = "";
    } else {
      els.releasesStageFilter.innerHTML = stages
        .map((stage) => {
          const isSelected = selected && stage.id === selected.id;
          const label = stageDisplayName(stage);
          return `<option value="${escapeAttr(stage.id)}" ${isSelected ? "selected" : ""}>${escapeHtml(label)}</option>`;
        })
        .join("");
      els.releasesStageFilter.value = selected ? selected.id : "";
    }
  }

  if (els.runtimeStageSelect) {
    if (stages.length === 0) {
      els.runtimeStageSelect.innerHTML = `<option value="">production</option>`;
      els.runtimeStageSelect.value = "";
    } else {
      els.runtimeStageSelect.innerHTML = stages
        .map((stage) => {
          const isSelected = selected && stage.id === selected.id;
          const label = stageDisplayName(stage);
          return `<option value="${escapeAttr(stage.id)}" ${isSelected ? "selected" : ""}>${escapeHtml(label)}</option>`;
        })
        .join("");
      els.runtimeStageSelect.value = selected ? selected.id : "";
    }
  }

  if (els.logsStageSelect) {
    if (stages.length === 0) {
      els.logsStageSelect.innerHTML = `<option value="">production</option>`;
      els.logsStageSelect.value = "";
    } else {
      els.logsStageSelect.innerHTML = stages
        .map((stage) => {
          const isSelected = selected && stage.id === selected.id;
          const label = stageDisplayName(stage);
          return `<option value="${escapeAttr(stage.id)}" ${isSelected ? "selected" : ""}>${escapeHtml(label)}</option>`;
        })
        .join("");
      els.logsStageSelect.value = selected ? selected.id : "";
    }
  }

  renderStageIndicator();
}

function renderStageIndicator() {
  if (!els.stageIndicatorChip) return;
  const stage = selectedStage();
  if (!stage) {
    els.stageIndicatorChip.classList.add("hidden");
    return;
  }
  els.stageIndicatorChip.textContent = `Выбранный контур: ${stageDisplayName(stage)}`;
  els.stageIndicatorChip.classList.remove("hidden");
}

function renderStages() {
  if (!els.stageList || !els.stageListEmpty) {
    return;
  }

  const stages = (state.stages || []).filter((stage) => stage.status !== "deleted");
  normalizeSelectedStage();
  const selected = selectedStage();

  els.stageList.innerHTML = "";
  els.stageListEmpty.classList.toggle("hidden", stages.length > 0);

  stages.forEach((stage) => {
    const card = document.createElement("article");
    card.className = `stage-card ${selected && selected.id === stage.id ? "active" : ""}`;
    card.innerHTML = `
      <div class="panel-head compact">
        <div>
          <h4>${escapeHtml(stage.name || stage.slug)}</h4>
          <div class="project-card-meta">
            <span>slug: ${escapeHtml(stage.slug || "—")}</span>
            <span>id: ${escapeHtml(stage.id || "—")}</span>
          </div>
        </div>
        <span class="status-pill ${statusClass(stage.status)}">${escapeHtml(stage.status || "unknown")}</span>
      </div>
      <div class="action-row">
        <button class="ghost-button stage-select-button" data-stage-id="${escapeAttr(stage.id)}" type="button">
          ${selected && selected.id === stage.id ? "Выбран" : "Выбрать"}
        </button>
        ${
          stage.slug === "production"
            ? `<button class="ghost-button" type="button" disabled title="production удалить нельзя">Удалить</button>`
            : `<button class="danger-button stage-delete-button" data-stage-id="${escapeAttr(stage.id)}" data-stage-name="${escapeAttr(stage.name || stage.slug)}" type="button">Удалить</button>`
        }
      </div>
    `;
    els.stageList.appendChild(card);
  });

  els.stageList.querySelectorAll(".stage-select-button").forEach((button) => {
    button.addEventListener("click", async () => {
      const stageID = button.getAttribute("data-stage-id");
      try {
        await switchSelectedStage(stageID || "");
      } catch (error) {
        showToast(error.message);
      }
    });
  });

  els.stageList.querySelectorAll(".stage-delete-button").forEach((button) => {
    button.addEventListener("click", async () => {
      const projectID = getSelectedProjectOrWarn();
      if (!projectID) {
        return;
      }
      const stageID = button.getAttribute("data-stage-id");
      const stageName = button.getAttribute("data-stage-name") || "stage";
      if (!stageID) {
        return;
      }

      const confirmed = await confirmAction({
        title: "Удалить контур?",
        description: `Контур ${stageName} будет удален из проекта вместе с namespace внутри vcluster.`,
        confirmLabel: "Удалить контур",
        tone: "danger",
      });
      if (!confirmed) {
        return;
      }

      try {
        button.disabled = true;
        await api(`/projects/${projectID}/stages/${stageID}`, { method: "DELETE" });
        showToast(`Контур ${stageName} удален.`);
        if (state.selectedStageId === stageID) {
          state.selectedStageId = "";
        }
        await Promise.allSettled([loadStages(), loadRuntimeStatus(), loadReleases()]);
      } catch (error) {
        showToast(error.message);
      } finally {
        button.disabled = false;
      }
    });
  });
}

function setFieldValue(id, value) {
  const field = document.getElementById(id);
  if (field) {
    if (field.type === "checkbox") {
      field.checked = Boolean(value);
      return;
    }
    field.value = value;
  }
}

function dedicatedLoadBalancerEnabled() {
  return Boolean(document.getElementById("dedicatedLoadBalancer")?.checked);
}

function syncDedicatedLoadBalancerField() {
  const field = document.getElementById("dedicatedLoadBalancer");
  const serviceType = document.getElementById("serviceType")?.value || "LoadBalancer";
  if (!field) {
    return;
  }
  const enabled = serviceType === "LoadBalancer";
  field.disabled = !enabled;
  if (!enabled) {
    field.checked = false;
  }
}

function buildRepositoryURL(repositoryOwner, repositoryName) {
  const owner = String(repositoryOwner || "").trim();
  const name = String(repositoryName || "").trim();
  if (!owner || !name) {
    return "";
  }
  return `https://github.com/${owner}/${name}`;
}

function parseRepositoryReference(input) {
  const raw = String(input || "").trim();
  if (!raw) {
    return null;
  }

  let normalized = raw;
  if (normalized.startsWith("git@github.com:")) {
    normalized = `https://github.com/${normalized.slice("git@github.com:".length)}`;
  } else if (normalized.startsWith("ssh://git@github.com/")) {
    normalized = `https://github.com/${normalized.slice("ssh://git@github.com/".length)}`;
  } else if (normalized.startsWith("github.com/")) {
    normalized = `https://${normalized}`;
  } else if (/^[^/\s]+\/[^/\s]+$/.test(normalized) && !normalized.includes("://")) {
    normalized = `https://github.com/${normalized}`;
  }

  let parsedURL;
  try {
    parsedURL = new URL(normalized);
  } catch (error) {
    return null;
  }

  if (!["github.com", "www.github.com"].includes(parsedURL.hostname.toLowerCase())) {
    return null;
  }

  const parts = parsedURL.pathname
    .replace(/^\/+/, "")
    .replace(/\/+$/, "")
    .split("/")
    .filter(Boolean);
  if (parts.length < 2) {
    return null;
  }

  const repositoryOwner = parts[0];
  const repositoryName = parts[1].replace(/\.git$/i, "");
  if (!repositoryOwner || !repositoryName) {
    return null;
  }

  return { repositoryOwner, repositoryName };
}

function resolveRepositoryFields() {
  const raw = String(els.repositoryUrl?.value || "").trim();
  const parsed = parseRepositoryReference(raw);
  if (!parsed) {
    if (raw !== "") {
      return { repositoryOwner: "", repositoryName: "" };
    }
    const fallbackOwner = String(state.selectedProject?.repositoryOwner || "").trim();
    const fallbackName = String(state.selectedProject?.repositoryName || "").trim();
    if (!fallbackOwner || !fallbackName) {
      return { repositoryOwner: "", repositoryName: "" };
    }
    return { repositoryOwner: fallbackOwner, repositoryName: fallbackName };
  }
  return {
    repositoryOwner: parsed.repositoryOwner,
    repositoryName: parsed.repositoryName,
  };
}

function applyProjectDeploymentSettings(project) {
  state.applyingProjectSettings = true;
  setFieldValue(
    "repositoryUrl",
    buildRepositoryURL(project?.repositoryOwner || "", project?.repositoryName || ""),
  );
  setFieldValue("baseBranch", project?.baseBranch ?? "");
  setFieldValue("serviceName", project?.serviceName ?? "");
  setFieldValue("dockerfilePath", project?.dockerfilePath ?? "");
  setFieldValue("serviceType", project?.serviceType ?? "LoadBalancer");
  setFieldValue("dedicatedLoadBalancer", project?.dedicatedLoadBalancer ?? false);
  setFieldValue("servicePort", project?.servicePort ?? 80);
  setFieldValue("containerPort", project?.containerPort ?? 8080);
  setFieldValue("replicaCount", String(project?.replicaCount ?? 1));
  setFieldValue("resourceProfile", project?.resourceProfile ?? "balanced");
  syncDedicatedLoadBalancerField();
  state.applyingProjectSettings = false;
  state.deploymentSettingsDirty = false;
  renderDeployPlanSummary();
}

function applyPrimarySetupToDeployForm(project = state.selectedProject) {
  const source = project || {};
  const repositoryURL = buildRepositoryURL(
    source.repositoryOwner || "",
    source.repositoryName || "",
  );
  const baseBranch = String(source.baseBranch || "").trim();

  let applied = false;
  if (repositoryURL) {
    setFieldValue("repositoryUrl", repositoryURL);
    applied = true;
  }
  if (baseBranch) {
    setFieldValue("baseBranch", baseBranch);
    applied = true;
  }
  return applied;
}

function buildDeploymentSettingsPayloadFromProject(project, overrides = {}) {
  const source = project || {};
  return {
    repositoryOwner: String(overrides.repositoryOwner ?? source.repositoryOwner ?? "").trim(),
    repositoryName: String(overrides.repositoryName ?? source.repositoryName ?? "").trim(),
    baseBranch: String(overrides.baseBranch ?? source.baseBranch ?? "").trim(),
    serviceName: String(overrides.serviceName ?? source.serviceName ?? "").trim(),
    dockerfilePath: String(overrides.dockerfilePath ?? source.dockerfilePath ?? "").trim(),
    serviceType: String(overrides.serviceType ?? source.serviceType ?? "LoadBalancer"),
    dedicatedLoadBalancer: Boolean(
      overrides.dedicatedLoadBalancer ?? source.dedicatedLoadBalancer ?? false,
    ),
    servicePort: Number(overrides.servicePort ?? source.servicePort ?? 0),
    containerPort: Number(overrides.containerPort ?? source.containerPort ?? 0),
    replicaCount: Number(overrides.replicaCount ?? source.replicaCount ?? 1),
    resourceProfile: String(overrides.resourceProfile ?? source.resourceProfile ?? "balanced"),
  };
}

function deploymentSettingsPayload() {
  const repository = resolveRepositoryFields();
  return buildDeploymentSettingsPayloadFromProject(state.selectedProject, {
    repositoryOwner: repository.repositoryOwner,
    repositoryName: repository.repositoryName,
    baseBranch: document.getElementById("baseBranch").value.trim(),
    serviceName: document.getElementById("serviceName").value.trim(),
    dockerfilePath: document.getElementById("dockerfilePath").value.trim(),
    serviceType: document.getElementById("serviceType").value,
    dedicatedLoadBalancer: dedicatedLoadBalancerEnabled(),
    servicePort: Number(document.getElementById("servicePort").value || 0),
    containerPort: Number(document.getElementById("containerPort").value || 0),
    replicaCount: Number(document.getElementById("replicaCount").value || 1),
    resourceProfile: document.getElementById("resourceProfile").value,
  });
}

async function persistDeploymentSettings() {
  if (!state.selectedProjectId || state.applyingProjectSettings || !state.deploymentSettingsDirty) {
    return;
  }

  state.deploymentSettingsDirty = false;
  try {
    const project = await api(`/projects/${state.selectedProjectId}/deployment-settings`, {
      method: "PUT",
      body: JSON.stringify(deploymentSettingsPayload()),
    });
    state.selectedProject = project;
    syncProjectInList(project);
    renderProjectWorkspace();
    renderProjects();
    showToast("Настройки сохранены");
  } catch (err) {
    showToast("Не удалось сохранить настройки");
    state.deploymentSettingsDirty = true;
  }
}

function setGitHubBusy(isBusy, message = "") {
  els.detectGitHubButton.disabled = isBusy;
  els.createPrButton.disabled = isBusy;
  els.createPrButton.textContent = isBusy ? "Создаем PR..." : "Создать PR";
  if (message) {
    els.githubMessage.textContent = message;
  } else if (!isBusy) {
    els.githubMessage.textContent = "";
  }
}

function getSelectedProjectOrWarn() {
  if (!state.selectedProjectId) {
    showToast("Сначала выбери проект.");
    return null;
  }
  return state.selectedProjectId;
}

function clearSelectedProjectState() {
  state.selectedProjectId = "";
  state.selectedProject = null;
  state.projectGitHubTokenConfigured = false;
  state.cost = null;
  state.stages = [];
  state.selectedStageId = "";
  state.runtimeStatus = null;
  state.releases = [];
  state.projectLogs = null;
  state.logPodsCatalog = [];
  state.logContainersCatalog = [];
  state.logsFullscreen = false;
  state.workspaceDataUpdatedAt = 0;
  runtimeStatusInFlight = null;
  projectLogsInFlight = null;
  workspaceDataInFlight = null;
}

async function handleBulkDeleteSelected() {
  const projectIDs = Array.from(state.bulkDeleteSelection);
  if (projectIDs.length === 0 || state.bulkDeleteInProgress) {
    return;
  }

  const confirmed = await confirmAction({
    title: "Удалить выбранные проекты?",
    description: `Будут удалены ${projectIDs.length} проект(а/ов). Операцию нельзя отменить.`,
    confirmLabel: "Удалить выбранные",
    tone: "danger",
  });
  if (!confirmed) {
    return;
  }

  state.bulkDeleteInProgress = true;
  state.bulkDeleteReport = null;
  renderBulkDeletePage();

  const failures = [];
  let successCount = 0;

  for (const projectID of projectIDs) {
    try {
      await api(`/projects/${projectID}`, { method: "DELETE" });
      successCount += 1;
      state.bulkDeleteSelection.delete(projectID);
      state.projects = state.projects.filter((project) => project.id !== projectID);
      if (state.selectedProjectId === projectID) {
        clearSelectedProjectState();
        renderWorkspaceHeader();
      }
      renderProjects();
      renderDashboard();
      renderProjectWorkspace();
      renderBulkDeletePage();
    } catch (error) {
      failures.push({ id: projectID, error: error.message });
    }
  }

  state.bulkDeleteInProgress = false;
  state.bulkDeleteReport = {
    requestedCount: projectIDs.length,
    successCount,
    failures,
  };

  try {
    await loadProjects();
  } catch (error) {
    showToast(`Удаление завершено, но обновить список проектов не удалось: ${error.message}`);
  }

  renderProjects();
  renderDashboard();
  renderProjectWorkspace();
  renderWorkspaceHeader();
  renderBulkDeletePage();
  showToast(`Массовое удаление завершено: ${successCount} из ${projectIDs.length}.`);
}

function parseHash() {
  const hash = window.location.hash.replace(/^#/, "");
  if (!hash || hash === "/") {
    return { page: "dashboard" };
  }
  const parts = hash.replace(/^\//, "").split("/");
  if (parts[0] === "project" && parts[1]) {
    return {
      page: "project",
      projectId: parts[1],
      tab: parts[2] || "overview",
    };
  }
  if (parts[0] === "bulk-delete") {
    return { page: "projects", projectsEditMode: true };
  }
  if (["dashboard", "projects", "billing", "flow", "about", "api"].includes(parts[0])) {
    return { page: parts[0] };
  }
  return { page: "dashboard" };
}

function setHash(hash) {
  if (window.location.hash === hash) {
    void handleRouteChange();
    return;
  }
  window.location.hash = hash;
}

function navigateToPage(page) {
  setHash(`/${page}`);
}

function navigateToProject(projectId, tab = "overview") {
  setHash(`/project/${projectId}/${tab}`);
}

function projectById(projectId) {
  if (!projectId) {
    return null;
  }
  if (state.selectedProject && state.selectedProject.id === projectId) {
    return state.selectedProject;
  }
  return state.projects.find((project) => project.id === projectId) || null;
}

function isProjectCreating(project) {
  return Boolean(project && project.status === "creating");
}

function notifyProjectIsCreating(project) {
  const projectName = String(project?.name || "").trim();
  const subject = projectName ? `Проект «${projectName}»` : "Проект";
  showToast(`${subject} станет доступен сразу после завершения создания.`);
}

async function openProjectWhenReady(projectId, tab = "overview") {
  if (!projectId) {
    return false;
  }

  const project = projectById(projectId) || (await api(`/projects/${projectId}`));
  syncProjectInList(project);
  renderProjects();
  renderDashboard();

  if (isProjectCreating(project)) {
    startCreatingPoller(project.id);
    notifyProjectIsCreating(project);
    return false;
  }

  navigateToProject(project.id, tab);
  return true;
}

function syncProjectInList(project) {
  const existing = state.projects.some((item) => item.id === project.id);
  state.projects = existing
    ? state.projects.map((item) => (item.id === project.id ? project : item))
    : [project, ...state.projects];
}

// Опрос для проекта в статусе creating
let _creatingPoller = null;

function startCreatingPoller(projectId) {
  stopCreatingPoller();
  _creatingPoller = setInterval(async () => {
    try {
      const project = await api(`/projects/${projectId}`);
      const prev = state.projects.find((p) => p.id === projectId);
      const prevStatus = prev?.status;
      syncProjectInList(project);
      if (state.selectedProjectId === projectId) {
        state.selectedProject = project;
        renderProjects();
        renderDashboard();
        renderProjectWorkspace();
      } else {
        renderProjects();
        renderDashboard();
      }
      if (project.status !== "creating") {
        stopCreatingPoller();
        if (prevStatus === "creating" && project.status === "active") {
          showToast("Проект готов к работе!", "success");
          if (state.selectedProjectId === projectId) {
            await loadProjectWorkspaceData();
          }
        } else if (project.status === "failed") {
          showToast("Не удалось создать проект. Попробуйте еще раз.", "error");
        }
      }
    } catch (_) {
      // Игнорируем временные ошибки во время опроса
    }
  }, 3000);
}

function stopCreatingPoller() {
  if (_creatingPoller !== null) {
    clearInterval(_creatingPoller);
    _creatingPoller = null;
  }
}
// Конец блока опроса для проекта в статусе creating

async function loadProjects() {
  const projects = await api("/projects");
  state.projects = (Array.isArray(projects) ? projects : []).sort((left, right) => {
    return new Date(right.updatedAt || 0).getTime() - new Date(left.updatedAt || 0).getTime();
  });
  normalizeBulkDeleteSelection();
  renderProjects();
  renderDashboard();
  renderBulkDeletePage();
}

async function loadBilling() {
  state.billing = await api("/billing/summary");
  if (state.selectedProjectId) {
    try {
      state.selectedProject = await api(`/projects/${state.selectedProjectId}`);
      syncProjectInList(state.selectedProject);
    } catch (error) {
      console.warn("failed to sync selected project after billing refresh", error);
    }
  }
  renderBilling();
  renderProjects();
  renderDashboard();
  renderProjectWorkspace();
}

async function loadProjectGitHubTokenStatus() {
  const projectId = state.selectedProjectId;
  if (!projectId) {
    return;
  }
  const status = await api(`/projects/${projectId}/github-token`);
  state.projectGitHubTokenConfigured = Boolean(status && status.configured);
  renderProjectGitHubTokenSection();
}

async function saveProjectGitHubTokenForProject(projectId, token) {
  if (!projectId) {
    return;
  }
  const status = await api(`/projects/${projectId}/github-token`, {
    method: "PUT",
    body: JSON.stringify({ token }),
  });
  state.projectGitHubTokenConfigured = Boolean(status && status.configured);
  renderProjectGitHubTokenSection();
}

async function saveProjectGitHubToken(token) {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) {
    return;
  }
  await saveProjectGitHubTokenForProject(projectId, token);
}
async function deleteProjectGitHubToken() {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) {
    return;
  }
  const status = await api(`/projects/${projectId}/github-token`, { method: "DELETE" });
  state.projectGitHubTokenConfigured = Boolean(status && status.configured);
  renderProjectGitHubTokenSection();
}

function renderProjectGitHubTokenSection() {
  const hasProject = Boolean(state.selectedProjectId);
  if (els.projectGitHubTokenStatus) {
    if (!hasProject) {
      els.projectGitHubTokenStatus.textContent = "Выбери проект, чтобы настроить GitHub токен.";
    } else {
      els.projectGitHubTokenStatus.textContent = state.projectGitHubTokenConfigured
        ? "Токен сохранен. Можно использовать автодетект и создание PR без ввода разового токена."
        : "Токен не задан. Добавь его кнопкой выше или используй разовый токен в форме деплоя.";
    }
  }
  if (els.openProjectGitHubTokenModalButton) {
    els.openProjectGitHubTokenModalButton.disabled = !hasProject;
    els.openProjectGitHubTokenModalButton.textContent = state.projectGitHubTokenConfigured
      ? "Изменить токен"
      : "Добавить токен";
  }
  if (els.deleteProjectGitHubTokenButton) {
    els.deleteProjectGitHubTokenButton.classList.toggle(
      "hidden",
      !state.projectGitHubTokenConfigured,
    );
  }
}

async function loadProject(projectId, preloadedProject = null) {
  if (state.selectedProjectId && state.selectedProjectId !== projectId) {
    state.stages = [];
    state.selectedStageId = "";
    state.projectLogs = null;
    state.logsPod = "";
    state.logsContainer = "";
    state.logPodsCatalog = [];
    state.logContainersCatalog = [];
    state.workspaceDataUpdatedAt = 0;
    projectLogsInFlight = null;
    workspaceDataInFlight = null;
    stopCreatingPoller();
  }
  state.selectedProject = preloadedProject || (await api(`/projects/${projectId}`));
  state.selectedProjectId = projectId;
  syncProjectInList(state.selectedProject);
  renderProjects();
  renderDashboard();
  renderProjectWorkspace();
  if (state.selectedProject?.status === "creating" && _creatingPoller === null) {
    startCreatingPoller(projectId);
  }
}

async function loadCost() {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) {
    return;
  }
  state.cost = await api(`/projects/${projectId}/cost`);
  renderCost();
  renderDashboard();
}

async function loadStages() {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) {
    return;
  }
  const stages = await api(`/projects/${projectId}/stages`);
  state.stages = Array.isArray(stages) ? stages : [];
  normalizeSelectedStage();
  renderStageSelectors();
  renderStages();
}

async function loadReleases() {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) {
    return;
  }
  const stage = selectedStage();
  let releases = [];
  if (stage) {
    const scoped = await api(
      `/projects/${projectId}/releases?stageId=${encodeURIComponent(stage.id)}`,
    );
    releases = Array.isArray(scoped) ? scoped : [];
    if (releases.length === 0) {
      const all = await api(`/projects/${projectId}/releases`);
      const allReleases = Array.isArray(all) ? all : [];
      const matched = allReleases.filter((release) => {
        const releaseStageID = String(release.stageId || "").trim();
        return releaseStageID === stage.id;
      });
      if (matched.length > 0) {
        releases = matched;
      } else if (stage.slug === "production") {
        releases = allReleases.filter((release) => {
          const releaseStageID = String(release.stageId || "").trim();
          return releaseStageID === "" || releaseStageID === stage.id;
        });
      } else {
        // Резервный сценарий по возможности для релизов, созданных без привязки к stage.
        releases = allReleases;
      }
    }
  } else {
    const all = await api(`/projects/${projectId}/releases`);
    releases = Array.isArray(all) ? all : [];
  }

  state.releases = releases;
  renderReleases(state.releases);
  renderDashboard();
}

async function loadProjectURLs() {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) return;
  if (els.urlsContent) {
    els.urlsContent.innerHTML = '<p class="muted">Загрузка...</p>';
  }
  try {
    const result = await api(`/projects/${projectId}/urls`);
    renderProjectURLs(result);
  } catch (err) {
    if (els.urlsContent) {
      els.urlsContent.innerHTML = `<p class="muted">Не удалось загрузить адреса: ${escapeHtml(err.message)}</p>`;
    }
  }
}

function renderOverviewServiceURLs(fallbackURL) {
  const serviceUrls = state.runtimeStatus?.serviceUrls;
  if (!els.overviewServiceUrls) return;
  if (serviceUrls && serviceUrls.length > 0) {
    let html = "";
    for (const svc of serviceUrls) {
      html += `<div class="summary-row">`;
      html += `<span>${escapeHtml(svc.name)}</span>`;
      html += `<div>${renderLinkedValue(svc.url, "—", { compact: true })}</div>`;
      html += `</div>`;
    }
    els.overviewServiceUrls.innerHTML = html;
  } else {
    els.overviewServiceUrls.innerHTML = `<div class="summary-row"><span>Сервисы</span><div id="projectUrlValue">${renderLinkedValue(fallbackURL, "—", { compact: true })}</div></div>`;
  }
}

function renderProjectURLs(result) {
  if (!els.urlsContent) return;
  const stages = result && Array.isArray(result.stages) ? result.stages : [];
  if (stages.length === 0) {
    els.urlsContent.innerHTML =
      '<p class="muted">Нет данных. Возможно, ни один сервис ещё не задеплоен.</p>';
    return;
  }
  let html = "";
  for (const stage of stages) {
    const services = Array.isArray(stage.services) ? stage.services : [];
    html += `<div class="urls-stage-block">`;
    html += `<h4 class="urls-stage-title">${escapeHtml(stage.stageName || stage.slug)}</h4>`;
    if (services.length === 0) {
      html += `<p class="muted urls-empty">Нет публичных адресов для этого контура.</p>`;
    } else {
      html += `<div class="urls-service-list">`;
      for (const svc of services) {
        html += `<div class="urls-service-row">`;
        html += `<span class="urls-service-name">${escapeHtml(svc.name)}</span>`;
        html += `<a href="${escapeHtml(svc.url)}" target="_blank" rel="noopener noreferrer" class="urls-service-link">${escapeHtml(svc.url)}</a>`;
        html += `</div>`;
      }
      html += `</div>`;
    }
    html += `</div>`;
  }
  els.urlsContent.innerHTML = html;
}

async function loadRuntimeStatus() {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) {
    return;
  }
  const stage = selectedStage();
  const stageId = stage ? stage.id : "";
  const path = stage
    ? `/projects/${projectId}/stages/${stage.id}/runtime-status`
    : `/projects/${projectId}/runtime-status`;
  const requestKey = `${projectId}:${stageId || "project"}`;

  if (runtimeStatusInFlight && runtimeStatusInFlight.key === requestKey) {
    return runtimeStatusInFlight.promise;
  }

  const requestPromise = (async () => {
    const runtimeStatus = await api(path);
    if (state.selectedProjectId !== projectId) {
      return runtimeStatus;
    }
    const currentStage = selectedStage();
    const currentStageID = currentStage ? currentStage.id : "";
    if (currentStageID !== stageId) {
      return runtimeStatus;
    }
    state.runtimeStatus = runtimeStatus;
    updateLogCatalogs(null);
    renderStageSelectors();
    renderRuntimeCards();
    renderDashboard();
    renderProjectWorkspace();
    renderProjectLogs();
    return runtimeStatus;
  })();

  runtimeStatusInFlight = { key: requestKey, promise: requestPromise };
  try {
    return await requestPromise;
  } finally {
    if (runtimeStatusInFlight && runtimeStatusInFlight.promise === requestPromise) {
      runtimeStatusInFlight = null;
    }
  }
}

async function loadProjectLogs(options = {}) {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) {
    return;
  }

  if (options.readInputs !== false) {
    syncLogFiltersFromInputs();
  }

  const stage = selectedStage();
  const stageId = stage ? stage.id : "";
  const requestKey = [
    projectId,
    stageId || "project",
    state.logsSearch,
    state.logsLevel,
    state.logsSince,
    state.logsPod,
    state.logsContainer,
  ].join(":");
  if (projectLogsInFlight && projectLogsInFlight.key === requestKey) {
    return projectLogsInFlight.promise;
  }

  const params = new URLSearchParams();
  if (stageId) params.set("stageId", stageId);
  if (state.logsSearch) params.set("search", state.logsSearch);
  if (state.logsLevel) params.set("level", state.logsLevel);
  if (state.logsSince) params.set("since", state.logsSince);
  if (state.logsPod) params.set("pod", state.logsPod);
  if (state.logsContainer) params.set("container", state.logsContainer);
  params.set("limit", "200");

  const requestPromise = (async () => {
    const response = await api(`/projects/${projectId}/logs?${params.toString()}`);
    if (state.selectedProjectId !== projectId) {
      return response;
    }
    const currentStage = selectedStage();
    const currentStageID = currentStage ? currentStage.id : "";
    if (currentStageID !== stageId) {
      return response;
    }
    state.projectLogs = response;
    updateLogCatalogs(response);
    renderProjectLogs();
    return response;
  })();

  projectLogsInFlight = { key: requestKey, promise: requestPromise };
  try {
    return await requestPromise;
  } finally {
    if (projectLogsInFlight && projectLogsInFlight.promise === requestPromise) {
      projectLogsInFlight = null;
    }
  }
}

async function loadProjectWorkspaceData() {
  if (!state.selectedProjectId) {
    return;
  }
  await loadStages();
  const tasks = [loadCost(), loadReleases(), loadRuntimeStatus(), loadProjectGitHubTokenStatus()];
  if (state.currentProjectTab === "logs") {
    tasks.push(loadProjectLogs({ readInputs: false }));
  }
  await Promise.allSettled(tasks);
  state.workspaceDataUpdatedAt = Date.now();
}

function isWorkspaceDataFresh(projectId = state.selectedProjectId) {
  if (!projectId || state.selectedProjectId !== projectId) {
    return false;
  }
  return Date.now() - Number(state.workspaceDataUpdatedAt || 0) < workspaceDataTTL;
}

function ensureWorkspaceDataWarm(options = {}) {
  const { force = false, projectId = state.selectedProjectId } = options;
  if (!projectId || state.selectedProjectId !== projectId) {
    return Promise.resolve();
  }
  if (!force && isWorkspaceDataFresh(projectId)) {
    return Promise.resolve();
  }
  if (
    workspaceDataInFlight &&
    workspaceDataInFlight.projectId === projectId &&
    (force ? workspaceDataInFlight.force : true)
  ) {
    return workspaceDataInFlight.promise;
  }

  const requestPromise = loadProjectWorkspaceData().finally(() => {
    if (workspaceDataInFlight && workspaceDataInFlight.promise === requestPromise) {
      workspaceDataInFlight = null;
    }
  });
  workspaceDataInFlight = { projectId, promise: requestPromise, force };
  return requestPromise;
}

function prefetchProjectTabData(tab, options = {}) {
  const projectId = options.projectId || state.selectedProjectId;
  if (!projectId || state.selectedProjectId !== projectId) {
    return;
  }
  const delay = options.delay ?? tabPrefetchDelays[tab] ?? 0;
  window.setTimeout(() => {
    if (!state.selectedProjectId || state.selectedProjectId !== projectId) {
      return;
    }
    if (tab === "logs") {
      void loadProjectLogs({ readInputs: false }).catch((error) => {
        console.warn("failed to prefetch project logs", error);
      });
      return;
    }
    if (["runtime", "releases", "access", "overview", "stages", "deploy"].includes(tab)) {
      void ensureWorkspaceDataWarm({ projectId }).catch((error) => {
        console.warn("failed to warm project workspace", error);
      });
    }
  }, delay);
}

async function downloadKubeconfig(rotate) {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) {
    return;
  }
  const path = rotate
    ? `/projects/${projectId}/kubeconfig/rotate`
    : `/projects/${projectId}/kubeconfig`;
  const response = await fetch(path, {
    method: rotate ? "POST" : "GET",
    headers: {
      Authorization: `Bearer ${state.token}`,
    },
  });
  const body = await response.text();
  if (!response.ok) {
    let message = body || `${response.status} ${response.statusText}`;
    const contentType = response.headers.get("content-type") || "";
    if (contentType.includes("application/json")) {
      try {
        const parsed = JSON.parse(body);
        if (parsed && typeof parsed.error === "string" && parsed.error.trim()) {
          message = parsed.error;
        }
      } catch (error) {
        // Откатываемся к исходному телу ответа
      }
    }
    throw new Error(message);
  }
  const blob = new Blob([body], { type: "application/yaml" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${projectId}-kubeconfig.yaml`;
  link.click();
  URL.revokeObjectURL(url);
  els.kubeconfigMessage.textContent = rotate
    ? "Новый kubeconfig создан и скачан."
    : "Kubeconfig скачан.";
}

function toBase64NoWrap(value) {
  const bytes = new TextEncoder().encode(String(value || ""));
  let binary = "";
  for (let i = 0; i < bytes.length; i += 1) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary);
}

async function copyTextToClipboard(value) {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(value);
      return;
    } catch (error) {
      throw new Error("CLIPBOARD_PERMISSION_DENIED");
    }
  }

  const textarea = document.createElement("textarea");
  textarea.value = value;
  textarea.setAttribute("readonly", "readonly");
  textarea.style.position = "fixed";
  textarea.style.top = "-9999px";
  textarea.style.left = "-9999px";
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  const copied = document.execCommand("copy");
  document.body.removeChild(textarea);
  if (!copied) {
    throw new Error("CLIPBOARD_PERMISSION_DENIED");
  }
}

async function copyProjectKubeconfigBase64() {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) {
    return;
  }

  if (state.selectedProject?.status === "creating") {
    showToast(
      "Kubeconfig будет доступен после перехода проекта в статус «активен». Дождитесь завершения создания.",
    );
    return;
  }

  const response = await fetch(`/projects/${projectId}/kubeconfig`, {
    method: "GET",
    headers: {
      Authorization: `Bearer ${state.token}`,
    },
  });
  const kubeconfig = await response.text();
  if (!response.ok) {
    let message = kubeconfig || `${response.status} ${response.statusText}`;
    const contentType = response.headers.get("content-type") || "";
    if (contentType.includes("application/json")) {
      try {
        const parsed = JSON.parse(kubeconfig);
        if (parsed && typeof parsed.error === "string" && parsed.error.trim()) {
          message = parsed.error;
        }
      } catch (error) {
        // Откатываемся к исходному телу ответа
      }
    }
    throw new Error(message);
  }

  const encoded = toBase64NoWrap(kubeconfig);
  if (els.kubeconfigBase64Value) {
    els.kubeconfigBase64Value.value = encoded;
  }
  if (els.kubeconfigBase64Block) {
    els.kubeconfigBase64Block.classList.remove("hidden");
  }
  let copied = false;
  try {
    await copyTextToClipboard(encoded);
    copied = true;
  } catch (error) {
    copied = false;
  }
  if (els.kubeconfigMessage) {
    els.kubeconfigMessage.textContent = copied
      ? "KUBECONFIG_BASE64 vcluster-окружения скопирован. Обнови этим значением GitHub Secret проекта."
      : "Значение KUBECONFIG_BASE64 для vcluster уже готово. Скопируй его вручную и обнови GitHub Secret проекта.";
  }
  return copied;
}

function populateGitHubForm(data) {
  if (!data) {
    return;
  }
  const currentReplicaCount = document.getElementById("replicaCount")?.value;
  const currentResourceProfile = document.getElementById("resourceProfile")?.value;

  setFieldValue(
    "repositoryUrl",
    buildRepositoryURL(data.repositoryOwner || "", data.repositoryName || ""),
  );
  setFieldValue("baseBranch", data.baseBranch || "");
  setFieldValue("dockerfilePath", data.detectedDockerfile || "");
  setFieldValue("serviceName", data.detectedServiceName || "");
  setFieldValue("servicePort", data.detectedServicePort || "");
  setFieldValue("containerPort", data.detectedContainerPort || "");
  setFieldValue("serviceType", data.detectedServiceType || "LoadBalancer");
  setFieldValue("dedicatedLoadBalancer", state.selectedProject?.dedicatedLoadBalancer || false);
  if (!currentReplicaCount) {
    setFieldValue("replicaCount", state.selectedProject?.replicaCount || "1");
  }
  if (!currentResourceProfile) {
    setFieldValue("resourceProfile", state.selectedProject?.resourceProfile || "balanced");
  }
  syncDedicatedLoadBalancerField();
  renderDeployPlanSummary();
}

async function initiateTopUp(amountRub) {
  const result = await api("/billing/top-up/initiate", {
    method: "POST",
    body: JSON.stringify({ amountRub }),
  });
  if (result.confirmationUrl) {
    window.location.href = result.confirmationUrl;
  }
}

async function handleRouteChange() {
  const route = parseHash();
  state.currentPage = route.page;
  if (route.page === "projects") {
    setProjectsBulkEditMode(Boolean(route.projectsEditMode));
  } else {
    setProjectsBulkEditMode(false);
  }
  if (route.page === "project" && route.projectId) {
    state.currentProjectTab = [
      "overview",
      "stages",
      "deploy",
      "runtime",
      "releases",
      "logs",
      "access",
      "urls",
    ].includes(route.tab)
      ? route.tab
      : "overview";
    if (state.token) {
      const project = await api(`/projects/${route.projectId}`);
      syncProjectInList(project);
      if (isProjectCreating(project)) {
        renderProjects();
        renderDashboard();
        startCreatingPoller(project.id);
        clearSelectedProjectState();
        notifyProjectIsCreating(project);
        navigateToPage("projects");
        return;
      }
      await loadProject(route.projectId, project);
      renderWorkspaceHeader();
      renderNavigation();
      void ensureWorkspaceDataWarm({ projectId: route.projectId }).catch((error) => {
        console.warn("failed to warm project workspace after route change", error);
      });
      if (state.currentProjectTab === "logs") {
        void loadProjectLogs({ readInputs: false }).catch((error) => {
          console.warn("failed to load logs after route change", error);
        });
      } else {
        prefetchProjectTabData(state.currentProjectTab, { projectId: route.projectId, delay: 0 });
      }
      return;
    } else {
      state.selectedProjectId = route.projectId;
    }
  } else {
    if (route.page !== "project") {
      state.currentProjectTab = "overview";
    }
  }
  renderWorkspaceHeader();
  renderNavigation();
}

function logout(notify = true) {
  state.token = "";
  try {
    sessionStorage.removeItem("deploy_token");
  } catch (_) {}
  state.projectGitHubTokenConfigured = false;
  state.projectsBulkEditMode = false;
  state.searchQuery = "";
  state.projectStatusFilter = "";
  state.projects = [];
  clearSelectedProjectState();
  state.billing = null;
  state.telegramSettings = null;
  state.bulkDeleteSelection = new Set();
  state.bulkDeleteReport = null;
  state.bulkDeleteInProgress = false;
  state.projectSetupProjectId = "";
  fetch("/auth/logout", { method: "POST" }).catch(() => {});
  renderShell();
  renderWorkspaceHeader();
  renderNavigation();
  renderProjects();
  renderBilling();
  renderDashboard();
  renderProjectWorkspace();
  renderBulkDeletePage();
  closeProjectSetupModal();
  closeProjectGitHubTokenModal();
  if (notify) {
    showToast("Сессия завершена.");
  }
}

[
  "repositoryUrl",
  "baseBranch",
  "serviceName",
  "dockerfilePath",
  "serviceType",
  "dedicatedLoadBalancer",
  "servicePort",
  "containerPort",
  "replicaCount",
  "resourceProfile",
].forEach((id) => {
  const field = document.getElementById(id);
  if (!field) {
    return;
  }
  const scheduleSave = () => {
    if (id === "serviceType") {
      syncDedicatedLoadBalancerField();
    }
    renderDeployPlanSummary();
    if (state.applyingProjectSettings) {
      return;
    }
    state.deploymentSettingsDirty = true;
    clearTimeout(scheduleSave.timer);
    scheduleSave.timer = setTimeout(() => {
      persistDeploymentSettings().catch((error) => {
        state.deploymentSettingsDirty = true;
        showToast(`Не удалось сохранить настройки деплоя: ${error.message}`);
      });
    }, 500);
  };
  field.addEventListener("input", scheduleSave);
  field.addEventListener("change", scheduleSave);
});

on(els.loginTab, "click", () => setMode("login"));
on(els.registerTab, "click", () => setMode("register"));
(els.themeToggleButtons || []).forEach((button) => on(button, "click", () => toggleTheme()));
on(els.logoutButton, "click", () => logout(true));

function openProjectListWithFilter(status) {
  setProjectsBulkEditMode(false);
  if (state.currentPage !== "projects") {
    navigateToPage("projects");
  }
  setProjectStatusFilter(status);
}

on(els.navDashboard, "click", () => navigateToPage("dashboard"));
on(els.navProjects, "click", () => {
  setProjectsBulkEditMode(false);
  navigateToPage("projects");
});
on(els.navBilling, "click", () => navigateToPage("billing"));
on(els.navFlow, "click", () => navigateToPage("flow"));
on(els.navAbout, "click", () => navigateToPage("about"));

on(els.projectsEditModeButton, "click", () => {
  if (state.currentPage !== "projects") {
    navigateToPage("projects");
  }
  setProjectsBulkEditMode(!state.projectsBulkEditMode);
  renderBulkDeletePage();
});

on(els.bulkDeleteSelectAllButton, "click", () => {
  bulkDeletableProjects().forEach((project) => {
    state.bulkDeleteSelection.add(project.id);
  });
  renderBulkDeletePage();
});

on(els.bulkDeleteClearButton, "click", () => {
  state.bulkDeleteSelection.clear();
  renderBulkDeletePage();
});

on(els.bulkDeleteDeleteButton, "click", async () => {
  try {
    await handleBulkDeleteSelected();
  } catch (error) {
    state.bulkDeleteInProgress = false;
    renderBulkDeletePage();
    showToast(error.message);
  }
});

on(els.projectStatusFilterClear, "click", () => {
  setProjectStatusFilter("");
});

on(els.statCardAllProjects, "click", () => {
  openProjectListWithFilter("");
});

on(els.statCardActiveProjects, "click", () => {
  openProjectListWithFilter("active");
});

on(els.statCardSuspendedProjects, "click", () => {
  openProjectListWithFilter("suspended");
});

on(els.headerSelectProjectButton, "click", () => {
  if (state.selectedProjectId) {
    navigateToProject(state.selectedProjectId, "overview");
  }
});

on(els.dashboardCreateProjectButton, "click", () => {
  setProjectsBulkEditMode(false);
  navigateToPage("projects");
  window.setTimeout(() => {
    const input = document.getElementById("projectName");
    if (!input) {
      return;
    }
    input.scrollIntoView({ behavior: "smooth", block: "center" });
    input.focus();
  }, 0);
});

on(els.dashboardOpenBillingButton, "click", () => navigateToPage("billing"));
on(els.dashboardBillingPageButton, "click", () => navigateToPage("billing"));

const billingTopUpForm = document.getElementById("billingTopUpForm");
const billingTopUpAmountInput = document.getElementById("billingTopUpAmount");
if (billingTopUpForm) {
  billingTopUpForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const amount = parseFloat(billingTopUpAmountInput?.value || "");
    if (!amount || amount < 0.01) {
      showToast("Введите сумму пополнения.");
      return;
    }
    const button = els.billingTopUpButton;
    try {
      if (button) button.disabled = true;
      await initiateTopUp(amount);
    } catch (error) {
      showToast(error.message);
      if (button) button.disabled = false;
    }
  });
  billingTopUpForm.querySelectorAll(".top-up-preset-btn").forEach((btn) => {
    btn.addEventListener("click", () => {
      const amount = parseInt(btn.dataset.amount, 10);
      if (billingTopUpAmountInput) billingTopUpAmountInput.value = amount;
    });
  });
}

on(els.telegramSettingsForm, "submit", async (event) => {
  event.preventDefault();
  try {
    if (els.telegramSaveButton) {
      els.telegramSaveButton.disabled = true;
    }
    state.telegramSettings = await api("/me/telegram", {
      method: "PUT",
      body: JSON.stringify({
        username: els.telegramUsernameInput?.value.trim() || "",
        notificationsEnabled: Boolean(els.telegramNotificationsEnabled?.checked),
      }),
    });
    renderTelegramSettings();
    showToast("Telegram-настройки сохранены.");
  } catch (error) {
    showToast(error.message);
  } finally {
    if (els.telegramSaveButton) {
      els.telegramSaveButton.disabled = false;
    }
  }
});

on(els.telegramDisconnectButton, "click", async () => {
  try {
    state.telegramSettings = await api("/me/telegram", { method: "DELETE" });
    renderTelegramSettings();
    showToast("Telegram-связка очищена.");
  } catch (error) {
    showToast(error.message);
  }
});

on(els.telegramCopyCodeButton, "click", async () => {
  const command = telegramStartCommand();
  if (!command) {
    showToast("Сначала сохрани Telegram username, чтобы получить код привязки.");
    return;
  }
  try {
    await copyTextToClipboard(command);
    showToast("Команда /start скопирована.");
  } catch (error) {
    showToast("Не удалось скопировать команду автоматически.");
  }
});

on(els.projectSearch, "input", (event) => {
  state.searchQuery = event.target.value || "";
  renderProjects();
});

on(els.repositoryUrl, "change", () => {
  const value = (els.repositoryUrl?.value || "").trim();
  if (!value) {
    return;
  }
  if (!parseRepositoryReference(value)) {
    showToast("Не удалось распознать ссылку репозитория. Используй формат github.com/owner/repo.");
  }
});

on(els.toggleGitHubTokenButton, "click", () => {
  const field = document.getElementById("githubToken");
  const visible = field.type === "password";
  field.type = visible ? "text" : "password";
  els.toggleGitHubTokenButton.setAttribute(
    "aria-label",
    visible ? "Скрыть токен" : "Показать токен",
  );
  els.toggleGitHubTokenButton.setAttribute("title", visible ? "Скрыть токен" : "Показать токен");
});

on(els.openProjectGitHubTokenModalButton, "click", () => openProjectGitHubTokenModal());
on(els.projectGitHubTokenModalClose, "click", () => closeProjectGitHubTokenModal());
on(els.projectGitHubTokenModal, "click", (event) => {
  if (event.target === els.projectGitHubTokenModal) {
    closeProjectGitHubTokenModal();
  }
});

on(els.toggleProjectGitHubTokenButton, "click", () => {
  if (!els.projectGitHubTokenInput) {
    return;
  }
  const visible = els.projectGitHubTokenInput.type === "password";
  els.projectGitHubTokenInput.type = visible ? "text" : "password";
  els.toggleProjectGitHubTokenButton.setAttribute(
    "aria-label",
    visible ? "Скрыть токен" : "Показать токен",
  );
  els.toggleProjectGitHubTokenButton.setAttribute(
    "title",
    visible ? "Скрыть токен" : "Показать токен",
  );
});

on(els.saveProjectGitHubTokenButton, "click", async () => {
  const token = (els.projectGitHubTokenInput?.value || "").trim();
  if (!token) {
    showToast("Укажи GitHub токен.");
    return;
  }
  const button = els.saveProjectGitHubTokenButton;
  try {
    if (button) button.disabled = true;
    await saveProjectGitHubToken(token);
    if (els.projectGitHubTokenInput) {
      els.projectGitHubTokenInput.value = "";
      els.projectGitHubTokenInput.type = "password";
    }
    closeProjectGitHubTokenModal();
    showToast("GitHub токен проекта сохранен.");
  } catch (error) {
    showToast(error.message);
  } finally {
    if (button) button.disabled = false;
  }
});

on(els.deleteProjectGitHubTokenButton, "click", async () => {
  if (!state.projectGitHubTokenConfigured) {
    return;
  }
  const confirmed = await confirmAction({
    title: "Удалить GitHub токен проекта?",
    description: "После удаления для GitHub-операций снова потребуется разовый токен в форме.",
    confirmLabel: "Удалить токен",
    tone: "danger",
  });
  if (!confirmed) {
    return;
  }
  const button = els.deleteProjectGitHubTokenButton;
  try {
    if (button) button.disabled = true;
    await deleteProjectGitHubToken();
    closeProjectGitHubTokenModal();
    showToast("GitHub токен проекта удален.");
  } catch (error) {
    showToast(error.message);
  } finally {
    if (button) button.disabled = false;
  }
});

on(els.projectSetupModalClose, "click", () => closeProjectSetupModal());
on(els.projectSetupLaterButton, "click", () => closeProjectSetupModal());
on(els.projectSetupModal, "click", (event) => {
  if (event.target === els.projectSetupModal) {
    closeProjectSetupModal();
  }
});

on(els.projectSetupForm, "submit", async (event) => {
  event.preventDefault();

  const projectId = state.projectSetupProjectId || state.selectedProjectId;
  if (!projectId) {
    showToast("Проект для настройки не найден.");
    return;
  }

  const parsedRepository = parseRepositoryReference(els.projectSetupRepositoryUrl?.value || "");
  if (!parsedRepository) {
    showToast("Укажи корректную ссылку на GitHub репозиторий.");
    return;
  }

  const baseBranch = (els.projectSetupBaseBranch?.value || "").trim();
  if (!baseBranch) {
    showToast("Укажи основную ветку.");
    return;
  }

  try {
    setProjectSetupModalBusy(true);

    const currentProject =
      state.selectedProject && state.selectedProject.id === projectId
        ? state.selectedProject
        : await api(`/projects/${projectId}`);

    const updatedProject = await api(`/projects/${projectId}/deployment-settings`, {
      method: "PUT",
      body: JSON.stringify(
        buildDeploymentSettingsPayloadFromProject(currentProject, {
          repositoryOwner: parsedRepository.repositoryOwner,
          repositoryName: parsedRepository.repositoryName,
          baseBranch,
        }),
      ),
    });

    state.selectedProjectId = projectId;
    state.selectedProject = updatedProject;
    syncProjectInList(updatedProject);
    applyProjectDeploymentSettings(updatedProject);
    renderProjects();
    renderDashboard();
    renderProjectWorkspace();
    renderWorkspaceHeader();
    closeProjectSetupModal();
    showToast("Первичная настройка сохранена. Параметры можно изменить позже во вкладке «Деплой».");
  } catch (error) {
    showToast(error.message);
  } finally {
    setProjectSetupModalBusy(false);
  }
});

on(els.authForm, "submit", async (event) => {
  event.preventDefault();
  clearAuthError();
  try {
    const endpoint = state.mode === "login" ? "/auth/login" : "/auth/register";
    const response = await api(endpoint, {
      method: "POST",
      body: JSON.stringify(authPayloadFromForm()),
    });
    setToken(response.token);
    renderShell();
    await Promise.all([loadBilling(), loadProjects(), loadTelegramSettings()]);
    await handleRouteChange();
    showToast(state.mode === "login" ? "Вход выполнен." : "Регистрация завершена.");
  } catch (error) {
    showAuthError(error.message);
  }
});

on(els.createProjectForm, "submit", async (event) => {
  event.preventDefault();
  const input = document.getElementById("projectName");
  const submitButton = els.createProjectForm.querySelector('button[type="submit"]');
  const originalText = submitButton.textContent;

  try {
    submitButton.disabled = true;
    submitButton.textContent = "Создание...";
    input.disabled = true;

    showToast("Создаем проект. Подготовка среды может занять около минуты.");

    console.log(`[UI] Creating project: ${input.value.trim()}`);

    const project = await api("/projects", {
      method: "POST",
      body: JSON.stringify({ name: input.value.trim() }),
      timeout: 120000,
    });

    input.value = "";
    await loadProjects();
    showToast("Проект создается. Уведомим, когда будет готов.");
    startCreatingPoller(project.id);
  } catch (error) {
    console.error("[UI] Project creation failed:", error);
    showToast(`Не удалось создать проект: ${error.message}`);
  } finally {
    submitButton.disabled = false;
    submitButton.textContent = originalText;
    input.disabled = false;
  }
});

on(els.refreshProjectsButton, "click", async () => {
  const originalText = els.refreshProjectsButton.textContent;

  try {
    els.refreshProjectsButton.disabled = true;
    els.refreshProjectsButton.textContent = "Обновление...";

    await loadProjects();
    showToast("Список проектов обновлен.");
  } catch (error) {
    showToast(`Не удалось обновить список проектов: ${error.message}`);
  } finally {
    els.refreshProjectsButton.disabled = false;
    els.refreshProjectsButton.textContent = originalText;
  }
});

on(els.detectGitHubButton, "click", async () => {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) {
    return;
  }

  applyPrimarySetupToDeployForm();
  const repository = resolveRepositoryFields();
  if (!repository.repositoryOwner || !repository.repositoryName) {
    showToast(
      "В первичной настройке не найден репозиторий. Открой «Деплой» и укажи корректную ссылку GitHub.",
    );
    return;
  }
  const baseBranch = document.getElementById("baseBranch").value.trim();
  if (!baseBranch) {
    showToast("В первичной настройке не найдена основная ветка. Укажи ее на вкладке «Деплой».");
    return;
  }

  const payload = {
    repositoryOwner: repository.repositoryOwner,
    repositoryName: repository.repositoryName,
    baseBranch,
    dockerfilePath: document.getElementById("dockerfilePath").value.trim(),
    serviceName: document.getElementById("serviceName").value.trim(),
    githubToken: githubTokenValue(),
  };

  try {
    const response = await api(`/projects/${projectId}/github/questions`, {
      method: "POST",
      body: JSON.stringify(payload),
    });
    populateGitHubForm(response);
    state.deploymentSettingsDirty = true;
    await persistDeploymentSettings();
    els.githubMessage.textContent = "Автодетект выполнен. Поля обновлены по данным GitHub.";
    showToast("Подсказки из GitHub загружены.");
  } catch (error) {
    showToast(error.message);
  }
});

on(els.githubForm, "submit", async (event) => {
  event.preventDefault();
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) {
    return;
  }
  const repository = resolveRepositoryFields();
  if (!repository.repositoryOwner || !repository.repositoryName) {
    showToast("Укажи корректную ссылку на GitHub репозиторий.");
    return;
  }

  const payload = {
    repositoryOwner: repository.repositoryOwner,
    repositoryName: repository.repositoryName,
    baseBranch: document.getElementById("baseBranch").value.trim(),
    stageSlug: document.getElementById("deployStageSlug").value || "production",
    serviceName: document.getElementById("serviceName").value.trim(),
    dockerfilePath: document.getElementById("dockerfilePath").value.trim(),
    servicePort: Number(document.getElementById("servicePort").value || 0),
    containerPort: Number(document.getElementById("containerPort").value || 0),
    serviceType: document.getElementById("serviceType").value,
    dedicatedLoadBalancer: dedicatedLoadBalancerEnabled(),
    replicaCount: Number(document.getElementById("replicaCount").value || 1),
    resourceProfile: document.getElementById("resourceProfile").value,
    githubToken: githubTokenValue(),
  };

  if (!payload.githubToken && !state.projectGitHubTokenConfigured) {
    showToast(
      "Укажи разовый GitHub токен в форме или сохрани токен проекта в разделе «Интеграция» выше.",
    );
    return;
  }

  try {
    setGitHubBusy(
      true,
      "Создаем Pull Request и записываем файлы в репозиторий. Обычно это занимает до 10 секунд.",
    );
    const response = await api(`/projects/${projectId}/github/bootstrap`, {
      method: "POST",
      body: JSON.stringify(payload),
    });
    await loadBilling();
    if (response.pullRequestUrl) {
      els.githubMessage.innerHTML = `PR создан: <a href="${escapeAttr(response.pullRequestUrl)}" target="_blank" rel="noreferrer">${escapeHtml(response.pullRequestUrl)}</a>`;
      showToast("PR создан.");
    } else if (response.noChanges) {
      els.githubMessage.textContent =
        "Изменений для нового PR нет: deploy-конфиг уже совпадает с текущим состоянием репозитория.";
      showToast("Изменений для PR нет.");
    } else {
      els.githubMessage.textContent = "Настройки репозитория сохранены.";
      showToast("Настройки репозитория сохранены.");
    }
    await loadProjects();
    await loadProject(projectId);
  } catch (error) {
    showToast(error.message);
    els.githubMessage.textContent = `Не удалось создать PR: ${error.message}`;
  } finally {
    setGitHubBusy(false);
  }
});

on(els.loadCostButton, "click", async () => {
  try {
    await loadCost();
    showToast("Стоимость обновлена.");
  } catch (error) {
    showToast(error.message);
  }
});

[els.runtimeRefreshButton, els.overviewRefreshRuntimeButton].filter(Boolean).forEach((button) => {
  button.addEventListener("click", async () => {
    try {
      await loadRuntimeStatus();
      showToast("Статус среды обновлен.");
    } catch (error) {
      showToast(error.message);
    }
  });
});

on(els.refreshReleasesButton, "click", async () => {
  const originalText = els.refreshReleasesButton.textContent;

  try {
    els.refreshReleasesButton.disabled = true;
    els.refreshReleasesButton.textContent = "Обновление...";

    await loadReleases();
    showToast("Релизы обновлены.");
  } catch (error) {
    showToast(`Не удалось обновить релизы: ${error.message}`);
  } finally {
    els.refreshReleasesButton.disabled = false;
    els.refreshReleasesButton.textContent = originalText;
  }
});

async function switchSelectedStage(stageID) {
  if (!stageID || stageID === state.selectedStageId) {
    return;
  }
  state.selectedStageId = stageID;
  renderStageSelectors();
  renderStages();
  const tasks = [loadReleases(), loadRuntimeStatus()];
  if (state.currentProjectTab === "logs") {
    tasks.push(loadProjectLogs({ readInputs: false }));
  }
  await Promise.allSettled(tasks);
}

on(els.releasesStageFilter, "change", async (event) => {
  const stageID = event.target.value || "";
  try {
    await switchSelectedStage(stageID);
  } catch (error) {
    showToast(error.message);
  }
});

on(els.runtimeStageSelect, "change", async (event) => {
  const stageID = event.target.value || "";
  try {
    await switchSelectedStage(stageID);
  } catch (error) {
    showToast(error.message);
  }
});

on(els.logsStageSelect, "change", async (event) => {
  const stageID = event.target.value || "";
  try {
    await switchSelectedStage(stageID);
  } catch (error) {
    showToast(error.message);
  }
});

on(els.refreshStagesButton, "click", async () => {
  const originalText = els.refreshStagesButton.textContent;

  try {
    els.refreshStagesButton.disabled = true;
    els.refreshStagesButton.textContent = "Обновление...";
    await loadStages();
    showToast("Контуры обновлены.");
  } catch (error) {
    showToast(`Не удалось обновить контуры: ${error.message}`);
  } finally {
    els.refreshStagesButton.disabled = false;
    els.refreshStagesButton.textContent = originalText;
  }
});

on(els.createStageForm, "submit", async (event) => {
  event.preventDefault();
  const projectID = getSelectedProjectOrWarn();
  if (!projectID) {
    return;
  }

  const stageName = (els.stageNameInput?.value || "").trim();
  if (!stageName) {
    showToast("Укажи имя контура.");
    return;
  }

  try {
    els.createStageButton.disabled = true;
    els.createStageButton.textContent = "Создание...";
    const stage = await api(`/projects/${projectID}/stages`, {
      method: "POST",
      body: JSON.stringify({ name: stageName }),
    });
    els.stageNameInput.value = "";
    if (stage && stage.id) {
      state.selectedStageId = stage.id;
    }
    await Promise.allSettled([loadStages(), loadRuntimeStatus(), loadReleases()]);
    showToast(`Контур ${stageName} создан.`);
  } catch (error) {
    showToast(error.message);
  } finally {
    els.createStageButton.disabled = false;
    els.createStageButton.textContent = "Создать";
  }
});

on(els.logsRefreshButton, "click", async () => {
  const originalText = els.logsRefreshButton.textContent;
  try {
    els.logsRefreshButton.disabled = true;
    els.logsRefreshButton.textContent = "Загрузка...";
    await loadProjectLogs();
    showToast("Логи обновлены.");
  } catch (error) {
    showToast(`Не удалось загрузить логи: ${error.message}`);
  } finally {
    els.logsRefreshButton.disabled = false;
    els.logsRefreshButton.textContent = originalText;
  }
});

[
  els.logsSearchInput,
  els.logsLevelSelect,
  els.logsSinceSelect,
  els.logsPodInput,
  els.logsContainerInput,
]
  .filter(Boolean)
  .forEach((element) => {
    element.addEventListener("input", syncLogFiltersFromInputs);
    element.addEventListener("change", syncLogFiltersFromInputs);
  });

on(els.logsFullscreenButton, "click", () => {
  setLogsFullscreen(!state.logsFullscreen);
});

on(els.logsOpenGrafanaButton, "click", () => {
  const url = String(state.selectedProject?.grafanaUrl || "").trim();
  if (!url) {
    showToast("Grafana для проекта пока недоступна.");
    return;
  }
  window.open(url, "_blank", "noopener,noreferrer");
});

on(els.downloadKubeconfigButton, "click", async () => {
  try {
    await downloadKubeconfig(false);
    showToast("Kubeconfig скачан.");
  } catch (error) {
    showToast(error.message);
  }
});

on(els.rotateKubeconfigButton, "click", async () => {
  try {
    await downloadKubeconfig(true);
    showToast("Kubeconfig перевыпущен.");
  } catch (error) {
    showToast(error.message);
  }
});

on(els.copyKubeconfigBase64Button, "click", async () => {
  if (!els.copyKubeconfigBase64Button) {
    return;
  }
  const originalText = els.copyKubeconfigBase64Button.textContent;

  try {
    els.copyKubeconfigBase64Button.disabled = true;
    els.copyKubeconfigBase64Button.textContent = "Готовим значение...";
    const copied = await copyProjectKubeconfigBase64();
    showToast(
      copied
        ? "KUBECONFIG_BASE64 для vcluster скопирован."
        : "KUBECONFIG_BASE64 для vcluster готов. Скопируй его из блока ниже.",
    );
  } catch (error) {
    showToast(error.message);
  } finally {
    els.copyKubeconfigBase64Button.disabled = false;
    els.copyKubeconfigBase64Button.textContent = originalText;
  }
});

on(els.copyVisibleKubeconfigBase64Button, "click", async () => {
  const value = els.kubeconfigBase64Value?.value || "";
  if (!value) {
    showToast("Сначала получи KUBECONFIG_BASE64 для vcluster.");
    return;
  }

  try {
    await copyTextToClipboard(value);
    showToast("KUBECONFIG_BASE64 для vcluster скопирован.");
  } catch (error) {
    els.kubeconfigBase64Value?.focus();
    els.kubeconfigBase64Value?.select();
    showToast(
      "Браузер не дал скопировать автоматически. Значение уже выделено, скопируй его вручную.",
    );
  }
});

on(els.suspendButton, "click", async () => {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) {
    return;
  }

  const confirmed = await confirmAction({
    title: "Приостановить проект?",
    description:
      "Среда останется в системе, но новые действия по проекту будут остановлены до возобновления.",
    confirmLabel: "Приостановить",
  });
  if (!confirmed) {
    return;
  }

  const originalText = els.suspendButton.textContent;

  try {
    els.suspendButton.disabled = true;
    els.suspendButton.textContent = "Приостановка...";

    await api(`/projects/${projectId}/suspend`, { method: "POST" });
    showToast("Проект приостановлен.");
    await Promise.all([loadProjects(), loadProject(projectId)]);
  } catch (error) {
    showToast(`Не удалось приостановить проект: ${error.message}`);
  } finally {
    els.suspendButton.disabled = false;
    els.suspendButton.textContent = originalText;
  }
});

on(els.resumeButton, "click", async () => {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) {
    return;
  }

  const originalText = els.resumeButton.textContent;

  try {
    await loadBilling();
    const guard = getProjectGuardState();
    if (guard && guard.kind === "insufficient-balance") {
      navigateToPage("billing");
      showToast("Сначала пополни баланс: проект остановлен из-за нулевого доступного остатка.");
      return;
    }

    const confirmed = await confirmAction({
      title: "Возобновить проект?",
      description: "Среда снова перейдет в активное состояние и проект вернется в рабочий режим.",
      confirmLabel: "Возобновить",
    });
    if (!confirmed) {
      return;
    }

    els.resumeButton.disabled = true;
    els.resumeButton.textContent = "Возобновление...";

    await api(`/projects/${projectId}/resume`, {
      method: "POST",
      timeout: 30000,
    });

    showToast("Проект снова активен.");
    await Promise.all([loadProjects(), loadProject(projectId), loadBilling()]);
  } catch (error) {
    showToast(`Не удалось возобновить проект: ${error.message}`);
  } finally {
    els.resumeButton.disabled = false;
    els.resumeButton.textContent = originalText;
  }
});

on(els.deleteButton, "click", async () => {
  const projectId = getSelectedProjectOrWarn();
  if (!projectId) {
    return;
  }
  const confirmed = await confirmAction({
    title: "Удалить проект?",
    description: `Проект ${projectId} будет удален из консоли. Используйте это действие только если среда больше не нужна.`,
    confirmLabel: "Удалить проект",
    tone: "danger",
  });
  if (!confirmed) {
    return;
  }
  try {
    await api(`/projects/${projectId}`, { method: "DELETE" });
    clearSelectedProjectState();
    state.bulkDeleteSelection.delete(projectId);
    await loadProjects();
    navigateToPage("dashboard");
    showToast("Проект удален.");
  } catch (error) {
    showToast(error.message);
  }
});

on(els.tabOverview, "click", () => {
  if (state.selectedProjectId) navigateToProject(state.selectedProjectId, "overview");
});
on(els.tabStages, "click", () => {
  if (state.selectedProjectId) navigateToProject(state.selectedProjectId, "stages");
});
on(els.tabDeploy, "click", () => {
  if (state.selectedProjectId) navigateToProject(state.selectedProjectId, "deploy");
});
on(els.tabRuntime, "click", () => {
  if (state.selectedProjectId) navigateToProject(state.selectedProjectId, "runtime");
});
on(els.tabReleases, "click", () => {
  if (state.selectedProjectId) navigateToProject(state.selectedProjectId, "releases");
});
on(els.tabLogs, "click", () => {
  if (state.selectedProjectId) navigateToProject(state.selectedProjectId, "logs");
});
on(els.tabAccess, "click", () => {
  if (state.selectedProjectId) navigateToProject(state.selectedProjectId, "access");
});
on(els.tabUrls, "click", async () => {
  if (state.selectedProjectId) {
    navigateToProject(state.selectedProjectId, "urls");
    await loadProjectURLs();
  }
});
on(els.refreshUrlsButton, "click", () => loadProjectURLs());

[
  [els.tabOverview, "overview"],
  [els.tabStages, "stages"],
  [els.tabDeploy, "deploy"],
  [els.tabRuntime, "runtime"],
  [els.tabReleases, "releases"],
  [els.tabLogs, "logs"],
  [els.tabAccess, "access"],
].forEach(([button, tab]) => {
  if (!button) {
    return;
  }
  button.addEventListener("pointerenter", () => prefetchProjectTabData(tab));
  button.addEventListener("focus", () => prefetchProjectTabData(tab));
});

on(els.confirmModalCancel, "click", () => closeConfirmModal(false));
on(els.confirmModalClose, "click", () => closeConfirmModal(false));
on(els.confirmModalSubmit, "click", () => closeConfirmModal(true));
on(els.confirmModal, "click", (event) => {
  if (event.target === els.confirmModal) {
    closeConfirmModal(false);
  }
});

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape" && state.logsFullscreen) {
    setLogsFullscreen(false);
    return;
  }
  if (event.key === "Escape" && pendingConfirmResolver) {
    closeConfirmModal(false);
    return;
  }
  if (
    event.key === "Escape" &&
    els.projectGitHubTokenModal &&
    !els.projectGitHubTokenModal.classList.contains("hidden")
  ) {
    closeProjectGitHubTokenModal();
    return;
  }
  if (
    event.key === "Escape" &&
    els.projectSetupModal &&
    !els.projectSetupModal.classList.contains("hidden")
  ) {
    closeProjectSetupModal();
  }
});

on(els.projectGuardActionButton, "click", async () => {
  const guard = getProjectGuardState();
  if (!guard) {
    return;
  }
  if (guard.kind === "insufficient-balance") {
    navigateToPage("billing");
    showToast("Пополнение баланса разблокирует безопасное возобновление.");
    return;
  }
  els.resumeButton.click();
});

window.addEventListener("hashchange", () => {
  handleRouteChange().catch((error) => {
    showToast(error.message);
  });
});

async function bootstrap() {
  applyTheme(readStoredTheme());
  if (!state.token) {
    try {
      state.token = sessionStorage.getItem("deploy_token") || "";
    } catch (_) {}
  }
  renderShell();
  renderWorkspaceHeader();
  renderNavigation();
  renderBilling();
  renderDashboard();
  renderProjectWorkspace();
  renderProjects();
  renderDeployPlanSummary();
  renderBulkDeletePage();
  setMode("login");

  if (!state.token) {
    return;
  }

  try {
    await Promise.all([loadBilling(), loadProjects(), loadTelegramSettings()]);
    await handleRouteChange();
  } catch (error) {
    logout(false);
    showToast(error.message);
  }
}

bootstrap().catch((error) => {
  showToast(error.message);
});
