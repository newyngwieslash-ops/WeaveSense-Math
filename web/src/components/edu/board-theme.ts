/**
 * 板书主题（DEV-072）：板书、字幕与图形标签统一使用随应用打包的固定字体，
 * 不从外部字体服务加载，不依赖学生设备是否安装中文字体。
 * 字体包 @fontsource-variable/noto-sans-sc 在 package.json 中精确锁定版本，
 * 完整性 hash 记录在 bun.lock，由 test/edu-board-theme.test.ts 校验。
 * 公式以服务端生成的纯文本呈现，不引入 KaTeX/MathJax 等运行时排版库。
 */
export const BOARD_THEME_VERSION = "board-theme-v1";
export const BOARD_FONT_PACKAGE = "@fontsource-variable/noto-sans-sc";
export const BOARD_FONT_VERSION = "5.3.1";
export const BOARD_FONT_FACE = "Noto Sans SC Variable";
export const BOARD_FONT_FAMILY = `"${BOARD_FONT_FACE}", "PingFang SC", "Noto Sans CJK SC", "Microsoft YaHei", sans-serif`;
export const boardFontStyle = { fontFamily: BOARD_FONT_FAMILY } as const;
