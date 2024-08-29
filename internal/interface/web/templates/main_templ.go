// Code generated by templ - DO NOT EDIT.

// templ: version: v0.2.771
package templates

//lint:file-ignore SA4006 This context is only used if a nested component is present.

import "github.com/a-h/templ"
import templruntime "github.com/a-h/templ/runtime"

import "github.com/ArkLabsHQ/ark-node/internal/interface/web/types"

// in order for tailwind to detect this classes, this javascript code
// needs to be inside a .templ file, which means this script code
// must be here, cannot be on /assets/script.js
func PagesScript() templ.ComponentScript {
	return templ.ComponentScript{
		Name: `__templ_PagesScript_7a43`,
		Function: `function __templ_PagesScript_7a43(){// from 4321 to 0.00004321 with '0' in gray
  const prettyBtc = (btc) => {
    if (!/\d/.test(btc)) return data // not a number
    const prefix = /^[\D]/.test(btc) ? btc[0] : ''
    const number = /^[\D]/.test(btc) ? btc.slice(1) : btc
	  const length = String(number).length
    if (length > 8) return number
    let padding = '0.'
    for (let i = length; i < 8; i++) padding += '0'
	  return ` + "`" + `${prefix}<span class="text-gray-400">${padding}</span>${number}` + "`" + `
  }
}`,
		Call:       templ.SafeScript(`__templ_PagesScript_7a43`),
		CallInline: templ.SafeScriptInline(`__templ_PagesScript_7a43`),
	}
}

func runOnLoad() templ.ComponentScript {
	return templ.ComponentScript{
		Name: `__templ_runOnLoad_5bdd`,
		Function: `function __templ_runOnLoad_5bdd(){updateFiatValues()
	//
	document.querySelectorAll('select').forEach((x) => {
		// hack, or else select will not have current value selected
		if (x.getAttribute('value')) x.value = x.getAttribute('value')
	})

	const settings = JSON.parse(document.querySelector('#settings').textContent)
	if (settings.unit !== "sat") toggleUnit()
}`,
		Call:       templ.SafeScript(`__templ_runOnLoad_5bdd`),
		CallInline: templ.SafeScriptInline(`__templ_runOnLoad_5bdd`),
	}
}

