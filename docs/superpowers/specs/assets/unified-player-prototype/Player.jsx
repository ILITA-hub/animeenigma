// AnimeEnigma — flagship "Neon Tokyo" player. One unified skin that wraps every
// source (Our English, Kodik, AniLib, Hanime, Raw) so all providers look alike.
// Cosmetic recreation: the "video" is a still stand-in.
//
// Source model mirrors the repo's watch-together protocol: a "translation" =
// a source (PlayerKind) + an audio kind. Whether Sub/Dub exists is PER-SOURCE,
// so the Sub/Dub choice lives inside Source — never a standalone toggle.

// Each provider advertises its audio kinds, track languages, and its own
// servers (mirrors). The two big filters (audio + language) narrow this list.
// AnimeEnigma is the first-party source, served from the in-house "SVO" server.
const PROVIDERS = [
  { id:'ae',     name:'AnimeEnigma', hue:'#00d4ff', audios:['Sub','Dub'], langs:['English','Русский'], servers:['SVO','SVO Mirror'] },
  { id:'our',    name:'Our English', hue:'#3b82f6', audios:['Sub','Dub'], langs:['English'],            servers:['Server 1','Server 2'] },
  { id:'kodik',  name:'Kodik',       hue:'#22d3ee', audios:['Dub','Sub'], langs:['Русский','English'], servers:['Server 1','Server 2','Server 3'] },
  { id:'anilib', name:'AniLib',      hue:'#ff8a3d', audios:['Sub'],       langs:['Русский'],            servers:['Server 1','Alpha (beta)'] },
  { id:'hanime', name:'Hanime',      hue:'#ff4d8d', audios:['Dub'],       langs:['Русский'],            servers:['Server 1'] },
  { id:'raw',    name:'Raw',         hue:'#fb7185', audios:['Sub'],       langs:['日本語'],             servers:['Server 1'] },
];
const AUDIO_KINDS = ['Sub', 'Dub'];
const TRACK_LANGS = ['English', 'Русский', '日本語'];
// Translation teams (fandub / fansub groups), Kodik-style — keyed by audio + language.
const TEAMS = {
  Sub: { English: ['AnimeEnigma TL', 'Crunchyroll', 'HorribleSubs'], 'Русский': ['SovetRomantica', 'AniLibria'], '日本語': ['Official captions'] },
  Dub: { English: ['AnimeEnigma Dub', 'Funimation'],                  'Русский': ['AniLibria', 'AniDub', 'SHIZA', 'JAM'], '日本語': ['Original'] },
};
const teamsFor = (a, l) => (TEAMS[a] && TEAMS[a][l]) || [];
const QUALITIES = ['Auto', '1080p', '720p', '480p'];
const SPEEDS = [0.75, 1, 1.25, 1.5, 2];
const SUB_LANGS = ['Off', 'English', 'Русский', '日本語'];
// External subtitle tracks for the "browse all subtitles" modal (Jimaku /
// OpenSubtitles / Kitsunekko), grouped by language in the UI.
const SUB_TRACKS = [
  { url:'t1', provider:'Jimaku',        lang:'日本語',   label:'[Kawaisubs] Re:Zero S1 – 12', format:'ass' },
  { url:'t2', provider:'Jimaku',        lang:'日本語',   label:'Official JP captions',        format:'srt' },
  { url:'t3', provider:'Kitsunekko',    lang:'日本語',   label:'rezero_12_jp.ass',            format:'ass' },
  { url:'t4', provider:'OpenSubtitles', lang:'English',  label:'Re:Zero – 12 [HorribleSubs]',  format:'srt' },
  { url:'t5', provider:'Jimaku',        lang:'English',  label:'Re:Zero 12 – fan TL v2',       format:'ass' },
  { url:'t6', provider:'OpenSubtitles', lang:'English',  label:'Re Zero ep12 1080p BD',       format:'ass' },
  { url:'t7', provider:'OpenSubtitles', lang:'Русский',  label:'Re:Zero – 12 [AniLibria]',     format:'ass' },
  { url:'t8', provider:'OpenSubtitles', lang:'Español',  label:'Re:Zero – 12 sub esp',         format:'srt' },
];
const REACTIONS = ['❤️', '🔥', '😂', '😮', '😢', '👏'];
// Default party rosters for the demo. In production these come from the
// watch-together RoomSnapshot (members + host). The PLAYER never owns this —
// it's passed in, so the exact same component runs solo or in a party.
const PARTY_HOST = {
  role: 'host', host: 'kenji', inviteUrl: 'animeenigma.tv/w/8fa2c1',
  members: [{ id:'m1', name:'kenji', host:true, you:true }, { id:'m2', name:'yuki' }, { id:'m3', name:'mei' }],
};
const PARTY_GUEST = {
  role: 'guest', host: 'yuki', inviteUrl: 'animeenigma.tv/w/8fa2c1',
  members: [{ id:'m2', name:'yuki', host:true }, { id:'m1', name:'kenji', you:true }, { id:'m3', name:'mei' }, { id:'m4', name:'taro' }],
};
const DURATION = 1421; // 23:41
const SUB_LINES = {
  English: "I'll save her — no matter how many times it takes.",
  'Русский': 'Я спасу её — сколько бы раз ни пришлось.',
  '日本語': '何度だって、彼女を救ってみせる。',
  Off: '',
};
const fmt = (s) => `${Math.floor(s/60)}:${String(Math.floor(s%60)).padStart(2,'0')}`;

