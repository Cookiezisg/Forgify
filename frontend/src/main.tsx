import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./index.css";
import { initBackend } from "./lib/events";

// Read backend port from URL query param injected by Electron
const port = parseInt(new URLSearchParams(window.location.search).get("port") ?? "0");
if (port > 0) initBackend(port);

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
