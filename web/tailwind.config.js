// HeroUI v2 需要 Tailwind 扫描其主题包内的类名工厂产物
import { heroui } from '@heroui/react';

export default {
  content: [
    './index.html',
    './src/**/*.{js,ts,jsx,tsx}',
    './node_modules/@heroui/theme/dist/**/*.{js,ts,jsx,tsx}',
  ],
  theme: { extend: {} },
  darkMode: 'class',
  plugins: [heroui()],
};
