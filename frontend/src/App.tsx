import { Routes, Route } from "react-router-dom";
import Layout from "./components/Layout";
import LogViewerPage from "./pages/LogViewerPage";
import ExportPage from "./pages/ExportPage";
import JobsPage from "./pages/JobsPage";
import SettingsPage from "./pages/SettingsPage";

function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<LogViewerPage />} />
        <Route path="/export" element={<ExportPage />} />
        <Route path="/jobs" element={<JobsPage />} />
        <Route path="/settings" element={<SettingsPage />} />
      </Route>
    </Routes>
  );
}

export default App;
