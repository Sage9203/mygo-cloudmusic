# 构建与发布

## Windows x64

需要 Git、PowerShell 和 Go 1.27.3。完整依赖已包含在 `vendor/` 中。

在仓库根目录运行：

```powershell
powershell -NoProfile -File scripts/build-windows.ps1
```

脚本会运行测试、静态检查和 Windows 构建，输出位于 `build/windows-amd64/`。首次打包可能需要下载安装器工具；`-Offline` 仅用于已准备好相关缓存的环境。

## 检查

账号交互测试使用模拟网络和临时目录，不需要真实账号。带 `LIVE` 的联网测试须由开发者主动启用。更多说明见 [测试](docs/TESTING.md)。

## 发布

确认对应提交的 GitHub Actions 通过后，从该提交创建版本标签，并提供便携包、完整对应源码和 SHA256 校验文件。预发布版本应在 Release 中勾选 Pre-release。

保留 LICENSE、第三方许可和必要的构建文件。发布包不要包含凭据、缓存、日志、个人截图或应用数据。尚未验证的设备、平台或安装流程应如实说明。

当前工作流负责检查和构建，不自动发布 Release。依赖版本及补丁说明见 [DEPENDENCIES.md](docs/DEPENDENCIES.md)。
