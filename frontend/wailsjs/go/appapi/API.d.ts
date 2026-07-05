/*
 _       __      _ __
| |     / /___ _(_) /____
| | /| / / __ `/ / / ___/
| |/ |/ / /_/ / / (__  )
|__/|__/\__,_/_/_/____/
The electron alternative for Go
(c) Lea Anthony 2019-present
*/

import type {
  Config,
  Profile,
  StatusResponse,
  ApplyResult,
  RouteRow,
} from "../models";

export function IsElevated(): Promise<boolean>;
export function IsMaximised(): Promise<boolean>;
export function EngineActive(): Promise<boolean>;
export function RelaunchElevated(): Promise<void>;
export function AutoStartInstalled(): Promise<boolean>;
export function InstallAutoStart(): Promise<void>;
export function UninstallAutoStart(): Promise<void>;
export function GetStatus(): Promise<StatusResponse>;
export function GetConfig(): Promise<Config>;
export function SaveConfig(config: Config): Promise<void>;
export function SaveProfile(profile: Profile): Promise<void>;
export function DeleteProfile(id: string): Promise<void>;
export function SetActiveProfile(id: string): Promise<void>;
export function ApplyNow(): Promise<ApplyResult>;
export function GetRouteTable(): Promise<RouteRow[]>;
export function Ping(target: string): Promise<void>;
export function Tracert(target: string): Promise<void>;
export function StopDiag(): Promise<void>;
export function SubscribeLogs(level: string): Promise<void>;
export function RecentLogs(n: number): Promise<string[]>;

export function GetAppInfo(): Promise<AppInfo>;
export function GetLogLevel(): Promise<string>;
export function SetLogLevel(level: string): Promise<void>;
export function OpenLogFolder(): Promise<void>;
