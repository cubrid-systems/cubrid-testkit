package status

// page is the whole of it: one file, no build step, no dependency to keep
// current. A test harness that needs its own toolchain to draw a progress bar
// has bought a second thing to maintain.
//
// No web fonts, and that is the subject rather than laziness: a slot runs in a
// network namespace with no route off the machine, and a QA host often has no
// route off the site. A font that has to be fetched is a font that silently
// does not arrive, so the stack is the one already on the machine.
//
// One theme, deliberately. This is a screen someone leaves open beside a
// terminal while a run of hours goes past, and it is the only thing it is;
// every colour is stated rather than inherited, so it holds whatever ground the
// browser paints behind it.
const page = `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>testkit run</title>
<style>
 :root{
   /* Neutrals carry a slight blue bias so the ground reads as chosen rather
      than as an unedited grey. */
   --bg:#0e1013; --panel:#14171c; --line:#232830; --line-soft:#1a1e24;
   --ink:#dfe3e8; --ink-dim:#7d8794; --ink-faint:#4d5561;
   /* The accent is the run itself: progress, the rate, the things that are
      about how far along it is. Verdicts get their own hues, because a page
      that paints "passing" and "progress" the same colour cannot show one
      without the other. */
   --accent:#5b9dd9;
   --pass:#63b389; --fail:#d3766e; --warn:#d9a75b;
 }
 *{box-sizing:border-box}
 body{margin:0;background:var(--bg);color:var(--ink);
      font:13px/1.55 ui-monospace,SFMono-Regular,"SF Mono",Menlo,Consolas,monospace;
      font-variant-numeric:tabular-nums;padding:1.4rem 1.6rem 3rem}
 header{display:flex;align-items:baseline;gap:.9rem;margin-bottom:1.4rem}
 h1{font-size:.8rem;font-weight:600;letter-spacing:.14em;text-transform:uppercase;
    color:var(--ink-dim);margin:0}
 #state{font-size:.75rem;letter-spacing:.1em;text-transform:uppercase;
        color:var(--accent);padding:.1rem .5rem;border:1px solid var(--line);border-radius:2px}

 .figures{display:flex;gap:2.6rem;align-items:flex-end;flex-wrap:wrap;margin-bottom:1rem}
 .fig{display:flex;flex-direction:column;gap:.15rem}
 .fig b{font-size:1.9rem;font-weight:600;line-height:1;letter-spacing:-.02em}
 .fig span{font-size:.7rem;letter-spacing:.11em;text-transform:uppercase;color:var(--ink-faint)}
 .fig.pass b{color:var(--pass)} .fig.fail b{color:var(--fail)}
 .fig .of{font-size:.9rem;font-weight:400;color:var(--ink-faint)}
 .spark{margin-left:auto;display:flex;flex-direction:column;gap:.2rem;align-items:flex-end}

 .bar{height:4px;background:var(--line-soft);border-radius:2px;overflow:hidden;margin-bottom:1.8rem}
 .bar i{display:block;height:100%;width:0;background:var(--accent);transition:width .5s ease}

 section{margin-bottom:2rem}
 h2{font-size:.7rem;font-weight:600;letter-spacing:.13em;text-transform:uppercase;
    color:var(--ink-faint);margin:0 0 .5rem;display:flex;gap:.6rem;align-items:baseline}
 .scroll{overflow-x:auto}
 table{border-collapse:collapse;width:100%;min-width:34rem}
 th{text-align:left;font-weight:400;font-size:.7rem;letter-spacing:.09em;text-transform:uppercase;
    color:var(--ink-faint);padding:0 .9rem .35rem 0;border-bottom:1px solid var(--line)}
 td{padding:.28rem .9rem .28rem 0;border-bottom:1px solid var(--line-soft);vertical-align:top}
 tr:last-child td{border-bottom:0}
 .slot{color:var(--ink-dim);width:4.5rem}
 .case{width:100%;max-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
 .num{text-align:right;width:5rem;color:var(--ink-dim)}
 .empty{color:var(--ink-faint);padding:.4rem 0}

 /* What is interactive looks interactive: the toggle reads as a control at
    rest, not only once the pointer is over it. */
 .seg{display:flex;gap:0;border:1px solid var(--line);border-radius:3px;overflow:hidden}
 .seg button{appearance:none;background:none;border:0;color:var(--ink-dim);cursor:pointer;
   font:inherit;font-size:.7rem;letter-spacing:.1em;text-transform:uppercase;
   padding:.15rem .6rem}
 .seg button+button{border-left:1px solid var(--line)}
 .seg button[aria-pressed=true]{background:var(--line);color:var(--ink)}
 .seg button:focus-visible,th.sortable:focus-visible{outline:2px solid var(--accent);outline-offset:1px}
 th.sortable{cursor:pointer;user-select:none}
 th.sortable:hover{color:var(--ink-dim)}
 th.sortable[aria-sort]:not([aria-sort=none]){color:var(--accent)}
 th .caret{opacity:.55;font-size:.85em}
 .count{letter-spacing:0;text-transform:none;color:var(--ink-faint)}

 /* A slot that has held a case a long time is the thing this page exists to
    surface, so it is marked in shape as well as colour: the row gains a rail. */
 tr.held td:first-child{box-shadow:inset 3px 0 0 var(--warn);padding-left:.55rem}
 tr.held .num{color:var(--warn)}
 .v{display:inline-block;min-width:2.6rem;font-size:.72rem;letter-spacing:.07em;
    padding:.02rem .35rem;border-radius:2px;text-align:center}
 .v.ok{color:var(--pass);background:rgba(99,179,137,.13)}
 .v.no{color:var(--fail);background:rgba(211,118,110,.15)}
 @media (prefers-reduced-motion:reduce){*{transition:none!important}}
</style>

<header>
  <h1>testkit</h1>
  <span id=state>starting</span>
</header>

<div class=figures>
  <div class=fig><b><span id=done>0</span><span class=of>/<span id=total>0</span></span></b><span>cases</span></div>
  <div class="fig pass"><b id=ok>0</b><span>ok</span></div>
  <div class="fig fail"><b id=nok>0</b><span>nok</span></div>
  <div class=fig><b id=elapsed>0s</b><span>elapsed</span></div>
  <div class=fig><b id=remain>&mdash;</b><span>remaining</span></div>
  <div class=spark>
    <svg id=rate width=180 height=34 viewBox="0 0 180 34" aria-label="cases finished per interval"></svg>
    <span style="font-size:.68rem;letter-spacing:.1em;text-transform:uppercase;color:var(--ink-faint)"
      >rate &middot; <span id=ratenow>0</span>/min</span>
  </div>
</div>
<div class=bar><i id=fill></i></div>

<section>
  <h2>slots <span id=nslots style="letter-spacing:0;text-transform:none"></span></h2>
  <div class=scroll><table>
    <thead><tr><th class=slot>slot<th class=case>running<th class=num>for</tr></thead>
    <tbody id=slots><tr><td colspan=3 class=empty>waiting for the first case</tr></tbody>
  </table></div>
</section>

<section>
  <h2>finished
    <span class=seg role=group aria-label="which cases">
      <button id=fAll aria-pressed=true>recent</button><button id=fBad aria-pressed=false>failed</button>
    </span>
    <span class=count id=fcount></span>
  </h2>
  <div class=scroll><table>
    <thead><tr>
      <th class=slot>slot
      <th class="case sortable" tabindex=0 data-k=case aria-sort=none>case
      <th class=num>verdict
      <th class="num sortable" tabindex=0 data-k=took aria-sort=none>took
    </tr></thead>
    <tbody id=recent><tr><td colspan=4 class=empty>nothing yet</tr></tbody>
  </table></div>
</section>

<script>
const $ = id => document.getElementById(id)
const HELD = 120 // seconds before a slot is worth looking at

const secs = s => s == null ? '—'
  : s < 60 ? s + 's'
  : s < 3600 ? Math.floor(s/60) + 'm' + String(s%60).padStart(2,'0')
  : Math.floor(s/3600) + 'h' + String(Math.floor(s%3600/60)).padStart(2,'0')

// A case is named by the family it is in and its own name; the path above that
// is the same for every row and would push the part that differs off the edge.
const short = c => {
  const p = c.split('/cases/')[0].split('/')
  return p.slice(-2).join('/')
}

function spark(series, span) {
  const el = $('rate'), W = 180, H = 34, n = 34
  const d = series.slice(-n)
  const per = span ? 60/span : 1
  $('ratenow').textContent = d.length ? Math.round(d[d.length-1]*per) : 0
  if (d.length < 2) { el.innerHTML = ''; return }
  const max = Math.max(1, ...d)
  const x = i => i * W/(d.length-1)
  const y = v => H - 2 - (v/max)*(H-5)
  const line = d.map((v,i) => (i?'L':'M') + x(i).toFixed(1) + ' ' + y(v).toFixed(1)).join(' ')
  const area = line + ' L' + W + ' ' + H + ' L0 ' + H + ' Z'
  el.innerHTML =
    '<path d="' + area + '" fill="var(--accent)" fill-opacity=".13"/>' +
    '<path d="' + line + '" fill="none" stroke="var(--accent)" stroke-width="1.5"/>' +
    '<circle cx="' + x(d.length-1).toFixed(1) + '" cy="' + y(d[d.length-1]).toFixed(1) +
    '" r="2.4" fill="var(--accent)"/>'
}

// The table is either the tail of the run or its failures, and it is sorted by
// what the reader picked -- both held across refreshes, or a list would reorder
// itself under the pointer once a second.
let showFailed = false, sortKey = null, sortDir = -1
let lastView = null

function pick(failed) {
  showFailed = failed
  $('fAll').setAttribute('aria-pressed', String(!failed))
  $('fBad').setAttribute('aria-pressed', String(failed))
  if (lastView) draw(lastView)
}
$('fAll').onclick = () => pick(false)
$('fBad').onclick = () => pick(true)

function sortBy(k) {
  sortDir = sortKey === k ? -sortDir : -1
  sortKey = k
  for (const th of document.querySelectorAll('th.sortable')) {
    th.setAttribute('aria-sort', th.dataset.k !== k ? 'none' : (sortDir < 0 ? 'descending' : 'ascending'))
    const base = th.dataset.k
    th.innerHTML = base + (th.dataset.k === k ? ' <span class=caret>' + (sortDir < 0 ? '\u25be' : '\u25b4') + '</span>' : '')
  }
  if (lastView) draw(lastView)
}
for (const th of document.querySelectorAll('th.sortable')) {
  th.onclick = () => sortBy(th.dataset.k)
  th.onkeydown = e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); sortBy(th.dataset.k) } }
}

function draw(v) {
  const rows = (showFailed ? v.failed : v.recent) || []
  const list = sortKey ? rows.slice().sort((a, b) =>
    sortDir * (sortKey === 'took' ? a.took - b.took : short(a.case).localeCompare(short(b.case)))) : rows
  $('fcount').textContent = showFailed
    ? (v.failed || []).length + ' of ' + v.done
    : 'last ' + Math.min(rows.length, 40)
  $('recent').innerHTML = list.length ? list.map(r =>
    '<tr><td class=slot>' + r.slot +
    '<td class=case title="' + r.case + '">' + short(r.case) +
    '<td class=num><span class="v ' + (r.ok?'ok':'no') + '">' + (r.ok?'OK':'NOK') + '</span>' +
    '<td class=num>' + secs(r.took) + '</tr>').join('')
    : '<tr><td colspan=4 class=empty>' + (showFailed ? 'nothing has failed' : 'nothing yet') + '</tr>'
}

async function tick() {
  let v
  try { v = await (await fetch('api')).json() } catch (e) { $('state').textContent = 'no answer'; return }

  $('done').textContent = v.done
  $('total').textContent = v.total
  $('ok').textContent = v.ok
  $('nok').textContent = v.nok
  $('elapsed').textContent = secs(v.elapsed)
  $('remain').textContent = v.finished ? '—' : (v.remain ? secs(v.remain) : '—')
  $('fill').style.width = (v.total ? 100*v.done/v.total : 0) + '%'
  $('state').textContent = v.finished ? 'finished' : (v.slots||[]).length + ' running'
  document.title = v.total ? v.done + '/' + v.total + ' testkit' : 'testkit run'
  spark(v.rate||[], v.rateSpan)

  const slots = v.slots||[]
  $('nslots').textContent = slots.length ? '' : ''
  $('slots').innerHTML = slots.length ? slots.map(s =>
    '<tr' + (s.held > HELD ? ' class=held' : '') + '><td class=slot>' + s.slot +
    '<td class=case title="' + s.case + '">' + short(s.case) +
    '<td class=num>' + secs(s.held) + '</tr>').join('')
    : '<tr><td colspan=3 class=empty>' + (v.finished ? 'all slots idle' : 'waiting for the first case') + '</tr>'

  lastView = v
  draw(v)
}
tick(); setInterval(tick, 1000)
</script>
`
