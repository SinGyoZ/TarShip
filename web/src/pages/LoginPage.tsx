import { useEffect, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { addToast, Button, Card, CardBody, Input, Progress } from '@heroui/react';
import { api, type AuthStatus } from '../api';

export default function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const queryClient = useQueryClient();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');

  const status = useQuery({
    queryKey: ['auth-status'],
    queryFn: () => api<AuthStatus>('/api/auth/status'),
  });

  useEffect(() => {
    document.title = 'TarShip 控制台 - 登录';
  }, []);

  const onAuthSuccess = () => {
    // 登录/初始化成功后刷新会话缓存并跳回原目标页
    queryClient.invalidateQueries({ queryKey: ['me'] });
    const from = (location.state as { from?: string })?.from ?? '/';
    navigate(from, { replace: true });
  };

  const setup = useMutation({
    mutationFn: () =>
      api<{ username: string }>('/api/auth/setup', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      }),
    onSuccess: onAuthSuccess,
    onError: (e: Error) => addToast({ title: '初始化失败', description: e.message, color: 'danger' }),
  });

  const login = useMutation({
    mutationFn: () =>
      api<{ username: string }>('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      }),
    onSuccess: onAuthSuccess,
    onError: (e: Error) => addToast({ title: '登录失败', description: e.message, color: 'danger' }),
  });

  if (status.isPending) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Progress size="sm" isIndeterminate className="max-w-xs" aria-label="加载中" />
      </div>
    );
  }

  const isSetup = !(status.data?.initialized ?? true);
  const submitting = login.isPending || setup.isPending;

  // 初始化时校验两次密码一致；密码强度交由后端校验（≥8 位）
  const submit = () => {
    if (isSetup && password !== confirmPassword) {
      addToast({ title: '两次输入的密码不一致', color: 'danger' });
      return;
    }
    if (isSetup) {
      setup.mutate();
    } else {
      login.mutate();
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <Card isBlurred className="w-full max-w-sm border border-default-100">
        <CardBody className="gap-5 p-8">
          <div className="text-center">
            <span aria-hidden className="text-4xl">🚢</span>
            <h1 className="mt-2 text-2xl font-bold text-foreground">TarShip</h1>
            <p className="mt-1 text-sm text-foreground-500">
              {isSetup ? '首次使用，请创建管理员账号' : '目录备份运输系统控制台'}
            </p>
          </div>

          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
          >
            <Input
              label="用户名"
              labelPlacement="outside"
              placeholder={isSetup ? '设置管理员用户名' : '输入用户名'}
              value={username}
              onValueChange={setUsername}
              autoComplete="username"
              isRequired
            />
            <Input
              type="password"
              label="密码"
              labelPlacement="outside"
              placeholder={isSetup ? '至少 8 位' : '输入密码'}
              value={password}
              onValueChange={setPassword}
              autoComplete={isSetup ? 'new-password' : 'current-password'}
              isRequired
            />
            {isSetup && (
              <Input
                type="password"
                label="确认密码"
                labelPlacement="outside"
                placeholder="再输入一次"
                value={confirmPassword}
                onValueChange={setConfirmPassword}
                autoComplete="new-password"
                isRequired
              />
            )}
            <Button type="submit" color="primary" isLoading={submitting}>
              {isSetup ? '完成初始化' : '登录'}
            </Button>
          </form>

          {status.data?.version && (
            <p className="text-center text-xs text-foreground-400">Hub v{status.data.version}</p>
          )}
        </CardBody>
      </Card>
    </div>
  );
}
