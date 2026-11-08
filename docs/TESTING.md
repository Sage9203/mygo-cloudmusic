# 测试

普通检查由 scripts/build-windows.ps1 执行，包括应用、MyGo 音频和界面测试，以及模拟网络的账号交互测试。真实账号联网测试默认关闭。

账号交互测试在临时源码副本中排队执行异步完成通知，不访问真实账号，也不增加程序运行时开关：

```powershell
powershell -NoProfile -File scripts/check-account-callbacks.ps1
```

具备兼容的 MinGW-w64 编译器时，可运行 scripts/check-race.ps1；界面软件渲染在竞态检测下较慢，脚本保留15分钟上限。

发布前还应实际检查：扫码与取消、关闭重开后的登录保存、切歌、暂停继续、拖动进度、在其他客户端修改歌单后的刷新，以及和其他应用同时发声。安装/升级/卸载需要独立验收，编译成功不能替代实际运行测试。
