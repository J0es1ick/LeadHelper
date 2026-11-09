import React, { Component, type ErrorInfo, type ReactNode } from "react";
import ReactDOM from "react-dom/client";
import { App } from "./app/App";
import { Provider } from "./app/store";
import "./styles.css";
import "./readability.css";
import "./theme.css";
import "./workspace.css";
import "./dark.css";
class ErrorBoundary extends Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(error, info);
  }
  render() {
    return this.state.failed ? (
      <div className="app-loading">
        <h2>Не удалось отобразить экран</h2>
        <p>Сохранённые данные остаются на сервере.</p>
        <button className="button primary" onClick={() => location.reload()}>
          Обновить страницу
        </button>
      </div>
    ) : (
      this.props.children
    );
  }
}
ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ErrorBoundary>
      <Provider>
        <App />
      </Provider>
    </ErrorBoundary>
  </React.StrictMode>,
);
