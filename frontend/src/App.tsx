import { Routes, Route } from "react-router-dom";
import Layout from "./components/Layout";
import ExportPage from "./pages/ExportPage";
import QueriesPage from "./pages/QueriesPage";
import JobsPage from "./pages/JobsPage";
import SettingsPage from "./pages/SettingsPage";

function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<ExportPage />} />
        <Route path="/queries" element={<QueriesPage />} />
        <Route path="/jobs" element={<JobsPage />} />
        <Route path="/settings" element={<SettingsPage />} />
      </Route>
    </Routes>
  );
}

export default App;
