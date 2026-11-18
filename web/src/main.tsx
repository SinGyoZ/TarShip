import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { HeroUIProvider, ToastProvider } from '@heroui/react';
import App from './App';
import './main.css';

// staleTime 短一些：服务器在线状态本就低频变化，事件驱动 invalidate 是主要刷新来源
const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, staleTime: 5_000, refetchOnWindowFocus: false },
  },
});

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <HeroUIProvider>
        <ToastProvider placement="top-right" />
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </HeroUIProvider>
    </QueryClientProvider>
  </StrictMode>,
);
