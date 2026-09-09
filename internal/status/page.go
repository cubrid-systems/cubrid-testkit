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
 /* A grouping table can be ninety-nine rows once the corpus is the whole tree.
    It scrolls inside its panel rather than pushing everything below it away. */
 .scroll.tall{max-height:22rem;overflow-y:auto}
 table{border-collapse:collapse;width:100%;min-width:34rem}
 th{text-align:left;font-weight:400;font-size:.7rem;letter-spacing:.09em;text-transform:uppercase;
    color:var(--ink-faint);padding:0 .9rem .35rem 0;border-bottom:1px solid var(--line)}
 td{padding:.28rem .9rem .28rem 0;border-bottom:1px solid var(--line-soft);vertical-align:top}
 tr:last-child td{border-bottom:0}
 .slot{color:var(--ink-dim);width:4.5rem}
 .lanecol{width:4.5rem}
 /* With lanes off every slot is in the same one, and a column that says the
    same word on every row is not information. It comes back when there are two. */
 body.onelane .lanecol{display:none}
 .slots{color:var(--ink-dim);white-space:nowrap}
 /* The elastic column: it takes what is left and ellipsizes rather than pushing
    the numbers off the edge. max-width:0 with width:100% is what makes a table
    cell do that -- but only the body cells. On a header it collapses the word
    itself, which is how "running" came to read "run…" and made the whole table
    look misaligned. */
 td.case{width:100%;max-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
 th.case{width:100%;white-space:nowrap}
 .num{text-align:right;width:5rem;color:var(--ink-dim)}
 .empty{color:var(--ink-faint);padding:.4rem 0}
 /* A failing case provokes one question -- why -- and feedback.log already has
    the answer, so the case name is a link to it. */
 .case a{color:inherit;text-decoration:none;border-bottom:1px dotted var(--line)}
 .case a:hover{color:var(--accent);border-bottom-color:var(--accent)}
 .detail{margin:0;padding:.7rem .9rem;background:var(--line-soft);border-radius:3px;
   overflow:auto;max-height:26rem;font-size:.72rem;line-height:1.45;white-space:pre}

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
 /* A finished run played back looks exactly like one happening now, and
    mistaking the first for the second costs an afternoon. */
 /* The replay's knobs sit where the machine panel would be: a finished run has
    no machine worth reporting, and a scrub bar is the thing you reach for. */
 .rpbar{display:flex;align-items:center;gap:.5rem;flex-wrap:wrap}
 .rpbtn{appearance:none;background:none;border:1px solid var(--line);border-radius:3px;
   color:var(--ink-dim);cursor:pointer;font:inherit;font-size:.7rem;letter-spacing:.06em;
   padding:.15rem .55rem}
 .rpbtn:hover{color:var(--ink);border-color:var(--ink-faint)}
 .rpbtn[aria-pressed=true]{background:var(--line);color:var(--accent)}
 .seg .rpbtn{border:0;border-radius:0}
 .seg .rpbtn+.rpbtn{border-left:1px solid var(--line)}
 #rpseek{flex:1 1 14rem;min-width:8rem;accent-color:var(--accent)}
 .replay{font-size:.68rem;letter-spacing:.12em;text-transform:uppercase;
   padding:.08rem .45rem;border-radius:2px;color:var(--warn);
   border:1px solid color-mix(in srgb,var(--warn) 45%,transparent)}

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
 /* A table three-across has about 26rem, and the global 34rem floor made it
    overflow -- so nok, total and worst were cut off the right-hand edge and the
    panel looked like it had two columns. Inside a panel a table sizes to what it
    holds; the full-width tables below keep the floor. */
 .panels table{min-width:0}
 .panels .num{width:auto;min-width:2.6rem;padding-right:.6rem}
 .panels th,.panels td{padding-right:.6rem}
 /* The machine panel is an instrument rather than a summary, so it gets a row
    of its own and the numbers are grouped by the question they answer. */
 .mgrid{display:grid;grid-template-columns:repeat(auto-fit,minmax(13rem,1fr));gap:.9rem 1.6rem}
 .mg h3{margin:0 0 .3rem;font-size:.66rem;font-weight:600;letter-spacing:.12em;
   text-transform:uppercase;color:var(--ink-faint)}
 .mg table{min-width:0}
 .mg td{border:0;padding:.1rem .7rem .1rem 0;white-space:nowrap}
 .mg td:first-child{color:var(--ink-faint);width:5.5rem}
 .mg td.warn{color:var(--warn)}
 /* A bar under a number says how full it is without needing a second glance. */
 .mbar{height:4px;background:var(--line);border-radius:2px;overflow:hidden;margin-top:.25rem}
 .mbar i{display:block;height:100%;background:var(--accent)}
 .mbar i.warn{background:var(--warn)}
 table.kv td{border:0;padding:.15rem .9rem .15rem 0}
 table.kv td:first-child{color:var(--ink-faint);width:9rem}
 table.kv td.warn{color:var(--warn)}

 /* A verdict from patched source is a different claim from a verdict about the
    corpus. It is marked wherever the verdict is shown, and marked in the warning
    hue rather than a decorative one, because it is a caveat and not a feature. */
 .patched{font-size:.68rem;letter-spacing:.06em;text-transform:uppercase;
          color:var(--warn);border:1px solid var(--warn);border-radius:2px;
          padding:0 .28rem;margin-left:.4rem;opacity:.85;white-space:nowrap}
 /* A refused patch is worse than a patched case: the case ran as neither the
    corpus nor the patch has it, and without a mark of its own it looks like an
    ordinary failure. */
 .patched.bad{color:var(--fail);border-color:var(--fail);opacity:1}
 /* An idle slot is a fact, not a fault: it is between cases, or the admission
    policy is holding the queue back. Dimmed so the busy rows carry the eye. */
 tr.idle td{color:var(--ink-faint)}
 /* Which patch, above the case's own output, because a caveat the reader cannot
    follow up is half a caveat. */
 .detailpatch{font-size:.75rem;margin:0 0 .5rem;padding:.35rem .6rem;
              border-left:2px solid var(--warn);color:var(--ink-dim);
              background:var(--line-soft);word-break:break-all}
 .detailpatch b{color:var(--warn);font-weight:600}
 .detailpatch.bad{border-left-color:var(--fail)} .detailpatch.bad b{color:var(--fail)}

 /* Three groups side by side, each a narrow key/value list. They are read by
    scanning for the one line that is not the default, so the changed rows carry
    the only colour in the panel. */
 .setupgrid{display:grid;gap:.5rem 2.4rem;grid-template-columns:repeat(auto-fit,minmax(17rem,1fr))}
 .setupgrid h3{font-size:.68rem;font-weight:600;letter-spacing:.11em;text-transform:uppercase;
               color:var(--ink-faint);margin:0 0 .3rem;padding-bottom:.25rem;
               border-bottom:1px solid var(--line)}
 .setupgrid table{width:100%;min-width:0}
 .setupgrid td{border:0;padding:.14rem .7rem .14rem 0;vertical-align:baseline}
 .setupgrid td.k{color:var(--ink-faint);white-space:nowrap}
 .setupgrid td.v{color:var(--ink);word-break:break-all}
 .setupgrid tr.changed td.v{color:var(--warn)}
 .setupgrid tr.changed td.k{color:var(--warn);opacity:.75}
 .setupgrid .was{color:var(--ink-faint);font-size:.92em}

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
  <span id=replay class=replay hidden>replay</span>
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

<section class=panel id=replaywrap hidden style="margin-bottom:1.6rem">
  <h2>replay <span class=count id=rpwhen></span></h2>
  <div class=rpbar>
    <button id=rpplay class=rpbtn title="space">pause</button>
    <button class="rpbtn step" data-step="-60">&laquo; 1m</button>
    <button class="rpbtn step" data-step="-10">&laquo; 10s</button>
    <button class="rpbtn step" data-step="10">10s &raquo;</button>
    <button class="rpbtn step" data-step="60">1m &raquo;</button>
    <input id=rpseek type=range min=0 max=1000 value=0 aria-label="position in the run">
    <span class=seg role=group aria-label="speed">
      <button class="rpbtn speed" data-speed="1">1&times;</button
      ><button class="rpbtn speed" data-speed="10">10&times;</button
      ><button class="rpbtn speed" data-speed="60">60&times;</button
      ><button class="rpbtn speed" data-speed="600">600&times;</button
      ><button class="rpbtn speed" data-speed="3600">3600&times;</button>
    </span>
  </div>
</section>

<section class=panel id=machinewrap style="margin-bottom:1.6rem">
  <h2>machine <span class=count id=mwhen>every second</span></h2>
  <div class=mgrid id=machine></div>
</section>

<section class=panel id=setupwrap hidden style="margin-bottom:1.6rem">
  <h2>configuration <span class=count id=setupwhen>what the run was told to do</span></h2>
  <div class=setupgrid id=setup></div>
</section>

<div class=panels>
  <section class=panel>
    <h2>how long cases take <span class=count>cases &middot; share of time</span></h2>
    <div id=hist class=hist></div>
  </section>
</div>

<div class=panels>
  <section class=panel>
    <h2>lanes <span class=count>where the writes go</span></h2>
    <div class=scroll><table>
      <thead><tr><th>lane<th>slots<th class=num>done<th class=num>nok<th class=num>total<th class=num>share</tr></thead>
      <tbody id=lanes><tr><td colspan=6 class=empty>no lane reported</tr></tbody>
    </table></div>
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
    <h2>by family <span class=count id=nfamily>slowest first</span></h2>
    <div class="scroll tall"><table>
      <thead><tr><th>family<th class=num>done<th class=num>nok<th class=num>total<th class=num>worst</tr></thead>
      <tbody id=family></tbody>
    </table></div>
  </section>
  <section class=panel>
    <h2>by slot <span class=count>every slot, from the moment it opens</span></h2>
    <div class="scroll tall"><table>
      <thead><tr><th>slot<th class=lanecol>lane<th class=num>done<th class=num>nok<th class=num>total<th class=num>worst</tr></thead>
      <tbody id=slot></tbody>
    </table></div>
  </section>
</div>

<section>
  <h2>slots <span id=nslots style="letter-spacing:0;text-transform:none"></span></h2>
  <div class=scroll><table>
    <thead><tr><th class="slot sortable" tabindex=0 data-sk=slot aria-sort=none>slot<th class=lanecol>lane<th class=case>running<th class="num sortable" tabindex=0 data-sk=held aria-sort=none>for<th class=num>plan</tr></thead>
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

<section id=detailwrap hidden>
  <h2>case detail <span class=count id=detailname></span>
    <span class=seg><button id=detailclose>close</button></span>
  </h2>
  <p id=detailpatch class=detailpatch hidden></p>
  <pre id=detail class=detail></pre>
</section>

<script>
const $ = id => document.getElementById(id)
const HELD = 120 // seconds before a slot is worth looking at

const secs = s => s == null ? '—'
  : s < 60 ? s + 's'
  : s < 3600 ? Math.floor(s/60) + 'm' + String(s%60).padStart(2,'0')
  : Math.floor(s/3600) + 'h' + String(Math.floor(s%3600/60)).padStart(2,'0')

// A case is named from its family down; the path above that is the same for
// every row and would push the part that differs off the edge.
//
// It cannot be "the last two segments", which is what this was. That happened to
// be right while the scenario root was _01_utility and a case sat at
// <family>/<case>/cases/, and it silently dropped the family the moment the
// whole corpus was run: _06_issues/_14_1h/bug_bts_12352 rendered as
// "_14_1h/bug_bts_12352", and a sub-family on its own says nothing about which
// family it is in.
//
// So it starts at the first segment that looks like a family -- _NN_something,
// which is the corpus's own convention and the same rule the Go side groups by.
// The title attribute keeps the full path either way.
const short = c => {
  const p = c.split('/cases/')[0].split('/')
  const at = p.findIndex(seg => /^_\d+_/.test(seg))
  return (at >= 0 ? p.slice(at) : p.slice(-2)).join('/')
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
// The slots table sorts on its own: it answers "which slot has been on
// something longest", which is a different question from the finished table's.
let slotKey = 'slot', slotDir = 1
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

// slot10 belongs after slot9: the names are slotN and a plain compare puts 10
// before 9. Returns the same sense as the other comparators here: positive when
// a should come first.
function slotLess(a, b) {
  const na = parseInt(String(a).replace(/\D+/g, ''), 10)
  const nb = parseInt(String(b).replace(/\D+/g, ''), 10)
  if (!isNaN(na) && !isNaN(nb) && na !== nb) return na < nb ? 1 : -1
  return a < b ? 1 : (a > b ? -1 : 0)
}

function sortSlotsBy(k) {
  slotDir = slotKey === k ? -slotDir : (k === 'held' ? -1 : 1)
  slotKey = k
  for (const th of document.querySelectorAll('th[data-sk]')) {
    th.setAttribute('aria-sort', th.dataset.sk !== k ? 'none' : (slotDir < 0 ? 'descending' : 'ascending'))
  }
  if (lastView) draw(lastView)
}
for (const th of document.querySelectorAll('th[data-sk]')) {
  th.onclick = () => sortSlotsBy(th.dataset.sk)
  th.onkeydown = e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); sortSlotsBy(th.dataset.sk) } }
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
    : rows.length + ' kept') + (showN && rows.length > showN ? ', showing ' + showN : '') +
    (v.npatched ? ' \u00b7 ' + v.npatched + ' patched' : '') +
    (v.nrefused ? ' \u00b7 ' + v.nrefused + ' patch refused' : '')
  $('recent').innerHTML = list.length ? list.map(r =>
    '<tr><td class=slot>' + r.slot +
    '<td class=case title="' + r.case + '"><a href="#" data-case="' + r.case + '">' + short(r.case) + '</a>' +
    (r.refused
       ? ' <span class="patched bad" title="the compatibility patch ' + esc(r.patch) +
         ' would not apply, so this case ran as neither the corpus nor the patch has it">patch refused</span>'
       : r.patch
       ? ' <span class=patched title="ran against ' + esc(r.patch) +
         ', so the verdict is about the patched case and not about the corpus">patched</span>'
       : '') +
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
  $('replay').hidden = !v.replaying
  const busy = (v.slots||[]).filter(x => x.case).length
  $('state').textContent = v.finished ? 'finished' : busy + ' running'
  document.title = v.total ? v.done + '/' + v.total + ' testkit' : 'testkit run'
  spark(v.rate||[], v.rateSpan)

  let slots = (v.slots||[]).slice()
  $('nslots').textContent = slots.length ? busy + ' of ' + slots.length + ' busy' : ''
  if (slotKey === 'held') {
    // Idle slots have no duration, so they sort to the end either way rather
    // than pretending to be the shortest-held.
    slots.sort((a, b) => (a.case ? 0 : 1) - (b.case ? 0 : 1) || slotDir * ((a.held||0) - (b.held||0)))
  } else {
    slots.sort((a, b) => slotDir * -slotLess(a.slot, b.slot))
  }
  // Every slot, busy or not. A table of only the busy ones makes a slot between
  // cases look like a slot that never gets anything, while the ones holding long
  // cases stay listed as the others come and go.
  $('slots').innerHTML = slots.length ? slots.map(s => {
    if (!s.case) {
      return '<tr class=idle><td class=slot>' + esc(s.slot) +
        '<td class=lanecol>' + esc(s.lane || '') +
        '<td class=case colspan=3>idle</tr>'
    }
    // Well past its plan is what a stuck case looks like while it is still
    // stuck, rather than at the timeout twenty minutes later.
    const over = s.plan > 0 && s.held > s.plan * 3
    return '<tr' + (s.held > HELD || over ? ' class=held' : '') +
      '><td class=slot>' + esc(s.slot) +
      '<td class=lanecol>' + esc(s.lane || '') +
      '<td class=case title="' + esc(s.case) + '">' + short(s.case) +
      '<td class=num>' + secs(s.held) +
      '<td class=num' + (over ? ' style="color:var(--fail)"' : '') + '>' +
        (s.plan > 0 ? secs(s.plan) : '\u2014') + '</tr>'
  }).join('')
    : '<tr><td colspan=5 class=empty>' + (v.finished ? 'all slots idle' : 'waiting for the first case') + '</tr>'

  // A replay is a finished run: the machine numbers would be this machine now,
  // not the one the run happened on, and a panel that says something true about
  // the wrong thing is worse than no panel.
  $('machinewrap').hidden = !!v.replaying
  replayBar(v.replay)
  machine(v.machine || {})
  hist(v.hist || [], v.histSecs || [], v.histEdge || [])
  $('nfamily').textContent = (v.family || []).length + ' groups, slowest first'
  groups('family', v.family || [], false)
  groups('slot', v.slot || [], true)
  document.body.classList.toggle('onelane', (v.lanes || []).length < 2)
  lanes(v.lanes || [])
  setup(v.setup || [])
  templates(v.templates)
  lastView = v
  draw(v)
}

