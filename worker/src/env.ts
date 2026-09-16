export interface Env {
  BROKER: DurableObjectNamespace;
  /** 与 Go 版 EASY_UNLOCKER_PAIRING_TOKEN 同一个值：CLI 用它发请求，App 用它换设备令牌 */
  EASY_UNLOCKER_PAIRING_TOKEN: string;
  /** 与 Go 版 EASY_UNLOCKER_ADMIN_TOKEN 同一个值，给 /v1/admin/pending 用 */
  EASY_UNLOCKER_ADMIN_TOKEN: string;
  /** HMAC 决策签名用的 key；spike 里只有 ntfy 按钮那条路会用到它 */
  EASY_UNLOCKER_DECISION_KEY: string;
  /** Firebase service account JSON 原文，走 wrangler secret */
  EASY_UNLOCKER_FCM_CREDENTIALS?: string;
  MAX_TTL_SECONDS?: string;
  MAX_WAIT_MS?: string;
  /** 最近一次 release 的 tag（vYYYY.MM.DD[-n]），/v1/version 报给 CLI 做更新提示。release.sh 发版时同步 wrangler.toml 的 [vars]。 */
  CLI_VERSION?: string;
}
