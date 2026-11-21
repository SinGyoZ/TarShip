import { QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { ApiError } from "./api";
import App from "./App";
import "./index.css";

// 跟随系统的深浅色偏好；HeroUI 通过 html 上的 dark 类与 data-theme 切换主题。
const media = window.matchMedia("(prefers-color-scheme: dark)");
const applyTheme = () => {
  document.documentElement.classList.toggle("dark", media.matches);
  document.documentElement.dataset.theme = media.matches ? "dark" : "light";
};
applyTheme();
media.addEventListener("change", applyTheme);

const queryClient = new QueryClient({
  queryCache: new QueryCache({
    // 任何请求返回 401（会话过期），都把登录状态置空，路由守卫会跳转到登录页。
    onError: (error) => {
      if (error instanceof ApiError && error.status === 401) {
        queryClient.setQueryData(["me"], null);
      }
    },
  }),
  defaultOptions: { queries: { retry: false, refetchOnWindowFocus: true } },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
