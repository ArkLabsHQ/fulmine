// Code generated by templ - DO NOT EDIT.

// templ: version: v0.3.857
package templates

//lint:file-ignore SA4006 This context is only used if a nested component is present.

import "github.com/a-h/templ"
import templruntime "github.com/a-h/templ/runtime"

import (
	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
)

func runOnLoad() templ.ComponentScript {
	return templ.ComponentScript{
		Name: `__templ_runOnLoad_dc70`,
		Function: `function __templ_runOnLoad_dc70(){updateFiatValues()
//
document.querySelectorAll('select').forEach((x) => {
// hack, or else select will not have current value selected
if (x.getAttribute('value')) x.value = x.getAttribute('value')
})
// check for bitcoin unit (btc or sat)
const settings = JSON.parse(document.querySelector('#settings').textContent)
if (settings.Unit !== "sat") toggleUnit()
}`,
		Call:       templ.SafeScript(`__templ_runOnLoad_dc70`),
		CallInline: templ.SafeScriptInline(`__templ_runOnLoad_dc70`),
	}
}

func Layout(bodyContent templ.Component, settings domain.Settings) templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
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
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 1, "<!doctype html><html lang=\"en\"><head><title>Fulmine</title><meta charset=\"UTF-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><meta http-equiv=\"X-UA-Compatible\" content=\"ie=edge\"><meta name=\"keywords\" content=\"Ark\"><meta name=\"description\" content=\"Fulmine frontend\"><meta name=\"theme-color\" content=\"#FEFEF5\"><link rel=\"dns-prefetch\" href=\"//fonts.googleapis.com\"><link rel=\"dns-prefetch\" href=\"//fonts.gstatic.com\"><link rel=\"preconnect\" href=\"//fonts.googleapis.com\" crossorigin><link rel=\"preconnect\" href=\"//fonts.gstatic.com\" crossorigin><link rel=\"preload\" href=\"/static/Switzer-Variable.17d61fe9.woff2\" as=\"font\" type=\"font/woff2\" crossorigin><link rel=\"preload\" href=\"/static/Switzer-Variable.f14a5147.woff\" as=\"font\" type=\"font/woff\" crossorigin><link rel=\"preload\" href=\"/static/Switzer-Variable.398e4d30.ttf\" as=\"font\" type=\"font/ttf\" crossorigin><link rel=\"manifest\" href=\"/static/manifest.webmanifest\"><link rel=\"apple-touch-icon\" href=\"/static/apple-touch-icon.png\"><link rel=\"shortcut icon\" href=\"/static/favicon.ico\" type=\"image/x-icon\"><link rel=\"icon\" href=\"/static/favicon.svg\" type=\"image/svg+xml\"><link rel=\"icon\" href=\"/static/favicon.png\" sizes=\"any\"><link href=\"/static/styles.css\" rel=\"stylesheet\"><script src=\"/static/htmx@2.0.4.min.js\"></script><script src=\"/static/htmx-ext-sse@2.2.3-patch.js\"></script><script src=\"/static/decimal.js\"></script><script src=\"/static/scripts.js\"></script></head>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templ.RenderScriptItems(ctx, templ_7745c5c3_Buffer, runOnLoad())
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 2, "<body onload=\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var2 templ.ComponentScript = runOnLoad()
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var2.Call)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 3, "\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templ.JSONScript("settings", settings).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 4, "<div id=\"app\"><div id=\"toast\"></div><div id=\"modal\"></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = bodyContent.Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 5, "</div></body><script>\n\tconst toastTimeout = 2100\n\n\tconst disableButton = (button) => {\n\t\tbutton.classList.add(\"disabled\")\n\t}\n\n\t// re enable buttons after receiving a htmx:load event\n\tconst enableButtons = () => {\n\t\tdocument.querySelectorAll('.disabled').forEach((x) => {\n\t\t\tx.classList.remove('disabled')\n\t\t})\n\t}\n\n\tconst prettyNum = (num, maximumFractionDigits = 2) => {\n\t\tif (!num) return '0'\n\t\treturn new Intl.NumberFormat('en', {\n\t\t\tstyle: 'decimal',\n\t\t\tmaximumFractionDigits,\n\t\t}).format(num)\n\t}\n\n\tconst fromSatoshis = (sats) => {\n\t\treturn Decimal.div(sats, 100_000_000).toNumber()\n\t}\n\n\tconst toSatoshis = (btc) => {\n\t\treturn Decimal.mul(btc, 100_000_000).toNumber()\n\t}\n\n\tconst redirect = (path) => {\n\t\tconst url = new URL(window.location.href)\n\t\tconst params = new URLSearchParams(url.search);\n\t\tconst serverUrl = params.get('serverurl')\n\t\twindow.location.href = path + (serverUrl ? `/?serverurl=${serverUrl}` : '')\n\t}\n\n\tconst copyToClipboard = (id) => {\n\t\tif (!navigator.clipboard) return\n\t\tconst addr = document.querySelector(id).innerText\n\t\tnavigator.clipboard.writeText(addr)\n\t}\n\n\tconst pasteFromClipboard = (id) => {\n\t\tif (navigator.clipboard) {\n\t\t\tnavigator.clipboard.readText().then((addr) => {\n\t\t\t\tdocument.querySelector(id).value = addr\n\t\t\t})\n\t\t}\n\t}\n\n\tconst setMaxValue = (val = 0) => {\n\t\tconst sats = val || document.querySelector('#amount').getAttribute('max')\n\t\tif (isNaN(sats)) return\n\t\tconst unit = document.querySelector('#unit').innerText\n\t\tdocument.querySelector('#sats').value = sats\n\t\tdocument.querySelector('#amount').value = unit === 'SATS' ? sats : fromSatoshis(sats)\n\t\tdocument.querySelector('#amount').dispatchEvent(new Event('change'))\n\t}\n\n\t// change between btc and sat\n\tconst toggleUnit = () => {\n\t\tconst unit = document.querySelector('#unit')\n\t\tif (!unit) return\n\t\tconst currUnit = unit.innerText\n\t\tconst nextUnit = currUnit === 'SATS' ? 'BTC' : 'SATS'\n\t\t// change unit\n\t\tif (unit) unit.innerText = nextUnit\n\t\t// change availability\n\t\tconst available = document.querySelector('#available')\n\t\tif (available) {\n\t\t\tconst maxSats = document.querySelector('#amount').getAttribute('max')\n\t\t\tconst num = !maxSats ? 0 : (nextUnit === 'SATS' ? prettyNum(maxSats) : fromSatoshis(maxSats))\n\t\t\tavailable.innerText = `Available ${num} ${nextUnit}`\n\t\t}\n\t\t// change value inside input\n\t\tconst currVal = document.querySelector('#amount').value\n\t\tif (currVal) {\n\t\t\tconst nextVal = nextUnit === 'SATS' ? toSatoshis(currVal) : fromSatoshis(currVal)\n\t\t\tdocument.querySelector('#amount').value = nextVal\n\t\t}\n\t\t// change steps\n\t\tdocument.querySelector('#amount').step = nextUnit === 'SATS' ? \"\" : \"0.00000001\"\n\t}\n\n\t// hide/show input password visibility\n\tconst togglePasswordVisibility = () => {\n\t\tdocument.querySelectorAll(\".eyes span\").forEach((el) => {\n\t\t\tel.style.display = el.style.display === 'none' ? 'block' : 'none'\n\t\t})\n\t\tdocument.querySelectorAll(\".eyeopener\").forEach((el) => {\n\t\t\tel.type = el.type === 'text' ? 'password' : 'text'\n\t\t})\n\t}\n\n\t// this function finds every crypto amount in the dom and make it use\n\t// the preferred unit choosen in settings: sats or btc\n\tconst updateCryptoValues = () => {\n\t\tconst settings = JSON.parse(document.querySelector('#settings').textContent)\n\t\tdocument.querySelectorAll('.cryptoAmount').forEach((x) => {\n\t\t\tconst sats = parseInt(x.getAttribute('sats'))\n\t\t\tif (isNaN(sats)) return\n\t\t\tconst suffix = settings.Unit === 'sat' ? 'SATS' : 'BTC'\n\t\t\tconst cryptoValue = settings.Unit === 'sat' ? sats : fromSatoshis(sats)\n\t\t\tx.innerHTML = `${prettyNum(cryptoValue, 8)} ${suffix}`\n\t\t})\n\t}\n\n\t// this function finds every fiat amount in the dom and checks live\n\t// rate of the preferred currency choosen in setttings: usd or euro\n\tconst updateFiatValues = () => {\n\t\tconst settings = JSON.parse(document.querySelector('#settings').textContent)\n\t\tfetch('https://blockchain.info/ticker').then((res) => {\n\t\t\tres.json().then((json) => {\n\t\t\t\tdocument.querySelectorAll('.fiatAmount').forEach((x) => {\n\t\t\t\t\tconst sats = parseInt(x.getAttribute('sats'))\n\t\t\t\t\tif (isNaN(sats)) return\n\t\t\t\t\tconst prefix = settings.Currency === 'eur' ? '€' : '$'\n\t\t\t\t\tconst fiatValue = (sats * json[settings.Currency.toUpperCase()]?.last) / 100_000_000\n\t\t\t\t\tx.innerHTML = `${prefix} ${prettyNum(fiatValue)}`\n\t\t\t\t})\n\t\t\t})\n\t\t})\n\t}\n\n\tconst notImplemented = (event) => {\n\t\tevent.preventDefault()\n\t\tdocument.querySelector(\"#toast\").innerHTML = '<p class=\"border border-1 border border-red/20 bg-errorbg capitalize text-red px-2 py-1 rounded-md mx-auto\">Not implemented</p>'\n\t\tsetTimeout(() => document.querySelector(\"#toast\").innerHTML = \"\", toastTimeout)\n\t}\n\n\tconst showTx = (el) => {\n\t\tconst txid = el.getAttribute(\"txid\")\n\t\tif (txid) redirect(`/tx/${txid}`)\n\t}\n\n\tdocument.addEventListener('htmx:load', () => {\n\t\tenableButtons()\n\t\tupdateFiatValues()\n\t\tupdateCryptoValues()\n\n\t\t// pass along query parameter with serverUrl\n\t\tconst url = new URL(window.location.href)\n\t\tconst params = new URLSearchParams(url.search);\n\t\tconst serverUrl = params.get('serverurl')\n\t\tif (serverUrl) {\n\t\t\tconst hiddenInput = document.querySelector('[name=\"urlOnQuery\"]')\n\t\t\tif (hiddenInput) hiddenInput.value = serverUrl\n\t\t}\n\t})\n\n\t// after receiving a toast, delete it after 2 seconds\n\tdocument.body.addEventListener(\"toast\", (e) => {\n\t\tsetTimeout(() => document.querySelector(\"#toast\").innerHTML = \"\", toastTimeout)\n\t})\n\n\n</script></html>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return nil
	})
}

var _ = templruntime.GeneratedTemplate
