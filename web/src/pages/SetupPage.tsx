import { Button, FieldError, Input, Label, TextField } from "@heroui/react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import { api } from "../api";
import { BootError, FullPageSpinner, useBootstrap } from "../auth";
import { AuthShell } from "../components/AuthShell";
import { errorMessage } from "../api";

export default function SetupPage() {
  const boot = useBootstrap();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [token, setToken] = useState("");
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [localError, setLocalError] = useState<string | null>(null);

  const setup = useMutation({
    mutationFn: api.setup,
    onSuccess: (me) => {
      qc.setQueryData(["setup"], { initialized: true });
      qc.setQueryData(["me"], me);
      navigate("/", { replace: true });
    },
  });

  if (boot.loading) return <FullPageSpinner />;
  if (boot.error) return <BootError error={boot.error} onRetry={boot.retry} />;
  if (boot.initialized) return <Navigate to="/login" replace />;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (password !== confirm) {
      setLocalError("两次输入的密码不一致");
      return;
    }
    setLocalError(null);
    setup.mutate({ setup_token: token.trim(), username, password });
  };

  const error = localError ?? (setup.error ? errorMessage(setup.error) : null);

  return (
    <AuthShell title="创建管理员账号" description="首次启动时，Hub 会在日志里打印一次性的 setup token，只有能看到日志的人才能完成初始化。">
      <form onSubmit={submit} className="space-y-4">
        <TextField value={token} onChange={setToken} isRequired autoFocus autoComplete="off" name="setup_token">
          <Label>Setup token</Label>
          <Input className="font-mono" placeholder="从 Hub 启动日志中复制" />
          <FieldError />
        </TextField>
        <TextField value={username} onChange={setUsername} isRequired autoComplete="username" name="username" minLength={3} maxLength={32}>
          <Label>用户名</Label>
          <Input />
          <FieldError />
        </TextField>
        <TextField value={password} onChange={setPassword} isRequired type="password" autoComplete="new-password" name="password" minLength={8} maxLength={72}>
          <Label>密码（至少 8 位）</Label>
          <Input />
          <FieldError />
        </TextField>
        <TextField value={confirm} onChange={setConfirm} isRequired type="password" autoComplete="new-password" name="confirm">
          <Label>确认密码</Label>
          <Input />
          <FieldError />
        </TextField>
        {error ? (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        ) : null}
        <Button type="submit" fullWidth isPending={setup.isPending}>
          创建并登录
        </Button>
      </form>
    </AuthShell>
  );
}