// Provider badge color for the subtitle modal (mirrors repo's providerVariant).
const SUB_PROV_HUE = { Jimaku:'#00d4ff', OpenSubtitles:'#ff2d7c', Kitsunekko:'#a78bfa' };

// "Browse all subtitles" — full modal, tracks grouped by language with
// provider + language filter chips. UX modeled on the repo's OtherSubsPanel.
function SubsModal({ selectedUrl, onSelect, onClose }) {
  const [provFilter, setProvFilter] = React.useState('All');
  const [langFilter, setLangFilter] = React.useState('All');
  const [q, setQ] = React.useState('');
  const query = q.trim().toLowerCase();
  const matchQ = (t) => !query || t.label.toLowerCase().includes(query) || t.provider.toLowerCase().includes(query);
  const providers = ['All', ...Array.from(new Set(SUB_TRACKS.map(t => t.provider)))];
  const langCounts = {};
  SUB_TRACKS.forEach(t => { if ((provFilter === 'All' || t.provider === provFilter) && matchQ(t)) langCounts[t.lang] = (langCounts[t.lang] || 0) + 1; });
  const langs = Object.keys(langCounts);
  const visible = SUB_TRACKS.filter(t => (provFilter === 'All' || t.provider === provFilter) && (langFilter === 'All' || t.lang === langFilter) && matchQ(t));
  const groups = langs.filter(l => langFilter === 'All' || l === langFilter).map(l => ({ lang: l, tracks: visible.filter(t => t.lang === l) })).filter(g => g.tracks.length);

  return (
    <div className="pl-modal-scrim" onClick={onClose}>
      <div className="pl-modal glass-elevated" onClick={e => e.stopPropagation()}>
        <div className="pl-modal-head">
          <div>
            <h3 className="pl-modal-title">Browse all subtitles</h3>
            <p className="pl-modal-sub">Community &amp; official tracks from across providers</p>
          </div>
          <button className="pl-icon" onClick={onClose}><Icon name="close" size={18} /></button>
        </div>

        <div className="pl-modal-filters">
          <div className="pl-modal-searchwrap">
            <Icon name="search" size={16} className="pl-modal-searchico" />
            <input className="pl-modal-search" placeholder="Search by release or group…" value={q} onChange={e => setQ(e.target.value)} autoFocus />
            {q && <button className="pl-modal-clear" onClick={() => setQ('')}><Icon name="close" size={14} /></button>}
          </div>
          <div className="pl-modal-filterrow">
            <span className="pl-modal-filterlabel">Provider</span>
            {providers.map(p => <button key={p} className={'pl-modal-chip' + (provFilter === p ? ' is-active' : '')} onClick={() => setProvFilter(p)}>{p}</button>)}
          </div>
          <div className="pl-modal-filterrow">
            <span className="pl-modal-filterlabel">Language</span>
            <button className={'pl-modal-chip' + (langFilter === 'All' ? ' is-active' : '')} onClick={() => setLangFilter('All')}>All</button>
            {langs.map(l => <button key={l} className={'pl-modal-chip' + (langFilter === l ? ' is-active' : '')} onClick={() => setLangFilter(l)}>{l} ({langCounts[l]})</button>)}
          </div>
        </div>

        <div className="pl-modal-body">
          {groups.length === 0 && <p className="pl-modal-empty">No subtitle tracks match these filters.</p>}
          {groups.map(g => (
            <section key={g.lang} className="pl-modal-group">
              <h4 className="pl-modal-grouph">{g.lang} <span className="pl-modal-groupn">({g.tracks.length})</span></h4>
              <ul className="pl-modal-list">
                {g.tracks.map(t => {
                  const sel = t.url === selectedUrl;
                  return (
                    <li key={t.url} className={'pl-modal-track' + (sel ? ' is-selected' : '')}>
                      <span className="pl-modal-badge" style={{ background: `color-mix(in srgb, ${SUB_PROV_HUE[t.provider] || '#888'} 22%, transparent)`, color: SUB_PROV_HUE[t.provider] || '#fff' }}>{t.provider}</span>
                      <div className="pl-modal-trackinfo">
                        <span className="pl-modal-tracklabel">{t.label}</span>
                        <span className="pl-modal-trackfmt mono">{t.format.toUpperCase()}</span>
                      </div>
                      <button className="pl-modal-select" disabled={sel} onClick={() => onSelect(t)}>{sel ? 'Selected' : 'Select'}</button>
                    </li>
                  );
                })}
              </ul>
            </section>
          ))}
          <p className="pl-modal-note">Kitsunekko is currently degraded — some tracks may be unavailable.</p>
        </div>
      </div>
    </div>
  );
}

