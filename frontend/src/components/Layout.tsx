import { NavLink, Outlet } from "react-router-dom";
import "./Layout.css";

export default function Layout() {
  return (
    <div className="app">
      <nav className="sidebar">
        <h2>LogDownloader</h2>
        <NavLink to="/" end>
          Export
        </NavLink>
        <NavLink to="/queries">Saved Queries</NavLink>
        <NavLink to="/jobs">Active Jobs</NavLink>
        <NavLink to="/settings">Settings</NavLink>
      </nav>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}
