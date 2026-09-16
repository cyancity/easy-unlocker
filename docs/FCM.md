# Google 推送（FCM）要走的流程

不是上架审核。个人 Google 账号即可。**FCM 现在是唯一的推送通道。**

## 你要在控制台点的（大约 10 分钟）

1. 打开 [Firebase Console](https://console.firebase.google.com/) 登录。
2. **添加项目**，名字随意（例如 `easy-unlocker`）。Google Analytics 可关。
3. 项目里 **添加应用 → Android**  
   - 包名必须是：`io.github.cyancity.easyunlocker`  
   - 下载 `google-services.json`，放到本仓库 **`android/app/google-services.json`**（已 gitignore，不要提交）。
4. 项目设置 → **服务账号** → 生成新私钥，得到一份 JSON。放到服务器（不要进 git）：

```bash
# 在自己终端，不要把 JSON 贴进聊天
scp fcm-service-account.json your-server:~/easy-unlocker/data/fcm.json
ssh your-server 'chmod 600 ~/easy-unlocker/data/fcm.json'
```

5. `.env` 增加一行（仍不要打印值）：

```
EASY_UNLOCKER_FCM_CREDENTIALS=/home/ubuntu/easy-unlocker/data/fcm.json
```

然后重启 Broker。App 重装一次以带上 `google-services.json`。配对后会把 FCM 设备令牌登记到 Broker。

没有这份 JSON 时：App 照样能用，**前台靠轮询**；后台系统通知要等上面 1–5 做完。启动 Broker 时 `--fcm-credentials`（或 `EASY_UNLOCKER_FCM_CREDENTIALS`）指到这份 JSON 即可；没有其它推送通道可退。
