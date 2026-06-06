// Inline stroked-SVG icon set — matches the product's Heroicons-outline language
// (24×24, stroke-width 2, round caps, currentColor). No icon font, no CDN.
const Icon = ({ name, size = 20, sw = 2, className = '', style = {} }) => {
  const paths = {
    search: <path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />,
    play: <path d="M5 3l14 9-14 9V3z" fill="currentColor" stroke="none" />,
    plus: <path d="M12 4v16m8-8H4" />,
    users: <path d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />,
    send: <path d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />,
    chevronDown: <path d="M19 9l-7 7-7-7" />,
    chevronLeft: <path d="M15 19l-7-7 7-7" />,
    chevronRight: <path d="M9 5l7 7-7 7" />,
    close: <path d="M6 18L18 6M6 6l12 12" />,
    star: <path d="M11.48 3.5l2.13 4.31 4.76.69-3.44 3.36.81 4.74L11.48 14.5 7.22 16.6l.81-4.74L4.59 8.5l4.76-.69L11.48 3.5z" fill="currentColor" stroke="none" />,
    bell: <path d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6 6 0 00-12 0v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />,
    calendar: <path d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />,
    music: <path d="M9 19V6l12-3v13M9 19a3 3 0 11-6 0 3 3 0 016 0zm12-3a3 3 0 11-6 0 3 3 0 016 0z" />,
    home: <path d="M3 12l9-9 9 9M5 10v10a1 1 0 001 1h3a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1h3a1 1 0 001-1V10" />,
    grid: <path d="M4 5a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM14 5a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1V5zM4 15a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1H5a1 1 0 01-1-1v-4zM14 15a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z" />,
    check: <path d="M5 13l4 4L19 7" />,
    bookmark: <path d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />,
    sliders: <path d="M3 6h13M3 12h9M3 18h13M17 4v4M13 10v4M17 16v4" />,
    volume: <path d="M11 5L6 9H2v6h4l5 4V5zM15.54 8.46a5 5 0 010 7.07M19.07 4.93a10 10 0 010 14.14" />,
    pause: <path d="M10 9v6m4-6v6" />,
    fullscreen: <path d="M4 8V4h4M16 4h4v4M20 16v4h-4M8 20H4v-4" />,
    trophy: <path d="M8 21h8m-4-4v4m-5-17h10v4a5 5 0 01-10 0V4zM5 6H3v2a3 3 0 003 3M19 6h2v2a3 3 0 01-3 3" />,
    settings: <path d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065zM15 12a3 3 0 11-6 0 3 3 0 016 0z" />,
    cc: <path d="M4 5h16a1 1 0 011 1v12a1 1 0 01-1 1H4a1 1 0 01-1-1V6a1 1 0 011-1zm5.5 5.2a1.8 1.8 0 100 3.6m7 0a1.8 1.8 0 110-3.6" />,
    pip: <path d="M19 11h-6a1 1 0 00-1 1v4a1 1 0 001 1h6a1 1 0 001-1v-4a1 1 0 00-1-1zM4 5h16a1 1 0 011 1v3M4 5a1 1 0 00-1 1v12a1 1 0 001 1h5" />,
    skipBack: <path d="M11 19l-7-7 7-7v14zm9 0l-7-7 7-7v14z" fill="currentColor" stroke="none" />,
    skipFwd: <path d="M13 5l7 7-7 7V5zM4 5l7 7-7 7V5z" fill="currentColor" stroke="none" />,
    list: <path d="M4 6h16M4 12h16M4 18h10" />,
    gauge: <path d="M12 14a2 2 0 002-2c0-1.1-2-5-2-5s-2 3.9-2 5a2 2 0 002 2zm0 0v0M5.6 17.6a9 9 0 1112.8 0" />,
    arrowLeft: <path d="M19 12H5m0 0l7 7m-7-7l7-7" />,
    expand: <path d="M4 8V5a1 1 0 011-1h3M16 4h3a1 1 0 011 1v3M20 16v3a1 1 0 01-1 1h-3M8 20H5a1 1 0 01-1-1v-3" />,
    forward10: <path d="M4 4v6h6M4 10a8 8 0 11-1 4" />,
    keyboard: <path d="M3 6h18a1 1 0 011 1v10a1 1 0 01-1 1H3a1 1 0 01-1-1V7a1 1 0 011-1zm4 4h.01M11 10h.01M15 10h.01M7 14h10" />,
    server: <path d="M4 5h16a1 1 0 011 1v3a1 1 0 01-1 1H4a1 1 0 01-1-1V6a1 1 0 011-1zm0 9h16a1 1 0 011 1v3a1 1 0 01-1 1H4a1 1 0 01-1-1v-3a1 1 0 011-1zm3-5h.01M7 17h.01" />,
    globe: <path d="M12 21a9 9 0 100-18 9 9 0 000 18zm0 0c2.5-2.5 2.5-15 0-18m0 18c-2.5-2.5-2.5-15 0-18M3.5 9h17M3.5 15h17" />,
  };
  const filled = name === 'play' || name === 'star';
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none"
      stroke={filled ? 'none' : 'currentColor'} strokeWidth={sw}
      strokeLinecap="round" strokeLinejoin="round" className={className} style={style} aria-hidden="true">
      {paths[name]}
    </svg>
  );
};

window.Icon = Icon;
