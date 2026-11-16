图标使用红色 `#EA403F` 和白色 `#FFFFFF`，PNG 为透明背景，构建与运行都不需要 SVG 渲染库。

- 播放/暂停按用户提供的 SVG 形状处理为红底白色符号。
- 其余控件基于 Lucide Static 0.468.0 的圆角 SVG，许可见 LICENSE-lucide。
- 修改 SVG 后，可安装 CairoSVG 并运行 `python hack/render_playback_icons.py` 重新生成 PNG。
- PNG 在 Kitty 首次显示某个控件状态时编码上传，后续只在位置/状态变化时更新摆放。
- GNOME Terminal 或图像输出失败时使用相同点击布局的字符控件。
