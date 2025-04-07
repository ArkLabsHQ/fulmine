// Code generated by templ - DO NOT EDIT.

// templ: version: v0.3.857
package templates

//lint:file-ignore SA4006 This context is only used if a nested component is present.

import "github.com/a-h/templ"
import templruntime "github.com/a-h/templ/runtime"

import (
	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
)

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
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 1, "<!doctype html><html lang=\"en\"><head><meta charset=\"UTF-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\"><meta http-equiv=\"X-UA-Compatible\" content=\"ie=edge\"><title>Fulmine - Bitcoin Wallet Daemon</title><meta name=\"description\" content=\"Fulmine is a Bitcoin wallet daemon that integrates Ark protocol&#39;s batched transaction model with Lightning Network infrastructure, enabling routing nodes, service providers and payment hubs to optimize channel liquidity while minimizing on-chain fees, without compromising on self-custody.\"><meta name=\"theme-color\" content=\"#FEFEF5\"><!-- Facebook Meta Tags --><meta property=\"og:url\" content=\"https://fulmine.arkade.sh\"><meta property=\"og:type\" content=\"website\"><meta property=\"og:title\" content=\"Fulmine - Bitcoin Wallet Daemon\"><meta property=\"og:description\" content=\"Fulmine is a Bitcoin wallet daemon that integrates Ark protocol&#39;s batched transaction model with Lightning Network infrastructure, enabling routing nodes, service providers and payment hubs to optimize channel liquidity while minimizing on-chain fees, without compromising on self-custody.\"><meta property=\"og:image\" content=\"/static/og-image.png\"><!-- Twitter Meta Tags --><meta name=\"twitter:card\" content=\"summary_large_image\"><meta property=\"twitter:domain\" content=\"fulmine.arkade.sh\"><meta property=\"twitter:url\" content=\"https://fulmine.arkade.sh\"><meta name=\"twitter:title\" content=\"Fulmine - Bitcoin Wallet Daemon\"><meta name=\"twitter:description\" content=\"Fulmine is a Bitcoin wallet daemon that integrates Ark protocol&#39;s batched transaction model with Lightning Network infrastructure, enabling routing nodes, service providers and payment hubs to optimize channel liquidity while minimizing on-chain fees, without compromising on self-custody.\"><meta name=\"twitter:image\" content=\"/static/og-image.png\"><link rel=\"dns-prefetch\" href=\"//fonts.googleapis.com\"><link rel=\"dns-prefetch\" href=\"//fonts.gstatic.com\"><link rel=\"preconnect\" href=\"//fonts.googleapis.com\" crossorigin><link rel=\"preconnect\" href=\"//fonts.gstatic.com\" crossorigin><link rel=\"preload\" href=\"/static/Switzer-Variable.17d61fe9.woff2\" as=\"font\" type=\"font/woff2\" crossorigin><link rel=\"preload\" href=\"/static/Switzer-Variable.f14a5147.woff\" as=\"font\" type=\"font/woff\" crossorigin><link rel=\"preload\" href=\"/static/Switzer-Variable.398e4d30.ttf\" as=\"font\" type=\"font/ttf\" crossorigin><link rel=\"manifest\" href=\"/static/manifest.webmanifest\"><link rel=\"apple-touch-icon\" href=\"/static/apple-touch-icon.png\"><link rel=\"shortcut icon\" href=\"/static/favicon.ico\" type=\"image/x-icon\"><link rel=\"icon\" href=\"/static/favicon.svg\" type=\"image/svg+xml\"><link rel=\"icon\" href=\"/static/favicon.png\" sizes=\"any\"><link href=\"/static/styles.css\" rel=\"stylesheet\"><script src=\"/static/htmx@2.0.4.min.js\"></script><script src=\"/static/htmx-ext-sse@2.2.3-patch.js\"></script><script src=\"/static/decimal.js\"></script><script src=\"/static/qr.js\"></script></head><body>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templ.JSONScript("settings", settings).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 2, "<div id=\"app\"><div id=\"toast\"></div><div id=\"modal\"></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = bodyContent.Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 3, "</div></body><script>\n\t\t\tconst toastTimeout = 2100\n\t\t\n\t\t\tconst copyToClipboard = (element, event) => {\n\t\t\t\tevent.preventDefault()\n\t\t\t\tif (!navigator.clipboard) return\n\t\t\t\tconst value = element.dataset.value\n\t\t\t\tif (!value) return\n\t\t\t\tnavigator.clipboard.writeText(value)\n\t\t\t\t// change the button text to \"Copied\"\n\t\t\t\tbuttonText = element.querySelector('span.buttonText')\n\t\t\t\tif (buttonText) {\n\t\t\t\t\tconst previousText = buttonText.innerText\n\t\t\t\t\tbuttonText.innerText = 'Copied'\n\t\t\t\t\tsetTimeout(() => buttonText.innerText = previousText, 2100)\n\t\t\t\t}\n\t\t\t\t// update icons if present\n\t\t\t\tconst c = element.querySelector(\"[data-kind='copy']\")\n\t\t\t\tconst d = element.querySelector(\"[data-kind='done']\")\n\t\t\t\tif (c && d) {\n\t\t\t\t\tc.classList.add(\"hidden\")\n\t\t\t\t\td.classList.remove(\"hidden\")\n\t\t\t\t\tsetTimeout(() => {\n\t\t\t\t\t\tc.classList.remove(\"hidden\")\n\t\t\t\t\t\td.classList.add(\"hidden\")\n\t\t\t\t\t}, 1000)\n\t\t\t\t}\n\t\t\t}\n\t\t\n\t\t\tconst disableButton = (button) => {\n\t\t\t\tbutton.classList.add(\"disabled\")\n\t\t\t}\n\t\t\t\n\t\t\tconst enableButtons = () => {\n\t\t\t\tdocument.querySelectorAll('.disabled').forEach((x) => {\n\t\t\t\t\tx.classList.remove('disabled')\n\t\t\t\t})\n\t\t\t}\n\t\t\n\t\t\tconst fromSatoshis = (sats) => {\n\t\t\t\treturn Decimal.div(sats, 100_000_000).toNumber()\n\t\t\t}\n\t\t\n\t\t\tconst handleCancel = (event) => {\n\t\t\t\tevent.preventDefault()\n\t\t\t\tredirect('/')\n\t\t\t}\n\t\t\n\t\t\tconst hideCopyButtonsIfInsecure = () => {\n\t\t\t\tif (!navigator.clipboard) {\n\t\t\t\t\tdocument.querySelectorAll('[data-needs=\"clipboard\"]').forEach((el) => {\n\t\t\t\t\t\tel.classList.add('hidden')\n\t\t\t\t\t})\n\t\t\t\t}\n\t\t\t}\n\n\t\t\tconst hideScanButtonsIfNoCameraAvailable = async () => {\n\t\t\t\tif (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {\n\t\t\t\t\tdocument.querySelectorAll('[data-needs=\"camera\"]').forEach((el) => {\n\t\t\t\t\t\tel.classList.add('hidden')\n\t\t\t\t\t})\n\t\t\t\t}\n\t\t\t}\n\n\t\t\t// pass along query parameter with serverUrl\n\t\t\tconst passAlongQueryParams = () => {\n\t\t\t\tconst url = new URL(window.location.href)\n\t\t\t\tconst params = new URLSearchParams(url.search);\n\t\t\t\tconst serverUrl = params.get('serverurl')\n\t\t\t\tif (serverUrl) {\n\t\t\t\t\tconst hiddenInput = document.querySelector('[name=\"urlOnQuery\"]')\n\t\t\t\t\tif (hiddenInput) hiddenInput.value = serverUrl\n\t\t\t\t}\n\t\t\t}\n\t\t\n\t\t\tconst pasteFromClipboard = (element, event) => {\n\t\t\t\tevent.preventDefault()\n\t\t\t\tconst id = element.dataset.id\n\t\t\t\tif (id && navigator.clipboard) {\n\t\t\t\t\tnavigator.clipboard.readText().then((data) => {\n\t\t\t\t\t\tdocument.getElementById(id).value = data\n\t\t\t\t\t})\n\t\t\t\t}\n\t\t\t}\n\t\t\n\t\t\tconst prettyNum = (num, maximumFractionDigits = 2) => {\n\t\t\t\tif (!num) return '0'\n\t\t\t\treturn new Intl.NumberFormat('en', {\n\t\t\t\t\tstyle: 'decimal',\n\t\t\t\t\tmaximumFractionDigits,\n\t\t\t\t}).format(num)\n\t\t\t}\n\t\t\n\t\t\tconst redirect = (path) => {\n\t\t\t\tconst url = new URL(window.location.href)\n\t\t\t\tconst params = new URLSearchParams(url.search);\n\t\t\t\tconst serverUrl = params.get('serverurl')\n\t\t\t\twindow.location.href = path + (serverUrl ? `/?serverurl=${serverUrl}` : '')\n\t\t\t}\n\n\t\t\tlet scanClose = () => {}\n\n\t\t\tconst scanStart = async (id) => {\n\t\t\t\tconst title = document.getElementById('qr-title')\n\t\t\t\tconst video = document.getElementById('qr-video')\n\t\t\t\tconst closeModal = () => document.getElementById('modalContainer').style.display = 'none'\n\t\t\t\t// scan qr code\n\t\t\t\ttry {\n\t\t\t\t\tconst { frontalCamera, QRCanvas, frameLoop, getSize } = qr.dom\n\t\t\t\t\tawait navigator.mediaDevices.getUserMedia({ video: true })\n\t\t\t\t\ttitle.innerText = 'Scan QR Code'\n\t\t\t\t\tconst canvas = new QRCanvas()\n\t\t\t\t\tconst camera = await frontalCamera(video)\n\t\t\t\t\tconst devices = await camera.listDevices()\n\t\t\t\t\tawait camera.setDevice(devices[devices.length - 1].deviceId)\n\t\t\t\t\tconst cancel = frameLoop(() => {\n\t\t\t\t\t\tconst res = camera.readFrame(canvas)\n\t\t\t\t\t\tif (res) {\n\t\t\t\t\t\t\tdocument.getElementById(id).value = res\n\t\t\t\t\t\t\tif (camera) camera.stop()\n\t\t\t\t\t\t\tif (cancel) cancel()\n\t\t\t\t\t\t\tcancel()\n\t\t\t\t\t\t}\n\t\t\t\t\t})\n\t\t\t\t\t// func to close modal and stop camera\n\t\t\t\t\tscanClose = () => {\n\t\t\t\t\t\tif (camera) camera.stop()\n\t\t\t\t\t\tif (cancel) cancel()\n\t\t\t\t\t\tcloseModal()\n\t\t\t\t\t}\n\t\t\t\t} catch {\n\t\t\t\t\ttitle.innerText = 'Camera access denied'\n\t\t\t\t}\n\t\t\t}\n\t\t\t\n\t\t\tconst setMaxValue = (val = 0) => {\n\t\t\t\tconst sats = val || document.querySelector('#amount').getAttribute('max')\n\t\t\t\tif (isNaN(sats)) return\n\t\t\t\tconst unit = document.querySelector('#unit').innerText\n\t\t\t\tdocument.querySelector('#sats').value = sats\n\t\t\t\tdocument.querySelector('#amount').value = unit === 'SATS' ? sats : fromSatoshis(sats)\n\t\t\t\tdocument.querySelector('#amount').dispatchEvent(new Event('change'))\n\t\t\t}\n\t\t\n\t\t\tconst showOptions = () => {\n\t\t\t\tevent.stopPropagation()\n\t\t\t\tconst options = document.getElementById('options')\n\t\t\t\toptions.classList.toggle('hidden')\n\t\t\t}\n\t\t\t\n\t\t\t// redirect user to tx page\n\t\t\tconst showTx = (el) => {\n\t\t\t\tconst txid = el.getAttribute(\"txid\")\n\t\t\t\tif (txid) redirect(`/tx/${txid}`)\n\t\t\t}\n\t\t\n\t\t\t// change between btc and sat\n\t\t\tconst toggleUnit = () => {\n\t\t\t\tconst unit = document.querySelector('#unit')\n\t\t\t\tif (!unit) return\n\t\t\t\tconst currUnit = unit.innerText\n\t\t\t\tconst nextUnit = currUnit === 'SATS' ? 'BTC' : 'SATS'\n\t\t\t\t// change unit\n\t\t\t\tif (unit) unit.innerText = nextUnit\n\t\t\t\t// change availability\n\t\t\t\tconst available = document.querySelector('#available')\n\t\t\t\tif (available) {\n\t\t\t\t\tconst maxSats = document.querySelector('#amount').getAttribute('max')\n\t\t\t\t\tconst num = !maxSats ? 0 : (nextUnit === 'SATS' ? prettyNum(maxSats) : fromSatoshis(maxSats))\n\t\t\t\t\tavailable.innerText = `Available ${num} ${nextUnit}`\n\t\t\t\t}\n\t\t\t\t// change value inside input\n\t\t\t\tconst currVal = document.querySelector('#amount').value\n\t\t\t\tif (currVal) {\n\t\t\t\t\tconst nextVal = nextUnit === 'SATS' ? toSatoshis(currVal) : fromSatoshis(currVal)\n\t\t\t\t\tdocument.querySelector('#amount').value = nextVal\n\t\t\t\t}\n\t\t\t\t// change steps\n\t\t\t\tdocument.querySelector('#amount').step = nextUnit === 'SATS' ? \"\" : \"0.00000001\"\n\t\t\t}\n\t\t\n\t\t\t// hide/show input password visibility\n\t\t\tconst togglePasswordVisibility = () => {\n\t\t\t\tdocument.querySelectorAll(\".eyes span\").forEach((el) => {\n\t\t\t\t\tel.style.display = el.style.display === 'none' ? 'block' : 'none'\n\t\t\t\t})\n\t\t\t\tdocument.querySelectorAll(\".eyeopener\").forEach((el) => {\n\t\t\t\t\tel.type = el.type === 'text' ? 'password' : 'text'\n\t\t\t\t})\n\t\t\t}\n\t\t\n\t\t\t// return a bitcoin amount in satoshis\n\t\t\tconst toSatoshis = (btc) => {\n\t\t\t\treturn Decimal.mul(btc, 100_000_000).toNumber()\n\t\t\t}\n\t\t\n\t\t\t// this function finds every crypto amount in the dom and make it use\n\t\t\t// the preferred unit choosen in settings: sats or btc\n\t\t\tconst updateCryptoValues = () => {\n\t\t\t\tconst settings = JSON.parse(document.querySelector('#settings').textContent)\n\t\t\t\tdocument.querySelectorAll('.cryptoAmount').forEach((x) => {\n\t\t\t\t\tconst sats = parseInt(x.getAttribute('sats'))\n\t\t\t\t\tif (isNaN(sats)) return\n\t\t\t\t\tconst suffix = settings.Unit === 'sat' ? 'SATS' : 'BTC'\n\t\t\t\t\tconst cryptoValue = settings.Unit === 'sat' ? sats : fromSatoshis(sats)\n\t\t\t\t\tx.innerHTML = `${prettyNum(cryptoValue, 8)} ${suffix}`\n\t\t\t\t})\n\t\t\t}\n\t\t\n\t\t\t// this function finds every fiat amount in the dom and checks live\n\t\t\t// rate of the preferred currency choosen in setttings: usd or euro\n\t\t\tconst updateFiatValues = () => {\n\t\t\t\tconst settings = JSON.parse(document.querySelector('#settings').textContent)\n\t\t\t\tfetch('https://blockchain.info/ticker').then((res) => {\n\t\t\t\t\tres.json().then((json) => {\n\t\t\t\t\t\tdocument.querySelectorAll('.fiatAmount').forEach((x) => {\n\t\t\t\t\t\t\tconst sats = parseInt(x.getAttribute('sats'))\n\t\t\t\t\t\t\tif (isNaN(sats)) return\n\t\t\t\t\t\t\tconst prefix = settings.Currency === 'eur' ? '€' : '$'\n\t\t\t\t\t\t\tconst fiatValue = (sats * json[settings.Currency.toUpperCase()]?.last) / 100_000_000\n\t\t\t\t\t\t\tx.innerHTML = `${prefix} ${prettyNum(fiatValue)}`\n\t\t\t\t\t\t})\n\t\t\t\t\t})\n\t\t\t\t})\n\t\t\t}\n\t\t\n\t\t\tdocument.addEventListener('htmx:load', () => {\n\t\t\t\tenableButtons()\n\t\t\t\tupdateFiatValues()\n\t\t\t\tupdateCryptoValues()\n\t\t\t})\n\n\t\t\tdocument.addEventListener('load', () => {\n\t\t\t\tpassAlongQueryParams()\n\t\t\t\thideCopyButtonsIfInsecure()\n\t\t\t\thideScanButtonsIfNoCameraAvailable()\n\t\t\t\t// hack, or else select will not have current value selected\n\t\t\t\tdocument.querySelectorAll('select').forEach((x) => {\n\t\t\t\t\tif (x.getAttribute('value')) x.value = x.getAttribute('value')\n\t\t\t\t})\n\t\t\t})\n\t\t\n\t\t\t// after receiving a toast, delete it after 2 seconds\n\t\t\tdocument.body.addEventListener(\"toast\", (e) => {\n\t\t\t\tsetTimeout(() => document.querySelector(\"#toast\").innerHTML = \"\", toastTimeout)\n\t\t\t})\n  \n  \n\t\t</script></html>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return nil
	})
}

var _ = templruntime.GeneratedTemplate