function AnimePlayer({ anime, theater, onToggleTheater, onOpenEpisodes, party = null }) {
  const [playing, setPlaying] = React.useState(true);
  const [progress, setProgress] = React.useState(34); // %
  const [volume, setVolume] = React.useState(80);
  const [muted, setMuted] = React.useState(false);
  const [menu, setMenu] = React.useState(null); // settings|subs|subsettings
  const [quality, setQuality] = React.useState('1080p');
  const [speed, setSpeed] = React.useState(1);
  const [provider, setProvider] = React.useState('ae');
  const [audioType, setAudioType] = React.useState('Sub'); // SUB/DUB filter
  const [trackLang, setTrackLang] = React.useState('English'); // language filter
  const [team, setTeam] = React.useState('AnimeEnigma TL'); // fandub/sub group
  const [srcOpen, setSrcOpen] = React.useState(false); // Source & translation panel
  const [subLang, setSubLang] = React.useState('English');
  const [subSize, setSubSize] = React.useState(26);
  const [subColor, setSubColor] = React.useState('#ffffff');
  const [subBg, setSubBg] = React.useState(45);
  const [subOffset, setSubOffset] = React.useState(0); // timing offset, seconds
  const [chosenSub, setChosenSub] = React.useState(null); // track picked from the modal
  const [subsModalOpen, setSubsModalOpen] = React.useState(false);
  const [resume, setResume] = React.useState(true);
  const [hover, setHover] = React.useState(null);
  const [server, setServer] = React.useState('SVO');
  const [settingsView, setSettingsView] = React.useState('root');
  const [autoNext, setAutoNext] = React.useState(false);
  const [autoSkip, setAutoSkip] = React.useState(false);
  // Watch together — the player is agnostic: `party` is a prop (null = solo).
  const role = party ? party.role : 'solo';
  const members = party ? party.members : [];
  const host = party ? party.host : null;
  const inviteUrl = party ? party.inviteUrl : 'animeenigma.tv/w/8fa2c1';
  const [wtOpen, setWtOpen] = React.useState(false);
  const [synced, setSynced] = React.useState(true);  // host: everyone-follows · guest: following-host
  const [drifted, setDrifted] = React.useState(false); // guest seeked away from host
  const [reaction, setReaction] = React.useState(null);
  React.useEffect(() => { setWtOpen(false); setSynced(true); setDrifted(false); }, [role]);

  React.useEffect(() => {
    if (!playing) return;
    const t = setInterval(() => setProgress(p => (p >= 100 ? 100 : p + 0.25)), 250);
    return () => clearInterval(t);
  }, [playing]);

  const cur = (progress / 100) * DURATION;
  // Providers that satisfy the current SUB/DUB + language filters.
  const availProviders = PROVIDERS.filter(p => p.audios.includes(audioType) && p.langs.includes(trackLang));
  const prov = PROVIDERS.find(p => p.id === provider) || PROVIDERS[0];
  // Keep the active provider/server/team valid as the filters change.
  React.useEffect(() => {
    if (availProviders.length && !availProviders.some(p => p.id === provider)) {
      setProvider(availProviders[0].id);
      setServer(availProviders[0].servers[0]);
    }
    const teams = teamsFor(audioType, trackLang);
    if (teams.length && !teams.includes(team)) setTeam(teams[0]);
  }, [audioType, trackLang]);
  const inIntro = cur > 28 && cur < 95;
  const nearEnd = progress > 90;
  const closeMenu = () => setMenu(null);
  const pickProvider = (p) => { setProvider(p.id); setServer(p.servers[0]); };
  const flashReaction = (r) => { setReaction(r); setTimeout(() => setReaction(null), 1400); };

  const trackEl = (
    <div className="pl-track" onMouseMove={e => { const r = e.currentTarget.getBoundingClientRect(); setHover(((e.clientX - r.left) / r.width) * 100); }}
         onMouseLeave={() => setHover(null)}
         onClick={e => { const r = e.currentTarget.getBoundingClientRect(); setProgress(Math.max(0, Math.min(100, ((e.clientX - r.left) / r.width) * 100))); if (role === 'guest' && synced) setDrifted(true); }}>
      <div className="pl-buffered" style={{ width: Math.min(100, progress + 14) + '%' }}></div>
      <div className="pl-fill" style={{ width: progress + '%' }}><span className="pl-thumb"></span></div>
      <span className="pl-chapter" style={{ left: '2%', width: '4.6%' }} title="Intro"></span>
      <span className="pl-chapter" style={{ left: '90%', width: '10%' }} title="Outro"></span>
      {hover !== null && (
        <div className="pl-preview" style={{ left: hover + '%' }}>
          <div className="pl-preview-thumb" style={anime.still ? { backgroundImage: `url(${anime.still})`, backgroundSize: 'cover', backgroundPosition: 'center' } : { background: anime.grad }}></div>
          <span className="pl-preview-time mono">{fmt((hover/100)*DURATION)}</span>
        </div>
      )}
    </div>
  );

  return (
    <div className={'pl' + (theater ? ' is-theater' : '')} style={{ '--prov': prov.hue }} onClick={() => { closeMenu(); }}>
      <div className="pl-scene" style={anime.still ? { backgroundImage: `url(${anime.still})`, backgroundSize: 'cover', backgroundPosition: 'center' } : { background: anime.grad }}>
        <div className="pl-grain"></div>
        {subLang !== 'Off' && (
          <div className="pl-subs" style={{ bottom: playing ? '14%' : '18%' }}>
            <span className="pl-sub-text" style={{ fontSize: subSize, color: subColor, background: `rgba(0,0,0,${subBg/100})` }}>
              {SUB_LINES[subLang]}
            </span>
          </div>
        )}
        {reaction && <div className="pl-reaction-fly">{reaction}</div>}
      </div>

      {/* top bar */}
      <div className="pl-top" onClick={e => e.stopPropagation()}>
        <button className="pl-icon"><Icon name="arrowLeft" size={20} /></button>
        <div className="pl-title-block">
          <span className="pl-eyebrow mono">EP {anime.ep} · <span className="pl-eyebrow-src"><span className="pl-prov-dot" style={{ background: prov.hue, boxShadow: `0 0 8px ${prov.hue}` }}></span>{prov.name} · {audioType}</span></span>
          <h2 className="pl-title">{anime.title}</h2>
        </div>
        <div className="pl-top-right">
          <button className={'pl-icon pl-wt-btn' + (wtOpen ? ' is-open' : '') + (role !== 'solo' ? ' is-live' : '')} onClick={() => { setWtOpen(o => !o); setSrcOpen(false); }} title="Watch together">
            <Icon name="users" size={20} />{members.length > 0 && <span className="pl-wt-count">{members.length}</span>}
          </button>
          <button className="pl-icon" onClick={onOpenEpisodes} title="Episodes"><Icon name="list" size={20} /></button>
        </div>
      </div>

      {/* watch together panel — same player, role passed in as a prop */}
      {wtOpen && (
        <aside className="pl-wt glass-elevated" onClick={e => e.stopPropagation()}>
          <div className="pl-wt-head">
            <span className="pl-wt-title"><span className={'pl-wt-live' + (role === 'solo' ? ' is-idle' : '')}></span> Watch together</span>
            <button className="pl-icon" onClick={() => setWtOpen(false)}><Icon name="close" size={16} /></button>
          </div>

          {role === 'solo' && (<React.Fragment>
            <p className="pl-wt-pitch">Watch in perfect sync with friends. Start a room and share the link — playback, episode and source stay locked together.</p>
            <button className="pl-wt-start btn btn-primary"><Icon name="users" size={16} /> Start a watch party</button>
            <div className="pl-wt-invite">
              <Icon name="globe" size={15} />
              <input className="pl-wt-link mono" readOnly value={inviteUrl} onClick={e => e.target.select()} />
              <button className="pl-wt-copy">Copy</button>
            </div>
          </React.Fragment>)}

          {role === 'host' && (<React.Fragment>
            <div className="pl-wt-host"><Icon name="trophy" size={14} /> You're hosting · {members.length} watching</div>
            <div className="pl-wt-invite">
              <Icon name="globe" size={15} />
              <input className="pl-wt-link mono" readOnly value={inviteUrl} onClick={e => e.target.select()} />
              <button className="pl-wt-copy">Copy</button>
            </div>
            <button className="pl-wt-sync" onClick={() => setSynced(s => !s)}>
              <div><span className="pl-wt-sync-label">Sync playback</span><span className="pl-wt-sync-sub">Everyone follows you</span></div>
              <span className={'pl-switch' + (synced ? ' is-on' : '')}></span>
            </button>
          </React.Fragment>)}

          {role === 'guest' && (<React.Fragment>
            <div className="pl-wt-host"><Icon name="trophy" size={14} /> Hosted by <b>{host}</b> · {members.length} watching</div>
            <button className="pl-wt-sync" onClick={() => { setSynced(s => !s); if (!synced) setDrifted(false); }}>
              <div><span className="pl-wt-sync-label">{synced ? `Following ${host}` : 'Watching on your own'}</span><span className="pl-wt-sync-sub">{synced ? 'Your playback follows the host' : 'Tap to rejoin the host’s timeline'}</span></div>
              <span className={'pl-switch' + (synced ? ' is-on' : '')}></span>
            </button>
            <div className="pl-wt-invite">
              <Icon name="globe" size={15} />
              <input className="pl-wt-link mono" readOnly value={inviteUrl} onClick={e => e.target.select()} />
              <button className="pl-wt-copy">Copy</button>
            </div>
          </React.Fragment>)}

          <div className="pl-wt-section">Watching now</div>
          <div className="pl-wt-members">
            {members.map(m => (
              <div key={m.id} className="pl-wt-member">
                <span className="pl-wt-ava">{m.name.slice(0,2).toUpperCase()}<span className="pl-wt-presence"></span></span>
                <span className="pl-wt-name">{m.name}{m.you && <span className="you-tag">you</span>}</span>
                {m.host && <span className="pl-wt-crown" title="Host"><Icon name="trophy" size={13} /></span>}
              </div>
            ))}
          </div>

          {role !== 'solo' && (<React.Fragment>
            <div className="pl-wt-reactions">
              {REACTIONS.map(r => <button key={r} className="pl-wt-react" onClick={() => flashReaction(r)}>{r}</button>)}
            </div>
            <div className="pl-wt-chatrow">
              <input className="pl-wt-chat" placeholder="Message the room…" />
              <button className="pl-icon"><Icon name="send" size={16} /></button>
            </div>
            <p className="pl-wt-note">{role === 'host' ? 'Your episode, source & quality changes sync to everyone.' : `Only ${host} controls playback, episode & source for the room.`}</p>
            {role === 'host' && <button className="pl-wt-end">End party for everyone</button>}
            {role === 'guest' && <button className="pl-wt-end">Leave room</button>}
          </React.Fragment>)}
        </aside>
      )}

      {/* Source & translation panel — the full combo lives here, out of the gear menu */}
      {srcOpen && (
        <aside className="pl-srcpanel glass-elevated" onClick={e => e.stopPropagation()}>
          <div className="pl-wt-head">
            <span className="pl-wt-title"><Icon name="server" size={16} /> Source &amp; translation</span>
            <button className="pl-icon" onClick={() => setSrcOpen(false)}><Icon name="close" size={16} /></button>
          </div>

          <div className="pl-bigfilters">
            <div className="pl-bigfilter">
              <span className="pl-bigfilter-label">Audio</span>
              <div className="pl-slider pl-slider-2" data-on={audioType === 'Dub' ? '1' : '0'}>
                <span className="pl-slider-thumb"></span>
                {AUDIO_KINDS.map(a => <button key={a} className={'pl-slider-opt' + (audioType === a ? ' is-active' : '')} onClick={() => setAudioType(a)}>{a}</button>)}
              </div>
            </div>
            <div className="pl-bigfilter">
              <span className="pl-bigfilter-label">Language</span>
              <div className="pl-slider" style={{ '--n': TRACK_LANGS.length, '--i': TRACK_LANGS.indexOf(trackLang) }}>
                <span className="pl-slider-thumb"></span>
                {TRACK_LANGS.map(l => <button key={l} className={'pl-slider-opt' + (trackLang === l ? ' is-active' : '')} onClick={() => setTrackLang(l)}>{l}</button>)}
              </div>
            </div>
          </div>

          <div className="pl-src-sec">
            <span className="pl-wt-section">Team</span>
            <div className="pl-team-chips">
              {teamsFor(audioType, trackLang).map(tm => (
                <button key={tm} className={'pl-teamchip' + (team === tm ? ' is-active' : '')} onClick={() => setTeam(tm)}>{tm}</button>
              ))}
            </div>
          </div>

          <div className="pl-src-sec">
            <span className="pl-wt-section">Provider · {availProviders.length} available</span>
            <div className="pl-src-list">
              {availProviders.length === 0 && <p className="pl-set-hint">No providers for {audioType} · {trackLang}.</p>}
              {availProviders.map(p => (
                <button key={p.id} className={'pl-src-item' + (provider === p.id ? ' is-active' : '')} onClick={() => pickProvider(p)}>
                  <span className="pl-prov-dot" style={{ background: p.hue, boxShadow: `0 0 8px ${p.hue}` }}></span>
                  <span className="pl-src-name">{p.name}</span>
                  <span className="pl-menu-sub mono">{p.servers.length} srv</span>
                  {provider === p.id && <Icon name="check" size={15} />}
                </button>
              ))}
            </div>
          </div>

          <div className="pl-src-sec">
            <span className="pl-wt-section">Server</span>
            <div className="pl-src-list">
              {prov.servers.map(s => (
                <button key={s} className={'pl-src-item' + (server === s ? ' is-active' : '')} onClick={() => setServer(s)}>
                  <span className="pl-src-name">{s}</span>
                  {s.startsWith('SVO') && <span className="pl-hd">1st-party</span>}
                  {server === s && <Icon name="check" size={15} />}
                </button>
              ))}
            </div>
          </div>
        </aside>
      )}

      {/* guest resync pill — appears when a guest seeks away from the host */}
      {role === 'guest' && synced && drifted && (
        <button className="pl-resync glass-elevated" onClick={e => { e.stopPropagation(); setDrifted(false); setProgress(34); }}>
          <Icon name="forward10" size={15} /> Resync to {host}
        </button>
      )}

      {/* resume pill */}
      {resume && (
        <div className="pl-resume glass-elevated" onClick={e => e.stopPropagation()}>
          <span className="pl-resume-txt">Resume from <b className="mono">8:05</b>?</span>
          <button className="pl-resume-btn" onClick={() => { setProgress(34); setResume(false); }}>Resume</button>
          <button className="pl-resume-link" onClick={() => { setProgress(0); setResume(false); }}>Start over</button>
        </div>
      )}

      {!playing && (
        <button className="pl-bigplay" onClick={e => { e.stopPropagation(); setPlaying(true); }}><Icon name="play" size={38} /></button>
      )}

      {inIntro && (
        <button className="pl-skip" onClick={e => { e.stopPropagation(); setProgress(8); }}>Skip Intro <Icon name="skipFwd" size={15} /></button>
      )}

      {nearEnd && (
        <div className="pl-next glass-elevated" onClick={e => e.stopPropagation()}>
          <span className="pl-next-label">Up next · autoplay in 8s</span>
          <div className="pl-next-body">
            <span className="pl-next-thumb" style={anime.still ? { backgroundImage: `url(${anime.still})`, backgroundSize:'cover', backgroundPosition:'center' } : { background: anime.grad }}></span>
            <div>
              <p className="pl-next-ep mono">EP {anime.ep + 1}</p>
              <p className="pl-next-title">{anime.title}</p>
            </div>
          </div>
          <div className="pl-next-actions">
            <button className="btn btn-primary btn-sm" onClick={() => setProgress(2)}><Icon name="play" size={14} /> Play now</button>
            <button className="pl-next-cancel" onClick={() => setProgress(80)}>Cancel</button>
          </div>
        </div>
      )}

      {/* control bar */}
      <div className="pl-controls" onClick={e => e.stopPropagation()}>
        <div className="pl-scrub-row">
          <span className="pl-time mono">{fmt(cur)}</span>
          {trackEl}
          <span className="pl-time mono">{fmt(DURATION)}</span>
        </div>
        <div className="pl-btns">
          <button className="pl-icon" onClick={() => setPlaying(p => !p)}><Icon name={playing ? 'pause' : 'play'} size={22} /></button>
          <button className="pl-icon" onClick={() => setProgress(p => Math.max(0, p - 3.5))} title="Back 10s"><Icon name="forward10" size={19} style={{ transform:'scaleX(-1)' }} /></button>
          <button className="pl-icon" onClick={() => setProgress(p => Math.min(100, p + 3.5))} title="Forward 10s"><Icon name="forward10" size={19} /></button>
          <div className="pl-vol">
            <button className="pl-icon" onClick={() => setMuted(m => !m)}><Icon name="volume" size={19} /></button>
            <input type="range" min="0" max="100" value={muted ? 0 : volume} onChange={e => { setVolume(+e.target.value); setMuted(false); }} className="pl-vol-range" />
          </div>

          <div className="pl-spacer"></div>

          {/* Source & translation trigger — opens the dedicated panel */}
          <button className={'pl-srcbtn' + (srcOpen ? ' is-open' : '')} onClick={() => { setSrcOpen(o => !o); setWtOpen(false); closeMenu(); }} title="Source & translation">
            <span className="pl-prov-dot" style={{ background: prov.hue, boxShadow: `0 0 8px ${prov.hue}` }}></span>
            <span className="pl-srcbtn-text">{prov.name} · {audioType}</span>
            <Icon name="chevronDown" size={14} />
          </button>

          {/* subtitles */}
          <div className="pl-menu-wrap">
            <button className={'pl-icon' + (menu && menu.startsWith('sub') ? ' is-open' : '')} onClick={() => { setMenu(menu && menu.startsWith('sub') ? null : 'subs'); setSrcOpen(false); }} title="Subtitles"><Icon name="cc" size={20} /></button>
            {menu === 'subs' && (
              <div className="pl-menu glass-elevated">
                <div className="pl-menu-head">Subtitles</div>
                {SUB_LANGS.map(l => (
                  <button key={l} className={'pl-menu-row' + (subLang === l ? ' is-active' : '')} onClick={() => setSubLang(l)}>
                    <span className="pl-menu-label">{l}</span>{subLang === l && <Icon name="check" size={16} />}
                  </button>
                ))}
                <button className="pl-menu-row pl-menu-more" onClick={() => setMenu('subsettings')}><Icon name="settings" size={15} /> <span className="pl-menu-label">Subtitle settings</span><Icon name="chevronRight" size={14} /></button>
              </div>
            )}
            {menu === 'subsettings' && (
              <div className="pl-menu glass-elevated pl-subset">
                <button className="pl-menu-back" onClick={() => setMenu('subs')}><Icon name="chevronLeft" size={15} /> Subtitle settings</button>
                <div className="pl-set-row"><label>Text size</label><input type="range" min="16" max="40" value={subSize} onChange={e => setSubSize(+e.target.value)} /></div>
                <div className="pl-set-row"><label>Background</label><input type="range" min="0" max="90" value={subBg} onChange={e => setSubBg(+e.target.value)} /></div>
                <div className="pl-menu-divider"></div>
                <div className="pl-set-row"><label>Timing offset</label>
                  <div className="pl-stepper">
                    <button onClick={() => setSubOffset(o => +(o - 0.1).toFixed(2))} title="−0.1s">−</button>
                    <span className="pl-offset-field">
                      <input className="pl-offset-input mono" type="number" step="0.1" value={subOffset}
                        onChange={e => setSubOffset(e.target.value === '' || e.target.value === '-' ? 0 : +(+e.target.value).toFixed(2))} />
                      <span className="pl-offset-unit mono">s</span>
                    </span>
                    <button onClick={() => setSubOffset(o => +(o + 0.1).toFixed(2))} title="+0.1s">+</button>
                  </div>
                </div>
                <p className="pl-set-hint">{subOffset === 0 ? 'In sync · type or step in 0.1s' : subOffset > 0 ? `Subtitles ${subOffset.toFixed(1)}s later` : `Subtitles ${Math.abs(subOffset).toFixed(1)}s earlier`}{subOffset !== 0 && <button className="pl-set-reset" onClick={() => setSubOffset(0)}>Reset</button>}</p>
                <div className="pl-menu-divider"></div>
                <button className="pl-menu-row pl-subs-browse" onClick={() => { setSubsModalOpen(true); closeMenu(); }}>
                  <Icon name="cc" size={15} /><span className="pl-menu-label">Browse all subtitles</span>
                  {chosenSub ? <span className="pl-menu-sub mono">{chosenSub.provider}</span> : <Icon name="chevronRight" size={14} />}
                </button>
              </div>
            )}
          </div>

          {/* settings — playback preferences only (source/translation lives in its own panel) */}
          <div className="pl-menu-wrap">
            <button className={'pl-icon' + (menu === 'settings' ? ' is-open' : '')} onClick={() => { setMenu(menu === 'settings' ? null : 'settings'); setSettingsView('root'); setSrcOpen(false); }} title="Settings"><Icon name="settings" size={20} /></button>
            {menu === 'settings' && (
              <div className="pl-menu glass-elevated pl-settings">
                {settingsView === 'root' && (<React.Fragment>
                  <div className="pl-menu-head">Playback</div>
                  <button className="pl-menu-row" onClick={() => setSettingsView('quality')}><Icon name="gauge" size={15} /><span className="pl-menu-label">Quality</span><span className="pl-menu-val">{quality}</span><Icon name="chevronRight" size={14} /></button>
                  <button className="pl-menu-row" onClick={() => setSettingsView('speed')}><Icon name="forward10" size={15} /><span className="pl-menu-label">Speed</span><span className="pl-menu-val">{speed}×</span><Icon name="chevronRight" size={14} /></button>
                  <div className="pl-menu-divider"></div>
                  <button className="pl-menu-row pl-toggle-row" onClick={() => setAutoNext(v => !v)}><span className="pl-menu-label">Autoplay next</span><span className={'pl-switch' + (autoNext ? ' is-on' : '')}></span></button>
                  <button className="pl-menu-row pl-toggle-row" onClick={() => setAutoSkip(v => !v)}><span className="pl-menu-label">Auto-skip intro</span><span className={'pl-switch' + (autoSkip ? ' is-on' : '')}></span></button>
                </React.Fragment>)}

                {settingsView === 'quality' && (<React.Fragment>
                  <button className="pl-menu-back" onClick={() => setSettingsView('root')}><Icon name="chevronLeft" size={15} /> Quality</button>
                  {QUALITIES.map(q => <button key={q} className={'pl-menu-row' + (quality === q ? ' is-active' : '')} onClick={() => setQuality(q)}><span className="pl-menu-label">{q}</span>{q === '1080p' && <span className="pl-hd">HD</span>}{quality === q && <Icon name="check" size={15} />}</button>)}
                </React.Fragment>)}

                {settingsView === 'speed' && (<React.Fragment>
                  <button className="pl-menu-back" onClick={() => setSettingsView('root')}><Icon name="chevronLeft" size={15} /> Speed</button>
                  {SPEEDS.map(s => <button key={s} className={'pl-menu-row' + (speed === s ? ' is-active' : '')} onClick={() => setSpeed(s)}><span className="pl-menu-label">{s}×</span>{speed === s && <Icon name="check" size={15} />}</button>)}
                </React.Fragment>)}
              </div>
            )}
          </div>

          <button className="pl-icon" title="Picture in picture"><Icon name="pip" size={20} /></button>
          <button className={'pl-icon' + (theater ? ' is-open' : '')} onClick={onToggleTheater} title="Theater mode"><Icon name="expand" size={19} /></button>
          <button className="pl-icon" title="Fullscreen"><Icon name="fullscreen" size={19} /></button>
        </div>
      </div>

      {subsModalOpen && <SubsModal selectedUrl={chosenSub ? chosenSub.url : null} onSelect={t => { setChosenSub(t); setSubsModalOpen(false); }} onClose={() => setSubsModalOpen(false)} />}
    </div>
  );
}
window.AnimePlayer = AnimePlayer;
window.PLAYER_PROVIDERS = PROVIDERS;
