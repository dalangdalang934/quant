import "swr";

declare module "swr" {
  interface SWRConfiguration<_Data = unknown, _Error = unknown, _Fn = unknown> {
    refreshIntervalWhenHidden?: number;
    disableActivityTracking?: boolean;
  }
}
