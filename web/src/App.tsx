import { Navigate, Route, Routes } from "react-router-dom";
import { RequireAuth } from "./auth";
import Layout from "./components/Layout";
import BackupsPage from "./pages/BackupsPage";
import DashboardPage from "./pages/DashboardPage";
import LoginPage from "./pages/LoginPage";
import PlaceholderPage from "./pages/PlaceholderPage";
import ServersPage from "./pages/ServersPage";
import SetupPage from "./pages/SetupPage";
import StoragesPage from "./pages/StoragesPage";
import TasksPage from "./pages/TasksPage";

export default function App() {
  return (
    <Routes>
      <Route path="/setup" element={<SetupPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route element={<RequireAuth />}>
        <Route element={<Layout />}>
          <Route index element={<DashboardPage />} />
          <Route path="servers" element={<ServersPage />} />
          <Route path="storages" element={<StoragesPage />} />
          <Route path="tasks" element={<TasksPage />} />
          <Route path="backups" element={<BackupsPage />} />
          <Route path="settings" element={<PlaceholderPage title="设置" />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Route>
    </Routes>
  );
}
