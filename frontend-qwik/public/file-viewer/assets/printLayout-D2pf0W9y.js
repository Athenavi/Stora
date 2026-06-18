var e=96,t=e=>!Number.isFinite(e)||e<=0?0:Number(e.toFixed(3)),n=e=>`${t(e)}px`,r=n=>`${Number((t(n)/e).toFixed(4))}in`,i=e=>{let t=parseFloat(e);return Number.isFinite(t)&&t>0?t:0},a=(e,n={})=>{let r=window.getComputedStyle(e),a=i(r.width)||e.offsetWidth||n.width||e.getBoundingClientRect().width,o=i(r.height)||i(r.minHeight)||e.offsetHeight||n.height||e.getBoundingClientRect().height;return{width:t(a),height:t(o)}},o=(e,t,r={})=>{let i=n(t.width),a=n(t.height),o=r.heightMode||`fixed`;e.classList.add(`viewer-print-page`),e.style.setProperty(`--viewer-print-page-width`,i),e.style.setProperty(`--viewer-print-page-height`,a),e.style.width=i,e.style.maxWidth=`none`,e.style.minHeight=a,o===`fixed`?(e.style.height=a,e.style.overflow=`hidden`):(e.style.height=`auto`,e.style.overflow=`visible`)},s=({selector:e,width:t,height:i,heightMode:a=`fixed`})=>{let o=n(t),s=n(i),c=a===`fixed`?`height:${s}!important;min-height:${s}!important;overflow:hidden!important;`:`height:auto!important;min-height:${s}!important;overflow:visible!important;`;return`
    @page { size: ${r(t)} ${r(i)}; margin: 0; }
    @media print {
      html, body {
        width: ${o};
        min-width: ${o};
        background: #ffffff !important;
      }
      ${e} {
        width: ${o}!important;
        max-width: none!important;
        ${c}
        margin: 0!important;
        box-shadow: none!important;
        border: 0!important;
        break-after: page;
        page-break-after: always;
      }
      ${e}:last-child {
        break-after: auto;
        page-break-after: auto;
      }
    }
  `};export{a as i,s as n,n as r,o as t};