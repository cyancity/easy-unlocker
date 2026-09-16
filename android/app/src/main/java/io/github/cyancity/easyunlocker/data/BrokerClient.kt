package io.github.cyancity.easyunlocker.data

import org.json.JSONArray
import org.json.JSONObject
import java.io.OutputStreamWriter
import java.net.HttpURLConnection
import java.net.URL

class BrokerClient(
    var brokerUrl: String,
    var deviceToken: String,
) {
    fun pair(pairingToken: String, name: String, vaultId: String = ""): String {
        val body = JSONObject().put("name", name).put("vault_id", vaultId).toString()
        val raw = request("POST", "/v1/device/pair", body, pairingToken)
        return JSONObject(raw).getString("device_token")
    }

    fun pending(): List<PendingRequest> {
        val raw = request("GET", "/v1/device/pending", null, deviceToken)
        val arr = JSONArray(raw)
        return buildList {
            for (i in 0 until arr.length()) {
                val o = arr.getJSONObject(i)
                add(
                    PendingRequest(
                        request_id = o.optString("request_id"),
                        item = o.optString("item"),
                        mode = o.optString("mode"),
                        purpose = o.optString("purpose"),
                        ttl = o.optInt("ttl"),
                        target = o.optString("target"),
                        requester = o.optString("requester"),
                        expires_at = o.optString("expires_at"),
                        state = o.optString("state"),
                        seal_public_key = o.optString("seal_public_key"),
                        delivery = o.optString("delivery"),
                        public_key = o.optString("public_key"),
                        ssh_user = o.optString("ssh_user"),
                        cert_ttl = o.optInt("cert_ttl"),
                    )
                )
            }
        }
    }

    fun registerPushToken(pushToken: String) {
        if (pushToken.isBlank()) return
        request("POST", "/v1/device/push-token", JSONObject().put("token", pushToken).toString(), deviceToken)
    }

    fun decide(requestId: String, decision: String, payload: String?) {
        val body = JSONObject().put("decision", decision)
        if (!payload.isNullOrBlank()) body.put("payload", payload)
        request("POST", "/v1/decision/$requestId", body.toString(), deviceToken)
    }

    /** 生成一次性配对码（10 分钟），给新机器的 `easyGet pair` 用。返回 码 to 过期时刻（epoch 毫秒）。 */
    fun createPairCode(): Pair<String, Long> {
        val raw = request("POST", "/v1/device/pair-code", "{}", deviceToken)
        val o = JSONObject(raw)
        val expiresAt = runCatching { java.time.Instant.parse(o.getString("expires_at")).toEpochMilli() }.getOrDefault(0L)
        return o.getString("code") to expiresAt
    }

    /** 已配对设备列表（不含令牌本体）。 */
    fun devices(): List<PairedDevice> {
        val raw = request("GET", "/v1/device/devices", null, deviceToken)
        val arr = JSONObject(raw).optJSONArray("devices") ?: JSONArray()
        return buildList {
            for (i in 0 until arr.length()) {
                val o = arr.getJSONObject(i)
                add(
                    PairedDevice(
                        id = o.optString("id"),
                        name = o.optString("name"),
                        createdAt = o.optString("created_at"),
                        lastUsedAt = if (o.isNull("last_used_at")) "" else o.optString("last_used_at"),
                        expiresAt = o.optString("expires_at"),
                        current = o.optBoolean("current"),
                    )
                )
            }
        }
    }

    /** 撤销一台设备（可以是当前这台 = 登出）。 */
    fun revokeDevice(id: String) {
        request("POST", "/v1/device/revoke", JSONObject().put("id", id).toString(), deviceToken)
    }

    /** 给一台设备改名（同租户内，可以是当前这台）。 */
    fun renameDevice(id: String, name: String) {
        request("POST", "/v1/device/rename", JSONObject().put("id", id).put("name", name).toString(), deviceToken)
    }

    /** 给当前设备续期 180 天，返回新的过期时间。 */
    fun renewDevice(): String {
        val raw = request("POST", "/v1/device/renew", "{}", deviceToken)
        return JSONObject(raw).optString("expires_at")
    }

    private fun request(method: String, path: String, body: String?, bearer: String): String {
        val url = URL(brokerUrl.trimEnd('/') + path)
        val conn = (url.openConnection() as HttpURLConnection).apply {
            requestMethod = method
            connectTimeout = 15000
            readTimeout = 15000
            setRequestProperty("Authorization", "Bearer $bearer")
            if (body != null) {
                doOutput = true
                setRequestProperty("Content-Type", "application/json")
            }
        }
        if (body != null) {
            OutputStreamWriter(conn.outputStream, Charsets.UTF_8).use { it.write(body) }
        }
        val code = conn.responseCode
        val stream = if (code in 200..299) conn.inputStream else conn.errorStream
        val text = stream.bufferedReader().readText()
        if (code !in 200..299) throw RuntimeException("Broker $code $text")
        return text
    }
}
