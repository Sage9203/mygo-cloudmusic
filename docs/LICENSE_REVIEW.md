# 扫描结果与分发范围

扫描工具可能分析整个上游模块，而应用只使用其中部分包。处理告警时须核对命中文件、构建依赖和实际发布内容；不能只凭模块名称批准或拒绝。

## golang.org/x/text v0.43.0

上游 `cases/map_test.go`、`unicode/norm/normalize_test.go` 和 `internal/testtext/text.go` 包含取自 Creative Commons 许可页面的测试文本，注明了 CC-BY-SA 1.0、2.0、2.5、3.0 等来源。

本项目的 vendor 仅包含 `internal/tag`、`internal/language`、`internal/language/compact`、`language` 及模块许可。这些测试文件和 `internal/testtext` 不在源码包中，也不参与播放器构建。保留的模块根许可证为 BSD 风格许可。

来源：[固定版本的测试文本](https://github.com/golang/text/blob/ac40b0f06c159afb5bfca4d1860f8b4b60dbc160/internal/testtext/text.go)。如扫描工具的命中路径与上述范围不同，应重新审查；不要全局放行 CC-BY-SA。

## AGPL-3.0-only

这是本项目自身声明的许可证，见根目录 LICENSE。扫描策略将其标为需要审查，不等于发现未声明的许可证。接受本项目的开源发布方式时，应仅针对本项目记录审核结论，并继续保留源码及对应分发义务。

## CPL-1.0 与 NSIS

`third_party/nsis/COPYING` 和 `THIRD-PARTY-LICENSES.txt` 保留了 NSIS 的完整许可告知，其中包括 LZMA 模块的 CPL-1.0 文本及链接例外。该告知不将本项目整体改为 CPL。

当前便携包不包含 NSIS 编译器或安装器；MyGo 的打包命令会使用 NSIS 生成单独的安装器。以后分发安装器时须单独核对组件、源码和许可要求。不要为消除扫描告警而删除必要告知。

## golang.org/x/net v0.62.0

固定到 v0.62.0 以避开 [GO-2026-6860](https://pkg.go.dev/vuln/GO-2026-6860) 与 [GO-2026-6866](https://pkg.go.dev/vuln/GO-2026-6866) 的受影响版本。两项公告涉及 `html` 包；播放器实际依赖的是 `publicsuffix`，vendor 中不包含 `html`。

更新模块版本、校验和和清单后，应对新提交重新扫描。历史版本的报告仍可能显示原来的告警；审核结论应注明所针对的提交。
