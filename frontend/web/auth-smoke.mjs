import { chromium } from 'playwright';
import fs from 'fs';
const login = JSON.parse(fs.readFileSync('/tmp/login2.json','utf8')).data;
const cookieLine = fs.readFileSync('/tmp/ae_c2.txt','utf8').split('\n').find(l => l.includes('refresh_token'));
const refreshToken = cookieLine.trim().split('\t').pop();
const browser = await chromium.launch();
const ctx = await browser.newContext();
await ctx.addCookies([{ name: 'refresh_token', value: refreshToken, domain: 'animeenigma.ru', path: '/', httpOnly: true, secure: true, sameSite: 'Lax' }]);
await ctx.addInitScript(([tok, user]) => {
  if (!localStorage.getItem('token')) { localStorage.setItem('token', tok); localStorage.setItem('user', user); }
}, [login.access_token, JSON.stringify(login.user)]);
const page = await ctx.newPage();
const errors = [];
page.on('response', r => { if ([401,429,500].includes(r.status())) errors.push(`HTTP ${r.status()} ${r.url().replace('https://animeenigma.ru','')}`); });
page.on('console', m => { if (m.type() === 'error') errors.push('console: ' + m.text().slice(0,120)); });
await page.goto('https://animeenigma.ru/', { waitUntil: 'networkidle' }).catch(e=>errors.push('nav: '+e.message));
await page.waitForTimeout(2500);
await page.goto('https://animeenigma.ru/user/ui-audit-bot', { waitUntil: 'networkidle' }).catch(e=>errors.push('nav: '+e.message));
await page.waitForTimeout(3000);
const tok = await page.evaluate(() => { const t = localStorage.getItem('token'); return t ? 'present len='+t.length : 'MISSING'; });
const loggedIn = await page.evaluate(() => !!localStorage.getItem('user'));
console.log('token:', tok, 'user:', loggedIn);
console.log('--- 401/429/500/console errors ---');
console.log(errors.length ? errors.join('\n') : 'NONE');
await browser.close();
