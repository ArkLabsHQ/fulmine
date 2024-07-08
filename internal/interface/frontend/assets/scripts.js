import 'htmx.org'

const prettyNum = (num, maximumFractionDigits = 2) => {
  if (!num) return '0'
  return new Intl.NumberFormat('en', {
    style: 'decimal',
    maximumFractionDigits,
  }).format(num)
}

window.onload = () => {
  document.querySelectorAll('.sats').forEach((x) => {
    x.innerHTML = prettyNum(x.textContent)
  })

  fetch('https://btcoracle.bordalix.workers.dev/').then((res) => {
    res.json().then((json) => {
      document.querySelectorAll('.usd').forEach((x) => {
        const sats = parseInt(x.getAttribute('sats'))
        if (isNaN(sats)) return
        const usd = (sats * json.pricefeed.usd) / 100_000_000
        x.innerHTML = `$ ${prettyNum(usd)}`
      })
    })
  })
}
