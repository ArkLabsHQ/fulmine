import 'htmx.org'

const prettyNum = (num, maximumFractionDigits = 2) => {
  if (!num) return '0'
  return new Intl.NumberFormat('en', {
    style: 'decimal',
    maximumFractionDigits,
  }).format(num)
}

const fromSatoshis = (sats) => {
  return Decimal.div(sats, 100_000_000).toNumber()
}

window.onload = () => {
  document.querySelectorAll('.sats').forEach((x) => {
    x.innerHTML = prettyNum(x.textContent)
  })

  document.querySelectorAll('.btc').forEach((x) => {
    x.innerHTML = fromSatoshis(x.textContent)
  })

  document.querySelectorAll('.btcm').forEach((x) => {
    const sats = x.textContent
    x.innerHTML = (sats.charAt(0) === '+' ? '+' : '') + fromSatoshis(sats)
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
