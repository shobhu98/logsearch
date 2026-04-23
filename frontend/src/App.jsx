import { useState, useEffect, useRef, useCallback } from 'react'
import { Search, Database, Clock, ChevronLeft, ChevronRight,
         AlertCircle, Server, RefreshCw, Tag, Monitor, Hash,
         Activity, FileText, Layers, Upload, CheckCircle } from 'lucide-react'
import { fetchStats, fetchHealth, triggerReload, uploadParquet } from './api'
import { useSearch } from './useSearch'
import styles from './App.module.css'

// ── highlight matching terms in text ──────────────────────────────────────────
function highlight(text, query) {
  if (!query || !text) return text
  const terms = query.trim().split(/\s+/).filter(Boolean)
  if (!terms.length) return text
  const pattern = new RegExp(`(${terms.map(t => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})`, 'gi')
  const parts = text.split(pattern)
  return parts.map((part, i) =>
    pattern.test(part)
      ? <mark key={i} className={styles.highlight}>{part}</mark>
      : part
  )
}

// ── severity badge ─────────────────────────────────────────────────────────────
function SeverityBadge({ value }) {
  const v = (value || '').toLowerCase()
  const cls = v === 'warn' || v === 'warning' ? styles.badgeWarn
            : v === 'err'  || v === 'error'   ? styles.badgeErr
            : styles.badgeInfo
  return <span className={`${styles.badge} ${cls}`}>{value || 'info'}</span>
}

