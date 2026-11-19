// HeroUI v2.8 的类名工厂分散在各子包 dist（全部是 .mjs chunk），
// 且子包嵌套安装于 @heroui/react/node_modules/@heroui/* 内（npm 未提升到顶层）。
// 因此 content 必须用 ** 通配覆盖嵌套子包，且扩展名必须包含 mjs，
// 否则 Modal 遮罩层等运行时拼接的 fixed/inset-0/z-50 类不会生成，
// 弹窗会以 static 定位排进文档流而"消失"在视口外。
import { heroui } from '@heroui/react';

export default {
  content: [
    './index.html',
    './src/**/*.{js,ts,jsx,tsx}',
    './node_modules/@heroui/**/*.{js,mjs,ts,jsx,tsx}',
  ],
  theme: { extend: {} },
  darkMode: 'class',
  plugins: [heroui()],
};
