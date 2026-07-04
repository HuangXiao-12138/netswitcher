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

export function ServiceAvailable(): Promise<boolean>;
export function StartServiceElevated(): Promise<void>;
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
