// Frontend IPC wrapper. Re-exports the Wails bindings with friendlier names
// and centralizes the call surface so pages don't import paths directly.
import {
  ServiceAvailable,
  ServiceInstalled,
  StartServiceElevated,
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
} from "../../wailsjs/go/appapi/API";
import {
  EventsOn,
  EventsOff,
} from "../../wailsjs/runtime/runtime";

export const api = {
  serviceAvailable: () => ServiceAvailable(),
  serviceInstalled: () => ServiceInstalled(),
  startServiceElevated: () => StartServiceElevated(),
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
};

export const events = {
  on: EventsOn,
  off: EventsOff,
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
