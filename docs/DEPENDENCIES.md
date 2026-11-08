# 依赖与补丁

Go 模块版本由 go.mod 和 go.sum 固定，完整对应源码在 vendor/。许可证汇总见 THIRD-PARTY-LICENSES.txt，补充告知在 third_party/NOTICE.md。

MyGo 固定为 v0.8.1，包含音频输出、Windows 绘制和测试工具的局部修改。补丁在 patches/mygo-v0.8.1-local.patch，当前 vendor 已应用，不需要再次应用。

重新生成 vendor 会覆盖这些修改。升级依赖时须核对补丁、更新来源哈希并运行音频、界面和应用测试；不能只修改 go.mod。

模块许可清单见 dependency-inventory.json，补充许可见 additional-notice-inventory.json，嵌入文件与源码告知分别见 embed-inventory.json、source-header-notices.json。vendor-source-hashes.json 记录当前第三方源码哈希。
