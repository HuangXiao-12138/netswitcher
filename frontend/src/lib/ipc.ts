// Frontend IPC wrapper. Re-exports the Wails bindings with friendlier names
// and centralizes the call surface so pages don't import paths directly.
import {
  IsElevated,
  IsMaximised,
  EngineActive,
  RelaunchElevated,
  AutoStartInstalled,
  InstallAutoStart,
  UninstallAutoStart,
  GetStatus,
  GetConfig,
  SaveConfig,
  SaveProfile,
  DeleteProfile,
  SetActiveProfile,
  DeactivateProfile,
  ApplyNow,
  GetRouteTable,
  Ping,
  Tracert,
  StopDiag,
  SubscribeLogs,
  RecentLogs,
  GetAppInfo,
  GetLogLevel,
  SetLogLevel,
  OpenLogFolder,
} from "../../wailsjs/go/appapi/API";
import {
  EventsOn,
  EventsOff,
  WindowMinimise,
  WindowToggleMaximise,
  WindowHide,
} from "../../wailsjs/runtime/runtime";

export const api = {
  isElevated: () => IsElevated(),
  isMaximised: () => IsMaximised(),
  engineActive: () => EngineActive(),
  relaunchElevated: () => RelaunchElevated(),
  autoStartInstalled: () => AutoStartInstalled(),
  installAutoStart: () => InstallAutoStart(),
  uninstallAutoStart: () => UninstallAutoStart(),
  getStatus: () => GetStatus(),
  getConfig: () => GetConfig(),
  saveConfig: (c: any) => SaveConfig(c),
  saveProfile: (p: any) => SaveProfile(p),
  deleteProfile: (id: string) => DeleteProfile(id),
  setActiveProfile: (id: string) => SetActiveProfile(id),
  deactivateProfile: () => DeactivateProfile(),
  applyNow: () => ApplyNow(),
  getRouteTable: () => GetRouteTable(),
  ping: (target: string) => Ping(target),
  tracert: (target: string) => Tracert(target),
  stopDiag: () => StopDiag(),
  subscribeLogs: (level: string) => SubscribeLogs(level),
  recentLogs: (n: number) => RecentLogs(n),
  getAppInfo: () => GetAppInfo(),
  getLogLevel: () => GetLogLevel(),
  setLogLevel: (level: string) => SetLogLevel(level),
  openLogFolder: () => OpenLogFolder(),
};

export const events = {
  on: EventsOn,
  off: EventsOff,
};

// Window controls for the frameless custom title bar.
export const wc = {
  minimise: () => WindowMinimise(),
  toggleMax: () => WindowToggleMaximise(),
  hide: () => WindowHide(),
};

// ── Theme (UI preference, persisted in localStorage, applies a data-theme
// attribute on <html> that app.css keys off). Values: "a" | "b" | "c".
export type ThemeId = "a" | "b" | "c";
const THEME_KEY = "ns-theme";
const VALID: ThemeId[] = ["a", "b", "c"];

export function getTheme(): ThemeId {
  const t = localStorage.getItem(THEME_KEY);
  return VALID.includes(t as ThemeId) ? (t as ThemeId) : "a";
}
export function setTheme(t: ThemeId) {
  if (!VALID.includes(t)) return;
  localStorage.setItem(THEME_KEY, t);
  document.documentElement.dataset.theme = t;
}

// Event names emitted by the backend.
export const EVT = {
  diagLine: "diag:line",
  diagEnd: "diag:end",
  diagError: "diag:error",
  logLine: "log:line",
  logEnd: "log:end",
  statusChanged: "status:changed",
} as const;
