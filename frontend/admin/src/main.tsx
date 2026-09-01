import React from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./styles.css";

function clearLegacyAuthStorage() {
  try {
    localStorage.removeItem("hostctl-token");
    localStorage.removeItem("hostctl-admin-token");
    sessionStorage.removeItem("hostctl-token");
    sessionStorage.removeItem("hostctl-admin-token");
  } catch {
    // Storage may be unavailable in privacy-restricted browser contexts.
  }
}

clearLegacyAuthStorage();

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
