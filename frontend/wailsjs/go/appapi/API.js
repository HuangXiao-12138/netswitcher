/*
 _       __      _ __
| |     / /___ _(_) /____
| | /| / / __ `/ / / ___/
| |/ |/ / /_/ / / (__  )
|__/|__/\__,_/_/_/____/
The electron alternative for Go
(c) Lea Anthony 2019-present
*/

// Hand-written bindings (Wails' generator could not run on this toolchain).
// Wails injects window.go.<package>.<Type>.<Method> at runtime; these wrappers
// just give the frontend typed call sites. Each returns a Promise.

export function ServiceAvailable() {
  return window.go.appapi.API.ServiceAvailable();
}

export function StartServiceElevated() {
  return window.go.appapi.API.StartServiceElevated();
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