func Layout(bodyContent templ.Component, settings types.Settings) templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var1 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var1 == nil {
			templ_7745c5c3_Var1 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<!doctype html><html lang=\"en\"><head><title>Ark node</title><meta charset=\"UTF-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><meta http-equiv=\"X-UA-Compatible\" content=\"ie=edge\"><meta name=\"keywords\" content=\"Ark\"><meta name=\"description\" content=\"Ark node frontend\"><meta name=\"theme-color\" content=\"#FEFEF5\"><link rel=\"dns-prefetch\" href=\"//fonts.googleapis.com\"><link rel=\"dns-prefetch\" href=\"//fonts.gstatic.com\"><link rel=\"preconnect\" href=\"//fonts.googleapis.com\" crossorigin><link rel=\"preconnect\" href=\"//fonts.gstatic.com\" crossorigin><link rel=\"preload\" href=\"/static/Switzer-Variable.17d61fe9.woff2\" as=\"font\" type=\"font/woff2\" crossorigin><link rel=\"preload\" href=\"/static/Switzer-Variable.f14a5147.woff\" as=\"font\" type=\"font/woff\" crossorigin><link rel=\"preload\" href=\"/static/Switzer-Variable.398e4d30.ttf\" as=\"font\" type=\"font/ttf\" crossorigin><link rel=\"manifest\" href=\"/static/manifest.webmanifest\"><link rel=\"apple-touch-icon\" href=\"/static/apple-touch-icon.png\"><link rel=\"shortcut icon\" href=\"/static/favicon.ico\" type=\"image/x-icon\"><link rel=\"icon\" href=\"/static/favicon.svg\" type=\"image/svg+xml\"><link rel=\"icon\" href=\"/static/favicon.png\" sizes=\"any\"><link href=\"/static/styles.css\" rel=\"stylesheet\"></head>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templ.RenderScriptItems(ctx, templ_7745c5c3_Buffer, runOnLoad())
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<body onload=\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var2 templ.ComponentScript = runOnLoad()
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var2.Call)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templ.JSONScript("settings", settings).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div id=\"app\"><div id=\"toast\"></div><div id=\"modal\"></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = bodyContent.Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("</div><script src=\"/static/decimal.js\"></script><script src=\"/static/scripts.js\"></script></body><script>\n\t\t  const toastTimeout = 2100\n\n\t\t  const disableButton = (button) => {\n\t\t\t\tbutton.classList.add(\"disabled\")\n      }\n\n\t\t\t// re enable buttons after receiving a htmx:load event\n\t\t\tconst enableButtons = () => {\n\t\t\t\tdocument.querySelectorAll('.disabled').forEach((x) => {\n\t\t\t\t\tx.classList.remove('disabled')\n        })\n\t\t\t}\n\n      const prettyNum = (num, maximumFractionDigits = 2) => {\n\t\t\t\tif (!num) return '0'\n        return new Intl.NumberFormat('en', {\n          style: 'decimal',\n          maximumFractionDigits,\n        }).format(num)\n      }\n\n      const fromSatoshis = (sats ) => {\n        return Decimal.div(sats, 100_000_000).toNumber()\n      }\n\n\t\t\tconst toSatoshis = (btc) => {\n        return Decimal.mul(btc, 100_000_000).toNumber()\n      }\n\n\t\t\tconst redirect = (path) => {\n\t\t\t\tconst url = new URL(window.location.href)\n\t\t\t\tconst params = new URLSearchParams(url.search);\n\t\t\t\tconst aspurl = params.get('aspurl')\n\t\t\t\twindow.location.href = path + (aspurl ? `/?aspurl=${aspurl}` : '')\n\t\t\t}\n\n      const copyToClipboard = (id) => {\n\t\t  \tif (!navigator.clipboard) return\n\t\t  \tconst addr = document.querySelector(id).innerText\n        navigator.clipboard.writeText(addr)\n\t\t  }\n\n      const pasteFromClipboard = (id) => {\n\t\t  \tif (navigator.clipboard) {\n          navigator.clipboard.readText().then((addr) => {\n\t\t  \t\t\tdocument.querySelector(id).value = addr\n\t\t  \t  })\n        }\n\t\t  }\n\n      const setMaxValue = (val = 0) => {\n\t\t  \tconst sats = val || document.querySelector('#amount').getAttribute('max')\n        if (isNaN(sats)) return\n\t\t  \tconst unit = document.querySelector('#unit').innerText\n\t\t\t\tdocument.querySelector('#sats').value = sats\n\t\t  \tdocument.querySelector('#amount').value = unit === 'SATS' ? sats : fromSatoshis(sats)\n\t\t\t\tdocument.querySelector('#amount').dispatchEvent(new Event('change'))\n\t\t  }\n  \n\t    // change between btc and sat\n\t\t  const toggleUnit = () => {\n\t\t\t\tconst unit = document.querySelector('#unit')\n\t\t\t\tif (!unit) return\n\t\t  \tconst currUnit = unit.innerText\n\t\t  \tconst nextUnit = currUnit === 'SATS' ? 'BTC' : 'SATS'\n\t\t  \t// change unit\n\t\t  \tif (unit) unit.innerText = nextUnit\n\t\t  \t// change availability\n\t\t\t\tconst available = document.querySelector('#available')\n\t\t\t\tif (available) {\n\t\t\t\t\tconst maxSats = document.querySelector('#amount').getAttribute('max')\n\t\t\t\t  const num = !maxSats ? 0 : (nextUnit === 'SATS' ? prettyNum(maxSats) : fromSatoshis(maxSats))\n\t\t\t\t  available.innerText = `Available ${num} ${nextUnit}`\n\t\t\t\t}\n\t\t\t  // change value inside input\n\t\t  \tconst currVal = document.querySelector('#amount').value\n\t\t\t\tif (currVal) {\n\t\t\t\t\tconst nextVal = nextUnit === 'SATS' ? toSatoshis(currVal) : fromSatoshis(currVal)\n\t\t  \t\tdocument.querySelector('#amount').value = nextVal\n\t\t  \t}\n\t\t\t\t// change steps\n\t\t\t\tdocument.querySelector('#amount').step = nextUnit === 'SATS' ? \"\" : \"0.00000001\"\n\t\t  }\n\n      // hide/show input password visibility\n      const togglePasswordVisibility = () => {\n        document.querySelectorAll(\".eyes span\").forEach((el) => {\n          el.style.display = el.style.display === 'none' ? 'block' : 'none'\n        })\n        document.querySelectorAll(\".eyeopener\").forEach((el) => {\n          el.type = el.type === 'text' ? 'password' : 'text'\n        })\n      }\n\n      // this function finds every crypto amount in the dom and make it use\n\t\t\t// the preferred unit choosen in settings: sats or btc\n\t\t\tconst updateCryptoValues = () => {\n\t\t\t\tconst settings = JSON.parse(document.querySelector('#settings').textContent)\n\t\t\t\tdocument.querySelectorAll('.cryptoAmount').forEach((x) => {\n          const sats = parseInt(x.getAttribute('sats'))\n\t      \tif (isNaN(sats)) return\n\t      \tconst suffix = settings.unit === 'sat' ? 'SATS' : 'BTC'\n          const cryptoValue = settings.unit === 'sat' ? sats : fromSatoshis(sats)\n          x.innerHTML = `${prettyNum(cryptoValue, 8)} ${suffix}`\n        })\n\t\t\t}\n  \n\t\t\t// this function finds every fiat amount in the dom and checks live\n\t\t\t// rate of the preferred currency choosen in setttings: usd or euro\n\t\t\tconst updateFiatValues = () => {\n\t\t\t\tconst settings = JSON.parse(document.querySelector('#settings').textContent)\n\t\t\t\tfetch('https://btcoracle.bordalix.workers.dev/').then((res) => {\n          res.json().then((json) => {\n            document.querySelectorAll('.fiatAmount').forEach((x) => {\n              const sats = parseInt(x.getAttribute('sats'))\n\t      \t\t\tif (isNaN(sats)) return\n\t      \t\t\tconst prefix = settings.currency === 'eur' ? '€' : '$'\n              const fiatValue = (sats * json.pricefeed[settings.currency]) / 100_000_000\n              x.innerHTML = `${prefix} ${prettyNum(fiatValue)}`\n            })\n          })\n        })\n\t\t\t}\n\n\t\t\tconst notImplemented = (event) => {\n\t\t\t\tevent.preventDefault()\n\t\t\t\tdocument.querySelector(\"#toast\").innerHTML = '<p class=\"border border-1 border border-red/20 bg-errorbg capitalize text-red px-2 py-1 rounded-md mx-auto\">Not implemented</p>'\n\t\t\t\tsetTimeout(() => document.querySelector(\"#toast\").innerHTML = \"\", toastTimeout)\n\t\t\t}\n\n      document.addEventListener('htmx:load', () => {\n        enableButtons()\n        updateFiatValues()\n\t\t\t\tupdateCryptoValues()\n\n        // pass along query parameter with aspurl\n\t\t\t\tconst url = new URL(window.location.href)\n\t\t\t  const params = new URLSearchParams(url.search);\n\t\t\t  const aspurl = params.get('aspurl')\n        if (aspurl) {\n\t\t\t\t\tconst hiddenInput = document.querySelector(\"#aspurl\")\n\t\t\t\t\tif (hiddenInput) hiddenInput.value = aspurl\n\t\t\t\t}\n      })\n\n      // after receiving a toast, delete it after 2 seconds\n      document.body.addEventListener(\"toast\", (e) => {\n        setTimeout(() => document.querySelector(\"#toast\").innerHTML = \"\", toastTimeout)\n      })\n\t\t</script></html>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

var _ = templruntime.GeneratedTemplate
