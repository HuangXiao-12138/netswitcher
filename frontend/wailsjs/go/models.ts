/*
 _       __      _ __
| |     / /___ _(_) /____
| | /| / / __ `/ / / ___/
| |/ |/ / /_/ / / (__  )
|__/|__/\__,_/_/_/____/
The electron alternative for Go
(c) Lea Anthony 2019-present
*/

// Hand-mirrored from the Go structs. Field names match the JSON wire format:
// ifacemgr.Interface marshals with capitalised fields (no json tags); other
// types use their explicit camelCase tags.

// ----- config.Config / Profile / Rule -----

export interface MetricPolicy {
  preferredInterface?: string;
  preferredMetric?: number;
  othersMetric?: number;
}

export interface Rule {
  id: string;
  destination: string;
  viaInterface: string;
  viaGateway: string; // "auto" or an IPv4 literal
  metric?: number;
  enabled?: boolean;
}

export interface Profile {
  id: string;
  name: string;
  rules: Rule[];
  defaultRouteInterface?: string;
  autoManageMetrics?: boolean;
  metricPolicy?: MetricPolicy;
}

export interface Config {
  $schema?: string;
  version: number;
  activeProfile: string;
  profiles: Profile[];
  logLevel?: string;
}

// ----- ifacemgr.Interface (capitalised field names — no json tags) -----

export interface Interface {
  Index: number;
  Name: string;
  FriendlyName: string;
  MAC: string;
  IPv4: string[];
  Gateways: string[];
  IsUp: boolean;
  MediaType: string;
  IfType: number;
}

// ----- state.Entry (route records) -----

export interface Entry {
  destination: string;
  gateway: string;
  interface: string;
  ifIndex: number;
  metric: number;
}

// ----- routeengine.ApplyResult + sub-types -----

export interface SkippedRule {
  ruleId: string;
  destination: string;
  viaInterface: string;
  reason: string;
}

export interface RuleError {
  ruleId?: string;
  destination?: string;
  op: string;
  message: string;
}

export interface MetricChange {
  interface: string;
  newMetric: number;
}

export interface ApplyResult {
  applied: Entry[] | null;
  removed: Entry[] | null;
  skipped: SkippedRule[] | null;
  errors: RuleError[] | null;
  metrics: MetricChange[] | null;
  at: string;
  reason: string;
}

// ----- conflict.Conflict -----

export interface Conflict {
  type: string; // "vpn_present" | "external_override"
  description: string;
  interface?: string;
  destination?: string;
}

// ----- core.StatusResponse -----

export interface StatusResponse {
  interfaces: Interface[];
  activeProfile: Profile | null;
  lastResult: ApplyResult;
  conflicts: Conflict[] | null;
  snapshotAt: string;
}

// ----- appapi.RouteRow (Routes page) -----

export interface RouteRow {
  destinationPrefix: string;
  nextHop: string;
  interfaceIndex: number;
  interfaceAlias: string;
  routeMetric: number;
  interfaceMetric: number;
  source: string; // "managed" | "system" | "suspect"
}

// ----- appapi.AppInfo (Settings page) -----

export interface AppInfo {
  version: string;
  elevated: boolean;
  engineOn: boolean;
  configPath: string;
  statePath: string;
  logDir: string;
  pipeName?: string;
}
