import XCTest

/// iOS 端到端验收：对齐安卓验收 skill 的用例矩阵。
/// 测试前外部脚本在 0.0.0.0:8787 起 mock Broker（真机经 LAN IP 访问）（pairing-token=e2e-pair-token），
/// 并循环 `simctl spawn … notifyutil -p com.apple.BiometricKit_Sim.pearl.match`
/// 兜底建库/解锁时的 Face ID 闸。批准后一律走「用密码批准」，全确定。
final class EasyUnlockerUITests: XCTestCase {

    static let broker = "http://127.0.0.1:8787" // 模拟器走宿主机回环；真机验收改成跑 Broker 那台机器的 LAN IP
    static let pairToken = "e2e-pair-token" // pairing token 既配对也能发请求
    static let unlockPw = "happy" // 真机库当前密码；改了要同步这里
    static let sealKey = "O_NBb8UJowePx5TyCKFinsv9CGxmbf3BLZLsk7_peY4" // X25519 raw-url b64
    static var tag = "ui-req" // 每个用例覆盖成唯一值，pending 卡只认它
    /// 请求方设备 token——经「生成配对码→pair-claim」拿到，和 app 同租户。
    /// pairing-token 直发的请求归 default 租户，app 永远看不到。
    static var requesterToken = ""

    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        Self.drainRequestTasks()
        app = XCUIApplication()
        app.launchArguments = ["-ui-testing", "YES"] // 关生物识别；带值形式让 NSArgumentDomain 也能解析
        app.launchEnvironment["UI_TESTING"] = "1" // 真机启动通道不保证传 argv，双保险
        app.launch()
        // 系统弹窗（通知授权等）出现时统一处理
        _ = addUIInterruptionMonitor(withDescription: "system alert") { alert in
            for label in ["允许", "Allow", "OK", "好", "以后", "以后再说", "不存储", "不保存", "Not Now"] {
                let b = alert.buttons[label]
                if b.exists { b.tap(); return true }
            }
            return false
        }
        sleep(2)
        app.tap() // 触发 monitor
        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        for _ in 0..<3 {
            let allow = springboard.buttons["允许"]
            // monitor 可能已先一步点掉弹窗——用坐标点按，按钮消失也不炸断言
            if allow.exists { allow.coordinate(withNormalizedOffset: .zero).tap(); break }
            sleep(1)
        }
    }

    // MARK: - 小工具

    func waitText(_ text: String, timeout: TimeInterval = 8) -> XCUIElement {
        let pred = NSPredicate(format: "label CONTAINS[c] %@", text)
        let el = app.descendants(matching: .any).matching(pred).firstMatch
        XCTAssertTrue(el.waitForExistence(timeout: timeout), "等待文本「\(text)」")
        return el
    }

    func exists(_ text: String, timeout: TimeInterval = 2) -> Bool {
        app.descendants(matching: .any)
            .matching(NSPredicate(format: "label CONTAINS[c] %@", text))
            .firstMatch.waitForExistence(timeout: timeout)
    }

    @discardableResult
    func tapButton(_ label: String, timeout: TimeInterval = 8) -> Bool {
        let el = app.buttons.matching(NSPredicate(format: "label CONTAINS[c] %@", label)).firstMatch
        XCTAssertTrue(el.waitForExistence(timeout: timeout), "等按钮「\(label)」")
        el.tap()
        return true
    }

    /// 键盘会挡住 tab bar（hit point -1,-1 点不中）——切 tab 前统一收键盘再点。
    /// iOS 26 真机上键盘在 AX 里是 Other/Key 不是 Keyboard 类型，`app.keyboards` 查不到；
    /// 用「换行/Return 键存在」判定。收键盘：列表区中段下滑（interactive dismiss），
    /// 不在底缘滑——底缘会误触发 home 指示器把 app 切后台（=> 上锁）。
    /// 点完必须验证真切过去了（verify = 目标页 nav 标题，以 staticText 精确匹配，
    /// 避开 tab 按钮同名 label 的误报）；sheet 关闭动画中的点击会被吞，重试 3 次。
    func tapTab(_ label: String, verify title: String? = nil) {
        for attempt in 1...3 {
            if app.buttons["换行"].exists || app.buttons["Return"].exists || app.keyboards.firstMatch.exists {
                // 列表中段短下滑收键盘（scrollDismissesKeyboard）
                let start = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.35))
                start.press(forDuration: 0.05, thenDragTo: app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)))
                sleep(1)
            }
            let btn = app.tabBars.buttons[label]
            if attempt == 1 {
                btn.tap()
            } else {
                // hit point 算不出来（{-1,-1}）时，坐标点按兜底——直接落在 frame 中心
                btn.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            }
            if let title {
                if app.staticTexts[title].waitForExistence(timeout: 3) { return }
                continue // 没切过去——重试
            }
            return
        }
    }


    /// sheet 还活着时先收键盘——sheet 关掉后其 TextField 可能残留在屏外持焦，
    /// 主窗口的手势/点击都够不着它（iOS 26 实测），必须在关闭动作前收。
    /// 收法：点键盘自带的「换行/Return」（单行 field 的 onSubmit 会 resign）；
    /// 残留兜底再补一次中段下滑。
    func dismissKbInSheet() {
        for key in ["换行", "Return", "done", "完成", "return", "前往", "go"] {
            let b = app.buttons[key]
            if b.exists { b.tap(); sleep(1); break }
        }
        if app.buttons["换行"].exists || app.buttons["Return"].exists {
            let start = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.35))
            start.press(forDuration: 0.05, thenDragTo: app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)))
            sleep(1)
        }
    }

    func anyElement(_ label: String) -> XCUIElement {
        app.descendants(matching: .any)
            .matching(NSPredicate(format: "label == %@", label))
            .firstMatch
    }

    /// Broker /v1/request 是长连接——后台发、等 UI 操作完再看响应。
    final class RequestTask {
        let sem = DispatchSemaphore(value: 0)
        var json: [String: Any]?
        var http: URLSessionDataTask?
    }

    /// 没等完的请求任务——tearDown/setUp 里 cancel，broker 端随即撤掉 pending，不污染下个用例。
    static var requestTasks: [RequestTask] = []

    static func drainRequestTasks() {
        for t in requestTasks { t.http?.cancel() }
        requestTasks.removeAll()
    }

    func postRequest(_ body: [String: Any]) -> RequestTask {
        let task = RequestTask()
        Self.requestTasks.append(task)
        Thread.detachNewThread {
            var req = URLRequest(url: URL(string: "\(Self.broker)/v1/request")!)
            req.httpMethod = "POST"
            req.setValue("Bearer \(Self.requesterToken)", forHTTPHeaderField: "Authorization")
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try? JSONSerialization.data(withJSONObject: body)
            req.timeoutInterval = 150
            let http = URLSession.shared.dataTask(with: req) { data, _, err in
                if let data, let o = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                    task.json = o
                } else {
                    task.json = ["error": err?.localizedDescription ?? "nil"]
                }
                task.sem.signal()
            }
            task.http = http
            http.resume()
        }
        return task
    }

    func awaitBroker(_ task: RequestTask, timeout: TimeInterval = 60) {
        XCTAssertEqual(task.sem.wait(timeout: .now() + timeout), .success, "Broker 响应超时")
    }

    // MARK: - 状态推进

    /// 建库页展示的恢复码——抓到后所有解锁都走它，确定、不靠生物信号。
    static var recoveryCode: String?

    func captureRecoveryCode() {
        if Self.recoveryCode != nil { return }
        let pred = NSPredicate(format: "label MATCHES %@", "[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}")
        let el = app.staticTexts.matching(pred).firstMatch
        if el.waitForExistence(timeout: 3) {
            Self.recoveryCode = el.label
        }
    }

    /// 三种初始态：未建库（建库+FaceID信号兜底）/ 已锁（恢复码或密码）/ 已解锁。
    func ensureVault() {
        captureRecoveryCode()
        if app.buttons["创建保险库"].waitForExistence(timeout: 4) {
            app.buttons["创建保险库"].tap()
        }
        // 已锁：输入框藏在「用解锁密码/用恢复码」后面，先展开再输
        let pwField = app.secureTextFields["unlock.pw"]
        let recField = app.textFields["unlock.recovery"]
        if !pwField.waitForExistence(timeout: 2) && app.buttons["用解锁密码"].exists {
            app.buttons["用解锁密码"].tap()
        }
        if pwField.waitForExistence(timeout: 2) {
            pwField.tap()
            pwField.typeText(Self.unlockPw)
            app.buttons["确认"].tap()
        } else {
            if !recField.waitForExistence(timeout: 2) && app.buttons["用恢复码"].exists {
                app.buttons["用恢复码"].tap()
            }
            if let code = Self.recoveryCode, recField.waitForExistence(timeout: 2) {
                recField.tap()
                recField.typeText(code)
                app.buttons["解锁"].tap()
            }
        }
        if !exists("条目", timeout: 45) {
            let shot = XCTAttachment(screenshot: app.screenshot())
            shot.lifetime = .keepAlways
            add(shot)
            XCTFail("等「条目」超时，AX: \(app.debugDescription.prefix(1500))")
        }
    }

    func addItem(name: String, secret: String, note: String = "") {
        // vault 跨用例/重试持久化——条目已存在就别重复加（重名会报错卡 sheet）
        if app.staticTexts[name].waitForExistence(timeout: 3) { return }
        let add = anyElement("添加条目")
        XCTAssertTrue(add.waitForExistence(timeout: 6), "找不到添加条目按钮")
        add.tap()
        let nameField = app.textFields["edit.name"]
        XCTAssertTrue(nameField.waitForExistence(timeout: 4))
        nameField.tap()
        nameField.typeText(name)
        let secretField = app.textFields["edit.secret"]
        secretField.tap()
        secretField.typeText(secret)
        if !note.isEmpty {
            let noteField = app.textViews["edit.note"].exists ? app.textViews["edit.note"] : app.textFields["edit.note"]
            noteField.tap()
            noteField.typeText(note)
        }
        dismissKbInSheet() // 收键盘再保存——否则焦点死在关闭的 sheet 里盖住 tab bar
        app.buttons["保存"].tap()
        XCTAssertTrue(waitText(name, timeout: 6).exists)
    }

    /// 设临时解锁密码（若已设置则跳过）。批准全部走密码——全自动的关键。
    func setUnlockPassword() {
        tapTab("设置", verify: "设置")
        tapButton("解锁密码")
        if exists("已设置") {
            app.buttons["设置"].firstMatch.tap()
            return
        }
        let p1 = app.secureTextFields["pw.new"]
        XCTAssertTrue(p1.waitForExistence(timeout: 4))
        p1.tap()
        p1.typeText(Self.unlockPw)
        let p2 = app.secureTextFields["pw.repeat"]
        p2.tap()
        p2.typeText(Self.unlockPw)
        dismissKbInSheet()
        tapButton("保存")
        XCTAssertTrue(waitText("已设置", timeout: 6).exists)
        app.buttons["设置"].firstMatch.tap()
    }

    func pairGateway() {
        tapTab("设置", verify: "设置")
        tapButton("网关")
        // 每次都重新配对拿新 token——Broker 重置后旧 device_token 会失效，
        // 同 URL 再配只是更新条目，幂等。
        tapButton("添加网关")
        let urlField = app.textFields["pair.url"]
        XCTAssertTrue(urlField.waitForExistence(timeout: 4))
        urlField.tap()
        urlField.typeText(Self.broker)
        let tokenField = app.secureTextFields["pair.token"]
        tokenField.tap()
        tokenField.typeText(Self.pairToken)
        dismissKbInSheet()
        app.buttons["配对"].tap()
        // 「8787」会匹配到 sheet 里 url 字段的值——必须等 sheet 真正关掉。
        // 关掉后 网关列表行的 url 文本才出现；sheet 还在时返回键/边缘手势都失效。
        _ = waitText("8787", timeout: 10)
        let sheetGone = NSPredicate(format: "exists == false")
        let cancelBtn = app.buttons["取消"]
        expectation(for: sheetGone, evaluatedWith: cancelBtn)
        waitForExpectations(timeout: 8)
        // 配对完直接重启回主 tab——比依赖 nav 返回按钮稳（iOS 26 sheet 关闭后
        // 返回按钮 hit point 坏掉是已知现象）。
        app.terminate()
        app.launch()
        ensureVault()
        claimRequesterIfNeeded()
    }

    /// 走真实「生成配对码 → pair-claim」拿同租户 requester token，只领一次。
    /// 顺带验收设备管理页的配对码生成。
    func claimRequesterIfNeeded() {
        if !Self.requesterToken.isEmpty { return }
        tapTab("设置", verify: "设置")
        tapButton("设备")
        let codeEl = app.staticTexts["device.paircode"]
        if !codeEl.waitForExistence(timeout: 3) {
            var gen = app.buttons["添加设备（生成配对码）"]
            if !gen.exists { gen = app.buttons["重新生成配对码"] }
            XCTAssertTrue(gen.waitForExistence(timeout: 5), "设备页找不到生成配对码按钮")
            gen.tap()
            XCTAssertTrue(codeEl.waitForExistence(timeout: 10), "配对码没出现")
        }
        Self.requesterToken = claimPairCode(codeEl.label)
        app.terminate()
        app.launch()
        ensureVault()
    }

    /// POST /v1/device/pair-claim 用配对码换 requester 设备 token（同步 HTTP）。
    func claimPairCode(_ code: String) -> String {
        var req = URLRequest(url: URL(string: "\(Self.broker)/v1/pair/claim")!)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try? JSONSerialization.data(withJSONObject: ["code": code, "name": "e2e-cli"])
        var token = ""
        let sem = DispatchSemaphore(value: 0)
        URLSession.shared.dataTask(with: req) { data, _, _ in
            if let data, let o = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                token = o["device_token"] as? String ?? ""
            }
            sem.signal()
        }.resume()
        XCTAssertEqual(sem.wait(timeout: .now() + 15), .success, "pair-claim 超时")
        XCTAssertFalse(token.isEmpty, "pair-claim 没返回 device_token")
        return token
    }

    /// 批准当前请求：优先密码（确定），无密码则 Face ID（靠信号）。
    func approveByPassword() {
        let pwBtn = app.buttons["用密码批准"]
        if pwBtn.waitForExistence(timeout: 5) {
            pwBtn.tap()
            let f = app.secureTextFields["approve.pw"]
            XCTAssertTrue(f.waitForExistence(timeout: 4))
            f.tap()
            f.typeText(Self.unlockPw)
            dismissKbInSheet()
        tapButton("确认批准")
        } else {
            let approve = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'pending.approve.'")).firstMatch
            XCTAssertTrue(approve.waitForExistence(timeout: 8))
            approve.tap() // Face ID 路线，靠 pearl.match 信号
        }
    }

    func openPending() {
        tapTab("待批准", verify: "待批准")
        XCTAssertTrue(waitText(Self.tag, timeout: 25).exists)
    }

    // MARK: - 验收用例（对齐安卓验收矩阵）

    /// A1 建库 → 空库 → 加条目 → 详情看值 → 搜索过滤 → 设密码
    func testAVaultLifecycle() throws {
        ensureVault()
        addItem(name: "OPENAI_API_KEY", secret: "sk-test-e2e-0001", note: "e2e note")
        tapButton("OPENAI_API_KEY")
        XCTAssertTrue(waitText("sk-test-e2e-0001").exists)
        XCTAssertTrue(waitText("切到后台会自动上锁").exists)
        app.buttons["条目"].firstMatch.tap()
        let search = app.searchFields.firstMatch
        XCTAssertTrue(search.waitForExistence(timeout: 4))
        search.tap()
        search.typeText("XXXX")
        XCTAssertTrue(waitText("没有匹配的条目").exists)
        // 退出搜索态：点「关闭/取消」按钮收键盘+清搜索框，否则 tab bar 被挡住点不中
        let cancel = app.buttons.matching(NSPredicate(format: "label IN {'关闭','取消','Cancel'}")).firstMatch
        if cancel.waitForExistence(timeout: 3) { cancel.tap() }
        sleep(1)
        setUnlockPassword()
    }

    /// A2 配对网关 → 发请求 → 密码批准 → Broker 收 approved + v2 载荷 → 记录
    func testBApproveFlow() throws {
        ensureVault()
        addItem(name: "GH_TOKEN", secret: "ghp_e2e_secret_42")
        pairGateway()

        Self.tag = "ui-approve"
        let resp = postRequest([
            "item": "GH_TOKEN", "mode": "write", "seal_public_key": Self.sealKey, "purpose": "e2e approve",
            "ttl": 120, "requester": Self.tag, "delivery": "ephemeral",
        ])
        openPending()
        XCTAssertTrue(waitText(Self.tag).exists)
        XCTAssertTrue(waitText("完全匹配").exists)
        XCTAssertTrue(waitText("不落盘").exists)
        approveByPassword()
        XCTAssertTrue(waitText("已放行", timeout: 15).exists)
        awaitBroker(resp)
        XCTAssertEqual(resp.json?["status"] as? String, "approved", "\(resp.json ?? [:])")
        XCTAssertTrue((resp.json?["payload"] as? String ?? "").hasPrefix("v2."))
        tapTab("设置", verify: "设置")
        XCTAssertTrue(waitText("解锁密码", timeout: 8).exists) // 设置页就绪
        tapButton("批准记录")
        XCTAssertTrue(waitText("GH_TOKEN").exists)
        XCTAssertTrue(waitText("已放行").exists)
    }

    /// A3 拒绝 → Broker 返回 denied
    func testCDenyFlow() throws {
        ensureVault()
        pairGateway()
        Self.tag = "ui-deny"
        let resp = postRequest([
            "item": "GH_TOKEN", "mode": "write", "seal_public_key": Self.sealKey, "purpose": "e2e deny",
            "ttl": 120, "requester": Self.tag, "delivery": "ephemeral",
        ])
        openPending()
        tapButton("拒绝")
        XCTAssertTrue(waitText("已拒绝", timeout: 15).exists)
        awaitBroker(resp)
        XCTAssertEqual(resp.json?["status"] as? String, "denied", "\(resp.json ?? [:])")
    }

    /// A4 过期：不批准 → 请求从 pending 消失 → Broker 返回 expired
    func testDExpireFlow() throws {
        ensureVault()
        pairGateway()
        Self.tag = "ui-expire"
        let resp = postRequest([
            "item": "GH_TOKEN", "mode": "write", "seal_public_key": Self.sealKey, "purpose": "e2e expire",
            "ttl": 6, "requester": Self.tag, "delivery": "ephemeral",
        ])
        openPending()
        XCTAssertTrue(waitText("没有等待你的请求", timeout: 25).exists)
        awaitBroker(resp, timeout: 40)
        XCTAssertEqual(resp.json?["status"] as? String, "expired", "\(resp.json ?? [:])")
    }

    /// A5 #items 列表请求 → 密码批准 → Broker 收 approved + v2 载荷（名单在内）
    func testEListFlow() throws {
        ensureVault()
        pairGateway()
        Self.tag = "ui-list"
        let resp = postRequest([
            "item": "#items", "mode": "write", "seal_public_key": Self.sealKey, "purpose": "e2e list",
            "ttl": 120, "requester": Self.tag, "delivery": "ephemeral",
        ])
        openPending()
        XCTAssertTrue(waitText("只给名字").exists)
        approveByPassword()
        XCTAssertTrue(waitText("已放行", timeout: 15).exists)
        awaitBroker(resp)
        XCTAssertEqual(resp.json?["status"] as? String, "approved", "\(resp.json ?? [:])")
        XCTAssertTrue((resp.json?["payload"] as? String ?? "").hasPrefix("v2."))
    }
}
