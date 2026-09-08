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
 .lanecol{width:4.5rem}
 .slots{color:var(--ink-dim);white-space:nowrap}
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
 .panels{display:grid;grid-template-columns:repeat(auto-fit,minmax(20rem,1fr));gap:1.6rem;margin-bottom:2rem}
 .panel{min-width:0}
 table.kv td{border:0;padding:.15rem .9rem .15rem 0}
 table.kv td:first-child{color:var(--ink-faint);width:9rem}
 table.kv td.warn{color:var(--warn)}

 /* The distribution is bars rather than a curve: the buckets are the shape,
    and a reader wants to know how many cases sit in each rather than to read a
    value off an axis. */
 .hist{display:flex;flex-direction:column;gap:.18rem}
 .hrow{display:flex;align-items:center;gap:.6rem}
 .hrow span:first-child{width:5rem;text-align:right;color:var(--ink-faint);font-size:.72rem}
 .hrow i{display:block;height:9px;background:var(--accent);opacity:.75;border-radius:1px;min-width:1px}
 .hrow span:last-child{color:var(--ink-dim);font-size:.72rem}
 /* The histogram carries two numbers now -- how many cases, and how much of
    the run's time they are -- because a lane split is a threshold on the
    second and the first cannot show where to put it. */
 .hrow .n{width:2.6rem;text-align:right}
 .hrow .sh{width:2.8rem;text-align:right;color:var(--ink-faint)}
 .hrow.heavy .sh{color:var(--accent)}
 .hrow.heavy i{opacity:1}

 /* A lane is where a slot's corpus writes go. Shown as a badge because it is a
    property of the slot rather than a measurement of it, and because the
    question it answers -- is the stuck slot the one holding memory -- is asked
    by glancing at the rail. */
 .lane{display:inline-block;font-size:.66rem;letter-spacing:.08em;text-transform:uppercase;
   padding:.02rem .34rem;border-radius:2px;border:1px solid var(--line);color:var(--ink-faint)}
 .lane.tmpfs{color:var(--accent);border-color:color-mix(in srgb,var(--accent) 45%,transparent)}
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

<div class=panels>
  <section class=panel>
    <h2>machine</h2>
    <table class=kv><tbody id=machine></tbody></table>
  </section>
  <section class=panel>
    <h2>how long cases take <span class=count>cases &middot; share of time</span></h2>
    <div id=hist class=hist></div>
  </section>
  <section class=panel id=tplpanel hidden>
    <h2>template cache <span class=count id=tplwhere></span></h2>
    <table class=kv><tbody id=tplkv></tbody></table>
    <div class=scroll><table style="margin-top:.5rem">
      <thead><tr><th>most used<th class=num>used<th class=num>size<th>built from</tr></thead>
      <tbody id=tpltop></tbody>
    </table></div>
  </section>
  <section class=panel>
    <h2>lanes <span class=count>where the writes go</span></h2>
    <div class=scroll><table>
      <thead><tr><th>lane<th>slots<th class=num>done<th class=num>nok<th class=num>total<th class=num>share</tr></thead>
      <tbody id=lanes><tr><td colspan=6 class=empty>no lane reported</tr></tbody>
    </table></div>
  </section>
</div>

<div class=panels>
  <section class=panel>
    <h2>by family <span class=count>slowest first</span></h2>
    <div class=scroll><table>
      <thead><tr><th>family<th class=num>done<th class=num>nok<th class=num>total<th class=num>worst</tr></thead>
      <tbody id=family></tbody>
    </table></div>
  </section>
  <section class=panel>
    <h2>by slot <span class=count>every slot, from the moment it opens</span></h2>
    <div class=scroll><table>
      <thead><tr><th>slot<th class=lanecol>lane<th class=num>done<th class=num>nok<th class=num>total<th class=num>worst</tr></thead>
      <tbody id=slot></tbody>
    </table></div>
  </section>
</div>

<section>
  <h2>slots <span id=nslots style="letter-spacing:0;text-transform:none"></span></h2>
  <div class=scroll><table>
    <thead><tr><th class=slot>slot<th class=lanecol>lane<th class=case>running<th class=num>for</tr></thead>
    <tbody id=slots><tr><td colspan=4 class=empty>waiting for the first case</tr></tbody>
  </table></div>
</section>

<section>
  <h2>finished
    <span class=seg role=group aria-label="which cases">
      <button id=fAll aria-pressed=true>recent</button><button id=fBad aria-pressed=false>failed</button>
    </span>
    <span class=seg role=group aria-label="how many">
      <button class=nbtn data-n=10>10</button><button class=nbtn data-n=40 aria-pressed=true>40</button
      ><button class=nbtn data-n=100>100</button><button class=nbtn data-n=0>all</button>
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
let showFailed = false, sortKey = null, sortDir = -1, showN = 40
let lastView = null

for (const b of document.querySelectorAll('.nbtn')) {
  b.onclick = () => {
    showN = +b.dataset.n
    for (const o of document.querySelectorAll('.nbtn'))
      o.setAttribute('aria-pressed', String(o === b))
    if (lastView) draw(lastView)
  }
}

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
  let list = sortKey ? rows.slice().sort((a, b) =>
    sortDir * (sortKey === 'took' ? a.took - b.took : short(a.case).localeCompare(short(b.case)))) : rows
  if (showN) list = list.slice(0, showN)
  $('fcount').textContent = (showFailed
    ? (v.failed || []).length + ' failed of ' + v.done
    : rows.length + ' kept') + (showN && rows.length > showN ? ', showing ' + showN : '')
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

  machine(v.machine || {})
  hist(v.hist || [], v.histSecs || [], v.histEdge || [])
  groups('family', v.family || [], false)
  groups('slot', v.slot || [], true)
  lanes(v.lanes || [])
  templates(v.templates)
  lastView = v
  draw(v)
}

