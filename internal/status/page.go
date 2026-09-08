package status

// page is the whole of it: one file, no build step, no dependency to keep
// current. A test harness that needs its own toolchain to show a progress bar
// has bought a second thing to maintain.
const page = `<!doctype html>
<meta charset="utf-8">
<title>testkit</title>
<style>
 body{font:14px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;margin:0;padding:1.5rem;
      background:#111;color:#ddd}
 h1{font-size:1rem;font-weight:600;margin:0 0 1rem;color:#888}
 .n{font-size:2rem;font-weight:600}
 .row{display:flex;gap:2.5rem;align-items:baseline;margin-bottom:1.2rem;flex-wrap:wrap}
 .ok{color:#7c9}
 .nok{color:#d77}
 .bar{height:6px;background:#222;border-radius:3px;overflow:hidden;margin-bottom:1.5rem}
 .bar div{height:100%;background:#7c9;transition:width .4s}
 table{border-collapse:collapse;width:100%;margin-bottom:2rem}
 th{text-align:left;font-weight:400;color:#777;padding:.25rem .8rem .25rem 0;
    border-bottom:1px solid #262626}
 td{padding:.2rem .8rem .2rem 0;border-bottom:1px solid #1a1a1a;
    white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
 td.c{max-width:56ch}
 .slow{color:#dc8}
 .dim{color:#666}
</style>
<h1>testkit</h1>
<div class=row>
  <div><div class=n id=done>-</div><div class=dim>of <span id=total>-</span></div></div>
  <div><div class="n ok" id=ok>-</div><div class=dim>ok</div></div>
  <div><div class="n nok" id=nok>-</div><div class=dim>nok</div></div>
  <div><div class=n id=elapsed>-</div><div class=dim>elapsed</div></div>
  <div><div class=n id=remain>-</div><div class=dim>remaining</div></div>
</div>
<div class=bar><div id=fill style=width:0%></div></div>

<table><thead><tr><th>slot<th>running<th>held</tr></thead><tbody id=slots></tbody></table>
<table><thead><tr><th>slot<th>done<th></th><th>took</tr></thead><tbody id=recent></tbody></table>

<script>
const $ = id => document.getElementById(id)
const secs = s => s == null ? '-' :
  s < 60 ? s + 's' : Math.floor(s/60) + 'm' + String(s%60).padStart(2,'0')
const short = c => c.replace(/^.*\/scenario\//, '').replace(/\/cases\/[^/]*$/, '')

async function tick() {
  let v
  try { v = await (await fetch('api')).json() } catch (e) { return }
  $('done').textContent = v.done
  $('total').textContent = v.total
  $('ok').textContent = v.ok
  $('nok').textContent = v.nok
  $('elapsed').textContent = secs(v.elapsed)
  $('remain').textContent = v.finished ? 'done' : secs(v.remain)
  $('fill').style.width = (v.total ? 100*v.done/v.total : 0) + '%'

  $('slots').innerHTML = (v.slots||[]).map(s =>
    '<tr><td>' + s.slot + '<td class="c' + (s.held > 120 ? ' slow' : '') + '">' +
    short(s.case) + '<td' + (s.held > 120 ? ' class=slow' : '') + '>' + secs(s.held) +
    '</tr>').join('') || '<tr><td colspan=3 class=dim>idle</tr>'

  $('recent').innerHTML = (v.recent||[]).map(r =>
    '<tr><td>' + r.slot + '<td class=c>' + short(r.case) +
    '<td class=' + (r.ok ? 'ok>OK' : 'nok>NOK') + '<td>' + secs(r.took) + '</tr>').join('')
}
tick(); setInterval(tick, 1000)
</script>
`