const gb = mb => mb >= 1024 ? (mb/1024).toFixed(1) + ' GB' : mb + ' MB'

// The machine panel, grouped by the question each number answers.
//
// CPU is split because "load 50 on sixteen cores" said nothing until it was
// split -- most of it was iowait, which is a disk problem wearing a CPU
// costume. Disk is in both bytes and operations because those say different
// things: 300 MB/s in 300 operations is a stream, and in 30,000 it is thrashing.
// And memory names what is holding it, because a tmpfs shows up as shmem and
// the corpus row is the same megabytes seen from the run's side.
function machine(m) {
  const pct = x => (x == null ? '—' : x.toFixed(0) + '%')
  const mbs = x => (x == null ? '—' : (x >= 100 ? x.toFixed(0) : x.toFixed(1)) + ' MB/s')
  const ops = x => (x == null ? '—' : x >= 1000 ? (x/1000).toFixed(1) + 'k' : x.toFixed(0))

  const bar = (used, all, warn) => !all ? '' :
    '<div class=mbar><i class="' + (warn ? 'warn' : '') +
    '" style="width:' + Math.min(100, 100*used/all) + '%"></i></div>'

  const group = (title, rows, foot) =>
    '<div class=mg><h3>' + title + '</h3><table><tbody>' +
    rows.map(([k, v, warn]) => '<tr><td>' + k + '<td' + (warn ? ' class=warn' : '') + '>' + v + '</tr>').join('') +
    '</tbody></table>' + (foot || '') + '</div>'

  const busy = (m.cpuUser || 0) + (m.cpuSys || 0)
  const cpu = group('cpu', [
    ['busy', pct(busy), busy > 90],
    ['user', pct(m.cpuUser)],
    ['system', pct(m.cpuSys)],
    // iowait is the one that explains a load nobody ordered.
    ['iowait', pct(m.cpuIowait), (m.cpuIowait || 0) > 20],
  ], bar(busy, 100, busy > 90))

  const load = group('load', [
    ['1 min', (m.load1 == null ? '—' : m.load1.toFixed(2)), m.cores && m.load1 > m.cores],
    ['5 min', (m.load5 == null ? '—' : m.load5.toFixed(2))],
    ['15 min', (m.load15 == null ? '—' : m.load15.toFixed(2))],
    ['cores', m.cores + (m.procs ? '  ·  ' + m.procs + ' procs' : '')],
  ])

  const mem = group('memory', [
    ['used', gb(m.memUsed) + ' of ' + gb(m.memAll), m.memAll && m.memUsed > m.memAll * 0.9],
    ['free', gb(m.memFree)],
    ['cache', gb(m.memCache)],
    // A tmpfs is shmem, so this is the corpus overlay seen from the machine.
    ['tmpfs', gb(m.memShmem)],
  ], bar(m.memUsed, m.memAll, m.memAll && m.memUsed > m.memAll * 0.9))

  const diskRows = [
    ['read', mbs(m.readMBs) + '  ·  ' + ops(m.readIops) + ' IOPS'],
    ['write', mbs(m.writeMBs) + '  ·  ' + ops(m.writeIops) + ' IOPS'],
    ['free', m.corpus == null ? '—' : gb(m.corpus), m.corpus < 5120],
  ]
  if (m.swapAll) diskRows.push(['swap', gb(m.swapUsed) + ' of ' + gb(m.swapAll), m.swapUsed > 0])
  const disk = group('disk', diskRows)

  // The corpus tmpfs against the ceiling it was given. Shown even at zero:
  // zero is the value worth seeing, and hiding the row exactly then is what
  // the first version of this did.
  const corpus = m.ramCap ? group('corpus in tmpfs', [
    ['held', gb(m.ram) + ' of ' + gb(m.ramCap), m.ram > m.ramCap * 0.9],
    ['ceiling', gb(m.ramCap)],
    // Past 90% a case that runs out of space fails like a case that got the
    // wrong answer, and nothing else in the output would say so.
    ['headroom', gb(Math.max(0, m.ramCap - m.ram)), m.ram > m.ramCap * 0.9],
  ], bar(m.ram, m.ramCap, m.ram > m.ramCap * 0.9)) : ''

  $('machine').innerHTML = cpu + load + mem + disk + corpus
}

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

