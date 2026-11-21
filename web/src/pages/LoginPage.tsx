import { Button, FieldError, Input, Label, TextField } from "@heroui/react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import { api, errorMessage } from "../api";
import { BootError, FullPageSpinner, useBootstrap } from "../auth";
import { AuthShell } from "../components/AuthShell";

export default function LoginPage() {
  const boot = useBootstrap();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const login = useMutation({
    mutationFn: api.login,
    onSuccess: (me) => {
      qc.setQueryData(["me"], me);
      navigate("/", { replace: true });
    },
  });

  if (boot.loading) return <FullPageSpinner />;
  if (boot.error) return <BootError error={boot.error} onRetry={boot.retry} />;
  if (!boot.initialized) return <Navigate to="/setup" replace />;
  if (boot.me) return <Navigate to="/" replace />;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    login.mutate({ username, password });
  };

  return (
    <AuthShell title="登录 TarShip">
      <form onSubmit={submit} className="space-y-4">
        <TextField value={username} onChange={setUsername} isRequired autoFocus autoComplete="username" name="username">
          <Label>用户名</Label>
          <Input />
          <FieldError />
        </TextField>
        <TextField value={password} onChange={setPassword} isRequired type="password" autoComplete="current-password" name="password">
          <Label>密码</Label>
          <Input />
          <FieldError />
        </TextField>
        {login.error ? (
          <p role="alert" className="text-sm text-danger">
            {errorMessage(login.error)}
          </p>
        ) : null}
        <Button type="submit" fullWidth isPending={login.isPending}>
          登录
        </Button>
      </form>
    </AuthShell>
  );
}
