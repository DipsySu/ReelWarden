import { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import type { FormEvent } from 'react';
import './style.css';

type Health = {
  status: string;
  service: string;
  started_at: string;
};

type GateResult = {
  gate_id: string;
  status: string;
  allowed: boolean;
  reason: string;
};

type LibraryRoot = {
  id: string;
  path: string;
  mode: string;
  created_at: string;
};

type MediaAsset = {
  id: string;
  library_root_id: string;
  relative_path: string;
  size_bytes: number;
  modified_at: string;
  scan_state: string;
  parsed_title: string;
  parsed_year?: number;
  match_state: string;
  confirmed_candidate_id?: string;
  created_at: string;
  updated_at: string;
};

type Evidence = {
  group: string;
  code: string;
  message: string;
  points: number;
};

type Candidate = {
  id: string;
  asset_id: string;
  provider: string;
  provider_id: string;
  title: string;
  year?: number;
  score: number;
  score_band: string;
  evidence: Evidence[];
};

type ActionPlan = {
  id: string;
  asset_id: string;
  source_relative_path: string;
  target_relative_path: string;
  dry_run: boolean;
  state: string;
  created_at: string;
  warnings: string[];
};

type ScanResult = {
  scanned: number;
  assets: MediaAsset[];
};

type QueryHypothesis = {
  title: string;
  year?: number;
  media_type?: string;
  external_ids?: Record<string, string>;
  source: string;
};

type ParsedIdentity = {
  id: string;
  media_asset_id: string;
  raw_title: string;
  normalized_title: string;
  comparison_keys?: string[];
  year?: number;
  edition?: string;
  release_group?: string;
  technical_tags?: string[];
  media_type_hint?: string;
  parent_dir_name?: string;
  hypotheses?: QueryHypothesis[];
  confidence: number;
  parser_version: string;
  state: string;
  created_at: string;
};

type RuntimeConfig = {
  server: {
    listen: string;
    data_dir: string;
    log_level: string;
  };
  database: {
    driver: string;
    path: string;
    wal: boolean;
    max_open_conns: number;
    backup_dir: string;
  };
  metadata: {
    default_provider: string;
    providers: {
      mock: { enabled: boolean };
      local_nfo: { enabled: boolean };
      tmdb: {
        enabled: boolean;
        auth_type: string;
        api_key_configured: boolean;
        token_configured: boolean;
        language: string;
        fallback_language: string;
        region: string;
        official_endpoint_only: boolean;
        api_base_url: string;
        proxy_configured: boolean;
        timeout_seconds: number;
        max_retries: number;
      };
    };
  };
  ai: {
    enabled: boolean;
    provider: string;
    base_url: string;
    api_key_configured: boolean;
    model: string;
    protocol: string;
    timeout_seconds: number;
    max_retries: number;
    capabilities: {
      streaming: string;
      tool_calling: string;
      structured_output: string;
    };
    privacy: {
      mode: string;
      send_absolute_paths: boolean;
      send_provider_content: boolean;
      send_nfo_content: boolean;
    };
  };
  compliance: {
    tmdb_ai: string;
  };
};

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      ...(init?.body ? { 'content-type': 'application/json' } : {}),
      ...init?.headers,
    },
  });
  if (!response.ok) {
    let detail = response.statusText;
    try {
      const body = await response.json() as { error_code?: string; message?: string };
      detail = body.error_code ? `${body.error_code}: ${body.message ?? response.statusText}` : body.message ?? detail;
    } catch {
      detail = response.statusText;
    }
    throw new Error(detail);
  }
  return response.json() as Promise<T>;
}