// ── single result card ─────────────────────────────────────────────────────────
function ResultCard({ result, query, index }) {
  const [expanded, setExpanded] = useState(false)
  const r = result.record

  const ts = r.Timestamp
    ? new Date(r.Timestamp).toLocaleString('en-US', { month:'short', day:'numeric', hour:'2-digit', minute:'2-digit', second:'2-digit', hour12:false })
    : r.NanoTimeStamp

  return (
    <div
      className={styles.card}
      style={{ animationDelay: `${index * 30}ms` }}
      onClick={() => setExpanded(e => !e)}
    >
      <div className={styles.cardTop}>
        <div className={styles.cardLeft}>
          <div className={styles.cardMeta}>
            <span className={styles.appChip}>
              <Monitor size={10} strokeWidth={2.5} />
              {r.AppName || '—'}
            </span>
            <SeverityBadge value={r.SeverityString} />
            {r.Hostname && (
              <span className={styles.metaItem}>
                <Server size={10} />
                {r.Hostname}
              </span>
            )}
            {r.Namespace && (
              <span className={styles.metaItem}>
                <Layers size={10} />
                {r.Namespace}
              </span>
            )}
          </div>
          <p className={styles.cardMessage}>
            {highlight(r.Message?.slice(0, 280), query)}
            {r.Message?.length > 280 && <span className={styles.ellipsis}>…</span>}
          </p>
        </div>
        <div className={styles.cardRight}>
          <div className={styles.scoreBar}>
            <span className={styles.scoreLabel}>score</span>
            <span className={styles.scoreVal}>{result.score}</span>
          </div>
          <span className={styles.timestamp}>{ts}</span>
        </div>
      </div>

      {expanded && (
        <div className={styles.cardExpanded} onClick={e => e.stopPropagation()}>
          <div className={styles.expandGrid}>
            {r.ProcId && (
              <ExpandRow icon={<Hash size={11}/>} label="ProcId" value={r.ProcId} query={query} />
            )}
            {r.Sender && (
              <ExpandRow icon={<Activity size={11}/>} label="Sender" value={r.Sender} />
            )}
            {r.MsgId && (
              <ExpandRow icon={<Tag size={11}/>} label="MsgId" value={r.MsgId} mono />
            )}
            {r.Tag && (
              <ExpandRow icon={<Tag size={11}/>} label="Tag" value={r.Tag} query={query} />
            )}
            {r.StructuredData && r.StructuredData !== '{}' && (
              <ExpandRow icon={<FileText size={11}/>} label="StructuredData"
                value={r.StructuredData} mono long query={query} />
            )}
            {r.MessageRaw && (
              <ExpandRow icon={<FileText size={11}/>} label="Raw"
                value={r.MessageRaw} mono long query={query} />
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function ExpandRow({ icon, label, value, mono, long, query }) {
  return (
    <div className={`${styles.expandRow} ${long ? styles.expandRowFull : ''}`}>
      <span className={styles.expandLabel}>{icon}{label}</span>
      <span className={`${styles.expandVal} ${mono ? styles.mono : ''}`}>
        {query ? highlight(value, query) : value}
      </span>
    </div>
  )
}

// ── pagination ─────────────────────────────────────────────────────────────────
function Pagination({ page, totalPages, onPage }) {
  if (totalPages <= 1) return null
  const pages = []
  const start = Math.max(0, page - 2)
  const end   = Math.min(totalPages - 1, page + 2)
  for (let i = start; i <= end; i++) pages.push(i)

  return (
    <div className={styles.pagination}>
      <button className={styles.pageBtn} onClick={() => onPage(page - 1)} disabled={page === 0}>
        <ChevronLeft size={14} />
      </button>
      {start > 0 && <><button className={styles.pageBtn} onClick={() => onPage(0)}>1</button><span className={styles.pageDots}>…</span></>}
      {pages.map(p => (
        <button key={p} className={`${styles.pageBtn} ${p === page ? styles.pageBtnActive : ''}`}
          onClick={() => onPage(p)}>{p + 1}</button>
      ))}
      {end < totalPages - 1 && <><span className={styles.pageDots}>…</span><button className={styles.pageBtn} onClick={() => onPage(totalPages - 1)}>{totalPages}</button></>}
      <button className={styles.pageBtn} onClick={() => onPage(page + 1)} disabled={page >= totalPages - 1}>
        <ChevronRight size={14} />
      </button>
    </div>
  )
}

// ── main app ───────────────────────────────────────────────────────────────────
export default function App() {
  const {
    query, setQuery, results, total, timeTaken,
    loading, error, page, totalPages, goToPage,
    searched, doSearch,
  } = useSearch()

  const [stats, setStats]     = useState(null)
  const [health, setHealth]   = useState(null)
  const [reloading, setReloading] = useState(false)
  const [uploadState, setUploadState] = useState('idle') // idle | uploading | processing | done | error
  const [uploadMsg, setUploadMsg]     = useState('')
  const fileInputRef = useRef(null)
  const inputRef = useRef(null)

  useEffect(() => {
    Promise.all([fetchStats(), fetchHealth()])
      .then(([s, h]) => { setStats(s); setHealth(h) })
      .catch(() => {})
  }, [])

  const handleKey = useCallback((e) => {
    if (e.key === 'Enter' && query.trim()) doSearch(query, 0)
  }, [query, doSearch])

  const handleReload = async () => {
    setReloading(true)
    try {
      await triggerReload()
      setTimeout(async () => {
        const [s] = await Promise.all([fetchStats()])
        setStats(s)
        setReloading(false)
      }, 3000)
    } catch { setReloading(false) }
  }

  const handleUploadClick = () => fileInputRef.current?.click()

  const handleFileChange = async (e) => {
    const file = e.target.files?.[0]
    if (!file) return
    e.target.value = ''

    setUploadState('uploading')
    setUploadMsg('')
    try {
      await uploadParquet(file)
      setUploadState('processing')
      // poll stats after the backend finishes indexing (~3-5s)
      setTimeout(async () => {
        try {
          const s = await fetchStats()
          setStats(s)
        } catch {}
        setUploadState('done')
        setTimeout(() => setUploadState('idle'), 2000)
      }, 4000)
    } catch (err) {
      setUploadState('error')
      setUploadMsg(err.message)
      setTimeout(() => setUploadState('idle'), 4000)
    }
  }

  const suggestions = ['kafka', 'snapshot', 'producer', 'partition', 'consumer', 'leader', 'otel', 'fluent']

  return (
    <div className={styles.root}>
      {/* ── grid lines bg ── */}
      <div className={styles.grid} aria-hidden />

      {/* ── header ── */}
      <header className={styles.header}>
        <div className={styles.headerInner}>
          <div className={styles.headerStats}>
            {stats && (
              <>
                <div className={styles.statPill}>
                  <Database size={11} />
                  <span>{stats.total_docs?.toLocaleString()} docs</span>
                </div>
                <div className={styles.statPill}>
                  <Hash size={11} />
                  <span>{stats.total_terms?.toLocaleString()} terms</span>
                </div>
              </>
            )}
            <div className={`${styles.statPill} ${health?.status === 'ok' ? styles.pillOnline : styles.pillOffline}`}>
              <span className={styles.dot} />
              <span>{health?.status === 'ok' ? 'online' : 'offline'}</span>
            </div>
            <button className={styles.reloadBtn} onClick={handleReload} disabled={reloading}
              title="Reload index">
              <RefreshCw size={13} className={reloading ? styles.spinning : ''} />
            </button>

            <button
              className={`${styles.reloadBtn} ${uploadState === 'error' ? styles.reloadBtnErr : ''} ${uploadState === 'done' ? styles.reloadBtnOk : ''}`}
              onClick={handleUploadClick}
              disabled={uploadState === 'uploading' || uploadState === 'processing'}
              title={uploadMsg || 'Upload Parquet file'}
            >
              {uploadState === 'uploading' || uploadState === 'processing'
                ? <span className={styles.spinner} style={{ width: 13, height: 13, borderWidth: 2 }} />
                : uploadState === 'done'
                  ? <CheckCircle size={13} />
                  : <Upload size={13} />
              }
            </button>
            <input
              ref={fileInputRef}
              type="file"
              style={{ display: 'none' }}
              onChange={handleFileChange}
            />
          </div>
        </div>
      </header>

      {/* ── hero search ── */}
      <main className={styles.main}>
        <div className={`${styles.hero} ${searched ? styles.heroCompact : ''}`}>
          {!searched && (
            <div className={styles.heroTitle}>
              <h1>Log Search</h1>
              <p>Search across {stats?.total_docs?.toLocaleString() || '24k'} telemetry records instantly</p>
            </div>
          )}

          <div className={styles.searchWrap}>
            <div className={styles.searchBox}>
              <Search size={18} className={styles.searchIcon} />
              <input
                ref={inputRef}
                className={styles.searchInput}
                type="text"
                placeholder="Search logs, events, metadata…"
                value={query}
                onChange={e => setQuery(e.target.value)}
                onKeyDown={handleKey}
                autoFocus
                spellCheck={false}
              />
              {query && (
                <button className={styles.clearBtn} onClick={() => { setQuery(''); inputRef.current?.focus() }}>
                  ×
                </button>
              )}
            </div>
            <button
              className={styles.searchBtn}
              onClick={() => doSearch(query, 0)}
              disabled={!query.trim() || loading}
            >
              {loading ? <span className={styles.spinner} /> : 'Search'}
            </button>
          </div>

          {!searched && (
            <div className={styles.suggestions}>
              <span className={styles.suggestLabel}>Try:</span>
              {suggestions.map(s => (
                <button key={s} className={styles.chip}
                  onClick={() => { setQuery(s); doSearch(s, 0) }}>
                  {s}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* ── results area ── */}
        {searched && (
          <div className={styles.results}>
            {/* meta bar */}
            <div className={styles.metaBar}>
              {error ? (
                <div className={styles.errorMsg}>
                  <AlertCircle size={14} /> {error}
                </div>
              ) : loading ? (
                <span className={styles.metaText}>Searching…</span>
              ) : (
                <span className={styles.metaText}>
                  <strong>{total.toLocaleString()}</strong> results for <em>"{query}"</em>
                  {timeTaken !== null && (
                    <span className={styles.timing}>
                      <Clock size={11} /> {timeTaken.toFixed(2)}ms
                    </span>
                  )}
                </span>
              )}
            </div>

            {/* result cards */}
            {!loading && !error && results.length === 0 && (
              <div className={styles.empty}>
                <Search size={32} strokeWidth={1} />
                <p>No results found for <em>"{query}"</em></p>
                <span>Try a different keyword or prefix</span>
              </div>
            )}

            <div className={styles.cardList}>
              {results.map((r, i) => (
                <ResultCard key={r.record.id} result={r} query={query} index={i} />
              ))}
            </div>

            {results.length > 0 && (
              <Pagination page={page} totalPages={totalPages} onPage={goToPage} />
            )}
          </div>
        )}
      </main>

      <footer className={styles.footer}>
        <span>made with <span className={styles.heart}>❤</span> by Shobhit Tiwari</span>
      </footer>
    </div>
  )
}
