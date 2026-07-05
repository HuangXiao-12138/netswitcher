/*
 _       __      _ __
| |     / /___ _(_) /____
| | /| / / __ `/ / / ___/
| |/ |/ / /_/ / / (__  )
|__/|__/\__,_/_/_/____/
The electron alternative for Go
(c) Lea Anthony 2019-present
*/

// Hand-written bindings. Wails injects window.go.<package>.<Type>.<Method> at
// runtime; these wrappers give the frontend typed call sites.

export function IsElevated() {
  return window.go.appapi.API.IsElevated();
}

export function EngineActive() {
  return window.go.appapi.API.EngineActive();
}

export function RelaunchElevated() {
  return window.go.appapi.API.RelaunchElevated();
}

export function AutoStartInstalled() {
  return window.go.appapi.API.AutoStartInstalled();
}

export function InstallAutoStart() {
  return window.go.appapi.API.InstallAutoStart();
}

export function UninstallAutoStart() {
  return window.go.appapi.API.UninstallAutoStart();
}

export function GetStatus() {
  return window.go.appapi.API.GetStatus();
}

export function GetConfig() {
  return window.go.appapi.API.GetConfig();
}

export function SaveConfig(config) {
  return window.go.appapi.API.SaveConfig(config);
}

export function SaveProfile(profile) {
  return window.go.appapi.API.SaveProfile(profile);
}

export function DeleteProfile(id) {
  return window.go.appapi.API.DeleteProfile(id);
}

export function SetActiveProfile(id) {
  return window.go.appapi.API.SetActiveProfile(id);
}

export function ApplyNow() {
  return window.go.appapi.API.ApplyNow();
}

export function GetRouteTable() {
  return window.go.appapi.API.GetRouteTable();
}

export function Ping(target) {
  return window.go.appapi.API.Ping(target);
}

export function Tracert(target) {
  return window.go.appapi.API.Tracert(target);
}

export function StopDiag() {
  return window.go.appapi.API.StopDiag();
}

export function SubscribeLogs(level) {
  return window.go.appapi.API.SubscribeLogs(level);
}

export function GetAppInfo() {
  return window.go.appapi.API.GetAppInfo();
}

export function GetLogLevel() {
  return window.go.appapi.API.GetLogLevel();
}

export function SetLogLevel(level) {
  return window.go.appapi.API.SetLogLevel(level);
}

export function OpenLogFolder() {
  return window.go.appapi.API.OpenLogFolder();
}
