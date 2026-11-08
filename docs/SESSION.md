# 登录与会话状态的归属

实现集中在 internal/netease/session_store.go、qr_login.go 和 credential_store.go。

## 状态由谁负责

- sessionManager 拥有当前账号 CookieJar、会话 revision、凭据存储和串行保存锁。生产默认实例不在运行时替换；测试可创建独立实例。
- QRLogin 拥有本次扫码的 context、取消函数和私有 CookieJar，并在创建时绑定所属 sessionManager。扫码返回 803 后还需要成功查询有效账号，再由 UI 接受。
- app.login.gen 表示某一次对话框流程；关闭或重开后旧回调不能改变新对话框。app.accountEpoch 表示 UI 接受的账号状态，防止启动时的旧账号查询覆盖新登录。这两者由 UI 线程修改，不能用 session revision 替代。

## 正常过程

创建 QRLogin -> 获取 key -> 轮询 -> 查询账号 -> UI 检查当前流程编号 -> Commit 接入内存并返回凭据保存任务 -> 工作线程执行 persist。

Commit 不访问系统凭据存储；只有当前有效 UI 流程可以调用它。返回的任务捕获当次不可变的 Cookie 快照与 revision，保存失败可重试。Cancel 取消私有扫码任务，不撤销已经提交的账号。保存失败保留当前内存登录态并提供重试，界面不能把失败显示为已经保存。

## 取消、退出与迟到结果

关闭/重开登录：取消旧 QRLogin，旧响应仅影响它自己的私有 CookieJar，旧 UI 回调通过流程检查被丢弃。

退出：BeginLogout 立即清空内存并增加 revision，返回的工作函数先清理凭据，再使用旧 CookieJar 的独立副本请求远程退出。调用者必须把返回函数交给工作线程执行并处理清理失败。

请求发送前，在同一把 mu 下取得 Cookie 和 revision；响应只有在 revision 未变化时才能写回。普通请求按请求域保存 Cookie，eapi 显式将登录 Cookie 归入站点域。封面 CDN 的 Cookie 不得变成站点登录 Cookie。

## 锁和持久化

mu 保护账号代际和 Cookie 组合操作；CookieJar 自身支持并发访问。网络请求和系统凭据 I/O 均不在 mu 下执行。

storeMu 串行化凭据写入/删除。持有 storeMu 后只短暂取得 mu 检查 revision；禁止持有 mu 等待 storeMu。保存前后的 revision 检查可以发现过期写入；若一次保存已经开始，随后退出产生的清理任务会排在该保存后执行。调用方必须执行返回的保存/清理任务，不能认为内存变化代表磁盘操作已经完成。

恢复凭据先记录 revision，读取完成后再检查。期间发生登录或退出时，旧恢复结果被丢弃。

## 测试与扩展边界

离线测试覆盖扫码取消、过期提交、保存失败重试、迟到网络 Cookie、旧保存任务、退出与在途保存交错、恢复期间退出、会话对象归属及 CDN Cookie 域。

普通测试不读取用户登录凭据。联网测试须明确启用，检查方法见 [TESTING.md](TESTING.md)。

目前保留包级账号 API 作为应用入口，不将每个接口都改成依赖注入框架。下一次确实需要多个同时存在的账号客户端时，再扩展调用边界。

