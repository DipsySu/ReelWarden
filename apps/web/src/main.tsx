import React from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';

function App() {
  return (
    <main className="shell">
      <section className="hero">
        <p className="eyebrow">ReelWarden（影卫）</p>
        <h1>Deterministic local media organizing, plan first.</h1>
        <p>
          This v0.1.1 scaffold starts with safe local configuration, SQLite/WAL,
          health checks, and a compliance gate that blocks TMDB + AI until the
          authority process accepts it.
        </p>
      </section>
    </main>
  );
}

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
