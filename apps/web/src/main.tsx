import React from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';

const steps = [
  'Register a read-only movie library root',
  'Scan common video containers without modifying files',
  'Parse title, year, and technical tags deterministically',
  'Generate mock provider candidates with grouped evidence',
  'Require manual confirmation',
  'Create a Jellyfin-style Dry Run Action Plan',
];

function App() {
  return (
    <main className="shell">
      <section className="hero">
        <p className="eyebrow">ReelWarden（影卫） MVP Slice</p>
        <h1>Plan first. Touch files never in v0.1.1.</h1>
        <p>
          The backend now exposes the read-only MVP loop: library root, scan,
          parse, mock candidates, manual confirmation, and dry-run planning.
        </p>
        <ol>
          {steps.map((step) => <li key={step}>{step}</li>)}
        </ol>
      </section>
    </main>
  );
}

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
