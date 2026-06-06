// Watch page — hosts the flagship player + episode strip + drawer.
function WatchPage() {
  const [id, setId] = React.useState('rez'); // Re:Zero (real media)
  const [theater, setTheater] = React.useState(false);
  const [drawer, setDrawer] = React.useState(false);
  const [partyRole, setPartyRole] = React.useState('solo'); // solo | host | guest
  const anime = window.ANIME.find(a => a.id === id) || window.ANIME[0];
  const party = partyRole === 'host' ? window.PARTY_HOST : partyRole === 'guest' ? window.PARTY_GUEST : null;

  React.useEffect(() => {
    document.body.classList.toggle('theater-mode', theater);
    return () => document.body.classList.remove('theater-mode');
  }, [theater]);

  return (
    <div className={'watch' + (theater ? ' is-theater' : '')}>
      {/* slim brand bar (hidden in theater) */}
      <header className="watch-bar glass-nav">
        <a className="brand-link"><span className="brand-mark"></span><span className="brand-wordmark"><span className="b1">Anime</span><span className="b2">Enigma</span></span></a>
        <span className="watch-crumb">Watching · <b>{anime.title}</b></span>
        <div className="watch-roleswitch" title="The same player component, in each Watch Together role">
          <span className="watch-roleswitch-label">Preview as</span>
          {['solo','host','guest'].map(r => (
            <button key={r} className={'watch-role-btn' + (partyRole === r ? ' is-active' : '')} onClick={() => setPartyRole(r)}>{r[0].toUpperCase() + r.slice(1)}</button>
          ))}
        </div>
      </header>

      <div className="watch-stage">
        <div className="watch-player-col">
          <AnimePlayer anime={anime} theater={theater} party={party} onToggleTheater={() => setTheater(t => !t)} onOpenEpisodes={() => setDrawer(true)} />

          {/* slim player-relevant strip — episode context + playback actions only */}
          <div className="watch-strip">
            <div className="watch-strip-info">
              <h1 className="watch-h1">{anime.title}</h1>
              <div className="watch-strip-meta">
                <span className="badge badge-primary">EP {anime.ep} / {anime.eps}</span>
                <span className="watch-strip-jp jp">{anime.jp}</span>
              </div>
            </div>
            <div className="watch-info-actions">
              <button className="btn btn-ghost btn-sm"><Icon name="bookmark" size={16} /> My list</button>
              <button className="btn btn-primary btn-sm"><Icon name="skipFwd" size={15} /> Next episode</button>
            </div>
          </div>
        </div>

        {/* episode side panel (inline desktop) */}
        <aside className="watch-eps glass-card">
          <div className="watch-eps-head">
            <h3 className="pane-title" style={{margin:0}}>Episodes</h3>
            <span className="watch-eps-count mono">{anime.ep}/{anime.eps}</span>
          </div>
          <div className="watch-eps-list">
            {Array.from({ length: anime.eps }, (_, i) => i + 1).map(n => (
              <button key={n} className={'watch-ep-row' + (n === anime.ep ? ' is-active' : '') + (n < anime.ep ? ' is-watched' : '')}>
                <span className="watch-ep-thumb" style={anime.still ? { backgroundImage: `url(${anime.still})`, backgroundSize: 'cover', backgroundPosition: 'center' } : { background: anime.grad }}>{n === anime.ep && <span className="watch-ep-playing"><Icon name="play" size={14} /></span>}</span>
                <div className="watch-ep-meta">
                  <span className="watch-ep-num mono">Episode {n}</span>
                  <span className="watch-ep-dur mono">23:41</span>
                </div>
                {n < anime.ep && <Icon name="check" size={15} className="watch-ep-check" />}
              </button>
            ))}
          </div>
        </aside>
      </div>

      {/* mobile / overlay episode drawer */}
      {drawer && (
        <div className="watch-drawer-scrim" onClick={() => setDrawer(false)}>
          <div className="watch-drawer glass-elevated" onClick={e => e.stopPropagation()}>
            <div className="watch-eps-head">
              <h3 className="pane-title" style={{margin:0}}>Episodes</h3>
              <button className="icon-btn-nt" onClick={() => setDrawer(false)}><Icon name="close" size={18} /></button>
            </div>
            <div className="watch-eps-list">
              {Array.from({ length: anime.eps }, (_, i) => i + 1).map(n => (
                <button key={n} className={'watch-ep-row' + (n === anime.ep ? ' is-active' : '') + (n < anime.ep ? ' is-watched' : '')} onClick={() => setDrawer(false)}>
                  <span className="watch-ep-thumb" style={anime.still ? { backgroundImage: `url(${anime.still})`, backgroundSize: 'cover', backgroundPosition: 'center' } : { background: anime.grad }}></span>
                  <div className="watch-ep-meta"><span className="watch-ep-num mono">Episode {n}</span><span className="watch-ep-dur mono">23:41</span></div>
                  {n < anime.ep && <Icon name="check" size={15} className="watch-ep-check" />}
                </button>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<WatchPage />);