// The replay's knobs. Drawn from what the server says the position is, not from
// what the page last asked for -- the playback moves on its own between clicks,
// and a slider that showed the last click would drift away from the run.
let rpDragging = false
function replayBar(rp) {
  $('replaywrap').hidden = !rp
  if (!rp) return
  $('rpwhen').textContent = secs(rp.at) + ' of ' + secs(rp.span) + '  ·  ' + rp.speed + '\u00d7'
  $('rpplay').textContent = rp.paused ? 'play' : 'pause'
  if (!rpDragging) {
    $('rpseek').value = rp.span ? Math.round(1000 * rp.at / rp.span) : 0
  }
  document.querySelectorAll('.rpbtn.speed').forEach(b =>
    b.setAttribute('aria-pressed', String(Number(b.dataset.speed) === rp.speed)))
}

// One call for every knob: the server owns the position and answers with it.
function rpSend(q) { fetch('/replay?' + q).then(r => r.json()).then(replayBar).catch(() => {}) }

document.addEventListener('click', e => {
  const b = e.target.closest('.rpbtn')
  if (!b) return
  if (b.id === 'rpplay') return rpSend('paused=' + ($('rpplay').textContent === 'pause' ? '1' : '0'))
  if (b.dataset.step) return rpSend('step=' + b.dataset.step)
  if (b.dataset.speed) return rpSend('speed=' + b.dataset.speed)
})
document.addEventListener('keydown', e => {
  if (e.target.tagName === 'INPUT' || $('replaywrap').hidden) return
  if (e.key === ' ') { e.preventDefault(); $('rpplay').click() }
  if (e.key === 'ArrowLeft') rpSend('step=-10')
  if (e.key === 'ArrowRight') rpSend('step=10')
})

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

