import { Navigate, Route, Routes } from "react-router-dom";
import { RequireAuth } from "./auth";
import Layout from "./components/Layout";
import DashboardPage from "./pages/DashboardPage";
import LoginPage from "./pages/LoginPage";
import PlaceholderPage from "./pages/PlaceholderPage";
import ServersPage from "./pages/ServersPage";
import SetupPage from "./pages/SetupPage";

export default function App() {
  return (
    <Routes>
      <Route path="/setup" element={<SetupPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route element={<RequireAuth />}>
        <Route element={<Layout />}>
          <Route index element={<DashboardPage />} />
          <Route path="servers" element={<ServersPage />} />
          <Route path="tasks" element={<PlaceholderPage title="备份任务" />} />
          <Route path="backups" element={<PlaceholderPage title="备份库" />} />
          <Route path="storages" element={<PlaceholderPage title="存储器" />} />
          <Route path="settings" element={<PlaceholderPage title="设置" />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Route>
    </Routes>
  );
}
