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
  ApplyNow,
  GetRouteTable,
  Ping,
  Tracert,
  StopDiag,
  SubscribeLogs,
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
  applyNow: () => ApplyNow(),
  getRouteTable: () => GetRouteTable(),
  ping: (target: string) => Ping(target),
  tracert: (target: string) => Tracert(target),
  stopDiag: () => StopDiag(),
  subscribeLogs: (level: string) => SubscribeLogs(level),
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

// Event names emitted by the backend.
export const EVT = {
  diagLine: "diag:line",
  diagEnd: "diag:end",
  diagError: "diag:error",
  logLine: "log:line",
  logEnd: "log:end",
  statusChanged: "status:changed",
} as const;
