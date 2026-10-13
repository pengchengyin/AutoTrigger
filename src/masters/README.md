# 模板母版

此目录只接收真实安装器截图裁剪得到的 100% DPI、浅色主题、8-bit 灰度母版。
采集方法见 `docs/图像模板制作说明.md`。没有截图时不合成假模板来凑数量。

每个 PNG 旁提供同名 `.source.json`：

```json
{"screenshot":"screenshots/inno-next.png","bounds":{"left":300,"top":220,"right":388,"bottom":246},"dpi":100,"theme":"light","installer":"Inno Setup","capture_origin":"real_installer"}
```

`screenshot` 相对母版根目录。必须保留原始截图供模板回匹配。
母版命名：`<family>__<action>__<lang>__100__light.png`。
执行 `go run ./tools/template-gen --masters masters --out internal/resources/assets/builtin/templates`。
发布模式构建要求至少 50 张母版和 180 个有效变体；物料不足会明确失败。