const gb = mb => mb >= 1024 ? (mb/1024).toFixed(1) + ' GB' : mb + ' MB'

function machine(m) {
  const rows = []
  const hot = m.cores && m.load > m.cores
  rows.push(['load', m.load == null ? '—' : m.load.toFixed(2) + ' of ' + m.cores + ' cores', hot])
  if (m.memAll) rows.push(['memory', gb(m.memUsed) + ' of ' + gb(m.memAll),
                           m.memUsed > m.memAll * 0.9])
  // Shown even at zero. Zero is the value worth seeing, and hiding the row
  // exactly then is what the first version of this did.
  if (m.ramCap) rows.push(['corpus tmpfs', gb(m.ram) + ' of ' + gb(m.ramCap),
                           m.ram > m.ramCap * 0.9])
  rows.push(['disk free', m.corpus == null ? '—' : gb(m.corpus), m.corpus < 5120])
  $('machine').innerHTML = rows.map(([k, val, warn]) =>
    '<tr><td>' + k + '<td' + (warn ? ' class=warn' : '') + '>' + val + '</tr>').join('')
}

// A lane is one word, and an unset one is nothing rather than a placeholder.
const lane = l => l ? '<span class="lane ' + l + '">' + l + '</span>' : ''

// The cache is off in most runs, so the panel is absent rather than empty: a
// panel of zeroes reads as "nothing is hitting" when the truth is "nobody asked
// for a cache".
function templates(t) {
  $('tplpanel').hidden = !t
  if (!t) return
  $('tplwhere').textContent = t.dir
  const asked = (t.restored || 0) + (t.built || 0)
  const hit = asked ? Math.round(100 * t.restored / asked) : 0
  const rows = [
    ['store', t.count + ' templates, ' + gb(t.mb) + (t.capMB ? ' of ' + gb(t.capMB) : ''),
     t.capMB && t.mb > t.capMB * 0.9],
    // Restored against built is the hit rate, which is the number the cache
    // exists for -- and the one that says whether it is earning its keep.
    ['this run', t.restored + ' restored, ' + t.built + ' built' + (asked ? '  (' + hit + '% hit)' : ''), false],
  ]
  $('tplkv').innerHTML = rows.map(([k, val, warn]) =>
    '<tr><td>' + k + '<td' + (warn ? ' class=warn' : '') + '>' + val + '</tr>').join('')
  const top = t.top || []
  $('tpltop').innerHTML = top.length ? top.map(r =>
    '<tr><td class=case title="' + r.key + '">' + r.key.slice(0, 12) +
    '<td class=num>' + r.refs +
    '<td class=num>' + gb(r.mb) +
    '<td class=case title="' + (r.origin || '') + '">' + (r.origin || '') + '</tr>').join('')
    : '<tr><td colspan=4 class=empty>the store is empty</tr>'
}

function lanes(ls) {
  $('lanes').innerHTML = ls.length ? ls.map(l =>
    '<tr><td>' + lane(l.name) +
    '<td class=slots title="' + l.nslots + ' slots">' + l.slots +
    '<td class=num>' + l.done +
    '<td class=num>' + (l.nok ? '<span class="v no">' + l.nok + '</span>' : '') +
    '<td class=num>' + secs(l.secs) +
    '<td class=num>' + l.share + '%</tr>').join('')
    : '<tr><td colspan=6 class=empty>no lane reported</tr>'
}

// Two numbers a bucket: how many cases are in it, and how much of the run's
// time they are between them. The bar stays the count, because that is the
// distribution the panel is named for; the share is what a lane threshold is
// chosen from, and a bucket holding a fifth of the run is marked so it can be
// found without reading every row.
function hist(h, secsIn, edges) {
  const max = Math.max(1, ...h)
  const total = secsIn.reduce((a, b) => a + b, 0)
  const label = i => i === 0 ? '< ' + edges[0] + 's'
    : i < edges.length ? edges[i-1] + '–' + edges[i] + 's'
    : '> ' + edges[edges.length-1] + 's'
  $('hist').innerHTML = h.map((n, i) => {
    const share = total ? Math.round(100 * (secsIn[i] || 0) / total) : 0
    return '<div class="hrow' + (share >= 20 ? ' heavy' : '') + '"><span>' + label(i) + '</span>' +
      '<i style="width:' + (100 * n / max) + '%"></i>' +
      '<span class=n>' + (n || '') + '</span>' +
      '<span class=sh>' + (n ? share + '%' : '') + '</span></div>'
  }).join('')
}

function groups(id, gs, withLane) {
  const cols = withLane ? 6 : 5
  $(id).innerHTML = gs.length ? gs.map(g =>
    '<tr><td class=case title="' + g.name + '">' + g.name +
    (withLane ? '<td class=lanecol>' + lane(g.lane) : '') +
    // A slot that has opened but finished nothing shows a dash rather than a
    // zero: nothing has happened there yet, which is different from none.
    '<td class=num>' + (g.done || '—') +
    '<td class=num>' + (g.nok ? '<span class="v no">' + g.nok + '</span>' : '') +
    '<td class=num>' + (g.done ? secs(g.secs) : '') +
    '<td class=num>' + (g.done ? secs(g.max) : '') + '</tr>').join('')
    : '<tr><td colspan=' + cols + ' class=empty>nothing yet</tr>'
}
tick(); setInterval(tick, 1000)
</script>
`
