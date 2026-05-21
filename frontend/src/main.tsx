import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import "./index.css";
import App from "./App";

// React mount'idan oldin theme'ni qo'llab qo'yish — flash of unstyled / wrong
// theme bo'lmasligi uchun. localStorage'da yo'q bo'lsa OS prefersini olamiz.
(() => {
  const stored = localStorage.getItem("theme");
  const theme =
    stored === "light" || stored === "dark"
      ? stored
      : window.matchMedia?.("(prefers-color-scheme: light)").matches
      ? "light"
      : "dark";
  document.documentElement.setAttribute("data-theme", theme);
})();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>
);