// The configuration is static, so it is drawn once and then left alone. A row
// whose value differs from the default the engine shipped is the row the reader
// came for, so it says what the default was rather than only that it changed.
var setupDrawn = false;
// Text into markup. Everything the page interpolates is a case path, a patch
// path or a configuration value -- none of it hostile, all of it capable of
// holding a character that ends an attribute early and silently swallows the
// rest of a row.
function esc(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;')
}

function setup(rows) {
  var wrap = document.getElementById('setupwrap');
  if (!rows || !rows.length) { wrap.hidden = true; return; }
  wrap.hidden = false;
  if (setupDrawn) return;
  setupDrawn = true;

  var order = [], byGroup = {};
  rows.forEach(function (r) {
    if (!byGroup[r.group]) { byGroup[r.group] = []; order.push(r.group); }
    byGroup[r.group].push(r);
  });
  var nch = 0;
  var html = order.map(function (g) {
    var body = byGroup[g].map(function (r) {
      var changed = r.default && r.value !== r.default;
      if (changed) nch++;
      var v = esc(r.value);
      if (changed) v += ' <span class=was>(default ' + esc(r.default) + ')</span>';
      else if (r.note) v += ' <span class=was>' + esc(r.note) + '</span>';
      return '<tr class="' + (changed ? 'changed' : '') + '">' +
             '<td class=k>' + esc(r.key) + '<td class=v>' + v + '</tr>';
    }).join('');
    return '<div><h3>' + esc(g) + '</h3><table><tbody>' + body + '</tbody></table></div>';
  }).join('');
  document.getElementById('setup').innerHTML = html;
  document.getElementById('setupwhen').textContent =
    nch ? nch + (nch === 1 ? ' value differs from the engine default' : ' values differ from the engine defaults')
        : 'what the run was told to do';
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
// Clicking a finished case shows what feedback.log recorded for it: its checks
// and, when it failed, the console output of the run.
document.addEventListener('click', e => {
  const a = e.target.closest('a[data-case]')
  if (!a) return
  e.preventDefault()
  const name = a.getAttribute('data-case')
  $('detailname').textContent = short(name)
  // Which patch this verdict is about, said above the output rather than only in
  // a tooltip on a table that may already be scrolled away.
  const row = ((lastView && (lastView.recent || [])).concat((lastView && lastView.failed) || []))
    .find(r => r.case === name)
  const box = $('detailpatch')
  if (row && row.patch) {
    box.hidden = false
    box.classList.toggle('bad', !!row.refused)
    box.innerHTML = row.refused
      ? '<b>patch refused</b> \u2014 ' + esc(row.patch) +
        ' would not apply, so this case ran as neither the corpus nor the patch has it'
      : '<b>patched</b> \u2014 ran against ' + esc(row.patch) +
        ', so this verdict is about the patched case and not about the corpus'
  } else {
    box.hidden = true
  }
  $('detail').textContent = 'loading…'
  $('detailwrap').hidden = false
  $('detailwrap').scrollIntoView({block: 'nearest'})
  fetch('/case?name=' + encodeURIComponent(name))
    .then(r => r.text())
    .then(t => { $('detail').textContent = t })
    .catch(err => { $('detail').textContent = String(err) })
})
$('detailclose').addEventListener('click', () => { $('detailwrap').hidden = true })

// Dragging the scrub bar seeks; while a finger is down the poll must not fight
// it for the slider's value.
$('rpseek').addEventListener('input', () => { rpDragging = true })
$('rpseek').addEventListener('change', e => {
  rpDragging = false
  const span = Number(($('rpwhen').dataset.span) || 0)
  rpSend('seek=' + (Number(e.target.value) / 1000 * (lastView && lastView.replay ? lastView.replay.span : 0)))
})

tick(); setInterval(tick, 1000)
</script>
`
