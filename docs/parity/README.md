# Python/Go 对照验证已退役

早期迁移使用这组工具比较 Python 与 Go 的任务目录、转写结构、笔记输入和纪要输出。桌面现在统一使用 Go，旧 Python 实现、对照脚本和对应 CI 已移除。

旧工具及其验证说明保留在 Git 历史的 `f2a059a` 中。它们不再是当前版本的验收入口。现行检查见[桌面开发说明](../../desktop/README.md#checks)和 [Go 核心说明](../../core-go/README.md)。

Go 的任务恢复、目录边界和 manifest 兼容性继续由现役 Go 与 Rust 测试检查。此项清理未重新运行云端对照，也未触发付费 API 请求。
