# GoPay OTP Forwarder

GoPay 场景使用的 Android OTP 通知转发器，把包含 OTP 的通知 POST 到 gpt-service 暴露的 webhook。

默认 webhook payload：

```json
{
  "otp": "123456",
  "source": "whatsapp"
}
```

服务端地址填现有 GPT/Web API 暴露的 OTP 提交 URL，例如：

```text
http://192.168.0.115:8081/local/gopay
```

Telegram 用户来源使用 `http://192.168.0.115:8081/tg:<user_id>/gopay`。

## 构建

```bash
cd channels/gopay/whatsapp-relay
./gradlew assembleDebug
```

也可以在构建时写入默认 webhook：

```bash
./gradlew assembleDebug -PdefaultWebhookUrl=http://192.168.0.115:8081/local/gopay
```

APK 输出：

```text
channels/gopay/whatsapp-relay/app/build/outputs/apk/debug/app-debug.apk
```

## 手机端设置

1. 安装 APK。
2. 打开应用，填写 webhook URL，保存。
3. 点击 `Open`，在系统通知访问设置里启用 `WhatsApp Forwarder`。
4. 允许通知权限；应用会显示一条低优先级常驻通知，用于提高后台存活率。
5. 点击 `Battery settings`，允许忽略电池优化；部分 ROM 还需要允许自启动、后台运行并锁定后台。
6. 点击 `Test`，服务端 gpt-service 日志应出现 OTP accepted。

说明：保活服务使用 `specialUse` 前台服务类型，适配 Android 15+ 的前台服务限制。开机广播会尝试重新绑定通知监听器并启动保活服务；如果厂商 ROM 拦截自启动，重启后手动打开一次应用即可恢复。

## 通知处理

候选 OTP 通知会直接 POST 到 webhook，并解析 `EXTRA_TEXT`、`EXTRA_BIG_TEXT`、`EXTRA_TEXT_LINES` 和 MessagingStyle messages，适配 WhatsApp 聚合通知。

实现参考了 ItsAzni/NotificationForwarder 的通知监听思路：https://github.com/ItsAzni/NotificationForwarder