function formatBytes(size: number) {
  if (size < 1024) {
    return `${size} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  if (size < 1024 * 1024 * 1024) {
    return `${(size / 1024 / 1024).toFixed(1)} MB`;
  }
  return `${(size / 1024 / 1024 / 1024).toFixed(1)} GB`;
}

function formatExternalIDs(ids?: Record<string, string>) {
  if (!ids || Object.keys(ids).length === 0) {
    return '';
  }
  return Object.entries(ids)
    .map(([provider, id]) => `${provider}:${id}`)
    .join(' ');
}

function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [runtimeConfig, setRuntimeConfig] = useState<RuntimeConfig | null>(null);
  const [gates, setGates] = useState<GateResult[]>([]);
  const [roots, setRoots] = useState<LibraryRoot[]>([]);
  const [assets, setAssets] = useState<MediaAsset[]>([]);
  const [plans, setPlans] = useState<ActionPlan[]>([]);
  const [candidates, setCandidates] = useState<Record<string, Candidate[]>>({});
  const [identities, setIdentities] = useState<Record<string, ParsedIdentity>>({});
  const [rootPath, setRootPath] = useState('');
  const [selectedRootId, setSelectedRootId] = useState('');
  const [activeAssetId, setActiveAssetId] = useState('');
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  const activeAsset = useMemo(
    () => assets.find((asset) => asset.id === activeAssetId) ?? assets[0],
    [activeAssetId, assets],
  );
  const selectedRoot = useMemo(
    () => roots.find((root) => root.id === selectedRootId),
    [roots, selectedRootId],
  );
  const activeCandidates = activeAsset ? candidates[activeAsset.id] ?? [] : [];
  const activeIdentity = activeAsset ? identities[activeAsset.id] : undefined;
  const activePlans = activeAsset ? plans.filter((plan) => plan.asset_id === activeAsset.id) : plans;
  const confirmedAssets = assets.filter((asset) => asset.match_state === 'confirmed').length;
  const pendingAssets = assets.length - confirmedAssets;

  async function refresh() {
    setBusy('Refreshing');
    setError('');
    try {
      const [nextHealth, nextRuntimeConfig, nextGates, nextRoots, nextAssets, nextPlans] = await Promise.all([
        api<Health>('/health'),
        api<RuntimeConfig>('/api/config/runtime'),
        api<GateResult[]>('/api/compliance/gates'),
        api<LibraryRoot[]>('/api/library_roots'),
        api<MediaAsset[]>('/api/assets'),
        api<ActionPlan[]>('/api/plans'),
      ]);
      setHealth(nextHealth);
      setRuntimeConfig(nextRuntimeConfig);
      setGates(nextGates);
      setRoots(nextRoots);
      setAssets(nextAssets);
      setPlans(nextPlans);
      setSelectedRootId((current) => current || nextRoots[0]?.id || '');
      setActiveAssetId((current) => current || nextAssets[0]?.id || '');
    } catch (err) {
      setHealth(null);
      setRuntimeConfig(null);
      setError(err instanceof Error ? err.message : 'API request failed');
    } finally {
      setBusy('');
    }
  }

  async function loadCandidates(assetId: string) {
    try {
      const nextCandidates = await api<Candidate[]>(`/api/assets/${assetId}/candidates`);
      setCandidates((current) => ({ ...current, [assetId]: nextCandidates }));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Candidate request failed');
    }
  }

  async function loadIdentity(assetId: string) {
    try {
      const nextIdentity = await api<ParsedIdentity>(`/api/assets/${assetId}/identity`);
      setIdentities((current) => ({ ...current, [assetId]: nextIdentity }));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Identity request failed');
    }
  }

  async function loadAssetDetails(assetId: string) {
    await Promise.all([loadCandidates(assetId), loadIdentity(assetId)]);
  }

  async function addRoot(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!rootPath.trim()) {
      setError('Library root path is required');
      return;
    }
    setBusy('Adding root');
    setError('');
    setNotice('');
    try {
      const root = await api<LibraryRoot>('/api/library_roots', {
        method: 'POST',
        body: JSON.stringify({ path: rootPath.trim() }),
      });
      setRootPath('');
      setSelectedRootId(root.id);
      setNotice('Library root registered');
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Root registration failed');
    } finally {
      setBusy('');
    }
  }

  async function scanRoot() {
    if (!selectedRootId) {
      setError('Select a library root before scanning');
      return;
    }
    setBusy('Scanning');
    setError('');
    setNotice('');
    try {
      const result = await api<ScanResult>('/api/scans', {
        method: 'POST',
        body: JSON.stringify({ library_root_id: selectedRootId }),
      });
      setNotice(`Scan complete: ${result.scanned} video file${result.scanned === 1 ? '' : 's'}`);
      await refresh();
      if (result.assets[0]) {
        setActiveAssetId(result.assets[0].id);
        await loadAssetDetails(result.assets[0].id);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Scan failed');
    } finally {
      setBusy('');
    }
  }

  async function confirmCandidate(candidateId: string) {
    if (!activeAsset) {
      return;
    }
    setBusy('Confirming');
    setError('');
    setNotice('');
    try {
      await api<MediaAsset>(`/api/assets/${activeAsset.id}/confirm`, {
        method: 'POST',
        body: JSON.stringify({ candidate_id: candidateId }),
      });
      setNotice('Candidate confirmed');
      await refresh();
      setActiveAssetId(activeAsset.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Confirmation failed');
    } finally {
      setBusy('');
    }
  }

  async function createPlan() {
    if (!activeAsset) {
      return;
    }
    setBusy('Creating plan');
    setError('');
    setNotice('');
    try {
      await api<ActionPlan>('/api/plans', {
        method: 'POST',
        body: JSON.stringify({ asset_id: activeAsset.id }),
      });
      setNotice('Dry-run plan created');
      await refresh();
      setActiveAssetId(activeAsset.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Plan creation failed');
    } finally {
      setBusy('');
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (activeAsset?.id) {
      void loadAssetDetails(activeAsset.id);
    }
  }, [activeAsset?.id]);

  return (
    <main className="app-shell">
      <header className="topbar">
        <div>
          <p className="eyebrow">ReelWarden</p>
          <h1>Read-only MVP console</h1>
        </div>
        <div className="status-strip" aria-live="polite">
          <span className={health ? 'status-dot online' : 'status-dot'} />
          <span>{health ? `${health.service} online` : 'API offline'}</span>
          {busy ? <span className="busy">{busy}</span> : null}
        </div>
      </header>

      {error ? <p className="banner error">{error}</p> : null}
      {notice ? <p className="banner notice">{notice}</p> : null}

      <section className="summary-grid" aria-label="Runtime summary">
        <div className="summary-tile">
          <span>Roots</span>
          <strong>{roots.length}</strong>
        </div>
        <div className="summary-tile">
          <span>Assets</span>
          <strong>{assets.length}</strong>
          <small>{pendingAssets} pending</small>
        </div>
        <div className="summary-tile">
          <span>Plans</span>
          <strong>{plans.length}</strong>
        </div>
        <div className="summary-tile">
          <span>AI</span>
          <strong>{runtimeConfig?.ai.enabled ? 'On' : 'Off'}</strong>
          <small>{runtimeConfig?.ai.model ?? 'qwen3'}</small>
        </div>
        <div className="summary-tile">
          <span>TMDB</span>
          <strong>{runtimeConfig?.metadata.providers.tmdb.enabled ? 'On' : 'Off'}</strong>
          <small>{runtimeConfig?.metadata.providers.tmdb.language ?? 'zh-CN'}</small>
        </div>
      </section>

      <section className="workspace">
        <aside className="panel">
          <div className="panel-heading">
            <h2>Library</h2>
            <button type="button" onClick={() => void refresh()} disabled={Boolean(busy)}>
              Refresh
            </button>
          </div>

          <form className="root-form" onSubmit={(event) => void addRoot(event)}>
            <label htmlFor="root-path">Root path</label>
            <div className="input-row">
              <input
                id="root-path"
                value={rootPath}
                onChange={(event) => setRootPath(event.target.value)}
                placeholder="/Volumes/Movies"
              />
              <button type="submit" disabled={Boolean(busy)}>Add</button>
            </div>
          </form>

          <label htmlFor="root-select">Active root</label>
          <select
            id="root-select"
            value={selectedRootId}
            onChange={(event) => setSelectedRootId(event.target.value)}
          >
            <option value="">No root selected</option>
            {roots.map((root) => (
              <option value={root.id} key={root.id}>{root.path}</option>
            ))}
          </select>

          <button className="primary wide" type="button" onClick={() => void scanRoot()} disabled={Boolean(busy || !selectedRootId)}>
            {busy === 'Scanning' ? 'Scanning' : 'Scan root'}
          </button>

          {selectedRoot ? (
            <div className="root-chip">
              <span>{selectedRoot.mode}</span>
              <strong>{selectedRoot.path}</strong>
            </div>
          ) : null}

          <div className="asset-list">
            <div className="list-title">
              <h2>Assets</h2>
              <span>{assets.length}</span>
            </div>
            {assets.length === 0 ? (
              <p className="empty">No assets scanned.</p>
            ) : assets.map((asset) => (
              <button
                className={`asset-row ${activeAsset?.id === asset.id ? 'active' : ''}`}
                type="button"
                key={asset.id}
                aria-pressed={activeAsset?.id === asset.id}
                onClick={() => setActiveAssetId(asset.id)}
              >
                <span>{asset.relative_path}</span>
                <small>{asset.match_state}</small>
              </button>
            ))}
          </div>
        </aside>

        <section className="panel detail-panel">
          <div className="panel-heading">
            <h2>Review</h2>
            {activeAsset ? <span className="pill">{activeAsset.match_state}</span> : null}
          </div>

          {activeAsset ? (
            <>
              <div className="asset-summary">
                <div>
                  <span className="label">Title</span>
                  <strong>{activeAsset.parsed_title || 'Untitled'}</strong>
                </div>
                <div>
                  <span className="label">Year</span>
                  <strong>{activeAsset.parsed_year ?? 'Unknown'}</strong>
                </div>
                <div>
                  <span className="label">Size</span>
                  <strong>{formatBytes(activeAsset.size_bytes)}</strong>
                </div>
              </div>

              <p className="path-line">{activeAsset.relative_path}</p>

              {activeIdentity ? (
                <section className="preflight-panel" aria-label="Preflight identity">
                  <div className="panel-heading slim">
                    <h2>Preflight</h2>
                    <span className="pill">{activeIdentity.parser_version}</span>
                  </div>
                  <div className="preflight-grid">
                    <div>
                      <span className="label compact">Raw title</span>
                      <strong>{activeIdentity.raw_title || 'Unknown'}</strong>
                    </div>
                    <div>
                      <span className="label compact">Media type</span>
                      <strong>{activeIdentity.media_type_hint || 'unknown'}</strong>
                    </div>
                    <div>
                      <span className="label compact">Edition</span>
                      <strong>{activeIdentity.edition || 'none'}</strong>
                    </div>
                    <div>
                      <span className="label compact">Confidence</span>
                      <strong>{Math.round(activeIdentity.confidence * 100)}%</strong>
                    </div>
                  </div>
                  <div className="tag-row">
                    {(activeIdentity.technical_tags ?? []).length === 0 ? (
                      <span>no technical tags</span>
                    ) : activeIdentity.technical_tags?.map((tag) => (
                      <span key={tag}>{tag}</span>
                    ))}
                  </div>
                  <div className="hypothesis-list">
                    {(activeIdentity.hypotheses ?? []).slice(0, 4).map((hypothesis, index) => (
                      <div className="hypothesis-row" key={`${hypothesis.source}-${hypothesis.title}-${index}`}>
                        <strong>{hypothesis.title || 'ID lookup'}</strong>
                        <span>{hypothesis.year ?? 'any year'}</span>
                        <span>{hypothesis.media_type || 'any type'}</span>
                        {formatExternalIDs(hypothesis.external_ids) ? (
                          <span>{formatExternalIDs(hypothesis.external_ids)}</span>
                        ) : null}
                        <small>{hypothesis.source}</small>
                      </div>
                    ))}
                  </div>
                </section>
              ) : null}

              <div className="candidate-grid">
                {activeCandidates.length === 0 ? (
                  <p className="empty">No candidates loaded.</p>
                ) : activeCandidates.map((candidate) => (
                  <article className="candidate-card" key={candidate.id}>
                    <div className="candidate-head">
                      <div>
                        <h3>{candidate.title}</h3>
                        <p>{candidate.provider} / {candidate.provider_id}</p>
                      </div>
                      <span className={`score ${candidate.score_band}`}>{candidate.score}</span>
                    </div>
                    <div className="evidence-list">
                      {candidate.evidence.map((item) => (
                        <span key={`${candidate.id}-${item.code}`}>{item.code} +{item.points}</span>
                      ))}
                    </div>
                    <button
                      type="button"
                      onClick={() => void confirmCandidate(candidate.id)}
                      disabled={Boolean(busy || activeAsset.confirmed_candidate_id === candidate.id)}
                    >
                      {activeAsset.confirmed_candidate_id === candidate.id ? 'Confirmed' : 'Use candidate'}
                    </button>
                  </article>
                ))}
              </div>

              <div className="plan-actions">
                <button
                  className="primary"
                  type="button"
                  onClick={() => void createPlan()}
                  disabled={Boolean(busy || activeAsset.match_state !== 'confirmed')}
                >
                  Create dry-run plan
                </button>
              </div>
            </>
          ) : (
            <p className="empty">No active asset.</p>
          )}
        </section>

        <aside className="panel">
          <div className="panel-heading">
            <h2>Plans</h2>
            <span className="pill">{plans.length}</span>
          </div>
          <div className="plan-list">
            {activePlans.length === 0 ? (
              <p className="empty">No dry-run plans.</p>
            ) : activePlans.map((plan) => (
              <article className="plan-card" key={plan.id}>
                <span className="pill">{plan.state}</span>
                <p className="from-path">{plan.source_relative_path}</p>
                <p className="to-path">{plan.target_relative_path}</p>
                {plan.warnings.map((warning) => (
                  <small key={warning}>{warning}</small>
                ))}
              </article>
            ))}
          </div>

          <div className="gate-list">
            <h2>Runtime</h2>
            {runtimeConfig ? (
              <div className="runtime-stack">
                <article className="runtime-card">
                  <div className="runtime-head">
                    <div>
                      <span className="label compact">AI</span>
                      <strong>{runtimeConfig.ai.provider}</strong>
                    </div>
                    <span className={`pill ${runtimeConfig.ai.enabled ? 'enabled' : ''}`}>
                      {runtimeConfig.ai.enabled ? 'enabled' : 'disabled'}
                    </span>
                  </div>
                  <dl>
                    <div><dt>Base URL</dt><dd>{runtimeConfig.ai.base_url}</dd></div>
                    <div><dt>Model</dt><dd>{runtimeConfig.ai.model}</dd></div>
                    <div><dt>API key</dt><dd>{runtimeConfig.ai.api_key_configured ? 'configured' : 'missing'}</dd></div>
                    <div><dt>Privacy</dt><dd>{runtimeConfig.ai.privacy.mode}</dd></div>
                  </dl>
                </article>

                <article className="runtime-card">
                  <div className="runtime-head">
                    <div>
                      <span className="label compact">TMDB</span>
                      <strong>{runtimeConfig.metadata.providers.tmdb.auth_type}</strong>
                    </div>
                    <span className={`pill ${runtimeConfig.metadata.providers.tmdb.enabled ? 'enabled' : ''}`}>
                      {runtimeConfig.metadata.providers.tmdb.enabled ? 'enabled' : 'disabled'}
                    </span>
                  </div>
                  <dl>
                    <div><dt>Locale</dt><dd>{runtimeConfig.metadata.providers.tmdb.language} / {runtimeConfig.metadata.providers.tmdb.region}</dd></div>
                    <div><dt>Token</dt><dd>{runtimeConfig.metadata.providers.tmdb.token_configured ? 'configured' : 'missing'}</dd></div>
                    <div><dt>API key</dt><dd>{runtimeConfig.metadata.providers.tmdb.api_key_configured ? 'configured' : 'missing'}</dd></div>
                    <div><dt>Proxy</dt><dd>{runtimeConfig.metadata.providers.tmdb.proxy_configured ? 'configured' : 'off'}</dd></div>
                  </dl>
                </article>
              </div>
            ) : (
              <p className="empty">Runtime config unavailable.</p>
            )}
          </div>

          <div className="gate-list">
            <h2>Gates</h2>
            {gates.map((gate) => (
              <div className="gate-row" key={gate.gate_id}>
                <div>
                  <span>{gate.gate_id}</span>
                  <small>{gate.reason}</small>
                </div>
                <strong>{gate.allowed ? 'allowed' : gate.status}</strong>
              </div>
            ))}
          </div>
        </aside>
      </section>
    </main>
  );
}

createRoot(document.getElementById('root')!).render(
  <App />,
);
